// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"
	stdlibtime "time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen,gocognit,revive // .
func (s *Scheduler) runPushAnnouncementsProcessor(ctx context.Context, workerNumber int64) {
	var (
		batchNumber                 int64
		now, lastIterationStartedAt = time.Now(), time.Now()
		errs                        = make([]error, 0)
		successedAnnouncements      = make([]*broadcastPushNotification[push.Notification[push.SubscriptionTopic]], 0, schedulerPushBatchSize)
		announcements               = make([]*scheduledAnnouncement, schedulerPushBatchSize)
		toSendAnnouncements         = make([]*broadcastPushNotification[push.Notification[push.SubscriptionTopic]], 0, schedulerPushBatchSize)
		err                         error
	)
	resetVars := func(success bool) {
		if success && len(announcements) < int(schedulerPushBatchSize) {
			go s.telemetryAnnouncements.collectElapsed(0, *lastIterationStartedAt.Time)
			batchNumber = 0
		}
		now = time.Now()
		errs = errs[:0]
		toSendAnnouncements = toSendAnnouncements[:0]
		announcements = announcements[:0]
		successedAnnouncements = successedAnnouncements[:0]
		lastIterationStartedAt = now

		stdlibtime.Sleep(1 * stdlibtime.Second)
	}
	for ctx.Err() == nil {
		/******************************************************************************************************************************************************
			1. Fetching a new batch of scheduled announcements.
		******************************************************************************************************************************************************/
		before := time.Now()
		reqCtx, reqCancel := context.WithTimeout(ctx, requestDeadline)
		announcements, err = s.fetchScheduledAnnouncements(reqCtx, now, workerNumber)
		if err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to fetch scheduled announcements for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)

			continue
		}
		if len(announcements) > 0 {
			go s.telemetryAnnouncements.collectElapsed(1, *before.Time)
		}
		reqCancel()

		/******************************************************************************************************************************************************
			2. Processing batch of announcements.
		******************************************************************************************************************************************************/
		before = time.Now()
		for _, an := range announcements {
			tmpl, found := allPushNotificationTemplates[NotificationType(an.NotificationType)][an.Language]
			if !found {
				log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", an.Language, an.NotificationType))
				if tmpl, found = allPushNotificationTemplates[NotificationType(an.NotificationType)][defaultLanguage]; !found {
					log.Panic(fmt.Sprintf("no default translations provided for announcement, lang:%v, notificationType:%v", an.Language, an.NotificationType))
				}
			}
			deeplink := s.getDeeplink(NotificationType(an.NotificationType), an.Data)
			toSendAnnouncements = append(toSendAnnouncements, &broadcastPushNotification[push.Notification[push.SubscriptionTopic]]{
				pn: &push.Notification[push.SubscriptionTopic]{
					Data:   map[string]string{"deeplink": deeplink},
					Target: push.SubscriptionTopic(an.NotificationChannelValue),
					Title:  tmpl.getTitle(an.Data),
					Body:   tmpl.getBody(an.Data),
				},
				sa: &sentAnnouncement{
					SentAt:   now,
					Language: an.Language,
					sentAnnouncementPK: sentAnnouncementPK{
						Uniqueness:               an.Uniqueness,
						NotificationType:         NotificationType(an.NotificationType),
						NotificationChannel:      PushNotificationChannel,
						NotificationChannelValue: an.NotificationChannelValue,
					},
				},
			})
		}

		/******************************************************************************************************************************************************
			3. Send all announcements concurrently.
		******************************************************************************************************************************************************/
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if rErr := runConcurrentlyBatch(reqCtx, s.broadcastPushNotification, toSendAnnouncements, func(arg *broadcastPushNotification[push.Notification[push.SubscriptionTopic]], err error) { //nolint:lll // .
			log.Error(errors.Wrapf(err, "can't send announcement for lang:%v, notificationChannel:%v, notificationChannelValue:%v, notificationType:%v", arg.sa.Language, arg.sa.NotificationChannel, arg.sa.NotificationChannelValue, arg.sa.NotificationType)) //nolint:lll // .
		}, func(arg *broadcastPushNotification[push.Notification[push.SubscriptionTopic]]) {
			s.schedulerPushAnnouncementsMX.Lock()
			defer s.schedulerPushAnnouncementsMX.Unlock()

			successedAnnouncements = append(successedAnnouncements, arg)
		}); rErr != nil {
			log.Error(errors.Wrapf(rErr, "[scheduler] failed to sending announcements concurrently for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber)) //nolint:lll // .
		}
		reqCancel()
		if len(toSendAnnouncements) > 0 {
			go s.telemetryAnnouncements.collectElapsed(2, *before.Time) //nolint:gomnd,mnd // .
		}

		/******************************************************************************************************************************************************
			4. Delete announcements from scheduled.
		******************************************************************************************************************************************************/
		before = time.Now()
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if dErr := s.markScheduledAnnouncementAsSent(reqCtx, now, successedAnnouncements); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't delete scheduled announcements"))
		}
		reqCancel()

		if err = multierror.Append(nil, errs...).ErrorOrNil(); err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to delete scheduled announcements for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)

			continue
		}
		if len(successedAnnouncements) > 0 {
			go s.telemetryAnnouncements.collectElapsed(3, *before.Time) //nolint:gomnd,mnd // .
		}

		batchNumber++
		resetVars(true)
	}
}

func (s *Scheduler) broadcastPushNotification(
	ctx context.Context, bpn *broadcastPushNotification[push.Notification[push.SubscriptionTopic]],
) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}

	return errors.Wrapf(s.pushNotificationsClient.Broadcast(ctx, bpn.pn), "can't broadcast push notification for:%#v", bpn)
}

func (s *Scheduler) fetchScheduledAnnouncements(ctx context.Context, now *time.Time, workerNumber int64) (resp []*scheduledAnnouncement, err error) {
	sql := fmt.Sprintf(`SELECT *
							FROM scheduled_announcements
						WHERE MOD(i, %[1]v) = %[2]v
					  		  AND scheduled_for <= $1
						ORDER BY MOD(i, %[1]v), scheduled_for ASC
						LIMIT %[3]v;`, schedulerWorkersCount, workerNumber, schedulerPushBatchSize)
	resp, err = storage.ExecMany[scheduledAnnouncement](ctx, s.db, sql, now.Time)

	return resp, errors.Wrapf(err, "failed to fetch scheduled announcements for worker:%v", workerNumber)
}

func (s *Scheduler) markScheduledAnnouncementAsSent(
	ctx context.Context, now *time.Time, announcements []*broadcastPushNotification[push.Notification[push.SubscriptionTopic]],
) error {
	if len(announcements) == 0 {
		return nil
	}
	const fieldNumber = 6
	values := make([]string, 0, fieldNumber*len(announcements))
	params := make([]any, 0, fieldNumber*len(announcements))
	for idx, an := range announcements {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v, $%[6]v)", idx*fieldNumber+1, idx*fieldNumber+2, idx*fieldNumber+3, idx*fieldNumber+4, idx*fieldNumber+5, idx*fieldNumber+6)) //nolint:mnd,gomnd,lll // .
		params = append(params, now.Time, an.sa.Language, an.sa.Uniqueness, an.sa.NotificationType, an.sa.NotificationChannel, an.sa.NotificationChannelValue)
	}
	sql := fmt.Sprintf(`WITH sent AS (
		INSERT INTO sent_announcements(sent_at, language, uniqueness, notification_type, notification_channel, notification_channel_value)
			VALUES %[1]v
			ON CONFLICT DO NOTHING
			RETURNING uniqueness, notification_type, notification_channel, notification_channel_value
		)
		DELETE FROM scheduled_announcements 
			WHERE (uniqueness, notification_type, notification_channel, notification_channel_value) IN (SELECT * FROM sent)`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, s.db, sql, params...)

	return errors.Wrapf(err, "failed to delete scheduled announcements %#v", announcements)
}

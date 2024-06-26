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

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (s *Scheduler) runPushNotificationsProcessor(ctx context.Context, workerNumber int64) {
	var (
		batchNumber                 int64
		now, lastIterationStartedAt = time.Now(), time.Now()
		errs                        = make([]error, 0)
		successedNotifications      = make([]*sentNotification, 0, schedulerPushBatchSize)
		emptyTokensNotifications    = make([]*scheduledNotification, 0, schedulerPushBatchSize)
		invalidTokens               = make([]*invalidToken, 0, schedulerPushBatchSize)
		toSendPushNotifications     = make([]*pushNotification, 0, schedulerPushBatchSize)
		notifications               = make([]*scheduledNotificationInfo, schedulerPushBatchSize)
		err                         error
	)
	resetVars := func(success bool) {
		if success && len(notifications) < int(schedulerPushBatchSize) {
			go s.telemetryPushNotifications.collectElapsed(0, *lastIterationStartedAt.Time)
			batchNumber = 0
		}
		now = time.Now()
		errs = errs[:0]
		successedNotifications = successedNotifications[:0]
		toSendPushNotifications = toSendPushNotifications[:0]
		invalidTokens = invalidTokens[:0]
		notifications = notifications[:0]
		emptyTokensNotifications = emptyTokensNotifications[:0]
		lastIterationStartedAt = now

		stdlibtime.Sleep(1 * stdlibtime.Second)
	}

	for ctx.Err() == nil {
		/******************************************************************************************************************************************************
			1. Fetching a new batch of scheduled messages.
		******************************************************************************************************************************************************/
		before := time.Now()
		reqCtx, reqCancel := context.WithTimeout(ctx, requestDeadline)
		notifications, err = s.fetchScheduledNotifications(reqCtx, now, PushNotificationChannel, schedulerPushBatchSize, workerNumber)
		if err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to fetch scheduled notifications for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)

			continue
		}
		if len(notifications) > 0 {
			go s.telemetryPushNotifications.collectElapsed(1, *before.Time)
		}
		reqCancel()

		/******************************************************************************************************************************************************
			2. Processing batch of notifications.
		******************************************************************************************************************************************************/
		before = time.Now()
	out:
		for _, notification := range notifications {
			tmpl, found := allPushNotificationTemplates[NotificationType(notification.NotificationType)][notification.Language]
			if !found {
				log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", notification.Language, notification.NotificationType))

				continue
			}
			if notification.DisabledPushNotificationDomains != nil {
				domain := getDomainByNotificationType(NotificationType(notification.NotificationType))
				for _, disabledDomain := range *notification.DisabledPushNotificationDomains {
					if disabledDomain == domain {
						log.Warn(fmt.Sprintf("notification with disabled notification domain:%v with notification type:%v for notification:%#v", domain, notification.NotificationType, notification)) //nolint:lll // .

						continue out
					}
				}
			}
			if notification.PushNotificationTokens == nil || len(*notification.PushNotificationTokens) == 0 {
				emptyTokensNotifications = append(emptyTokensNotifications, &notification.scheduledNotification)

				continue
			}
			for _, deviceToken := range *notification.PushNotificationTokens {
				notif := &pushNotification{
					pn: &push.Notification[push.DeviceToken]{
						Data:   map[string]string{"deeplink": s.getDeeplink(NotificationType(notification.NotificationType), notification.Data)},
						Target: deviceToken,
						Title:  tmpl.getTitle(notification.Data),
						Body:   tmpl.getBody(notification.Data),
					},
					sn: &sentNotification{
						SentAt:   now,
						Language: notification.Language,
						sentNotificationPK: sentNotificationPK{
							UserID:                   notification.UserID,
							Uniqueness:               notification.Uniqueness,
							NotificationType:         NotificationType(notification.NotificationType),
							NotificationChannel:      NotificationChannel(notification.NotificationChannel),
							NotificationChannelValue: notification.NotificationChannelValue,
						},
					},
				}
				toSendPushNotifications = append(toSendPushNotifications, notif)
			}
		}

		/******************************************************************************************************************************************************
			3. Send all notifications concurrently.
		******************************************************************************************************************************************************/

		if eErr := runConcurrentlyBatch(ctx, s.sendPushNotification, toSendPushNotifications, func(arg *pushNotification, err error) {
			if errors.Is(err, push.ErrInvalidDeviceToken) {
				s.schedulerPushNotificationsMX.Lock()
				invalidTokens = append(invalidTokens, &invalidToken{
					UserID: arg.sn.UserID,
					Token:  arg.pn.Target,
				})
				s.schedulerPushNotificationsMX.Unlock()
			} else {
				log.Error(errors.Wrapf(err, "can't send push notification for:%v", arg.sn.UserID))
			}
		}, func(arg *pushNotification) {
			s.schedulerPushNotificationsMX.Lock()
			defer s.schedulerPushNotificationsMX.Unlock()

			successedNotifications = append(successedNotifications, arg.sn)
		}); eErr != nil {
			log.Error(errors.Wrapf(eErr, "[scheduler] failed to execute concurrently sending push notifications for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber)) //nolint:lll // .
		}
		if len(toSendPushNotifications) > 0 {
			go s.telemetryPushNotifications.collectElapsed(2, *before.Time) //nolint:gomnd,mnd // .
		}

		/******************************************************************************************************************************************************
			4. Clear invalid push notification tokens
		******************************************************************************************************************************************************/
		before = time.Now()
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if cErr := s.clearInvalidPushNotificationTokens(reqCtx, invalidTokens); cErr != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to clear invalid push notification tokens for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
		}
		reqCancel()
		if len(invalidTokens) > 0 {
			go s.telemetryPushNotifications.collectElapsed(3, *before.Time) //nolint:gomnd,mnd // .
		}

		/******************************************************************************************************************************************************
			5. Delete notifications from scheduled.
		******************************************************************************************************************************************************/
		before = time.Now()
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if dErr := s.markScheduledNotificationAsSent(reqCtx, now, successedNotifications); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't insert sent scheduled notification"))
		}
		if dErr := s.deleteScheduledNotifications(reqCtx, emptyTokensNotifications); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't delete scheduled notifications for:%#v", emptyTokensNotifications))
		}
		if len(successedNotifications)+len(emptyTokensNotifications) > 0 {
			go s.telemetryPushNotifications.collectElapsed(4, *before.Time) //nolint:gomnd,mnd // .
		}
		if err = multierror.Append(nil, errs...).ErrorOrNil(); err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to mark/delete scheduled notifications for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)
			reqCancel()

			continue
		}
		reqCancel()

		batchNumber++
		resetVars(true)
	}
}

func (s *Scheduler) sendPushNotification(ctx context.Context, pn *pushNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	responder := make(chan error, 1)
	defer close(responder)
	s.pushNotificationsClient.Send(ctx, pn.pn, responder)

	return errors.Wrapf(<-responder, "can't send push notification for pn:%#v", pn)
}

func (s *Scheduler) clearInvalidPushNotificationTokens(ctx context.Context, pnInvalidTokens []*invalidToken) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if len(pnInvalidTokens) == 0 {
		return nil
	}
	tokens := make([]push.DeviceToken, 0, len(pnInvalidTokens))
	userIDs := make([]string, 0, len(pnInvalidTokens))
	for _, inv := range pnInvalidTokens {
		tokens = append(tokens, inv.Token)
		userIDs = append(userIDs, inv.UserID)
	}
	sql := `UPDATE device_metadata
				SET push_notification_token = null
				WHERE user_id = ANY($1)
					  AND push_notification_token = ANY($2)`
	_, err := storage.Exec(ctx, s.db, sql, userIDs, tokens)

	return errors.Wrapf(err, "failed to update push_notification_token to empty for userID:%v and token %v", userIDs, tokens)
}

func (s *Scheduler) markScheduledNotificationAsSent(ctx context.Context, now *time.Time, notifications []*sentNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	const numFields = 7
	values := make([]string, 0, numFields*len(notifications))
	params := make([]any, 0, numFields*len(notifications))
	for idx, n := range notifications {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v, $%[6]v, $%[7]v)", idx*numFields+1, idx*numFields+2, idx*numFields+3, idx*numFields+4, idx*numFields+5, idx*numFields+6, idx*numFields+7)) //nolint:gomnd,mnd,lll // .
		params = append(params, now.Time, n.Language, n.UserID, n.Uniqueness, n.NotificationType, n.NotificationChannel, n.NotificationChannelValue)
	}
	sql := fmt.Sprintf(`WITH sent AS (
		INSERT INTO sent_notifications(sent_at, language, user_id, uniqueness, notification_type, notification_channel, notification_channel_value)
			VALUES %[1]v
			ON CONFLICT DO NOTHING
			RETURNING user_id, uniqueness, notification_type, notification_channel, notification_channel_value
		)
		DELETE FROM scheduled_notifications 
			WHERE (user_id, uniqueness, notification_type, notification_channel, notification_channel_value) IN (SELECT * FROM sent)`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, s.db, sql, params...)

	return errors.Wrapf(err, "failed to delete scheduled notifications %#v", notifications)
}

func (s *Scheduler) deleteScheduledNotifications(ctx context.Context, notifications []*scheduledNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	const numFields = 5
	values := make([]string, 0, numFields*len(notifications))
	params := make([]any, 0, numFields*len(notifications))
	for idx, n := range notifications {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v)", idx*numFields+1, idx*numFields+2, idx*numFields+3, idx*numFields+4, idx*numFields+5)) //nolint:gomnd,mnd,lll // .
		params = append(params, n.UserID, n.Uniqueness, n.NotificationType, n.NotificationChannel, n.NotificationChannelValue)
	}
	sql := fmt.Sprintf(`DELETE FROM scheduled_notifications 
							   WHERE (user_id, uniqueness, notification_type, notification_channel, notification_channel_value) IN (%v)`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, s.db, sql, params...)

	return errors.Wrapf(err, "failed to delete scheduled notifications %#v", notifications)
}

func (s *Scheduler) fetchScheduledNotifications(
	ctx context.Context, now *time.Time, notificationChannel NotificationChannel, batchSize, workerNumber int64,
) (resp []*scheduledNotificationInfo, err error) {
	sql := fmt.Sprintf(`SELECT sn.*,
				   array_agg(dm.push_notification_token) filter (where dm.push_notification_token is not null)  AS push_notification_tokens,
				   u.disabled_push_notification_domains,
				   u.telegram_user_id,
				   u.telegram_bot_id
			FROM scheduled_notifications sn
				JOIN users u
					ON sn.user_id = u.user_id
				LEFT JOIN device_metadata dm
					ON (u.disabled_push_notification_domains IS NULL 
						OR NOT (u.disabled_push_notification_domains && ARRAY['%v'] )
					)
					AND dm.user_id = u.user_id
					AND dm.push_notification_token IS NOT NULL 
					AND dm.push_notification_token != ''
				WHERE MOD(i, %[2]v) = %[3]v 
				      AND scheduled_for <= $1
					  AND notification_channel = $2
				GROUP BY sn.i, sn.scheduled_at, sn.scheduled_for, sn.data, sn.language, sn.user_id, sn.uniqueness, sn.notification_type,
						 sn.notification_channel, sn.notification_channel_value, u.disabled_push_notification_domains,
						 u.telegram_user_id, u.telegram_bot_id
				ORDER BY MOD(i, %[2]v), scheduled_for, notification_channel ASC
				LIMIT %[4]v`,
		AllNotificationDomain, schedulerWorkersCount, workerNumber, batchSize)
	resp, err = storage.ExecMany[scheduledNotificationInfo](ctx, s.db, sql, now.Time, notificationChannel)

	return resp, errors.Wrapf(err, "failed to fetch scheduled notifications worker:%v", workerNumber)
}

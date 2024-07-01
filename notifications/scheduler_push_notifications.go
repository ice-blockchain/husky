// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
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
		toDelete                    = make([]*scheduledNotification, 0, schedulerPushBatchSize)
		invalidTokens               = make([]*invalidToken, 0)
		toSendPushNotifications     = make([]*pushNotification, 0)
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
		toDelete = toDelete[:0]
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
				tmpl, found = allPushNotificationTemplates[NotificationType(notification.NotificationType)][defaultLanguage]
				if !found {
					log.Panic(fmt.Sprintf("no default translations provided for notification, lang:%v, notificationType:%v", notification.Language, notification.NotificationType)) //nolint:lll // .
				}
			}
			if notification.DisabledPushNotificationDomains != nil {
				domain := getDomainByNotificationType(NotificationType(notification.NotificationType))
				for _, disabledDomain := range *notification.DisabledPushNotificationDomains {
					if disabledDomain == domain {
						toDelete = append(toDelete, &notification.scheduledNotification)
						log.Warn(fmt.Sprintf("notification with disabled notification domain:%v with notification type:%v for notification:%#v", domain, notification.NotificationType, notification)) //nolint:lll // .

						continue out
					}
				}
			}
			if notification.PushNotificationTokens == nil || len(*notification.PushNotificationTokens) == 0 {
				toDelete = append(toDelete, &notification.scheduledNotification)

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
				log.Error(errors.Wrapf(err, "can't send notification for:%v", arg.sn.UserID))
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
		if dErr := s.deleteScheduledNotifications(reqCtx, toDelete); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't delete scheduled notifications for:%#v", toDelete))
		}
		if len(successedNotifications)+len(toDelete) > 0 {
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

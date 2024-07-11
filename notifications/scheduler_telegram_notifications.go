// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	stdlibtime "time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/telegram"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (s *Scheduler) runTelegramNotificationsProcessor(ctx context.Context, workerNumber int64) {
	schedulerTelegramBatchSize := s.getTelegramBatchSize()
	var (
		batchNumber                 int64
		now, lastIterationStartedAt = time.Now(), time.Now()
		errs                        = make([]error, 0)
		successedNotifications      = make([]*sentNotification, 0, schedulerTelegramBatchSize)
		toSendTelegramNotifications = make([]*telegramNotification, 0, schedulerTelegramBatchSize)
		notifications               = make([]*scheduledNotificationInfo, schedulerTelegramBatchSize)
		failedTelegramNotifications = make([]*scheduledNotification, 0, schedulerTelegramBatchSize)
		err                         error
	)
	resetVars := func(success bool) {
		if success && len(notifications) < int(schedulerTelegramBatchSize) {
			go s.telemetryTelegramNotifications.collectElapsed(0, *lastIterationStartedAt.Time)
			batchNumber = 0
		}
		now = time.Now()
		errs = errs[:0]
		successedNotifications = successedNotifications[:0]
		toSendTelegramNotifications = toSendTelegramNotifications[:0]
		notifications = notifications[:0]
		failedTelegramNotifications = failedTelegramNotifications[:0]
		lastIterationStartedAt = now

		stdlibtime.Sleep(1 * stdlibtime.Second)
	}

	for ctx.Err() == nil {
		/******************************************************************************************************************************************************
			1. Fetching a new batch of scheduled messages.
		******************************************************************************************************************************************************/
		before := time.Now()
		reqCtx, reqCancel := context.WithTimeout(ctx, requestDeadline)
		notifications, err = s.fetchScheduledNotifications(reqCtx, now, TelegramNotificationChannel, schedulerTelegramBatchSize, workerNumber)
		if err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to fetch scheduled notifications for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)

			continue
		}
		if len(notifications) > 0 {
			go s.telemetryTelegramNotifications.collectElapsed(1, *before.Time)
		}
		reqCancel()

		/******************************************************************************************************************************************************
			2. Processing batch of notifications.
		******************************************************************************************************************************************************/
		before = time.Now()
		for _, notification := range notifications {
			tmpl, found := allTelegramNotificationTemplates[NotificationType(notification.NotificationType)][notification.Language]
			if !found {
				log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` telegram notifications config", notification.Language, notification.NotificationType))
				tmpl, found = allTelegramNotificationTemplates[NotificationType(notification.NotificationType)][defaultLanguage]
				if !found {
					log.Panic(fmt.Sprintf("no default translations provided for notification, lang:%v, notificationType:%v", notification.Language, notification.NotificationType)) //nolint:lll // .
				}
			}
			if notification.TelegramBotID == "" || notification.TelegramBotID == notification.UserID ||
				notification.TelegramUserID == "" || notification.TelegramUserID == notification.UserID {
				failedTelegramNotifications = append(failedTelegramNotifications, &notification.scheduledNotification)

				continue
			}
			botInfo, found := s.cfg.TelegramBots[strings.ToLower(notification.TelegramBotID)]
			if !found {
				log.Warn(fmt.Sprintf("no bot token for bot id:%v for user:%v", notification.TelegramBotID, notification.UserID))
				failedTelegramNotifications = append(failedTelegramNotifications, &notification.scheduledNotification)

				continue
			}
			tn := &telegramNotification{
				tn: &telegram.Notification{
					ChatID:   notification.TelegramUserID,
					Text:     tmpl.getBody(notification.Data),
					BotToken: botInfo.BotToken,
				},
				sn: &sentNotification{
					SentAt:   now,
					Language: notification.Language,
					sentNotificationPK: sentNotificationPK{
						UserID:                   notification.UserID,
						Uniqueness:               notification.Uniqueness,
						NotificationType:         NotificationType(notification.NotificationType),
						NotificationChannel:      TelegramNotificationChannel,
						NotificationChannelValue: notification.NotificationChannelValue,
					},
				},
				scheduled: notification.scheduledNotification,
			}
			switch notification.NotificationType {
			case string(SocialsNotificationType):
				tn.tn.Buttons = append(tn.tn.Buttons, prepareTelegramButtonsForSocialNotificationType(s.cfg, tmpl.ButtonText)...)
			case string(ReplyNotificationType):
				replyMessageID, pErr := strconv.ParseInt(notification.NotificationChannelValue, 10, 64)
				if pErr != nil {
					log.Warn("can't convert notification chanel value to integer for reply notification type", pErr)
					failedTelegramNotifications = append(failedTelegramNotifications, &notification.scheduledNotification)

					continue
				}
				tn.tn.ReplyMessageID = replyMessageID
			default:
				buttonText := tmpl.getButtonText(notification.Data, 0)
				buttonLink := getTelegramDeeplink(NotificationType(notification.NotificationType), s.cfg, notification.Username, tmpl.getInviteText(notification.Data))
				if buttonText != "" && buttonLink != "" {
					tn.tn.Buttons = append(tn.tn.Buttons, struct {
						Text string `json:"text,omitempty"`
						URL  string `json:"url,omitempty"`
					}{
						Text: buttonText,
						URL:  buttonLink,
					})
				}
			}
			toSendTelegramNotifications = append(toSendTelegramNotifications, tn)
		}

		/******************************************************************************************************************************************************
			3. Send all notifications concurrently.
		******************************************************************************************************************************************************/
		if eErr := runConcurrentlyBatch(ctx, s.sendTelegramNotification, toSendTelegramNotifications, func(arg *telegramNotification, err error) {
			if errors.Is(err, telegram.ErrTelegramNotificationBadRequest) || errors.Is(err, telegram.ErrTelegramNotificationChatNotFound) ||
				errors.Is(err, telegram.ErrTelegramNotificationForbidden) {
				s.schedulerTelegramNotificationsMX.Lock()
				defer s.schedulerTelegramNotificationsMX.Unlock()

				failedTelegramNotifications = append(failedTelegramNotifications, &arg.scheduled)
			} else {
				log.Error(errors.Wrapf(err, "can't send telegram notification for:%v", arg.sn.UserID))
			}
		}, func(arg *telegramNotification) {
			s.schedulerTelegramNotificationsMX.Lock()
			defer s.schedulerTelegramNotificationsMX.Unlock()

			successedNotifications = append(successedNotifications, arg.sn)
		}); eErr != nil {
			log.Error(errors.Wrapf(eErr, "[scheduler] failed to execute concurrently sending telegram notifications for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber)) //nolint:lll // .
		}
		if len(toSendTelegramNotifications) > 0 {
			go s.telemetryTelegramNotifications.collectElapsed(2, *before.Time) //nolint:gomnd,mnd // .
		}

		/******************************************************************************************************************************************************
			4. Delete notifications from scheduled.
		******************************************************************************************************************************************************/
		before = time.Now()
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if dErr := s.markScheduledNotificationAsSent(reqCtx, now, successedNotifications); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't insert sent scheduled notification"))
		}
		if dErr := s.deleteScheduledNotifications(reqCtx, failedTelegramNotifications); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't delete scheduled notifications for:%#v", failedTelegramNotifications))
		}
		if len(successedNotifications)+len(failedTelegramNotifications) > 0 {
			go s.telemetryTelegramNotifications.collectElapsed(3, *before.Time) //nolint:gomnd,mnd // .
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

func (s *Scheduler) sendTelegramNotification(ctx context.Context, tn *telegramNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}

	return errors.Wrapf(s.telegramNotificationsClient.Send(ctx, tn.tn), "can't send telegram notification for pn:%#v", tn)
}

func (s *Scheduler) getTelegramBatchSize() int64 {
	if len(s.cfg.TelegramBots) < 5 { //nolint:gomnd,mnd // .
		return 10 //nolint:gomnd,mnd // .
	}

	return int64(len(s.cfg.TelegramBots))
}

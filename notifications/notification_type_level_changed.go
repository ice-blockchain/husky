// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/notifications/telegram"
	"github.com/ice-blockchain/wintr/time"
)

func (s *completedLevelsSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen,gocognit,gocyclo,revive,cyclop // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	type (
		CompletedLevel struct {
			UserID          string `json:"userId"`
			Type            string `json:"type"`
			CompletedLevels uint64 `json:"completedLevels" `
		}
	)
	message := new(CompletedLevel)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.UserID == "" {
		return nil
	}
	if s.cfg.IsLevelNotificationDisabled(message.Type) {
		return nil
	}
	now := time.Now()
	deeplink := fmt.Sprintf("%v://profile?userId=%v", s.cfg.DeeplinkScheme, message.UserID)
	in := &inAppNotification{
		in: &inapp.Parcel{
			Time: now,
			Data: map[string]any{
				"deeplink": deeplink,
			},
			Action: string(LevelChangedNotificationType),
			Actor: inapp.ID{
				Type:  "system",
				Value: "system",
			},
			Subject: inapp.ID{
				Type:  "levelValue",
				Value: strconv.FormatUint(message.CompletedLevels, 10),
			},
		},
		sn: &sentNotification{
			SentAt: now,
			sentNotificationPK: sentNotificationPK{
				UserID:              message.UserID,
				Uniqueness:          message.Type,
				NotificationType:    LevelChangedNotificationType,
				NotificationChannel: InAppNotificationChannel,
			},
		},
	}
	tokens, err := s.getPushNotificationTokens(ctx, AchievementsNotificationDomain, message.UserID)
	if err != nil || tokens == nil {
		return multierror.Append( //nolint:wrapcheck // .
			err,
			errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", LevelChangedNotificationType, in),
		).ErrorOrNil()
	}
	var exConcurrently []func() error
	if tokens.PushNotificationTokens != nil && len(*tokens.PushNotificationTokens) != 0 { //nolint:dupl // .
		tmpl, found := allPushNotificationTemplates[LevelChangedNotificationType][tokens.Language]
		if !found {
			log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", tokens.Language, LevelChangedNotificationType))

			return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", LevelChangedNotificationType, in)
		}
		pn := make([]*pushNotification, 0, len(*tokens.PushNotificationTokens))
		for _, token := range *tokens.PushNotificationTokens {
			pn = append(pn, &pushNotification{
				pn: &push.Notification[push.DeviceToken]{
					Data:   map[string]string{"deeplink": deeplink},
					Target: token,
					Title:  tmpl.getTitle(nil),
					Body:   tmpl.getBody(nil),
				},
				sn: &sentNotification{
					SentAt:   now,
					Language: tokens.Language,
					sentNotificationPK: sentNotificationPK{
						UserID:                   message.UserID,
						Uniqueness:               message.Type,
						NotificationType:         LevelChangedNotificationType,
						NotificationChannel:      PushNotificationChannel,
						NotificationChannelValue: string(token),
					},
				},
			})
		}
		exConcurrently = append(exConcurrently, func() error {
			return errors.Wrapf(runConcurrently(ctx, s.sendPushNotification, pn), "failed to sendPushNotifications atleast to some devices for %v, args:%#v", LevelChangedNotificationType, pn) //nolint:lll // .
		}, func() error {
			return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", LevelChangedNotificationType, in)
		})
	}
	if false && (tokens.TelegramBotID != "" && tokens.TelegramBotID != tokens.UserID && //nolint:revive,nestif // .
		tokens.TelegramUserID != "" && tokens.TelegramUserID != tokens.UserID) {
		if tmplTelegram, found := allTelegramNotificationTemplates[LevelChangedNotificationType][tokens.Language]; !found {
			log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` telegram config", tokens.Language, LevelChangedNotificationType))
		} else { //nolint:dupl // .
			if botInfo, botFound := s.cfg.TelegramBots[strings.ToLower(tokens.TelegramBotID)]; botFound {
				tn := &telegramNotification{
					tn: &telegram.Notification{
						ChatID:   tokens.TelegramUserID,
						Text:     tmplTelegram.getBody(nil),
						BotToken: botInfo.BotToken,
					},
					sn: &sentNotification{
						SentAt:   now,
						Language: tokens.Language,
						sentNotificationPK: sentNotificationPK{
							UserID:                   message.UserID,
							Uniqueness:               message.Type,
							NotificationType:         LevelChangedNotificationType,
							NotificationChannel:      TelegramNotificationChannel,
							NotificationChannelValue: message.UserID,
						},
					},
				}
				buttonText := tmplTelegram.getButtonText(nil, 0)
				buttonLink := getTelegramDeeplink(LevelChangedNotificationType, s.cfg, "", "")
				if buttonText != "" && buttonLink != "" {
					tn.tn.Buttons = append(tn.tn.Buttons, struct {
						Text string `json:"text,omitempty"`
						URL  string `json:"url,omitempty"`
					}{
						Text: buttonText,
						URL:  buttonLink,
					})
				}
				exConcurrently = append(exConcurrently, func() error {
					return errors.Wrapf(s.sendTelegramNotification(ctx, tn), "failed to send telegram notification for %v, notif:%#v", LevelChangedNotificationType, in)
				})
			}
		}
	}

	return errors.Wrap(executeConcurrently(exConcurrently...), "failed to executeConcurrently")
}

// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/analytics"
	"github.com/ice-blockchain/wintr/analytics/tracking"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func (s *completedLevelsSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen // .
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
	imageURL := s.pictureClient.DownloadURL("assets/push-notifications/level-change.png")
	in := &inAppNotification{
		in: &inapp.Parcel{
			Time: now,
			Data: map[string]any{
				"deeplink": deeplink,
				"imageUrl": imageURL,
			},
			Action: string(LevelChangedNotificationType),
			Actor: inapp.ID{
				Type:  "system",
				Value: "system",
			},
			Subject: inapp.ID{
				Type:  "levelValue",
				Value: fmt.Sprint(message.CompletedLevels),
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
			errors.Wrap(executeConcurrently(func() error {
				return errors.Wrapf(s.sendAnalyticsSetUserAttributesCommandMessage(ctx, &analytics.SetUserAttributesCommand{
					Attributes: map[string]any{
						"Current Level": fmt.Sprint(message.CompletedLevels),
					},
					UserID: message.UserID,
				}),
					"failed to sendAnalyticsSetUserAttributesCommandMessage %#v", message)
			}, func() error {
				return errors.Wrapf(s.sendAnalyticsTrackActionCommandMessage(ctx, &analytics.TrackActionCommand{
					Action: &tracking.Action{
						Attributes: map[string]any{
							"Current Level": fmt.Sprint(message.CompletedLevels),
						},
						Name: "Level Changed",
					},
					ID:     fmt.Sprintf("%v_level_%v", message.UserID, message.Type),
					UserID: message.UserID,
				}),
					"failed to sendAnalyticsTrackActionCommandMessage %#v", message)
			}), "at least one analytics command failed to execute"),
		).ErrorOrNil()
	}
	tmpl, found := allPushNotificationTemplates[LevelChangedNotificationType][tokens.Language]
	if !found {
		log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", tokens.Language, LevelChangedNotificationType))

		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", LevelChangedNotificationType, in)
	}
	pn := make([]*pushNotification, 0, len(*tokens.PushNotificationTokens))
	for _, token := range *tokens.PushNotificationTokens {
		pn = append(pn, &pushNotification{
			pn: &push.Notification[push.DeviceToken]{
				Data:     map[string]string{"deeplink": deeplink},
				Target:   token,
				Title:    tmpl.getTitle(nil),
				Body:     tmpl.getBody(nil),
				ImageURL: imageURL,
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

	return errors.Wrap(executeConcurrently(func() error {
		return errors.Wrapf(runConcurrently(ctx, s.sendPushNotification, pn), "failed to sendPushNotifications atleast to some devices for %v, args:%#v", LevelChangedNotificationType, pn) //nolint:lll // .
	}, func() error {
		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", LevelChangedNotificationType, in)
	}), "failed to executeConcurrently")
}

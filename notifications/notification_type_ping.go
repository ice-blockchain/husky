// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strconv"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func (s *userPingSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	message := new(UserPing)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.UserID == "" {
		return nil
	}
	pingedBy, err := s.getUserByID(ctx, message.PingedBy)
	if err != nil {
		return errors.Wrapf(err, "failed to getUserByID for pingedBy:%v", pingedBy)
	}
	now := time.Now()
	uniqueness := strconv.FormatInt(message.LastPingCooldownEndedAt.UnixNano()/s.cfg.PingCooldown.Nanoseconds(), 10)
	deeplink := fmt.Sprintf("%v://home", s.cfg.DeeplinkScheme)
	imageURL := s.pictureClient.DownloadURL(fmt.Sprintf("profile/%v", pingedBy.ProfilePictureName))
	in := &inAppNotification{
		in: &inapp.Parcel{
			Time:        now,
			ReferenceID: fmt.Sprintf("%v:%v", PingNotificationType, uniqueness),
			Data: map[string]any{
				"username": pingedBy.Username,
				"deeplink": deeplink,
				"imageUrl": imageURL,
			},
			Action: "pinged",
			Actor: inapp.ID{
				Type:  "userId",
				Value: message.PingedBy,
			},
			Subject: inapp.ID{
				Type:  "userId",
				Value: message.UserID,
			},
		},
		sn: &sentNotification{
			SentAt: now,
			sentNotificationPK: sentNotificationPK{
				UserID:              message.UserID,
				Uniqueness:          uniqueness,
				NotificationType:    PingNotificationType,
				NotificationChannel: InAppNotificationChannel,
			},
		},
	}
	tokens, err := s.getPushNotificationTokens(ctx, MicroCommunityNotificationDomain, message.UserID)
	if err != nil || tokens == nil {
		return multierror.Append( //nolint:wrapcheck // .
			err,
			errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", PingNotificationType, in),
		).ErrorOrNil()
	}
	tmpl, found := allPushNotificationTemplates[PingNotificationType][tokens.Language]
	if !found {
		log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", tokens.Language, PingNotificationType))

		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", PingNotificationType, in)
	}
	pn := make([]*pushNotification, 0, len(*tokens.PushNotificationTokens))
	data := struct{ Username string }{Username: fmt.Sprintf("@%v", pingedBy.Username)}
	for _, token := range *tokens.PushNotificationTokens {
		pn = append(pn, &pushNotification{
			pn: &push.Notification[push.DeviceToken]{
				Data:     map[string]string{"deeplink": deeplink},
				Target:   token,
				Title:    tmpl.getTitle(data),
				Body:     tmpl.getBody(nil),
				ImageURL: imageURL,
			},
			sn: &sentNotification{
				SentAt:   now,
				Language: tokens.Language,
				sentNotificationPK: sentNotificationPK{
					UserID:                   message.UserID,
					Uniqueness:               uniqueness,
					NotificationType:         PingNotificationType,
					NotificationChannel:      PushNotificationChannel,
					NotificationChannelValue: string(token),
				},
			},
		})
	}

	return errors.Wrap(executeConcurrently(func() error {
		return errors.Wrapf(runConcurrently(ctx, s.sendPushNotification, pn), "failed to sendPushNotifications atleast to some devices for %v, args:%#v", PingNotificationType, pn) //nolint:lll // .
	}, func() error {
		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", PingNotificationType, in)
	}), "failed to executeConcurrently")
}

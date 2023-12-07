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
	"github.com/ice-blockchain/wintr/email"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func (s *availableDailyBonusSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	type extraBonusSummary struct {
		UserID          string `json:"userId,omitempty" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		ExtraBonusIndex uint64 `json:"extraBonusIndex,omitempty" example:"1"`
	}
	message := new(extraBonusSummary)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.UserID == "" {
		return nil
	}
	now := time.Now()
	deeplink := fmt.Sprintf("%v://claim-daily-bonus", s.cfg.DeeplinkScheme)
	imageURL := s.pictureClient.DownloadURL("assets/push-notifications/daily-bonus.png")
	in := &inAppNotification{
		in: &inapp.Parcel{
			Time: now,
			Data: map[string]any{
				"deeplink": deeplink,
				"imageUrl": imageURL,
			},
			Action: "daily_bonus_became_available",
			Actor: inapp.ID{
				Type:  "system",
				Value: "system",
			},
			Subject: inapp.ID{
				Type:  "dailyBonus",
				Value: strconv.FormatUint(message.ExtraBonusIndex, 10),
			},
		},
		sn: &sentNotification{
			SentAt: now,
			sentNotificationPK: sentNotificationPK{
				UserID:              message.UserID,
				Uniqueness:          strconv.FormatUint(message.ExtraBonusIndex, 10),
				NotificationType:    DailyBonusNotificationType,
				NotificationChannel: InAppNotificationChannel,
			},
		},
	}
	tokens, err := s.getPushNotificationTokens(ctx, DailyBonusNotificationDomain, message.UserID)
	if err != nil || tokens == nil {
		return multierror.Append( //nolint:wrapcheck // .
			err,
			errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", DailyBonusNotificationType, in),
			errors.Wrapf(s.trySendEmailNotification(ctx, strconv.FormatUint(message.ExtraBonusIndex, 10), message.UserID), "failed to trySendEmailNotification for %v, message:%#v", DailyBonusNotificationType, message), //nolint:lll // .
		).ErrorOrNil()
	}
	tmpl, found := allPushNotificationTemplates[DailyBonusNotificationType][tokens.Language]
	if !found {
		log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", tokens.Language, DailyBonusNotificationType))

		return multierror.Append( //nolint:wrapcheck // .
			errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", DailyBonusNotificationType, in),
			errors.Wrapf(s.trySendEmailNotification(ctx, strconv.FormatUint(message.ExtraBonusIndex, 10), message.UserID), "failed to trySendEmailNotification for %v, message:%#v", DailyBonusNotificationType, message), //nolint:lll // .
		).ErrorOrNil()
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
					Uniqueness:               strconv.FormatUint(message.ExtraBonusIndex, 10),
					NotificationType:         DailyBonusNotificationType,
					NotificationChannel:      PushNotificationChannel,
					NotificationChannelValue: string(token),
				},
			},
		})
	}

	return errors.Wrap(executeConcurrently(func() error {
		return errors.Wrapf(runConcurrently(ctx, s.sendPushNotification, pn), "failed to sendPushNotifications atleast to some devices for %v, args:%#v", DailyBonusNotificationType, pn) //nolint:lll // .
	}, func() error {
		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", DailyBonusNotificationType, in)
	}), "failed to executeConcurrently")
}

func (s *availableDailyBonusSource) trySendEmailNotification(ctx context.Context, uniqueness, userID string) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	params, err := s.getEmailNotificationParams(ctx, DailyBonusNotificationDomain, userID, true)
	if err != nil || params == nil || true { //nolint:revive // TODO:: temporarily disabled
		return errors.Wrapf(err, "failed to getEmailNotificationParams for notif:%v, userID:%v", DailyBonusNotificationDomain, userID)
	}
	en := &emailNotification{
		displayName: params.DisplayName,
		en: &email.Parcel{
			Body: &email.Body{
				Type: email.TextHTML,
				Data: fmt.Sprintf("<p>[%v]TODO daily bonus available</p>", params.Language),
			},
			Subject: fmt.Sprintf("[%v]TODO daily bonus available", params.Language),
		},
		sn: &sentNotification{
			SentAt:   time.Now(),
			Language: params.Language,
			sentNotificationPK: sentNotificationPK{
				UserID:                   userID,
				Uniqueness:               uniqueness,
				NotificationType:         DailyBonusNotificationType,
				NotificationChannel:      EmailNotificationChannel,
				NotificationChannelValue: params.Email,
			},
		},
	}

	return errors.Wrapf(s.sendEmailNotification(ctx, en), "failed to sendEmailNotification for notif:%#v", en)
}

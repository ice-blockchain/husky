// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strconv"

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

func (s *achievedBadgesSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen,gocyclo,gocognit,revive,cyclop // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	type (
		GroupType     string
		AchievedBadge struct {
			UserID    string    `json:"userId"`
			Type      string    `json:"type"`
			Name      string    `json:"name" `
			GroupType GroupType `json:"groupType"`
		}
	)
	//nolint:gocritic,revive // Not an issue.
	const (
		LevelGroupType  GroupType = "level"
		CoinGroupType   GroupType = "coin"
		SocialGroupType GroupType = "social"
	)
	message := new(AchievedBadge)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.UserID == "" {
		return nil
	}
	if s.cfg.IsBadgeNotificationDisabled(message.Type) {
		return nil
	}
	var notifType NotificationType
	switch message.GroupType {
	case LevelGroupType:
		notifType = LevelBadgeUnlockedNotificationType
	case CoinGroupType:
		notifType = CoinBadgeUnlockedNotificationType
	case SocialGroupType:
		notifType = SocialBadgeUnlockedNotificationType
	}
	now := time.Now()
	deeplink := fmt.Sprintf("%v://profile?section=badges&userId=%v", s.cfg.DeeplinkScheme, message.UserID)
	imageURL := s.pictureClient.DownloadURL(fmt.Sprintf("badges/%v.png", message.Type))
	badgeIndex, err := strconv.Atoi(message.Type[1:])
	log.Panic(err) //nolint:revive // Intended.
	badgeIndex--
	in := &inAppNotification{
		in: &inapp.Parcel{
			Time: now,
			Data: map[string]any{
				"deeplink": deeplink,
				"imageUrl": imageURL,
			},
			Action: string(notifType),
			Actor: inapp.ID{
				Type:  "system",
				Value: "system",
			},
			Subject: inapp.ID{
				Type:  "badgeIndex",
				Value: fmt.Sprint(badgeIndex),
			},
		},
		sn: &sentNotification{
			SentAt: now,
			sentNotificationPK: sentNotificationPK{
				UserID:              message.UserID,
				Uniqueness:          message.Type,
				NotificationType:    notifType,
				NotificationChannel: InAppNotificationChannel,
			},
		},
	}
	if s.cfg.DisableBadgeUnlockedPushOrAnalyticsNotifications {
		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", notifType, in)
	}
	tokens, err := s.getPushNotificationTokens(ctx, AchievementsNotificationDomain, message.UserID)
	if err != nil || tokens == nil {
		return multierror.Append( //nolint:wrapcheck // .
			err,
			errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", notifType, in),
			errors.Wrap(executeConcurrently(func() error {
				key := ""
				switch message.GroupType {
				case LevelGroupType:
					key = "Current Social Badge"
				case CoinGroupType:
					key = "Current Coins Badge"
				case SocialGroupType:
					key = "Current Level Badge"
				}

				return errors.Wrapf(s.sendAnalyticsSetUserAttributesCommandMessage(ctx, &analytics.SetUserAttributesCommand{
					Attributes: map[string]any{
						key: message.Name,
					},
					UserID: message.UserID,
				}),
					"failed to sendAnalyticsSetUserAttributesCommandMessage %#v", message)
			}, func() error {
				return errors.Wrapf(s.sendAnalyticsTrackActionCommandMessage(ctx, &analytics.TrackActionCommand{
					Action: &tracking.Action{
						Attributes: map[string]any{
							"Badge Type": message.GroupType,
							"Badge Name": message.Name,
						},
						Name: "Badge Unlocked",
					},
					ID:     fmt.Sprintf("%v_badge_type_%v", message.UserID, message.Type),
					UserID: message.UserID,
				}),
					"failed to sendAnalyticsTrackActionCommandMessage %#v", message)
			}), "at least one analytics command failed to execute"),
		).ErrorOrNil()
	}
	tmpl, found := allPushNotificationTemplates[notifType][tokens.Language]
	if !found {
		log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", tokens.Language, notifType))

		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", notifType, in)
	}
	pn := make([]*pushNotification, 0, len(*tokens.PushNotificationTokens))
	data := struct{ BadgeName string }{BadgeName: message.Name}
	for _, token := range *tokens.PushNotificationTokens {
		pn = append(pn, &pushNotification{
			pn: &push.Notification[push.DeviceToken]{
				Data:     map[string]string{"deeplink": deeplink},
				Target:   token,
				Title:    tmpl.getTitle(nil),
				Body:     tmpl.getBody(data),
				ImageURL: imageURL,
			},
			sn: &sentNotification{
				SentAt:   now,
				Language: tokens.Language,
				sentNotificationPK: sentNotificationPK{
					UserID:                   message.UserID,
					Uniqueness:               message.Type,
					NotificationType:         notifType,
					NotificationChannel:      PushNotificationChannel,
					NotificationChannelValue: string(token),
				},
			},
		})
	}

	return errors.Wrap(executeConcurrently(func() error {
		return errors.Wrapf(runConcurrently(ctx, s.sendPushNotification, pn), "failed to sendPushNotifications atleast to some devices for %v, args:%#v", notifType, pn) //nolint:lll // .
	}, func() error {
		return errors.Wrapf(s.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", notifType, in)
	}), "failed to executeConcurrently")
}

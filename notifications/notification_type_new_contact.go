// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	storagev2 "github.com/ice-blockchain/wintr/connectors/storage/v2"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) sendNewContactNotification(ctx context.Context, us *users.UserSnapshot) error { //nolint:funlen,gocognit,gocyclo,revive,cyclop // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if us.User == nil ||
		us.User.PhoneNumberHash == "" ||
		us.User.Username == "" ||
		us.User.Username == us.User.ID ||
		us.User.ReferredBy == "" ||
		us.User.ReferredBy == us.User.ID ||
		us.User.PhoneNumberHash == us.User.ID ||
		(us.Before != nil && us.Before.PhoneNumberHash != "" && us.Before.PhoneNumberHash != us.User.ID &&
			us.Before.ReferredBy != us.User.ID && us.Before.ReferredBy != "") {
		return nil
	}
	tokens, err := r.getPushNotificationTokensForNewContactNotification(ctx, us.User.PhoneNumberHash)
	if err != nil || len(tokens) == 0 {
		return err
	}
	const (
		actionName = "contact_joined_ice"
	)
	now := time.Now()
	data := struct{ Username string }{Username: fmt.Sprintf("@%v", us.User.Username)}
	pn, in := make([]*pushNotification, 0, len(tokens)), make([]*inAppNotification, 0, len(tokens))
	for _, token := range tokens {
		deeplink := fmt.Sprintf("%v://profile?userId=%v", r.cfg.DeeplinkScheme, us.User.ID)
		in = append(in, &inAppNotification{
			in: &inapp.Parcel{
				Time:        now,
				ReferenceID: fmt.Sprintf("%v:userId:%v", actionName, us.User.ID),
				Data: map[string]any{
					"username": us.User.Username,
					"deeplink": deeplink,
					"imageUrl": us.User.ProfilePictureURL,
				},
				Action: actionName,
				Actor: inapp.ID{
					Type:  "userId",
					Value: us.User.ID,
				},
				Subject: inapp.ID{
					Type:  "userId",
					Value: us.User.ID,
				},
			},
			sn: &sentNotification{
				SentAt: now,
				sentNotificationPK: sentNotificationPK{
					UserID:              token.UserID,
					Uniqueness:          us.User.ID,
					NotificationType:    NewContactNotificationType,
					NotificationChannel: InAppNotificationChannel,
				},
			},
		})
		if token.PushNotificationTokens == nil || len(*token.PushNotificationTokens) == 0 {
			continue
		}
		tmpl, found := allPushNotificationTemplates[NewContactNotificationType][token.Language]
		if !found {
			log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", token.Language, NewContactNotificationType))

			continue
		}
		for _, deviceToken := range *token.PushNotificationTokens {
			pn = append(pn, &pushNotification{
				pn: &push.Notification[push.DeviceToken]{
					Data:     map[string]string{"deeplink": deeplink},
					Target:   deviceToken,
					Title:    tmpl.getTitle(data),
					Body:     tmpl.getBody(nil),
					ImageURL: us.User.ProfilePictureURL,
				},
				sn: &sentNotification{
					SentAt:   now,
					Language: token.Language,
					sentNotificationPK: sentNotificationPK{
						UserID:                   token.UserID,
						Uniqueness:               us.User.ID,
						NotificationType:         NewContactNotificationType,
						NotificationChannel:      PushNotificationChannel,
						NotificationChannelValue: string(deviceToken),
					},
				},
			})
		}
	}

	return errors.Wrap(executeConcurrently(func() error {
		return errors.Wrapf(runConcurrently(ctx, r.sendPushNotification, pn),
			"failed to sendPushNotifications atleast to some devices for %v, args:%#v", NewContactNotificationType, pn)
	}, func() error {
		return errors.Wrapf(runConcurrently(ctx, r.sendInAppNotification, in),
			"failed to sendInAppNotifications atleast to some users for %v, args:%#v", NewContactNotificationType, in)
	}), "failed to executeConcurrently")
}

func (r *repository) getPushNotificationTokensForNewContactNotification( //nolint:funlen // .
	ctx context.Context, phoneNumberHash string,
) ([]*pushNotificationTokens, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := fmt.Sprintf(`SELECT STRING_AGG(dm.push_notification_token, ',') AS push_notification_tokens, 
							   u.language,
							   u.user_id
						FROM users u
							 LEFT JOIN device_metadata dm
									ON ( u.disabled_push_notification_domains IS NULL 
										OR (
											POSITION('%[1]v' IN u.disabled_push_notification_domains) = 0
								   			AND 
								   			POSITION('%[2]v' IN u.disabled_push_notification_domains) = 0
								   		   )
								   	   )
								   AND dm.user_id = u.user_id
								   AND dm.push_notification_token IS NOT NULL 
								   AND dm.push_notification_token != ''
						WHERE u.agenda_phone_number_hashes IS NOT NULL
						  AND POSITION($1 IN u.agenda_phone_number_hashes) != 0
						GROUP BY u.user_id`, MicroCommunityNotificationDomain, AllNotificationDomain)

	resp, err := storagev2.Select[pushNotificationTokens](ctx, r.db, sql, phoneNumberHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to select for push notification tokens for `%v`, phomeNumberHash:%v", NewContactNotificationType, phoneNumberHash)
	}

	return resp, nil
}

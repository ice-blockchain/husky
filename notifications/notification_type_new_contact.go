// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) sendNewContactNotification(ctx context.Context, contact *users.Contact) error { //nolint:funlen,gocognit // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	tokens, err := r.getPushNotificationTokensForNewContactNotification(ctx, contact.UserID, contact.ContactUserID)
	if err != nil || len(tokens) == 0 {
		return err
	}
	joinedUsr, err := r.getUserByID(ctx, contact.ContactUserID)
	if err != nil {
		return errors.Wrapf(err, "failed to getUserByID for userID:%v", contact.UserID)
	}
	const (
		actionName = "contact_joined_ice"
	)
	now := time.Now()
	data := struct{ Username string }{Username: fmt.Sprintf("@%v", joinedUsr.Username)}
	profilePictureURL := r.pictureClient.DownloadURL(joinedUsr.ProfilePictureName)
	pn, in := make([]*pushNotification, 0, len(tokens)), make([]*inAppNotification, 0, len(tokens))
	for _, token := range tokens {
		deeplink := fmt.Sprintf("%v://profile?userId=%v", r.cfg.DeeplinkScheme, contact.ContactUserID)
		in = append(in, &inAppNotification{
			in: &inapp.Parcel{
				Time:        now,
				ReferenceID: fmt.Sprintf("%v:userId:%v", actionName, contact.ContactUserID),
				Data: map[string]any{
					"username": joinedUsr.Username,
					"deeplink": deeplink,
					"imageUrl": profilePictureURL,
				},
				Action: actionName,
				Actor: inapp.ID{
					Type:  "userId",
					Value: contact.ContactUserID,
				},
				Subject: inapp.ID{
					Type:  "userId",
					Value: contact.ContactUserID,
				},
			},
			sn: &sentNotification{
				SentAt: now,
				sentNotificationPK: sentNotificationPK{
					UserID:              contact.UserID,
					Uniqueness:          contact.ContactUserID,
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
					ImageURL: profilePictureURL,
				},
				sn: &sentNotification{
					SentAt:   now,
					Language: token.Language,
					sentNotificationPK: sentNotificationPK{
						UserID:                   contact.UserID,
						Uniqueness:               contact.ContactUserID,
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

func (r *repository) getPushNotificationTokensForNewContactNotification(
	ctx context.Context, userID, contactID string,
) ([]*pushNotificationTokens, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := fmt.Sprintf(`SELECT array_agg(dm.push_notification_token) filter (where dm.push_notification_token is not null) AS push_notification_tokens, 
							   u.language,
							   u.user_id
						FROM users u
							 LEFT JOIN device_metadata dm
									ON ( u.disabled_push_notification_domains IS NULL 
										OR NOT (u.disabled_push_notification_domains @> ARRAY['%[1]v', '%[2]v' ])
								   	   )
								   AND dm.user_id = u.user_id
								   AND dm.push_notification_token IS NOT NULL 
								   AND dm.push_notification_token != ''
						WHERE u.user_id = $1
							  AND $2 = ANY(u.agenda_contact_user_ids)
						GROUP BY u.user_id`, MicroCommunityNotificationDomain, AllNotificationDomain)

	resp, err := storage.Select[pushNotificationTokens](ctx, r.db, sql, userID, contactID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to select for push notification tokens for `%v`, userID:%v", NewContactNotificationType, userID)
	}

	return resp, nil
}

// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) sendNewReferralNotification(ctx context.Context, us *users.UserSnapshot) error { //nolint:funlen,gocognit,gocyclo,revive,cyclop // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if us.User == nil || us.User.ReferredBy == "" || us.User.ReferredBy == us.User.ID ||
		(us.Before != nil && us.Before.ID != "" && us.User != nil && us.User.ID != "" && us.User.ReferredBy == us.Before.ReferredBy) ||
		us.Username == "" || us.Username == us.ID {
		return nil
	}
	const (
		actionName = "referral_joined_team"
	)
	now := time.Now()
	deeplink := fmt.Sprintf("%v://profile?userId=%v", r.cfg.DeeplinkScheme, us.User.ID)
	in := &inAppNotification{
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
				Value: us.User.ReferredBy,
			},
		},
		sn: &sentNotification{
			SentAt: now,
			sentNotificationPK: sentNotificationPK{
				UserID:              us.User.ReferredBy,
				Uniqueness:          us.User.ID,
				NotificationType:    NewReferralNotificationType,
				NotificationChannel: InAppNotificationChannel,
			},
		},
	}
	tokens, err := r.getPushNotificationTokens(ctx, MicroCommunityNotificationDomain, us.User.ReferredBy)
	if err != nil || tokens == nil {
		return multierror.Append( //nolint:wrapcheck // .
			err,
			errors.Wrapf(r.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", NewReferralNotificationType, in),
		).ErrorOrNil()
	}
	tmpl, found := allPushNotificationTemplates[NewReferralNotificationType][tokens.Language]
	if !found {
		log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", tokens.Language, NewReferralNotificationType))

		return errors.Wrapf(r.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", NewReferralNotificationType, in)
	}
	pn := make([]*pushNotification, 0, len(*tokens.PushNotificationTokens))
	data := struct{ Username string }{Username: fmt.Sprintf("@%v", us.User.Username)}
	for _, token := range *tokens.PushNotificationTokens {
		pn = append(pn, &pushNotification{
			pn: &push.Notification[push.DeviceToken]{
				Data:     map[string]string{"deeplink": deeplink},
				Target:   token,
				Title:    tmpl.getTitle(data),
				Body:     tmpl.getBody(nil),
				ImageURL: us.User.ProfilePictureURL,
			},
			sn: &sentNotification{
				SentAt:   now,
				Language: tokens.Language,
				sentNotificationPK: sentNotificationPK{
					UserID:                   us.User.ReferredBy,
					Uniqueness:               us.User.ID,
					NotificationType:         NewReferralNotificationType,
					NotificationChannel:      PushNotificationChannel,
					NotificationChannelValue: string(token),
				},
			},
		})
	}

	return errors.Wrap(executeConcurrently(func() error {
		return errors.Wrapf(runConcurrently(ctx, r.sendPushNotification, pn), "failed to sendPushNotifications atleast to some devices for %v, args:%#v", NewReferralNotificationType, pn) //nolint:lll // .
	}, func() error {
		return errors.Wrapf(r.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", NewReferralNotificationType, in)
	}), "failed to executeConcurrently")
}

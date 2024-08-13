// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"
	stdlibtime "time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/freezer/model"
	"github.com/ice-blockchain/freezer/tokenomics"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	storagev3 "github.com/ice-blockchain/wintr/connectors/storage/v3"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) sendNewReferralNotification(ctx context.Context, us *users.UserSnapshot) error { //nolint:funlen,gocyclo,revive,cyclop,gocognit // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	usernameEmpty := us.Username == "" || us.Username == us.ID
	usernameJustChanged := us.Before != nil && (us.Before.Username == us.Before.ID || us.Before.Username == "") && us.User != nil && !usernameEmpty
	referredByNotChanged := us.Before != nil && us.Before.ID != "" && us.User != nil && us.User.ID != "" && us.User.ReferredBy == us.Before.ReferredBy
	if us.User == nil || us.User.ReferredBy == "" || us.User.ReferredBy == us.User.ID ||
		(referredByNotChanged && !usernameJustChanged) || (!referredByNotChanged && usernameEmpty) ||
		us.User.ReferredBy == r.cfg.DefaultReferralName || us.User.ReferredBy == "bogus" || us.User.ReferredBy == "icenetwork" {
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
	var exConcurrently []func() error
	data := struct {
		Username, Coin string
		Amount         uint64
	}{
		Username: fmt.Sprintf("@%v", us.User.Username),
		Coin:     r.cfg.TokenName,
		Amount:   r.getNewReferralCoinAmount(ctx, us.User.ReferredBy),
	}
	if tokens.PushNotificationTokens != nil && len(*tokens.PushNotificationTokens) != 0 {
		tmpl, found := allPushNotificationTemplates[NewReferralNotificationType][tokens.Language]
		if !found {
			log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", tokens.Language, NewReferralNotificationType))

			return errors.Wrapf(r.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", NewReferralNotificationType, in)
		}
		pn := make([]*pushNotification, 0, len(*tokens.PushNotificationTokens))
		for _, token := range *tokens.PushNotificationTokens {
			var body string
			if data.Amount > 0 {
				body = tmpl.getBody(data)
			} else {
				body = tmpl.getAltBody(nil)
			}
			pn = append(pn, &pushNotification{
				pn: &push.Notification[push.DeviceToken]{
					Data:     map[string]string{"deeplink": deeplink},
					Target:   token,
					Title:    tmpl.getTitle(data),
					Body:     body,
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
		exConcurrently = append(exConcurrently, func() error {
			return errors.Wrapf(runConcurrently(ctx, r.sendPushNotification, pn), "failed to sendPushNotifications atleast to some devices for %v, args:%#v", NewReferralNotificationType, pn) //nolint:lll // .
		}, func() error {
			return errors.Wrapf(r.sendInAppNotification(ctx, in), "failed to sendInAppNotification for %v, notif:%#v", NewReferralNotificationType, in)
		})
	}
	if rErr := r.removeScheduledNotifications(ctx, us.ReferredBy, []NotificationType{NewReferralNotificationType}); rErr != nil {
		return errors.Wrapf(rErr, "can't remove scheduled notifications for:%v", us.ReferredBy)
	}
	newReferralCountLast24HInfo, gErr := r.getNewReferralCountLast24H(ctx, us.ReferredBy)
	if gErr != nil {
		return errors.Wrapf(gErr, "can't call getNewReferralCountLast24H for:%v", us.ReferredBy)
	}
	if newReferralCountLast24HInfo != nil && newReferralCountLast24HInfo.Count > 0 {
		var scheduledFor *time.Time
		var uniqueTime string
		if r.cfg.Development {
			uniqueTime = fmt.Sprintf("%v:%02d:%02d %02d:%02d:%02d", now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
			scheduledFor = time.New(time.Now().Add(1 * stdlibtime.Minute))
		} else {
			daysPassed := (stdlibtime.Duration(time.Now().Sub(*newReferralCountLast24HInfo.CreatedAt.Time).Hours()/24) + 1) * 24 * stdlibtime.Hour
			nextTime := time.New(newReferralCountLast24HInfo.CreatedAt.Add(daysPassed))
			uniqueTime = fmt.Sprintf("%v:%02d:%02d %02d:%02d:%02d", nextTime.Year(), int(nextTime.Month()), nextTime.Day(), nextTime.Hour(), nextTime.Minute(), nextTime.Second()) //nolint:lll // .
			scheduledFor = nextTime
		}
		if iErr := insertScheduledNotifications(ctx, r.db, []*scheduledNotification{{
			ScheduledAt:              now,
			ScheduledFor:             scheduledFor,
			Language:                 us.Language,
			UserID:                   us.ReferredBy,
			NotificationType:         string(NewReferralNotificationType),
			Uniqueness:               fmt.Sprintf("%v_%v_%v", us.ReferredBy, NewReferralNotificationType, uniqueTime),
			NotificationChannel:      string(TelegramNotificationChannel),
			NotificationChannelValue: us.ReferredBy,
			Data: &users.JSON{
				"TenantName":            r.cfg.TenantName,
				"DailyNewReferralCount": newReferralCountLast24HInfo.Count,
				"Amount":                r.getNewReferralCoinAmount(ctx, us.User.ReferredBy) * newReferralCountLast24HInfo.Count,
				"Coin":                  strings.ToUpper(r.cfg.TokenName),
			},
		}}); iErr != nil {
			return errors.Wrapf(iErr, "can't insert scheduled notifications for:%v", us.ReferredBy)
		}
	}

	return errors.Wrap(executeConcurrently(exConcurrently...), "failed to executeConcurrently")
}

func (r *repository) getNewReferralCountLast24H(ctx context.Context, referredBy string) (info *newReferralCountLast24HInfo, err error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `SELECT 
				COALESCE(count(*), 0) AS count,
				(SELECT created_at FROM users WHERE id = $1) AS created_at
			FROM users
			WHERE referred_by = $1 AND created_at > $2`
	resp, err := storage.Get[newReferralCountLast24HInfo](ctx, r.eskimoDB, sql, referredBy, time.New(time.Now().Add(-24*stdlibtime.Hour)))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to select new referral count for referredBy:`%v`", referredBy)
	}

	return resp, nil
}

func (r *repository) getNewReferralCoinAmount(ctx context.Context, referredBy string) uint64 {
	freezerInternalID, err := tokenomics.GetInternalID(ctx, r.freezerDB, referredBy)
	if err != nil {
		log.Error(errors.Wrapf(err, "failed to tokenomics.GetInternalID for referredBy: %v", referredBy))

		return uint64(r.cfg.WelcomeBonusV2Amount)
	}
	state, err := storagev3.Get[struct {
		model.UserIDField
		model.PreStakingBonusField
		model.BalanceT1WelcomeBonusPendingField
	}](ctx, r.freezerDB, model.SerializedUsersKey(freezerInternalID))
	if err != nil || len(state) == 0 {
		if err == nil {
			err = errors.Wrapf(ErrRelationNotFound, "missing state for freezerInternalID:%v referredBy:%v", freezerInternalID, referredBy)
		}
		log.Error(errors.Wrapf(err, "failed to get PreStakingBonus for freezerInternalID:%v referredBy:%v", freezerInternalID, referredBy))

		return uint64(r.cfg.WelcomeBonusV2Amount)
	}
	if state[0].BalanceT1WelcomeBonusPending >= float64(r.cfg.MaxReferralsCount)*r.cfg.WelcomeBonusV2Amount {
		return 0
	}
	if state[0].PreStakingBonus == 0 {
		return uint64(r.cfg.WelcomeBonusV2Amount)
	}

	return uint64((state[0].PreStakingBonus + 100.0) * r.cfg.WelcomeBonusV2Amount / 100.0) //nolint:gomnd,mnd // Nope.
}

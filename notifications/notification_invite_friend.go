// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen // .
func (r *repository) addScheduledInviteFriendNotifications(ctx context.Context, us *users.User) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	now := time.Now()
	var miningSessionDuration stdlibtime.Duration
	if r.cfg.Development {
		miningSessionDuration = 1 * stdlibtime.Minute
	} else {
		miningSessionDuration = 24 * stdlibtime.Hour
	}
	scheduled := []*scheduledNotification{
		{
			ScheduledAt:              now,
			ScheduledFor:             time.New(us.CreatedAt.Add(miningSessionDuration / 24)),
			Language:                 us.Language,
			UserID:                   us.ID,
			NotificationType:         string(InviteFriendNotificationType),
			Uniqueness:               fmt.Sprintf("%v_%v", us.ID, time.New(us.CreatedAt.Add(miningSessionDuration/24))),
			NotificationChannel:      string(PushNotificationChannel),
			NotificationChannelValue: us.ID,
			Data:                     &users.JSON{},
		}, {
			ScheduledAt:              now,
			ScheduledFor:             time.New(us.CreatedAt.Add(7 * miningSessionDuration)), //nolint:gomnd,mnd // .
			Language:                 us.Language,
			UserID:                   us.ID,
			NotificationType:         string(InviteFriendNotificationType),
			Uniqueness:               fmt.Sprintf("%v_%v", us.ID, time.New(us.CreatedAt.Add(7*miningSessionDuration))), //nolint:gomnd,mnd // .
			NotificationChannel:      string(PushNotificationChannel),
			NotificationChannelValue: us.ID,
			Data:                     &users.JSON{},
		},
	}

	return errors.Wrapf(r.insertScheduledNotifications(ctx, scheduled), "can't execute insertScheduledNotifications:%#v", scheduled)
}

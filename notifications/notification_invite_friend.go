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
	var dayDuration stdlibtime.Duration
	if r.cfg.Development {
		dayDuration = 1 * stdlibtime.Minute
	} else {
		dayDuration = 24 * stdlibtime.Hour
	}
	scheduled := []*scheduledNotification{
		{
			ScheduledAt:              now,
			ScheduledFor:             time.New(us.CreatedAt.Add(dayDuration / 24)),
			Language:                 us.Language,
			UserID:                   us.ID,
			NotificationType:         string(InviteFriendNotificationType),
			Uniqueness:               fmt.Sprintf("%v_%v", us.ID, time.New(us.CreatedAt.Add(dayDuration/24)).UnixNano()),
			NotificationChannel:      string(PushNotificationChannel),
			NotificationChannelValue: us.ID,
			Data:                     &users.JSON{},
		}, {
			ScheduledAt:              now,
			ScheduledFor:             time.New(us.CreatedAt.Add(7 * dayDuration)), //nolint:gomnd,mnd // .
			Language:                 us.Language,
			UserID:                   us.ID,
			NotificationType:         string(InviteFriendNotificationType),
			Uniqueness:               fmt.Sprintf("%v_%v", us.ID, time.New(us.CreatedAt.Add(7*dayDuration)).UnixNano()), //nolint:gomnd,mnd // .
			NotificationChannel:      string(PushNotificationChannel),
			NotificationChannelValue: us.ID,
			Data:                     &users.JSON{},
		},
	}

	return errors.Wrapf(r.insertScheduledNotifications(ctx, scheduled), "can't execute insertScheduledNotifications:%#v", scheduled)
}

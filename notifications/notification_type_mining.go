// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/freezer/tokenomics"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen // .
func (m *miningSource) Process(ctx context.Context, msg *messagebroker.Message) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	message := new(tokenomics.MiningSession)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.UserID == nil ||
		*message.UserID == "" ||
		message.StartedAt == nil ||
		message.EndedAt == nil {
		return nil
	}
	usr, err := m.getUserByID(ctx, *message.UserID)
	if err != nil {
		return errors.Wrapf(err, "can't find the user: %#v", usr)
	}
	notificationTypes := []NotificationType{
		MiningEndingSoonNotificationType,
		MiningExpiredNotificationType,
		MiningExtendNotificationType,
		MiningNotActiveNotificationType,
	}
	if rErr := m.removeScheduledNotifications(ctx, *message.UserID, notificationTypes); rErr != nil {
		return errors.Wrapf(rErr, "can't remove scheduled notifications for:%v", *message.UserID)
	}

	return errors.Wrapf(m.upsertScheduledNotifications(ctx, message, usr.Language), "can't insert/update mining scheduled notifications for:%#v", message)
}

//nolint:funlen // .
func (m *miningSource) upsertScheduledNotifications(ctx context.Context, message *tokenomics.MiningSession, language string) error {
	now := time.Now()
	const notificationsNum = 7
	var miningSessionDuration stdlibtime.Duration
	if m.cfg.Development {
		miningSessionDuration = 1 * stdlibtime.Minute
	} else {
		miningSessionDuration = 24 * stdlibtime.Hour
	}
	scheduled := make([]*scheduledNotification, 0, notificationsNum)
	scheduled = append(scheduled, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(-(miningSessionDuration / 2))),
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningExtendNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v", *message.UserID, time.New(message.EndedAt.Add(-(miningSessionDuration / 2)))),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(-miningSessionDuration / 24)),
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningEndingSoonNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v", *message.UserID, time.New(message.EndedAt.Add(-miningSessionDuration/24))),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             message.EndedAt,
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningExpiredNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v", *message.UserID, message.EndedAt),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(miningSessionDuration)),
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningNotActiveNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v", *message.UserID, time.New(message.EndedAt.Add(miningSessionDuration))),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data: &users.JSON{
			"Days": 1,
		},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(7 * miningSessionDuration)), //nolint:gomnd,mnd // .
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningNotActiveNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v", *message.UserID, time.New(message.EndedAt.Add(7*miningSessionDuration))), //nolint:gomnd,mnd // .
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data: &users.JSON{
			"Days": 7,
		},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(14 * miningSessionDuration)), //nolint:gomnd,mnd // .
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningNotActiveNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v", *message.UserID, time.New(message.EndedAt.Add(14*miningSessionDuration))), //nolint:gomnd,mnd // .
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data: &users.JSON{
			"Days": 14,
		},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(30 * miningSessionDuration)), //nolint:gomnd,mnd // .
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningNotActiveNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v", *message.UserID, time.New(message.EndedAt.Add(30*miningSessionDuration))), //nolint:gomnd,mnd // .
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data: &users.JSON{
			"Days": 30,
		},
	})

	return errors.Wrapf(m.insertScheduledNotifications(ctx, scheduled), "can't execute insertScheduledNotifications:%#v", scheduled)
}

func (r *repository) insertScheduledNotifications(ctx context.Context, scheduled []*scheduledNotification) error {
	const numFields = 9
	values := make([]string, 0, numFields*len(scheduled))
	params := make([]any, 0, numFields*len(scheduled))
	for ix, sch := range scheduled {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v, $%[6]v, $%[7]v, $%[8]v, $%[9]v)", ix*numFields+1, ix*numFields+2, ix*numFields+3, ix*numFields+4, ix*numFields+5, ix*numFields+6, ix*numFields+7, ix*numFields+8, ix*numFields+9)) //nolint:lll,gomnd,mnd // .
		params = append(params, sch.ScheduledAt.Time, sch.ScheduledFor.Time, sch.Data, sch.Language, sch.UserID, sch.Uniqueness, sch.NotificationType, sch.NotificationChannel, sch.NotificationChannelValue)                                                            //nolint:lll // .
	}
	sql := fmt.Sprintf(`INSERT INTO scheduled_notifications(
				scheduled_at,
				scheduled_for,
				data,
				language,
				user_id,
				uniqueness,
				notification_type,
				notification_channel,
				notification_channel_value
			)
			VALUES %v
			ON CONFLICT(user_id, uniqueness, notification_type, notification_channel, notification_channel_value)
			DO UPDATE SET
				scheduled_at  = EXCLUDED.scheduled_at,
				scheduled_for = EXCLUDED.scheduled_for,
				data 		  = EXCLUDED.data,
				language      = EXCLUDED.language`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, r.db, sql, params...)

	return errors.Wrapf(err, "can't execute insert/update scheduled notifications records:%#v", values)
}

func (r *repository) removeScheduledNotifications(ctx context.Context, userID string, nt []NotificationType) error {
	sql := `DELETE FROM scheduled_notifications
						WHERE user_id = $1
							  AND notification_type = ANY($2)`

	_, err := storage.Exec(ctx, r.db, sql, userID, nt)

	return errors.Wrapf(err, "can't remove scheduled notification records for userID:%v, notification types:%#v", userID, nt)
}

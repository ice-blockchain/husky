// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/freezer/tokenomics"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (m *miningSessionSource) Process(ctx context.Context, msg *messagebroker.Message) error {
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
	old, err := m.getMiningScheduledNotifications(ctx, *message.UserID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return errors.Wrapf(err, "can't get mining scheduled notifications for:%v", *message.UserID)
	}
	notificationTypes := []NotificationType{
		MiningEndingSoonNotificationType,
		MiningExpiredNotificationType,
		MiningExtendNotificationType,
	}
	if len(old) > 0 {
		if rErr := m.removeScheduledNotifications(ctx, *message.UserID, notificationTypes); rErr != nil {
			return errors.Wrapf(rErr, "can't remove scheduled notifications for:%v", *message.UserID)
		}
	}
	if iErr := m.insertMiningScheduledNotifications(ctx, message, usr.Language); iErr != nil {
		if errors.Is(iErr, storage.ErrDuplicate) {
			log.Warn("race condition for: %#v", message)

			return nil
		}
		var mErr *multierror.Error
		if len(old) > 0 {
			mErr = multierror.Append(mErr, errors.Wrapf(m.insertScheduledNotifications(ctx, old), "can't rollback scheduled notifications for:%v", message))
		}

		return errors.Wrapf(multierror.Append(
			mErr,
			errors.Wrapf(iErr, "can't insert mining scheduled notifications for:%#v", message),
		).ErrorOrNil(), "one of executions failed for:%#v", message)
	}

	return nil
}

func (m *miningSessionSource) getMiningScheduledNotifications(ctx context.Context, userID string) (resp []*scheduledNotification, err error) {
	notificationTypes := []NotificationType{
		MiningEndingSoonNotificationType,
		MiningExpiredNotificationType,
		MiningExtendNotificationType,
	}
	sql := `SELECT *
				FROM scheduled_notifications
				WHERE user_id = $1
				      AND notification_type = ANY($2)`
	resp, err = storage.ExecMany[scheduledNotification](ctx, m.db, sql, userID, notificationTypes)

	return resp, errors.Wrapf(err, "failed to get mining scheduled notifications for userID:%v", userID)
}

//nolint:funlen // .
func (m *miningSessionSource) insertMiningScheduledNotifications(ctx context.Context, message *tokenomics.MiningSession, language string) error {
	now := time.Now()
	const notificationsNum = 7
	var dayDuration stdlibtime.Duration
	var uniquenessTime string
	if m.cfg.Development {
		dayDuration = 1 * stdlibtime.Minute
		uniquenessTime = fmt.Sprintf("%v:%02d:%02d %02d:%02d:%02d", now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), 0)
	} else {
		dayDuration = 24 * stdlibtime.Hour
		uniquenessTime = fmt.Sprintf("%v:%02d:%02d", now.Year(), int(now.Month()), now.Day())
	}
	scheduled := make([]*scheduledNotification, 0, notificationsNum)
	scheduled = append(scheduled, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             message.ResettableStartingAt,
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningExtendNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v_%v", *message.UserID, MiningExtendNotificationType, uniquenessTime),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data: &users.JSON{
			"TenantName": m.cfg.TenantName,
		},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             message.WarnAboutExpirationStartingAt,
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningEndingSoonNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v_%v", *message.UserID, MiningEndingSoonNotificationType, uniquenessTime),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             message.EndedAt,
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningNotActiveNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v_%v", *message.UserID, MiningNotActiveNotificationType, uniquenessTime),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(dayDuration)),
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningExpiredNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v_1d_%v", *message.UserID, MiningExpiredNotificationType, uniquenessTime),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(7 * dayDuration)), //nolint:gomnd,mnd // .
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningExpiredNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v_7d_%v", *message.UserID, MiningExpiredNotificationType, uniquenessTime),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(14 * dayDuration)), //nolint:gomnd,mnd // .
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningExpiredNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v_14d_%v", *message.UserID, MiningExpiredNotificationType, uniquenessTime),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
	}, &scheduledNotification{
		ScheduledAt:              now,
		ScheduledFor:             time.New(message.EndedAt.Add(30 * dayDuration)), //nolint:gomnd,mnd // .
		Language:                 language,
		UserID:                   *message.UserID,
		NotificationType:         string(MiningExpiredNotificationType),
		Uniqueness:               fmt.Sprintf("%v_%v_30d_%v", *message.UserID, MiningExpiredNotificationType, uniquenessTime),
		NotificationChannel:      string(PushNotificationChannel),
		NotificationChannelValue: *message.UserID,
		Data:                     &users.JSON{},
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
			VALUES %v`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, r.db, sql, params...)

	return errors.Wrapf(err, "can't execute insert scheduled notifications records:%#v", values)
}

func (r *repository) removeScheduledNotifications(ctx context.Context, userID string, nt []NotificationType) error {
	sql := `DELETE FROM scheduled_notifications
						WHERE user_id = $1
							  AND notification_type = ANY($2)`

	_, err := storage.Exec(ctx, r.db, sql, userID, nt)

	return errors.Wrapf(err, "can't remove scheduled notification records for userID:%v, notification types:%#v", userID, nt)
}

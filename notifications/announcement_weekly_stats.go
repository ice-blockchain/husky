// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

func (s *Scheduler) addScheduledWeeklyStatsAnnouncement(ctx context.Context) error {
	now := time.Now()
	languages := allPushNotificationTemplates[WeeklyStatsNotificationType]
	scheduled := make([]*scheduledAnnouncement, 0, len(languages))
	var nextTime *time.Time
	var nextTimeFormatted string
	if s.cfg.Development {
		nextTime = time.New(now.Add(stdlibtime.Minute))
		nextTimeFormatted = fmt.Sprintf("%v:%02d:%02d %02d:%02d:%02d", nextTime.Year(), int(nextTime.Month()), nextTime.Day(), nextTime.Hour(), nextTime.Minute(), nextTime.Second()) //nolint:lll // .
	} else {
		nextTime = nextTimeMatch(now, s.cfg.WeeklyStats.Weekday, s.cfg.WeeklyStats.Hour, s.cfg.WeeklyStats.Minutes)
		nextTimeFormatted = fmt.Sprintf("%v:%02d:%02d", nextTime.Year(), int(nextTime.Month()), nextTime.Day())
	}
	for language := range languages {
		scheduled = append(scheduled, &scheduledAnnouncement{
			ScheduledAt:              now,
			ScheduledFor:             nextTime,
			Language:                 language,
			Uniqueness:               fmt.Sprintf("%v_%v_%v", WeeklyStatsNotificationType, language, nextTimeFormatted),
			NotificationType:         string(WeeklyStatsNotificationType),
			NotificationChannel:      string(PushNotificationChannel),
			NotificationChannelValue: fmt.Sprintf("system_%v", language),
			Data:                     &users.JSON{},
		})
	}

	return errors.Wrap(s.insertScheduledAnnouncement(ctx, scheduled), "can't insert stats scheduled announcements")
}

func (s *Scheduler) insertScheduledAnnouncement(ctx context.Context, scheduled []*scheduledAnnouncement) error {
	const numFields = 8
	values := make([]string, 0, len(scheduled)*numFields)
	params := make([]any, 0, len(scheduled)*numFields)
	for ix, sch := range scheduled {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v, $%[6]v, $%[7]v, $%[8]v)", ix*numFields+1, ix*numFields+2, ix*numFields+3, ix*numFields+4, ix*numFields+5, ix*numFields+6, ix*numFields+7, ix*numFields+8)) //nolint:lll,gomnd,mnd // .
		params = append(params, sch.ScheduledAt.Time, sch.ScheduledFor.Time, sch.Data, sch.Language, sch.Uniqueness, sch.NotificationType, sch.NotificationChannel, sch.NotificationChannelValue)                                                //nolint:lll // .
	}
	sql := fmt.Sprintf(`INSERT INTO scheduled_announcements(
							scheduled_at,
							scheduled_for,
							data,
							language,
							uniqueness,
							notification_type,
							notification_channel,
							notification_channel_value)
						VALUES %v
				`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, s.db, sql, params...)
	if err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			return errors.Wrapf(ErrDuplicate, "scheduled announcement has already existed:%v", params...)
		}

		return errors.Wrapf(err, "can't execute insert/update scheduled announcement records:%v", params...)
	}

	return nil
}

func nextTimeMatch(now *time.Time, weekday stdlibtime.Weekday, hour, minute int) *time.Time {
	y, m, d := now.Date()
	loc := now.Location()
	currentWeekday := int(now.Weekday())
	beginningOfDay := stdlibtime.Date(y, m, d, 0, 0, 0, 0, loc)
	beginningOfWeek := beginningOfDay.AddDate(0, 0, -currentWeekday)

	var nextDate stdlibtime.Time
	if (currentWeekday == int(weekday) && now.Hour() < hour) || currentWeekday < int(weekday) {
		nextDate = beginningOfWeek.AddDate(0, 0, int(weekday))
	} else {
		nextDate = beginningOfWeek.AddDate(0, 0, int(weekday)+7) //nolint:mnd,gomnd // .
	}

	nextY, nextM, nextD := nextDate.Date()
	nextLoc := nextDate.Location()

	return time.New(stdlibtime.Date(nextY, nextM, nextD, hour, minute, 0, 0, nextLoc))
}

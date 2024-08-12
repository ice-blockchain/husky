// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	stdlibtime "time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	appcfg "github.com/ice-blockchain/wintr/config"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/multimedia/picture"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/notifications/telegram"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen // .
func MustStartScheduler(ctx context.Context, cancel context.CancelFunc) *Scheduler {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)

	loadPushNotificationTranslationTemplates(cfg.TenantName)
	loadTelegramNotificationTranslationTemplates(cfg.TenantName)

	ddlWorkersParam := fmt.Sprintf(ddl, schedulerWorkersCount)
	eskimoDB := storage.MustConnect(ctx, "", eskimoApplicationYamlKey)
	sh := Scheduler{
		cfg:                              &cfg,
		db:                               storage.MustConnect(context.Background(), ddlWorkersParam, applicationYamlKey), //nolint:contextcheck // .
		eskimoDB:                         eskimoDB,
		pictureClient:                    picture.New(applicationYamlKey),
		pushNotificationsClient:          push.New(applicationYamlKey),
		wg:                               new(sync.WaitGroup),
		telegramNotificationsClient:      telegram.New(applicationYamlKey),
		telemetryPushNotifications:       new(telemetry).mustInit([]string{"scheduler push notifications[full iteration]", "get_push_scheduled_notifications", "process_push_notifications", "clear_invalid_tokens", "delete_push_scheduled_notifications"}), //nolint:lll // .
		telemetryTelegramNotifications:   new(telemetry).mustInit([]string{"scheduler telegram notifications[full iteration]", "get_telegram_scheduled_notifications", "process_telegram_notifications", "delete_telegram_scheduled_notifications"}),         //nolint:lll // .
		telemetryAnnouncements:           new(telemetry).mustInit([]string{"scheduler announcements[full iteration]", "get_scheduled_announcements", "process_announcements", "delete_scheduled_announcements"}),                                             //nolint:lll // .
		schedulerPushAnnouncementsMX:     &sync.Mutex{},
		schedulerPushNotificationsMX:     &sync.Mutex{},
		schedulerTelegramNotificationsMX: &sync.Mutex{},
	}
	go sh.startWeeklyStatsUpdater(ctx)
	go sh.runOneTimeMiningExtendFeature(ctx)
	sh.wg = new(sync.WaitGroup)
	sh.wg.Add(3 * int(schedulerWorkersCount)) //nolint:gomnd,mnd // .
	sh.cancel = cancel
	for workerNumber := range schedulerWorkersCount {
		go func(wn int64) {
			defer sh.wg.Done()
			sh.runPushNotificationsProcessor(ctx, wn)
		}(workerNumber)
		go func(wn int64) {
			defer sh.wg.Done()
			sh.runPushAnnouncementsProcessor(ctx, wn)
		}(workerNumber)
		go func(wn int64) {
			defer sh.wg.Done()
			sh.runTelegramNotificationsProcessor(ctx, wn)
		}(workerNumber)
	}

	return &sh
}

func (s *Scheduler) CheckHealth(ctx context.Context) error {
	if err := s.db.Ping(ctx); err != nil {
		return errors.Wrap(err, "[health-check] failed to ping DB")
	}
	if err := s.eskimoDB.Ping(ctx); err != nil {
		return errors.Wrap(err, "[health-check] failed to ping eskimo DB")
	}

	return nil
}

func (s *Scheduler) startWeeklyStatsUpdater(ctx context.Context) {
	ticker := stdlibtime.NewTicker(1 * stdlibtime.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, 10*stdlibtime.Minute) //nolint:gomnd,mnd // .
			if err := s.addScheduledWeeklyStatsAnnouncement(reqCtx); err != nil && !errors.Is(err, ErrDuplicate) {
				log.Error(errors.Wrap(err, "failed to addScheduledWeeklyStatsAnnouncement"))
			}
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) Close() error {
	s.cancel()
	s.wg.Wait()

	return errors.Wrap(multierror.Append(nil, s.db.Close(), s.eskimoDB.Close()).ErrorOrNil(), "can't close one of postgres connection")
}

//nolint:funlen // .
func runConcurrentlyBatch[ARG any](
	ctx context.Context, run func(ctx context.Context, arg ARG) error, args []ARG, failureFunc func(arg ARG, err error), successFunc func(arg ARG),
) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if len(args) == 0 {
		return nil
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(args))
	errChan := make(chan error, len(args))
	for idx := range args {
		go func(ix int) {
			defer wg.Done()
			err := errors.Wrapf(run(ctx, args[ix]), "failed to run:%#v", args[ix])
			if err != nil {
				failureFunc(args[ix], err)
			} else {
				successFunc(args[ix])
			}
			errChan <- err
		}(idx)
	}
	wg.Wait()
	close(errChan)
	errs := make([]error, 0, len(args))
	for err := range errChan {
		errs = append(errs, err)
	}

	return errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "at least one execution failed")
}

//nolint:exhaustive // We know what cases need to be handled only.
func (s *Scheduler) getDeeplink(nt NotificationType, data *users.JSON) string {
	switch nt {
	case MiningExtendNotificationType, MiningEndingSoonNotificationType, MiningExpiredNotificationType, MiningNotActiveNotificationType:
		return fmt.Sprintf("%v://home", s.cfg.DeeplinkScheme)
	case InviteFriendNotificationType:
		return fmt.Sprintf("%v://invite", s.cfg.DeeplinkScheme)
	case SocialsFollowIceOnXNotificationType, SocialsFollowUsOnXNotificationType, SocialsFollowZeusOnXNotificationType,
		SocialsFollowIONOnTelegramNotificationType, SocialsFollowOurTelegramNotificationType:
		return fmt.Sprintf("%v://browser?url=%v", s.cfg.DeeplinkScheme, url.QueryEscape(fmt.Sprintf("%v", (*data)["SocialUrl"])))
	case WeeklyStatsNotificationType:
		return fmt.Sprintf("%v://stats", s.cfg.DeeplinkScheme)
	default:
		log.Panic(fmt.Sprintf("wrong notification type:%v", nt))
	}

	return ""
}

//nolint:exhaustive // We know what cases need to be handled only.
func getDomainByNotificationType(nt NotificationType) NotificationDomain {
	switch nt {
	case MiningExtendNotificationType, MiningEndingSoonNotificationType, MiningExpiredNotificationType, MiningNotActiveNotificationType:
		return MiningNotificationDomain
	case InviteFriendNotificationType:
		return MicroCommunityNotificationDomain
	case SocialsFollowIceOnXNotificationType, SocialsFollowUsOnXNotificationType, SocialsFollowZeusOnXNotificationType,
		SocialsFollowIONOnTelegramNotificationType, SocialsFollowOurTelegramNotificationType:
		return PromotionsNotificationDomain
	case WeeklyStatsNotificationType:
		return WeeklyStatsNotificationDomain
	default:
		log.Panic(fmt.Sprintf("wrong notification type:%v", nt))
	}

	return ""
}

func (s *Scheduler) fetchScheduledNotifications(
	ctx context.Context, now *time.Time, notificationChannel NotificationChannel, batchSize, workerNumber int64,
) (resp []*scheduledNotificationInfo, err error) {
	sql := fmt.Sprintf(`SELECT sn.*,
				   array_agg(dm.push_notification_token) filter (where dm.push_notification_token is not null)  AS push_notification_tokens,
				   u.disabled_push_notification_domains,
				   COALESCE(u.telegram_user_id, '') AS telegram_user_id,
				   COALESCE(u.telegram_bot_id, '') AS telegram_bot_id,
				   u.username
			FROM scheduled_notifications sn
				JOIN users u
					ON sn.user_id = u.user_id
				LEFT JOIN device_metadata dm
					ON (u.disabled_push_notification_domains IS NULL 
						OR NOT (u.disabled_push_notification_domains && ARRAY['%v'] )
					)
					AND dm.user_id = u.user_id
					AND dm.push_notification_token IS NOT NULL 
					AND dm.push_notification_token != ''
				WHERE MOD(i, %[2]v) = %[3]v 
				      AND scheduled_for <= $1
					  AND notification_channel = $2
				GROUP BY sn.i, sn.scheduled_at, sn.scheduled_for, sn.data, sn.language, sn.user_id, sn.uniqueness, sn.notification_type,
						 sn.notification_channel, sn.notification_channel_value, u.disabled_push_notification_domains,
						 u.telegram_user_id,u.telegram_bot_id, u.username
				ORDER BY MOD(i, %[2]v), scheduled_for ASC
				LIMIT %[4]v`,
		AllNotificationDomain, schedulerWorkersCount, workerNumber, batchSize)
	resp, err = storage.ExecMany[scheduledNotificationInfo](ctx, s.db, sql, now.Time, notificationChannel)

	return resp, errors.Wrapf(err, "failed to fetch scheduled notifications worker:%v", workerNumber)
}

func (s *Scheduler) markScheduledNotificationAsSent(ctx context.Context, now *time.Time, notifications []*sentNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	const numFields = 7
	values := make([]string, 0, numFields*len(notifications))
	params := make([]any, 0, numFields*len(notifications))
	for idx, n := range notifications {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v, $%[6]v, $%[7]v)", idx*numFields+1, idx*numFields+2, idx*numFields+3, idx*numFields+4, idx*numFields+5, idx*numFields+6, idx*numFields+7)) //nolint:gomnd,mnd,lll // .
		params = append(params, now.Time, n.Language, n.UserID, n.Uniqueness, n.NotificationType, n.NotificationChannel, n.NotificationChannelValue)
	}
	sql := fmt.Sprintf(`WITH sent AS (
		INSERT INTO sent_notifications(sent_at, language, user_id, uniqueness, notification_type, notification_channel, notification_channel_value)
			VALUES %[1]v
			ON CONFLICT DO NOTHING
			RETURNING user_id, uniqueness, notification_type, notification_channel, notification_channel_value
		)
		DELETE FROM scheduled_notifications 
			WHERE (user_id, uniqueness, notification_type, notification_channel, notification_channel_value) IN (SELECT * FROM sent)`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, s.db, sql, params...)

	return errors.Wrapf(err, "failed to delete scheduled notifications %#v", notifications)
}

func (s *Scheduler) deleteScheduledNotifications(ctx context.Context, notifications []*scheduledNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	const numFields = 5
	values := make([]string, 0, numFields*len(notifications))
	params := make([]any, 0, numFields*len(notifications))
	for idx, n := range notifications {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v)", idx*numFields+1, idx*numFields+2, idx*numFields+3, idx*numFields+4, idx*numFields+5)) //nolint:gomnd,mnd,lll // .
		params = append(params, n.UserID, n.Uniqueness, n.NotificationType, n.NotificationChannel, n.NotificationChannelValue)
	}
	sql := fmt.Sprintf(`DELETE FROM scheduled_notifications 
							   WHERE (user_id, uniqueness, notification_type, notification_channel, notification_channel_value) IN (%v)`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, s.db, sql, params...)

	return errors.Wrapf(err, "failed to delete scheduled notifications %#v", notifications)
}

//nolint:funlen // .
func (s *Scheduler) runOneTimeMiningExtendFeature(ctx context.Context) {
	type notActiveUser struct {
		ID       string
		Language string
	}
	const limit = 1000
	offset := uint64(0)
	now := time.Now()
	for {
		sql := fmt.Sprintf(`SELECT 
								id,
								language
							FROM users
							WHERE last_mining_ended_at IS NOT NULL AND last_mining_ended_at < $1
							ORDER BY created_at ASC
							LIMIT %v OFFSET %v`, limit, offset)
		resp, err := storage.Select[notActiveUser](ctx, s.eskimoDB, sql, now)
		if err != nil {
			log.Error(err, "can't get users to send the single mining extend message")
			stdlibtime.Sleep(100 * stdlibtime.Millisecond) //nolint:gomnd,mnd // .

			continue
		}
		if len(resp) == 0 {
			break
		}
		var randMax int
		if s.cfg.Development {
			randMax = 2
		} else {
			randMax = 240
		}
		scheduled := make([]*scheduledNotification, 0, len(resp))
		for _, usr := range resp {
			scheduled = append(scheduled, &scheduledNotification{
				ScheduledAt:              now,
				ScheduledFor:             time.New(time.Now().Add(stdlibtime.Duration(1+rand.Intn(randMax)) * stdlibtime.Minute)), //nolint:gosec // Not an  issue.
				Language:                 usr.Language,
				UserID:                   usr.ID,
				NotificationType:         string(MiningEndingSoonNotificationType),
				Uniqueness:               fmt.Sprintf("%v_%v_single", usr.ID, MiningEndingSoonNotificationType),
				NotificationChannel:      string(TelegramNotificationChannel),
				NotificationChannelValue: usr.ID,
				Data: &users.JSON{
					"TenantName": s.cfg.TenantName,
					"TokenName":  strings.ToUpper(s.cfg.TokenName),
				},
			})
		}
		if iErr := insertScheduledNotificationsWithSentCheck(ctx, s.db, scheduled); iErr != nil {
			log.Error(iErr, "can't insert scheduled notifications for single mining ending soon call", resp)
		}

		offset += limit
		stdlibtime.Sleep(100 * stdlibtime.Millisecond) //nolint:gomnd,mnd // .
	}
	diff := stdlibtime.Since(*now.Time)
	log.Info("Finished handling one time mining extend notifications generation in", diff)
}

//nolint:dupl // .
func insertScheduledNotificationsWithSentCheck(ctx context.Context, db *storage.DB, scheduled []*scheduledNotification) error {
	const numFields = 9
	values := make([]string, 0, numFields*len(scheduled))
	params := make([]any, 0, numFields*len(scheduled))
	for ix, sch := range scheduled {
		values = append(values, fmt.Sprintf("(to_timestamp($%[1]v, 'YYYY-MM-DD HH24:MI:SS'), to_timestamp($%[2]v, 'YYYY-MM-DD HH24:MI:SS'), $%[3]v::jsonb, $%[4]v, $%[5]v, $%[6]v, $%[7]v, $%[8]v, $%[9]v)", ix*numFields+1, ix*numFields+2, ix*numFields+3, ix*numFields+4, ix*numFields+5, ix*numFields+6, ix*numFields+7, ix*numFields+8, ix*numFields+9)) //nolint:lll,gomnd,mnd // .
		params = append(params, sch.ScheduledAt.Time, sch.ScheduledFor.Time, sch.Data, sch.Language, sch.UserID, sch.Uniqueness, sch.NotificationType, sch.NotificationChannel, sch.NotificationChannelValue)                                                                                                                                                 //nolint:lll // .
	}
	sql := fmt.Sprintf(`
				INSERT INTO scheduled_notifications(
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
				SELECT * FROM (VALUES %[1]v)
				WHERE NOT EXISTS (SELECT user_id FROM sent_notifications)`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, db, sql, params...)

	return errors.Wrapf(err, "can't execute insert scheduled notifications records:%#v", values)
}

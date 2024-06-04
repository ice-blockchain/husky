// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"
	"sync"
	stdlibtime "time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	appcfg "github.com/ice-blockchain/wintr/config"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/multimedia/picture"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

func MustStartScheduler(ctx context.Context) *Scheduler {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)

	sh := Scheduler{
		cfg:                      &cfg,
		mb:                       messagebroker.MustConnect(context.Background(), applicationYamlKey), //nolint:contextcheck // .
		db:                       storage.MustConnect(context.Background(), ddl, applicationYamlKey),  //nolint:contextcheck // .
		pictureClient:            picture.New(applicationYamlKey),
		pushNotificationsClient:  push.New(applicationYamlKey),
		telemetry:                new(telemetry).mustInit(),
		schedulerAnnouncementsMX: &sync.Mutex{},
		schedulerNotificationsMX: &sync.Mutex{},
	}

	loadPushNotificationTranslationTemplates()
	go sh.startWeeklyStatsUpdater(ctx)

	wg := new(sync.WaitGroup)
	wg.Add(int(cfg.Scheduler.Workers))
	defer wg.Wait()

	for workerNumber := range cfg.Scheduler.Workers {
		go func(wn int64) {
			defer wg.Done()
			sh.run(ctx, wn)
		}(workerNumber)
	}

	return &sh
}

func (s *Scheduler) CheckHealth(ctx context.Context) error {
	if err := s.db.Ping(ctx); err != nil {
		return errors.Wrap(err, "[health-check] failed to ping DB")
	}

	return nil
}

func (s *Scheduler) startWeeklyStatsUpdater(ctx context.Context) {
	var duration stdlibtime.Duration
	if s.cfg.Development {
		duration = stdlibtime.Minute
	} else {
		duration = 24 * stdlibtime.Hour
	}
	ticker := stdlibtime.NewTicker(duration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, 10*stdlibtime.Minute) //nolint:gomnd,mnd // .
			log.Error(errors.Wrap(s.addScheduledWeeklyStatsAnnouncement(reqCtx), "failed to addScheduledWeeklyStatsAnnouncement"))
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) Close() error {
	return errors.Wrap(multierror.Append(
		errors.Wrap(s.mb.Close(), "failed to close mb"),
		errors.Wrap(s.db.Close(), "failed to close db"),
	).ErrorOrNil(), "can't close the scheduler")
}

//nolint:funlen,gocognit,gocyclo,revive,cyclop,maintidx // .
func (s *Scheduler) run(ctx context.Context, workerNumber int64) {
	var (
		batchNumber                 int64
		now, lastIterationStartedAt = time.Now(), time.Now()
		batchSize                   = s.cfg.Scheduler.BatchSize
		errs                        = make([]error, 0)
		successedNotifications      = make([]*pushNotification, 0, batchNumber)
		emptyTokensNotifications    = make([]*scheduledNotification, 0, batchNumber)
		invalidTokens               = make([]*invalidToken, 0)
		successedAnnouncements      = make([]*broadcastPushNotification[push.Notification[push.SubscriptionTopic]], 0, batchNumber)
		exConcurrently              = make([]func() error, 0)
		toSendPushNotifications     = make([]*pushNotification, 0)
		notifications               = make([]*scheduledNotification, batchNumber)
		announcements               = make([]*scheduledAnnouncement, batchNumber)
		toSendAnnouncements         = make([]*broadcastPushNotification[push.Notification[push.SubscriptionTopic]], 0, batchNumber)
		imageURLNotification        = s.pictureClient.DownloadURL("assets/push-notifications/batch-notification.png")
		imageURLAnnouncement        = s.pictureClient.DownloadURL("assets/push-notifications/batch-announcement.png")
		err                         error
	)
	resetVars := func(success bool) {
		if success && len(notifications) < int(batchSize) && len(announcements) < int(batchSize) {
			go s.telemetry.collectElapsed(0, *lastIterationStartedAt.Time)
			batchNumber = 0
		}
		now = time.Now()
		errs = errs[:0]
		successedNotifications = successedNotifications[:0]
		successedAnnouncements = successedAnnouncements[:0]
		exConcurrently = exConcurrently[:0]
		toSendPushNotifications = toSendPushNotifications[:0]
		toSendAnnouncements = toSendAnnouncements[:0]
		invalidTokens = invalidTokens[:0]
		notifications = notifications[:0]
		announcements = announcements[:0]
		lastIterationStartedAt = time.Now()
	}

	for ctx.Err() == nil {
		/******************************************************************************************************************************************************
			1. Fetching a new batch of scheduled messages.
		******************************************************************************************************************************************************/
		before := time.Now()
		reqCtx, reqCancel := context.WithTimeout(ctx, requestDeadline)
		notifications, err = s.fetchScheduledNotifications(reqCtx, now, workerNumber)
		if err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to fetch scheduled notifications for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)

			continue
		}
		if len(notifications) > 0 {
			go s.telemetry.collectElapsed(1, *before.Time)
		}

		before = time.Now()
		announcements, err = s.fetchScheduledAnnouncements(ctx, now, workerNumber)
		if err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to fetch scheduled announcements for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)

			continue
		}
		if len(announcements) > 0 {
			go s.telemetry.collectElapsed(2, *before.Time) //nolint:gomnd,mnd // .
		}
		reqCancel()

		/******************************************************************************************************************************************************
			2. Processing batch of notifications.
		******************************************************************************************************************************************************/
		before = time.Now()
	out:
		for _, notification := range notifications {
			tmpl, found := allPushNotificationTemplates[NotificationType(notification.NotificationType)][notification.Language]
			if !found {
				log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", notification.Language, notification.NotificationType))

				continue
			}
			if notification.DisabledPushNotificationDomains != nil {
				domain := getDomainByNotificationType(NotificationType(notification.NotificationType))
				for _, disabledDomain := range *notification.DisabledPushNotificationDomains {
					if disabledDomain == domain {
						log.Warn(fmt.Sprintf("notification with disabled notification domain:%v with notification type:%v for notification:%#v", domain, notification.NotificationType, notification)) //nolint:lll // .

						continue out
					}
				}
			}
			if notification.PushNotificationTokens == nil || len(*notification.PushNotificationTokens) == 0 {
				emptyTokensNotifications = append(emptyTokensNotifications, notification)

				continue
			}
			for _, deviceToken := range *notification.PushNotificationTokens {
				var data map[string]string
				if notification.Data != nil {
					data = make(map[string]string, len(*notification.Data))
					for key, val := range *notification.Data {
						data[key] = fmt.Sprintf("%v", val)
					}
				}
				notif := &pushNotification{
					pn: &push.Notification[push.DeviceToken]{
						Data:     map[string]string{"deeplink": s.getDeeplink(NotificationType(notification.NotificationType), notification.Data)},
						Target:   deviceToken,
						Title:    tmpl.getTitle(nil),
						Body:     tmpl.getBody(data),
						ImageURL: imageURLNotification,
					},
					sn: &sentNotification{
						SentAt:   now,
						Language: notification.Language,
						sentNotificationPK: sentNotificationPK{
							UserID:                   notification.UserID,
							Uniqueness:               notification.Uniqueness,
							NotificationType:         NotificationType(notification.NotificationType),
							NotificationChannel:      NotificationChannel(notification.NotificationChannel),
							NotificationChannelValue: notification.NotificationChannelValue,
						},
					},
				}
				toSendPushNotifications = append(toSendPushNotifications, notif)
			}
		}
		exConcurrently = append(exConcurrently, func() error {
			return errors.Wrapf(runConcurrentlyBatch(ctx, s.sendPushNotification, toSendPushNotifications, func(arg *pushNotification, err error) {
				if errors.Is(err, push.ErrInvalidDeviceToken) {
					s.schedulerNotificationsMX.Lock()
					invalidTokens = append(invalidTokens, &invalidToken{
						UserID: arg.sn.UserID,
						Token:  arg.pn.Target,
					})
					s.schedulerNotificationsMX.Unlock()
				}
			}, func(arg *pushNotification) {
				s.schedulerNotificationsMX.Lock()
				defer s.schedulerNotificationsMX.Unlock()

				successedNotifications = append(successedNotifications, arg)
			}), "can't execute notifications batch concurrently for:%#v", toSendPushNotifications)
		})

		/******************************************************************************************************************************************************
			3. Processing batch of announcements.
		******************************************************************************************************************************************************/
		for _, an := range announcements {
			tmpl, found := allPushNotificationTemplates[NotificationType(an.NotificationType)][an.Language]
			if !found {
				log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` push config", an.Language, an.NotificationType))

				continue
			}
			toSendAnnouncements = append(toSendAnnouncements, &broadcastPushNotification[push.Notification[push.SubscriptionTopic]]{
				pn: &push.Notification[push.SubscriptionTopic]{
					Data:     getData(an.Data),
					Target:   push.SubscriptionTopic(an.NotificationChannelValue),
					Title:    tmpl.getTitle(nil),
					Body:     tmpl.getBody(nil),
					ImageURL: imageURLAnnouncement,
				},
				sa: &sentAnnouncement{
					SentAt:   now,
					Language: an.Language,
					sentAnnouncementPK: sentAnnouncementPK{
						Uniqueness:               an.Uniqueness,
						NotificationType:         NotificationType(an.NotificationType),
						NotificationChannel:      PushNotificationChannel,
						NotificationChannelValue: an.NotificationChannelValue,
					},
				},
			})
		}
		exConcurrently = append(exConcurrently, func() error {
			reqCtx, reqCancel = context.WithTimeout(context.Background(), requestDeadline)
			defer reqCancel()

			return errors.Wrapf(runConcurrentlyBatch(ctx, s.broadcastPushNotification, toSendAnnouncements, func(arg *broadcastPushNotification[push.Notification[push.SubscriptionTopic]], err error) { //nolint:lll // .
			}, func(arg *broadcastPushNotification[push.Notification[push.SubscriptionTopic]]) {
				s.schedulerAnnouncementsMX.Lock()
				defer s.schedulerAnnouncementsMX.Unlock()

				successedAnnouncements = append(successedAnnouncements, arg)
			}), "can't execute notifications batch concurrently for:%#v", toSendPushNotifications)
		})

		/******************************************************************************************************************************************************
			4. Send all notifications + announcements concurrently.
		******************************************************************************************************************************************************/
		if eErr := executeConcurrently(exConcurrently...); eErr != nil {
			log.Error(errors.Wrapf(eErr, "[scheduler] failed to execute concurrently sending push notifications for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber)) //nolint:lll // .
		}
		if len(toSendPushNotifications)+len(toSendAnnouncements) > 0 {
			go s.telemetry.collectElapsed(3, *before.Time) //nolint:gomnd,mnd // .
		}

		/******************************************************************************************************************************************************
			5. Clear invalid push notification tokens
		******************************************************************************************************************************************************/
		before = time.Now()
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if cErr := s.clearInvalidPushNotificationTokens(reqCtx, invalidTokens); cErr != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to clear invalid push notification tokens for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
		}
		reqCancel()
		if len(invalidTokens) > 0 {
			go s.telemetry.collectElapsed(4, *before.Time) //nolint:gomnd,mnd // .
		}

		/******************************************************************************************************************************************************
			6. Delete notifications from scheduled.
		******************************************************************************************************************************************************/
		before = time.Now()
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if dErr := s.insertSentNotifications(reqCtx, now, successedNotifications); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't insert sent scheduled notification"))
		}
		if dErr := s.deleteScheduledNotifications(reqCtx, emptyTokensNotifications); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't delete scheduled notifications for:%#v", emptyTokensNotifications))
		}
		reqCancel()
		if len(successedNotifications) > 0 {
			go s.telemetry.collectElapsed(5, *before.Time) //nolint:gomnd,mnd // .
		}

		/******************************************************************************************************************************************************
			7. Delete announcements from scheduled.
		******************************************************************************************************************************************************/
		before = time.Now()
		reqCtx, reqCancel = context.WithTimeout(ctx, requestDeadline)
		if dErr := s.deleteScheduledAnnouncements(reqCtx, now, successedAnnouncements); dErr != nil {
			errs = append(errs, errors.Wrapf(dErr, "can't delete scheduled announcements"))
		}
		reqCancel()

		if err = multierror.Append(nil, errs...).ErrorOrNil(); err != nil {
			log.Error(errors.Wrapf(err, "[scheduler] failed to delete scheduled announcements for batchNumber:%v,workerNumber:%v", batchNumber, workerNumber))
			resetVars(false)

			continue
		}
		if len(successedAnnouncements) > 0 {
			go s.telemetry.collectElapsed(6, *before.Time) //nolint:gomnd,mnd // .
		}

		batchNumber++
		resetVars(true)
	}
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

func (s *Scheduler) sendPushNotification(ctx context.Context, pn *pushNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	responder := make(chan error, 1)
	defer close(responder)
	s.pushNotificationsClient.Send(ctx, pn.pn, responder)

	return errors.Wrapf(<-responder, "can't send push notification for pn:%#v", pn)
}

func (s *Scheduler) broadcastPushNotification(
	ctx context.Context, bpn *broadcastPushNotification[push.Notification[push.SubscriptionTopic]],
) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}

	return errors.Wrapf(s.pushNotificationsClient.Broadcast(ctx, bpn.pn), "can't broadcast push notification for:%#v", bpn)
}

func (s *Scheduler) clearInvalidPushNotificationTokens(ctx context.Context, pnInvalidTokens []*invalidToken) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	tokens := make([]push.DeviceToken, 0, len(pnInvalidTokens))
	userIDs := make([]string, 0, len(pnInvalidTokens))
	for _, inv := range pnInvalidTokens {
		tokens = append(tokens, inv.Token)
		userIDs = append(userIDs, inv.UserID)
	}
	sql := `UPDATE device_metadata
				SET push_notification_token = null
				WHERE user_id = ANY($1)
					  AND push_notification_token = ANY($2)`
	_, err := storage.Exec(ctx, s.db, sql, userIDs, tokens)

	return errors.Wrapf(err, "failed to update push_notification_token to empty for userID:%v and token %v", userIDs, tokens)
}

func (s *Scheduler) insertSentNotifications(ctx context.Context, now *time.Time, notifications []*pushNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	const numFields = 7
	values := make([]string, 0, numFields*len(notifications))
	params := make([]any, 0, numFields*len(notifications))
	for idx, n := range notifications {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v, $%[6]v, $%[7]v)", idx*numFields+1, idx*numFields+2, idx*numFields+3, idx*numFields+4, idx*numFields+5, idx*numFields+6, idx*numFields+7)) //nolint:gomnd,mnd,lll // .
		params = append(params, now.Time, n.sn.Language, n.sn.UserID, n.sn.Uniqueness, n.sn.NotificationType, n.sn.NotificationChannel, n.sn.NotificationChannelValue)
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
	if errors.Is(err, storage.ErrDuplicate) {
		return nil
	}

	return errors.Wrapf(err, "failed to delete scheduled notifications %#v", notifications)
}

func (s *Scheduler) deleteScheduledAnnouncements(
	ctx context.Context, now *time.Time, announcements []*broadcastPushNotification[push.Notification[push.SubscriptionTopic]],
) error {
	if len(announcements) == 0 {
		return nil
	}
	const fieldNumber = 6
	values := make([]string, 0, fieldNumber*len(announcements))
	params := make([]any, 0, fieldNumber*len(announcements))
	for idx, an := range announcements {
		values = append(values, fmt.Sprintf("($%[1]v, $%[2]v, $%[3]v, $%[4]v, $%[5]v, $%[6]v)", idx*fieldNumber+1, idx*fieldNumber+2, idx*fieldNumber+3, idx*fieldNumber+4, idx*fieldNumber+5, idx*fieldNumber+6)) //nolint:mnd,gomnd,lll // .
		params = append(params, now.Time, an.sa.Language, an.sa.Uniqueness, an.sa.NotificationType, an.sa.NotificationChannel, an.sa.NotificationChannelValue)
	}
	sql := fmt.Sprintf(`WITH sent AS (
		INSERT INTO sent_announcements(sent_at, language, uniqueness, notification_type, notification_channel, notification_channel_value)
			VALUES %[1]v
			RETURNING uniqueness, notification_type, notification_channel, notification_channel_value
		)
		DELETE FROM scheduled_announcements 
			WHERE (uniqueness, notification_type, notification_channel, notification_channel_value) IN (SELECT * FROM sent)`, strings.Join(values, ","))
	_, err := storage.Exec(ctx, s.db, sql, params...)
	if errors.Is(err, storage.ErrDuplicate) {
		return nil
	}

	return errors.Wrapf(err, "failed to delete scheduled announcements %#v", announcements)
}

func (s *Scheduler) fetchScheduledNotifications(ctx context.Context, now *time.Time, workerNumber int64) (resp []*scheduledNotification, err error) {
	sql := fmt.Sprintf(`SELECT sn.*,
				   array_agg(dm.push_notification_token) filter (where dm.push_notification_token is not null)  AS push_notification_tokens,
				   u.disabled_push_notification_domains
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
				WHERE MOD(i, %v) = %v 
				      AND scheduled_for <= $1
				GROUP BY sn.i, sn.scheduled_at, sn.scheduled_for, sn.data, sn.language, sn.user_id, sn.uniqueness, sn.notification_type,
					     sn.notification_channel, sn.notification_channel_value, u.disabled_push_notification_domains
				LIMIT %v`,
		AllNotificationDomain, s.cfg.Scheduler.Workers, workerNumber, s.cfg.Scheduler.BatchSize)
	resp, err = storage.ExecMany[scheduledNotification](ctx, s.db, sql, now.Time)

	return resp, errors.Wrapf(err, "failed to fetch scheduled notifications worker:%v", workerNumber)
}

func (s *Scheduler) fetchScheduledAnnouncements(ctx context.Context, now *time.Time, workerNumber int64) (resp []*scheduledAnnouncement, err error) {
	sql := fmt.Sprintf(`SELECT sn.*
				FROM scheduled_announcements sn
				WHERE MOD(i, %v) = %v
				AND scheduled_for <= $1
			LIMIT %v;`, s.cfg.Scheduler.Workers, workerNumber, s.cfg.Scheduler.BatchSize)
	resp, err = storage.ExecMany[scheduledAnnouncement](ctx, s.db, sql, now.Time)

	return resp, errors.Wrapf(err, "failed to fetch scheduled announcements for worker:%v", workerNumber)
}

//nolint:exhaustive // We know what cases need to be handled only.
func (s *Scheduler) getDeeplink(nt NotificationType, data *users.JSON) string {
	switch nt {
	case MiningExtendNotificationType:
		return fmt.Sprintf("%v://home", s.cfg.DeeplinkScheme)
	case MiningEndingSoonNotificationType:
		return fmt.Sprintf("%v://home", s.cfg.DeeplinkScheme)
	case MiningExpiredNotificationType:
		return fmt.Sprintf("%v://home", s.cfg.DeeplinkScheme)
	case MiningNotActiveNotificationType:
		return fmt.Sprintf("%v://home", s.cfg.DeeplinkScheme)
	case InviteFriendNotificationType:
		return fmt.Sprintf("%v://invite", s.cfg.DeeplinkScheme)
	case SocialsNotificationType:
		return fmt.Sprintf("%v://browser?url=%v", s.cfg.DeeplinkScheme, (*data)["SocialUrl"])
	case WeeklyStatsNotificationType:
		return fmt.Sprintf("%v://stats", s.cfg.DeeplinkScheme)
	default:
		log.Panic(fmt.Sprintf("wrong notification type:%v", nt))
	}

	return ""
}

func getData(jsonMap *users.JSON) map[string]string {
	if jsonMap == nil {
		return nil
	}
	res := make(map[string]string, len(*jsonMap))
	for key, val := range *jsonMap {
		res[key] = fmt.Sprintf("%v", val)
	}

	return res
}

//nolint:exhaustive // We know what cases need to be handled only.
func getDomainByNotificationType(nt NotificationType) NotificationDomain {
	switch nt {
	case MiningExtendNotificationType:
		return MiningNotificationDomain
	case MiningEndingSoonNotificationType:
		return MiningNotificationDomain
	case MiningExpiredNotificationType:
		return MiningNotificationDomain
	case MiningNotActiveNotificationType:
		return MiningNotificationDomain
	case InviteFriendNotificationType:
		return MicroCommunityNotificationDomain
	case SocialsNotificationType:
		return PromotionsNotificationDomain
	case WeeklyStatsNotificationType:
		return WeeklyStatsNotificationDomain
	default:
		log.Panic(fmt.Sprintf("wrong notification type:%v", nt))
	}

	return ""
}

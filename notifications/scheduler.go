// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"net/url"
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
)

func MustStartScheduler(ctx context.Context) *Scheduler {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)
	ddlWorkersParam := fmt.Sprintf(ddl, schedulerWorkersCount)
	sh := Scheduler{
		cfg:                      &cfg,
		db:                       storage.MustConnect(context.Background(), ddlWorkersParam, applicationYamlKey), //nolint:contextcheck // .
		pictureClient:            picture.New(applicationYamlKey),
		pushNotificationsClient:  push.New(applicationYamlKey),
		telemetryNotifications:   new(telemetry).mustInit([]string{"scheduler notifications[full iteration]", "get_scheduled_notifications", "process_notifications", "clear_invalid_tokens", "delete_scheduled_notifications"}), //nolint:lll // .
		telemetryAnnouncements:   new(telemetry).mustInit([]string{"scheduler announcements[full iteration]", "get_scheduled_announcements", "process_announcements", "delete_scheduled_announcements"}),                         //nolint:lll // .
		schedulerAnnouncementsMX: &sync.Mutex{},
		schedulerNotificationsMX: &sync.Mutex{},
	}
	loadPushNotificationTranslationTemplates()
	go sh.startWeeklyStatsUpdater(ctx)
	wg := new(sync.WaitGroup)
	wg.Add(2 * int(schedulerWorkersCount)) //nolint:gomnd,mnd // .
	defer wg.Wait()
	for workerNumber := range schedulerWorkersCount {
		go func(wn int64) {
			defer wg.Done()
			sh.runNotificationsProcessor(ctx, wn)
		}(workerNumber)
		go func(wn int64) {
			defer wg.Done()
			sh.runAnnouncementsProcessor(ctx, wn)
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
	return errors.Wrap(multierror.Append(
		errors.Wrap(s.db.Close(), "failed to close db"),
	).ErrorOrNil(), "can't close the scheduler")
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

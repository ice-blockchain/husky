// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"math/rand"
	"sync"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	appCfg "github.com/ice-blockchain/wintr/config"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/email"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/multimedia/picture"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:gochecknoinits // We load embedded stuff at runtime.
func init() {
	loadPushNotificationTranslationTemplates()
}

func New(ctx context.Context, cancel context.CancelFunc) Repository {
	var cfg config
	appCfg.MustLoadFromKey(applicationYamlKey, &cfg)

	db := storage.MustConnect(ctx, ddlV2, applicationYamlKey)

	return &repository{
		cfg:           &cfg,
		shutdown:      db.Close,
		db:            db,
		pictureClient: picture.New(applicationYamlKey),
	}
}

func StartProcessor(ctx context.Context, cancel context.CancelFunc) Processor { //nolint:funlen // A lot of startup & shutdown ceremony.
	var cfg config
	appCfg.MustLoadFromKey(applicationYamlKey, &cfg)

	var mbConsumer messagebroker.Client
	prc := &processor{repository: &repository{
		cfg:                     &cfg,
		db:                      storage.MustConnect(ctx, ddlV2, applicationYamlKey),
		mb:                      messagebroker.MustConnect(ctx, applicationYamlKey),
		pushNotificationsClient: push.New(applicationYamlKey),
		pictureClient:           picture.New(applicationYamlKey),
		emailClient:             email.New(applicationYamlKey),
		personalInAppFeed:       inapp.New(applicationYamlKey, "notifications"),
		globalInAppFeed:         inapp.New(applicationYamlKey, "announcements"),
	}}
	//nolint:contextcheck // It's intended. Cuz we want to close everything gracefully.
	mbConsumer = messagebroker.MustConnectAndStartConsuming(context.Background(), cancel, applicationYamlKey,
		&userTableSource{processor: prc},
		&deviceMetadataTableSource{processor: prc},
		&adoptionTableSource{processor: prc},
		&newsTableSource{processor: prc},
		&availableDailyBonusSource{processor: prc},
		&userPingSource{processor: prc},
		&startedDaysOffSource{processor: prc},
		&achievedBadgesSource{processor: prc},
		&completedLevelsSource{processor: prc},
		&enabledRolesSource{processor: prc},
	)
	prc.shutdown = closeAll(mbConsumer, prc.mb, prc.db, prc.pushNotificationsClient.Close)
	go prc.startOldSentNotificationsCleaner(ctx)
	go prc.startOldSentAnnouncementsCleaner(ctx)

	return prc
}

func (r *repository) Close() error {
	return errors.Wrap(r.shutdown(), "closing repository failed")
}

func closeAll(mbConsumer, mbProducer messagebroker.Client, db *storage.DB, otherClosers ...func() error) func() error {
	return func() error {
		err1 := errors.Wrap(mbConsumer.Close(), "closing message broker consumer connection failed")
		err2 := errors.Wrap(db.Close(), "closing db connection failed")
		err3 := errors.Wrap(mbProducer.Close(), "closing message broker producer connection failed")
		errs := make([]error, 0, 1+1+1+len(otherClosers))
		errs = append(errs, err1, err2, err3)
		for _, closeOther := range otherClosers {
			if err := closeOther(); err != nil {
				errs = append(errs, err)
			}
		}

		return errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "failed to close resources")
	}
}

func (p *processor) CheckHealth(ctx context.Context) error {
	if err := p.db.Ping(ctx); err != nil {
		return errors.Wrap(err, "[health-check] failed to ping DB")
	}
	type ts struct {
		TS *time.Time `json:"ts"`
	}
	now := ts{TS: time.Now()}
	bytes, err := json.MarshalContext(ctx, now)
	if err != nil {
		return errors.Wrapf(err, "[health-check] failed to marshal %#v", now)
	}
	responder := make(chan error, 1)
	p.mb.SendMessage(ctx, &messagebroker.Message{
		Headers: map[string]string{"producer": "husky"},
		Key:     p.cfg.MessageBroker.Topics[0].Name,
		Topic:   p.cfg.MessageBroker.Topics[0].Name,
		Value:   bytes,
	}, responder)

	return errors.Wrapf(<-responder, "[health-check] failed to send health check message to broker")
}

func (p *processor) startOldSentNotificationsCleaner(ctx context.Context) {
	ticker := stdlibtime.NewTicker(stdlibtime.Duration(1+rand.Intn(24)) * stdlibtime.Minute) //nolint:gosec,gomnd // Not an  issue.
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			const deadline = 30 * stdlibtime.Second
			reqCtx, cancel := context.WithTimeout(ctx, deadline)
			log.Error(errors.Wrap(p.deleteOldSentNotifications(reqCtx), "failed to deleteOldSentNotifications"))
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (p *processor) startOldSentAnnouncementsCleaner(ctx context.Context) {
	ticker := stdlibtime.NewTicker(stdlibtime.Duration(1+rand.Intn(24)) * stdlibtime.Minute) //nolint:gosec,gomnd // Not an  issue.
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			const deadline = 30 * stdlibtime.Second
			reqCtx, cancel := context.WithTimeout(ctx, deadline)
			log.Error(errors.Wrap(p.deleteOldSentAnnouncements(reqCtx), "failed to deleteOldSentAnnouncements"))
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (p *processor) deleteOldSentNotifications(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `DELETE FROM sent_notifications WHERE sent_at < $1`
	if _, err := storage.Exec(ctx, p.db, sql, stdlibtime.Now().Add(-24*stdlibtime.Hour)); err != nil {
		return errors.Wrap(err, "failed to delete old data from sent_notifications")
	}

	return nil
}

func (p *processor) deleteOldSentAnnouncements(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `DELETE FROM sent_announcements WHERE sent_at < $1`
	if _, err := storage.Exec(ctx, p.db, sql, stdlibtime.Now().Add(-24*stdlibtime.Hour)); err != nil {
		return errors.Wrap(err, "failed to delete old data from sent_announcements")
	}

	return nil
}

func requestingUserID(ctx context.Context) (requestingUserID string) {
	requestingUserID, _ = ctx.Value(requestingUserIDCtxValueKey).(string) //nolint:errcheck // Not needed.

	return
}

func runConcurrently[ARG any](ctx context.Context, run func(context.Context, ARG) error, args []ARG) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if len(args) == 0 {
		return nil
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(args))
	errChan := make(chan error, len(args))
	for i := range args {
		go func(ix int) {
			defer wg.Done()
			errChan <- errors.Wrapf(run(ctx, args[ix]), "failed to run:%#v", args[ix])
		}(i)
	}
	wg.Wait()
	close(errChan)
	errs := make([]error, 0, len(args))
	for err := range errChan {
		errs = append(errs, err)
	}

	return errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "at least one execution failed")
}

func executeConcurrently(fs ...func() error) error {
	if len(fs) == 0 {
		return nil
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(fs))
	errChan := make(chan error, len(fs))
	for i := range fs {
		go func(ix int) {
			defer wg.Done()
			errChan <- errors.Wrapf(fs[ix](), "failed to run func with index [%v]", ix)
		}(i)
	}
	wg.Wait()
	close(errChan)
	errs := make([]error, 0, len(fs))
	for err := range errChan {
		errs = append(errs, err)
	}

	return errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "at least one execution failed")
}

func (r *repository) insertSentNotification(ctx context.Context, conn storage.Execer, sn *sentNotification) error {
	sql := `INSERT INTO sent_notifications (
                                SENT_AT,
                                language,
                                USER_ID,
                                UNIQUENESS,
                                NOTIFICATION_TYPE,
                                NOTIFICATION_CHANNEL,
                                NOTIFICATION_CHANNEL_VALUE
        	) VALUES ($1,$2, $3,$4,$5,$6,$7);`

	if _, err := storage.Exec(ctx, conn, sql,
		sn.SentAt.Time,
		sn.Language,
		sn.UserID,
		sn.Uniqueness,
		sn.NotificationType,
		sn.NotificationChannel,
		sn.NotificationChannelValue,
	); err != nil {
		return errors.Wrapf(err, "failed to insert sent notification %#v", sn)
	}
	return nil
}

func (r *repository) insertSentAnnouncement(ctx context.Context, conn storage.Execer, sa *sentAnnouncement) error {
	sql := `INSERT INTO sent_announcements (SENT_AT, LANGUAGE, UNIQUENESS, NOTIFICATION_TYPE, NOTIFICATION_CHANNEL, NOTIFICATION_CHANNEL_VALUE)
        	) VALUES ($1,$2, $3,$4,$5,$6);`

	if _, err := storage.Exec(ctx, conn, sql,
		sa.SentAt.Time,
		sa.Language,
		sa.Uniqueness,
		sa.NotificationType,
		sa.NotificationChannel,
		sa.NotificationChannelValue,
	); err != nil {
		return errors.Wrapf(err, "failed to insert sent announcement %#v", sa)
	}
	return nil
}

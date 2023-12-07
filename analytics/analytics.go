// SPDX-License-Identifier: ice License 1.0

package analytics

import (
	"context"
	"math/rand"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/analytics/tracking"
	appcfg "github.com/ice-blockchain/wintr/config"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func StartProcessor(ctx context.Context, cancel context.CancelFunc) Processor {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)

	var (
		db         *storage.DB
		mbConsumer messagebroker.Client
		prc        = &processor{}
	)
	db = storage.MustConnect(ctx, ddl, applicationYamlKey)
	prc.repository = &repository{
		cfg:            &cfg,
		db:             db,
		trackingClient: tracking.New(applicationYamlKey),
	}
	//nolint:contextcheck // It's intended. Cuz we want to close everything gracefully.
	mbConsumer = messagebroker.MustConnectAndStartConsuming(context.Background(), cancel, applicationYamlKey,
		&setUserAttributesSource{processor: prc},
		&trackActionSource{processor: prc},
	)
	prc.shutdown = closeAll(mbConsumer, db)
	go prc.startOldTrackedActionsCleaner(ctx)

	return prc
}

func (r *repository) Close() error {
	return errors.Wrap(r.shutdown(), "closing repository failed")
}

func closeAll(mbConsumer messagebroker.Client, db *storage.DB, otherClosers ...func() error) func() error {
	return func() error {
		err1 := errors.Wrap(mbConsumer.Close(), "closing message broker consumer connection failed")
		err2 := errors.Wrap(db.Close(), "closing db connection failed")
		errs := make([]error, 0, 1+1+len(otherClosers))
		errs = append(errs, err1, err2)
		for _, closeOther := range otherClosers {
			if err := closeOther(); err != nil {
				errs = append(errs, err)
			}
		}

		return errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "failed to close resources")
	}
}

func (p *processor) CheckHealth(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if err := p.db.Ping(ctx); err != nil {
		return errors.Wrap(err, "[health-check] failed to ping DB")
	}

	return nil
}

func (s *setUserAttributesSource) Process(ctx context.Context, msg *messagebroker.Message) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	cmd := new(SetUserAttributesCommand)
	if err := json.UnmarshalContext(ctx, msg.Value, cmd); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), cmd)
	}
	if cmd.UserID == "" || len(cmd.Attributes) == 0 {
		return nil
	}

	const deadline = 62 * stdlibtime.Second
	trackingCtx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	for trackingCtx.Err() == nil {
		//nolint:contextcheck // We need more time.
		err := errors.Wrapf(s.trackingClient.SetUserAttributes(trackingCtx, cmd.UserID, cmd.Attributes), "failed to SetUserAttributes for %#v", cmd)
		log.Error(err)
		if err == nil {
			return nil
		}
	}

	return errors.Wrapf(trackingCtx.Err(), "deadline expired and couldn't set attributes %#v", cmd)
}

func (s *trackActionSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	cmd := new(TrackActionCommand)
	if err := json.UnmarshalContext(ctx, msg.Value, cmd); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), cmd)
	}
	if cmd.UserID == "" || cmd.Action == nil {
		return nil
	}
	tuple := &trackedAction{SentAt: time.New(msg.Timestamp), ID: cmd.ID}
	sql := `INSERT INTO TRACKED_ACTIONS (SENT_AT, ACTION_ID) VALUES ($1, $2)`
	if _, err := storage.Exec(ctx, s.db, sql, tuple.SentAt.Time, tuple.ID); err != nil {
		return errors.Wrapf(err, "failed to insert TRACKED_ACTIONS %#v", tuple)
	}
	const deadline = 62 * stdlibtime.Second
	trackingCtx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	for trackingCtx.Err() == nil {
		//nolint:contextcheck // We need more time.
		err := errors.Wrapf(s.trackingClient.TrackAction(trackingCtx, cmd.UserID, cmd.Action), "failed to TrackAction for %#v", cmd)
		log.Error(err)
		if err == nil {
			return nil
		}
	}

	return errors.Wrapf(trackingCtx.Err(), "deadline expired and couldn't track action %#v", cmd)
}

func (p *processor) startOldTrackedActionsCleaner(ctx context.Context) {
	ticker := stdlibtime.NewTicker(stdlibtime.Duration(1+rand.Intn(24)) * stdlibtime.Minute) //nolint:gosec,gomnd // Not an  issue.
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			const deadline = 30 * stdlibtime.Second
			reqCtx, cancel := context.WithTimeout(ctx, deadline)
			log.Error(errors.Wrap(p.deleteOldTrackedActions(reqCtx), "failed to deleteOldTrackedActions"))
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (p *processor) deleteOldTrackedActions(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `DELETE FROM tracked_actions WHERE sent_at < $1`
	if _, err := storage.Exec(ctx, p.db, sql, time.New(time.Now().Add(-24*stdlibtime.Hour)).Time); err != nil {
		return errors.Wrap(err, "failed to delete old data from tracked_actions")
	}

	return nil
}

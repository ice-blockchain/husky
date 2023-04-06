// SPDX-License-Identifier: ice License 1.0

package analytics

import (
	"context"
	storagev2 "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"math/rand"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/go-tarantool-client"
	"github.com/ice-blockchain/wintr/analytics/tracking"
	appCfg "github.com/ice-blockchain/wintr/config"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func StartProcessor(ctx context.Context, cancel context.CancelFunc) Processor {
	var cfg config
	appCfg.MustLoadFromKey(applicationYamlKey, &cfg)

	var (
		db         tarantool.Connector
		mbConsumer messagebroker.Client
		prc        = &processor{}
	)
	db = storage.MustConnect(context.Background(), func() { //nolint:contextcheck // It's intended. Cuz we want to close everything gracefully.
		if mbConsumer != nil {
			log.Error(errors.Wrap(mbConsumer.Close(), "failed to close mbConsumer due to db premature cancellation"))
		}
		cancel()
	}, ddl, applicationYamlKey)
	dbV2 := storagev2.MustConnect(ctx, ddlV2, applicationYamlKey)
	prc.repository = &repository{
		cfg:            &cfg,
		db:             db,
		dbV2:           dbV2,
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

func closeAll(mbConsumer messagebroker.Client, db tarantool.Connector, otherClosers ...func() error) func() error {
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
	if _, err := p.db.Ping(); err != nil {
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

func (s *trackActionSource) Process(ctx context.Context, msg *messagebroker.Message) error {
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
	if err := storage.CheckNoSQLDMLErr(s.db.InsertTyped("TRACKED_ACTIONS", tuple, &[]*trackedAction{})); err != nil {
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
	sql := `DELETE FROM tracked_actions WHERE sent_at < :reference_date`
	params := make(map[string]any, 1)
	params["reference_date"] = time.New(time.Now().Add(-24 * stdlibtime.Hour))
	if _, err := storage.CheckSQLDMLResponse(p.db.PrepareExecute(sql, params)); err != nil {
		return errors.Wrap(err, "failed to delete old data from tracked_actions")
	}

	return nil
}

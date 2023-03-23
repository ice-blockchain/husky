// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"fmt"
	"mime/multipart"
	"strconv"
	"strings"
	"sync"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/go-tarantool-client"
	appCfg "github.com/ice-blockchain/wintr/config"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/multimedia/picture"
	"github.com/ice-blockchain/wintr/time"
)

func New(ctx context.Context, cancel context.CancelFunc) Repository {
	var cfg config
	appCfg.MustLoadFromKey(applicationYamlKey, &cfg)

	db := storage.MustConnect(ctx, cancel, ddl, applicationYamlKey)

	return &repository{
		cfg:           &cfg,
		shutdown:      db.Close,
		db:            db,
		pictureClient: picture.New(applicationYamlKey),
	}
}

func StartProcessor(ctx context.Context, cancel context.CancelFunc) Processor {
	var cfg config
	appCfg.MustLoadFromKey(applicationYamlKey, &cfg)

	db := storage.MustConnect(context.Background(), cancel, ddl, applicationYamlKey) //nolint:contextcheck // We need to gracefully shut it down.
	mbProducer := messagebroker.MustConnect(ctx, applicationYamlKey)

	return &processor{repository: &repository{
		cfg:           &cfg,
		shutdown:      closeAll(mbProducer, db),
		db:            db,
		mb:            mbProducer,
		pictureClient: picture.New(applicationYamlKey),
	}}
}

func (r *repository) Close() error {
	return errors.Wrap(r.shutdown(), "closing repository failed")
}

func closeAll(mbProducer messagebroker.Client, db tarantool.Connector, otherClosers ...func() error) func() error {
	return func() error {
		err1 := errors.Wrap(db.Close(), "closing db connection failed")
		err2 := errors.Wrap(mbProducer.Close(), "closing message broker producer connection failed")
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
	if _, err := p.db.Ping(); err != nil {
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

func requestingUserID(ctx context.Context) (requestingUserID string) {
	requestingUserID, _ = ctx.Value(requestingUserIDCtxValueKey).(string) //nolint:errcheck // Not needed.

	return
}

func lastUpdatedAt(ctx context.Context) *time.Time {
	checksum, ok := ctx.Value(checksumCtxValueKey).(string)
	if !ok || checksum == "" {
		return nil
	}

	nanos, err := strconv.Atoi(checksum)
	if err != nil {
		log.Error(errors.Wrapf(err, "checksum %v is not numeric", checksum))

		return nil
	}

	return time.New(stdlibtime.Unix(0, int64(nanos)))
}

func ContextWithChecksum(ctx context.Context, checksum string) context.Context {
	if checksum == "" {
		return ctx
	}

	return context.WithValue(ctx, checksumCtxValueKey, checksum) //nolint:revive,staticcheck //.
}

func (n *TaggedNews) Checksum() string {
	if n.UpdatedAt == nil {
		return ""
	}

	return fmt.Sprint(n.UpdatedAt.UnixNano())
}

func mergeStringField(oldData, newData string) string {
	if newData != "" {
		return newData
	}

	return oldData
}

func sendMessagesConcurrently[M any](ctx context.Context, sendMessage func(context.Context, *M) error, messages []*M) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if len(messages) == 0 {
		return nil
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(messages))
	errChan := make(chan error, len(messages))
	for i := range messages {
		go func(ix int) {
			defer wg.Done()
			errChan <- errors.Wrapf(sendMessage(ctx, messages[ix]), "failed to sendMessage:%#v", messages[ix])
		}(i)
	}
	wg.Wait()
	close(errChan)
	errs := make([]error, 0, len(messages))
	for err := range errChan {
		errs = append(errs, err)
	}

	return errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "at least one message sends failed")
}

func (r *repository) validateAndUploadImage(ctx context.Context, image *multipart.FileHeader, newsID string, now *time.Time) error {
	if image == nil || ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	var imageExtension string
	if lastDotIdx := strings.LastIndex(image.Filename, "."); lastDotIdx > 0 {
		imageExtension = strings.ToLower(image.Filename[lastDotIdx:])
	}
	if imageExtension != ".png" && imageExtension != ".jpg" {
		return errors.Wrapf(ErrInvalidImageExtension, "extension `%v` is invalid. Allowed are .png or .jpg", imageExtension)
	}
	image.Filename = fmt.Sprintf("%v_%v%v", newsID, now.UnixNano(), imageExtension)

	return errors.Wrapf(r.pictureClient.UploadPicture(ctx, image, ""), "can't upload the image for newsID:%v", newsID)
}

func (r *repository) sendTaggedNewsSnapshotMessage(ctx context.Context, ss *TaggedNewsSnapshot) error {
	valueBytes, err := json.MarshalContext(ctx, ss)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %#v", ss)
	}
	var key string
	if ss.TaggedNews == nil {
		key = ss.Before.ID + "~~~" + ss.Before.Language
	} else {
		key = ss.ID + "~~~" + ss.Language
	}
	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "husky"},
		Key:     key,
		Topic:   r.cfg.MessageBroker.Topics[1].Name,
		Value:   valueBytes,
	}
	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send news snapshot message to broker")
}

// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) CreateNews(ctx context.Context, news []*TaggedNews, image *multipart.FileHeader) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	id, now := uuid.NewString(), time.Now()
	if err := r.validateAndUploadImage(ctx, image, id, now); err != nil {
		return errors.Wrapf(err, "failed to validateAndUploadImage for news:%#v", news)
	}
	snapshots := make([]*TaggedNewsSnapshot, 0, len(news))
	for _, nws := range news {
		nws.ID, nws.CreatedAt, nws.UpdatedAt, nws.ImageURL = id, now, now, image.Filename
		snapshots = append(snapshots, &TaggedNewsSnapshot{TaggedNews: nws})
	}
	if err := r.insertNews(ctx, news); err != nil {
		return errors.Wrapf(err, "failed to call insertNews for:%#v", news)
	}
	if err := r.addNewsTags(ctx, news...); err != nil {
		return errors.Wrapf(err, "failed to add news tags for:%#v", news)
	}
	if err := r.addNewsTagsPerNews(ctx, news...); err != nil {
		return errors.Wrapf(err, "failed to call addNewsTagsPerNews for:%#v", news)
	}
	for _, nws := range news {
		nws.ImageURL = r.pictureClient.DownloadURL(image.Filename)
	}

	return errors.Wrapf(sendMessagesConcurrently(ctx, r.sendTaggedNewsSnapshotMessage, snapshots), "failed to sendTaggedNewsSnapshotMessages:%#v", snapshots)
}

func (r *repository) insertNews(ctx context.Context, news []*TaggedNews) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	const fields = 8
	params := make(map[string]any, len(news)*fields)
	values := make([]string, 0, len(news))
	for ix, nws := range news {
		params[fmt.Sprintf(`created_at%v`, ix)] = nws.CreatedAt
		params[fmt.Sprintf(`notification_channels%v`, ix)] = nws.NotificationChannels.NotificationChannels
		params[fmt.Sprintf(`id%v`, ix)] = nws.ID
		params[fmt.Sprintf(`type%v`, ix)] = nws.Type
		params[fmt.Sprintf(`language%v`, ix)] = nws.Language
		params[fmt.Sprintf(`title%v`, ix)] = nws.Title
		params[fmt.Sprintf(`image_url%v`, ix)] = nws.ImageURL
		params[fmt.Sprintf(`url%v`, ix)] = nws.URL
		values = append(values, fmt.Sprintf(`(:created_at%[1]v, :created_at%[1]v, :notification_channels%[1]v, :id%[1]v, :type%[1]v, :language%[1]v, :title%[1]v, :image_url%[1]v, :url%[1]v)`, ix)) //nolint:lll // .
	}
	sql := fmt.Sprintf(`INSERT INTO news (CREATED_AT, UPDATED_AT, NOTIFICATION_CHANNELS, ID, TYPE, LANGUAGE, TITLE, IMAGE_URL, URL) VALUES %v`, strings.Join(values, ",")) //nolint:lll // .
	if err := storage.CheckSQLDMLErr(r.db.PrepareExecute(sql, params)); err != nil {
		return errors.Wrapf(detectAndParseDuplicateDatabaseError(err), "failed to insert news %#v", news)
	}

	return nil
}

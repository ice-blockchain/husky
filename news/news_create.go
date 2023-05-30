// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
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
	const fields = 9
	args := make([]any, 0, len(news)*fields)
	values := make([]string, 0, len(news))
	for ix, nws := range news {
		args = append(args, nws.CreatedAt.Time, nws.UpdatedAt.Time, nws.NotificationChannels.NotificationChannels, nws.ID, nws.Type, nws.Language,
			nws.Title, nws.ImageURL, nws.URL,
		)
		values = append(values, fmt.Sprintf("($%[1]v,$%[2]v,$%[3]v,$%[4]v,$%[5]v,$%[6]v,$%[7]v,$%[8]v,$%[9]v)",
			fields*ix+1, fields*ix+2, fields*ix+3, fields*ix+4, fields*ix+5, fields*ix+6, fields*ix+7, fields*ix+8, fields*ix+9)) //nolint:gomnd // .
	}
	sql := fmt.Sprintf(`INSERT INTO news (CREATED_AT, UPDATED_AT, NOTIFICATION_CHANNELS, ID, TYPE, LANGUAGE, TITLE, IMAGE_URL, URL) VALUES %v`, strings.Join(values, ",")) //nolint:lll // .
	if _, err := storage.Exec(ctx, r.db, sql, args...); err != nil {
		return errors.Wrapf(detectAndParseDuplicateDatabaseError(err), "failed to insert news %#v", news)
	}

	return nil
}

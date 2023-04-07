// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"

	"github.com/pkg/errors"

	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func (r *repository) DeleteNews(ctx context.Context, newsID, language string) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	gNews, err := r.getNewsByPK(ctx, newsID, language)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			err = ErrNotFound
		}

		return errors.Wrapf(err, "failed to get news for pk(newID:%v,language:%v)", newsID, language)
	}
	sql := `DELETE FROM news WHERE language = $1 AND id = $2`
	if _, err := storage.Exec(ctx, r.db, sql, language, newsID); err != nil {
		return errors.Wrapf(err, "failed to delete news by (newsID:%v,language:%v)", newsID, language)
	}
	gNews.ImageURL = r.pictureClient.DownloadURL(gNews.ImageURL)
	ss := &TaggedNewsSnapshot{Before: gNews}

	return errors.Wrapf(r.sendTaggedNewsSnapshotMessage(ctx, ss), "failed to send deleted news message: %#v", ss)
}

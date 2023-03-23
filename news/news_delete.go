// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage"
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
	sql := `DELETE FROM news WHERE language = :language AND id = :id`
	args := make(map[string]any, 1+1)
	args["id"] = newsID
	args["language"] = language
	if err = storage.CheckSQLDMLErr(r.db.PrepareExecute(sql, args)); err != nil {
		return errors.Wrapf(err, "failed to delete news by (newsID:%v,language:%v)", newsID, language)
	}
	gNews.ImageURL = r.pictureClient.DownloadURL(gNews.ImageURL)
	ss := &TaggedNewsSnapshot{Before: gNews}

	return errors.Wrapf(r.sendTaggedNewsSnapshotMessage(ctx, ss), "failed to send deleted news message: %#v", ss)
}

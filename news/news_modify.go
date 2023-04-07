// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"fmt"
	"log"
	"mime/multipart"

	"github.com/goccy/go-json"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/terror"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen,gocognit // Better to be grouped together.
func (r *repository) ModifyNews(ctx context.Context, news *TaggedNews, image *multipart.FileHeader) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	oldNews, err := r.getNewsByPK(ctx, news.ID, news.Language)
	if err != nil {
		return errors.Wrapf(err, "get news by pk: (%v,%v) failed", news.ID, news.Language)
	}
	if lu := lastUpdatedAt(ctx); lu != nil && oldNews.UpdatedAt.UnixNano() != lu.UnixNano() {
		return ErrRaceCondition
	}
	news.UpdatedAt = time.Now()
	if err = r.validateAndUploadImage(ctx, image, news.ID, news.UpdatedAt); err != nil {
		return errors.Wrapf(err, "failed to validateAndUploadImage for news:%#v", news)
	}
	if image != nil {
		news.ImageURL = image.Filename
	}
	if err = r.updateNews(ctx, news); err != nil {
		return errors.Wrapf(err, "failed to updateNews for news:%#v", news)
	}
	*news = *oldNews.override(news)
	if err = r.addNewsTags(ctx, news); err != nil {
		return errors.Wrapf(err, "failed to add news tags for:%#v", news)
	}
	if news.Tags != nil && len(*news.Tags) != 0 {
		if err = r.removeAllNewsTagsPerNews(ctx, news); err != nil {
			return errors.Wrapf(err, "failed to call removeAllNewsTagsPerNews for:%#v", news)
		}
	}
	if err = r.addNewsTagsPerNews(ctx, news); err != nil {
		return errors.Wrapf(err, "failed to call addNewsTagsPerNews for:%#v", news)
	}
	news.ImageURL = r.pictureClient.DownloadURL(news.ImageURL)
	if oldNews != nil {
		oldNews.ImageURL = r.pictureClient.DownloadURL(oldNews.ImageURL)
	}
	message := &TaggedNewsSnapshot{TaggedNews: news, Before: oldNews}

	return errors.Wrapf(r.sendTaggedNewsSnapshotMessage(ctx, message), "failed to sendTaggedNewsSnapshotMessage:%#v", message)
}

func (r *repository) updateNews(ctx context.Context, news *TaggedNews) error { //nolint:funlen // Better to be grouped together.
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	var args []any
	fieldIndex := 1
	args = append(args, news.UpdatedAt.Time)
	sql := fmt.Sprintf("UPDATE NEWS set UPDATED_AT = $%v", fieldIndex)
	fieldIndex++
	if news.Type != "" {
		args = append(args, news.Type)
		sql += fmt.Sprintf(", TYPE = $%v", fieldIndex)
		fieldIndex++
	}
	if news.Title != "" {
		sql += fmt.Sprintf(", TITLE = $%v", fieldIndex)
		args = append(args, news.Title)
		fieldIndex++
	}
	if news.ImageURL != "" {
		args = append(args, news.ImageURL)
		sql += fmt.Sprintf(", IMAGE_URL = $%v", fieldIndex)
		fieldIndex++
	}
	if news.URL != "" {
		args = append(args, news.URL)
		sql += fmt.Sprintf(", URL = $%v", fieldIndex)
		fieldIndex++
	}
	args = append(args, news.ID, news.Language)
	sql += fmt.Sprintf(" WHERE ID = $%v AND LANGUAGE = $%v", fieldIndex, fieldIndex+1)
	fieldIndex += 2
	if lu := lastUpdatedAt(ctx); lu != nil {
		args = append(args, lu.Time)
		sql += fmt.Sprintf(" AND UPDATED_AT = $%v", fieldIndex)
	}
	if _, err := storage.Exec(ctx, r.db, sql, args...); err != nil {
		if err := detectAndParseDuplicateDatabaseError(err); storage.IsErr(err, storage.ErrNotFound) {
			err = ErrRaceCondition
		}

		return errors.Wrapf(err, "failed to update news:%#v", news)
	}

	return nil
}

func (n *TaggedNews) override(news *TaggedNews) *TaggedNews {
	nws := new(TaggedNews)
	*nws = *n

	nws.UpdatedAt = news.UpdatedAt
	nws.Type = mergeStringField(n.Type, news.Type)
	nws.Title = mergeStringField(n.Title, news.Title)
	nws.ImageURL = mergeStringField(n.ImageURL, news.ImageURL)
	nws.URL = mergeStringField(n.URL, news.URL)
	if news.Tags != nil && len(*news.Tags) != 0 {
		nws.Tags = news.Tags
	}
	if news.Views > 0 {
		nws.Views = news.Views
	}

	return nws
}

func (r *repository) IncrementViews(ctx context.Context, newsID, language string) error { //nolint:funlen // A lot of negative flow handling.
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), " context failed")
	}
	tuple := &ViewedNews{
		CreatedAt: time.Now(),
		NewsID:    newsID,
		Language:  language,
		UserID:    requestingUserID(ctx),
	}

	return errors.Wrapf(storage.DoInTransaction(ctx, r.db, func(conn storage.QueryExecer) error {
		args := []any{tuple.CreatedAt.Time, tuple.NewsID, tuple.Language, tuple.UserID}
		sql := `INSERT INTO NEWS_VIEWED_BY_USERS (created_at, news_id, language, user_id) VALUES($1, $2, $3, $4)`
		if _, err := storage.Exec(ctx, r.db, sql, args...); err != nil {
			return errors.Wrapf(err, "failed to insert NEWS_VIEWED_BY_USERS %#v", tuple)
		}

		sql = `UPDATE news SET views = views + 1 WHERE language = $1 AND id = $2`
		if _, err := storage.Exec(ctx, r.db, sql, language, newsID); err != nil { //nolint:lll,revive // .
			if storage.IsErr(err, storage.ErrNotFound) {
				err = ErrNotFound
			}

			return errors.Wrapf(err, "failed to increment news views count for newsId:%v,language:%v", newsID, language)
		}

		return errors.Wrapf(r.sendNewsViewedMessage(ctx, tuple), "failed to sendNewsViewedMessage for %#v", tuple)
	}), "can't execute increment views transaction")
}

func (r *repository) sendNewsViewedMessage(ctx context.Context, vn *ViewedNews) error {
	valueBytes, err := json.MarshalContext(ctx, vn)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %#v", vn)
	}
	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "husky"},
		Key:     vn.NewsID + "~~~" + vn.Language + "~~~" + vn.UserID,
		Topic:   r.cfg.MessageBroker.Topics[2].Name,
		Value:   valueBytes,
	}
	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send news viewed message to broker")
}

func detectAndParseDuplicateDatabaseError(err error) error {
	if errors.Is(err, storage.ErrDuplicate) {
		field := ""
		if storage.IsErr(err, storage.ErrDuplicate, "url") {
			field = "url"
		} else if storage.IsErr(err, storage.ErrDuplicate, "language") {
			field = "language"
		} else {
			log.Panic("unexpected duplicate for news space")
		}

		return terror.New(storage.ErrDuplicate, map[string]any{"field": field})
	}

	return err
}

// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"mime/multipart"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/go-tarantool-client"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/log"
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
	sql := "UPDATE NEWS set UPDATED_AT = :updatedAt"
	params := make(map[string]any)
	params["id"] = news.ID
	params["language"] = news.Language
	params["updatedAt"] = news.UpdatedAt
	if news.Type != "" {
		params["type"] = news.Type
		sql += ", TYPE = :type"
	}
	if news.Title != "" {
		params["title"] = news.Title
		sql += ", TITLE = :title"
	}
	if news.ImageURL != "" {
		params["image_url"] = news.ImageURL
		sql += ", IMAGE_URL = :image_url"
	}
	if news.URL != "" {
		params["url"] = news.URL
		sql += ", URL = :url"
	}
	sql += " WHERE ID = :id AND LANGUAGE = :language"
	if lu := lastUpdatedAt(ctx); lu != nil {
		params["lastUpdatedAt"] = lu
		sql += " AND UPDATED_AT = :lastUpdatedAt"
	}
	if err := storage.CheckSQLDMLErr(r.db.PrepareExecute(sql, params)); err != nil {
		if err = detectAndParseDuplicateDatabaseError(err); errors.Is(err, storage.ErrNotFound) {
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
	if err := storage.CheckNoSQLDMLErr(r.db.InsertTyped("NEWS_VIEWED_BY_USERS", tuple, &[]*ViewedNews{})); err != nil {
		return errors.Wrapf(err, "failed to insert NEWS_VIEWED_BY_USERS %#v", tuple)
	}
	ops := append(make([]tarantool.Op, 0, 1), tarantool.Op{Op: "+", Field: 9, Arg: uint64(1)}) //nolint:gomnd // That's the `views` column index.
	result := make([]*News, 0, 1)
	if err := storage.CheckNoSQLDMLErr(r.db.UpdateTyped("NEWS", "pk_unnamed_NEWS_2", []any{language, newsID}, ops, &result)); err != nil || len(result) == 0 || result[0].ID == "" { //nolint:lll,revive // .
		if err == nil {
			err = ErrNotFound
		}

		return errors.Wrapf(err, "failed to increment news views count for newsId:%v,language:%v", newsID, language)
	}
	if err := r.sendNewsViewedMessage(ctx, tuple); err != nil {
		bErr := errors.Wrapf(err, "failed to sendNewsViewedMessage for %#v", tuple)
		ops = append(make([]tarantool.Op, 0, 1), tarantool.Op{Op: "-", Field: 9, Arg: uint64(1)}) //nolint:gomnd // That's the `views` column index.
		if err = storage.CheckNoSQLDMLErr(r.db.UpdateTyped("NEWS", "pk_unnamed_NEWS_2", []any{language, newsID}, ops, &[]*News{})); err != nil {
			return multierror.Append(bErr, errors.Wrapf(err, "[rollback]failed to decrement news views count for newsId:%v,language:%v", newsID, language)).ErrorOrNil() //nolint:lll,wrapcheck // .
		}
		if err = storage.CheckNoSQLDMLErr(r.db.DeleteTyped("NEWS_VIEWED_BY_USERS", "pk_unnamed_NEWS_VIEWED_BY_USERS_1", []any{language, tuple.NewsID, tuple.UserID}, &[]*ViewedNews{})); err != nil { //nolint:lll // .
			return multierror.Append(bErr, errors.Wrapf(err, "[rollback]failed to delete NEWS_VIEWED_BY_USERS%#v", tuple)).ErrorOrNil() //nolint:wrapcheck // .
		}

		return bErr
	}

	return nil
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
	if tErr := terror.As(err); tErr != nil && errors.Is(err, storage.ErrDuplicate) {
		field := ""
		switch tErr.Data[storage.IndexName] {
		case "unique_unnamed_NEWS_1":
			field = "url"
		case "pk_unnamed_NEWS_2":
			field = "language"
		default:
			log.Panic("unexpected indexName `%v` for news space", tErr.Data[storage.IndexName])
		}

		return terror.New(storage.ErrDuplicate, map[string]any{"field": field})
	}

	return err
}

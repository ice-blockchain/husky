// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) addNewsTags(ctx context.Context, news ...*TaggedNews) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	const estimatedTags = 10
	errChan := make(chan error, len(news)*estimatedTags)
	wg := new(sync.WaitGroup)
	for _, nws := range news {
		if nws.Tags == nil {
			continue
		}
		for _, tag := range *nws.Tags {
			wg.Add(1)
			go func(tg Tag, nw *TaggedNews) {
				defer wg.Done()
				errChan <- errors.Wrapf(r.insertTag(ctx, nw, tg), "failed to insertTag[%v],news:%#v", tg, nw)
			}(tag, nws)
		}
	}
	wg.Wait()
	close(errChan)
	errs := make([]error, 0, cap(errChan))
	for err := range errChan {
		errs = append(errs, err)
	}

	return errors.Wrapf(multierror.Append(nil, errs...).ErrorOrNil(), "failed to insert atleast one tag from news:%#v", news)
}

func (r *repository) insertTag(ctx context.Context, nws *TaggedNews, tag Tag) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	type newsTag struct {
		CreatedAt *time.Time `json:"createdAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		Language  string     `json:"language" example:"en"`
		Value     string     `json:"value" example:"cats"`
	}
	tuple := &newsTag{
		CreatedAt: nws.CreatedAt,
		Language:  nws.Language,
		Value:     tag,
	}
	sql := `INSERT INTO news_tags (CREATED_AT, LANGUAGE, VALUE) VALUES ($1, $2, $3)`
	if _, err := storage.Exec(ctx, r.db, sql, nws.CreatedAt.Time, nws.Language, tag); err != nil {
		return errors.Wrapf(err, "failed to insert tag:%#v", tuple)
	}

	return nil
}

func (r *repository) addNewsTagsPerNews(ctx context.Context, news ...*TaggedNews) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	const fields, estimatedTags = 4, 10
	var args []any
	values := make([]string, 0, len(news)*estimatedTags)
	nextIdx := 0
	for _, nws := range news {
		if nws.Tags == nil {
			continue
		}
		for _, tag := range *nws.Tags {
			args = append(args, nws.CreatedAt.Time, nws.ID, nws.Language, tag)
			values = append(values, fmt.Sprintf(`($%[1]v, $%[2]v, $%[3]v, $%[4]v)`, nextIdx+1, nextIdx+2, nextIdx+3, nextIdx+4)) //nolint:gomnd // .
			nextIdx += fields
		}
	}
	if len(values) == 0 {
		return nil
	}
	sql := fmt.Sprintf(`INSERT INTO news_tags_per_news (CREATED_AT, NEWS_ID, LANGUAGE, NEWS_TAG) VALUES %v`, strings.Join(values, ","))
	if _, err := storage.Exec(ctx, r.db, sql, args...); err != nil {
		return errors.Wrapf(err, "failed to insert news_tags_per_news for news:%#v", news)
	}

	return nil
}

func (r *repository) removeAllNewsTagsPerNews(ctx context.Context, news *TaggedNews) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	sql := `DELETE FROM news_tags_per_news WHERE language = $1 AND news_id =  $2`
	args := []any{news.Language, news.ID}
	_, err := storage.Exec(ctx, r.db, sql, args...)

	return errors.Wrapf(err, "failed to delete from news_tags_per_news for args:%#v", args...)
}

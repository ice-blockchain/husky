// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:revive,funlen // The alternative worse and requires to create one more struct.
func (r *repository) GetNews(ctx context.Context, newsType Type, language string, limit, offset uint64, createdAfter *time.Time) ([]*PersonalNews, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "get news failed because context failed")
	}
	args := []any{requestingUserID(ctx), language, newsType, int64(limit), int64(offset)}
	sql := fmt.Sprintf(`SELECT (nvu.created_at IS NOT NULL OR nvu_en.created_at IS NOT NULL) AS viewed,
					    COALESCE(n.created_at,n_en.created_at) AS created_at,
						COALESCE(n.updated_at, n_en.updated_at) AS updated_at,
						COALESCE(v.views,v_en.views) as views,
						COALESCE(n.notification_channels, n_en.notification_channels) AS notification_channels,
						COALESCE(n.id,n_en.id) AS id,
						COALESCE(n.type, n_en.type) AS type,
						COALESCE(n.language, n_en.language) AS language,
						COALESCE(n.title, n_en.title) AS title,
						COALESCE(n.image_url, n_en.image_url) AS image_url,
						COALESCE(n.url, n_en.url) AS url
			FROM news n_en
				LEFT JOIN news n on n.id = n_en.id and n.language = $2
				LEFT JOIN news_viewed_by_users nvu 
					   ON nvu.language = n.language
					  AND nvu.news_id = n.id
					  AND nvu.user_id = $1
				LEFT JOIN news_viewed_by_users nvu_en 
					   ON nvu_en.language = n_en.language
					  AND nvu_en.news_id = n_en.id
					  AND nvu_en.user_id = $1
				LEFT JOIN news_views v ON v.id = n.id
				LEFT JOIN news_views v_en ON v_en.id = n_en.id
			WHERE n_en.language = '%[1]v'
				  AND n_en.type = $3
			ORDER BY 
				(CASE WHEN n.type = 'regular' OR n_en.type = 'regular'
					THEN (nvu.created_at IS NULL AND nvu_en.created_at IS NULL)
					ELSE FALSE
				END) DESC,
				COALESCE(n.created_at,n_en.created_at) DESC
			LIMIT $4 OFFSET $5`, fallbackLanguage)
	result, err := storage.Select[PersonalNews](ctx, r.db, sql, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get news for args:%#v", args...)
	}
	if result == nil {
		return []*PersonalNews{}, nil
	}
	trueVal := true
	for _, elem := range result {
		if elem.Viewed != nil && !*elem.Viewed && elem.CreatedAt.Before(*createdAfter.Time) {
			elem.Viewed = &trueVal
		}
		elem.NotificationChannels = nil
		elem.UpdatedAt = nil
		elem.ImageURL = r.pictureClient.DownloadURL(elem.ImageURL)
	}

	return result, nil
}

//nolint:funlen // SQL
func (r *repository) GetUnreadNewsCount(ctx context.Context, language string, createdAfter *time.Time) (*UnreadNewsCount, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	args := []any{requestingUserID(ctx), language, RegularNewsType, FeaturedNewsType, createdAfter.Time}
	sql := fmt.Sprintf(`
		WITH featured_count AS (
			 SELECT (CASE WHEN (nvu.created_at IS NULL AND nvu_en.created_at IS NULL) THEN 1 ELSE 0 END) AS count
				FROM news n_en
				  LEFT JOIN news n 
				 ON n.id = n_en.id 
				AND n.language = $2
				AND n.type = $4
				  LEFT JOIN news_viewed_by_users nvu
							 ON nvu.language = n.language
								AND nvu.news_id = n.id
								AND nvu.user_id = $1
				  LEFT JOIN news_viewed_by_users nvu_en
							 ON nvu_en.language = n_en.language
								AND nvu_en.news_id = n_en.id
								AND nvu_en.user_id = $1
				WHERE
				  n_en.language = '%[1]v'
				  AND n_en.type = $4
				  AND (n.created_at >= $5 OR n_en.created_at >= $5)
				ORDER BY COALESCE(n.created_at, n_en.created_at) DESC LIMIT 1
		) 
		SELECT featured_count.count + regular_count.count as count FROM 
		(
			SELECT COUNT(COALESCE(n.id,n_en.id)) as count
				FROM news n_en	
				    	LEFT JOIN news n 
						 ON n.id = n_en.id 
						AND n.language = $2
				    	AND n.type = $3
						LEFT JOIN news_viewed_by_users nvu
							ON nvu.language = n.language
							AND nvu.news_id = n.id 
							AND nvu.user_id = $1
						LEFT JOIN news_viewed_by_users nvu_en
							ON nvu_en.language = n_en.language
							AND nvu_en.news_id = n_en.id 
							AND nvu_en.user_id = $1
				WHERE n_en.language = '%[1]v'
					AND n_en.type = $3
					AND (n_en.created_at >= $5 OR n.created_at >= $5)
					AND nvu_en.created_at IS NULL 
				    AND nvu.created_at IS NULL
			) regular_count CROSS JOIN featured_count`, fallbackLanguage)
	result, err := storage.Get[UnreadNewsCount](ctx, r.db, sql, args...)
	if err != nil {
		if storage.IsErr(err, storage.ErrNotFound) { // All news are filtered by createdAfter.
			return &UnreadNewsCount{Count: 0}, nil
		}

		return nil, errors.Wrapf(err, "failed to get unread news count for params:%#v", args...)
	}

	return &UnreadNewsCount{Count: result.Count}, nil
}

func (r *repository) getNewsByPK(ctx context.Context, newsID, language string) (*TaggedNews, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	sql := `SELECT array_agg(t.news_tag) filter (where t.news_tag is not null) AS tags,
				   n.* 
			FROM news n
			      LEFT JOIN news_tags_per_news t
			      		 ON t.language = n.language
			      		AND t.news_id  = n.id
			WHERE n.language = $1
			 		AND n.id = $2
			GROUP BY n.id, n.language, t.created_at
            ORDER BY t.created_at`
	result, err := storage.Get[TaggedNews](ctx, r.db, sql, language, newsID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to select news article by (newsID:%v,language:%v)", newsID, language)
	}

	return result, nil
}

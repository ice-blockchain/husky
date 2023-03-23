// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) GetNews(ctx context.Context, newsType Type, language string, limit, offset uint64) ([]*PersonalNews, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "get news failed because context failed")
	}
	params := make(map[string]any, 1+1+1+1)
	params["type"] = newsType
	params["offset"] = offset
	params["user_id"] = requestingUserID(ctx)
	params["language"] = language
	sql := fmt.Sprintf(`SELECT nvu.created_at IS NOT NULL as viewed,
							   n.*
						FROM news n
							LEFT JOIN news_viewed_by_users nvu 
								   ON nvu.language = n.language
								  AND nvu.news_id = n.id
								  AND nvu.user_id = :user_id
						WHERE n.language = :language AND n.type = :type
						ORDER BY nvu.created_at IS NULL DESC,
								 n.created_at DESC
						LIMIT %v OFFSET :offset`, limit)
	result := make([]*PersonalNews, 0, limit)
	if err := r.db.PrepareExecuteTyped(sql, params, &result); err != nil {
		return nil, errors.Wrapf(err, "failed to get news for params:%#v", params)
	}
	for _, elem := range result {
		elem.NotificationChannels = nil
		elem.UpdatedAt = nil
		elem.ImageURL = r.pictureClient.DownloadURL(elem.ImageURL)
	}

	return result, nil
}

func (r *repository) GetUnreadNewsCount(ctx context.Context, language string, createdAfter *time.Time) (*UnreadNewsCount, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	params := make(map[string]any, 1+1+1)
	params["user_id"] = requestingUserID(ctx)
	params["language"] = language
	params["createdAfter"] = createdAfter
	sql := fmt.Sprintf(`SELECT COUNT(n.id) as count
						FROM news n	
							 LEFT JOIN news_viewed_by_users nvu
								    ON nvu.language = n.language
								   AND nvu.news_id = n.id 
								   AND nvu.user_id = :user_id
						WHERE n.language = :language 
						  AND (n.type = '%[1]v' OR n.type = '%[2]v')
						  AND n.created_at >= :createdAfter
						  AND nvu.created_at IS NULL`, RegularNewsType, FeaturedNewsType)
	result := make([]*UnreadNewsCount, 0, 1)
	if err := r.db.PrepareExecuteTyped(sql, params, &result); err != nil {
		return nil, errors.Wrapf(err, "failed to get unread news count for params:%#v", params)
	}
	if len(result) == 0 {
		return new(UnreadNewsCount), nil
	}

	return &UnreadNewsCount{Count: result[0].Count}, nil
}

func (r *repository) getNewsByPK(ctx context.Context, newsID, language string) (*TaggedNews, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	params := make(map[string]any, 1+1)
	params["id"] = newsID
	params["language"] = language
	sql := `SELECT GROUP_CONCAT(t.news_tag) AS tags,
				   n.* 
			FROM news n
			      LEFT JOIN news_tags_per_news t
			      		 ON t.language = n.language
			      		AND t.news_id  = n.id
			WHERE n.language = :language
			  AND n.id = :id
            ORDER BY t.created_at`
	rows := make([]*TaggedNews, 0, 1)
	if err := r.db.PrepareExecuteTyped(sql, params, &rows); err != nil {
		return nil, errors.Wrapf(err, "failed to select news article by (newsID:%v,language:%v)", newsID, language)
	}
	if len(rows) == 0 || rows[0].ID == "" { //nolint:revive // False negative.
		return nil, errors.Wrapf(ErrNotFound, "no news article found with (newsID:%v,language:%v)", newsID, language)
	}

	return rows[0], nil
}

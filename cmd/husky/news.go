// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"strings"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/wintr/server"
	"github.com/ice-blockchain/wintr/time"
)

func (s *service) setupNewsRoutes(router *server.Router) {
	router.
		Group("v1r").
		GET("news/:language", server.RootHandler(s.GetNews)).
		GET("unread-news-count/:language", server.RootHandler(s.GetUnreadNewsCount))
}

// GetNews godoc
//
//	@Schemes
//	@Description	Returns a list of news.
//	@Tags			News
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Insert your access token"							default(Bearer <Add access token here>)
//	@Param			type			query		string	false	"type of news to look for. Default is `regular`."	enums(regular,featured)
//	@Param			language		path		string	true	"the language of the news article"
//	@Param			limit			query		uint64	false	"Limit of elements to return. Defaults to 10"
//	@Param			offset			query		uint64	false	"Elements to skip before starting to look for"
//	@Param			createdAfter	query		string	false	"Example `2022-01-03T16:20:52.156534Z`. If unspecified, the creation date of the news articles will be ignored."
//	@Success		200				{array}		news.PersonalNews
//	@Failure		400				{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401				{object}	server.ErrorResponse	"if not authorized"
//	@Failure		422				{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500				{object}	server.ErrorResponse
//	@Failure		504				{object}	server.ErrorResponse	"if request times out"
//	@Router			/news/{language} [GET].
func (s *service) GetNews( //nolint:gocritic,funlen // False negative.
	ctx context.Context,
	req *server.Request[GetNewsArg, []*news.PersonalNews],
) (*server.Response[[]*news.PersonalNews], *server.Response[server.ErrorResponse]) {
	var createdAfter *time.Time
	if req.Data.CreatedAfter == "" {
		createdAfter = time.New(stdlibtime.Unix(0, 0).UTC())
	} else {
		createdAfter = new(time.Time)
		if err := createdAfter.UnmarshalJSON(ctx, []byte(`"`+req.Data.CreatedAfter+`"`)); err != nil {
			return nil, server.UnprocessableEntity(errors.Errorf("invalid createdAfter `%v`", req.Data.CreatedAfter), invalidPropertiesErrorCode)
		}
	}
	if req.Data.Type == "" {
		req.Data.Type = news.RegularNewsType
	}
	if req.Data.Type == news.FeaturedNewsType {
		req.Data.Limit, req.Data.Offset = 1, 0
	}
	if req.Data.Limit == 0 {
		req.Data.Limit = 10
	}
	if req.Data.Limit > 1000 { //nolint:gomnd //.
		req.Data.Limit = 1000
	}
	req.Data.Language = strings.ToLower(req.Data.Language)
	if _, validLanguage := languages[req.Data.Language]; !validLanguage {
		return nil, server.BadRequest(errors.Errorf("invalid language `%v`", req.Data.Language), invalidPropertiesErrorCode)
	}
	if req.Data.Type != news.RegularNewsType && req.Data.Type != news.FeaturedNewsType {
		return nil, server.BadRequest(errors.Errorf("invalid type %v", req.Data.Type), invalidPropertiesErrorCode)
	}
	resp, err := s.newsRepository.GetNews(ctx, req.Data.Type, req.Data.Language, req.Data.Limit, req.Data.Offset, createdAfter)
	if err != nil {
		return nil, server.Unexpected(errors.Wrapf(err, "failed to get news by %#v", req.Data))
	}

	return server.OK(&resp), nil
}

// GetUnreadNewsCount godoc
//
//	@Schemes
//	@Description	Returns the number of unread news the authorized user has.
//	@Tags			News
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Param			language		path		string	true	"The language of the news to be counted"
//	@Param			createdAfter	query		string	false	"Example `2022-01-03T16:20:52.156534Z`. If unspecified, the creation date of the news articles will be ignored."
//	@Success		200				{object}	news.UnreadNewsCount
//	@Failure		401				{object}	server.ErrorResponse	"if not authorized"
//	@Failure		422				{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500				{object}	server.ErrorResponse
//	@Failure		504				{object}	server.ErrorResponse	"if request times out"
//	@Router			/unread-news-count/{language} [GET].
func (s *service) GetUnreadNewsCount( //nolint:gocritic // False negative.
	ctx context.Context,
	req *server.Request[GetUnreadNewsCountArg, news.UnreadNewsCount],
) (*server.Response[news.UnreadNewsCount], *server.Response[server.ErrorResponse]) {
	var createdAfter *time.Time
	if req.Data.CreatedAfter == "" {
		createdAfter = time.New(stdlibtime.Unix(0, 0).UTC())
	} else {
		createdAfter = new(time.Time)
		if err := createdAfter.UnmarshalJSON(ctx, []byte(`"`+req.Data.CreatedAfter+`"`)); err != nil {
			return nil, server.UnprocessableEntity(errors.Errorf("invalid createdAfter `%v`", req.Data.CreatedAfter), invalidPropertiesErrorCode)
		}
	}
	req.Data.Language = strings.ToLower(req.Data.Language)
	if _, validLanguage := languages[req.Data.Language]; !validLanguage {
		return nil, server.BadRequest(errors.Errorf("invalid language `%v`", req.Data.Language), invalidPropertiesErrorCode)
	}
	resp, err := s.newsRepository.GetUnreadNewsCount(ctx, req.Data.Language, createdAfter)
	if err != nil {
		return nil, server.Unexpected(errors.Wrapf(err, "failed to get unread news count for userID:%v", req.AuthenticatedUser.UserID))
	}

	return server.OK(resp), nil
}

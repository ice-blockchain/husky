// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"strings"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/husky/notifications"
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
//	@Success		200				{array}		news.PersonalNews
//	@Failure		400				{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401				{object}	server.ErrorResponse	"if not authorized"
//	@Failure		422				{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500				{object}	server.ErrorResponse
//	@Failure		504				{object}	server.ErrorResponse	"if request times out"
//	@Router			/news/{language} [GET].
func (s *service) GetNews( //nolint:gocritic,funlen // False negative. Temporary.
	ctx context.Context,
	req *server.Request[GetNewsArg, []*news.PersonalNews],
) (*server.Response[[]*news.PersonalNews], *server.Response[server.ErrorResponse]) {
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
	if !strings.HasPrefix(s.cfg.Host, "staging.") {
		falseVal := false
		updatedAt := time.New(time.Now().Add(-10 * 24 * stdlibtime.Hour))
		var resp []*news.PersonalNews
		if req.Data.Type == news.FeaturedNewsType {
			resp = append(resp, &news.PersonalNews{
				News: &news.News{
					CreatedAt: updatedAt,
					UpdatedAt: updatedAt,
					NotificationChannels: &notifications.NotificationChannels{
						NotificationChannels: &users.Enum[notifications.NotificationChannel]{notifications.PushNotificationChannel},
					},
					ID:       "aaf40a9a-c8a4-49c6-bafc-b6682a4c4ddc",
					Type:     news.FeaturedNewsType,
					Language: "en",
					Title:    "Early release version",
					URL:      "https://ice.io/early-release-version",
					ImageURL: "https://ice-production.b-cdn.net/news/a275d3fe-d96a-4f12-84de-6355564d85d3_1679390631040234346.png",
					Views:    1125, //nolint:gomnd // .
				},
				Viewed: &falseVal,
			})
		} else {
			resp = append(resp, &news.PersonalNews{
				News: &news.News{
					CreatedAt: time.New(updatedAt.Add(-1 * stdlibtime.Second)),
					UpdatedAt: time.New(updatedAt.Add(-1 * stdlibtime.Second)),
					NotificationChannels: &notifications.NotificationChannels{
						NotificationChannels: &users.Enum[notifications.NotificationChannel]{notifications.PushNotificationChannel},
					},
					ID:       "bbf40a9a-c8a4-49c6-bafc-aa682a4c4ddc",
					Type:     news.RegularNewsType,
					Language: "en",
					Title:    "The Future is Now: How Present Actions are Determining Our Destiny",
					URL:      "https://ice.io/the-future-is-now-how-present-actions-are-determining-our-destiny",
					ImageURL: "https://ice-production.b-cdn.net/news/9bf2f930-dd40-40d4-a794-3745f873d820_1679336606304175788.png",
					Views:    913, //nolint:gomnd // .
				},
				Viewed: &falseVal,
			},
				&news.PersonalNews{
					News: &news.News{
						CreatedAt: time.New(updatedAt.Add(-3 * stdlibtime.Second)),
						UpdatedAt: time.New(updatedAt.Add(-3 * stdlibtime.Second)),
						NotificationChannels: &notifications.NotificationChannels{
							NotificationChannels: &users.Enum[notifications.NotificationChannel]{notifications.PushNotificationChannel},
						},
						ID:       "ccf40a9a-c8a4-49c6-bafc-b6682a4c4ddc",
						Type:     news.RegularNewsType,
						Language: "en",
						Title:    "Get Ready for a New Era – The Launch of the ice Project",
						URL:      "https://ice.io/get-ready-for-a-new-era-the-launch-of-the-ice-project",
						ImageURL: "https://ice-production.b-cdn.net/news/e3d6b2c8-0299-41ab-9c82-84c5173b2386_1679668581190361312.png",
						Views:    817, //nolint:gomnd // .
					},
					Viewed: &falseVal,
				},
				&news.PersonalNews{
					News: &news.News{
						CreatedAt: time.New(updatedAt.Add(-4 * stdlibtime.Second)),
						UpdatedAt: time.New(updatedAt.Add(-4 * stdlibtime.Second)),
						NotificationChannels: &notifications.NotificationChannels{
							NotificationChannels: &users.Enum[notifications.NotificationChannel]{notifications.PushNotificationChannel},
						},
						ID:       "ddf40a9a-c8a4-49c6-bafc-b6682a4c4ddc",
						Type:     news.RegularNewsType,
						Language: "en",
						Title:    "The ice network: A Solution to Restore Trust in Crypto Assets?",
						URL:      "https://ice.io/the-ice-network-a-solution-to-restore-trust-in-crypto-assets",
						ImageURL: "https://ice-production.b-cdn.net/news/76cce00e-7642-4530-b0c7-0e7e65920d8d_1679669742645375645.png",
						Views:    789, //nolint:gomnd // .
					},
					Viewed: &falseVal,
				},
				&news.PersonalNews{
					News: &news.News{
						CreatedAt: time.New(updatedAt.Add(-5 * stdlibtime.Second)),
						UpdatedAt: time.New(updatedAt.Add(-5 * stdlibtime.Second)),
						NotificationChannels: &notifications.NotificationChannels{
							NotificationChannels: &users.Enum[notifications.NotificationChannel]{notifications.PushNotificationChannel},
						},
						ID:       "eef40a9a-c8a4-49c6-bafc-b6682a4c4ddc",
						Type:     news.RegularNewsType,
						Language: "en",
						Title:    "Is it too late to get into the crypto game?",
						URL:      "https://ice.io/is-it-too-late-to-get-into-the-crypto-game",
						ImageURL: "https://ice-production.b-cdn.net/news/8f373914-9901-4017-a059-ba4efef0b25d_1679651118362251974.png",
						Views:    656, //nolint:gomnd // .
					},
					Viewed: &falseVal,
				})
		}

		return server.OK(&resp), nil
	}
	resp, err := s.newsRepository.GetNews(ctx, req.Data.Type, req.Data.Language, req.Data.Limit, req.Data.Offset)
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
	if !strings.HasPrefix(s.cfg.Host, "staging.") {
		resp := &news.UnreadNewsCount{}

		return server.OK(resp), nil
	}
	resp, err := s.newsRepository.GetUnreadNewsCount(ctx, req.Data.Language, createdAfter)
	if err != nil {
		return nil, server.Unexpected(errors.Wrapf(err, "failed to get unread news count for userID:%v", req.AuthenticatedUser.UserID))
	}

	return server.OK(resp), nil
}

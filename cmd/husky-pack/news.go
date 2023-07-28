// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/goccy/go-json"
	httpclient "github.com/imroc/req/v3"
	"github.com/pkg/errors"
	"golang.org/x/net/html"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/husky/notifications"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/server"
	"github.com/ice-blockchain/wintr/terror"
)

func (s *service) setupNewsRoutes(router *server.Router) {
	router.
		Group("v1w").
		POST("news", server.RootHandler(s.CreateNews)).
		DELETE("news/:language/:newsId", server.RootHandler(s.DeleteNews)).
		PATCH("news/:language/:newsId", server.RootHandler(s.ModifyNews))
}

// CreateNews godoc
//
//	@Schemes
//	@Description	Creates a news article, for each specified language.
//	@Tags			News
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			Authorization		header		string					true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Param			multiPartFormData	formData	CreateNewsRequestBody	true	"Request params"
//	@Param			image				formData	file					true	"The image for the news article"
//	@Param			newsImportFile		formData	file					false	"A json file with an array of all language variants for 1 news article"
//	@Success		201					{array}		News
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"not allowed"
//	@Failure		409					{object}	server.ErrorResponse	"if it conflicts with existing news articles"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/news [POST].
func (s *service) CreateNews( //nolint:gocritic,funlen // .
	ctx context.Context,
	req *server.Request[CreateNewsRequestBody, []*News],
) (*server.Response[[]*News], *server.Response[server.ErrorResponse]) {
	if err := verifyIfAuthorizedToAlterNews(&req.AuthenticatedUser); err != nil {
		return nil, server.Forbidden(err)
	}
	inputNews, err := req.Data.parseNews()
	if err != nil {
		return nil, server.UnprocessableEntity(errors.Wrapf(err, "failed to parse news"), invalidPropertiesErrorCode)
	}
	if err = req.Data.validateNews(inputNews); err != nil {
		return nil, server.BadRequest(errors.Wrapf(err, "invalid news records"), invalidPropertiesErrorCode)
	}
	if err = s.newsProcessor.CreateNews(ctx, inputNews, req.Data.Image); err != nil {
		err = errors.Wrapf(err, "failed to create news for %#v", inputNews)
		switch {
		case errors.Is(err, news.ErrDuplicate):
			if tErr := terror.As(err); tErr != nil {
				return nil, server.Conflict(err, duplicateNewsErrorCode, tErr.Data)
			}
		default:
			return nil, server.Unexpected(err)
		}
	}
	resp := make([]*News, 0, len(inputNews))
	for _, nws := range inputNews {
		resp = append(resp, &News{
			TaggedNews: nws,
			Checksum:   nws.Checksum(),
		})
	}

	return server.Created(&resp), nil
}

func (req *CreateNewsRequestBody) parseNews() (resp []*news.TaggedNews, err error) {
	var rawBytes []byte
	if req.NewsImportFile != nil {
		newsImportFile, oErr := req.NewsImportFile.Open()
		if oErr != nil {
			return nil, errors.Wrap(oErr, "failed to open newsImportFile")
		}
		if rawBytes, err = io.ReadAll(newsImportFile); err != nil {
			return nil, errors.Wrap(err, "failed to read from newsImportFile")
		}
		if err = newsImportFile.Close(); err != nil {
			return nil, errors.Wrap(err, "failed to close newsImportFile")
		}
	}
	if req.News != "" {
		rawBytes = []byte(req.News)
	}
	if len(rawBytes) == 0 {
		return nil, errors.Errorf("please provide a value for either `news` or `newsImportFile` parameters")
	}
	err = errors.Wrapf(json.Unmarshal(rawBytes, &resp), "failed to json unmarshal `%v` into %#v", string(rawBytes), resp)

	return
}

//nolint:funlen,gocognit,gocyclo,revive,cyclop // Better to be grouped together.
func (*CreateNewsRequestBody) validateNews(nws []*news.TaggedNews) error {
	if len(nws) == 0 {
		return errors.New("at least 1 news element is required")
	}
	const conditions = 20
	allAllowedNewsNotificationChannels := users.Enum[notifications.NotificationChannel]{
		notifications.PushOrFallbackToEmailNotificationChannel,
		notifications.InAppNotificationChannel,
		notifications.EmailNotificationChannel,
		notifications.PushNotificationChannel,
	}
	errChan := make(chan string, (conditions+len(allAllowedNewsNotificationChannels))*len(nws))
	wg := new(sync.WaitGroup)
	wg.Add(len(nws))
	allLanguages := make(map[string]int, len(nws))
	allURLs := make(map[string]int, len(nws))
	for idx, nw := range nws {
		if nw.Type != news.FeaturedNewsType && nw.Type != news.RegularNewsType {
			errChan <- fmt.Sprintf("invalid `[%v].type=%q`", idx, nw.Type)
		}
		if nw.NotificationChannels == nil || nw.NotificationChannels.NotificationChannels == nil {
			nw.NotificationChannels = &notifications.NotificationChannels{
				NotificationChannels: &users.Enum[notifications.NotificationChannel]{notifications.PushOrFallbackToEmailNotificationChannel},
			}
		} else {
			channels := make(map[notifications.NotificationChannel]int, len(allAllowedNewsNotificationChannels))
			for jj, channel := range *nw.NotificationChannels.NotificationChannels {
				channels[channel]++
				var matched bool
				for _, notificationChannel := range allAllowedNewsNotificationChannels {
					if notificationChannel == channel {
						matched = true

						break
					}
				}
				if !matched {
					errChan <- fmt.Sprintf("invalid `[%v].notificationChannels[%v]=%q`. Allowed: empty array or %#v", idx, jj, channel, allAllowedNewsNotificationChannels) //nolint:lll // .
				}
			}
			for _, val := range channels {
				if val > 1 {
					errChan <- fmt.Sprintf("invalid `[%v].notificationChannels`. list contains duplicates", idx)

					break
				}
			}
		}
		nw.Language = strings.ToLower(nw.Language)
		if _, found := languages[nw.Language]; !found {
			errChan <- fmt.Sprintf("invalid `[%v].language=%q`", idx, nw.Language)
		} else {
			allLanguages[nw.Language]++
		}
		if nw.Tags == nil || len(*nw.Tags) == 0 {
			errChan <- fmt.Sprintf("missing `[%v].tags`", idx)
		} else {
			tags := *nw.Tags
			for i := range tags {
				tags[i] = strings.ToLower(tags[i])
				if tags[i] == "" {
					errChan <- fmt.Sprintf("empty `[%v].tags[%v]`", idx, i)
				}
			}
		}
		if nw.Title == "" {
			errChan <- fmt.Sprintf("missing `[%v].title`", idx)
		}
		go func(ix int) {
			defer wg.Done()
			if nws[ix].URL == "" {
				errChan <- fmt.Sprintf("missing `[%v].url`", ix)
			} else if err := validateURL(nws[ix].URL); err != nil {
				errChan <- fmt.Sprintf("invalid `[%v].url=%q`, %v", ix, nws[ix].URL, err)
			} else {
				allURLs[nws[ix].URL]++
			}
		}(idx)
	}
	wg.Wait()
	for k, v := range allLanguages {
		if v > 1 {
			errChan <- fmt.Sprintf("language `%v` is present in multiple items", k)

			break
		}
	}
	if false {
		for k, v := range allURLs {
			if v > 1 {
				errChan <- fmt.Sprintf("url `%v` is present in multiple items", k)

				break
			}
		}
	}
	close(errChan)
	errs := make([]string, 0, len(errChan))
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return errors.New(strings.Join(errs, "------"))
	}

	return nil
}

// DeleteNews godoc
//
//	@Schemes
//	@Description	Deletes a language variant of a news article
//	@Tags			News
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header	string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Param			newsId			path	string	true	"ID of the news article"
//	@Param			language		path	string	true	"the language code"
//	@Success		200				"OK - found and deleted"
//	@Success		204				"No Content - already deleted"
//	@Failure		400				{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401				{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403				{object}	server.ErrorResponse	"if not allowed"
//	@Failure		422				{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500				{object}	server.ErrorResponse
//	@Failure		504				{object}	server.ErrorResponse	"if request times out"
//	@Router			/news/{language}/{newsId} [DELETE].
func (s *service) DeleteNews( //nolint:gocritic // .
	ctx context.Context,
	req *server.Request[DeleteNewsArg, any],
) (*server.Response[any], *server.Response[server.ErrorResponse]) {
	if err := verifyIfAuthorizedToAlterNews(&req.AuthenticatedUser); err != nil {
		return nil, server.Forbidden(err)
	}
	if err := s.newsProcessor.DeleteNews(ctx, req.Data.NewsID, req.Data.Language); err != nil {
		err = errors.Wrapf(err, "failed to delete news for %#v", req.Data)
		switch {
		case errors.Is(err, news.ErrNotFound):
			return server.NoContent(), nil
		default:
			return nil, server.Unexpected(err)
		}
	}

	return server.OK[any](), nil
}

// ModifyNews godoc
//
//	@Schemes
//	@Description	Modifies a language variant of a news article
//	@Tags			News
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			Authorization		header		string					true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Param			newsId				path		string					true	"ID of the news article"
//	@Param			language			path		string					true	"The language of the news article"
//	@Param			multiPartFormData	formData	ModifyNewsRequestBody	false	"Request params"
//	@Param			image				formData	file					false	"The image for the news article"
//	@Success		200					{object}	News
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"if not allowed"
//	@Failure		404					{object}	server.ErrorResponse	"if news or user not found"
//	@Failure		409					{object}	server.ErrorResponse	"if conflict occurs"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/news/{language}/{newsId} [PATCH].
func (s *service) ModifyNews( //nolint:gocritic,funlen // .
	ctx context.Context,
	req *server.Request[ModifyNewsRequestBody, News],
) (*server.Response[News], *server.Response[server.ErrorResponse]) {
	if req.Data.MarkViewed != nil && *req.Data.MarkViewed {
		if err := s.newsProcessor.IncrementViews(ctx, req.Data.NewsID, req.Data.Language); err != nil {
			err = errors.Wrapf(err, "failed to increment views for %#v", req.Data)
			switch {
			case errors.Is(err, storage.ErrNotFound):
				return nil, server.NotFound(err, newsNotFoundErrorCode)
			case errors.Is(err, storage.ErrDuplicate):
				return nil, server.Conflict(err, alreadyViewedNewsErrorCode)
			default:
				return nil, server.Unexpected(err)
			}
		}

		return server.OK[News](), nil
	}
	if err := verifyIfAuthorizedToAlterNews(&req.AuthenticatedUser); err != nil {
		return nil, server.Forbidden(err)
	}
	if err := req.Data.validate(); err != nil {
		return nil, server.BadRequest(errors.Wrapf(err, "invalid request properties"), invalidPropertiesErrorCode)
	}
	nws := &news.TaggedNews{
		News: &news.News{
			ID:       req.Data.NewsID,
			Language: req.Data.Language,
			Type:     req.Data.Type,
			Title:    req.Data.Title,
			URL:      req.Data.URL,
		},
		Tags: req.Data.Tags,
	}
	if err := s.newsProcessor.ModifyNews(news.ContextWithChecksum(ctx, req.Data.Checksum), nws, req.Data.Image); err != nil {
		err = errors.Wrapf(err, "failed to modify news for %#v", req.Data)
		switch {
		case errors.Is(err, news.ErrInvalidImageExtension):
			return nil, server.BadRequest(errors.Wrapf(err, "invalid image extension"), invalidPropertiesErrorCode)
		case errors.Is(err, news.ErrRaceCondition):
			return nil, server.BadRequest(err, raceConditionErrorCode)
		case errors.Is(err, news.ErrNotFound):
			return nil, server.NotFound(err, newsNotFoundErrorCode)
		case errors.Is(err, news.ErrDuplicate):
			return nil, server.Conflict(err, duplicateNewsErrorCode)
		default:
			return nil, server.Unexpected(err)
		}
	}

	return server.OK(&News{TaggedNews: nws, Checksum: nws.Checksum()}), nil
}

func (req *ModifyNewsRequestBody) validate() error { //nolint:gocognit // Beg to differ.
	var errs []string
	if req.Type != "" && req.Type != news.FeaturedNewsType && req.Type != news.RegularNewsType {
		errs = append(errs, fmt.Sprintf("invalid `type=%q`", req.Type))
	}
	if err := validateURL(req.URL); err != nil {
		errs = append(errs, fmt.Sprintf("invalid `url=%q`, %v", req.URL, err))
	}
	if req.Type == "" && req.Image == nil && req.Title == "" && req.Tags == nil && req.URL == "" {
		errs = append(errs, "at least one property has to be specified")
	}
	if len(errs) != 0 {
		return errors.New(strings.Join(errs, "------"))
	}

	return nil
}

func verifyIfAuthorizedToAlterNews(usr *server.AuthenticatedUser) error {
	if !strings.EqualFold(usr.Role, "admin") && !strings.EqualFold(usr.Role, "author") {
		return errors.Errorf("access denied, invalid role `%v`", usr.Role)
	}

	return nil
}

func validateURL(url string) error {
	if url == "" {
		return nil
	}
	if resp, err := httpclient.Get(url); err != nil {
		return errors.Wrapf(err, "unable to execute get request for url:%v", url)
	} else if resp.Err != nil {
		return errors.Wrapf(err, "verifying url request returns some underlying error for url:%v", url)
	} else if _, err = html.Parse(resp.Body); err != nil {
		return errors.Wrapf(err, "html invalid in the response from url:%v", url)
	}

	return nil
}

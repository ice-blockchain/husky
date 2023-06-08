// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/cmd/husky/api"
	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/husky/notifications"
	appCfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/server"
	"github.com/ice-blockchain/wintr/time"
)

// @title						Notifications API
// @version					latest
// @description				API that handles everything related to read-only operations for notifying users about anything worthwhile.
// @query.collection.format	multi
// @schemes					https
// @contact.name				ice.io
// @contact.url				https://ice.io
// @BasePath					/v1r
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cfg config
	appCfg.MustLoadFromKey(applicationYamlKey, &cfg)
	api.SwaggerInfo.Host = cfg.Host
	api.SwaggerInfo.Version = cfg.Version
	s := new(service)
	s.cfg = &cfg
	server.New(s, applicationYamlKey, swaggerRoot).ListenAndServe(ctx, cancel)
}

func (s *service) RegisterRoutes(router *server.Router) {
	s.setupNewsRoutes(router)
	s.setupNotificationsRoutes(router)
}

func (s *service) Init(ctx context.Context, cancel context.CancelFunc) {
	s.newsRepository = news.New(ctx, cancel)
	s.notificationsRepository = notifications.New(ctx, cancel)
}

func (s *service) Close(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "could not close service because context ended")
	}

	return multierror.Append( //nolint:wrapcheck // .
		errors.Wrap(s.newsRepository.Close(), "could not close news repository"),
		errors.Wrap(s.notificationsRepository.Close(), "could not close notifications repository"),
	).ErrorOrNil()
}

func (s *service) CheckHealth(ctx context.Context) error {
	log.Debug("checking health...", "package", "news")
	if _, err := s.newsRepository.GetNews(ctx, news.FeaturedNewsType, "en", 1, 0, time.Now()); err != nil {
		return errors.Wrap(err, "failed to get featured news")
	}
	log.Debug("checking health...", "package", "notifications")
	if _, err := s.notificationsRepository.GetNotificationChannelToggles(ctx, notifications.PushNotificationChannel, "bogus"); err != nil {
		return errors.Wrap(err, "failed to GetNotificationChannelToggles")
	}

	return nil
}

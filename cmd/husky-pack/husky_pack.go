// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/analytics"
	"github.com/ice-blockchain/husky/cmd/husky-pack/api"
	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/husky/notifications"
	appCfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/server"
)

// @title						Notifications API
// @version					latest
// @description				API that handles everything related to write-only operations for notifying users about anything worthwhile.
// @query.collection.format	multi
// @schemes					https
// @contact.name				ice.io
// @contact.url				https://ice.io
// @BasePath					/v1w
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cfg config
	appCfg.MustLoadFromKey(applicationYamlKey, &cfg)
	api.SwaggerInfo.Host = cfg.Host
	api.SwaggerInfo.Version = cfg.Version
	server.New(new(service), applicationYamlKey, swaggerRoot).ListenAndServe(ctx, cancel)
}

func (s *service) RegisterRoutes(router *server.Router) {
	s.setupNotificationsRoutes(router)
	s.setupNewsRoutes(router)
}

func (s *service) Init(ctx context.Context, cancel context.CancelFunc) {
	s.analyticsProcessor = analytics.StartProcessor(ctx, cancel)
	s.newsProcessor = news.StartProcessor(ctx, cancel)
	s.notificationsProcessor = notifications.StartProcessor(ctx, cancel)
}

func (s *service) Close(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "could not close processors because context ended")
	}

	return errors.Wrap(multierror.Append(
		errors.Wrap(s.newsProcessor.Close(), "could not close news processor"),
		errors.Wrap(s.notificationsProcessor.Close(), "could not close notifications processor"),
		errors.Wrap(s.analyticsProcessor.Close(), "could not close analytics processor"),
	).ErrorOrNil(), "could not close processors")
}

func (s *service) CheckHealth(ctx context.Context) error {
	log.Debug("checking health...", "package", "news")

	if err := s.newsProcessor.CheckHealth(ctx); err != nil {
		return errors.Wrapf(err, "news processor health check failed")
	}

	log.Debug("checking health...", "package", "notifications")
	if err := s.notificationsProcessor.CheckHealth(ctx); err != nil {
		return errors.Wrapf(err, "notifications processor health check failed")
	}
	log.Debug("checking health...", "package", "analytics")
	if err := s.analyticsProcessor.CheckHealth(ctx); err != nil {
		return errors.Wrapf(err, "analytics processor health check failed")
	}

	return nil
}

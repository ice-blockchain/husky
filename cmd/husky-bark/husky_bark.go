// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/cmd/husky-pack/api"
	"github.com/ice-blockchain/husky/notifications"
	appcfg "github.com/ice-blockchain/wintr/config"
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
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)
	api.SwaggerInfo.Host = cfg.Host
	api.SwaggerInfo.Version = cfg.Version
	server.New(new(service), applicationYamlKey, swaggerRoot).ListenAndServe(ctx, cancel)
}

func (*service) RegisterRoutes(_ *server.Router) {
}

func (s *service) Init(ctx context.Context, _ context.CancelFunc) {
	s.notificationsScheduler = notifications.MustStartScheduler(ctx)
}

func (s *service) Close(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "could not close processors because context ended")
	}

	return errors.Wrap(s.notificationsScheduler.Close(), "could not close notifications scheduler")
}

func (s *service) CheckHealth(ctx context.Context) error {
	log.Debug("checking health...", "package", "notifications")
	if err := s.notificationsScheduler.CheckHealth(ctx); err != nil {
		return errors.Wrapf(err, "notifications scheduler health check failed")
	}

	return nil
}

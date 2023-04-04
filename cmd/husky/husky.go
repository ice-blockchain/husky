// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/cmd/husky/api"
	appCfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/server"
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
	server.New(new(service), applicationYamlKey, swaggerRoot).ListenAndServe(ctx, cancel)
}

func (s *service) RegisterRoutes(router *server.Router) {
	s.setupNewsRoutes(router)
	s.setupNotificationsRoutes(router)
}

func (*service) Init(_ context.Context, _ context.CancelFunc) {
}

func (*service) Close(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "could not close service because context ended")
	}

	return nil
}

func (*service) CheckHealth(_ context.Context) error {
	log.Debug("checking health...", "package", "news")

	return nil
}

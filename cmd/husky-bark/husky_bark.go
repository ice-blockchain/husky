// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/notifications"
	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/server"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const pkgName = "cmd/husky-bark"

	var cfg struct{ Version string }
	appcfg.MustLoadFromKey(pkgName, &cfg)

	log.Info(fmt.Sprintf("starting version `%v`...", cfg.Version))

	server.New(new(service), pkgName, "").ListenAndServe(ctx, cancel)
}

type (
	// | service implements server.State and is responsible for managing the state and lifecycle of the package.
	service struct{ notificationsScheduler *notifications.Scheduler }
)

func (*service) RegisterRoutes(_ *server.Router) {
}

func (s *service) Init(ctx context.Context, cancel context.CancelFunc) {
	s.notificationsScheduler = notifications.MustStartScheduler(ctx, cancel)
}

func (s *service) Close(_ context.Context) error {
	return errors.Wrap(s.notificationsScheduler.Close(), "could not close service")
}

func (s *service) CheckHealth(ctx context.Context) error {
	log.Debug("checking health...", "package", "notifications")
	if err := s.notificationsScheduler.CheckHealth(ctx); err != nil {
		return errors.Wrapf(err, "notifications scheduler health check failed")
	}

	return nil
}

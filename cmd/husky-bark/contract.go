// SPDX-License-Identifier: ice License 1.0

package main

import (
	"github.com/ice-blockchain/husky/notifications"
)

// Private API.

const (
	applicationYamlKey = "cmd/husky-bark"
	swaggerRoot        = "/notifications/w"
)

type (
	// | service implements server.State and is responsible for managing the state and lifecycle of the package.
	service struct {
		notificationsScheduler *notifications.Scheduler
	}
	config struct {
		Host    string `yaml:"host"`
		Version string `yaml:"version"`
	}
)

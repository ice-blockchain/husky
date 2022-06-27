// SPDX-License-Identifier: BUSL-1.1

package main

import (
	_ "embed"

	"github.com/ice-blockchain/husky/notifications"
	"github.com/ice-blockchain/wintr/server"
)

// Public API.

type (
	RequestNotifyUser struct {
		AuthenticatedUser server.AuthenticatedUser `json:"-" swaggerignore:"true"`
		notifications.NotifyUserArg
	}
)

// Private API.

const (
	applicationYamlKey = "cmd/husky-pack"
)

// Values for server.ErrorResponse#Code.
const (
	userNotFoundErrorCode        = "USER_NOT_FOUND"
	userAlreadyNotifiedErrorCode = "USER_ALREADY_NOTIFIED"
	invalidPropertiesErrorCode   = "INVALID_PROPERTIES"
)

//nolint:gochecknoglobals // Because its loaded once, at runtime.
var cfg config

type (
	// | service implements server.State and is responsible for managing the state and lifecycle of the package.
	service struct {
		notificationsProcessor notifications.Processor
	}
	config struct {
		Host    string `yaml:"host"`
		Version string `yaml:"version"`
	}
)

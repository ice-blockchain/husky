// SPDX-License-Identifier: BUSL-1.1

package notifications

import (
	"context"
	_ "embed"
	"io"

	"github.com/framey-io/go-tarantool"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/connectors/storage"
)

// Public API.

const (
	PingNotification = "PING"
)

var (
	ErrNotFound              = storage.ErrNotFound
	ErrDuplicate             = storage.ErrDuplicate
	ErrPingingUserNotAllowed = errors.New("pinging user is not allowed")
	//nolint:gochecknoglobals // It's just for more descriptive validation messages.
	NotificationTypes = [1]string{PingNotification}
)

type (
	Repository interface {
		io.Closer
		NotifyUser(context.Context, *NotifyUserArg) error
	}
	Processor interface {
		Repository
		CheckHealth(context.Context) error
	}
)

// Args.

type (
	NotifyUserArg struct {
		ActorUserID   string `json:"-" swaggerignore:"true"`
		SubjectUserID string `json:"userId" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		// Optional, default is `PING`.
		NotificationType string `json:"type" enums:"PING"`
	}
)

// Private API.

const (
	applicationYamlKey = "notifications"
)

var (
	//go:embed DDL.lua
	ddl string
	//nolint:gochecknoglobals // Because its loaded once, at runtime.
	cfg config
)

type (

	// | repository implements the public API that this package exposes.
	repository struct {
		db    tarantool.Connector
		mb    messagebroker.Client
		close func() error
	}
	processor struct {
		*repository
	}
	// | config holds the configuration of this package mounted from `application.yaml`.
	config struct {
		MessageBroker struct {
			ConsumingTopics []string `yaml:"consumingTopics"`
			Topics          []struct {
				Name string `yaml:"name" json:"name"`
			} `yaml:"topics"`
		} `yaml:"messageBroker"`
	}
)

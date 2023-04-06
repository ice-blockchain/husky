// SPDX-License-Identifier: ice License 1.0

package news

import (
	"context"
	_ "embed"
	storagev2 "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"io"
	"mime/multipart"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/go-tarantool-client"
	"github.com/ice-blockchain/husky/notifications"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/multimedia/picture"
	"github.com/ice-blockchain/wintr/time"
)

// Public API.

const (
	RegularNewsType  Type = "regular"
	FeaturedNewsType Type = "featured"
)

var (
	ErrNotFound              = storage.ErrNotFound
	ErrDuplicate             = storage.ErrDuplicate
	ErrRaceCondition         = errors.New("race condition")
	ErrInvalidImageExtension = errors.New("invalid image extension")
)

type (
	Type         = string
	Tag          = string
	Tags         = users.Enum[Tag]
	PersonalNews struct {
		Viewed *bool `json:"viewed,omitempty" example:"true"`
		*News
	}
	TaggedNews struct {
		Tags *Tags `json:"tags,omitempty" example:"cats,dogs,frogs"`
		*News
	}
	News struct {
		_msgpack  struct{}   `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // To insert we need asArray
		CreatedAt *time.Time `json:"createdAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		UpdatedAt *time.Time `json:"updatedAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		*notifications.NotificationChannels
		ID       string `json:"id,omitempty" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		Type     Type   `json:"type,omitempty" example:"regular"`
		Language string `json:"language,omitempty" example:"en"`
		Title    string `json:"title,omitempty" example:"The importance of the blockchain technology"`
		ImageURL string `json:"imageUrl,omitempty" example:"https://somewebsite.com/blockchain.jpg"`
		URL      string `json:"url,omitempty" example:"https://somewebsite.com/blockchain"`
		Views    uint64 `json:"views,omitempty" example:"123"`
	}
	TaggedNewsSnapshot struct {
		*TaggedNews
		Before *TaggedNews `json:"before,omitempty"`
	}
	ViewedNews struct {
		_msgpack  struct{}   `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // To insert we need asArray
		CreatedAt *time.Time `json:"createdAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		NewsID    string     `json:"newsId" example:"edfd8c02-75e0-4687-9ac2-1ce4723865c4"`
		Language  string     `json:"language" example:"en"`
		UserID    string     `json:"userId" example:"7bed2a2d-cb25-4b59-8e9b-93708630d8dc"`
	}
	UnreadNewsCount struct {
		_msgpack struct{} `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // To insert we need asArray
		Count    uint64   `json:"count" example:"1"`
	}
	ReadRepository interface {
		GetNews(ctx context.Context, newsType Type, language string, limit, offset uint64) ([]*PersonalNews, error)
		GetUnreadNewsCount(ctx context.Context, language string, createdAfter *time.Time) (*UnreadNewsCount, error)
	}
	WriteRepository interface {
		CreateNews(ctx context.Context, news []*TaggedNews, image *multipart.FileHeader) error
		ModifyNews(ctx context.Context, news *TaggedNews, image *multipart.FileHeader) error
		DeleteNews(ctx context.Context, newsID, language string) error
		IncrementViews(ctx context.Context, newsID, language string) error
	}
	Repository interface {
		io.Closer

		ReadRepository
		WriteRepository
	}
	Processor interface {
		Repository
		CheckHealth(context.Context) error
	}
)

// Private API.

const (
	applicationYamlKey          = "news"
	requestingUserIDCtxValueKey = "requestingUserIDCtxValueKey"
	checksumCtxValueKey         = "versioningChecksumCtxValueKey"
)

// .
var (
	//go:embed DDL.lua
	ddl string
	//go:embed DDL.sql
	ddlV2 string
)

type (
	// | repository implements the public API that this package exposes.
	repository struct {
		cfg           *config
		shutdown      func() error
		db            tarantool.Connector
		dbV2          *storagev2.DB
		mb            messagebroker.Client
		pictureClient picture.Client
	}

	processor struct {
		*repository
	}
	// | config holds the configuration of this package mounted from `application.yaml`.
	config struct {
		DeeplinkApp          string                   `yaml:"deeplinkApp"`
		messagebroker.Config `mapstructure:",squash"` //nolint:tagliatelle // Nope.
	}
)

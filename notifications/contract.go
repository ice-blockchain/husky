// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"embed"
	"io"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storagev2 "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/email"
	"github.com/ice-blockchain/wintr/multimedia/picture"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

// Public API.

const (
	InAppNotificationChannel                                      NotificationChannel = "inapp"
	SMSNotificationChannel                                        NotificationChannel = "sms"
	EmailNotificationChannel                                      NotificationChannel = "email"
	PushNotificationChannel                                       NotificationChannel = "push"
	AnalyticsNotificationChannel                                  NotificationChannel = "analytics"
	PushOrFallbackToAnalyticsNotificationChannel                  NotificationChannel = "push||analytics"
	PushOrFallbackToEmailNotificationChannel                      NotificationChannel = "push||email"
	PushOrFallbackToEmailOrFallbackToAnalyticsNotificationChannel NotificationChannel = "push||email||analytics"
)

const (
	DisableAllNotificationDomain     NotificationDomain = "disable_all"
	AllNotificationDomain            NotificationDomain = "all"
	WeeklyReportNotificationDomain   NotificationDomain = "weekly_report"
	WeeklyStatsNotificationDomain    NotificationDomain = "weekly_stats"
	AchievementsNotificationDomain   NotificationDomain = "achievements"
	PromotionsNotificationDomain     NotificationDomain = "promotions"
	NewsNotificationDomain           NotificationDomain = "news"
	MicroCommunityNotificationDomain NotificationDomain = "micro_community"
	MiningNotificationDomain         NotificationDomain = "mining"
	DailyBonusNotificationDomain     NotificationDomain = "daily_bonus"
	SystemNotificationDomain         NotificationDomain = "system"
)

const (
	AdoptionChangedNotificationType     NotificationType = "adoption_changed"
	DailyBonusNotificationType          NotificationType = "daily_bonus"
	NewContactNotificationType          NotificationType = "new_contact"
	NewReferralNotificationType         NotificationType = "new_referral"
	NewsAddedNotificationType           NotificationType = "news_added"
	PingNotificationType                NotificationType = "ping"
	LevelBadgeUnlockedNotificationType  NotificationType = "level_badge_unlocked"
	CoinBadgeUnlockedNotificationType   NotificationType = "coin_badge_unlocked"
	SocialBadgeUnlockedNotificationType NotificationType = "social_badge_unlocked"
	RoleChangedNotificationType         NotificationType = "role_changed"
	LevelChangedNotificationType        NotificationType = "level_changed"
)

var (
	ErrNotFound              = storagev2.ErrNotFound
	ErrDuplicate             = storagev2.ErrDuplicate
	ErrRelationNotFound      = storagev2.ErrRelationNotFound
	ErrPingingUserNotAllowed = errors.New("pinging user is not allowed")
	//nolint:gochecknoglobals // It's just for more descriptive validation messages.
	AllNotificationChannels = users.Enum[NotificationChannel]{
		PushOrFallbackToEmailOrFallbackToAnalyticsNotificationChannel,
		PushOrFallbackToAnalyticsNotificationChannel,
		PushOrFallbackToEmailNotificationChannel,
		AnalyticsNotificationChannel,
		InAppNotificationChannel,
		SMSNotificationChannel,
		EmailNotificationChannel,
		PushNotificationChannel,
	}
	//nolint:gochecknoglobals // It's just for more descriptive validation messages.
	AllNotificationTypes = users.Enum[NotificationType]{
		AdoptionChangedNotificationType,
		DailyBonusNotificationType,
		NewContactNotificationType,
		NewReferralNotificationType,
		NewsAddedNotificationType,
		PingNotificationType,
		LevelBadgeUnlockedNotificationType,
		CoinBadgeUnlockedNotificationType,
		SocialBadgeUnlockedNotificationType,
		RoleChangedNotificationType,
		LevelChangedNotificationType,
	}
	//nolint:gochecknoglobals // It's just for more descriptive validation messages.
	AllNotificationDomains = map[NotificationChannel][]NotificationDomain{
		EmailNotificationChannel: {
			DisableAllNotificationDomain,
			WeeklyReportNotificationDomain,
			AchievementsNotificationDomain,
			PromotionsNotificationDomain,
			NewsNotificationDomain,
			MicroCommunityNotificationDomain,
			MiningNotificationDomain,
			DailyBonusNotificationDomain,
			SystemNotificationDomain,
		},
		PushNotificationChannel: {
			DisableAllNotificationDomain,
			WeeklyStatsNotificationDomain,
			AchievementsNotificationDomain,
			PromotionsNotificationDomain,
			NewsNotificationDomain,
			MicroCommunityNotificationDomain,
			MiningNotificationDomain,
			DailyBonusNotificationDomain,
			SystemNotificationDomain,
		},
	}
)

type (
	InAppNotificationsUserAuthToken = inapp.Token
	NotificationChannel             string
	NotificationDomain              string
	NotificationType                string
	NotificationChannels            struct {
		NotificationChannels *users.Enum[NotificationChannel] `json:"notificationChannels,omitempty" swaggertype:"array,string" enums:"inapp,sms,email,push,analytics,push||analytics,push||email,push||email||analytics"` //nolint:lll // .
	}
	NotificationChannelToggle struct {
		Type    NotificationDomain `json:"type" example:"system"`
		Enabled bool               `json:"enabled" example:"true"`
	}
	UserPing struct {
		LastPingCooldownEndedAt *time.Time `json:"lastPingCooldownEndedAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		UserID                  string     `json:"userId,omitempty" example:"edfd8c02-75e0-4687-9ac2-1ce4723865c4"`
		PingedBy                string     `json:"pingedBy,omitempty" example:"edfd8c02-75e0-4687-9ac2-1ce4723865c4"`
	}
	ReadRepository interface {
		GetNotificationChannelToggles(ctx context.Context, channel NotificationChannel, userID string) ([]*NotificationChannelToggle, error)
	}
	WriteRepository interface {
		ToggleNotificationChannelDomain(ctx context.Context, channel NotificationChannel, domain NotificationDomain, enabled bool, userID string) error

		GenerateInAppNotificationsUserAuthToken(ctx context.Context, userID string) (*InAppNotificationsUserAuthToken, error)

		PingUser(ctx context.Context, userID string) error
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
	applicationYamlKey          = "notifications"
	requestingUserIDCtxValueKey = "requestingUserIDCtxValueKey"
)

var (
	//go:embed DDL.lua
	ddl string
	//go:embed DDL.sql
	ddlV2 string
	//go:embed translations
	translations embed.FS
	//nolint:gochecknoglobals // Its loaded once at startup.
	allPushNotificationTemplates map[NotificationType]map[languageCode]*pushNotificationTemplate
	//nolint:gochecknoglobals // Its loaded once at startup.
	internationalizedEmailDisplayNames = map[string]string{
		"en": "ice: Decentralized Future",
	}
)

type (
	languageCode = string
	user         struct {
		LastPingCooldownEndedAt          *time.Time                      `json:"lastPingCooldownEndedAt,omitempty"`
		DisabledPushNotificationDomains  *users.Enum[NotificationDomain] `json:"disabledPushNotificationDomains,omitempty"`
		DisabledEmailNotificationDomains *users.Enum[NotificationDomain] `json:"disabledEmailNotificationDomains,omitempty"`
		DisabledSMSNotificationDomains   *users.Enum[NotificationDomain] `json:"disabledSMSNotificationDomains,omitempty"` //nolint:tagliatelle // Wrong.
		PhoneNumber                      string                          `json:"phoneNumber,omitempty"`
		Email                            string                          `json:"email,omitempty"`
		FirstName                        string                          `json:"firstName,omitempty"`
		LastName                         string                          `json:"lastName,omitempty"`
		UserID                           string                          `json:"userId,omitempty"`
		Username                         string                          `json:"username,omitempty"`
		ProfilePictureName               string                          `json:"profilePictureName,omitempty"`
		ReferredBy                       string                          `json:"referredBy,omitempty"`
		PhoneNumberHash                  string                          `json:"phoneNumberHash,omitempty"`
		AgendaPhoneNumberHashes          string                          `json:"agendaPhoneNumberHashes,omitempty"`
		Language                         string                          `json:"language,omitempty"`
	}
	userTableSource struct {
		*processor
	}
	deviceMetadataTableSource struct {
		*processor
	}
	adoptionTableSource struct {
		*processor
	}
	newsTableSource struct {
		*processor
	}
	availableDailyBonusSource struct {
		*processor
	}
	userPingSource struct {
		*processor
	}
	startedDaysOffSource struct {
		*processor
	}
	achievedBadgesSource struct {
		*processor
	}
	completedLevelsSource struct {
		*processor
	}
	enabledRolesSource struct {
		*processor
	}
	repository struct {
		cfg                     *config
		shutdown                func() error
		db                      *storagev2.DB
		mb                      messagebroker.Client
		pushNotificationsClient push.Client
		emailClient             email.Client
		pictureClient           picture.Client
		personalInAppFeed       inapp.Client
		globalInAppFeed         inapp.Client
	}
	processor struct {
		*repository
	}
	config struct {
		DeeplinkScheme                                   string                   `yaml:"deeplinkScheme"`
		messagebroker.Config                             `mapstructure:",squash"` //nolint:tagliatelle // Nope.
		PingCooldown                                     stdlibtime.Duration      `yaml:"pingCooldown"`
		DisableBadgeUnlockedPushOrAnalyticsNotifications bool                     `yaml:"disableBadgeUnlockedPushOrAnalyticsNotifications"`
	}
)

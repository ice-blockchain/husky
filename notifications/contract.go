// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"embed"
	"io"
	"sync"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	storagev3 "github.com/ice-blockchain/wintr/connectors/storage/v3"
	"github.com/ice-blockchain/wintr/email"
	"github.com/ice-blockchain/wintr/multimedia/picture"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/notifications/telegram"
	"github.com/ice-blockchain/wintr/time"
)

// Public API.

const (
	InAppNotificationChannel                 NotificationChannel = "inapp"
	SMSNotificationChannel                   NotificationChannel = "sms"
	EmailNotificationChannel                 NotificationChannel = "email"
	PushNotificationChannel                  NotificationChannel = "push"
	TelegramNotificationChannel              NotificationChannel = "telegram"
	PushOrFallbackToEmailNotificationChannel NotificationChannel = "push||email"
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
	AdoptionChangedNotificationType            NotificationType = "adoption_changed"
	DailyBonusNotificationType                 NotificationType = "daily_bonus"
	NewContactNotificationType                 NotificationType = "new_contact"
	NewReferralNotificationType                NotificationType = "new_referral"
	NewsAddedNotificationType                  NotificationType = "news_added"
	PingNotificationType                       NotificationType = "ping"
	LevelBadgeUnlockedNotificationType         NotificationType = "level_badge_unlocked"
	CoinBadgeUnlockedNotificationType          NotificationType = "coin_badge_unlocked"
	SocialBadgeUnlockedNotificationType        NotificationType = "social_badge_unlocked"
	RoleChangedNotificationType                NotificationType = "role_changed"
	LevelChangedNotificationType               NotificationType = "level_changed"
	MiningExtendNotificationType               NotificationType = "mining_extend"
	MiningEndingSoonNotificationType           NotificationType = "mining_ending_soon"
	MiningExpiredNotificationType              NotificationType = "mining_expired"
	MiningNotActiveNotificationType            NotificationType = "mining_not_active"
	InviteFriendNotificationType               NotificationType = "invite_friend"
	SocialsFollowIceOnXNotificationType        NotificationType = "follow_ion_on_x"
	SocialsFollowUsOnXNotificationType         NotificationType = "follow_us_on_x"
	SocialsFollowZeusOnXNotificationType       NotificationType = "follow_zeus_on_x"
	SocialsFollowIONOnTelegramNotificationType NotificationType = "join_ion_on_telegram"
	SocialsFollowOurTelegramNotificationType   NotificationType = "join_our_telegram"
	WeeklyStatsNotificationType                NotificationType = "weekly_stats"
	ReplyNotificationType                      NotificationType = "reply"
	SocialsNotificationType                    NotificationType = "socials"
)

var (
	ErrNotFound              = storage.ErrNotFound
	ErrDuplicate             = storage.ErrDuplicate
	ErrRelationNotFound      = storage.ErrRelationNotFound
	ErrPingingUserNotAllowed = errors.New("pinging user is not allowed")
	//nolint:gochecknoglobals // It's just for more descriptive validation messages.
	AllNotificationChannels = users.Enum[NotificationChannel]{
		PushOrFallbackToEmailNotificationChannel,
		InAppNotificationChannel,
		SMSNotificationChannel,
		EmailNotificationChannel,
		PushNotificationChannel,
		TelegramNotificationChannel,
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
		MiningExtendNotificationType,
		MiningEndingSoonNotificationType,
		MiningExpiredNotificationType,
		MiningNotActiveNotificationType,
		InviteFriendNotificationType,
		SocialsFollowIceOnXNotificationType,
		SocialsFollowUsOnXNotificationType,
		SocialsFollowZeusOnXNotificationType,
		SocialsFollowIONOnTelegramNotificationType,
		SocialsFollowOurTelegramNotificationType,
		WeeklyStatsNotificationType,
	}
	//nolint:gochecknoglobals // It's just for more descriptive validation messages.
	AllTelegramNotificationTypes = users.Enum[NotificationType]{
		LevelBadgeUnlockedNotificationType,
		CoinBadgeUnlockedNotificationType,
		SocialBadgeUnlockedNotificationType,
		RoleChangedNotificationType,
		LevelChangedNotificationType,
		MiningExtendNotificationType,
		MiningEndingSoonNotificationType,
		MiningExpiredNotificationType,
		MiningNotActiveNotificationType,
		InviteFriendNotificationType,
		NewReferralNotificationType,
		SocialsNotificationType,
		ReplyNotificationType,
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
		NotificationChannels *users.Enum[NotificationChannel] `json:"notificationChannels,omitempty" swaggertype:"array,string" enums:"inapp,sms,email,push,push||email"` //nolint:lll // .
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
		CheckHealth(ctx context.Context) error
	}
	Scheduler struct {
		pictureClient                    picture.Client
		pushNotificationsClient          push.Client
		telegramNotificationsClient      telegram.Client
		cfg                              *config
		schedulerPushAnnouncementsMX     *sync.Mutex
		schedulerPushNotificationsMX     *sync.Mutex
		schedulerTelegramNotificationsMX *sync.Mutex
		telemetryPushNotifications       *telemetry
		telemetryTelegramNotifications   *telemetry
		telemetryAnnouncements           *telemetry
		db                               *storage.DB
		wg                               *sync.WaitGroup
		cancel                           context.CancelFunc
	}
)

// Private API.

const (
	applicationYamlKey          = "notifications"
	requestingUserIDCtxValueKey = "requestingUserIDCtxValueKey"
	requestDeadline             = 30 * stdlibtime.Second

	schedulerWorkersCount    int64 = 10
	schedulerPushBatchSize   int64 = 250
	telegramLongPollingLimit int64 = 100

	defaultLanguage = "en"
	// Protection from getting ErrDuplicate on session creation due to ReferralsCountChangeGuardUpdatedAt on freezer.
	minerLatencyToFinishProcessingIteration = 10 * stdlibtime.Minute
)

var (
	//go:embed DDL.sql
	ddl string
	//go:embed translations
	translations embed.FS
	//nolint:gochecknoglobals // Its loaded once at startup.
	allPushNotificationTemplates map[NotificationType]map[languageCode]*pushNotificationTemplate
	//nolint:gochecknoglobals // Its loaded once at startup.
	allTelegramNotificationTemplates map[NotificationType]map[languageCode]*telegramNotificationTemplate
	//nolint:gochecknoglobals // Its loaded once at startup.
	internationalizedEmailDisplayNames = map[string]string{
		"en": "ice: Decentralized Future",
	}
)

type (
	languageCode     = string
	telegramBotID    = string
	telegramBotToken = string
	telegramUserID   = string
	user             struct {
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
		Language                         string                          `json:"language,omitempty"`
		TelegramUserID                   string                          `json:"telegramUserId,omitempty"`
		TelegramBotID                    string                          `json:"telegramBotId,omitempty"`
		AgendaContactUserIDs             []string                        `json:"agendaContactUserIDs,omitempty" db:"agenda_contact_user_ids"`
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
	achievedBadgesSource struct {
		*processor
	}
	completedLevelsSource struct {
		*processor
	}
	enabledRolesSource struct {
		*processor
	}
	agendaContactsSource struct {
		*processor
	}
	miningSessionSource struct {
		*processor
	}
	repository struct {
		cfg                         *config
		shutdown                    func() error
		db                          *storage.DB
		freezerDB                   storagev3.DB
		mb                          messagebroker.Client
		pushNotificationsClient     push.Client
		telegramNotificationsClient telegram.Client
		emailClient                 email.Client
		pictureClient               picture.Client
		personalInAppFeed           inapp.Client
		globalInAppFeed             inapp.Client
	}
	processor struct {
		*repository
	}
	notificationDelayConfig struct {
		MinNotificationDelaySec uint `yaml:"minNotificationDelaySec"`
		MaxNotificationDelaySec uint `yaml:"maxNotificationDelaySec"`
	}
	scheduledNotificationInfo struct {
		TelegramUserID                  string
		TelegramBotID                   string
		Username                        string
		PushNotificationTokens          *users.Enum[push.DeviceToken]
		DisabledPushNotificationDomains *users.Enum[NotificationDomain]
		scheduledNotification
	}
	scheduledNotification struct {
		ScheduledAt              *time.Time  `json:"scheduledAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		ScheduledFor             *time.Time  `json:"scheduledFor,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		Data                     *users.JSON `json:"data,omitempty"`
		Language                 string      `json:"language,omitempty" example:"en"`
		UserID                   string      `json:"userId,omitempty" example:"edfd8c02-75e0-4687-9ac2-1ce4723865c4"`
		Uniqueness               string      `json:"uniqueness,omitempty" example:"anything"`
		NotificationType         string      `json:"notificationType,omitempty" example:"adoption_changed"`
		NotificationChannel      string      `json:"notificationChannel,omitempty" example:"email"`
		NotificationChannelValue string      `json:"notificationChannelValue,omitempty" example:"jdoe@example.com"`
		I                        int64       `json:"i" example:"1"`
	}
	scheduledAnnouncement struct {
		ScheduledAt              *time.Time  `json:"scheduledAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		ScheduledFor             *time.Time  `json:"scheduledFor,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		Data                     *users.JSON `json:"data,omitempty"`
		Language                 string      `json:"language,omitempty" example:"en"`
		Uniqueness               string      `json:"uniqueness,omitempty" example:"anything"`
		NotificationType         string      `json:"notificationType,omitempty" example:"adoption_changed"`
		NotificationChannel      string      `json:"notificationChannel,omitempty" example:"email"`
		NotificationChannelValue string      `json:"notificationChannelValue,omitempty" example:"jdoe@example.com"`
		I                        int64       `json:"i" example:"1"`
	}
	invalidToken struct {
		UserID string
		Token  push.DeviceToken
	}
	telegramNotification struct {
		tn        *telegram.Notification
		sn        *sentNotification
		scheduled scheduledNotification
	}
	telegramUserInfo struct {
		UserID         string `json:"userId,omitempty"`
		TelegramUserID string `json:"telegramUserId,omitempty"`
		Language       string `json:"language,omitempty"`
	}
	config struct {
		TenantName string `yaml:"tenantName"`
		TokenName  string `yaml:"tokenName"`
		Socials    []struct {
			NotificationType string `yaml:"notificationType"`
			Link             string `yaml:"link"`
		} `yaml:"socials"`
		DisabledAchievementsNotifications struct {
			Badges []string `yaml:"badges"`
			Levels []string `yaml:"levels"`
			Roles  []string `yaml:"roles"`
		} `yaml:"disabledAchievementsNotifications" `
		NotificationDelaysByTopic map[push.SubscriptionTopic]notificationDelayConfig `yaml:"notificationDelaysByTopic" mapstructure:"notificationDelaysByTopic"` //nolint:lll // .
		TelegramBots              map[telegramBotID]struct {
			BotToken telegramBotToken `yaml:"botToken"`
		} `yaml:"telegramBots" mapstructure:"telegramBots"`
		Telegram struct {
			BotToken telegramBotToken `yaml:"botToken"`
		} `yaml:"wintr/notifications/telegram" mapstructure:"wintr/notifications/telegram"` //nolint:tagliatelle // Nope.
		DeeplinkScheme          string                   `yaml:"deeplinkScheme"`
		WebAppLink              string                   `yaml:"webAppLink"`
		WebSiteURL              string                   `yaml:"webSiteUrl"`
		InviteURL               string                   `yaml:"inviteUrl"`
		messagebroker.Config    `mapstructure:",squash"` //nolint:tagliatelle // Nope.
		notificationDelayConfig `mapstructure:",squash"`
		PingCooldown            stdlibtime.Duration `yaml:"pingCooldown"`
		WeeklyStats             struct {
			Weekday stdlibtime.Weekday `yaml:"weekday"`
			Hour    int                `yaml:"hour"`
			Minutes int                `yaml:"minutes"`
		} `yaml:"weeklyStats"`
		Development bool `yaml:"development"`
	}
)

// SPDX-License-Identifier: ice License 1.0

package main

import (
	"mime/multipart"

	"github.com/ice-blockchain/husky/analytics"
	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/husky/notifications"
)

// Public API.

type (
	CreateNewsRequestBody struct {
		// Required.
		Image *multipart.FileHeader `form:"image" formMultipart:"image" swaggerignore:"true" required:"true"`
		// Required, if `news` param is not specified.
		NewsImportFile *multipart.FileHeader `form:"newsImportFile" formMultipart:"newsImportFile" swaggerignore:"true"`
		// Required, if `newsImportFile` param is not specified.
		News string `form:"news" formMultipart:"news"`
	}
	ModifyNewsRequestBody struct {
		MarkViewed *bool `form:"markViewed" formMultipart:"markViewed"`
		// Optional.
		Image *multipart.FileHeader `form:"image" formMultipart:"image" swaggerignore:"true"`
		// Optional. Example: `financial`.
		Tags *news.Tags `form:"tags" formMultipart:"tags"`
		// Optional. Example: any of `regular`, `featured`.
		Type news.Type `form:"type" formMultipart:"type" swaggertype:"string" enums:"regular,featured"`
		// Optional.
		Title string `form:"title" formMultipart:"title"`
		// Optional. Example: `https://somewebsite.com/blockchain`.
		URL      string `form:"url" formMultipart:"url"`
		NewsID   string `uri:"newsId" swaggerignore:"true" required:"true" example:"0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		Language string `uri:"language" swaggerignore:"true" required:"true" example:"en"`
		// Optional. Setting this will save you from race conditions. Example:`1232412415326543647657`.
		Checksum string `form:"checksum" formMultipart:"checksum"`
	}
	DeleteNewsArg struct {
		NewsID   string `uri:"newsId" swaggerignore:"true" required:"true" example:"0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		Language string `uri:"language" swaggerignore:"true" required:"true" example:"en"`
	}
	PingUserArg struct {
		UserID string `uri:"userId" allowForbiddenWriteOperation:"true" required:"true" swaggerignore:"true" example:"edfd8c02-75e0-4687-9ac2-1ce4723865c4"`
	}
	ToggleNotificationChannelDomainRequestBody struct {
		Enabled             *bool                             `json:"enabled" required:"true" example:"true"`
		Type                notifications.NotificationDomain  `uri:"type" example:"system"  swaggerignore:"true" required:"true" enums:"disable_all,weekly_report,weekly_stats,achievements,promotions,news,micro_community,mining,daily_bonus,system"` //nolint:lll // .
		NotificationChannel notifications.NotificationChannel `uri:"notificationChannel" example:"push" swaggerignore:"true" enums:"push,email" required:"true"`
	}
	News struct {
		*news.TaggedNews
		Checksum string `json:"checksum,omitempty" example:"1232412415326543647657"`
	}
	GenerateInAppNotificationsUserAuthTokenArg struct{}
)

// Private API.

const (
	applicationYamlKey = "cmd/husky-pack"
	swaggerRoot        = "/notifications/w"
)

// .
var (
	//nolint:gochecknoglobals // It's only read from during runtime.
	languages = map[string]struct{}{
		"aa":      {},
		"ab":      {},
		"ae":      {},
		"af":      {},
		"ak":      {},
		"am":      {},
		"an":      {},
		"ar":      {},
		"as":      {},
		"av":      {},
		"ay":      {},
		"az":      {},
		"ba":      {},
		"be":      {},
		"bg":      {},
		"bh":      {},
		"bi":      {},
		"bm":      {},
		"bn":      {},
		"bo":      {},
		"br":      {},
		"bs":      {},
		"ca":      {},
		"ce":      {},
		"ch":      {},
		"co":      {},
		"cr":      {},
		"cs":      {},
		"cu":      {},
		"cv":      {},
		"cy":      {},
		"da":      {},
		"de":      {},
		"dv":      {},
		"dz":      {},
		"ee":      {},
		"el":      {},
		"en":      {},
		"eo":      {},
		"es":      {},
		"et":      {},
		"eu":      {},
		"fa":      {},
		"ff":      {},
		"fi":      {},
		"fil":     {},
		"fj":      {},
		"fo":      {},
		"fr":      {},
		"fy":      {},
		"ga":      {},
		"gd":      {},
		"gl":      {},
		"gn":      {},
		"gu":      {},
		"gv":      {},
		"ha":      {},
		"he":      {},
		"hi":      {},
		"ho":      {},
		"hr":      {},
		"ht":      {},
		"hu":      {},
		"hy":      {},
		"hz":      {},
		"ia":      {},
		"id":      {},
		"ie":      {},
		"ig":      {},
		"ii":      {},
		"ik":      {},
		"io":      {},
		"is":      {},
		"it":      {},
		"iu":      {},
		"ja":      {},
		"jv":      {},
		"ka":      {},
		"kg":      {},
		"ki":      {},
		"kj":      {},
		"kk":      {},
		"kl":      {},
		"km":      {},
		"kn":      {},
		"ko":      {},
		"kr":      {},
		"ks":      {},
		"ku":      {},
		"kv":      {},
		"kw":      {},
		"ky":      {},
		"la":      {},
		"lb":      {},
		"lg":      {},
		"li":      {},
		"ln":      {},
		"lo":      {},
		"lt":      {},
		"lu":      {},
		"lv":      {},
		"mg":      {},
		"mh":      {},
		"mi":      {},
		"mk":      {},
		"ml":      {},
		"mn":      {},
		"mr":      {},
		"ms":      {},
		"mt":      {},
		"my":      {},
		"na":      {},
		"nb":      {},
		"nd":      {},
		"ne":      {},
		"ng":      {},
		"nl":      {},
		"nn":      {},
		"no":      {},
		"nr":      {},
		"nv":      {},
		"ny":      {},
		"oc":      {},
		"oj":      {},
		"om":      {},
		"or":      {},
		"os":      {},
		"pa":      {},
		"pi":      {},
		"pl":      {},
		"ps":      {},
		"pt":      {},
		"qu":      {},
		"rm":      {},
		"rn":      {},
		"ro":      {},
		"ru":      {},
		"rw":      {},
		"sa":      {},
		"sc":      {},
		"sd":      {},
		"se":      {},
		"sg":      {},
		"si":      {},
		"sk":      {},
		"sl":      {},
		"sm":      {},
		"sn":      {},
		"so":      {},
		"sq":      {},
		"sr":      {},
		"ss":      {},
		"st":      {},
		"su":      {},
		"sv":      {},
		"sw":      {},
		"ta":      {},
		"te":      {},
		"tg":      {},
		"th":      {},
		"ti":      {},
		"tk":      {},
		"tl":      {},
		"tn":      {},
		"to":      {},
		"tr":      {},
		"ts":      {},
		"tt":      {},
		"tw":      {},
		"ty":      {},
		"ug":      {},
		"uk":      {},
		"ur":      {},
		"uz":      {},
		"ve":      {},
		"vi":      {},
		"vo":      {},
		"wa":      {},
		"wo":      {},
		"xh":      {},
		"yi":      {},
		"yo":      {},
		"za":      {},
		"zh":      {},
		"zh-hans": {},
		"zh-hant": {},
		"zu":      {},
	}
)

// Values for server.ErrorResponse#Code.
const (
	duplicateNewsErrorCode     = "CONFLICT_WITH_ANOTHER_NEWS"
	alreadyViewedNewsErrorCode = "ALREADY_VIEWED_NEWS"
	raceConditionErrorCode     = "RACE_CONDITION"
	newsNotFoundErrorCode      = "NEWS_NOT_FOUND"
	userNotFoundErrorCode      = "USER_NOT_FOUND"
	userAlreadyPingedErrorCode = "USER_ALREADY_PINGED"
	invalidPropertiesErrorCode = "INVALID_PROPERTIES"
)

type (
	// | service implements server.State and is responsible for managing the state and lifecycle of the package.
	service struct {
		notificationsProcessor notifications.Processor
		newsProcessor          news.Processor
		analyticsProcessor     analytics.Processor
	}
	config struct {
		Host    string `yaml:"host"`
		Version string `yaml:"version"`
	}
)

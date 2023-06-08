// SPDX-License-Identifier: ice License 1.0

package main

import (
	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/husky/notifications"
)

// Public API.

type (
	GetNotificationChannelTogglesArg struct {
		NotificationChannel notifications.NotificationChannel `uri:"notificationChannel" example:"push" enums:"push,email" required:"true"`
	}
	GetNewsArg struct {
		// Default is `regular`.
		Type         news.Type `form:"type" example:"regular" enums:"regular,featured"`
		CreatedAfter string    `form:"createdAfter" example:"2022-01-03T16:20:52.156534Z"`
		Language     string    `uri:"language" example:"en" required:"true"`
		Limit        uint64    `form:"limit" maximum:"1000" example:"10"` // 10 by default.
		Offset       uint64    `form:"offset" example:"5"`
	}
	GetUnreadNewsCountArg struct {
		CreatedAfter string `form:"createdAfter" example:"2022-01-03T16:20:52.156534Z"`
		Language     string `uri:"language" example:"en" required:"true"`
	}
)

// Private API.

const (
	applicationYamlKey = "cmd/husky"
	swaggerRoot        = "/notifications/r"
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
	invalidPropertiesErrorCode = "INVALID_PROPERTIES"
)

type (
	// | service implements server.State and is responsible for managing the state and lifecycle of the package.
	service struct {
		newsRepository          news.Repository
		notificationsRepository notifications.Repository
		cfg                     *config
	}
	config struct {
		Host    string `yaml:"host"`
		Version string `yaml:"version"`
	}
)

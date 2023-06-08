// SPDX-License-Identifier: ice License 1.0

//go:build !test

package seeding

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	stdlibtime "time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/go-tarantool-client"
	"github.com/ice-blockchain/husky/news"
	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
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

func StartSeeding() {
	before := stdlibtime.Now()
	db := dbConnector()
	defer func() {
		log.Panic(db.Close()) //nolint:revive // It doesnt really matter.
		log.Info(fmt.Sprintf("seeding finalized in %v", stdlibtime.Since(before).String()))
	}()

	generateNews(db)
}

func dbConnector() tarantool.Connector {
	parts := strings.Split(os.Getenv("MASTER_DB_INSTANCE_ADDRESS"), "@")
	userAndPass := strings.Split(parts[0], ":")
	opts := tarantool.Opts{
		User: userAndPass[0],
		Pass: userAndPass[1],
	}
	db, err := tarantool.Connect(parts[1], opts)
	log.Panic(err)

	return db
}

func generateNews(db tarantool.Connector) { //nolint:funlen // .
	now := time.Now()
	for language := range languages {
		insertTag(db, now, language, fmt.Sprintf("testing%v", now))
	}
	const items = 100
	for ix := 0; ix < items; ix++ {
		nws := make([]*news.TaggedNews, 0, (items+1+1)*len(languages))
		if ix%(items/(1+1)) == 0 {
			id := uuid.NewString()
			for language := range languages {
				nws = append(nws, &news.TaggedNews{
					Tags: &news.Tags{fmt.Sprintf("testing%v", now)},
					News: &news.News{
						CreatedAt: now,
						UpdatedAt: now,
						ID:        id,
						Type:      news.FeaturedNewsType,
						Language:  language,
						Title:     fmt.Sprintf("[%[1]v]%[2]v%[2]v%[2]v%[2]v%[2]v%[2]v%[2]v%[2]v", language, id),
						ImageURL:  fmt.Sprintf("%v.jpeg", ix+1),
						URL:       fmt.Sprintf("https://www.google.com/search?q=ice.io&hl=%v&oq=ice.io&rnd=%v", language, id),
						Views:     rand.Uint64(), //nolint:gosec // .
					},
				})
			}
		}
		id := uuid.NewString()
		for language := range languages {
			nws = append(nws, &news.TaggedNews{
				Tags: &news.Tags{fmt.Sprintf("testing%v", now)},
				News: &news.News{
					CreatedAt: now,
					UpdatedAt: now,
					ID:        id,
					Type:      news.RegularNewsType,
					Language:  language,
					Title:     fmt.Sprintf("[%[1]v]%[2]v%[2]v%[2]v%[2]v%[2]v%[2]v%[2]v%[2]v", language, id),
					ImageURL:  fmt.Sprintf("%v.jpeg", ix+1),
					URL:       fmt.Sprintf("https://www.google.com/search?q=ice.io&hl=%v&oq=ice.io&rnd=%v", language, id),
					Views:     rand.Uint64(), //nolint:gosec // .
				},
			})
		}
		insertNews(db, nws...)
		addNewsTagsPerNews(db, nws...)
	}
}

func insertNews(db tarantool.Connector, taggedNews ...*news.TaggedNews) {
	const fields = 8
	params := make(map[string]any, len(taggedNews)*fields)
	values := make([]string, 0, len(taggedNews))
	for ix, nws := range taggedNews {
		params[fmt.Sprintf(`created_at%v`, ix)] = nws.CreatedAt
		params[fmt.Sprintf(`notification_channels%v`, ix)] = nws.NotificationChannels.NotificationChannels
		params[fmt.Sprintf(`id%v`, ix)] = nws.ID
		params[fmt.Sprintf(`type%v`, ix)] = nws.Type
		params[fmt.Sprintf(`language%v`, ix)] = nws.Language
		params[fmt.Sprintf(`title%v`, ix)] = nws.Title
		params[fmt.Sprintf(`image_url%v`, ix)] = nws.ImageURL
		params[fmt.Sprintf(`url%v`, ix)] = nws.URL
		params[fmt.Sprintf(`views%v`, ix)] = nws.Views
		values = append(values, fmt.Sprintf(`(:created_at%[1]v, :created_at%[1]v, :notification_channels%[1]v, :id%[1]v, :type%[1]v, :language%[1]v, :title%[1]v, :image_url%[1]v, :url%[1]v, :views%[1]v)`, ix)) //nolint:lll // .
	}
	log.Panic(storage.CheckSQLDMLErr(db.PrepareExecute(fmt.Sprintf(`INSERT INTO news (CREATED_AT, UPDATED_AT, NOTIFICATION_CHANNELS, ID, TYPE, LANGUAGE, TITLE, IMAGE_URL, URL, VIEWS) VALUES %v`, strings.Join(values, ",")), params))) //nolint:lll // .
}

func insertTag(db tarantool.Connector, createdAt *time.Time, language string, tag news.Tag) {
	type newsTag struct {
		_msgpack  struct{}   `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase,structcheck // To insert we need asArray
		CreatedAt *time.Time `json:"createdAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		Language  string     `json:"language" example:"en"`
		Value     string     `json:"value" example:"cats"`
	}
	tuple := &newsTag{
		CreatedAt: createdAt,
		Language:  language,
		Value:     tag,
	}
	if err := storage.CheckNoSQLDMLErr(db.InsertTyped("NEWS_TAGS", tuple, &[]*newsTag{})); err != nil && !errors.Is(err, storage.ErrDuplicate) {
		log.Panic(errors.Wrapf(err, "failed to insert tag:%#v", tuple))
	}
}

func addNewsTagsPerNews(db tarantool.Connector, taggedNews ...*news.TaggedNews) {
	const fields, estimatedTags = 4, 10
	params := make(map[string]any, len(taggedNews)*estimatedTags*fields)
	values := make([]string, 0, len(taggedNews)*estimatedTags)
	for ix, nws := range taggedNews {
		if nws.Tags == nil {
			continue
		}
		for j, tag := range *nws.Tags {
			params[fmt.Sprintf(`created_at%v_%v`, ix, j)] = nws.CreatedAt
			params[fmt.Sprintf(`news_id%v_%v`, ix, j)] = nws.ID
			params[fmt.Sprintf(`language%v_%v`, ix, j)] = nws.Language
			params[fmt.Sprintf(`news_tag%v_%v`, ix, j)] = tag
			values = append(values, fmt.Sprintf(`(:created_at%[1]v_%[2]v, :news_id%[1]v_%[2]v, :language%[1]v_%[2]v, :news_tag%[1]v_%[2]v)`, ix, j))
		}
	}
	if len(values) == 0 {
		return
	}
	sql := fmt.Sprintf(`INSERT INTO news_tags_per_news (CREATED_AT, NEWS_ID, LANGUAGE, NEWS_TAG) VALUES %v`, strings.Join(values, ","))

	log.Panic(errors.Wrapf(storage.CheckSQLDMLErr(db.PrepareExecute(sql, params)), "failed to insert news_tags_per_news for news:%#v", taggedNews))
}

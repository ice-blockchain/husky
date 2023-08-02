-- SPDX-License-Identifier: ice License 1.0
-- news
CREATE TABLE IF NOT EXISTS news (
                    created_at            TIMESTAMP NOT NULL,
                    updated_at            TIMESTAMP NOT NULL,
                    views                 BIGINT NOT NULL DEFAULT 0,
                    notification_channels TEXT[],
                    id                    TEXT NOT NULL,
                    type                  TEXT NOT NULL,
                    language              TEXT NOT NULL,
                    title                 TEXT NOT NULL,
                    image_url             TEXT NOT NULL,
                    url                   TEXT NOT NULL,
                    PRIMARY KEY(language,id)
                    );
CREATE INDEX IF NOT EXISTS most_recent_news_lookup_ix ON news (language, type, created_at DESC);
ALTER TABLE news DROP CONSTRAINT IF EXISTS news_url_key;
CREATE UNIQUE INDEX IF NOT EXISTS news_url_language_ix ON news (url,language);
-- news_viewed_by_users
CREATE TABLE IF NOT EXISTS news_viewed_by_users (
                   created_at TIMESTAMP NOT NULL,
                   news_id    TEXT NOT NULL,
                   language   TEXT NOT NULL,
                   user_id    TEXT NOT NULL,
                   PRIMARY KEY(language,news_id,user_id),
                   FOREIGN KEY(language,news_id) REFERENCES news(language,id) ON DELETE CASCADE
                   );
-- news_tags
CREATE TABLE IF NOT EXISTS news_tags  (
                   created_at TIMESTAMP NOT NULL,
                   language   TEXT NOT NULL,
                   value      TEXT NOT NULL,
                   PRIMARY KEY(language,value)
                   );
-- news_tags_per_news
CREATE TABLE IF NOT EXISTS news_tags_per_news  (
                   created_at  TIMESTAMP NOT NULL,
                   news_id     TEXT NOT NULL,
                   language    TEXT NOT NULL,
                   news_tag    TEXT NOT NULL,
                   PRIMARY KEY(language, news_id, news_tag),
                   FOREIGN KEY(language, news_id) REFERENCES news(language,id) ON DELETE CASCADE,
                   FOREIGN KEY(language, news_tag) REFERENCES news_tags(language,value) ON DELETE CASCADE
                   );

-- aggregated news views across languages
CREATE MATERIALIZED VIEW IF NOT EXISTS news_views (
                                 news_id, views
                            ) AS (
                                SELECT id, COALESCE(SUM(views),0) as views
                                FROM news
                                GROUP BY id
                            );
CREATE UNIQUE INDEX ON news_views (news_id);
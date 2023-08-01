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

CREATE MATERIALIZED VIEW IF NOT EXISTS news_views_by_language (
                                 news_id, language, views
                            ) AS (
                                SELECT news_id, language, COALESCE(COUNT(*),0) as views
                                FROM news_viewed_by_users
                                GROUP BY news_id, language
                            );

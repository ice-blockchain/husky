-- SPDX-License-Identifier: ice License 1.0
-- news
CREATE TABLE IF NOT EXISTS news (
                    created_at            TIMESTAMP NOT NULL,
                    updated_at            TIMESTAMP NOT NULL,
                    notification_channels TEXT,
                    id                    TEXT NOT NULL,
                    type                  TEXT NOT NULL,
                    language              TEXT NOT NULL,
                    title                 TEXT NOT NULL,
                    image_url             TEXT NOT NULL,
                    url                   TEXT NOT NULL UNIQUE,
                    views                 BIGINT NOT NULL DEFAULT 0,
                    PRIMARY KEY(language,id)
                    );
CREATE INDEX IF NOT EXISTS most_recent_news_lookup_ix ON news (language, type, created_at);
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
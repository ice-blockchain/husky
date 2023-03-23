-- SPDX-License-Identifier: ice License 1.0
-- news
box.execute([[CREATE TABLE IF NOT EXISTS news (
                    created_at            UNSIGNED NOT NULL,
                    updated_at            UNSIGNED NOT NULL,
                    notification_channels STRING,
                    id                    STRING NOT NULL,
                    type                  STRING NOT NULL,
                    language              STRING NOT NULL,
                    title                 STRING NOT NULL,
                    image_url             STRING NOT NULL,
                    url                   STRING NOT NULL UNIQUE,
                    views                 UNSIGNED NOT NULL DEFAULT 0,
                    PRIMARY KEY(language,id)
                    ) WITH ENGINE = 'memtx';]])
box.execute([[CREATE INDEX IF NOT EXISTS most_recent_news_lookup_ix ON news (language, type, created_at);]])
-- news_viewed_by_users
box.execute([[CREATE TABLE IF NOT EXISTS news_viewed_by_users (
                   created_at UNSIGNED NOT NULL,
                   news_id    STRING NOT NULL,
                   language   STRING NOT NULL,
                   user_id    STRING NOT NULL,
                   PRIMARY KEY(language,news_id,user_id),
                   FOREIGN KEY(language,news_id) REFERENCES news(language,id) ON DELETE CASCADE
                   ) WITH ENGINE = 'memtx';]])
-- news_tags
box.execute([[CREATE TABLE IF NOT EXISTS news_tags  (
                   created_at UNSIGNED NOT NULL,
                   language   STRING NOT NULL,
                   value      STRING NOT NULL,
                   PRIMARY KEY(language,value)
                   ) WITH ENGINE = 'memtx';]])
-- news_tags_per_news
box.execute([[CREATE TABLE IF NOT EXISTS news_tags_per_news  (
                   created_at  UNSIGNED NOT NULL,
                   news_id     STRING NOT NULL,
                   language    STRING NOT NULL,
                   news_tag    STRING NOT NULL,
                   PRIMARY KEY(language, news_id, news_tag),
                   FOREIGN KEY(language, news_id) REFERENCES news(language,id) ON DELETE CASCADE,
                   FOREIGN KEY(language, news_tag) REFERENCES news_tags(language,value) ON DELETE CASCADE
                   ) WITH ENGINE = 'memtx';]])
-- SPDX-License-Identifier: ice License 1.0
--************************************************************************************************************************************
-- users
box.execute([[CREATE TABLE IF NOT EXISTS users  (
                    last_ping_cooldown_ended_at             UNSIGNED,
                    disabled_push_notification_domains      STRING,
                    disabled_email_notification_domains     STRING,
                    disabled_sms_notification_domains       STRING,
                    phone_number                            STRING,
                    email                                   STRING,
                    first_name                              STRING,
                    last_name                               STRING,
                    user_id                                 STRING NOT NULL primary key,
                    username                                STRING,
                    profile_picture_name                    STRING,
                    referred_by                             STRING,
                    phone_number_hash                       STRING,
                    agenda_phone_number_hashes              STRING,
                    language                                STRING NOT NULL default 'en'
                  ) WITH ENGINE = 'memtx';]])
box.execute([[CREATE INDEX IF NOT EXISTS users_agenda_phone_number_hashes_ix ON users (agenda_phone_number_hashes);]])
--************************************************************************************************************************************
-- sent_notifications
box.execute([[CREATE TABLE IF NOT EXISTS sent_notifications  (
                    sent_at                     UNSIGNED NOT NULL,
                    language                    STRING NOT NULL,
                    user_id                     STRING NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
                    uniqueness                  STRING NOT NULL,
                    notification_type           STRING NOT NULL,
                    notification_channel        STRING NOT NULL,
                    notification_channel_value  STRING NOT NULL,
                    primary key(user_id,uniqueness,notification_type,notification_channel,notification_channel_value)) WITH ENGINE = 'memtx';]])
box.execute([[CREATE INDEX IF NOT EXISTS sent_notifications_sent_at_ix ON sent_notifications (sent_at);]])
--************************************************************************************************************************************
-- sent_announcements
box.execute([[CREATE TABLE IF NOT EXISTS sent_announcements (
                    sent_at                         UNSIGNED NOT NULL,
                    language                        STRING NOT NULL,
                    uniqueness                      STRING NOT NULL,
                    notification_type               STRING NOT NULL,
                    notification_channel            STRING NOT NULL,
                    notification_channel_value      STRING NOT NULL,
                    primary key(uniqueness,notification_type,notification_channel,notification_channel_value)) WITH ENGINE = 'memtx';]])
box.execute([[CREATE INDEX IF NOT EXISTS sent_announcements_sent_at_ix ON sent_announcements (sent_at);]])
--************************************************************************************************************************************
-- device_metadata
box.execute([[CREATE TABLE IF NOT EXISTS device_metadata (
                    user_id                     STRING NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
                    device_unique_id            STRING NOT NULL,
                    push_notification_token     STRING,
                    primary key(user_id, device_unique_id)) WITH ENGINE = 'memtx';]])
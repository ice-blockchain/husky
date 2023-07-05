-- SPDX-License-Identifier: ice License 1.0
--************************************************************************************************************************************
-- users
CREATE TABLE IF NOT EXISTS users  (
                    last_ping_cooldown_ended_at             TIMESTAMP,
                    disabled_push_notification_domains      TEXT[],
                    disabled_email_notification_domains     TEXT[],
                    disabled_sms_notification_domains       TEXT[],
                    agenda_contact_user_ids                 TEXT[],
                    phone_number                            TEXT,
                    email                                   TEXT,
                    first_name                              TEXT,
                    last_name                               TEXT,
                    user_id                                 TEXT NOT NULL primary key,
                    username                                TEXT,
                    profile_picture_name                    TEXT,
                    referred_by                             TEXT,
                    phone_number_hash                       TEXT,
                    language                                TEXT NOT NULL default 'en'
                  );
--************************************************************************************************************************************
-- sent_notifications
CREATE TABLE IF NOT EXISTS sent_notifications  (
                    sent_at                     TIMESTAMP NOT NULL,
                    language                    TEXT NOT NULL,
                    user_id                     TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
                    uniqueness                  TEXT NOT NULL,
                    notification_type           TEXT NOT NULL,
                    notification_channel        TEXT NOT NULL,
                    notification_channel_value  TEXT NOT NULL,
                    primary key(user_id,uniqueness,notification_type,notification_channel,notification_channel_value));
CREATE INDEX IF NOT EXISTS sent_notifications_sent_at_ix ON sent_notifications (sent_at);
--************************************************************************************************************************************
-- sent_announcements
CREATE TABLE IF NOT EXISTS sent_announcements (
                    sent_at                         TIMESTAMP NOT NULL,
                    language                        TEXT NOT NULL,
                    uniqueness                      TEXT NOT NULL,
                    notification_type               TEXT NOT NULL,
                    notification_channel            TEXT NOT NULL,
                    notification_channel_value      TEXT NOT NULL,
                    primary key(uniqueness,notification_type,notification_channel,notification_channel_value));
CREATE INDEX IF NOT EXISTS sent_announcements_sent_at_ix ON sent_announcements (sent_at);
--************************************************************************************************************************************
-- device_metadata
CREATE TABLE IF NOT EXISTS device_metadata (
                    user_id                     TEXT NOT NULL,
                    device_unique_id            TEXT NOT NULL,
                    push_notification_token     TEXT,
                    primary key(user_id, device_unique_id));
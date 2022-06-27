-- SPDX-License-Identifier: BUSL-1.1
box.execute([[CREATE TABLE IF NOT EXISTS users  (
                    created_at UNSIGNED NOT NULL,
                    updated_at UNSIGNED NOT NULL,
                    last_ping_sent_at UNSIGNED DEFAULT 0,
                    last_mining_started_at UNSIGNED DEFAULT 0,
                    user_id STRING primary key,
                    referred_by STRING NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
                    username STRING NOT NULL UNIQUE,
                    first_name STRING,
                    last_name STRING,
                    phone_number STRING,
                    email STRING,
                    profile_picture_name STRING NOT NULL) WITH ENGINE = 'vinyl';]])
box.execute([[CREATE TABLE IF NOT EXISTS notification_types (name STRING primary key, description STRING NOT NULL) WITH ENGINE = 'vinyl';]])
box.execute([[INSERT INTO notification_types (name, description)
                           VALUES ('T1', 'If the t0 referral pinged the user.'),
                                  ('T2', 'If mining has been finished for the previous 24 hours: `Start Mining / Don’t forget to start your miner for today.`'),
                                  ('T3', 'If miner has not been active for at least 48 hours: `Start Mining / For each day you don’t mine you are loosing ICE coins.`'),
                                  ('T4', 'If 7/14/30 days have passed and miner has not been active: `We Miss You / Hey there, don’t forget that you need to start mining on a daily basis.`'),
                                  ('T5', 'After 1 hour/7 days since the account has been created: `Invite Friends / Earn 25% more ICE from your referred friends.`'),
                                  ('T6', 'Weekly mining stats for active miners: `Weekly Mining Stats / Check your weekly mining stats`.'),
                                  ('T7', '1 Day after registration: `Join our Telegram / Come join us on Telegram and be one of the first to see updates.`'),
                                  ('T8', '2 Days after registration: `Follow us on Twitter / Join our Twitter community and stay updated!`'),
                                  ('T9', '3 Days after registration: `Follow us on Instagram / Join our Instagram community and stay updated!`'),
                                  ('T10', '4 Days after registration: `Follow us on Facebook / Join our Facebook community and stay updated!`'),
                                  ('T11', '5 Days after registration: `Follow us on Linkedin / Join our Linkedin community and stay updated!`'),
                                  ('T12', '6 Days after registration: `Follow us on Youtube / Join our Youtube community and stay updated!`')
           ]])
box.execute([[CREATE TABLE IF NOT EXISTS sent_notifications_history  (
                    sent_at UNSIGNED NOT NULL,
                    user_id STRING NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
                    notification_type STRING NOT NULL REFERENCES notification_types(name),
                    notification_channel STRING NOT NULL CHECK (lower(notification_channel) == 'push' or
                                                                lower(notification_channel) == 'in_app' or
                                                                lower(notification_channel) == 'email' or
                                                                lower(notification_channel) == 'sms'),
                    primary key(user_id, notification_type, notification_channel)) WITH ENGINE = 'vinyl';]])
box.execute([[CREATE TABLE IF NOT EXISTS user_device_settings  (
                    updated_at                  UNSIGNED NOT NULL,
                    user_id                     STRING NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
                    device_unique_id            STRING NOT NULL,
                    language                    STRING NOT NULL DEFAULT 'en',
                    notification_channels       STRING,
                    push_notification_token     STRING,
                    primary key(user_id, device_unique_id)) WITH ENGINE = 'vinyl';]])
-- TODO will add indexes later on

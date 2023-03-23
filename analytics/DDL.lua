-- SPDX-License-Identifier: ice License 1.0
--************************************************************************************************************************************
-- tracked_actions
box.execute([[CREATE TABLE IF NOT EXISTS tracked_actions (
                    sent_at    UNSIGNED NOT NULL,
                    action_id  STRING NOT NULL PRIMARY KEY
                    ) WITH ENGINE = 'memtx';]])
box.execute([[CREATE INDEX IF NOT EXISTS tracked_actions_sent_at_ix ON tracked_actions (sent_at);]])
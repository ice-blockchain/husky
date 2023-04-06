-- SPDX-License-Identifier: ice License 1.0
--************************************************************************************************************************************
-- tracked_actions
CREATE TABLE IF NOT EXISTS tracked_actions (
                    sent_at    TIMESTAMP NOT NULL,
                    action_id  TEXT NOT NULL PRIMARY KEY
                    );
CREATE INDEX IF NOT EXISTS tracked_actions_sent_at_ix ON tracked_actions (sent_at);
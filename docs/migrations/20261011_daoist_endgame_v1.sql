-- Apply once to an existing database after a backup; safely repeatable.
BEGIN;
-- Daoist endgame v1. Additive; native inventories and durable UIDs are unchanged.
CREATE TABLE IF NOT EXISTS equipment_endgame_state (
 uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
 revision BIGINT NOT NULL CHECK(revision >= 0),
 unique_id INTEGER NOT NULL CHECK(unique_id > 0),
 unique_roll_bp INTEGER NOT NULL CHECK(unique_roll_bp BETWEEN 8000 AND 12000)
);
CREATE TABLE IF NOT EXISTS character_endgame_state (
 char_id BIGINT PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
 revision BIGINT NOT NULL CHECK(revision > 0),
 payload JSONB NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS endgame_receipts (
 char_id BIGINT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL CHECK(length(request_id) BETWEEN 16 AND 64),
 operation TEXT NOT NULL,
 digest TEXT NOT NULL CHECK(length(digest)=64),
 revision BIGINT NOT NULL CHECK(revision > 0),
 response JSONB NOT NULL,
 PRIMARY KEY(char_id,request_id)
);
CREATE TABLE IF NOT EXISTS endgame_runs (
 run_id TEXT PRIMARY KEY CHECK(length(run_id) BETWEEN 1 AND 64),
 revision BIGINT NOT NULL CHECK(revision > 0),
 state TEXT NOT NULL,
 payload JSONB NOT NULL
);
ALTER TABLE characters ADD COLUMN IF NOT EXISTS trial_key_reward INTEGER NOT NULL DEFAULT 0 CHECK(trial_key_reward BETWEEN 0 AND 14);
ALTER TABLE characters ADD COLUMN IF NOT EXISTS trial_completion_id TEXT NOT NULL DEFAULT '' CHECK(length(trial_completion_id)<=64);
COMMIT;

BEGIN;
-- Persist daily spirit objectives and the shared per-character claim day.
ALTER TABLE character_quests ADD COLUMN IF NOT EXISTS spirit_daily jsonb NOT NULL DEFAULT '{}'::jsonb;
COMMIT;

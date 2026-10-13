-- V5 proc cooldowns survive equipment changes, transfers and reconnects.
ALTER TABLE characters ADD COLUMN IF NOT EXISTS equipment_proc_cooldowns jsonb NOT NULL DEFAULT '{}'::jsonb;

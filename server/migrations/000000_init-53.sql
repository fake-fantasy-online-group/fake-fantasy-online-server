-- V5 catalogs are opt-in for newly created/awakened instances only.
-- Existing rows and items with no overlay remain explicitly frozen at V4 (0).
ALTER TABLE equipment_endgame_state ADD COLUMN IF NOT EXISTS unique_content_version integer NOT NULL DEFAULT 0 CHECK (unique_content_version IN (0,5));
ALTER TABLE equipment_endgame_state ADD COLUMN IF NOT EXISTS formation_content_version integer NOT NULL DEFAULT 0 CHECK (formation_content_version IN (0,5));

-- V6 admits new equipment while preserving every existing V4/V5 row and roll.
-- Do not rewrite the deployed V5 migration or upgrade existing item versions.
ALTER TABLE equipment_endgame_state
    DROP CONSTRAINT IF EXISTS equipment_endgame_state_unique_content_version_check;
ALTER TABLE equipment_endgame_state
    ADD CONSTRAINT equipment_endgame_state_unique_content_version_check
    CHECK (unique_content_version IN (0, 5, 6));
ALTER TABLE equipment_endgame_state
    DROP CONSTRAINT IF EXISTS equipment_endgame_state_formation_content_version_check;
ALTER TABLE equipment_endgame_state
    ADD CONSTRAINT equipment_endgame_state_formation_content_version_check
    CHECK (formation_content_version IN (0, 5, 6));

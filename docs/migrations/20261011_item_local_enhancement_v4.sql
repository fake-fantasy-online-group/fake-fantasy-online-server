-- Stop the server and back up before this repeatable upgrade.
BEGIN;
-- Item-local enhancement v4. Preserve all existing UIDs, revisions and raw rolls.
-- Zero means a legacy fixed-midpoint formation; it is never lazily rerolled.
ALTER TABLE equipment_endgame_state
 ADD COLUMN IF NOT EXISTS formation_roll_bp INTEGER NOT NULL DEFAULT 0;
ALTER TABLE equipment_endgame_state
 DROP CONSTRAINT IF EXISTS equipment_endgame_state_formation_roll_bp_check;
ALTER TABLE equipment_endgame_state
 ADD CONSTRAINT equipment_endgame_state_formation_roll_bp_check
 CHECK (formation_roll_bp = 0 OR formation_roll_bp BETWEEN 8000 AND 12000);
COMMIT;

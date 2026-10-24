-- Existing databases: apply atomically; do not rerun fresh init.
BEGIN;
-- Preserve immutable monster provenance for new equipment, including ordinary
-- drops later awakened in the cube. Existing items remain zero and are not rerolled.
ALTER TABLE equipment_endgame_state ADD COLUMN IF NOT EXISTS source_level INTEGER NOT NULL DEFAULT 0;
DO $equipment_source_level$
BEGIN
 IF NOT EXISTS (SELECT FROM pg_attribute a WHERE a.attrelid='equipment_endgame_state'::regclass
  AND a.attname='source_level' AND NOT a.attisdropped AND a.atttypid='integer'::regtype
  AND a.attnotnull AND a.atthasdef
  AND pg_get_expr((SELECT adbin FROM pg_attrdef WHERE adrelid=a.attrelid AND adnum=a.attnum),a.attrelid)='0') THEN
  RAISE EXCEPTION 'equipment source level requires integer, not null, default zero';
 END IF;
 IF NOT EXISTS (SELECT FROM pg_constraint WHERE conrelid='equipment_endgame_state'::regclass
  AND conname='equipment_endgame_source_level_nonnegative') THEN
  ALTER TABLE equipment_endgame_state ADD CONSTRAINT equipment_endgame_source_level_nonnegative CHECK (source_level >= 0);
 END IF;
 IF NOT EXISTS (SELECT FROM pg_constraint WHERE conrelid='equipment_endgame_state'::regclass
  AND conname='equipment_endgame_source_level_nonnegative' AND contype='c' AND convalidated
  AND pg_get_expr(conbin,conrelid)='(source_level >= 0)') THEN
  RAISE EXCEPTION 'equipment source level requires validated nonnegative constraint';
 END IF;
END $equipment_source_level$;
COMMIT;

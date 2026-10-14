-- Retire four Zhou Tian Ding operations without deleting definitions, material
-- rows, saved equipment, socket cards, formations or historical receipts.
ALTER TABLE game_cube_recipes ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
DO $migration$
BEGIN
 IF (SELECT count(*) FROM game_cube_recipes WHERE id IN (1007,1008,1009,1012)) <> 4 THEN
  RAISE EXCEPTION 'cube retirement requires all four historical recipes';
 END IF;
 UPDATE game_cube_recipes SET enabled=FALSE WHERE id IN (1007,1008,1009,1012);
END $migration$;

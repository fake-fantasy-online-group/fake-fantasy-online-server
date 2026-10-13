-- New admissions use exactly twice the v2 normal/blue/yellow population.
-- Version 1 remains immutable, including the zero/missing serialized field.
-- Already admitted runs and saved floor plans retain their density and seed.
ALTER TABLE game_rift_rule_versions ADD COLUMN IF NOT EXISTS floor_population_multiplier INTEGER NOT NULL DEFAULT 0 CHECK (floor_population_multiplier BETWEEN 0 AND 2);
DO $density$
DECLARE original JSONB; doubled JSONB;
BEGIN
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r WHERE version=1 AND published;
 IF original IS NULL THEN RAISE EXCEPTION 'density upgrade requires published rift rules version 1'; END IF;
 doubled := original || '{"version":2,"published":false,"floor_population_multiplier":2}'::jsonb;
 IF NOT EXISTS (SELECT FROM game_rift_rule_versions WHERE version=2) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions, doubled)).*;
  INSERT INTO game_rift_class_rules SELECT 2,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=1;
  INSERT INTO game_rift_ability_rules SELECT 2,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=1;
  UPDATE game_rift_rule_versions SET published=true WHERE version=2;
 ELSE
  IF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=2) IS DISTINCT FROM (doubled || '{"published":true}'::jsonb) THEN
   RAISE EXCEPTION 'rift rules version 2 is already occupied by different configuration';
  END IF;
 END IF;
 IF EXISTS ((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=1 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=2) UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=2 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=1)) OR
 EXISTS ((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=1 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=2) UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=2 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=1)) THEN
  RAISE EXCEPTION 'rift density version 2 must preserve class and ability rules';
 END IF;
 -- Do not overwrite an operator's later active version when replaying this file.
 UPDATE game_rift_rule_active SET version=2 WHERE id=1 AND version=1;
END $density$;

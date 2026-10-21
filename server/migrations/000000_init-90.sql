-- New admissions only: connected combat-region density and payer guardian rotation.
-- Deploy the matching server before this migration. No client assets/protocol fields
-- are added. v1-v7 and already saved plans/reward snapshots remain authoritative.
ALTER TABLE game_rift_rule_versions
 ADD COLUMN IF NOT EXISTS floor_layout_version INTEGER NOT NULL DEFAULT 0 CHECK (floor_layout_version IN (0,4)),
 ADD COLUMN IF NOT EXISTS guardian_recent_window INTEGER NOT NULL DEFAULT 0 CHECK (guardian_recent_window IN (0,3));
DO $rift_coverage$
DECLARE active_version INTEGER; original JSONB; revised JSONB;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL OR active_version<7 THEN
  RAISE EXCEPTION 'rift coverage rollout requires active rift7';
 END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules IN SHARE MODE;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
  WHERE version=7 AND published AND equipment_pools IS NOT NULL AND elite_rules IS NOT NULL
    AND floor_layout_version=0 AND guardian_recent_window=0;
 IF original IS NULL THEN RAISE EXCEPTION 'rift coverage requires published legacy rift7'; END IF;
 revised := original || '{"version":8,"published":false,"floor_layout_version":4,"guardian_recent_window":3}'::jsonb;
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=8) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 8,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=7;
  INSERT INTO game_rift_ability_rules SELECT 8,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=7;
  UPDATE game_rift_rule_versions SET published=true WHERE version=8;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=8) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift8 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=7 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=8)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=8 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=7)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=7 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=8)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=8 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=7)) THEN
  RAISE EXCEPTION 'rift8 must preserve class and ability dependencies';
 END IF;
 -- Replaying must never roll an operator's newer selector backwards.
 UPDATE game_rift_rule_active SET version=8 WHERE id=1 AND version=7;
END $rift_coverage$;

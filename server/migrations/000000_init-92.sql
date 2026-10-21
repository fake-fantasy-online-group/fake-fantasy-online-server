-- New admissions only: v5 density = min(320, ceil(v4 population * 1.5)).
-- Existing clients remain compatible; no aura, clone, reward, or progress budget changes.
-- Deploy the matching server before activation. Previously admitted v8 runs keep v4.
ALTER TABLE game_rift_rule_versions DROP CONSTRAINT IF EXISTS game_rift_rule_versions_floor_layout_version_check;
ALTER TABLE game_rift_rule_versions ADD CONSTRAINT game_rift_rule_versions_floor_layout_version_check CHECK (floor_layout_version IN (0,4,5));
DO $rift_density_150$
DECLARE active_version INTEGER; original JSONB; revised JSONB;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL OR active_version<8 THEN
  RAISE EXCEPTION 'rift density rollout requires active rift8';
 END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules IN SHARE MODE;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
  WHERE version=8 AND published AND equipment_pools IS NOT NULL AND elite_rules IS NOT NULL
    AND floor_layout_version=4 AND guardian_recent_window=3;
 IF original IS NULL THEN RAISE EXCEPTION 'rift density requires published rift8'; END IF;
 revised := original || '{"version":9,"published":false,"floor_layout_version":5,"guardian_recent_window":3}'::jsonb;
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=9) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 9,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=8;
  INSERT INTO game_rift_ability_rules SELECT 9,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=8;
  UPDATE game_rift_rule_versions SET published=true WHERE version=9;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=9) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift9 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=8 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=9)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=9 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=8)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=8 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=9)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=9 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=8)) THEN
  RAISE EXCEPTION 'rift9 must preserve class and ability dependencies';
 END IF;
 -- Replaying must never roll an operator's newer selector backwards.
 UPDATE game_rift_rule_active SET version=9 WHERE id=1 AND version=8;
END $rift_density_150$;

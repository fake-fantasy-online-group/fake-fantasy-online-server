-- Publish immutable v10 / generator6, but deliberately retain the active selector.
-- Deploy the matching paging-capable client before separately activating v10.
-- Old 1.0.1 cannot join v10; server admission rejects it before charging a key.
-- Existing paid runs retain their frozen rule/generator versions.
ALTER TABLE game_rift_rule_versions DROP CONSTRAINT IF EXISTS game_rift_rule_versions_floor_layout_version_check;
ALTER TABLE game_rift_rule_versions ADD CONSTRAINT game_rift_rule_versions_floor_layout_version_check CHECK (floor_layout_version IN (0,4,5,6));
DO $rift_full_coverage$
DECLARE active_version INTEGER; original JSONB; revised JSONB;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL OR active_version<9 THEN
  RAISE EXCEPTION 'rift density rollout requires active rift9';
 END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules IN SHARE MODE;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
  WHERE version=9 AND published AND equipment_pools IS NOT NULL AND elite_rules IS NOT NULL
    AND floor_layout_version=5 AND guardian_recent_window=3;
 IF original IS NULL THEN RAISE EXCEPTION 'rift density requires published rift9'; END IF;
 revised := original || '{"version":10,"published":false,"floor_layout_version":6,"guardian_recent_window":3}'::jsonb;
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=10) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 10,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=9;
  INSERT INTO game_rift_ability_rules SELECT 10,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=9;
  UPDATE game_rift_rule_versions SET published=true WHERE version=10;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=10) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift10 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=9 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=10)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=10 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=9)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=9 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=10)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=10 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=9)) THEN
  RAISE EXCEPTION 'rift10 must preserve class and ability dependencies';
 END IF;
 -- Replaying must never roll an operator's newer selector backwards.
END $rift_full_coverage$;

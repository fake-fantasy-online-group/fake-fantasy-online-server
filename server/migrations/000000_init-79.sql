-- New admissions use a durable five-second guardian arrival. Historical rift
-- versions retain immediate guardians and their exact reward/scaling snapshots.
ALTER TABLE game_rift_rule_versions
 ADD COLUMN IF NOT EXISTS guardian_arrival_ms BIGINT NOT NULL DEFAULT 0 CHECK (guardian_arrival_ms IN (0,5000));

DO $rift_arrival$
DECLARE original JSONB; revised JSONB;
BEGIN
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
 WHERE version=4 AND published AND loot_version=2 AND floor_population_multiplier=2
   AND stat_anchor_tier=30 AND normal_boss_equipment=10 AND completion_level_experience
   AND guardian_arrival_ms=0;
 IF original IS NULL THEN
  RAISE EXCEPTION 'rift arrival requires published rebalanced rift4';
 END IF;
 revised := original || jsonb_build_object('version',5,'published',false,'guardian_arrival_ms',5000,'exit_name','归元道使');
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=5) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 5,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=4;
  INSERT INTO game_rift_ability_rules SELECT 5,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=4;
  UPDATE game_rift_rule_versions SET published=true WHERE version=5;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=5) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift rules version 5 is already occupied by different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=4 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=5)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=5 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=4)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=4 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=5)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=5 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=4)) THEN
  RAISE EXCEPTION 'rift rules version 5 must preserve class and ability rules';
 END IF;
 UPDATE game_rift_rule_active SET version=5 WHERE id=1 AND version=4;
END $rift_arrival$;

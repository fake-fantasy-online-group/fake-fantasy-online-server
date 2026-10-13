-- Publish new-run rift4 only. Historical rules/snapshots remain byte-compatible:
-- zero anchor means tier1 and zero normal_boss_equipment means no extra slots.
-- The level-specific trial base, class/party multipliers, growth rates, special
-- gear chances/guarantee, rune/dust quantities and weighted rune pool are unchanged.
ALTER TABLE game_rift_rule_versions
 ADD COLUMN IF NOT EXISTS completion_level_experience BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN IF NOT EXISTS stat_anchor_tier INTEGER NOT NULL DEFAULT 0 CHECK (stat_anchor_tier BETWEEN 0 AND max_tier),
 ADD COLUMN IF NOT EXISTS normal_boss_equipment INTEGER NOT NULL DEFAULT 0 CHECK (normal_boss_equipment BETWEEN 0 AND 20);

DO $rift_rebalance$
DECLARE original JSONB; revised JSONB;
BEGIN
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
 WHERE version=3 AND published AND loot_version=2 AND floor_population_multiplier=2
   AND stat_anchor_tier=0 AND normal_boss_equipment=0 AND NOT completion_level_experience;
 IF original IS NULL THEN
  RAISE EXCEPTION 'rift rebalance requires published weighted-rune rift3';
 END IF;
 revised := original || jsonb_build_object(
  'version',4,'published',false,'completion_level_experience',true,'stat_anchor_tier',30,'normal_boss_equipment',10,
  'exit_name','洞天残魂','exit_greeting','好久没见到别的人了……这里的风，总算静下来了。来吧，我这就送你回去。',
  'exit_sprite','npc362','exit_portrait','','npc_model',362);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=4) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 4,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=3;
  INSERT INTO game_rift_ability_rules SELECT 4,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=3;
  UPDATE game_rift_rule_versions SET published=true WHERE version=4;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=4) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift rules version 4 is already occupied by different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=3 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=4)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=4 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=3)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=3 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=4)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=4 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=3)) THEN
  RAISE EXCEPTION 'rift rules version 4 must preserve class and ability rules';
 END IF;
 -- Replaying this migration never replaces an operator's future active version.
 UPDATE game_rift_rule_active SET version=4 WHERE id=1 AND version=3;
END $rift_rebalance$;

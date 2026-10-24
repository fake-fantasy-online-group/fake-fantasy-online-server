-- New admissions only: 10% cumulative Legendary entitlement per Rift tier.
-- The special entitlement replaces both old special equipment slots; the ten
-- normal Boss items use the frozen outdoor loot3 policy, including personal MF.
-- Historical rules, paid-run snapshots and ready grants remain untouched.
ALTER TABLE game_rift_rule_versions ADD COLUMN IF NOT EXISTS cumulative_legendary_per_tier INTEGER;
DO $rift_cumulative$
DECLARE active_version INTEGER; original JSONB; revised JSONB;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL THEN RAISE EXCEPTION 'cumulative Rift rewards require an active selector'; END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules,
  game_loot_rule_versions,game_loot_rune_pool,game_loot_socket_levels,game_loot_grades IN SHARE MODE;
 IF NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid='game_rift_rule_versions'::regclass
  AND a.attname='cumulative_legendary_per_tier' AND NOT a.attisdropped
  AND a.atttypid='integer'::regtype AND NOT a.attnotnull AND NOT a.atthasdef) THEN
  RAISE EXCEPTION 'cumulative Rift rewards require a nullable INTEGER column without a default';
 END IF;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname='immutable_published_rules' AND NOT tgisinternal
  AND tgenabled IN ('O','A') AND tgtype=31 AND tgfoid='guard_published_endgame_rules()'::regprocedure
  AND tgrelid IN ('game_rift_rule_versions'::regclass,'game_rift_class_rules'::regclass,
   'game_rift_ability_rules'::regclass,'game_loot_rule_versions'::regclass,'game_loot_rune_pool'::regclass,
   'game_loot_socket_levels'::regclass,'game_loot_grades'::regclass))<>7 THEN
  RAISE EXCEPTION 'cumulative Rift rewards require enabled immutable rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r WHERE version=11 AND published
  AND loot_version=2 AND equipment_pools IS NOT NULL AND equipment_slot_fallbacks IS NOT NULL
  AND elite_rules IS NOT NULL AND floor_layout_version=6 AND guardian_recent_window=3
  AND normal_boss_equipment=10 AND equipment=1 AND timely_equipment=1
  AND cumulative_legendary_per_tier IS NULL;
 IF original IS NULL THEN RAISE EXCEPTION 'cumulative Rift rewards require unchanged published rift11'; END IF;
 IF NOT EXISTS(SELECT FROM game_loot_rule_versions WHERE version=3 AND published AND outdoor_equipment=
  '{"AffixWeights":[20,25,20,15,10,5],"LegendaryBasePPB":50000000}'::jsonb) OR
  (SELECT count(*) FROM game_loot_rune_pool WHERE version=3)<>28 OR
  (SELECT count(*) FROM game_loot_socket_levels WHERE version=3)<>4 OR
  (SELECT count(*) FROM game_loot_grades WHERE version=3)<>4 THEN
  RAISE EXCEPTION 'cumulative Rift rewards require complete published outdoor loot3';
 END IF;
 IF (SELECT count(*) FROM game_rift_class_rules WHERE version=11)<>4 OR
    (SELECT count(*) FROM game_rift_ability_rules WHERE version=11)<>3 THEN
  RAISE EXCEPTION 'cumulative Rift rewards require complete rift11 dependencies';
 END IF;
 revised := original || jsonb_build_object('version',12,'published',false,'loot_version',3,
  'cumulative_legendary_per_tier',10,'equipment',0,'timely_equipment',0,
  'unique_base_chance',0,'unique_per_tier',0,'unique_cap',0);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=12) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 12,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=11;
  INSERT INTO game_rift_ability_rules SELECT 12,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=11;
  UPDATE game_rift_rule_versions SET published=true WHERE version=12;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=12)
  IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift12 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=11
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=12)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=12
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=11)) OR
 EXISTS((SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=11
          EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=12)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=12
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=11)) THEN
  RAISE EXCEPTION 'rift12 must preserve class and ability dependencies';
 END IF;
 UPDATE game_rift_rule_active SET version=12 WHERE id=1 AND version=11;
END $rift_cumulative$;

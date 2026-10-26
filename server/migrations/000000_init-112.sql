-- Versioned power selection: old snapshots retain mixed pools and exact RNG.
-- Only newly admitted Rift16 and new loot4 drops use the two-stage policy.
ALTER TABLE game_loot_rule_versions ADD COLUMN IF NOT EXISTS legendary_power_policy INTEGER NOT NULL DEFAULT 0;
DO $legendary_power_policy$
DECLARE loot_active INTEGER; rift_active INTEGER; original JSONB; revised JSONB;
 dependency TEXT; source_version INTEGER; target_version INTEGER; expected_count INTEGER; actual_count INTEGER; differs BOOLEAN;
BEGIN
 SELECT version INTO loot_active FROM game_loot_rule_active WHERE id=1 FOR UPDATE;
 SELECT version INTO rift_active FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF loot_active IS NULL OR rift_active IS NULL THEN RAISE EXCEPTION 'power policy rollout requires active loot and rift selectors'; END IF;
 LOCK TABLE game_loot_rule_versions,game_loot_rune_pool,game_loot_socket_levels,game_loot_grades,
  game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules IN SHARE MODE;
 IF NOT EXISTS(SELECT FROM pg_attribute a JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
  WHERE a.attrelid='game_loot_rule_versions'::regclass AND a.attname='legendary_power_policy'
  AND NOT a.attisdropped AND a.atttypid='integer'::regtype AND a.attnotnull
  AND pg_get_expr(d.adbin,d.adrelid)='0') THEN
  RAISE EXCEPTION 'power policy rollout requires integer policy column with NOT NULL DEFAULT 0';
 END IF;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname='immutable_published_rules' AND NOT tgisinternal
  AND tgenabled IN ('O','A') AND tgtype=31 AND tgfoid='guard_published_endgame_rules()'::regprocedure
  AND tgrelid IN ('game_loot_rule_versions'::regclass,'game_loot_rune_pool'::regclass,
   'game_loot_socket_levels'::regclass,'game_loot_grades'::regclass,'game_rift_rule_versions'::regclass,
   'game_rift_class_rules'::regclass,'game_rift_ability_rules'::regclass))<>7 THEN
  RAISE EXCEPTION 'power policy rollout requires enabled immutable rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_loot_rule_versions r WHERE version=3 AND published AND legendary_power_policy=0;
 IF original IS NULL OR original IS DISTINCT FROM
  ((SELECT to_jsonb(r) FROM game_loot_rule_versions r WHERE version=2 AND published AND legendary_power_policy=0)
   || jsonb_build_object('version',3,'outdoor_equipment','{"AffixWeights":[20,25,20,15,10,5],"LegendaryBasePPB":50000000}'::jsonb,
   'outdoor_normal_equipment','{"AffixWeights":[46,20,15,10,5,2],"LegendaryBasePPB":20000000}'::jsonb)) THEN
  RAISE EXCEPTION 'power policy rollout requires unchanged published loot3';
 END IF;
 revised := original || jsonb_build_object('version',4,'published',false,'legendary_power_policy',1);
 IF NOT EXISTS(SELECT FROM game_loot_rule_versions WHERE version=4) THEN
  INSERT INTO game_loot_rule_versions SELECT (jsonb_populate_record(NULL::game_loot_rule_versions,revised)).*;
 ELSIF (SELECT to_jsonb(r) FROM game_loot_rule_versions r WHERE version=4)
  IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'loot4 is occupied by different configuration';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r WHERE version=15 AND published
  AND loot_version=3 AND cumulative_legendary_per_tier=10 AND normal_boss_equipment=10
  AND equipment=0 AND timely_equipment=0 AND unique_base_chance=0 AND unique_per_tier=0
  AND unique_cap=0 AND guardian_hp_divisor=10 AND jsonb_typeof(wing_rewards)='object'
  AND wing_rewards->'RareFeather'='{"Item":11682,"Count":1,"ChanceBP":50}'::jsonb;
 IF original IS NULL OR original IS DISTINCT FROM
  ((SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=14 AND published) || '{"version":15}'::jsonb) THEN
  RAISE EXCEPTION 'power policy rollout requires unchanged published rift15';
 END IF;
 revised := original || jsonb_build_object('version',16,'published',false,'loot_version',4);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=16) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=16)
  IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift16 is occupied by different configuration';
 END IF;
 FOR dependency,source_version,target_version,expected_count IN SELECT * FROM (VALUES
  ('game_loot_rune_pool',3,4,28),('game_loot_socket_levels',3,4,4),('game_loot_grades',3,4,4),
  ('game_rift_class_rules',15,16,4),('game_rift_ability_rules',15,16,3)) AS deps(t,s,d,n)
 LOOP
  EXECUTE format('SELECT count(*) FROM %I WHERE version=$1',dependency) INTO actual_count USING source_version;
  IF actual_count<>expected_count THEN RAISE EXCEPTION 'power policy rollout requires complete source dependencies: %',dependency; END IF;
  -- A published destination must be checked, never repaired silently on replay.
  IF (target_version=4 AND NOT (SELECT published FROM game_loot_rule_versions WHERE version=4)) OR
     (target_version=16 AND NOT (SELECT published FROM game_rift_rule_versions WHERE version=16)) THEN
   EXECUTE format('INSERT INTO %I SELECT (jsonb_populate_record(NULL::%I,to_jsonb(r)||jsonb_build_object(''version'',$1))).* FROM %I r WHERE version=$2',dependency,dependency,dependency)
    USING target_version,source_version;
  END IF;
  EXECUTE format('SELECT EXISTS((SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=$1 EXCEPT ALL SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=$2) UNION ALL (SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=$2 EXCEPT ALL SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=$1))',dependency,dependency,dependency,dependency)
   INTO differs USING source_version,target_version;
  IF differs THEN RAISE EXCEPTION 'power policy rollout must preserve source dependencies: %',dependency; END IF;
 END LOOP;
 UPDATE game_loot_rule_versions SET published=true WHERE version=4 AND NOT published;
 UPDATE game_rift_rule_versions SET published=true WHERE version=16 AND NOT published;
 -- Preserve older operator selections and do not downgrade a future release.
 UPDATE game_loot_rule_active SET version=4 WHERE id=1 AND version=3;
 UPDATE game_rift_rule_active SET version=16 WHERE id=1 AND version=15;
END $legendary_power_policy$;

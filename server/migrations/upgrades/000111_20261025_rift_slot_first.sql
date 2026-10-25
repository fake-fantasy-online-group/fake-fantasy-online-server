-- Existing databases: apply atomically; do not rerun fresh init.
BEGIN;
-- New admissions use slot-first equipment selection in the matching server.
-- Clone every Rift14 field unchanged; historical paid runs and ready grants stay frozen.
DO $rift_slot_first$
DECLARE active_version INTEGER; original JSONB; revised JSONB;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL THEN RAISE EXCEPTION 'slot-first rollout requires an active selector'; END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules IN SHARE MODE;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname='immutable_published_rules' AND NOT tgisinternal
  AND tgenabled IN ('O','A') AND tgtype=31 AND tgfoid='guard_published_endgame_rules()'::regprocedure
  AND tgrelid IN ('game_rift_rule_versions'::regclass,'game_rift_class_rules'::regclass,
   'game_rift_ability_rules'::regclass))<>3 THEN
  RAISE EXCEPTION 'slot-first rollout requires enabled immutable rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r WHERE version=14 AND published
  AND loot_version=3 AND cumulative_legendary_per_tier=10 AND normal_boss_equipment=10
  AND equipment=0 AND timely_equipment=0 AND unique_base_chance=0 AND unique_per_tier=0
  AND unique_cap=0 AND guardian_hp_divisor=10 AND jsonb_typeof(wing_rewards)='object'
  AND wing_rewards->'RareFeather'='{"Item":11682,"Count":1,"ChanceBP":50}'::jsonb;
 IF original IS NULL THEN RAISE EXCEPTION 'slot-first rollout requires unchanged published rift14'; END IF;
 IF (SELECT count(*) FROM game_rift_class_rules WHERE version=14)<>4 OR
    (SELECT count(*) FROM game_rift_ability_rules WHERE version=14)<>3 THEN
  RAISE EXCEPTION 'slot-first rollout requires complete rift14 dependencies';
 END IF;
 revised := original || jsonb_build_object('version',15,'published',false);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=15) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 15,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=14;
  INSERT INTO game_rift_ability_rules SELECT 15,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=14;
  UPDATE game_rift_rule_versions SET published=true WHERE version=15;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=15)
  IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift15 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=14
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=15)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=15
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=14)) OR
 EXISTS((SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=14
          EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=15)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=15
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=14)) THEN
  RAISE EXCEPTION 'rift15 must preserve class and ability dependencies';
 END IF;
 -- Preserve an operator's older selection and never downgrade a newer release.
 UPDATE game_rift_rule_active SET version=15 WHERE id=1 AND version=14;
END $rift_slot_first$;
COMMIT;

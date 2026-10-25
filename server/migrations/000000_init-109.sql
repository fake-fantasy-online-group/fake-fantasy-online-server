-- New admissions only: Rift guardian HP is one tenth of the final scaled HP.
-- Keep historical runs, all other monster stats, and rewards exactly unchanged.
ALTER TABLE game_rift_rule_versions ADD COLUMN IF NOT EXISTS guardian_hp_divisor INTEGER;
DO $rift_guardian_hp$
DECLARE active_version INTEGER; original JSONB; revised JSONB;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL THEN RAISE EXCEPTION 'guardian HP rollout requires an active selector'; END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules IN SHARE MODE;
 IF NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid='game_rift_rule_versions'::regclass
  AND a.attname='guardian_hp_divisor' AND NOT a.attisdropped
  AND a.atttypid='integer'::regtype AND NOT a.attnotnull AND NOT a.atthasdef) THEN
  RAISE EXCEPTION 'guardian HP rollout requires a nullable INTEGER column without a default';
 END IF;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname='immutable_published_rules' AND NOT tgisinternal
  AND tgenabled IN ('O','A') AND tgtype=31 AND tgfoid='guard_published_endgame_rules()'::regprocedure
  AND tgrelid IN ('game_rift_rule_versions'::regclass,'game_rift_class_rules'::regclass,
   'game_rift_ability_rules'::regclass))<>3 THEN
  RAISE EXCEPTION 'guardian HP rollout requires enabled immutable rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r WHERE version=12 AND published
  AND loot_version=3 AND cumulative_legendary_per_tier=10 AND normal_boss_equipment=10
  AND equipment=0 AND timely_equipment=0 AND unique_base_chance=0 AND unique_per_tier=0
  AND unique_cap=0 AND guardian_hp_divisor IS NULL;
 IF original IS NULL THEN RAISE EXCEPTION 'guardian HP rollout requires unchanged published rift12'; END IF;
 IF (SELECT count(*) FROM game_rift_class_rules WHERE version=12)<>4 OR
    (SELECT count(*) FROM game_rift_ability_rules WHERE version=12)<>3 THEN
  RAISE EXCEPTION 'guardian HP rollout requires complete rift12 dependencies';
 END IF;
 revised := original || jsonb_build_object('version',13,'published',false,'guardian_hp_divisor',10);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=13) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 13,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=12;
  INSERT INTO game_rift_ability_rules SELECT 13,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=12;
  UPDATE game_rift_rule_versions SET published=true WHERE version=13;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=13)
  IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift13 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=12
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=13)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=13
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_class_rules r WHERE version=12)) OR
 EXISTS((SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=12
          EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=13)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=13
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_rift_ability_rules r WHERE version=12)) THEN
  RAISE EXCEPTION 'rift13 must preserve class and ability dependencies';
 END IF;
 -- Preserve an operator's older selection and never downgrade a newer release.
 UPDATE game_rift_rule_active SET version=13 WHERE id=1 AND version=12;
END $rift_guardian_hp$;

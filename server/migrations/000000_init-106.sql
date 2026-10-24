-- New outdoor normal and Boss/elite drops only. Keep historical loot snapshots, rune
-- occurrence/selection, sockets, Rift rules/selectors, and player data intact.
-- The six conditional ordinary-colour weights total 95; an independent 5%
-- legendary promotion yields overall 20/25/20/15/10/5/5 on eligible gear.
-- Outdoor normal monsters use total98 weights46/20/15/10/5/2 and independent2%
-- promotion, giving overall46/20/15/10/5/2/2. Theft shares its target source.
ALTER TABLE game_loot_rule_versions ADD COLUMN IF NOT EXISTS outdoor_equipment JSONB;
ALTER TABLE game_loot_rule_versions ADD COLUMN IF NOT EXISTS outdoor_normal_equipment JSONB;
DO $outdoor_equipment$
DECLARE active_version INTEGER; original JSONB; revised JSONB;
BEGIN
 SELECT version INTO active_version FROM game_loot_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL THEN
  RAISE EXCEPTION 'outdoor equipment rollout requires an active loot selector';
 END IF;
 LOCK TABLE game_loot_rule_versions,game_loot_rune_pool,
  game_loot_socket_levels,game_loot_grades IN SHARE MODE;
 -- IF NOT EXISTS must not silently accept a conflicting schema on replay.
 IF (SELECT count(*) FROM pg_attribute a WHERE a.attrelid='game_loot_rule_versions'::regclass
  AND a.attname IN ('outdoor_equipment','outdoor_normal_equipment') AND NOT a.attisdropped
  AND a.atttypid='jsonb'::regtype AND NOT a.attnotnull AND NOT a.atthasdef)<>2 THEN
  RAISE EXCEPTION 'outdoor equipment rollout requires a nullable JSONB column without a default';
 END IF;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname='immutable_published_rules' AND NOT tgisinternal
   AND tgenabled IN ('O','A') AND tgtype=31 AND tgfoid='guard_published_endgame_rules()'::regprocedure
   AND tgrelid IN ('game_loot_rule_versions'::regclass,'game_loot_rune_pool'::regclass,
    'game_loot_socket_levels'::regclass,'game_loot_grades'::regclass))<>4 THEN
  RAISE EXCEPTION 'outdoor equipment rollout requires enabled immutable rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_loot_rule_versions r
  WHERE version=2 AND published AND outdoor_equipment IS NULL AND outdoor_normal_equipment IS NULL;
 IF original IS NULL THEN
  RAISE EXCEPTION 'outdoor equipment rollout requires unchanged published loot2';
 END IF;
 IF (SELECT count(*) FROM game_loot_rune_pool WHERE version=2)<>28 OR
    (SELECT count(*) FROM game_loot_socket_levels WHERE version=2)<>4 OR
    (SELECT count(*) FROM game_loot_grades WHERE version=2)<>4 THEN
  RAISE EXCEPTION 'outdoor equipment rollout requires complete loot2 dependencies';
 END IF;
 revised := original || jsonb_build_object('version',3,'published',false,'outdoor_equipment',
  '{"AffixWeights":[20,25,20,15,10,5],"LegendaryBasePPB":50000000}'::jsonb,
  'outdoor_normal_equipment','{"AffixWeights":[46,20,15,10,5,2],"LegendaryBasePPB":20000000}'::jsonb);
 IF NOT EXISTS(SELECT FROM game_loot_rule_versions WHERE version=3) THEN
  INSERT INTO game_loot_rule_versions SELECT (jsonb_populate_record(NULL::game_loot_rule_versions,revised)).*;
  INSERT INTO game_loot_rune_pool
   SELECT (jsonb_populate_record(NULL::game_loot_rune_pool,to_jsonb(r)||'{"version":3}'::jsonb)).*
   FROM game_loot_rune_pool r WHERE version=2;
  INSERT INTO game_loot_socket_levels
   SELECT (jsonb_populate_record(NULL::game_loot_socket_levels,to_jsonb(r)||'{"version":3}'::jsonb)).*
   FROM game_loot_socket_levels r WHERE version=2;
  INSERT INTO game_loot_grades
   SELECT (jsonb_populate_record(NULL::game_loot_grades,to_jsonb(r)||'{"version":3}'::jsonb)).*
   FROM game_loot_grades r WHERE version=2;
  UPDATE game_loot_rule_versions SET published=true WHERE version=3;
 ELSIF (SELECT to_jsonb(r) FROM game_loot_rule_versions r WHERE version=3)
  IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'loot3 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT to_jsonb(r)-'version' FROM game_loot_rune_pool r WHERE version=2
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_loot_rune_pool r WHERE version=3)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_loot_rune_pool r WHERE version=3
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_loot_rune_pool r WHERE version=2)) OR
 EXISTS((SELECT to_jsonb(r)-'version' FROM game_loot_socket_levels r WHERE version=2
          EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_loot_socket_levels r WHERE version=3)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_loot_socket_levels r WHERE version=3
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_loot_socket_levels r WHERE version=2)) OR
 EXISTS((SELECT to_jsonb(r)-'version' FROM game_loot_grades r WHERE version=2
          EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_loot_grades r WHERE version=3)
  UNION ALL (SELECT to_jsonb(r)-'version' FROM game_loot_grades r WHERE version=3
             EXCEPT ALL SELECT to_jsonb(r)-'version' FROM game_loot_grades r WHERE version=2)) THEN
  RAISE EXCEPTION 'loot3 must preserve every loot2 rune, socket, and grade dependency';
 END IF;
 -- Preserve an operator's older selection and never downgrade a newer release.
 UPDATE game_loot_rule_active SET version=3 WHERE id=1 AND version=2;
END $outdoor_equipment$;

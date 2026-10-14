-- Publish DB-authored, native elite equipment pools for new rift runs only.
-- Deploy with the color-first server. Existing run snapshots keep their pools.
-- IDs are materialized from actual native drop rows, not the global item catalog.
ALTER TABLE game_rift_rule_versions ADD COLUMN IF NOT EXISTS equipment_pools JSONB;
DO $rift_equipment$
DECLARE active_version INTEGER; original JSONB; revised JSONB; pools JSONB;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL OR active_version<6 THEN
  RAISE EXCEPTION 'rift equipment rollout requires active rift6';
 END IF;
 IF active_version>7 THEN RETURN; END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules,
  game_monsters,map_monster_spawns,game_monster_drops,game_equipment,gamedata.ov_arm IN SHARE MODE;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname='immutable_published_rules' AND NOT tgisinternal
   AND tgenabled IN ('O','A') AND tgfoid='guard_published_endgame_rules()'::regprocedure
   AND tgrelid IN ('game_rift_rule_versions'::regclass,'game_rift_class_rules'::regclass,'game_rift_ability_rules'::regclass))<>3 THEN
  RAISE EXCEPTION 'rift equipment rollout requires enabled immutable rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
  WHERE version=6 AND published AND equipment_pools IS NULL AND normal_boss_equipment=10;
 IF original IS NULL THEN RAISE EXCEPTION 'rift equipment rollout requires unchanged published rift6'; END IF;
 -- Directly spawned elites and aura replacements of real native spawn monsters.
 -- Both explicit named drops and native kind/resource-level templates are resolved.
 WITH sources AS (
  SELECT m.id,m.level FROM game_monsters m WHERE m.level>0 AND
   ((m.kind='精英' AND EXISTS(SELECT FROM map_monster_spawns s WHERE s.monster=m.id)) OR
    EXISTS(SELECT FROM game_monsters base JOIN map_monster_spawns s ON s.monster=base.id WHERE base.elite_id=m.id))
 ), kinds(query_type,kind) AS (VALUES
  (1,'长枪'),(2,'长剑'),(3,'双手剑'),(4,'法杖'),(5,'双刃'),(6,'暗器'),(7,'盾牌'),
  (8,'服装'),(9,'头盔'),(10,'鞋子'),(11,'手套'),(12,'戒指'),(13,'背包'),(14,'项链'),(15,'面具'),(49,'耳环'),(51,'腰带')
 ), native AS (
  SELECT e.id,a.level,COALESCE(k.kind,CASE a.position_ WHEN 114 THEN '耳环' WHEN 116 THEN '腰带' END) AS kind,
   (a.no_type_drop=0 AND a.level>=0 AND (k.kind IS NOT NULL OR a.position_ IN(114,116))) AS random_eligible
  FROM game_equipment e JOIN gamedata.ov_arm a ON a.index=e.id LEFT JOIN kinds k ON k.query_type=a.query_type
  WHERE e.level_req BETWEEN 0 AND 60 AND a.level_need BETWEEN 0 AND 60
   -- Ascension-only gear can have a low level requirement: inspect original job gates.
   AND NOT (COALESCE(a.newbie,0)=0 AND COALESCE(a.warrior,0)=0 AND COALESCE(a.swordman,0)=0
    AND COALESCE(a.stabber,0)=0 AND COALESCE(a.druggist,0)=0 AND COALESCE(a.magician,0)=0
    AND (COALESCE(a.sr_warrior,0)<>0 OR COALESCE(a.sr_swordman,0)<>0 OR COALESCE(a.sr_stabber,0)<>0
      OR COALESCE(a.sr_druggist,0)<>0 OR COALESCE(a.sr_magician,0)<>0))
 ), candidates AS (
  SELECT DISTINCT s.level,s.id AS monster_id,n.id AS item_id
  FROM sources s JOIN game_monster_drops d ON d.monster_id=s.id AND d.rate>0 AND d.rate<=100 AND d.item_src NOT LIKE 'dungeon_template:%'
  JOIN native n ON (d.item_id=n.id AND d.item_src='arm') OR
   (d.item_id=0 AND n.random_eligible AND btrim(d.kind_name)=n.kind AND d.lv_min>=0 AND d.lv_max-d.lv_min<=1000 AND n.level BETWEEN d.lv_min AND d.lv_max)
 ), tiers AS (
  SELECT level,jsonb_build_object('MonsterLevel',level,'SourceMonsters',jsonb_agg(DISTINCT monster_id ORDER BY monster_id),
   'Items',jsonb_agg(DISTINCT item_id ORDER BY item_id)) AS config FROM candidates GROUP BY level
 ) SELECT jsonb_agg(config ORDER BY level) INTO pools FROM tiers;
 IF pools IS NULL OR jsonb_array_length(pools)<1 THEN RAISE EXCEPTION 'no native preascension elite equipment pools'; END IF;
 revised := original || jsonb_build_object('version',7,'published',false,'equipment_pools',pools);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=7) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 7,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=6;
  INSERT INTO game_rift_ability_rules SELECT 7,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=6;
  UPDATE game_rift_rule_versions SET published=true WHERE version=7;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=7) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift7 equipment configuration differs from native source';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=6 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=7)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=7 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=6)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=6 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=7)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=7 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=6)) THEN
  RAISE EXCEPTION 'rift7 must preserve complete class and ability dependencies';
 END IF;
 IF active_version=6 THEN
  UPDATE game_rift_rule_active SET version=7 WHERE id=1 AND version=6;
  IF NOT FOUND THEN RAISE EXCEPTION 'rift selector changed during equipment activation'; END IF;
 END IF;
END $rift_equipment$;

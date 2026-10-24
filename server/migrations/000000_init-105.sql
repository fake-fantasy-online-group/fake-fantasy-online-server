-- New Rift admissions only: remove the twelve confirmed cosmetic hats and
-- freeze normal face/backpack bases for wearer-legal missing-slot fallback.
-- Do not edit native drops, equipment,
-- legendary minimum levels, player instances, or old paid-run snapshots.
-- v10's paging client gate is unchanged: activate only an already active v10.
ALTER TABLE game_rift_rule_versions ADD COLUMN IF NOT EXISTS equipment_slot_fallbacks JSONB;
DO $rift_cosmetic_hats$
DECLARE
 active_version INTEGER; original JSONB; revised JSONB; pools JSONB;
 fallbacks JSONB; fallback_slots INTEGER;
 excluded JSONB := $excluded$[
  {"id":2037,"name":"枪手帽","sex":0},
  {"id":2038,"name":"海盗帽","sex":0},
  {"id":2039,"name":"新郎彩冠","sex":1},
  {"id":2040,"name":"新娘彩冠","sex":2},
  {"id":2041,"name":"小鸡妹妹帽","sex":2},
  {"id":2042,"name":"小鸡哥哥帽","sex":1},
  {"id":2043,"name":"熊猫妹妹帽","sex":2},
  {"id":2044,"name":"熊猫哥哥帽","sex":1},
  {"id":2060,"name":"图腾头饰","sex":0},
  {"id":2061,"name":"兽图腾头饰","sex":0},
  {"id":2062,"name":"男生帽（舶来品）","sex":1},
  {"id":2063,"name":"仕女冠（舶来品）","sex":2}
 ]$excluded$::jsonb;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL OR active_version<9 THEN
  RAISE EXCEPTION 'rift cosmetic hat exclusion requires active rift9 or later';
 END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules,
  game_equipment,gamedata.ov_arm,game_equip_extra IN SHARE MODE;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname='immutable_published_rules' AND NOT tgisinternal
   AND tgenabled IN ('O','A') AND tgfoid='guard_published_endgame_rules()'::regprocedure
   AND tgrelid IN ('game_rift_rule_versions'::regclass,'game_rift_class_rules'::regclass,'game_rift_ability_rules'::regclass))<>3 THEN
  RAISE EXCEPTION 'rift cosmetic hat exclusion requires enabled immutable rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
  WHERE version=10 AND published AND equipment_pools IS NOT NULL AND elite_rules IS NOT NULL
    AND floor_layout_version=6 AND guardian_recent_window=3 AND normal_boss_equipment=10;
 IF original IS NULL THEN RAISE EXCEPTION 'rift cosmetic hat exclusion requires published rift10'; END IF;
 -- Exact reviewed identities, not a global level-one or head-slot predicate.
 IF jsonb_array_length(excluded)<>12 OR EXISTS (
  SELECT FROM jsonb_to_recordset(excluded) AS x(id INTEGER,name TEXT,sex INTEGER)
  LEFT JOIN game_equipment e ON e.id=x.id
  LEFT JOIN gamedata.ov_arm a ON a.index=x.id
  WHERE e.id IS NULL OR a.index IS NULL OR
   ROW(e.name,e.slot,e.level_req,a.level_need,a.level,a.sex,a.query_type,a.no_type_drop)
    IS DISTINCT FROM ROW(x.name,2,1,1,5,x.sex,9,0)
 ) THEN RAISE EXCEPTION 'rift cosmetic hat source identity mismatch'; END IF;
 IF EXISTS (
  SELECT FROM jsonb_to_recordset(excluded) AS x(id INTEGER)
  WHERE NOT EXISTS (
   SELECT FROM jsonb_array_elements(original->'equipment_pools') p,
    jsonb_array_elements_text(p->'Items') i WHERE i::INTEGER=x.id
  )
 ) THEN RAISE EXCEPTION 'rift10 is missing a reviewed cosmetic hat'; END IF;
 -- Preserve every tier, source monster, item order, and unrelated JSON field.
 SELECT jsonb_agg(p || jsonb_build_object('Items',COALESCE((
  SELECT jsonb_agg(i ORDER BY item_ordinal)
  FROM jsonb_array_elements(p->'Items') WITH ORDINALITY AS items(i,item_ordinal)
  WHERE NOT EXISTS (SELECT FROM jsonb_to_recordset(excluded) AS x(id INTEGER) WHERE x.id=i::INTEGER)
 ),'[]'::jsonb)) ORDER BY pool_ordinal) INTO pools
 FROM jsonb_array_elements(original->'equipment_pools') WITH ORDINALITY AS tiers(p,pool_ordinal);
 IF pools IS NULL OR jsonb_array_length(pools)=0 OR EXISTS (
  SELECT FROM jsonb_array_elements(pools) p WHERE jsonb_array_length(p->'Items')=0
 ) THEN RAISE EXCEPTION 'rift cosmetic hat exclusion would empty an equipment tier'; END IF;
 -- Keep fallback membership separate from native tier Items. Runtime first
 -- selects the unchanged nearest native pool for the party, then fills only
 -- a recipient's empty face/backpack slot with their highest wearable resource
 -- tier. Freezing the IDs here avoids a live global-catalog fallback on old runs.
 WITH fallback_candidates AS (
  SELECT e.id,e.slot FROM game_equipment e JOIN gamedata.ov_arm a ON a.index=e.id
  WHERE ((e.slot=1 AND a.query_type=15 AND a.position_=101) OR
         (e.slot=10 AND a.query_type=13 AND a.position_=110))
   AND a.no_type_drop=0 AND a.level>=0
   AND e.level_req BETWEEN 0 AND 60 AND a.level_need BETWEEN 0 AND 60
   AND NOT (COALESCE(a.newbie,0)=0 AND COALESCE(a.warrior,0)=0 AND COALESCE(a.swordman,0)=0
    AND COALESCE(a.stabber,0)=0 AND COALESCE(a.druggist,0)=0 AND COALESCE(a.magician,0)=0
    AND (COALESCE(a.sr_warrior,0)<>0 OR COALESCE(a.sr_swordman,0)<>0 OR COALESCE(a.sr_stabber,0)<>0
      OR COALESCE(a.sr_druggist,0)<>0 OR COALESCE(a.sr_magician,0)<>0))
   AND NOT EXISTS(SELECT FROM game_equip_extra x WHERE x.arm_id=e.id)
 )
 SELECT jsonb_agg(id ORDER BY id),count(DISTINCT slot) INTO fallbacks,fallback_slots FROM fallback_candidates;
 IF fallbacks IS NULL OR fallback_slots<>2 THEN
  RAISE EXCEPTION 'rift missing-slot fallback requires normal face and backpack bases';
 END IF;
 revised := original || jsonb_build_object('version',11,'published',false,'equipment_pools',pools,
  'equipment_slot_fallbacks',fallbacks);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=11) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 11,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=10;
  INSERT INTO game_rift_ability_rules SELECT 11,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=10;
  UPDATE game_rift_rule_versions SET published=true WHERE version=11;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=11) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift11 is occupied by different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=10 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=11)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=11 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=10)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=10 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=11)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=11 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=10)) THEN
  RAISE EXCEPTION 'rift11 must preserve class and ability dependencies';
 END IF;
 -- No upgrade from v9 (client rollout is a separate operation), no downgrade.
 UPDATE game_rift_rule_active SET version=11 WHERE id=1 AND version=10;
END $rift_cosmetic_hats$;

-- Add only the two native alternate class-set weapons to their matching
-- Pure Yang Tower bosses. Keep all existing rows and minimum item counts.
-- Named drops stay named: no_type_drop and ordinary random pools are untouched.
DO $alternate_set_weapons$
DECLARE r RECORD; source_pool game_boss_fixed_equipment_pool%ROWTYPE;
 actual_pool game_boss_fixed_equipment_pool%ROWTYPE; source_rows JSONB; target_rows JSONB;
 base_seq INTEGER;
BEGIN
 LOCK TABLE game_monster_drops,game_boss_fixed_equipment_pool IN SHARE ROW EXCLUSIVE MODE;
 FOR r IN SELECT * FROM (VALUES
  (3060,'彷徨的凶灵（头目）',8005,8051,'胜利之光',4,2,4,4,13),
  (3062,'嫉妒的凶灵（头目）',8401,8451,'夜色之息',1,8,32,13,13)
 ) AS v(monster_id,monster_name,source_id,target_id,item_name,suit_id,source_position,target_position,source_category,target_category)
 LOOP
  IF NOT EXISTS(SELECT FROM game_monsters WHERE id=r.monster_id AND name=r.monster_name AND kind='BOSS')
   OR NOT EXISTS(SELECT FROM map_defs d JOIN map_monster_spawns s ON s.map_file=d.file
    WHERE d.id=20600 AND d.name='纯阳宝塔' AND s.monster=r.monster_id) THEN
   RAISE EXCEPTION 'alternate set weapon boss identity or spawn changed: %',r.monster_id;
  END IF;
  IF (SELECT count(*) FROM gamedata.ov_arm a WHERE a.index IN(r.source_id,r.target_id)
    AND a.name=r.item_name AND a.exce_suit_id=r.suit_id AND a.level_need=33 AND a.no_type_drop=1
    AND a.warrior=0 AND a.druggist=0 AND a.magician=0
    AND a.swordman=CASE WHEN r.suit_id=4 THEN 1 ELSE 0 END
    AND a.stabber=CASE WHEN r.suit_id=1 THEN 1 ELSE 0 END
    AND a.position_=CASE WHEN a.index=r.source_id THEN r.source_position ELSE r.target_position END
    AND a.category=CASE WHEN a.index=r.source_id THEN r.source_category ELSE r.target_category END)<>2
   OR (SELECT count(*) FROM game_equipment WHERE id IN(r.source_id,r.target_id))<>2 THEN
   RAISE EXCEPTION 'alternate set weapon equipment identity changed: %',r.target_id;
  END IF;
  -- Both original weapons have two independently rolled named rows. Preserve
  -- that multiplicity (and the fixed-pool weight) for the alternate, not just
  -- one arbitrarily selected row. Do not repurpose either original row.
  IF (SELECT count(*) FROM game_monster_drops WHERE monster_id=r.monster_id AND item_id=r.source_id)<>2
   OR EXISTS(SELECT FROM game_monster_drops WHERE monster_id=r.monster_id AND item_id=r.source_id
    AND (item_name<>r.item_name OR item_src<>'arm' OR kind_name<>'' OR lv_min<>0 OR lv_max<>0 OR rate<=0 OR rate>100)) THEN
   RAISE EXCEPTION 'alternate set weapon source drops changed: %',r.source_id;
  END IF;
  SELECT jsonb_agg(jsonb_build_array(rate,item_name,item_src,kind_name,lv_min,lv_max) ORDER BY rate,item_name,item_src,kind_name,lv_min,lv_max)
   INTO source_rows FROM game_monster_drops WHERE monster_id=r.monster_id AND item_id=r.source_id;
  SELECT jsonb_agg(jsonb_build_array(rate,item_name,item_src,kind_name,lv_min,lv_max) ORDER BY rate,item_name,item_src,kind_name,lv_min,lv_max)
   INTO target_rows FROM game_monster_drops WHERE monster_id=r.monster_id AND item_id=r.target_id;
  IF target_rows IS NULL THEN
   SELECT COALESCE(max(seq),-1) INTO base_seq FROM game_monster_drops WHERE monster_id=r.monster_id;
   INSERT INTO game_monster_drops(monster_id,seq,rate,item_id,item_name,item_src,kind_name,lv_min,lv_max)
    SELECT monster_id,base_seq+row_number() OVER(ORDER BY seq)::INTEGER,rate,r.target_id,item_name,item_src,kind_name,lv_min,lv_max
     FROM game_monster_drops WHERE monster_id=r.monster_id AND item_id=r.source_id;
  ELSIF target_rows IS DISTINCT FROM source_rows THEN
   RAISE EXCEPTION 'alternate set weapon existing drops conflict: %',r.target_id;
  END IF;
  SELECT * INTO STRICT source_pool FROM game_boss_fixed_equipment_pool
   WHERE map_id=20600 AND monster_id=r.monster_id AND item_id=r.source_id;
  IF source_pool.minimum<>1 OR source_pool.weight<>2 OR source_pool.roll_affixes THEN
   RAISE EXCEPTION 'alternate set weapon fixed source changed: %',r.source_id;
  END IF;
  SELECT * INTO actual_pool FROM game_boss_fixed_equipment_pool
   WHERE map_id=20600 AND monster_id=r.monster_id AND item_id=r.target_id;
  IF NOT FOUND THEN
   SELECT COALESCE(max(seq),0)+1 INTO base_seq FROM game_boss_fixed_equipment_pool WHERE map_id=20600 AND monster_id=r.monster_id;
   INSERT INTO game_boss_fixed_equipment_pool(map_id,monster_id,seq,minimum,item_id,rate_pct,weight,roll_affixes)
    VALUES(20600,r.monster_id,base_seq,source_pool.minimum,r.target_id,source_pool.rate_pct,source_pool.weight,source_pool.roll_affixes);
  ELSIF (to_jsonb(actual_pool)-'seq'-'item_id') IS DISTINCT FROM (to_jsonb(source_pool)-'seq'-'item_id') THEN
   RAISE EXCEPTION 'alternate set weapon existing fixed pool conflicts: %',r.target_id;
  END IF;
 END LOOP;
END $alternate_set_weapons$;

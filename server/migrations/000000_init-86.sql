-- Rebalance the eight active Zhou Tian Ding operations. Keep historical IDs,
-- eligibility, RNG, and retired recipe flags untouched. All changes are atomic
-- and safe to replay; material_count always describes the normalized children.
DO $migration$
BEGIN
 IF (SELECT count(*) FROM game_cube_recipes WHERE id IN (1001,1002,1003,1006,1010,1011)
     OR id BETWEEN 11001 AND 11028 OR id BETWEEN 12001 AND 12028) <> 62 THEN
  RAISE EXCEPTION 'cube rebalance requires all 62 active recipe definitions';
 END IF;
 UPDATE game_cube_outputs SET count=1
 WHERE recipe_id=1002 AND item_id=1900028203;
 IF NOT FOUND THEN
  RAISE EXCEPTION 'cube rebalance requires unique salvage echo output';
 END IF;
 UPDATE game_cube_recipes SET money=CASE id WHEN 1010 THEN 10000000 WHEN 1011 THEN 1000000 ELSE 0 END
 WHERE id IN (1006,1010,1011) OR id BETWEEN 11001 AND 11028 OR id BETWEEN 12001 AND 12028;
 INSERT INTO game_cube_materials(recipe_id,seq,item_id,count)
 VALUES (1010,3,1900028203,4)
 ON CONFLICT(recipe_id,seq) DO UPDATE SET item_id=EXCLUDED.item_id,count=EXCLUDED.count;
 DELETE FROM game_cube_materials WHERE recipe_id BETWEEN 12001 AND 12028;
 UPDATE game_cube_recipes r SET material_count=(SELECT count(*) FROM game_cube_materials m WHERE m.recipe_id=r.id)
 WHERE r.id=1010 OR r.id BETWEEN 12001 AND 12028;
END $migration$;

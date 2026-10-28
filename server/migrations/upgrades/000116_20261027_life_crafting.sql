BEGIN;
-- Life crafting v1: configuration only; no inventory, wallet, historical
-- Rift receipt or already-paid run is rewritten. Apply upgrade atomically.
ALTER TABLE game_cube_recipes DROP CONSTRAINT IF EXISTS game_cube_recipes_operation_check;
ALTER TABLE game_cube_recipes ADD CONSTRAINT game_cube_recipes_operation_check CHECK
(operation IN ('salvage.normal','salvage.unique','salvage.rune','socket.open','socket.extract','socket.reorder','socket.array','unique.awaken','unique.reforge','unique.temper','rune.inscribe','rune.convert','life.salvage.normal','life.salvage.unique','life.affix.type','life.affix.value','life.affix.shuffle','life.affix.temper'));
-- Retain old definitions and balances for audit, but stop all obsolete costs.
UPDATE game_cube_recipes SET enabled=false WHERE id IN (1003,1006,1010,1011,1012)
 OR operation IN ('salvage.rune','rune.inscribe','rune.convert')
 OR id IN (SELECT recipe_id FROM game_cube_materials WHERE item_id IN (1900028201,1900028202,1900028203));
DELETE FROM game_cube_outputs WHERE recipe_id IN (1001,1002);
UPDATE game_cube_recipes SET operation=CASE id WHEN 1001 THEN 'life.salvage.normal' ELSE 'life.salvage.unique' END,
 description='分解装备获得原始矿石；装备等级决定矿种分布，天生品质与部位决定数量。孔内卡原样返还。',
 output_count=0, enabled=true WHERE id IN (1001,1002);
DO $guard$
BEGIN
 IF EXISTS (SELECT FROM game_cube_recipes WHERE (id BETWEEN 13001 AND 13004 OR id BETWEEN 13101 AND 13128 OR id IN(13201,13202)) AND operation NOT LIKE 'life.affix.%') THEN
 RAISE EXCEPTION 'life crafting recipe ID collision'; END IF;
END $guard$;
INSERT INTO game_cube_recipes(id,name,operation,description,target_required,money,source_count,quality_step_bp,material_count,output_count,enabled) VALUES
(13001,'青龙易象','life.affix.type','同象七星各一枚，重置第1顺位词缀及数值；可添一张卡片提高对应属性机缘。',true,0,0,0,7,0,true),
(13002,'玄武易象','life.affix.type','同象七星各一枚，重置第2顺位词缀及数值；可添一张卡片提高对应属性机缘。',true,0,0,0,7,0,true),
(13003,'白虎易象','life.affix.type','同象七星各一枚，重置第3顺位词缀及数值；可添一张卡片提高对应属性机缘。',true,0,0,0,7,0,true),
(13004,'朱雀易象','life.affix.type','同象七星各一枚，重置第4顺位词缀及数值；可添一张卡片提高对应属性机缘。',true,0,0,0,7,0,true),
(13101,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第1顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13102,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第1顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13103,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第1顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13104,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第1顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13105,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第1顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13106,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第1顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13107,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第1顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13108,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第2顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13109,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第2顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13110,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第2顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13111,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第2顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13112,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第2顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13113,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第2顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13114,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第2顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13115,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第3顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13116,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第3顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13117,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第3顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13118,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第3顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13119,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第3顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13120,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第3顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13121,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第3顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13122,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第4顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13123,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第4顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13124,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第4顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13125,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第4顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13126,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第4顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13127,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第4顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13128,'星篆洗炼','life.affix.value','同枚星篆三枚，重掷第4顺位词缀数值；星宿阶越高越偏向高值，但各阶均可触及完整范围。',true,0,0,0,1,0,true),
(13201,'六仪易序','life.affix.shuffle','神恩石三十枚，随机重排六条词缀；类型与数值不变。',true,0,0,0,1,0,true),
(13202,'暗金淬炼','life.affix.temper','强化十五级及以上暗金，消耗免爆宝石十枚；随机一条可变属性提高百分之五十，再次淬炼替换原加成，不叠加。',true,0,0,0,1,0,true)
ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,operation=EXCLUDED.operation,description=EXCLUDED.description,target_required=EXCLUDED.target_required,money=EXCLUDED.money,source_count=EXCLUDED.source_count,quality_step_bp=EXCLUDED.quality_step_bp,material_count=EXCLUDED.material_count,output_count=EXCLUDED.output_count,enabled=EXCLUDED.enabled;
DELETE FROM game_cube_materials WHERE recipe_id BETWEEN 13001 AND 13004 OR recipe_id BETWEEN 13101 AND 13128 OR recipe_id IN(13201,13202);
INSERT INTO game_cube_materials(recipe_id,seq,item_id,count) VALUES
(13001,1,1900028001,1),
(13101,1,1900028001,3),
(13001,2,1900028002,1),
(13102,1,1900028002,3),
(13001,3,1900028003,1),
(13103,1,1900028003,3),
(13001,4,1900028004,1),
(13104,1,1900028004,3),
(13001,5,1900028005,1),
(13105,1,1900028005,3),
(13001,6,1900028006,1),
(13106,1,1900028006,3),
(13001,7,1900028007,1),
(13107,1,1900028007,3),
(13002,1,1900028008,1),
(13108,1,1900028008,3),
(13002,2,1900028009,1),
(13109,1,1900028009,3),
(13002,3,1900028010,1),
(13110,1,1900028010,3),
(13002,4,1900028011,1),
(13111,1,1900028011,3),
(13002,5,1900028012,1),
(13112,1,1900028012,3),
(13002,6,1900028013,1),
(13113,1,1900028013,3),
(13002,7,1900028014,1),
(13114,1,1900028014,3),
(13003,1,1900028015,1),
(13115,1,1900028015,3),
(13003,2,1900028016,1),
(13116,1,1900028016,3),
(13003,3,1900028017,1),
(13117,1,1900028017,3),
(13003,4,1900028018,1),
(13118,1,1900028018,3),
(13003,5,1900028019,1),
(13119,1,1900028019,3),
(13003,6,1900028020,1),
(13120,1,1900028020,3),
(13003,7,1900028021,1),
(13121,1,1900028021,3),
(13004,1,1900028022,1),
(13122,1,1900028022,3),
(13004,2,1900028023,1),
(13123,1,1900028023,3),
(13004,3,1900028024,1),
(13124,1,1900028024,3),
(13004,4,1900028025,1),
(13125,1,1900028025,3),
(13004,5,1900028026,1),
(13126,1,1900028026,3),
(13004,6,1900028027,1),
(13127,1,1900028027,3),
(13004,7,1900028028,1),
(13128,1,1900028028,3),
(13201,1,4501,30),
(13202,1,4478,10);
-- All native fish can craft every star, so no rare species gates progression.
-- Native material encoding: quantity<<16 | itemID. Product IDs remain int32.
-- grade=1..7, value band=1+floor(native resource index/3).
-- Fish cost ceil(50*grade/band); ore cost ceil(200/band) or ceil(600/band).
DO $guard$
BEGIN
 IF EXISTS(SELECT FROM game_craft_extra_recipes WHERE row_no BETWEEN 5200 AND 5499 AND NOT (product_item_id BETWEEN 1900028001 AND 1900028028 OR product_item_id IN(4501,4478))) THEN
 RAISE EXCEPTION 'life furnace recipe ID collision'; END IF;
END $guard$;
INSERT INTO game_craft_extra_recipes(row_no,product_item_id,cost,need_lifeskill_id,prof_need,prof_max,prof_gain,disappear_rate,success_rate,skill_level_need,mat_count,mat_1,mat_2,mat_3,mat_4,mat_5,out_qty,show_type)
SELECT 5200+r*10+f,1900028001+r,0,0,0,0,0,0,100,0,1,
 (((50*((r%7)+1)+(1+f/3)-1)/(1+f/3))::bigint<<16)|(4401+f)::bigint,
 0,0,0,0,1,0 FROM generate_series(0,27) r CROSS JOIN generate_series(0,9) f
UNION ALL
SELECT 5480+o*2+k,CASE k WHEN 0 THEN 4501 ELSE 4478 END,0,0,0,0,0,0,100,0,1,
 (((CASE k WHEN 0 THEN 200 ELSE 600 END +(1+o/3)-1)/(1+o/3))::bigint<<16)|(4001+3*o)::bigint,
 0,0,0,0,1,0 FROM generate_series(0,9) o CROSS JOIN generate_series(0,1) k
ON CONFLICT(row_no) DO UPDATE SET product_item_id=EXCLUDED.product_item_id,cost=EXCLUDED.cost,need_lifeskill_id=EXCLUDED.need_lifeskill_id,prof_need=EXCLUDED.prof_need,prof_max=EXCLUDED.prof_max,prof_gain=EXCLUDED.prof_gain,disappear_rate=EXCLUDED.disappear_rate,success_rate=EXCLUDED.success_rate,skill_level_need=EXCLUDED.skill_level_need,mat_count=EXCLUDED.mat_count,mat_1=EXCLUDED.mat_1,mat_2=0,mat_3=0,mat_4=0,mat_5=0,out_qty=1,show_type=0;
DO $verify$
BEGIN
 IF (SELECT count(*) FROM game_craft_extra_recipes WHERE row_no BETWEEN 5200 AND 5499) <> 300 THEN RAISE EXCEPTION 'life furnace expects 300 routes'; END IF;
 IF EXISTS(SELECT FROM game_cube_recipes r WHERE r.enabled AND (r.operation IN('salvage.rune','rune.inscribe','rune.convert') OR r.id IN(1006,1010) OR EXISTS(SELECT FROM game_cube_materials m WHERE m.recipe_id=r.id AND m.item_id IN(1900028201,1900028202,1900028203)))) THEN RAISE EXCEPTION 'obsolete cube recipe remains active'; END IF;
 IF EXISTS(SELECT FROM game_cube_recipes r WHERE r.id BETWEEN 13001 AND 13202 AND (r.material_count<>(SELECT count(*) FROM game_cube_materials m WHERE m.recipe_id=r.id))) THEN RAISE EXCEPTION 'life cube child count mismatch'; END IF;
END $verify$;
COMMIT;

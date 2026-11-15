-- Existing databases: apply atomically. Do not run against production from this change set.
BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';

DO $rift_clear_pack$
DECLARE
 pack CONSTANT integer := 1900028304;
 pack_name CONSTANT text := '洞天通关礼包';
 pack_desc CONSTANT text := '洞天成功通关后的慰劳礼包。开启可获得天山雪莲、长白参和较多红蓝药水，并有极低概率开出本职业真*套装。永久绑定，不可交易。';
 icon integer;
BEGIN
 IF EXISTS(SELECT FROM game_items WHERE id=pack AND name<>pack_name) THEN
  RAISE EXCEPTION 'rift clear pack item identity collision %', pack;
 END IF;
 IF EXISTS(SELECT FROM game_equipment WHERE id=pack)
  OR EXISTS(SELECT FROM gamedata.ov_item WHERE (index=pack OR row_no=pack) AND name IS DISTINCT FROM pack_name AND name IS NOT NULL AND name <> '')
  OR EXISTS(SELECT FROM gamedata.ov_desc WHERE (index=pack OR row_no=pack) AND desc_ IS DISTINCT FROM pack_desc AND desc_ IS NOT NULL AND desc_ <> '') THEN
  RAISE EXCEPTION 'rift clear pack item identity collision %', pack;
 END IF;

 -- Potion rewards must already exist; 真* hard IDs are runtime-only and stay out of this table.
 IF (SELECT count(*) FROM game_items WHERE id IN (3121,3151,3012,3013,3014,3015,3016,3017,3018,3019,3122,3123,42432,42433)) <> 14 THEN
  RAISE EXCEPTION 'rift clear pack potion catalog is incomplete';
 END IF;

 SELECT small_map INTO icon FROM gamedata.ov_desc WHERE index=3121 ORDER BY row_no LIMIT 1;
 IF icon IS NULL OR icon<=0 THEN RAISE EXCEPTION 'rift clear pack native icon missing'; END IF;

 INSERT INTO game_items(id,name,price,level,raw,use_action,fixed_binding,binding_rule_source)
 VALUES(pack,pack_name,0,0,'\x','',true,'000130_20261115_rift_clear_pack.sql')
 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name, fixed_binding=true, binding_rule_source=EXCLUDED.binding_rule_source;

 INSERT INTO gamedata.ov_item SELECT (jsonb_populate_record(NULL::gamedata.ov_item,jsonb_build_object(
  'row_no',pack,'index',pack,'name',pack_name,'script','','category',0,'query_type',0,'level',0,'level_need',0,
  'can_pile',1,'can_mail',0,'can_deal',0,'can_buy',0,'can_drop',0,'weight',0,'buy_price',0,'sell_price',0,'self_use',1,'other_use',0,'use_waste',1))).*
  WHERE NOT EXISTS(SELECT FROM gamedata.ov_item WHERE index=pack OR row_no=pack);

 INSERT INTO gamedata.ov_desc SELECT (jsonb_populate_record(NULL::gamedata.ov_desc,jsonb_build_object(
  'row_no',pack,'index',pack,'avatar','','category',0,'type',0,'level',0,'level_need',0,'desc_',pack_desc,
  'small_map',icon,'big_map',icon,'can_pile',1,'can_mail',0,'can_deal',0,'can_buy',0,'can_drop',0,'weight',0,'sell_price',0,'self_use',1,'other_use',0))).*
  WHERE NOT EXISTS(SELECT FROM gamedata.ov_desc WHERE index=pack OR row_no=pack);

 INSERT INTO game_item_gacha_boxes(source_item_id,key_item_id,key_quantity,min_free_slots)
 VALUES(pack,0,0,32)
 ON CONFLICT(source_item_id) DO UPDATE SET key_item_id=0, key_quantity=0, min_free_slots=32;

 DELETE FROM game_item_gacha_rewards WHERE source_item_id=pack;
 INSERT INTO game_item_gacha_rewards(source_item_id,seq,reward_item_id,min_quantity,max_quantity,chance_bp) VALUES
  (pack,1,3121,3,5,10000),
  (pack,2,3151,3,5,10000),
  (pack,3,3012,20,30,10000),
  (pack,4,3013,20,30,10000),
  (pack,5,3014,20,30,10000),
  (pack,6,3015,20,30,10000),
  (pack,7,3016,15,25,10000),
  (pack,8,3017,15,25,10000),
  (pack,9,3018,15,25,10000),
  (pack,10,3019,15,25,10000),
  (pack,11,3122,10,20,10000),
  (pack,12,3123,10,20,10000),
  (pack,13,42432,8,15,10000),
  (pack,14,42433,8,15,10000);
END $rift_clear_pack$;

COMMIT;

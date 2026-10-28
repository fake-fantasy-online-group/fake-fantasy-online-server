BEGIN;
-- Nine sealed spirits: 10 gold each; new rifts alone gain spirit essence rewards.
CREATE TEMP TABLE spirit_shop_stage(kind int PRIMARY KEY,item int,npc int,name text,script text) ON COMMIT DROP;
INSERT INTO spirit_shop_stage VALUES
(1,24640,163,'鞋灵仙子','Nxieling'),(2,47028,241,'拳灵仙子','Nquanling'),
(3,47501,257,'羽灵仙子','Nyuling1'),(4,29874,273,'衣灵仙子','Nyiling'),
(5,24608,79,'项灵仙子','Nxiangling'),(6,47490,122,'面灵仙子','Nmianling'),
(7,47004,279,'帽灵仙子','Nmaoling'),(8,47640,283,'法宝灵仙子','Nfabaoling'),
(9,8038,287,'武灵仙子','Nwuling');
DO $shops$
BEGIN
 IF (SELECT count(*) FROM spirit_shop_stage s JOIN map_npcs n ON n.map_file='qw0007' AND n.npc_id=s.npc AND n.name=s.name AND n.script=s.script AND n.sell_list IN(0,6200+s.kind))<>9 THEN
  RAISE EXCEPTION 'spirit shop NPC identity or existing shop conflict'; END IF;
 IF (SELECT count(*) FROM spirit_shop_stage s JOIN game_equipment i ON i.id=s.item)<>9 THEN
  RAISE EXCEPTION 'spirit shop requires all nine base item definitions'; END IF;
 IF EXISTS(SELECT FROM spirit_shop_stage s JOIN game_shops g ON g.shop_id=6200+s.kind
   WHERE g.source_kind<>'inferred' OR g.source_note<>'spirit sealed purchase 10 gold' OR g.allow_repair OR NOT g.allow_sell OR g.stock_source_shop_id IS NOT NULL) OR
    EXISTS(SELECT FROM spirit_shop_stage s JOIN game_shop_items i ON i.shop_id=6200+s.kind WHERE i.slot<>0 OR i.item_id<>s.item) THEN
  RAISE EXCEPTION 'spirit shop ID occupied'; END IF;
END $shops$;
INSERT INTO game_shops(shop_id,source_kind,source_note,allow_sell,allow_repair)
 SELECT 6200+kind,'inferred','spirit sealed purchase 10 gold',true,false FROM spirit_shop_stage ON CONFLICT(shop_id) DO NOTHING;
INSERT INTO game_shop_items(shop_id,slot,item_id) SELECT 6200+kind,0,item FROM spirit_shop_stage ON CONFLICT(shop_id,slot) DO NOTHING;
UPDATE game_items i SET price=10000000 FROM spirit_shop_stage s WHERE i.id=s.item;
UPDATE game_equipment i SET price=10000000 FROM spirit_shop_stage s WHERE i.id=s.item;
UPDATE map_npcs n SET sell_list=6200+s.kind FROM spirit_shop_stage s WHERE n.map_file='qw0007' AND n.npc_id=s.npc;
DO $rift$
DECLARE original jsonb; revised jsonb; dependency text; expected int; actual int; differs boolean;
BEGIN
 PERFORM 1 FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'spirit rewards require active rift selector'; END IF;
 LOCK TABLE game_rift_rule_versions,game_rift_class_rules,game_rift_ability_rules IN SHARE MODE;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r WHERE version=18 AND published AND loot_version=6;
 IF original IS NULL THEN RAISE EXCEPTION 'spirit rewards require published rift18'; END IF;
 revised:=original||'{"version":19,"published":false}'::jsonb;
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=19) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=19) IS DISTINCT FROM (revised||'{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift19 occupied by different configuration'; END IF;
 FOR dependency,expected IN SELECT * FROM (VALUES ('game_rift_class_rules',4),('game_rift_ability_rules',3)) d(t,n) LOOP
  EXECUTE format('SELECT count(*) FROM %I WHERE version=18',dependency) INTO actual;
  IF actual<>expected THEN RAISE EXCEPTION 'incomplete spirit rift dependency: %',dependency; END IF;
  IF NOT(SELECT published FROM game_rift_rule_versions WHERE version=19) THEN
   EXECUTE format('INSERT INTO %I SELECT (jsonb_populate_record(NULL::%I,to_jsonb(r)||''{"version":19}''::jsonb)).* FROM %I r WHERE version=18',dependency,dependency,dependency);
  END IF;
  EXECUTE format('SELECT EXISTS((SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=18 EXCEPT ALL SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=19) UNION ALL (SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=19 EXCEPT ALL SELECT to_jsonb(r)-''version'' FROM %I r WHERE version=18))',dependency,dependency,dependency,dependency) INTO differs;
  IF differs THEN RAISE EXCEPTION 'rift19 dependency conflict: %',dependency; END IF;
 END LOOP;
 UPDATE game_rift_rule_versions SET published=true WHERE version=19 AND NOT published;
 UPDATE game_rift_rule_active SET version=19 WHERE id=1 AND version=18;
END $rift$;
COMMIT;

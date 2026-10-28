BEGIN;
-- All nine implemented spirit vendors are buy-only; native ring services stay untouched.
CREATE TEMP TABLE spirit_buy_only_stage(kind int PRIMARY KEY,item int,npc int,name text,script text) ON COMMIT DROP;
INSERT INTO spirit_buy_only_stage VALUES
(1,24640,163,'鞋灵仙子','Nxieling'),(2,47028,241,'拳灵仙子','Nquanling'),
(3,47501,257,'羽灵仙子','Nyuling1'),(4,29874,273,'衣灵仙子','Nyiling'),
(5,24608,79,'项灵仙子','Nxiangling'),(6,47490,122,'面灵仙子','Nmianling'),
(7,47004,279,'帽灵仙子','Nmaoling'),(8,47640,283,'法宝灵仙子','Nfabaoling'),
(9,8038,287,'武灵仙子','Nwuling');
DO $spirit_buy_only$
BEGIN
 IF (SELECT count(*) FROM spirit_buy_only_stage s JOIN map_npcs n ON
     n.map_file='qw0007' AND n.npc_id=s.npc AND n.name=s.name AND n.script=s.script AND n.sell_list=6200+s.kind)<>9 OR
    (SELECT count(*) FROM spirit_buy_only_stage s JOIN game_shops g ON
     g.shop_id=6200+s.kind AND g.source_kind='inferred' AND g.source_note='spirit sealed purchase 10 gold'
     AND NOT g.allow_repair AND g.stock_source_shop_id IS NULL)<>9 OR
    (SELECT count(*) FROM game_shop_items WHERE shop_id BETWEEN 6201 AND 6209)<>9 OR
    (SELECT count(*) FROM spirit_buy_only_stage s JOIN game_shop_items i ON
     i.shop_id=6200+s.kind AND i.slot=0 AND i.item_id=s.item)<>9 OR
    EXISTS(SELECT FROM map_npcs n JOIN spirit_buy_only_stage s ON n.sell_list=6200+s.kind
     WHERE n.map_file IS DISTINCT FROM 'qw0007' OR n.npc_id IS DISTINCT FROM s.npc) THEN
  RAISE EXCEPTION 'spirit buy-only requires nine exclusive native shops from 122';
 END IF;
 -- Only sale permission changes. Price, stock, NPC menu packet, tasks and services are preserved.
 UPDATE game_shops SET allow_sell=false WHERE shop_id BETWEEN 6201 AND 6209;
END $spirit_buy_only$;
COMMIT;

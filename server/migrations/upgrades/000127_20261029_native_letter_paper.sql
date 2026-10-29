BEGIN;
-- Native paper 3309 / script paper; native book-merchant greetings explicitly
-- sell skill books, letter paper and parcels. Restore paper only, not invented
-- parcel capacities/costs. Keep the existing item price and all existing stock.
DO $letter_paper$
DECLARE
    next_slot INTEGER;
    marker CONSTANT TEXT := 'native letter paper 3309 restored to book merchants';
BEGIN
    LOCK TABLE game_shops, game_shop_items IN SHARE ROW EXCLUSIVE MODE;
    LOCK TABLE game_items, map_npcs IN SHARE MODE;
    IF NOT EXISTS (SELECT FROM game_items WHERE id=3309 AND name='信纸'
                   AND price>0 AND price<=2147483647) THEN
        RAISE EXCEPTION 'letter paper item identity/price mismatch';
    END IF;
    IF NOT EXISTS (SELECT FROM game_shops WHERE shop_id=1002 AND stock_source_shop_id IS NULL)
       OR (SELECT count(*) FROM game_shops WHERE shop_id IN (1302,1402,1502,1902)
           AND stock_source_shop_id=1002)<>4 THEN
        RAISE EXCEPTION 'letter paper book merchant source mismatch';
    END IF;
    IF (SELECT count(*) FROM (VALUES
        ('qw0007',52,'鬼谷先生',1002),('qw0019',24,'无妄老人',1302),
        ('qw0032',39,'白羽书仙萧洞玄',1402),('qw0058',47,'先知风涯',1502),
        ('qw0154',24,'技能大师智羽',1902)
        ) AS expected(map_file,npc_id,name,sell_list)
        JOIN map_npcs n USING(map_file,npc_id,name,sell_list))<>5 THEN
        RAISE EXCEPTION 'letter paper native book merchant identity mismatch';
    END IF;
    IF EXISTS (SELECT FROM game_shop_items WHERE shop_id IN (1302,1402,1502,1902))
       OR NOT EXISTS (SELECT FROM game_shop_items WHERE shop_id=1002)
       OR EXISTS (SELECT FROM (SELECT slot,row_number() OVER (ORDER BY slot)-1 AS expected
            FROM game_shop_items WHERE shop_id=1002) slots WHERE slot<>expected) THEN
        RAISE EXCEPTION 'letter paper merchant stock is conflicting or noncontiguous';
    END IF;
    IF NOT EXISTS (SELECT FROM game_shop_items WHERE shop_id=1002 AND item_id=3309) THEN
        SELECT COALESCE(max(slot),-1)+1 INTO next_slot FROM game_shop_items WHERE shop_id=1002;
        IF next_slot>=120 THEN
            RAISE EXCEPTION 'letter paper book merchant stock is full';
        END IF;
        INSERT INTO game_shop_items(shop_id,slot,item_id) VALUES(1002,next_slot,3309);
    END IF;
    UPDATE game_shops SET source_note=concat_ws('; ',NULLIF(source_note,''),marker)
        WHERE shop_id=1002 AND strpos(source_note,marker)=0;
END
$letter_paper$;

COMMIT;

DO $pet_supplies$
DECLARE
    target INTEGER;
    source INTEGER;
    base_slot INTEGER;
    marker CONSTANT TEXT := 'pet supplies 3020-3031 restored from native feeding help';
BEGIN
    LOCK TABLE game_shops, game_shop_items IN SHARE ROW EXCLUSIVE MODE;
    LOCK TABLE game_items, map_npcs IN SHARE MODE;
    IF (SELECT count(*) FROM (VALUES
        (3020,'百叶草'),(3021,'千层肉'),(3022,'万般香'),(3023,'活血丹'),
        (3024,'回魂散'),(3025,'百花草'),(3026,'百果草'),(3027,'千块肉'),
        (3028,'千堆肉'),(3029,'万物香'),(3030,'万灵香'),(3031,'无尽淳')
        ) AS expected(id,name) JOIN game_items i USING(id,name)
        WHERE i.price > 0 AND i.price <= 2147483647) <> 12 THEN
        RAISE EXCEPTION 'pet supplies item identity/price mismatch';
    END IF;
    IF NOT EXISTS (SELECT FROM game_shops WHERE shop_id=1003 AND stock_source_shop_id IS NULL)
       OR (SELECT count(*) FROM game_shops WHERE shop_id IN (1303,1403,1503,1903,2102,2602,3003)
           AND stock_source_shop_id=1003) <> 7 THEN
        RAISE EXCEPTION 'pet supplies shop source mismatch';
    END IF;
    IF NOT EXISTS (SELECT FROM map_npcs WHERE sell_list=1101 AND name='道具商人清悠')
       OR NOT EXISTS (SELECT FROM map_npcs WHERE sell_list=1201 AND name='道具商人杜若') THEN
        RAISE EXCEPTION 'pet supplies beginner merchant identity mismatch';
    END IF;
    -- Keep the known five tools; abort if the deployment has a different base.
    IF (SELECT count(*) FROM game_shop_items WHERE shop_id=1003
        AND item_id IN (3382,3388,3383,3384,3389)) <> 5 THEN
        RAISE EXCEPTION 'pet supplies capture tools missing';
    END IF;
    FOREACH target IN ARRAY ARRAY[1003,1101,1201] LOOP
        SELECT stock_source_shop_id INTO source FROM game_shops WHERE shop_id=target;
        IF NOT FOUND OR (target<>1003 AND source IS NOT NULL AND source<>1001) THEN
            RAISE EXCEPTION 'pet supplies unexpected stock source for %',target;
        END IF;
        IF source=1001 THEN
            -- Detach only the two verified beginner merchants, not shared 1001.
            IF EXISTS (SELECT FROM game_shop_items WHERE shop_id=target)
               OR NOT EXISTS (SELECT FROM game_shops WHERE shop_id=1001 AND stock_source_shop_id IS NULL)
               OR NOT EXISTS (SELECT FROM game_shop_items WHERE shop_id=1001) THEN
                RAISE EXCEPTION 'pet supplies beginner source/local stock conflict: %',target;
            END IF;
            INSERT INTO game_shop_items(shop_id,slot,item_id)
                SELECT target,slot,item_id FROM game_shop_items WHERE shop_id=1001;
            UPDATE game_shops SET stock_source_shop_id=NULL,source_kind='inferred' WHERE shop_id=target;
        END IF;
        IF EXISTS (SELECT FROM (SELECT slot,row_number() OVER (ORDER BY slot)-1 AS expected
            FROM game_shop_items WHERE shop_id=target) slots WHERE slot<>expected) THEN
            RAISE EXCEPTION 'pet supplies noncontiguous stock: %',target;
        END IF;
        SELECT COALESCE(max(slot),-1) INTO base_slot FROM game_shop_items WHERE shop_id=target;
        INSERT INTO game_shop_items(shop_id,slot,item_id)
            SELECT target,base_slot+row_number() OVER (ORDER BY id)::INTEGER,id
            FROM generate_series(3020,3031) AS ids(id)
            WHERE NOT EXISTS (SELECT FROM game_shop_items WHERE shop_id=target AND item_id=id);
        -- Appending only absent IDs preserves existing order, extra stock and prices.
        UPDATE game_shops SET source_note=concat_ws('; ',NULLIF(source_note,''),marker)
            WHERE shop_id=target AND strpos(source_note,marker)=0;
    END LOOP;
END
$pet_supplies$;

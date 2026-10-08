-- NPC 商店是原服服务端配置，正式客户端资源只保留 ResSellList 结构而没有库存数据。
-- 这里区分真实抓包、按同职能复用、可闭合推断、已观测为空与尚未解析五种来源；
-- 未解析商店保留空库存，不能为了“看起来有商品”伪造特殊活动商店内容。
CREATE TABLE game_shops (
    shop_id              INT PRIMARY KEY CHECK (shop_id > 0),
    stock_source_shop_id INT NULL,
    source_kind          TEXT NOT NULL DEFAULT 'unresolved'
        CHECK (source_kind IN ('captured', 'inferred', 'category_reuse', 'observed_empty', 'unresolved')),
    source_note          TEXT NOT NULL DEFAULT '',
    allow_sell           BOOLEAN NOT NULL DEFAULT TRUE,
    allow_repair         BOOLEAN NOT NULL DEFAULT TRUE,
    -- 费用 = 物品买价 × 缺失耐久比例 × 该倍率；10000=1 倍。
    repair_price_bp         INT NOT NULL DEFAULT 10000 CHECK (repair_price_bp > 0),
    special_repair_price_bp INT NOT NULL DEFAULT 30000 CHECK (special_repair_price_bp > 0),
    special_repair_nianli   INT NOT NULL DEFAULT 0 CHECK (special_repair_nianli >= 0),
    CONSTRAINT game_shops_stock_source_fk FOREIGN KEY (stock_source_shop_id)
        REFERENCES game_shops(shop_id)
);

CREATE TABLE game_shop_items (
    shop_id INT NOT NULL REFERENCES game_shops(shop_id) ON DELETE CASCADE,
    slot    INT NOT NULL CHECK (slot >= 0 AND slot < 120),
    item_id INT NOT NULL CHECK (item_id > 0),
    PRIMARY KEY (shop_id, slot),
    UNIQUE (shop_id, item_id)
);

-- map_npcs.sell_list 是客户端 NPC 菜单实际引用的 ShopID；全部建行，保证每个
-- 已存在商店都能得到确定响应，新增 NPC 商店也不会被一份手抄 ID 清单漏掉。
INSERT INTO game_shops (shop_id)
SELECT DISTINCT sell_list FROM map_npcs WHERE sell_list > 0 ORDER BY sell_list;

-- 2026-07-22 正式客户端在龙城逐个调用 OpenShop 后抓到的原服库存。
UPDATE game_shops SET source_kind = 'captured', source_note = '2026-07-22 formal-client OpenShop capture'
 WHERE shop_id IN (1001,1002,1004,1005,1006,1007,1008,1009);

INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1001, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[3002,3033,3004,3005,3006,3007,3008,3101,3102,3103,3203,4072,3379,1901,1902,2864,2866]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1002, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[3868,3869,3877,3375,3376,3381,3862,3864,3865,3866,3941,3331,3380]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1004, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[2703,2704,2705,2706,2707,2403,2409,2404,2405,2406,2407,2408,2410,2553,2583,2554,2584,2555,2585,2556,2586,2557,2587,2558,2588,2559,2589,2560,2590,2502,2503,2504,2505,2506,2507,2508,2509,2102,2103,2104,2105,2106,2107,2108,2606,2608,2604,2610,2601,2607,2602,2603,2001,2008,2014,2029,2030,2051,2052,2053,2804,2806,2824,2886,2894,1503,1504,1505,1506,1507,1508,1509,1510]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1005, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[2703,2704,2705,2706,2707,2403,2409,2404,2405,2406,2407,2408,2410,2553,2583,2554,2584,2555,2585,2556,2586,2557,2587,2558,2588,2559,2589,2560,2590,2502,2503,2504,2505,2506,2507,2508,2509,2102,2103,2104,2105,2106,2107,2108,2606,2608,2604,2610,2601,2607,2602,2603,2003,2009,2015,2025,2026,2045,2046,2047,2820,2822,2832,2892,2902,1603,1604,1605,1606,1607,1608,1609,1610]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1006, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[2010,2004,2016,2027,2028,2048,2049,2050,2703,2704,2705,2706,2707,2403,2409,2404,2405,2406,2407,2408,2410,2553,2583,2554,2584,2555,2585,2556,2586,2557,2587,2558,2588,2559,2589,2560,2590,2502,2503,2504,2505,2506,2507,2508,2509,2102,2103,2104,2105,2106,2107,2108,2606,2608,2604,2610,2601,2607,2602,2603,1803,1905,1804,1906,1805,1907,1806,1908,1807,1909,1808,1910,1809,1911,1810,1912,2808,2810,2826,2884,2896]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1007, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[2703,2704,2705,2706,2707,2403,2409,2404,2405,2406,2407,2408,2410,2553,2583,2554,2584,2555,2585,2556,2586,2557,2587,2558,2588,2559,2589,2560,2590,2502,2503,2504,2505,2506,2507,2508,2509,2102,2103,2104,2105,2106,2107,2108,2606,2608,2604,2610,2601,2607,2602,2603,2006,2012,2018,2033,2034,2057,2058,2059,2816,2890,2898,2818,2830,1303,1304,1305,1306,1307,1308,1309,1310]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1008, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[2703,2704,2705,2706,2707,2403,2409,2404,2405,2406,2407,2408,2410,2553,2583,2554,2584,2555,2585,2556,2586,2557,2587,2558,2588,2559,2589,2560,2590,2502,2503,2504,2505,2506,2507,2508,2509,2102,2103,2104,2105,2106,2107,2108,2606,2608,2604,2610,2601,2607,2602,2603,2005,2011,2017,2031,2032,2054,2055,2056,2812,2814,2828,2888,2900,1303,1304,1305,1306,1307,1308,1309,1310]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1009, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[9507,9508,9509,9510,9513,9514]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);

-- 宠物捕捉工具由 ov_petgrow.capture_tool 与原服背包/价格样本双向闭合。
UPDATE game_shops SET source_kind = 'inferred',
       source_note = 'capture tools referenced by ov_petgrow.capture_tool'
 WHERE shop_id = 1003;
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1003, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[3382,3388,3383,3384,3389]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);

-- 1202 的真实 0x801c 已知恰为 21 项；以下是主数据中五职业低阶武器的完整集合。
UPDATE game_shops SET source_kind = 'inferred',
       source_note = '21-item formal packet count plus complete low-tier weapon set'
 WHERE shop_id = 1202;
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1202, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[1001,1002,1003,1119,1102,1103,1301,1302,1303,1501,1502,1503,1601,1602,1603,1801,1802,1803,1903,1904,1905]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);

-- 两位新手村防具师匠出售主数据中可购买的 1-10 级普通白装。这里排除
-- game_equip_extra 附加词条、活动时装和测试装备，仅保留低价基础防具。
UPDATE game_shops SET source_kind = 'inferred',
       source_note = 'user-confirmed 1-10 level ordinary armor without extra affixes'
 WHERE shop_id = 1203;
INSERT INTO game_shop_items (shop_id, slot, item_id)
SELECT 1203, ordinality::INT - 1, item_id
  FROM unnest(ARRAY[2002,2875,2876,2035,2036,2401,2551,2581,2605,2701,2101,2501,2001,2003,2005,2006,2010,2803,2804,2807,2808,2811,2812,2815,2816,2819,2820]::INT[])
       WITH ORDINALITY AS stock(item_id, ordinality);

-- 同职能商人沿用已确认库存。这里只复用明显同类名称/职业，不把神秘、活动、
-- 战场、收购等商店混进通用杂货表。
UPDATE game_shops SET stock_source_shop_id = 1001, source_kind = 'category_reuse',
       source_note = 'generic item/grocery merchant uses captured shop 1001 stock'
 WHERE shop_id IN (1101,1201,1301,1401,1501,1601,1901,2101,2103,2201,2301,2501,2503,2601,2603,2901,3001);
UPDATE game_shops SET stock_source_shop_id = 1002, source_kind = 'category_reuse',
       source_note = 'skill/book merchant uses captured shop 1002 stock'
 WHERE shop_id IN (1302,1402,1502,1902);
UPDATE game_shops SET stock_source_shop_id = 1003, source_kind = 'category_reuse',
       source_note = 'pet merchant uses inferred capture-tool stock'
 WHERE shop_id IN (1303,1403,1503,1903,2102,2602,3003);
UPDATE game_shops SET stock_source_shop_id = 1202, source_kind = 'category_reuse',
       source_note = 'starter weapon smith uses inferred shop 1202 stock'
 WHERE shop_id = 1102;
UPDATE game_shops SET stock_source_shop_id = 1203, source_kind = 'category_reuse',
       source_note = 'starter armor smith uses user-confirmed shop 1203 stock'
 WHERE shop_id = 1103;

-- “鲜花收购商”是玩家向 NPC 出售鲜花的入口：NPC 本身没有可购买库存，
-- allow_sell 保持开启。
UPDATE game_shops SET stock_source_shop_id = NULL, source_kind = 'inferred',
       source_note = 'user-confirmed player-sell-only flower buyer; no NPC stock',
       allow_sell = TRUE
 WHERE shop_id IN (1104,1204);

UPDATE game_shops SET stock_source_shop_id = 1004, source_kind = 'category_reuse', source_note = 'same class branch as captured shop 1004'
 WHERE shop_id IN (1304,1404,1504,1904,2604);
UPDATE game_shops SET stock_source_shop_id = 1005, source_kind = 'category_reuse', source_note = 'same class branch as captured shop 1005'
 WHERE shop_id IN (1305,1405,1505,1905,2605);
UPDATE game_shops SET stock_source_shop_id = 1006, source_kind = 'category_reuse', source_note = 'same class branch as captured shop 1006'
 WHERE shop_id IN (1306,1406,1506,1906,2606);
UPDATE game_shops SET stock_source_shop_id = 1007, source_kind = 'category_reuse', source_note = 'same class branch as captured shop 1007'
 WHERE shop_id IN (1307,1407,1507,1907,2607);
UPDATE game_shops SET stock_source_shop_id = 1008, source_kind = 'category_reuse', source_note = 'same class branch as captured shop 1008'
 WHERE shop_id IN (1308,1408,1508,1908,2608);
UPDATE game_shops SET stock_source_shop_id = 1009, source_kind = 'category_reuse', source_note = 'same honor branch as captured shop 1009'
 WHERE shop_id IN (1309,1409,1509,1909);

UPDATE game_shops SET source_kind = 'observed_empty',
       source_note = '2026-07-22 formal-client OpenShop capture returned zero items'
 WHERE shop_id = 2002;

-- 荣光使者复用原服已抓取的六件常规荣誉法宝：荡魔拂、碧如意、
-- 清气净璃瓶、赤葫芦、镇元塔、眩光镜。
UPDATE game_shops SET stock_source_shop_id = 1009, source_kind = 'category_reuse',
       source_note = 'user-confirmed honor magic-treasure seller using captured shop 1009 stock'
 WHERE shop_id IN (1801,2611);

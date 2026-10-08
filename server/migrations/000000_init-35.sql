-- 挖矿/钓鱼的服务端权威装备与消耗品前置。
-- 正式客户端会在物品栏齐全时先换装再发送 BeginWork，但服务端不能信任这段
-- 本地流程：开工和每次结算都必须按实际穿戴槽与背包重新裁决。

CREATE TABLE IF NOT EXISTS game_work_equipment_requirements (
    work_type       INT  NOT NULL,
    requirement_kind TEXT NOT NULL CHECK (requirement_kind IN ('outfit', 'tool')),
    equip_slot      INT  NOT NULL CHECK (equip_slot > 0),
    item_id         INT  NOT NULL CHECK (item_id > 0),
    PRIMARY KEY (work_type, requirement_kind, item_id)
);

CREATE TABLE IF NOT EXISTS game_work_consumable_requirements (
    work_type    INT PRIMARY KEY,
    item_id      INT NOT NULL CHECK (item_id > 0),
    qty_per_yield INT NOT NULL CHECK (qty_per_yield > 0)
);

-- 念力造物的结果不在施法包中，服务端按技能号读取本表。当前解包的三档技能
-- 都写明“制造一个小药瓶”，旧版官方资料把该产物明确为秘灵药，并给出
-- 每次 5 点念力；Ⅲ 的当前技能说明另外要求草鱼。
CREATE TABLE IF NOT EXISTS game_nianli_creation_rules (
    skill_id       INT PRIMARY KEY CHECK (skill_id IN (11008, 11009, 11010)),
    product_item_id INT NOT NULL REFERENCES game_items(id),
    product_qty    INT NOT NULL DEFAULT 1 CHECK (product_qty > 0),
    nianli_cost    INT NOT NULL DEFAULT 5 CHECK (nianli_cost > 0)
);

CREATE TABLE IF NOT EXISTS game_nianli_creation_materials (
    skill_id       INT NOT NULL REFERENCES game_nianli_creation_rules(skill_id) ON DELETE CASCADE,
    material_order INT NOT NULL CHECK (material_order > 0),
    item_id        INT NOT NULL REFERENCES game_items(id),
    qty            INT NOT NULL DEFAULT 1 CHECK (qty > 0),
    PRIMARY KEY (skill_id, material_order),
    UNIQUE (skill_id, item_id)
);

INSERT INTO game_work_equipment_requirements
    (work_type, requirement_kind, equip_slot, item_id)
VALUES
    -- 挖矿：矿工服装（男女）+ 双手采矿工具。精制锄与三档矿镐均是合法升级品。
    (10, 'outfit', 8, 2863),
    (10, 'outfit', 8, 2864),
    (10, 'tool', 13, 1901),
    (10, 'tool', 13, 15034),
    (10, 'tool', 13, 1163),
    (10, 'tool', 13, 1164),
    (10, 'tool', 13, 1165),
    -- 钓鱼：渔夫装（男女）+ 基础鱼竿及三档钓竿。
    (11, 'outfit', 8, 2865),
    (11, 'outfit', 8, 2866),
    (11, 'tool', 13, 1902),
    (11, 'tool', 13, 1166),
    (11, 'tool', 13, 1167),
    (11, 'tool', 13, 1168)
ON CONFLICT (work_type, requirement_kind, item_id) DO UPDATE SET
    equip_slot = EXCLUDED.equip_slot;

INSERT INTO game_work_consumable_requirements (work_type, item_id, qty_per_yield)
VALUES (11, 3379, 1)
ON CONFLICT (work_type) DO UPDATE SET
    item_id = EXCLUDED.item_id,
    qty_per_yield = EXCLUDED.qty_per_yield;

INSERT INTO game_nianli_creation_rules
    (skill_id, product_item_id, product_qty, nianli_cost)
VALUES
    (11008, 3032, 1, 5),
    (11009, 3032, 1, 5),
    (11010, 3032, 1, 5)
ON CONFLICT (skill_id) DO UPDATE SET
    product_item_id = EXCLUDED.product_item_id,
    product_qty = EXCLUDED.product_qty,
    nianli_cost = EXCLUDED.nianli_cost;

INSERT INTO game_nianli_creation_materials
    (skill_id, material_order, item_id, qty)
VALUES (11010, 1, 4402, 1)
ON CONFLICT (skill_id, material_order) DO UPDATE SET
    item_id = EXCLUDED.item_id,
    qty = EXCLUDED.qty;

-- ov_work 用 151/151 标记采矿这组熟练度模板，并不是要求角色达到 151 级。
-- 正式客户端实测 10 级角色可以学习矿工并采矿；最终服务端业务范围在这里
-- 明确落成 10~150，避免学会技能后仍永远被“等级不足”挡住。
UPDATE game_work
   SET lv_min = 10, lv_max = 150
 WHERE work_type = 10 AND lv_min = 151 AND lv_max = 151;

-- 当前客户端 ov_work 的采矿奖励槽被清成了 0，但本机正式服连续采矿记录保留了
-- 3,760 次真实产出。逐熟练度档交叉统计后，规则是累计解锁：铁矿权重 2，
-- 其余已解锁矿各权重 1；不是“这一档只出这一种矿”。相对权重能精确表达
-- 2/(n+1) 与 1/(n+1)，避免硬凑千分比时对 3、6、7 等分母产生舍入偏差。
WITH mining_tier(work_id, unlocked) AS (
    VALUES (9,1), (15,2), (17,3), (19,4), (21,5),
           (23,6), (25,7), (27,8), (29,9), (31,10)
), mining_ore(seq, item_id, practise_gain_max) AS (
    VALUES (0,4001,1), (1,4004,2), (2,4007,3), (3,4010,4), (4,4013,5),
           (5,4016,6), (6,4019,7), (7,4022,8), (8,4025,9), (9,4028,10)
)
INSERT INTO game_work_items
    (work_id, seq, item_id, qty, prob, practise_gain_max)
SELECT t.work_id, o.seq, o.item_id, 1,
       CASE WHEN o.seq = 0 THEN 2 ELSE 1 END,
       o.practise_gain_max
  FROM mining_tier t
  JOIN mining_ore o ON o.seq < t.unlocked
ON CONFLICT (work_id, seq) DO UPDATE SET
    item_id = EXCLUDED.item_id,
    qty = EXCLUDED.qty,
    prob = EXCLUDED.prob,
    practise_gain_max = EXCLUDED.practise_gain_max;

UPDATE game_work SET roll_mode = 1 WHERE work_type = 10;

DO $$
DECLARE n INT;
BEGIN
    SELECT count(*) INTO n FROM game_nianli_creation_rules;
    IF n <> 3 THEN RAISE EXCEPTION '念力造物规则应有 3 行，实得 %', n; END IF;

    SELECT count(*) INTO n FROM game_nianli_creation_materials
     WHERE skill_id = 11010 AND material_order = 1 AND item_id = 4402 AND qty = 1;
    IF n <> 1 THEN RAISE EXCEPTION '念力造物Ⅲ必须消耗草鱼×1'; END IF;

    SELECT count(*) INTO n
      FROM game_work_equipment_requirements r
      LEFT JOIN game_equipment e ON e.id = r.item_id
     WHERE e.id IS NULL OR e.slot <> r.equip_slot;
    IF n <> 0 THEN
        RAISE EXCEPTION '有 % 条打工装备要求引用不存在的装备或错误槽位', n;
    END IF;

    SELECT count(*) INTO n
      FROM game_work_consumable_requirements r
      LEFT JOIN game_items i ON i.id = r.item_id
     WHERE i.id IS NULL;
    IF n <> 0 THEN
        RAISE EXCEPTION '有 % 条打工消耗品要求引用不存在的物品', n;
    END IF;

    SELECT count(*) INTO n FROM game_work_equipment_requirements
     WHERE work_type = 10 AND requirement_kind = 'tool';
    IF n <> 5 THEN RAISE EXCEPTION '挖矿工具应有 5 档，实得 %', n; END IF;

    SELECT count(*) INTO n FROM game_work_equipment_requirements
     WHERE work_type = 11 AND requirement_kind = 'tool';
    IF n <> 4 THEN RAISE EXCEPTION '钓鱼工具应有 4 档，实得 %', n; END IF;

    SELECT count(*) INTO n FROM game_work
     WHERE work_type = 10 AND (lv_min <> 10 OR lv_max <> 150);
    IF n <> 0 THEN RAISE EXCEPTION '仍有 % 条采矿模板等级范围未规范化', n; END IF;

    SELECT count(*) INTO n
      FROM game_work_items i JOIN game_work w ON w.work_id = i.work_id
     WHERE w.work_type = 10;
    IF n <> 55 THEN RAISE EXCEPTION '采矿累计解锁产出应有 55 行，实得 %', n; END IF;

    SELECT count(*) INTO n
      FROM game_work w
     WHERE w.work_type = 10 AND (
         w.roll_mode <> 1 OR
         (SELECT count(*) FROM game_work_items i WHERE i.work_id = w.work_id) < 1 OR
         (SELECT count(*) FROM game_work_items i WHERE i.work_id = w.work_id)
             <> (SELECT max(i.practise_gain_max) FROM game_work_items i WHERE i.work_id = w.work_id) OR
         (SELECT sum(i.prob) FROM game_work_items i WHERE i.work_id = w.work_id)
             <> (SELECT count(*) + 1 FROM game_work_items i WHERE i.work_id = w.work_id)
     );
    IF n <> 0 THEN RAISE EXCEPTION '有 % 个采矿档不满足累计解锁/2:1 权重规则', n; END IF;

END $$;

-- 四件工作服的物品定义与正式客户端实际身体资源成对相反。原始 gamedata
-- 保持无损；服务端外观只通过显式覆盖表使用真机确认后的模型。
INSERT INTO game_equipment_appearance_overrides(item_id, avatar, reason) VALUES
  (2863, 'N952', '正式客户端工作服外观实测'),
  (2864, 'N953', '正式客户端工作服外观实测'),
  (2865, 'N950', '正式客户端工作服外观实测'),
  (2866, 'N951', '正式客户端工作服外观实测')
ON CONFLICT (item_id) DO UPDATE SET
  avatar = EXCLUDED.avatar,
  reason = EXCLUDED.reason;

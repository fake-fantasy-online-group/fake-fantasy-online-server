-- 概率礼包（开箱）：钥匙 + 独立概率 + 数量区间 + 背包空间前置限制。
--
-- 原服礼包开箱是服务端脚本行为（ov_item.script 只保留 Item[0-9]+ 等处理器名，
-- 脚本主体与概率数值未随静态数据提供），本表以纯结构化配置替代脚本：
--   - game_item_gacha_boxes  每个礼包一条：钥匙物品/数量 + 背包剩余格数门槛；
--   - game_item_gacha_rewards 每个礼包多条奖励：物品 + [min,max] 数量区间 +
--     万分比概率，每条奖励独立判定（与掉落表 RatePct 独立掷语义一致），
--     可同时命中多条，也可能一条不中。
--
-- 当前分片只建表与校验，不装载任何礼包数据；数据由后续分片按需配置，
-- 配置经本分片 DO 校验块与场景加载校验双重闭合。

CREATE TABLE IF NOT EXISTS game_item_gacha_boxes (
    source_item_id  INT PRIMARY KEY CHECK (source_item_id > 0),
    key_item_id     INT NOT NULL DEFAULT 0 CHECK (key_item_id >= 0),
    key_quantity    INT NOT NULL DEFAULT 0 CHECK (key_quantity >= 0),
    min_free_slots  INT NOT NULL DEFAULT 0 CHECK (min_free_slots >= 0),
    CHECK ((key_item_id = 0 AND key_quantity = 0) OR (key_item_id > 0 AND key_quantity > 0))
);

CREATE TABLE IF NOT EXISTS game_item_gacha_rewards (
    source_item_id  INT NOT NULL CHECK (source_item_id > 0),
    seq             SMALLINT NOT NULL CHECK (seq > 0),
    reward_item_id  INT NOT NULL CHECK (reward_item_id > 0),
    min_quantity    INT NOT NULL CHECK (min_quantity > 0),
    max_quantity    INT NOT NULL CHECK (max_quantity >= min_quantity),
    chance_bp       INT NOT NULL CHECK (chance_bp BETWEEN 1 AND 10000),
    PRIMARY KEY (source_item_id, seq)
);

DO $$
DECLARE n INT;
BEGIN
    -- 礼包本体必须是普通物品（game_items），装备不能作为礼包。
    SELECT count(*) INTO n FROM game_item_gacha_boxes b
     WHERE NOT EXISTS(SELECT 1 FROM game_items i WHERE i.id=b.source_item_id)
        OR EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=b.source_item_id);
    IF n <> 0 THEN RAISE EXCEPTION '概率礼包有%行本体不是普通物品', n; END IF;

    -- 钥匙必须闭合且不能是礼包自身。
    SELECT count(*) INTO n FROM game_item_gacha_boxes b
     WHERE b.key_item_id > 0
       AND (b.key_item_id = b.source_item_id
            OR (NOT EXISTS(SELECT 1 FROM game_items i WHERE i.id=b.key_item_id)
                AND NOT EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=b.key_item_id)));
    IF n <> 0 THEN RAISE EXCEPTION '概率礼包有%行钥匙配置无效', n; END IF;

    -- 奖励物品必须命中普通物品或装备。
    SELECT count(*) INTO n FROM game_item_gacha_rewards r
     WHERE NOT EXISTS(SELECT 1 FROM game_items i WHERE i.id=r.reward_item_id)
       AND NOT EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=r.reward_item_id);
    IF n <> 0 THEN RAISE EXCEPTION '概率礼包有%条奖励未命中物品定义', n; END IF;

    -- 每个概率礼包至少一条奖励，且奖励行 seq 从 1 连续。
    SELECT count(*) INTO n FROM game_item_gacha_boxes b
     WHERE NOT EXISTS(SELECT 1 FROM game_item_gacha_rewards r WHERE r.source_item_id=b.source_item_id);
    IF n <> 0 THEN RAISE EXCEPTION '概率礼包有%个缺少奖励行', n; END IF;

    -- 与固定礼包互斥：同一物品不能既是固定礼包又是概率礼包。
    SELECT count(*) INTO n FROM game_item_gacha_boxes b
     WHERE EXISTS(SELECT 1 FROM game_item_use_rewards u WHERE u.source_item_id=b.source_item_id);
    IF n <> 0 THEN RAISE EXCEPTION '概率礼包与固定礼包重叠%行', n; END IF;
END $$;

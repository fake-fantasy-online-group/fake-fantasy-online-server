-- 1.5.8 快速换装备用套装。这里保存的是从背包移入 ChangeSet 面板后仍归角色
-- 所有的装备实例；cell 使用客户端装备部位号 1..11，双手武器与单手武器共用
-- 4 号武器格。切换时服务端仍按 game_equipment.slot 恢复真实穿戴槽并重做门槛校验。
CREATE TABLE IF NOT EXISTS character_change_set_items (
    char_id                  BIGINT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    cell                     INT NOT NULL CHECK (cell BETWEEN 1 AND 11),
    uid                      BIGINT NOT NULL DEFAULT 0,
    item_id                  INT NOT NULL CHECK (item_id > 0),
    durability               INT NOT NULL DEFAULT 0,
    max_durability           INT NOT NULL DEFAULT 0 CHECK (max_durability >= 0),
    durability_wear_raw      INT NOT NULL DEFAULT 0 CHECK (durability_wear_raw BETWEEN 0 AND 299),
    bound                    BOOLEAN NOT NULL DEFAULT FALSE,
    locked                   BOOLEAN NOT NULL DEFAULT FALSE,
    refine_level             INT NOT NULL DEFAULT 0 CHECK (refine_level >= 0),
    socket_count             SMALLINT NOT NULL DEFAULT 0 CHECK (socket_count BETWEEN 0 AND 5),
    sockets                  INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0] CHECK (cardinality(sockets)=5),
    wash_quality             SMALLINT NOT NULL DEFAULT 0 CHECK (wash_quality BETWEEN 0 AND 4),
    wash_count               SMALLINT NOT NULL DEFAULT 0 CHECK (wash_count BETWEEN 0 AND 4),
    wash_attrs               INT[] NOT NULL DEFAULT ARRAY[0,0,0,0] CHECK (cardinality(wash_attrs)=4),
    wash_values              INT[] NOT NULL DEFAULT ARRAY[0,0,0,0] CHECK (cardinality(wash_values)=4),
    wash_modes               INT[] NOT NULL DEFAULT ARRAY[0,0,0,0] CHECK (cardinality(wash_modes)=4),
    fused_appearance_item_id INT NOT NULL DEFAULT 0 CHECK (fused_appearance_item_id >= 0),
    PRIMARY KEY (char_id, cell)
);

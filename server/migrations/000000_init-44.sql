-- 装备“染色”档位。装备名与属性行的颜色只由**属性条数**决定（见
-- scene/inventory.go 的 equipmentQuality），所以“染色率”就是掉落随机装备时
-- 抽取随机属性条数的权重。这里把权重按怪物类别分档：
--   档 1 = 精英（含运行时由 EliteID 转换出的光圈精英）与 BOSS：60/20/10/5/3/1；
--   档 2 = 普通怪与副本小怪：70/20/10，只配到 2 条，因此上限为天蓝。
-- 属性数值池 game_equipment_roll_options 与档位无关，各档共用同一套数值。
CREATE TABLE IF NOT EXISTS game_equipment_color_profiles (
    id   SMALLINT PRIMARY KEY CHECK (id > 0),
    name TEXT NOT NULL CHECK (name <> '')
);

INSERT INTO game_equipment_color_profiles(id,name) VALUES
    (1,'精英/BOSS：60/20/10/5/3/1'),
    (2,'普通怪与副本小怪：70/20/10，上限2条')
ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name;

ALTER TABLE game_equipment_roll_counts
    ADD COLUMN IF NOT EXISTS profile_id SMALLINT NOT NULL DEFAULT 1
    REFERENCES game_equipment_color_profiles(id);

-- 条数权重从“全局一组”变成“每档一组”。旧行由 DEFAULT 1 落在精英/BOSS 档，
-- 因此精英与 BOSS 的染色分布与改动前完全一致。
ALTER TABLE game_equipment_roll_counts
    DROP CONSTRAINT IF EXISTS game_equipment_roll_counts_pkey;
ALTER TABLE game_equipment_roll_counts
    ADD CONSTRAINT game_equipment_roll_counts_pkey PRIMARY KEY (profile_id, attr_count);

-- 普通档只配置 0/1/2 三档：没有配置的条数权重为 0，永远不会抽出 3 条以上。
INSERT INTO game_equipment_roll_counts(profile_id,attr_count,weight) VALUES
    (2,0,70),(2,1,20),(2,2,10)
ON CONFLICT(profile_id,attr_count) DO UPDATE SET weight=EXCLUDED.weight;

-- 怪物类别 -> 染色档位。kind 用 game_monsters 自己的措辞；“光圈怪”在运行时会被
-- 替换成 kind='精英' 的模板，因此与模板精英同档。空 kind 行是没见过的类别词的
-- 兜底（domain.ParseMonsterKind 同样把它按普通处理）。
CREATE TABLE IF NOT EXISTS game_monster_color_profiles (
    kind       TEXT PRIMARY KEY,
    profile_id SMALLINT NOT NULL REFERENCES game_equipment_color_profiles(id)
);

INSERT INTO game_monster_color_profiles(kind,profile_id) VALUES
    ('普通',2),('精英',1),('BOSS',1),('场景物件',2),('采集物',2),('',2)
ON CONFLICT(kind) DO UPDATE SET profile_id=EXCLUDED.profile_id;

DO $$
DECLARE n INT; max_count INT;
BEGIN
    SELECT count(*) INTO n FROM game_equipment_roll_counts WHERE profile_id=1;
    IF n<>6 THEN RAISE EXCEPTION '精英/BOSS 档属性条数应为6档，实得%',n; END IF;
    SELECT count(DISTINCT attr_count) INTO n FROM game_equipment_roll_counts WHERE profile_id=1;
    IF n<>6 THEN RAISE EXCEPTION '精英/BOSS 档属性条数应覆盖0..5，实得%档',n; END IF;
    SELECT max(attr_count) INTO max_count FROM game_equipment_roll_counts WHERE profile_id=2;
    IF max_count IS NULL OR max_count>2 THEN
        RAISE EXCEPTION '普通档属性条数上限应为2条，实得%',max_count;
    END IF;
    SELECT count(*) INTO n FROM game_monster_color_profiles k
     WHERE NOT EXISTS(SELECT 1 FROM game_equipment_roll_counts c WHERE c.profile_id=k.profile_id);
    IF n<>0 THEN RAISE EXCEPTION '有%个怪物类别指向了没有条数配置的染色档',n; END IF;
END $$;

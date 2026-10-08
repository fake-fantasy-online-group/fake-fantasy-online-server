-- 物品实例事实源。客户端仍只认识模板 item_id；装备与实体镶嵌卡的 UID、
-- 养成状态、随机属性和孔内卡实例全部由服务端保存。
CREATE SEQUENCE IF NOT EXISTS item_instance_legacy_uid_seq START WITH 1;

CREATE TABLE IF NOT EXISTS item_instances (
    uid            BIGINT PRIMARY KEY CHECK (uid > 0),
    item_id        INT NOT NULL CHECK (item_id > 0),
    instance_kind  SMALLINT NOT NULL CHECK (instance_kind IN (1,2,3)),
	bound          BOOLEAN NOT NULL DEFAULT FALSE,
	locked         BOOLEAN NOT NULL DEFAULT FALSE,
    created_source TEXT NOT NULL DEFAULT 'legacy' CHECK (length(created_source) > 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_item_instances_template ON item_instances(item_id);

-- 卡片的属性独立于位置与模板。空效果同样是已确定的实例状态。
CREATE TABLE IF NOT EXISTS card_instances (
    uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
    effect_count SMALLINT NOT NULL DEFAULT 0 CHECK (effect_count BETWEEN 0 AND 8),
    ops INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0,0,0,0] CHECK (cardinality(ops)=8 AND array_position(ops,NULL) IS NULL),
    attrs INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0,0,0,0] CHECK (cardinality(attrs)=8 AND array_position(attrs,NULL) IS NULL),
    modes INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0,0,0,0] CHECK (cardinality(modes)=8 AND array_position(modes,NULL) IS NULL),
    probabilities INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0,0,0,0] CHECK (cardinality(probabilities)=8 AND array_position(probabilities,NULL) IS NULL),
    effect_values INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0,0,0,0] CHECK (cardinality(effect_values)=8 AND array_position(effect_values,NULL) IS NULL)
);

CREATE TABLE IF NOT EXISTS equipment_instances (
    uid                       BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
    durability                INT NOT NULL DEFAULT 0,
    max_durability            INT NOT NULL DEFAULT 0 CHECK (max_durability >= 0),
    durability_wear_raw       INT NOT NULL DEFAULT 0 CHECK (durability_wear_raw BETWEEN 0 AND 299),
    bound                     BOOLEAN NOT NULL DEFAULT FALSE,
    locked                    BOOLEAN NOT NULL DEFAULT FALSE,
    refine_level              INT NOT NULL DEFAULT 0 CHECK (refine_level >= 0),
    socket_count              SMALLINT NOT NULL DEFAULT 0 CHECK (socket_count BETWEEN 0 AND 5),
    socket_item_ids           INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0] CHECK (cardinality(socket_item_ids)=5),
    socket_instance_uids      BIGINT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0] CHECK (cardinality(socket_instance_uids)=5),
    rolled_affix_count        SMALLINT NOT NULL DEFAULT 0 CHECK (rolled_affix_count BETWEEN 0 AND 5),
    rolled_card_ids           INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0] CHECK (cardinality(rolled_card_ids)=5),
    rolled_attrs              INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0] CHECK (cardinality(rolled_attrs)=5),
    rolled_values             INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0] CHECK (cardinality(rolled_values)=5),
    rolled_modes              INT[] NOT NULL DEFAULT ARRAY[0,0,0,0,0] CHECK (cardinality(rolled_modes)=5),
    wash_quality              SMALLINT NOT NULL DEFAULT 0 CHECK (wash_quality BETWEEN 0 AND 4),
    wash_count                SMALLINT NOT NULL DEFAULT 0 CHECK (wash_count BETWEEN 0 AND 4),
    wash_attrs                INT[] NOT NULL DEFAULT ARRAY[0,0,0,0] CHECK (cardinality(wash_attrs)=4),
    wash_values               INT[] NOT NULL DEFAULT ARRAY[0,0,0,0] CHECK (cardinality(wash_values)=4),
    wash_modes                INT[] NOT NULL DEFAULT ARRAY[0,0,0,0] CHECK (cardinality(wash_modes)=4),
    fused_appearance_item_id  INT NOT NULL DEFAULT 0 CHECK (fused_appearance_item_id >= 0),
    fused_soul_item_id        INT NOT NULL DEFAULT 0 CHECK (fused_soul_item_id >= 0)
);

-- 0x1036 kind=1 的部位灵层。唯一名单来自参考服保存的官方客户端内存目录
-- official_fitting_catalog.json（kind=1 共 134 条）；属性直接取这些物品自身
-- game_equip_extra，不从说明文本解析。part -> 目标槽也按官方目录逐组映射。
CREATE TABLE IF NOT EXISTS game_item_equipment_souls (
    item_id     INT PRIMARY KEY CHECK (item_id > 0),
    target_slot SMALLINT NOT NULL CHECK (target_slot IN (1,2,3,4,7,8,9,10,11))
);
CREATE TABLE IF NOT EXISTS game_item_equipment_soul_affixes (
    item_id INT NOT NULL REFERENCES game_item_equipment_souls(item_id) ON DELETE CASCADE,
    seq     SMALLINT NOT NULL CHECK (seq > 0),
    attr_id INT NOT NULL CHECK (attr_id > 0),
    value   INT NOT NULL,
    mode    INT NOT NULL CHECK (mode IN (0,1)),
    PRIMARY KEY(item_id,seq)
);

DELETE FROM game_item_equipment_souls WHERE TRUE;

WITH official(item_id) AS (
    SELECT unnest(ARRAY[
        23624,23625,23627,23629,23630,23632,23704,23705,23708,23709,23710,23711,
        23714,23715,23716,23717,23718,23729,23730,23731,23786,23787,23788,23789,
        23790,23791,23792,23793,23798,23799,23800,23801,23804,23819,23822,23823,
        23828,23829,23830,23831,23834,23835,23836,23837,23891,23892,23893,23894,
        23895,23968,23969,23970,24618,24619,24620,24621,24622,24623,24624,24625,
        24626,24627,24630,24631,24632,24633,24634,24635,24636,24637,24638,24639,
        29835,29836,29837,29838,29839,29840,29841,29842,29843,29844,
        29849,29850,29851,29852,29853,29854,29855,29856,29857,29858,
        29887,29888,29889,29890,29891,29892,29893,29894,29895,29896,
        47491,47492,47493,47494,47495,47496,47497,47498,47499,47500,
        47502,47503,47504,47505,47506,47507,47508,47509,47510,47511,47513,47514,
        47641,47642,47643,47644,47645,47646,47647,47648,47649,47650
    ]::INT[])
)
INSERT INTO game_item_equipment_souls(item_id,target_slot)
SELECT o.item_id,
       CASE WHEN o.item_id BETWEEN 47491 AND 47500 THEN 1
            WHEN o.item_id BETWEEN 29849 AND 29858 THEN 2
            WHEN o.item_id BETWEEN 24618 AND 24627 THEN 3
            WHEN o.item_id BETWEEN 29835 AND 29844 OR o.item_id IN(47513,47514) THEN 4
            WHEN o.item_id BETWEEN 23624 AND 23970 THEN 7
            WHEN o.item_id BETWEEN 29887 AND 29896 THEN 8
            WHEN o.item_id BETWEEN 24630 AND 24639 THEN 9
            WHEN o.item_id BETWEEN 47502 AND 47511 THEN 10
            WHEN o.item_id BETWEEN 47641 AND 47650 THEN 11 END
  FROM official o
ON CONFLICT(item_id) DO UPDATE SET target_slot=EXCLUDED.target_slot;

INSERT INTO game_item_equipment_soul_affixes(item_id,seq,attr_id,value,mode)
SELECT x.arm_id,row_number() OVER(PARTITION BY x.arm_id ORDER BY x.seq)::SMALLINT,
       x.attr_id,x.value,x.mode
  FROM game_equip_extra x
  JOIN game_item_equipment_souls s ON s.item_id=x.arm_id
 WHERE x.prob>=100 AND x.attr_name<>''
ON CONFLICT(item_id,seq) DO UPDATE SET attr_id=EXCLUDED.attr_id,value=EXCLUDED.value,mode=EXCLUDED.mode;

DO $$
DECLARE n INT;
BEGIN
    SELECT count(*) INTO n FROM game_item_equipment_souls;
    IF n<>134 THEN RAISE EXCEPTION '官方试衣目录部位灵应为134件，实得%',n; END IF;
    SELECT count(*) INTO n FROM game_item_equipment_soul_affixes;
    IF n<>461 THEN RAISE EXCEPTION '官方部位灵属性应为461行，实得%',n; END IF;
    SELECT count(*) INTO n FROM game_item_equipment_souls s
     WHERE NOT EXISTS(SELECT 1 FROM game_item_equipment_soul_affixes a WHERE a.item_id=s.item_id);
    IF n<>0 THEN RAISE EXCEPTION '有%件装备灵/元素缺少属性',n; END IF;
    SELECT count(*) INTO n FROM game_item_equipment_souls s
      LEFT JOIN game_equipment e ON e.id=s.item_id WHERE e.id IS NULL;
    IF n<>0 THEN RAISE EXCEPTION '有%件官方部位灵缺少装备模板',n; END IF;
END $$;

-- 失落神殿三层首领形态链。公开攻略给出顺序，怪物 ID、数值与掉落均来自
-- game_monsters/game_monster_drops；下一阶段继承触发实体的位置。
CREATE TABLE IF NOT EXISTS game_dungeon_encounter_steps (
    map_id             INT NOT NULL DEFAULT 0 REFERENCES map_defs(id),
    step_no            INT NOT NULL DEFAULT 0 CHECK (step_no >= 0),
    trigger_monster_id INT NOT NULL DEFAULT 0 REFERENCES game_monsters(id),
    next_monster_id    INT NOT NULL DEFAULT 0 REFERENCES game_monsters(id),
    PRIMARY KEY (map_id, step_no),
    UNIQUE (map_id, trigger_monster_id),
    CHECK (trigger_monster_id <> next_monster_id)
);

INSERT INTO game_dungeon_encounter_steps(map_id, step_no, trigger_monster_id, next_monster_id)
VALUES
  (20903, 0, 3124, 3125),
  (20903, 1, 3125, 3127),
  (20903, 2, 3127, 3126)
ON CONFLICT (map_id, step_no) DO UPDATE SET
  trigger_monster_id = EXCLUDED.trigger_monster_id,
  next_monster_id = EXCLUDED.next_monster_id;

-- 普通装备随机属性配置。运行时只读已经展开的整数份额；曲线参数、卡池边界
-- 和离线展开结果都在 PostgreSQL，服务端不硬编码概率或数值范围。
CREATE TABLE IF NOT EXISTS game_equipment_roll_profiles (
    id          SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    algorithm   TEXT NOT NULL DEFAULT 'lognormal_mode_at_max'
        CHECK (algorithm = 'lognormal_mode_at_max'),
    sigma       DOUBLE PRECISION NOT NULL DEFAULT 0.45 CHECK (sigma > 0),
    attr_weight BIGINT NOT NULL DEFAULT 1000000 CHECK (attr_weight > 0)
);

CREATE TABLE IF NOT EXISTS game_equipment_roll_card_ranges (
    seq         SMALLINT PRIMARY KEY CHECK (seq > 0),
    min_card_id INT NOT NULL DEFAULT 0 CHECK (min_card_id > 0),
    max_card_id INT NOT NULL DEFAULT 0 CHECK (max_card_id >= min_card_id)
);

CREATE TABLE IF NOT EXISTS game_equipment_roll_counts (
    attr_count SMALLINT PRIMARY KEY CHECK (attr_count BETWEEN 0 AND 5),
    weight     BIGINT NOT NULL CHECK (weight > 0)
);

CREATE TABLE IF NOT EXISTS game_equipment_roll_options (
    equipment_level INT NOT NULL CHECK (equipment_level >= 0),
    equipment_type  INT NOT NULL DEFAULT 0 CHECK (equipment_type >= 0),
    card_id         INT NOT NULL CHECK (card_id > 0),
    attr_id         INT NOT NULL CHECK (attr_id > 0),
    value           INT NOT NULL,
    mode            INT NOT NULL CHECK (mode IN (0,1)),
    weight          BIGINT NOT NULL CHECK (weight > 0),
    PRIMARY KEY (equipment_level,equipment_type,card_id,attr_id,value,mode)
);

INSERT INTO game_equipment_roll_profiles(id,algorithm,sigma,attr_weight)
VALUES (1,'lognormal_mode_at_max',0.45,1000000)
ON CONFLICT(id) DO UPDATE SET
    algorithm=EXCLUDED.algorithm,
    sigma=EXCLUDED.sigma,
    attr_weight=EXCLUDED.attr_weight;

INSERT INTO game_equipment_roll_card_ranges(seq,min_card_id,max_card_id) VALUES
    (1,5001,5997),
    (2,13141,13191)
ON CONFLICT(seq) DO UPDATE SET
    min_card_id=EXCLUDED.min_card_id,
    max_card_id=EXCLUDED.max_card_id;

INSERT INTO game_equipment_roll_counts(attr_count,weight) VALUES
    (0,60),(1,20),(2,10),(3,5),(4,3),(5,1)
ON CONFLICT(attr_count) DO UPDATE SET weight=EXCLUDED.weight;

-- ov_desc.type 与 ov_card 部位开关的映射来自客户端静态定义。只为
-- no_type_drop=0 的实际普通装备等级/类型生成矩阵；固定 item_id 掉落不使用它。
-- 每个属性先在自己的 mode 内按对数正态密度归一，再在同一 attr_id 的绝对值/
-- 百分比模式间均分，因此不同属性不会因为可选数值档更多而天然更容易出现。
DELETE FROM game_equipment_roll_options WHERE equipment_level>=0;
WITH type_map(equipment_type,part) AS (VALUES
    (1,'spear'),(2,'single_sword'),(4,'dual_sword'),(8,'dagger'),
    (16,'staff'),(32,'throw'),(101,'face'),(102,'hat'),
    (103,'necklace'),(105,'shield'),(106,'glove'),(107,'ring'),
    (108,'dress'),(109,'shoe'),(110,'bag'),(114,'ear'),
    (115,'bangle'),(116,'belt')
), levels AS (
    SELECT DISTINCT a.level AS equipment_level,d.type AS equipment_type
      FROM gamedata.ov_arm a
      JOIN gamedata.ov_desc d ON d.index=a.index
      JOIN type_map tm ON tm.equipment_type=d.type
     WHERE a.index<>0 AND a.no_type_drop=0 AND a.level BETWEEN 1 AND 17
), cards AS (
    SELECT c.index AS card_id,c.min_level,c.max_level,
           e.attr_id,e.value,e.mode,
           c.spear,c.single_sword,c.dual_sword,c.dagger,c.staff,c.throw,
           c.face,c.hat,c.necklace,c.shield,c.glove,c.ring,c.dress,c.shoe,
           c.bag,c.ear,c.bangle,c.belt
      FROM gamedata.ov_card c
      JOIN gamedata.ov_card_entry e USING(row_no)
      JOIN game_equipment_roll_card_ranges r
        ON c.index BETWEEN r.min_card_id AND r.max_card_id
     WHERE c.index<>0 AND c.category=0 AND c.fun_type=0
       AND e.op_type=1 AND e.prob=100 AND e.mode IN(0,1)
       AND e.attr_id IN(1,3,5,7,9,61,13,15,17,19,23,25,27,29,31,
                        34,36,38,40,47,66,69,70,118)
), expanded AS (
    SELECT l.equipment_level,l.equipment_type,
           c.card_id,c.attr_id,c.value,c.mode
      FROM levels l
      JOIN type_map tm USING(equipment_type)
      JOIN cards c
        ON c.min_level<=l.equipment_level AND c.max_level>=l.equipment_level
       AND CASE tm.part
           WHEN 'spear' THEN c.spear WHEN 'single_sword' THEN c.single_sword
           WHEN 'dual_sword' THEN c.dual_sword WHEN 'dagger' THEN c.dagger
           WHEN 'staff' THEN c.staff WHEN 'throw' THEN c.throw
           WHEN 'face' THEN c.face WHEN 'hat' THEN c.hat
           WHEN 'necklace' THEN c.necklace WHEN 'shield' THEN c.shield
           WHEN 'glove' THEN c.glove WHEN 'ring' THEN c.ring
           WHEN 'dress' THEN c.dress WHEN 'shoe' THEN c.shoe
           WHEN 'bag' THEN c.bag WHEN 'ear' THEN c.ear
           WHEN 'bangle' THEN c.bangle WHEN 'belt' THEN c.belt
           ELSE 0 END<>0
), bounds AS (
    SELECT e.*,
           min(abs(value)) OVER(
               PARTITION BY equipment_level,equipment_type,attr_id,mode) AS lo,
           max(abs(value)) OVER(
               PARTITION BY equipment_level,equipment_type,attr_id,mode) AS hi
      FROM expanded e
), scored AS (
    SELECT b.*,
           CASE WHEN hi=lo THEN 1.0 ELSE
               exp(-power(
                   ln(1.0+(abs(value)-lo)::DOUBLE PRECISION/(hi-lo))
                     -(ln(2.0)+p.sigma*p.sigma),2
               )/(2*p.sigma*p.sigma))
               /(1.0+(abs(value)-lo)::DOUBLE PRECISION/(hi-lo))
           END AS score
      FROM bounds b CROSS JOIN game_equipment_roll_profiles p
     WHERE p.id=1
), normalized AS (
    SELECT s.*,
           score/sum(score) OVER(
               PARTITION BY equipment_level,equipment_type,attr_id,mode) AS mode_share
      FROM scored s
), mode_counts AS (
    SELECT equipment_level,equipment_type,attr_id,count(DISTINCT mode) AS modes
      FROM expanded GROUP BY equipment_level,equipment_type,attr_id
)
INSERT INTO game_equipment_roll_options(
    equipment_level,equipment_type,card_id,attr_id,value,mode,weight)
SELECT n.equipment_level,n.equipment_type,n.card_id,n.attr_id,n.value,n.mode,
       greatest(1,round(p.attr_weight*n.mode_share/m.modes))::BIGINT
  FROM normalized n
  JOIN mode_counts m USING(equipment_level,equipment_type,attr_id)
  CROSS JOIN game_equipment_roll_profiles p
 WHERE p.id=1;

DO $$
DECLARE n INT;
BEGIN
    SELECT count(*) INTO n FROM game_equipment_roll_counts;
    IF n<>6 THEN RAISE EXCEPTION '装备随机属性条数档应为6，实得%',n; END IF;
    SELECT count(*) INTO n FROM game_equipment_roll_options;
    IF n<>16583 THEN RAISE EXCEPTION '装备随机属性结果份额应为16583，实得%',n; END IF;
    SELECT count(DISTINCT (equipment_level,equipment_type)) INTO n
      FROM game_equipment_roll_options;
    IF n<>135 THEN RAISE EXCEPTION '装备随机属性等级类型键应为135，实得%',n; END IF;
    SELECT count(*) INTO n FROM (
        SELECT equipment_level,equipment_type
          FROM game_equipment_roll_options
         GROUP BY equipment_level,equipment_type
        HAVING count(DISTINCT attr_id)<5
    ) insufficient;
    IF n<>0 THEN RAISE EXCEPTION '有%个装备随机属性池不足5种属性',n; END IF;
END $$;

-- 固定装备怪阶段：先按 rate_pct 正常掷，低于 minimum 时按 weight 补足，
-- 然后才执行原有 game_monster_drops。原始 kind 可能写“精英”或“普通”，
-- 因此最终/小 BOSS 均由地图和怪物号显式配置。当前保持空表待确认。
CREATE TABLE IF NOT EXISTS game_boss_fixed_equipment_pool (
    map_id      INT NOT NULL CHECK (map_id > 0),
    monster_id INT NOT NULL CHECK (monster_id > 0),
    seq         SMALLINT NOT NULL CHECK (seq > 0),
    minimum     SMALLINT NOT NULL CHECK (minimum BETWEEN 1 AND 2),
    item_id     INT NOT NULL CHECK (item_id > 0),
    rate_pct    DOUBLE PRECISION NOT NULL CHECK (rate_pct BETWEEN 0 AND 100),
    weight      BIGINT NOT NULL CHECK (weight > 0),
	roll_affixes BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (map_id,monster_id,seq),
    UNIQUE (map_id,monster_id,item_id)
);

-- 用户确认：精英的显式固定装备掉率统一提高到 1%。只更新实际存在刷怪点的
-- 精英；3125/3126 虽然模板 kind 写成精英，但属于动态最终 BOSS，保留自己的
-- 5%/10%/2% 固定池概率。
UPDATE game_monster_drops d
   SET rate=1
  FROM game_monsters m,game_equipment e
 WHERE d.monster_id=m.id AND d.item_id=e.id AND m.kind='精英'
   AND EXISTS(SELECT 1 FROM map_monster_spawns s WHERE s.monster=m.id);

-- BOSS 固定装备池。除明确列出的最终 BOSS 外最低 1 件；最终 BOSS 最低 2 件。
-- 这里只配置掉落，不实现阿卡哈/图哈特的阶段生成机制。
WITH fixed_defs(map_id,monster_id,minimum) AS (VALUES
    (20600,3059,1),(20600,3060,1),(20600,3061,1),(20600,3062,1),(20600,3063,1),
    (20701,3080,1),(20701,3081,1),(20701,3082,2),
    (20801,3067,2),(20802,3071,2),(20803,3075,2),
    (20901,3115,1),(20901,3116,1),(20901,3117,1),
    (20902,3118,1),(20902,3119,1),
    (20903,3125,2),(20903,3126,2)
), grouped AS (
    SELECT f.map_id,f.monster_id,f.minimum,d.item_id,count(*)::BIGINT AS copies,
           CASE WHEN bool_or(d.rate>=100) THEN 100::DOUBLE PRECISION
                ELSE 100*(1-exp(sum(ln(1-d.rate::DOUBLE PRECISION/100)))) END AS rate_pct
      FROM fixed_defs f
      JOIN game_monster_drops d ON d.monster_id=f.monster_id AND d.item_id>0
      JOIN game_equipment e ON e.id=d.item_id
     GROUP BY f.map_id,f.monster_id,f.minimum,d.item_id
), numbered AS (
    SELECT *,row_number() OVER(PARTITION BY map_id,monster_id ORDER BY item_id)::SMALLINT seq
      FROM grouped
)
INSERT INTO game_boss_fixed_equipment_pool(map_id,monster_id,seq,minimum,item_id,rate_pct,weight,roll_affixes)
SELECT map_id,monster_id,seq,minimum,item_id,rate_pct,copies,FALSE FROM numbered
ON CONFLICT(map_id,monster_id,item_id) DO UPDATE SET
    minimum=EXCLUDED.minimum,rate_pct=EXCLUDED.rate_pct,weight=EXCLUDED.weight,
    roll_affixes=EXCLUDED.roll_affixes;

DO $$
DECLARE n INT;
BEGIN
    SELECT count(DISTINCT (map_id,monster_id)) INTO n FROM game_boss_fixed_equipment_pool;
    IF n<>18 THEN RAISE EXCEPTION '固定装备BOSS应为18只，实得%',n; END IF;
    SELECT count(*) INTO n FROM game_boss_fixed_equipment_pool
     WHERE minimum=2 AND monster_id NOT IN(3082,3067,3071,3075,3125,3126);
    IF n<>0 THEN RAISE EXCEPTION '有%条非最终BOSS配置成2件保底',n; END IF;
    SELECT count(DISTINCT monster_id) INTO n FROM game_boss_fixed_equipment_pool WHERE minimum=2;
    IF n<>6 THEN RAISE EXCEPTION '最终BOSS应为6只，实得%',n; END IF;
END $$;

-- ===== 副本怪通用掉落补全 =====
--
-- 用户确认的叠加语义：
--   * 副本 BOSS 保留自己的收集品和固定装备，再追加等级最接近的光圈怪掉落模板；
--   * 副本小怪保留自己的全部掉落，再追加一份同等级普通怪模板，概率乘 2；
--   * BOSS 原始掉落中 ov_item.query_type=27 的收集品必掉。
--
-- 映射单独落库，方便审计“谁复用了谁的模板”；真正运行时掉落仍物化到
-- game_monster_drops，游戏服不在每次击杀时做配置关联查询。
CREATE TABLE IF NOT EXISTS game_dungeon_drop_templates (
    monster_id           INT PRIMARY KEY REFERENCES game_monsters(id),
    role                 TEXT NOT NULL CHECK (role IN ('boss','small')),
    source_monster_id    INT NOT NULL REFERENCES game_monsters(id),
    rate_multiplier      DOUBLE PRECISION NOT NULL CHECK (rate_multiplier BETWEEN 1 AND 2),
    collection_guaranteed BOOLEAN NOT NULL DEFAULT FALSE,
    CHECK (monster_id<>source_monster_id),
    CHECK ((role='boss' AND collection_guaranteed AND rate_multiplier=1)
        OR (role='small' AND NOT collection_guaranteed AND rate_multiplier=2))
);
CREATE INDEX IF NOT EXISTS idx_dungeon_drop_template_source
    ON game_dungeon_drop_templates(source_monster_id);

-- init 分片可重放：先清掉上一次物化的模板行，原始掉落始终保留。
DELETE FROM game_monster_drops WHERE item_src LIKE 'dungeon_template:%';
DELETE FROM game_dungeon_drop_templates WHERE TRUE;

WITH dungeon_targets AS (
    SELECT DISTINCT m.id,m.name,m.kind,m.level,
           CASE WHEN m.kind='BOSS' OR m.name LIKE '%（头目）%' OR m.name ILIKE '%BOSS%'
                THEN 'boss' ELSE 'small' END AS role
      FROM map_monster_spawns s
      JOIN game_monsters m ON m.id=s.monster
     WHERE s.map_file LIKE 'pw%'
       AND m.kind NOT IN ('场景物件','采集物','')
), source_counts AS (
    SELECT m.id,m.level,count(*)::INT AS row_count
      FROM game_monsters m
      JOIN game_monster_drops d ON d.monster_id=m.id
     WHERE d.item_src NOT LIKE 'dungeon_template:%'
     GROUP BY m.id,m.level
), light_ring_sources AS (
    SELECT DISTINCT elite.id,elite.level,c.row_count
      FROM game_monsters base
      JOIN game_monsters elite ON elite.id=base.elite_id
      JOIN source_counts c ON c.id=elite.id
     WHERE base.elite_id<>0
), ordinary_sources AS (
    SELECT m.id,m.level,c.row_count
      FROM game_monsters m
      JOIN source_counts c ON c.id=m.id
     WHERE m.kind='普通'
       AND NOT EXISTS (
           SELECT 1 FROM map_monster_spawns s
            WHERE s.monster=m.id AND s.map_file LIKE 'pw%'
       )
), chosen AS (
    SELECT target.id AS monster_id,target.role,source.id AS source_monster_id,
           1::DOUBLE PRECISION AS rate_multiplier,TRUE AS collection_guaranteed
      FROM dungeon_targets target
      JOIN LATERAL (
          SELECT candidate.id
            FROM light_ring_sources candidate
           ORDER BY abs(candidate.level-target.level),candidate.row_count DESC,candidate.id
           LIMIT 1
      ) source ON target.role='boss'
    UNION ALL
    SELECT target.id,target.role,source.id,
           2::DOUBLE PRECISION,FALSE
      FROM dungeon_targets target
      JOIN LATERAL (
          SELECT candidate.id
            FROM ordinary_sources candidate
           WHERE candidate.level=target.level
           ORDER BY candidate.row_count DESC,candidate.id
           LIMIT 1
      ) source ON target.role='small'
)
INSERT INTO game_dungeon_drop_templates(
    monster_id,role,source_monster_id,rate_multiplier,collection_guaranteed)
SELECT monster_id,role,source_monster_id,rate_multiplier,collection_guaranteed
  FROM chosen
ON CONFLICT(monster_id) DO UPDATE SET
    role=EXCLUDED.role,source_monster_id=EXCLUDED.source_monster_id,
    rate_multiplier=EXCLUDED.rate_multiplier,
    collection_guaranteed=EXCLUDED.collection_guaranteed;

-- 只把 BOSS 自己原始掉落里的收集品改为必掉；后面追加的模板药品/材料
-- 保留模板概率，不能因为是 item 行就全部必掉。
UPDATE game_monster_drops drop_row
   SET rate=100
  FROM game_dungeon_drop_templates template
 WHERE template.monster_id=drop_row.monster_id
   AND template.role='boss' AND template.collection_guaranteed
   AND drop_row.item_id>0
   AND drop_row.item_src NOT LIKE 'dungeon_template:%'
   AND EXISTS (
       SELECT 1 FROM gamedata.ov_item item
        WHERE item.index=drop_row.item_id AND item.query_type=27
   );

WITH base_seq AS (
    SELECT monster_id,COALESCE(max(seq),-1) AS max_seq
      FROM game_monster_drops
     GROUP BY monster_id
), generated AS (
    SELECT template.monster_id,
           COALESCE(base_seq.max_seq,-1)
             + row_number() OVER(PARTITION BY template.monster_id ORDER BY source.seq) AS seq,
           LEAST(100::DOUBLE PRECISION,
                 source.rate::DOUBLE PRECISION*template.rate_multiplier)::REAL AS rate,
           source.item_id,source.item_name,
           format('dungeon_template:%s:%s',template.source_monster_id,template.role) AS item_src,
           source.kind_name,source.lv_min,source.lv_max
      FROM game_dungeon_drop_templates template
      JOIN game_monster_drops source ON source.monster_id=template.source_monster_id
       AND source.item_src NOT LIKE 'dungeon_template:%'
      LEFT JOIN base_seq ON base_seq.monster_id=template.monster_id
)
INSERT INTO game_monster_drops(
    monster_id,seq,rate,item_id,item_name,item_src,kind_name,lv_min,lv_max)
SELECT monster_id,seq,rate,item_id,item_name,item_src,kind_name,lv_min,lv_max
  FROM generated
ORDER BY monster_id,seq;

DO $$
DECLARE n INT; eye_rate REAL;
BEGIN
    SELECT count(*) INTO n
      FROM map_monster_spawns spawn
      JOIN game_monsters monster ON monster.id=spawn.monster
     WHERE spawn.map_file LIKE 'pw%'
       AND monster.kind NOT IN ('场景物件','采集物','')
       AND NOT EXISTS (
           SELECT 1 FROM game_dungeon_drop_templates template
            WHERE template.monster_id=monster.id
       );
    IF n<>0 THEN RAISE EXCEPTION '有% 条副本战斗怪落位缺少掉落模板',n; END IF;

    SELECT count(*) INTO n
      FROM game_dungeon_drop_templates template
      JOIN game_monsters target ON target.id=template.monster_id
      JOIN game_monsters source ON source.id=template.source_monster_id
     WHERE template.role='small' AND target.level<>source.level;
    IF n<>0 THEN RAISE EXCEPTION '有% 只副本小怪没有叠加同等级普通怪模板',n; END IF;

    SELECT count(*) INTO n
      FROM game_dungeon_drop_templates template
     WHERE template.role='boss' AND NOT EXISTS (
         SELECT 1 FROM game_monsters base
          WHERE base.elite_id=template.source_monster_id
     );
    IF n<>0 THEN RAISE EXCEPTION '有% 只副本BOSS的补全源不是光圈怪',n; END IF;

    SELECT count(*) INTO n
      FROM game_dungeon_drop_templates template
      JOIN LATERAL (
          SELECT count(*) AS source_count
            FROM game_monster_drops source
           WHERE source.monster_id=template.source_monster_id
             AND source.item_src NOT LIKE 'dungeon_template:%'
      ) source_rows ON TRUE
      JOIN LATERAL (
          SELECT count(*) AS generated_count
            FROM game_monster_drops generated
           WHERE generated.monster_id=template.monster_id
             AND generated.item_src=
                 format('dungeon_template:%s:%s',template.source_monster_id,template.role)
      ) generated_rows ON TRUE
     WHERE source_rows.source_count<>generated_rows.generated_count;
    IF n<>0 THEN RAISE EXCEPTION '有% 只副本怪的掉落模板没有完整追加',n; END IF;

    SELECT rate INTO eye_rate
      FROM game_monster_drops
     WHERE monster_id=3082 AND item_id=4476
       AND item_src NOT LIKE 'dungeon_template:%'
     ORDER BY seq LIMIT 1;
    IF eye_rate IS DISTINCT FROM 100::REAL THEN
        RAISE EXCEPTION '下级天魔·斩首者的天魔之眼必须100%%掉落，实得%',eye_rate;
    END IF;
END $$;

SELECT setval('item_instance_legacy_uid_seq',GREATEST(COALESCE((
    SELECT max(uid) FROM (
        SELECT uid FROM character_items UNION ALL SELECT uid FROM character_equips
        UNION ALL SELECT uid FROM character_warehouse_items
        UNION ALL SELECT uid FROM character_change_set_items
        UNION ALL SELECT uid FROM character_stall_items
        UNION ALL SELECT uid FROM character_mail_attachments
        UNION ALL SELECT uid FROM game_family_stash_items
    ) located
),0)+1,1),FALSE);

DO $$
DECLARE bad INT;
BEGIN
    SELECT count(*) INTO bad FROM (
        SELECT item_id,count FROM character_items
        UNION ALL SELECT item_id,count FROM character_warehouse_items
        UNION ALL SELECT item_id,count FROM character_stall_items
        UNION ALL SELECT item_id,count FROM character_mail_attachments
        UNION ALL SELECT item_id,count FROM game_family_stash_items
    ) p
    WHERE p.count<>1 AND EXISTS(
        SELECT 1 FROM gamedata.ov_card c WHERE c.index=p.item_id AND c.item_id=p.item_id);
    IF bad<>0 THEN
        RAISE EXCEPTION '有%格实体镶嵌卡仍在堆叠，必须先逐件拆格再升级',bad;
    END IF;
END $$;

-- 旧位置表的装备/实体卡此前 uid=0；升级时逐件分配，不能让同模板的多件
-- 装备继续共享“0”这个伪身份。
UPDATE character_items p SET uid=nextval('item_instance_legacy_uid_seq')
 WHERE uid=0 AND (EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=p.item_id)
    OR EXISTS(SELECT 1 FROM gamedata.ov_card c WHERE c.index=p.item_id AND c.item_id=p.item_id));
UPDATE character_equips p SET uid=nextval('item_instance_legacy_uid_seq') WHERE uid=0;
UPDATE character_warehouse_items p SET uid=nextval('item_instance_legacy_uid_seq')
 WHERE uid=0 AND (EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=p.item_id)
    OR EXISTS(SELECT 1 FROM gamedata.ov_card c WHERE c.index=p.item_id AND c.item_id=p.item_id));
UPDATE character_change_set_items p SET uid=nextval('item_instance_legacy_uid_seq') WHERE uid=0;
UPDATE character_stall_items p SET uid=nextval('item_instance_legacy_uid_seq')
 WHERE uid=0 AND (EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=p.item_id)
    OR EXISTS(SELECT 1 FROM gamedata.ov_card c WHERE c.index=p.item_id AND c.item_id=p.item_id));
UPDATE character_mail_attachments p SET uid=nextval('item_instance_legacy_uid_seq')
 WHERE uid=0 AND (EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=p.item_id)
    OR EXISTS(SELECT 1 FROM gamedata.ov_card c WHERE c.index=p.item_id AND c.item_id=p.item_id));
UPDATE game_family_stash_items p SET uid=nextval('item_instance_legacy_uid_seq')
 WHERE uid=0 AND (EXISTS(SELECT 1 FROM game_equipment e WHERE e.id=p.item_id)
    OR EXISTS(SELECT 1 FROM gamedata.ov_card c WHERE c.index=p.item_id AND c.item_id=p.item_id));

WITH located AS (
    SELECT uid,item_id,bound,locked FROM character_items WHERE uid>0
    UNION ALL SELECT uid,item_id,bound,locked FROM character_equips WHERE uid>0
    UNION ALL SELECT uid,item_id,bound,locked FROM character_warehouse_items WHERE uid>0
    UNION ALL SELECT uid,item_id,bound,locked FROM character_change_set_items WHERE uid>0
    UNION ALL SELECT uid,item_id,bound,locked FROM character_stall_items WHERE uid>0
    UNION ALL SELECT uid,item_id,bound,locked FROM character_mail_attachments WHERE uid>0
    UNION ALL SELECT uid,item_id,bound,locked FROM game_family_stash_items WHERE uid>0
), unique_located AS (
    SELECT DISTINCT ON (uid) uid,item_id,bound,locked FROM located ORDER BY uid,item_id
)
INSERT INTO item_instances(uid,item_id,instance_kind,bound,locked,created_source)
SELECT l.uid,l.item_id,CASE WHEN e.id IS NOT NULL THEN 1 ELSE 2 END,l.bound,l.locked,'legacy'
  FROM unique_located l
  LEFT JOIN game_equipment e ON e.id=l.item_id
 WHERE e.id IS NOT NULL OR EXISTS(
       SELECT 1 FROM gamedata.ov_card c WHERE c.index=l.item_id AND c.item_id=l.item_id)
ON CONFLICT(uid) DO UPDATE SET item_id=EXCLUDED.item_id,
    instance_kind=EXCLUDED.instance_kind,updated_at=now();

WITH equipment_locations AS (
    SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
           socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
           fused_appearance_item_id FROM character_items WHERE uid>0
    UNION ALL
    SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
           socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
           fused_appearance_item_id FROM character_equips WHERE uid>0
    UNION ALL
    SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
           socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
           fused_appearance_item_id FROM character_warehouse_items WHERE uid>0
    UNION ALL
    SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
           socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
           fused_appearance_item_id FROM character_change_set_items WHERE uid>0
    UNION ALL
    SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
           socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
           fused_appearance_item_id FROM character_stall_items WHERE uid>0
    UNION ALL
    SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
           socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
           fused_appearance_item_id FROM character_mail_attachments WHERE uid>0
    UNION ALL
    SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
           socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
           fused_appearance_item_id FROM game_family_stash_items WHERE uid>0
), one_location AS (
    SELECT DISTINCT ON (l.uid) l.* FROM equipment_locations l
      JOIN item_instances i ON i.uid=l.uid AND i.instance_kind=1
     ORDER BY l.uid
)
INSERT INTO equipment_instances(uid,durability,max_durability,durability_wear_raw,bound,locked,
    refine_level,socket_count,socket_item_ids,wash_quality,wash_count,wash_attrs,wash_values,
    wash_modes,fused_appearance_item_id)
SELECT uid,durability,max_durability,durability_wear_raw,bound,locked,refine_level,
       socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
       fused_appearance_item_id
  FROM one_location
ON CONFLICT(uid) DO NOTHING;

-- 当前开发库没有已镶嵌历史数据；仍为将来导入旧档保留严格回填：孔里每张卡
-- 获得自己的实例号，并写入装备事实源，不再靠 item_id 重建。
DO $$
DECLARE eq RECORD; ids BIGINT[]; i INT; card_uid BIGINT;
BEGIN
    FOR eq IN SELECT uid,socket_item_ids FROM equipment_instances LOOP
        ids := ARRAY[0,0,0,0,0]::BIGINT[];
        FOR i IN 1..5 LOOP
            IF eq.socket_item_ids[i] > 0 THEN
                card_uid := nextval('item_instance_legacy_uid_seq');
                INSERT INTO item_instances(uid,item_id,instance_kind,created_source)
                VALUES(card_uid,eq.socket_item_ids[i],2,'legacy_socket');
                ids[i] := card_uid;
            END IF;
        END LOOP;
        UPDATE equipment_instances SET socket_instance_uids=ids WHERE uid=eq.uid;
    END LOOP;
END $$;

DO $$
DECLARE bad INT;
BEGIN
    SELECT count(*) INTO bad FROM item_instances i
     LEFT JOIN equipment_instances e ON e.uid=i.uid
     WHERE i.instance_kind=1 AND e.uid IS NULL;
    IF bad <> 0 THEN RAISE EXCEPTION '有%件装备缺少 equipment_instances 状态',bad; END IF;
    SELECT count(*) INTO bad FROM equipment_instances e
     JOIN item_instances i ON i.uid=e.uid
     WHERE i.instance_kind<>1;
    IF bad <> 0 THEN RAISE EXCEPTION '有%条装备状态关联到非装备实例',bad; END IF;
END $$;

-- 旧卡没有独立属性记录：仅在升级时从实体卡自己的原始定义固化一次。
-- 不拼接其它同 item_id 的装备词条，也不在登录/镶嵌时重新生成。
INSERT INTO card_instances(uid,effect_count,ops,attrs,modes,probabilities,effect_values)
SELECT i.uid,COALESCE(max(e.idx+1) FILTER(WHERE e.op_type<>0),0),
 array_agg(COALESCE(e.op_type,0) ORDER BY n.idx),
 array_agg(COALESCE(e.attr_id,0) ORDER BY n.idx),
 array_agg(COALESCE(e.mode,0)::INT ORDER BY n.idx),
 array_agg(COALESCE(e.prob,0)::INT ORDER BY n.idx),
 array_agg(COALESCE(e.value,0) ORDER BY n.idx)
FROM item_instances i
JOIN gamedata.ov_card c ON c.index=i.item_id AND c.item_id=i.item_id
CROSS JOIN generate_series(0,7) n(idx)
LEFT JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no AND e.idx=n.idx
WHERE i.instance_kind=2 AND NOT EXISTS(SELECT 1 FROM card_instances saved WHERE saved.uid=i.uid)
GROUP BY i.uid;

-- 卡片归收集页。逐张分配该玩家页内空位，容量不足就回滚整笔升级。
DO $$
DECLARE card RECORD; destination INT;
BEGIN
 FOR card IN
  SELECT p.char_id,p.slot,p.uid FROM character_items p
  JOIN item_instances i ON i.uid=p.uid AND i.instance_kind=2
  WHERE p.slot NOT BETWEEN 100 AND 199 ORDER BY p.char_id,p.slot
 LOOP
  SELECT n INTO destination FROM generate_series(100,199) n
  WHERE NOT EXISTS(SELECT 1 FROM character_items p WHERE p.char_id=card.char_id AND p.slot=n)
  ORDER BY n LIMIT 1;
  IF destination IS NULL THEN RAISE EXCEPTION '角色 % 收集页没有足够空间迁移卡片',card.char_id; END IF;
  UPDATE character_items SET slot=destination WHERE char_id=card.char_id AND slot=card.slot AND uid=card.uid;
 END LOOP;
 IF EXISTS(SELECT 1 FROM item_instances i LEFT JOIN card_instances c USING(uid) WHERE i.instance_kind=2 AND c.uid IS NULL)
 THEN RAISE EXCEPTION '有卡片实例缺少原始属性，拒绝生成不完整存档'; END IF;
END $$;

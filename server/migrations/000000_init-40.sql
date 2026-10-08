-- 角色直接经验道具：运行时只查本表，不读取粉粉兔网站或本地研究文件。
-- 原始物品脚本：12090=Item_3tao，12091=Item_6tao，12092=Item_9tao，25355=Item_bxexp。
-- 三种蟠桃沿用 https://fenfentu.org/character-exp 的分段公式。
-- 八仙灵丹：53/61/115/122/129 为网站样本；85..110 为网站公式；
-- 其余采用用户授权的线性插值/二次拟合。111 是拟合启用边界，取整约定向下。
-- 53 以下不外推八仙灵丹。配置保留至150级，运行时仍遵守角色等级上限。
CREATE TABLE game_item_player_experience (
    item_id INT NOT NULL REFERENCES game_items(id),
    player_level INT NOT NULL REFERENCES game_levels(level),
    experience BIGINT NOT NULL CHECK (experience > 0),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('sample','formula','interpolated','extrapolated')),
    source_url TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (item_id,player_level)
);

INSERT INTO game_item_player_experience(item_id,player_level,experience,source_kind,source_url)
SELECT item_id,level,
       CASE item_id
         WHEN 12090 THEN base_exp
         WHEN 12091 THEN CASE WHEN level<=110 THEN (base_exp*4+2)/3 ELSE base_exp*4 END
         WHEN 12092 THEN CASE WHEN level<=110
           THEN 285::BIGINT*(level-13)*(level-12)+20975::BIGINT*(level-12)+9802
           ELSE base_exp*9 END
       END,
       'formula','https://fenfentu.org/character-exp'
  FROM (SELECT level,171::BIGINT*level*level+8310::BIGINT*level-118463 AS base_exp
          FROM generate_series(12,150) AS level) l
 CROSS JOIN (VALUES (12090),(12091),(12092)) items(item_id);

INSERT INTO game_item_player_experience(item_id,player_level,experience,source_kind,source_url)
SELECT 25355,level,
       CASE WHEN level<=61 THEN 1604613::BIGINT+2804220::BIGINT*(level-53)/8
            WHEN level<85 THEN 4408833::BIGINT+7592417::BIGINT*(level-61)/24
            WHEN level<=110 THEN 1250::BIGINT*level*level+42000::BIGINT*level-600000
            ELSE (16755::BIGINT*level*level+815105::BIGINT*level-11653270)/14 END,
       CASE WHEN level IN (53,61,115,122,129) THEN 'sample'
            WHEN level BETWEEN 85 AND 110 THEN 'formula'
            WHEN level BETWEEN 111 AND 114 OR level>129 THEN 'extrapolated'
            ELSE 'interpolated' END,
       'https://assets.fenfentu.org/db/character-exps.json'
  FROM generate_series(53,150) AS level;

DO $$
BEGIN
    IF (SELECT count(*) FROM game_item_player_experience)<>515 THEN
        RAISE EXCEPTION '角色经验道具应为515条等级配置';
    END IF;
    IF EXISTS (
        SELECT 1 FROM (VALUES (53,1604613),(61,4408833),(115,21690620),
                              (122,24083640),(129,26593945)) samples(level,experience)
          LEFT JOIN game_item_player_experience p ON p.item_id=25355 AND p.player_level=samples.level
         WHERE p.experience IS DISTINCT FROM samples.experience
    ) THEN RAISE EXCEPTION '八仙灵丹配置未保留原始样本'; END IF;
END $$;

-- 正式客户端SuitFxDB.IsDragon/DragonSlot确认10件道具、neck/mask目标。
-- InventoryDlg原生使用分支传ApplyAvatar kind=2，独立于kind=0变装和kind=1部位灵。
ALTER TABLE equipment_instances ADD COLUMN fused_dragon_item_id INT NOT NULL DEFAULT 0 CHECK (fused_dragon_item_id >= 0);
CREATE TABLE game_item_dragon_fusions (
    item_id INT PRIMARY KEY REFERENCES game_items(id),
    target_slot SMALLINT NOT NULL CHECK (target_slot IN (1,3)),
    base_bonus_pct INT NOT NULL CHECK (base_bonus_pct BETWEEN 1 AND 100)
);
-- 百分比来自每件物品的ov_desc原始说明，仅加力量/敏捷/体质/智慧/精神。
INSERT INTO game_item_dragon_fusions(item_id,target_slot,base_bonus_pct) VALUES
    (10687,3,2),(10688,3,4),(10689,3,6),
    (11760,3,2),(11761,3,4),(11762,3,6),(11763,3,9),
    (14674,1,2),(14675,1,4),(14676,1,6);

-- 货架补齐：本服已确认的礼盒概率与未收录鞍具骑速规则。
ALTER TABLE char_pets ADD COLUMN pp_ai_used INT NOT NULL DEFAULT 0 CHECK (pp_ai_used >= 0);
CREATE TABLE game_item_pet_pp (
    item_id INT PRIMARY KEY REFERENCES game_items(id),
    kind TEXT NOT NULL CHECK (kind IN ('affection'))
);
INSERT INTO game_item_pet_pp(item_id,kind) VALUES (35680,'affection');

CREATE TABLE game_item_pet_rewards (
    item_id INT NOT NULL REFERENCES game_items(id),
    pet_id INT NOT NULL REFERENCES game_pet_profiles(pet_id),
    prefix_id INT NOT NULL CHECK (prefix_id >= 0),
    weight INT NOT NULL CHECK (weight BETWEEN 1 AND 10000),
    PRIMARY KEY (item_id,pet_id)
);
-- 物品说明确定两种种族和固定前缀；各50%为用户明确选择的本服概率。
INSERT INTO game_item_pet_rewards(item_id,pet_id,prefix_id,weight) VALUES
    (3287,6081,0,5000),(3287,6080,1,5000);

ALTER TABLE game_item_saddles ADD COLUMN use_pet_max_speed BOOLEAN NOT NULL DEFAULT FALSE;
INSERT INTO game_item_saddles(item_id,speed_bonus_bp,passengers,combination_model,distinct_pets,use_pet_max_speed) VALUES
    (13249,0,1,0,FALSE,TRUE),
    (13822,0,2,0,FALSE,TRUE),
    (13823,0,1,0,FALSE,TRUE),
    (13901,0,1,0,FALSE,TRUE),
    (13906,0,1,0,FALSE,TRUE),
    (13924,0,1,0,FALSE,TRUE),
    (13925,0,1,0,FALSE,TRUE),
    (13926,0,1,0,FALSE,TRUE),
    (14017,0,1,0,FALSE,TRUE),
    (14175,0,1,0,FALSE,TRUE),
    (14176,0,1,0,FALSE,TRUE),
    (14179,0,1,0,FALSE,TRUE),
    (14180,0,1,0,FALSE,TRUE),
    (14181,0,1,0,FALSE,TRUE),
    (14183,0,1,0,FALSE,TRUE),
    (14184,0,1,0,FALSE,TRUE),
    (14185,0,1,0,FALSE,TRUE),
    (35603,0,1,0,FALSE,TRUE);
-- 种族对应物品原始说明；骆驼包含原表三种，骑速分别使用各自上限。
INSERT INTO game_item_saddle_pets(item_id,pet_id) VALUES
    (13249,12356),
    (13249,12357),
    (13249,12396),
    (13822,12649),
    (13823,12656),
    (13823,12657),
    (13901,12740),
    (13901,12741),
    (13906,12744),
    (13906,12745),
    (13924,12746),
    (13925,12748),
    (13926,12749),
    (14017,12953),
    (14017,12954),
    (14175,11982),
    (14175,11983),
    (14176,32414),
    (14176,32447),
    (14179,32942),
    (14179,32943),
    (14180,10071),
    (14180,10072),
    (14181,11093),
    (14181,11094),
    (14183,11640),
    (14183,11641),
    (14184,12217),
    (14184,12218),
    (14185,10181),
    (14185,10203),
    (35603,12013),
    (35603,12014);

-- 用户选择：QQ宠物蛋规则未确认前下架；直接经验商品按下列彩玉价格上架。
UPDATE game_rack_goods SET active=FALSE WHERE item_id=3633;
INSERT INTO game_rack_goods(item_id,category_id,price,quantity,flags,remain,part,active) VALUES
    (12090,4,10,1,0,-1,'',TRUE),(12091,4,15,1,0,-1,'',TRUE),
    (12092,4,20,1,0,-1,'',TRUE),(25355,4,30,1,0,-1,'',TRUE)
ON CONFLICT(item_id) DO UPDATE SET category_id=EXCLUDED.category_id,price=EXCLUDED.price,
    quantity=EXCLUDED.quantity,flags=EXCLUDED.flags,remain=EXCLUDED.remain,part=EXCLUDED.part,active=TRUE;

-- 物品使用全量台账。它不重复保存效果数值，而是把已经闭合的
-- 业务配置和客户端声明的 self_use/other_use 投影成一个可查询台账。
-- 同一物品可同时有衣橱、融合等多个真实入口；未命中任何已知入口的
-- 声明行才会生成 enabled=false 和明确 disabled_reason。
CREATE OR REPLACE VIEW game_item_use_audit AS
WITH unique_avatar AS (
    SELECT index,min(category) AS slot,min(avatar_index) AS avatar_index
      FROM gamedata.ov_avatararm GROUP BY index HAVING count(*)=1
), wardrobe_items AS (
    SELECT u.index::INT AS item_id
      FROM unique_avatar u
      LEFT JOIN LATERAL (
          SELECT avatar FROM gamedata.ov_desc
           WHERE index=u.avatar_index ORDER BY row_no LIMIT 1
      ) d ON TRUE
      JOIN game_items gi ON gi.id=u.index
      LEFT JOIN game_equipment ge ON ge.id=u.index
     WHERE ge.id IS NULL AND (
       (u.slot=8 AND btrim(COALESCE(d.avatar,''), E' \t\r\n　') ~ '^(A|M|N|m|body)[0-9]+$') OR
       (u.slot=2 AND btrim(COALESCE(d.avatar,''), E' \t\r\n　') ~ '^(cap|cape)[0-9]+$') OR
       (u.slot=10 AND btrim(COALESCE(d.avatar,''), E' \t\r\n　') ~ '^backpack[0-9]+$') OR
       (u.slot=1 AND btrim(COALESCE(d.avatar,''), E' \t\r\n　') ~ '^face[0-9]+$') OR
       (u.slot IN (4,5,13) AND btrim(COALESCE(d.avatar,''), E' \t\r\n　') ~ '^weaponr[0-9]+$'))
), known_raw(item_id,route,handler,evidence) AS (
    SELECT DISTINCT i.index::INT,'use_item','status','ov_item_entry status id/level'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND e.op_type=1 AND e.prob=100 AND e.attr_id>=1000 AND e.value>0
       AND (e.mode=4 OR (e.mode=0 AND e.attr_id IN (1023,1024)))
       AND EXISTS(SELECT 1 FROM gamedata.ov_exceptdetail d
                   JOIN gamedata.ov_exceptdetail_entry x ON x.row_no=d.row_no
                  WHERE d.type=e.attr_id AND x.except_level=e.value)
    UNION ALL
    SELECT DISTINCT i.index::INT,'use_item','resource_restore','ov_item_entry attr 33/35'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND e.op_type=1 AND e.attr_id IN (33,35)
       AND e.mode IN (0,1) AND e.prob=100 AND e.value>0
    UNION ALL
    SELECT DISTINCT i.index::INT,'use_item','status_remove','ov_item_entry attr 87'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND e.op_type=1 AND e.attr_id=87 AND e.mode=0 AND e.prob=100
       AND e.value>=1000 AND EXISTS(SELECT 1 FROM gamedata.ov_exceptdesc d WHERE d.type=e.value)
    UNION ALL
    SELECT DISTINCT i.index::INT,'use_item','return','ov_item_entry attr 94'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND e.op_type=1 AND e.attr_id=94 AND e.mode=0 AND e.prob=100 AND e.value=0
    UNION ALL
    SELECT DISTINCT i.index::INT,'use_item','skill_book','ov_item skill_id'
      FROM gamedata.ov_item i WHERE i.self_use<>0 AND i.use_waste<>0 AND i.skill_id>0
       AND i.skill_id=i.skill_id_3 AND i.skill_id NOT BETWEEN 11000 AND 11999
       AND EXISTS(SELECT 1 FROM gamedata.ov_skilldesc s WHERE s.skill_id=i.skill_id)
    UNION ALL
    SELECT DISTINCT i.index::INT,'use_item','pet_restore','ov_item_entry pet attr 33/35'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND i.use_waste<>0 AND e.op_type=2 AND e.attr_id IN (33,35)
       AND e.mode=1 AND e.prob=100 AND e.value BETWEEN 1 AND 100
    UNION ALL
    SELECT DISTINCT i.index::INT,'use_item','status_remove','named player cleanse'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND i.use_waste<>0 AND e.op_type=2 AND e.mode=0 AND e.prob=100
       AND ((i.name='醒梦铃' AND e.attr_id=87 AND e.value=1002)
         OR (i.name='圣水' AND e.attr_id=88 AND e.value=0))
    UNION ALL
    SELECT DISTINCT i.index::INT,'use_item','pet_reset','ov_item_entry attr 455/458'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND i.use_waste<>0 AND e.op_type=1 AND e.attr_id IN (455,458)
       AND e.mode=1 AND e.prob=100 AND e.value=100
    UNION ALL
    SELECT index::INT,'use_item','stat_reset','ov_item script'
      FROM gamedata.ov_item WHERE self_use<>0 AND use_waste<>0
       AND script IN ('Item_attr01','Item_attr02','Item_attr03','Item_attr04','Item_attr05','Item_attr06','Item_rebirth')
    UNION ALL SELECT DISTINCT source_item_id,'use_item','fixed_reward','game_item_use_rewards' FROM game_item_use_rewards
    UNION ALL SELECT item_id,'use_item','currency_reward','game_item_currency_rewards' FROM game_item_currency_rewards
    UNION ALL SELECT item_id,'use_item','title_unlock','game_item_title_unlocks' FROM game_item_title_unlocks
    UNION ALL SELECT item_id,'use_item','experience_boost','game_item_experience_boosts' FROM game_item_experience_boosts
    UNION ALL SELECT DISTINCT item_id,'use_item','player_experience','game_item_player_experience' FROM game_item_player_experience
    UNION ALL SELECT item_id,'use_item','pet_transmog','game_item_pet_transmogs' FROM game_item_pet_transmogs
    UNION ALL SELECT DISTINCT item_id,'use_item','pet_experience','game_item_pet_experience' FROM game_item_pet_experience
    UNION ALL SELECT item_id,'use_item','pet_pp','game_item_pet_pp + ov_petppoint' FROM game_item_pet_pp
    UNION ALL SELECT DISTINCT item_id,'use_item','pet_reward','game_item_pet_rewards' FROM game_item_pet_rewards
    UNION ALL SELECT 10626,'furnace','craft_material','ov_combine material' WHERE EXISTS (
        SELECT 1 FROM gamedata.ov_combine c
        CROSS JOIN LATERAL (VALUES (1,c.mat_1),(2,c.mat_2),(3,c.mat_3),(4,c.mat_4),(5,c.mat_5)) m(n,value)
        WHERE m.n<=c.mat_count AND (m.value & 65535)=10626 AND c.show_type BETWEEN 0 AND 4)
    UNION ALL SELECT item_id,'use_item','saddle','pets.json saddles / game_item_saddles' FROM game_item_saddles
    UNION ALL SELECT item_id,'use_item','pet_learning','game_pet_learning_items' FROM game_pet_learning_items
    UNION ALL SELECT DISTINCT item_id,'use_item','pet_food','ov_petgrow eat slots' FROM (
        SELECT eats1_id::INT item_id FROM gamedata.ov_petgrow WHERE eats1_id>0
        UNION ALL SELECT eats2_id::INT FROM gamedata.ov_petgrow WHERE eats2_id>0
        UNION ALL SELECT eats3_id::INT FROM gamedata.ov_petgrow WHERE eats3_id>0
        UNION ALL SELECT eats4_id::INT FROM gamedata.ov_petgrow WHERE eats4_id>0) f
    UNION ALL SELECT index::INT,'stall','open','ov_item named stall coin' FROM gamedata.ov_item WHERE name IN ('黄金古币','白银古币')
    UNION ALL SELECT key_item_id,'wardrobe','unlock','game_wardrobe_rule' FROM game_wardrobe_rule WHERE id=1
    UNION ALL SELECT item_id,'avatar','fusion','game_item_avatar_fusions' FROM game_item_avatar_fusions
    UNION ALL SELECT item_id,'avatar','equipment_soul','game_item_equipment_souls' FROM game_item_equipment_souls
    UNION ALL SELECT item_id,'avatar','dragon','game_item_dragon_fusions' FROM game_item_dragon_fusions
    UNION ALL SELECT item_id,'wardrobe','store','ov_avatararm + appearance' FROM wardrobe_items
    UNION ALL SELECT 13580,'wardrobe','extract_voucher','wardrobe protocol constant'
    UNION ALL SELECT DISTINCT capture_tool::INT,'capture','standard_tool','ov_petgrow.capture_tool' FROM gamedata.ov_petgrow WHERE capture_tool>0
    UNION ALL SELECT i.index::INT,'capture','enhanced_tool','ov_item_entry attr 438/468'
      FROM gamedata.ov_item i JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
     WHERE i.self_use<>0 AND i.use_waste<>0 AND e.op_type=1 AND e.attr_id IN (438,468)
       AND e.mode=1 AND e.prob=100 GROUP BY i.index HAVING count(*) FILTER(WHERE e.attr_id=438)=1
    UNION ALL SELECT index::INT,'revive',CASE WHEN name='回魂娃娃' THEN 'city' ELSE 'in_place' END,'ov_item revive name'
      FROM gamedata.ov_item WHERE name IN ('替身娃娃','回魂娃娃') AND self_use<>0 AND use_waste<>0
    UNION ALL SELECT DISTINCT book_id::INT,'life_skill','learn','ov_skilldesc.book_id'
      FROM gamedata.ov_skilldesc WHERE skill_level=1 AND skill_id IN (11001,11002,11003,11004,11006,11007,11008,11009,11010) AND book_id>0
    UNION ALL SELECT unnest(ARRAY[3313,3314,3315]),'family','mail_stationery','family mail protocol constants'
    UNION ALL SELECT unnest(ARRAY[12581,25361]),'item_on','equipment_tool','UseItemOn protocol constants'
    UNION ALL SELECT DISTINCT (max(r.value) FILTER(WHERE r.req_type=2))::INT,
      'hair','dye','ov_haircolor requirements'
      FROM gamedata.ov_haircolor h JOIN gamedata.ov_haircolor_req_entry r ON r.row_no=h.row_no
     GROUP BY h.index HAVING max(r.value) FILTER(WHERE r.req_type=2)>0
    UNION ALL SELECT 26548,'quest','turn_in','task-only substitute doll'
), known AS (
    SELECT DISTINCT item_id,route,handler,evidence FROM known_raw WHERE item_id>0
), declared(item_id,source) AS (
    SELECT DISTINCT index::INT,'self' FROM gamedata.ov_item WHERE index>0 AND self_use<>0
    UNION SELECT DISTINCT index::INT,'other' FROM gamedata.ov_item WHERE index>0 AND other_use<>0
), unresolved AS (
    SELECT d.item_id,d.source,'unsupported'::TEXT AS route,''::TEXT AS handler,FALSE AS enabled,
           CASE
             WHEN d.source='other' AND i.index BETWEEN 14971 AND 14976
               THEN '虎娃之吻缺对他目标协议、变身模型与10分钟持久化'
             WHEN d.source='other' AND i.index=25544
               THEN '魔银离魂针依赖玩家互伤、无敌判定和城战目标协议'
             WHEN d.source='other' THEN '对他使用的目标协议和效果尚未闭合'
             WHEN i.index=3400 THEN '柏奚娃娃限荒雷岛使用，地图边界尚未闭合'
             WHEN i.is_time_item<>0 THEN '限时时装/饰品的槽位、过期和持久化尚未闭合'
             WHEN i.index BETWEEN 3209 AND 3214
               THEN '祝福卡只有效果名称，缺少对应状态定义、数值与时长'
             WHEN i.index IN (3254,3255)
               THEN '吉祥岛限定增益缺地图范围和不可驱散状态定义'
             WHEN i.index BETWEEN 10759 AND 10770
               THEN '宠物涅磐觉醒等级、坐骑限定与金银甲外形持久化尚未实现'
             WHEN i.index BETWEEN 25515 AND 25545
               THEN '城战地雷、建筑、守护灵或阵营目标系统尚未实现'
             WHEN i.index=34443 THEN '冲锋战场得分区域与积分系统尚未实现'
             WHEN i.index=35710 THEN '碎星石依赖太极装备解绑目标协议，原表概率为0'
             WHEN i.index=25152 THEN '神奇拼图缺少图案编辑入口、图案数据与场景展示协议'
             WHEN i.script ~ '^Item_miji' OR i.script ~ '^Item_aoyi'
               THEN '秘技/奥义脚本缺少可持久化的独立技能系统'
             WHEN i.script ~ '^Item_wbox'
               THEN '时装礼盒未提供完整奖池和各产出概率'
             WHEN i.script ~ '^Item_yh' OR i.script IN ('bless','squib')
               THEN '焰火/节日特效缺场景广播与客户端特效参数'
             WHEN i.script IN ('paper','pack','superpack')
               THEN '普通信纸/包袱需要邮件撰写入口与附件额度规则'
             WHEN i.script='forget'
               THEN '遗忘水晶缺少客户端选中技能的上行字段，不能凭服务端猜测'
             WHEN i.script<>'' AND (COALESCE(od.desc_,'') ~ '(随机|几率|概率|惊喜)')
               THEN '随机脚本物品未提供完整奖池和各产出概率'
             WHEN i.script ~ '^Item[0-9]+$'
               THEN '原客户端只保留独立处理器名，未提供对应脚本主体与参数'
             WHEN i.script<>'' THEN '原客户端脚本主体未随静态数据提供'
             WHEN EXISTS(SELECT 1 FROM gamedata.ov_item_entry e WHERE e.row_no=i.row_no)
               THEN '静态效果操作码尚未闭合'
             WHEN i.use_waste=0 AND COALESCE(od.desc_,'') ~ '(兑换|材料|上交|制作)'
               THEN '该物品是NPC兑换或制作材料，不应通过背包直接消耗；对应玩法配置未开放'
             WHEN i.index=10024 THEN '天神的契约需要NPC选择面板、召唤上限和10分钟生命周期'
             WHEN i.index=13680 THEN '决斗之王喇叭需要称号校验、文本输入和普通大喇叭代扣'
             WHEN i.index IN (3107,3108) THEN '秘技之卷/奥义之书没有效果脚本、技能编号或客户端选择入口'
             WHEN i.index=3385 THEN '魂之精是捕捉产物标记，没有可执行效果或独立使用入口'
             WHEN i.index=3387 THEN '家族钥匙是家族屋使用权标记，家族屋权限流程尚未开放'
             WHEN i.index=36130 THEN '魔龙之眼只有占位描述，没有效果操作、目标或数值定义'
             WHEN i.use_waste=0 THEN '非消耗型入口没有对应的服务端玩法配置'
             ELSE '原始物品数据没有可执行效果'
           END AS disabled_reason,
           'ov_item.'||d.source||'_use' AS evidence
      FROM declared d JOIN gamedata.ov_item i ON i.index=d.item_id
      LEFT JOIN LATERAL (
          SELECT desc_ FROM gamedata.ov_desc WHERE index=i.index ORDER BY row_no LIMIT 1
      ) od ON TRUE
     WHERE (d.source='self' AND NOT EXISTS(SELECT 1 FROM known k WHERE k.item_id=d.item_id))
        OR (d.source='other' AND NOT EXISTS(SELECT 1 FROM known k WHERE k.item_id=d.item_id AND k.route IN ('capture','target_item')))
)
SELECT k.item_id,'resolved'::TEXT AS source,k.route,k.handler,TRUE AS enabled,
       ''::TEXT AS disabled_reason,k.evidence FROM known k
UNION ALL
SELECT item_id,source,route,handler,enabled,disabled_reason,evidence FROM unresolved;

DO $$
DECLARE declared_count INT; accounted_count INT; bad INT;
BEGIN
    SELECT count(*) INTO declared_count FROM (
        SELECT DISTINCT index,'self' FROM gamedata.ov_item WHERE index>0 AND self_use<>0
        UNION SELECT DISTINCT index,'other' FROM gamedata.ov_item WHERE index>0 AND other_use<>0) d;
    SELECT count(*) INTO accounted_count FROM (
        SELECT DISTINCT i.index,flag.source
          FROM gamedata.ov_item i
          CROSS JOIN LATERAL (VALUES
              ('self',i.self_use),('other',i.other_use)) flag(source,value)
         WHERE i.index>0 AND flag.value<>0
           AND (EXISTS(SELECT 1 FROM game_item_use_audit a WHERE a.item_id=i.index AND a.enabled)
             OR EXISTS(SELECT 1 FROM game_item_use_audit a WHERE a.item_id=i.index AND a.source=flag.source AND NOT a.enabled))) a;
    IF accounted_count<>declared_count THEN
        RAISE EXCEPTION '物品使用台账未全量闭合: declared=% accounted=%',declared_count,accounted_count;
    END IF;
    SELECT count(*) INTO bad FROM game_item_use_audit
     WHERE (enabled AND handler='') OR (NOT enabled AND disabled_reason='');
    IF bad<>0 THEN RAISE EXCEPTION '物品使用台账有%条缺处理器或禁用原因',bad; END IF;
END $$;

-- 各处理器的基线与启动时真实加载数一致。这组断言能抓住
-- “全部有台账，但某个入口取错字段”的假阳性，例如染发剂必须取
-- req_type=2 的物品需求，不能把 req_type=1 的金钱费用当成物品号。
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN WITH expected(route,handler,want) AS (VALUES
      ('avatar','dragon',10),('avatar','equipment_soul',134),('avatar','fusion',5710),
      ('capture','enhanced_tool',2),('capture','standard_tool',5),
      ('family','mail_stationery',3),('hair','dye',8),
      ('item_on','equipment_tool',2),('life_skill','learn',9),
      ('quest','turn_in',1),('revive','city',1),('revive','in_place',2),
      ('stall','open',2),('use_item','currency_reward',11),
      ('use_item','experience_boost',5),('use_item','player_experience',4),('use_item','fixed_reward',19),
      ('use_item','pet_experience',1),('use_item','pet_pp',1),('use_item','pet_reward',1),('furnace','craft_material',1),('use_item','pet_food',10),('use_item','pet_learning',16),
      ('use_item','pet_reset',2),('use_item','pet_restore',3),
      ('use_item','saddle',130),('use_item','pet_transmog',47),('use_item','resource_restore',23),
      ('use_item','return',2),('use_item','skill_book',301),
      ('use_item','stat_reset',7),('use_item','status',107),
      ('use_item','status_remove',4),('use_item','title_unlock',45),
      ('wardrobe','extract_voucher',1),('wardrobe','store',5615),
      ('wardrobe','unlock',1)
    ), actual AS MATERIALIZED (
        SELECT route,handler,count(DISTINCT item_id)::INT AS got
          FROM game_item_use_audit WHERE enabled GROUP BY route,handler
    )
    SELECT e.route,e.handler,e.want,COALESCE(a.got,0) AS got
      FROM expected e LEFT JOIN actual a USING(route,handler)
    LOOP
        IF r.got<>r.want THEN
            RAISE EXCEPTION '物品使用台账 %.% 应为%件，实得%',r.route,r.handler,r.want,r.got;
        END IF;
    END LOOP;
END $$;

-- 同乘规则采用用户指定的腾讯官方双人坐骑小贴士：
-- https://ffo.qq.com/act/a20100122fish/ 。不限制同队；最后一条邀请有效，60秒后失效。
CREATE TABLE game_shared_ride_rule (
    id INT PRIMARY KEY CHECK (id=1),
    invite_seconds INT NOT NULL CHECK (invite_seconds>0),
    source_url TEXT NOT NULL DEFAULT ''
);
INSERT INTO game_shared_ride_rule(id,invite_seconds,source_url)
VALUES (1,60,'https://ffo.qq.com/act/a20100122fish/');

-- 狮心剑：客户端五级描述均明确给出伤害倍率与 100% 命中。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, power_pct, guaranteed_hit)
VALUES
    (10116, 1, 1, 'damage', 175, TRUE),
    (10116, 2, 1, 'damage', 185, TRUE),
    (10116, 3, 1, 'damage', 195, TRUE),
    (10116, 4, 1, 'damage', 205, TRUE),
    (10116, 5, 1, 'damage', 215, TRUE),

-- 五职业技能中所有明确写出“100%命中”的直接伤害段。带即死或隐身前置的
-- 技能仍由后续模块补齐附加语义；本批只闭合它们共有的伤害与必中结果。
    (10004, 1, 1, 'damage', 200, TRUE),
    (10004, 2, 1, 'damage', 210, TRUE),
    (10004, 3, 1, 'damage', 220, TRUE),
    (10004, 4, 1, 'damage', 230, TRUE),
    (10004, 5, 1, 'damage', 240, TRUE),
    (10015, 1, 1, 'damage', 165, TRUE),
    (10015, 2, 1, 'damage', 169, TRUE),
    (10015, 3, 1, 'damage', 173, TRUE),
    (10015, 4, 1, 'damage', 177, TRUE),
    (10015, 5, 1, 'damage', 181, TRUE),
    (10023, 1, 1, 'damage', 165, TRUE),
    (10023, 2, 1, 'damage', 170, TRUE),
    (10023, 3, 1, 'damage', 175, TRUE),
    (10023, 4, 1, 'damage', 180, TRUE),
    (10023, 5, 1, 'damage', 185, TRUE),
    (10209, 1, 1, 'damage', 300, TRUE),
    (10209, 2, 1, 'damage', 320, TRUE),
    (10209, 3, 1, 'damage', 340, TRUE),
    (10209, 4, 1, 'damage', 360, TRUE),
    (10209, 5, 1, 'damage', 380, TRUE),
    (10210, 1, 1, 'damage', 400, TRUE),
    (10210, 2, 1, 'damage', 410, TRUE),
    (10210, 3, 1, 'damage', 420, TRUE),
    (10210, 4, 1, 'damage', 430, TRUE),
    (10210, 5, 1, 'damage', 440, TRUE);

-- 复法术：固定恢复目标法力；五级恢复量与施法消耗都来自 ov_skilldesc。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, flat_value)
VALUES
    (10302, 1, 1, 'restore_mp', 100),
    (10302, 2, 1, 'restore_mp', 200),
    (10302, 3, 1, 'restore_mp', 300),
    (10302, 4, 1, 'restore_mp', 400),
    (10302, 5, 1, 'restore_mp', 500);

-- 破法之枪：先造成普通伤害；仅命中后再按施法者面板攻击力削减目标法力。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, power_pct,
     effect_target, requires_previous_landed)
VALUES
    (10013, 1, 1, 'damage',    100, 'target', FALSE),
    (10013, 1, 2, 'damage_mp',  60, 'target', TRUE),
    (10013, 2, 1, 'damage',    100, 'target', FALSE),
    (10013, 2, 2, 'damage_mp',  70, 'target', TRUE),
    (10013, 3, 1, 'damage',    100, 'target', FALSE),
    (10013, 3, 2, 'damage_mp',  80, 'target', TRUE),
    (10013, 4, 1, 'damage',    100, 'target', FALSE),
    (10013, 4, 2, 'damage_mp',  90, 'target', TRUE),
    (10013, 5, 1, 'damage',    100, 'target', FALSE),
    (10013, 5, 2, 'damage_mp', 100, 'target', TRUE);

-- 饮血剑/噬魂剑：伤害命中后，按施法者面板攻击力恢复施法者生命/法力。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, power_pct,
     effect_target, requires_previous_landed)
VALUES
    (10104, 1, 1, 'damage',     150, 'target', FALSE),
    (10104, 1, 2, 'restore_hp',  20, 'caster', TRUE),
    (10104, 2, 1, 'damage',     155, 'target', FALSE),
    (10104, 2, 2, 'restore_hp',  22, 'caster', TRUE),
    (10104, 3, 1, 'damage',     160, 'target', FALSE),
    (10104, 3, 2, 'restore_hp',  24, 'caster', TRUE),
    (10104, 4, 1, 'damage',     165, 'target', FALSE),
    (10104, 4, 2, 'restore_hp',  26, 'caster', TRUE),
    (10104, 5, 1, 'damage',     170, 'target', FALSE),
    (10104, 5, 2, 'restore_hp',  28, 'caster', TRUE),
    (10119, 1, 1, 'damage',     150, 'target', FALSE),
    (10119, 1, 2, 'restore_mp',  20, 'caster', TRUE),
    (10119, 2, 1, 'damage',     155, 'target', FALSE),
    (10119, 2, 2, 'restore_mp',  22, 'caster', TRUE),
    (10119, 3, 1, 'damage',     160, 'target', FALSE),
    (10119, 3, 2, 'restore_mp',  24, 'caster', TRUE),
    (10119, 4, 1, 'damage',     165, 'target', FALSE),
    (10119, 4, 2, 'restore_mp',  26, 'caster', TRUE),
    (10119, 5, 1, 'damage',     170, 'target', FALSE),
    (10119, 5, 2, 'restore_mp',  28, 'caster', TRUE);

-- 裂空之枪：伤害倍率与武器耐久损耗均由五级技能说明给出。耐久属于施法
-- 资源，按武器满耐久计算，即使伤害 MISS 也会在准入成功时扣除。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, power_pct,
     weapon_durability_pct)
VALUES
    (10003, 1, 1, 'damage', 180, 5),
    (10003, 2, 1, 'damage', 190, 6),
    (10003, 3, 1, 'damage', 200, 7),
    (10003, 4, 1, 'damage', 210, 8),
    (10003, 5, 1, 'damage', 220, 9);

-- B1：可以由现有状态运行时完整表达的五职业主动技能。base/step 分别描述
-- 1 级技能对应的状态等级/持续时间以及每升一级的增量；duration=0 沿用状态表。
INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec)
SELECT m.skill_id, lv, 1, m.status_id,
       m.status_level_base + (lv - 1) * m.status_level_step,
       m.duration_base + (lv - 1) * m.duration_step
  FROM (VALUES
        (10016, 1005,  1, 0,  5, 1), -- 石化之枪：石化 5..9 秒
        (10030, 1006, 10, 0,  5, 1), -- 缴械：攻击力 -100%，5..9 秒
        (10107, 1012,  1, 1,  0, 0), -- 铁城神盾：物理减伤 10/13/16/19/22%
        (10109, 1019, 16, 1, 10, 0), -- 仙风云体：定身并提高防御 80..160%
        (10114, 1022,  1, 1, 10, 1), -- 锁灵剑：阻挡有害状态 10..14 秒
        (10124, 1019, 11, 1, 30, 0), -- 山字诀：降攻并提高防御 10..22%
        (10310, 1009,  3, 1, 20, 0), -- 减速术：移动速度 -30..70%
        (10311, 1016,  4, 1, 30, 0), -- 加速术：移动速度 +20..40%
        (10324, 1006,  2, 1, 30, 0), -- 衰弱术：攻击力 -20..60%
        (10325, 1009, 11, 1, 30, 0), -- 枷锁术：攻击速度 -30..50%
        (10330, 1010,  1, 1, 20, 0), -- 针刺护体：物理反伤 10..50%
        (10413, 1034, 18, 0, 10, 1), -- 时之封印：定身 10..14 秒
        (10421, 1036,  1, 0, 20, 5), -- 燃烧：每 2 秒 -20 HP，总量 200..400
        (10428, 1004,  5, 0, 20, 5)  -- 冰封：冰冻 20..40 秒，受伤解除
  ) AS m(skill_id, status_id, status_level_base, status_level_step,
         duration_base, duration_step)
 CROSS JOIN generate_series(1, 5) AS lv;

-- B2：技能参数完整、但官方状态表没有等价等级的持续状态。
-- status_type: 0=增益、1=减益；attr 47=攻击、13=防御，mode 1=百分比。
INSERT INTO game_custom_statuses
    (status_id, status_level, name, description, status_type,
     duration_sec, interval_sec, hp_change_value,
     physical_damage_reduction_pct, magic_damage_reduction_pct,
     damage_shield, attack_cap)
SELECT 10115, lv, '火字诀',
       format('攻击力上升%s%%；防御力下降%s%%。', pct, pct),
       0, 30, 0, 0, 0, 0, 0, NULL
  FROM (VALUES (1,10),(2,13),(3,16),(4,19),(5,22)) AS v(lv,pct)
UNION ALL
SELECT 10129, lv, '致残', format('攻击力下降至%s。', cap),
       1, 4 + lv, 0, 0, 0, 0, 0, cap
  FROM (VALUES (1,0),(2,1),(3,2),(4,3),(5,4)) AS v(lv,cap)
UNION ALL
SELECT 10303, lv, '回春术', format('每秒恢复生命值%s点。', heal),
       0, duration, 1, heal, 0, 0, 0, NULL
  FROM (VALUES
        (1,120,14),(2,135,18),(3,150,22),(4,165,26),(5,180,30)
  ) AS v(lv,duration,heal)
UNION ALL
SELECT 10321, lv, '防御术', format('受到的伤害降低%s%%。', pct),
       0, 300, 0, 0, pct, pct, 0, NULL
  FROM (VALUES (1,10),(2,13),(3,16),(4,19),(5,22)) AS v(lv,pct);

INSERT INTO game_custom_status_affixes
    (status_id, status_level, affix_order, attr_id, value, mode)
SELECT 10115, lv, 1, 47, pct, 1
  FROM (VALUES (1,10),(2,13),(3,16),(4,19),(5,22)) AS v(lv,pct)
UNION ALL
SELECT 10115, lv, 2, 13, -pct, 1
  FROM (VALUES (1,10),(2,13),(3,16),(4,19),(5,22)) AS v(lv,pct);

INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec)
SELECT skill_id, lv, 1, skill_id, lv, 0
  FROM (VALUES (10115),(10129),(10303),(10321)) AS s(skill_id)
 CROSS JOIN generate_series(1, 5) AS lv;

-- 神圣庇护对应伤害吸收状态 1021 的第二组等级：数值 500..1300，官方状态
-- 时长 60..100 秒；技能说明明确该护盾同时免疫即死。
INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec,
     description_override, instant_death_immune)
SELECT 10308, lv, 1, 1021, lv + 10, 0,
       format('吸收伤害%s点；免疫即死效果。', 300 + lv * 200), TRUE
  FROM generate_series(1, 5) AS lv;

-- B3：选择性净化/驱散。原始客户端只保留“每次升级效果增强”，未保留旧服
-- 成功率脚本；成功率显式落库为 20/40/60/80/100%，以后有新实证可只调配置。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, chance_bp)
SELECT skill_id, lv, 1, 'remove_statuses', lv * 2000
  FROM (VALUES (10306),(10316),(10318),(10433)) AS s(skill_id)
 CROSS JOIN generate_series(1, 5) AS lv;

INSERT INTO game_skill_effect_remove_statuses
    (skill_id, skill_level, effect_order, status_id)
SELECT s.skill_id, lv, 1, status_id
  FROM (VALUES
        (10306, ARRAY[1001,1002]::INT[]),           -- 中毒、昏迷
        (10316, ARRAY[1006,1007,1009]::INT[]),      -- 攻防下降、减速
        (10318, ARRAY[1003,1004,1005,1034]::INT[]), -- 封印、冰冻、石化、定身
        (10433, ARRAY[1010,1011,1021,1037,1038]::INT[]) -- 针刺、魔镜、三种盾
  ) AS s(skill_id,status_ids)
 CROSS JOIN generate_series(1, 5) AS lv
 CROSS JOIN LATERAL unnest(s.status_ids) AS status_id;

-- 光之奇迹对存活目标依次净化、治疗；死亡目标只执行末段复活，恢复同样
-- 比例的生命，并取回本次死亡损失的 80/85/90/95/100%。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, max_resource_pct,
     loss_refund_pct)
SELECT 10309, lv, 1, 'clear_harmful', 0, 0
  FROM generate_series(1, 5) AS lv
UNION ALL
SELECT 10309, lv, 2, 'heal', 50 + lv * 5, 0
  FROM generate_series(1, 5) AS lv
UNION ALL
SELECT 10309, lv, 3, 'resurrect', 50 + lv * 5, 75 + lv * 5
  FROM generate_series(1, 5) AS lv;

-- C1：魔镜术与官方“魔法反弹”状态 1011 的前五级逐项闭合：持续 30 秒，
-- 触发率 30/35/40/45/50%。触发时抵消本次魔法伤害并把整次伤害折回施法者。
INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec)
SELECT 10417, lv, 1, 1011, lv, 0
  FROM generate_series(1, 5) AS lv;

-- C2：已闭合的被动数值修正。伤害修炼按技能列表显式关联，避免靠职业号把
-- 控制/辅助技能误增伤；同一技能命中多条修炼时加成相加后统一乘基础倍率。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id, passive_skill_level, modifier_order, modifier_kind,
     target_skill_id, effect_kind, value)
SELECT passive_id, lv, target_order, 'skill_damage_pct', target_skill, '', step * lv
  FROM (VALUES
        (10001,  2, ARRAY[10002,10003,10004,10005,10006,10007,10009,10013,10015,10017,10021,10022,10023,10027]::INT[]),
        (10101,  2, ARRAY[10102,10103,10104,10111,10113,10116,10117,10119,10121,10122,10123,10125,10126]::INT[]),
        (10118,  5, ARRAY[10117]::INT[]),
        (10424,  4, ARRAY[10401,10402,10405,10408]::INT[]),
        (10426,  4, ARRAY[10401,10402]::INT[]),
        (10431,  4, ARRAY[10409,10410,10411,10412]::INT[])
  ) AS p(passive_id,step,target_skills)
 CROSS JOIN generate_series(1,5) AS lv
 CROSS JOIN LATERAL unnest(p.target_skills) WITH ORDINALITY AS t(target_skill,target_order);

-- 破魔、血影、魔光直接增加技能说明中的“攻击力转资源”百分点。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id, passive_skill_level, modifier_order, modifier_kind,
     target_skill_id, effect_kind, value)
SELECT passive_id, lv, 1, 'effect_source_attack_pct', target_skill, effect_kind, step * lv
  FROM (VALUES
        (10014,10013,'damage_mp',10),
        (10112,10104,'restore_hp', 2),
        (10120,10119,'restore_mp', 2)
  ) AS p(passive_id,target_skill,effect_kind,step)
 CROSS JOIN generate_series(1,5) AS lv;

-- 龙杀剑：一次正常物理攻击外，每消耗 1 点法力追加 1 点物理伤害。
-- flat_value 与 ov_skilldesc.sp_chg_start 五级逐项一致，保持为可审计的显式配置。
INSERT INTO game_skill_effects
    (skill_id,skill_level,effect_order,effect_kind,power_pct,flat_value)
VALUES
    (10105,1,1,'damage',100,247),
    (10105,2,1,'damage',100,281),
    (10105,3,1,'damage',100,315),
    (10105,4,1,'damage',100,349),
    (10105,5,1,'damage',100,383);

-- 耐久与陷阱被动：维护每级减少裂空之枪 25% 额外耐久代价，4 级封顶；
-- 暗器回收只对被动自身 arm_type=32 的暗器生效；陷阱回避只在地面陷阱触发时判定。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id,passive_skill_level,modifier_order,modifier_kind,
     target_skill_id,value)
SELECT 10024,lv,1,'skill_durability_reduction_pct',10003,LEAST(lv * 25,100)
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10222,lv,1,'attack_durability_reduction_pct',0,lv * 10
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10229,lv,1,'trap_half_damage_chance',0,12 + lv * 8
  FROM generate_series(1,5) AS lv;

-- 冰系掌握：冰箭/冰爆/暴雪/绝对零度命中后独立以 30% 概率施加 10 秒减速，
-- 幅度按被动等级为 20/30/40/50/60%。
INSERT INTO game_custom_statuses
    (status_id,status_level,name,description,status_type,duration_sec)
SELECT 10427,lv,'冰系掌握',format('移动速度降低%s%%，持续10秒。',10 + lv * 10),1,10
  FROM generate_series(1,5) AS lv;

INSERT INTO game_custom_status_affixes
    (status_id,status_level,affix_order,attr_id,value,mode)
SELECT 10427,lv,1,27,-(10 + lv * 10),1
  FROM generate_series(1,5) AS lv;

INSERT INTO game_skill_passive_trigger_statuses
    (passive_skill_id,passive_skill_level,trigger_order,target_skill_id,
     status_id,status_level,chance_bp,duration_sec)
SELECT 10427,lv,target_order,target_skill,10427,lv,3000,0
  FROM generate_series(1,5) AS lv
 CROSS JOIN LATERAL unnest(ARRAY[10409,10410,10412,10411]::INT[])
     WITH ORDINALITY AS t(target_skill,target_order);

-- 冰封强化：只修饰冰封(10428)实际施加的冻结状态，不改变其持续时间与图标。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id,passive_skill_level,modifier_order,modifier_kind,
     target_skill_id,value)
SELECT 10429,lv,1,'status_physical_damage_taken_pct',10428,15 + lv * 5
  FROM generate_series(1,5) AS lv;

-- 药师术法精研与术士魔法修炼的吟唱缩短。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id, passive_skill_level, modifier_order, modifier_kind, value)
SELECT passive_id, lv, 1, 'cast_time_reduction_pct', 15 + lv * 5
  FROM (VALUES (10314),(10415)) AS p(passive_id)
 CROSS JOIN generate_series(1,5) AS lv;

-- 抗打断为相对减幅；基础概率和最低概率由 game_combat_timing_rule 配置。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id, passive_skill_level, modifier_order, modifier_kind, value)
SELECT 10314, lv, 2, 'cast_interrupt_reduction_pct', 15 + lv * 5
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10416, lv, 1, 'cast_interrupt_reduction_pct', lv * 20
  FROM generate_series(1,5) AS lv;

-- 魔法回避：20/28/36/44/52% 几率让本次魔法伤害减半。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id, passive_skill_level, modifier_order, modifier_kind, value)
SELECT 10230, lv, 1, 'incoming_magic_half_chance', 12 + lv * 8
  FROM generate_series(1,5) AS lv;

-- 进阶隐身把技能说明中的三项增量分别落成参数：时限 +2 秒/级、移速与
-- 隐身攻击伤害各 +5%/级。进阶背刺和进阶暗杀只改变对应技能的破隐概率。
INSERT INTO game_skill_passive_modifiers
    (passive_skill_id, passive_skill_level, modifier_order, modifier_kind,
     target_skill_id, value)
SELECT 10207, lv, 1, 'stealth_duration_sec', 10208, lv * 2
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10207, lv, 2, 'stealth_move_speed_pct', 10208, lv * 5
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10207, lv, 3, 'stealth_damage_pct', 10208, lv * 5
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10201, lv, 1, 'preserve_invisible_chance', 10209, 20
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10220, lv, 1, 'preserve_invisible_chance', 10210, 12 + lv * 8
  FROM generate_series(1,5) AS lv;

-- 数值明确且带装备条件的基础属性被动：盾防修炼只在盾(type=105)生效；
-- 双刃/暗器修炼分别只在 type=8/type=32 的武器生效。
INSERT INTO game_skill_passive_stat_modifiers
    (passive_skill_id, passive_skill_level, modifier_order,
     attr_id, value, mode, required_equip_type)
SELECT passive_id, lv, 1, attr_id, step * lv, 1, equip_type
  FROM (VALUES
        (10106,13,2,105),
        (10202,47,3,8),
        (10204,47,3,32)
  ) AS p(passive_id,attr_id,step,equip_type)
 CROSS JOIN generate_series(1,5) AS lv;

-- 意志/心镜抵抗昏迷，强健抵抗减速，抗魔抵抗冰冻、石化、定身与衰弱。
-- 技能说明逐级均为 10/20/30/40/50%。
INSERT INTO game_skill_passive_status_resists
    (passive_skill_id, passive_skill_level, resist_order, status_id, resist_pct)
SELECT passive_id, lv, status_order, status_id, lv * 10
  FROM (VALUES
        (10008, ARRAY[1002]::INT[]),
        (10025, ARRAY[1009]::INT[]),
        (10026, ARRAY[1004,1005,1034,1006]::INT[]),
        (10108, ARRAY[1002]::INT[])
  ) AS p(passive_id,status_ids)
 CROSS JOIN generate_series(1,5) AS lv
 CROSS JOIN LATERAL unnest(p.status_ids) WITH ORDINALITY AS s(status_id,status_order);

-- 全力投掷的专属减速：固定 5 秒、30% 触发，减速 50/60/70/80/90%。
INSERT INTO game_custom_statuses
    (status_id,status_level,name,description,status_type,duration_sec)
SELECT 10019,lv,'全力投掷',format('移动速度降低%s%%，持续5秒。',40 + lv * 10),1,5
  FROM generate_series(1,5) AS lv;

INSERT INTO game_custom_status_affixes
    (status_id,status_level,affix_order,attr_id,value,mode)
SELECT 10019,lv,1,27,-(40 + lv * 10),1
  FROM generate_series(1,5) AS lv;

-- 数值与状态等级均已闭合的关联触发：全力投掷减速、通背眩晕、
-- 高热风暴燃烧、淬冰风暴冻结。
INSERT INTO game_skill_passive_trigger_statuses
    (passive_skill_id,passive_skill_level,trigger_order,target_skill_id,
     status_id,status_level,chance_bp,duration_sec)
SELECT 10019,lv,1,10017,10019,lv,3000,0
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10110,lv,1,10102,1002,lv + 10,2000,0
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10423,lv,1,10405,1036,(lv + 1) / 2,
       CASE WHEN lv % 2 = 0 THEN 3500 ELSE 3000 END,0
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10430,lv,1,10412,1004,lv,2000,0
  FROM generate_series(1,5) AS lv;

-- 破甲：破防之枪命中后 15 秒内承受物理/魔法伤害增加
-- 20/23/26/29/32%。
INSERT INTO game_custom_statuses
    (status_id,status_level,name,description,status_type,duration_sec,
     physical_damage_taken_pct,magic_damage_taken_pct)
SELECT 10012,lv,'破甲',format('承受的物理与魔法伤害增加%s%%。',17 + lv * 3),
       1,15,17 + lv * 3,17 + lv * 3
  FROM generate_series(1,5) AS lv;

INSERT INTO game_skill_passive_trigger_statuses
    (passive_skill_id,passive_skill_level,trigger_order,target_skill_id,
     status_id,status_level,chance_bp,duration_sec)
SELECT 10012,lv,1,10009,10012,lv,10000,0
  FROM generate_series(1,5) AS lv;

-- 燃烧强化：在燃烧术原有 1036 状态上追加魔法防御降低
-- 30/35/40/45/50%，持续时间完全继承燃烧本身。
INSERT INTO game_skill_passive_status_affixes
    (passive_skill_id,passive_skill_level,modifier_order,target_skill_id,
     status_id,attr_id,value,mode)
SELECT 10422,lv,1,10421,1036,17,-(25 + lv * 5),1
  FROM generate_series(1,5) AS lv;

-- 震空枪气与爆发冲击：触发率来自逐级技能说明；击退距离复用已闭合的
-- 10223 震退技能 100 像素尺度。
INSERT INTO game_skill_passive_trigger_effects
    (passive_skill_id,passive_skill_level,trigger_order,target_skill_id,
     effect_kind,flat_value,chance_bp)
SELECT 10020,lv,1,10006,'knockback',100,(15 + lv * 5) * 100
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10226,lv,1,10225,'knockback',100,(5 + lv * 5) * 100
  FROM generate_series(1,5) AS lv;

-- 反击术：成功闪避后 40/43/46/49/52% 几率以普通物理伤害反击；
-- 被动自身的 arm_type=8，只有实际装备双刃时生效。
INSERT INTO game_skill_passive_counterattacks
    (passive_skill_id,passive_skill_level,chance_bp,damage_pct,required_equip_type)
SELECT 10203,lv,(37 + lv * 3) * 100,100,8
  FROM generate_series(1,5) AS lv;

-- D1：冲锋的五级持续时间与 50% 移速均由技能说明直接给出。状态使用技能号
-- 作为稳定身份，避免借用官方“加速”状态中不相干的 60 秒等级。
INSERT INTO game_custom_statuses
    (status_id, status_level, name, description, status_type, duration_sec)
SELECT 10018, lv, '冲锋', format('移动速度提升50%%，持续%s秒。', 3 + lv * 2),
       0, 3 + lv * 2
  FROM generate_series(1,5) AS lv;

INSERT INTO game_custom_status_affixes
    (status_id, status_level, affix_order, attr_id, value, mode)
SELECT 10018, lv, 1, 27, 50, 1
  FROM generate_series(1,5) AS lv;

INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec)
SELECT 10018, lv, 1, 10018, lv, 0
  FROM generate_series(1,5) AS lv;

-- 战吼/挑衅都以自身为中心拉取范围内敌人。客户端只留下“升级后效果更佳”，
-- 没有旧服绝对仇恨值；flat_value 显式保存可比较的等级优先级。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, flat_value)
SELECT skill_id, lv, 1, 'taunt', lv
  FROM (VALUES (10028),(10127)) AS s(skill_id)
 CROSS JOIN generate_series(1,5) AS lv;

-- D2：阴影跳跃使用客户端目标实体的权威坐标；秘术幻影使用 0x1013
-- UseSkillAt 的明确落点。后者的五级成功率由技能说明逐级给出。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, chance_bp, effect_target)
SELECT 10227, lv, 1, 'teleport_target', 10000, 'target'
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10432, lv, 1, 'teleport_point', 5000 + lv * 1000, 'caster'
  FROM generate_series(1,5) AS lv;

-- D3：五种陷阱共用客户端原生 SpawnTrap 实体协议。place_trap 的 flat_value
-- 是说明中明确给出的触发/效果半径 100；后续有序效果只在陷阱被踩中时结算。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, power_pct,
     max_resource_pct, flat_value, effect_target, requires_previous_landed)
SELECT skill_id, lv, 1, 'place_trap', 0, 0, 100, 'caster', FALSE
  FROM (VALUES (10213),(10214),(10215),(10216),(10217)) AS s(skill_id)
 CROSS JOIN generate_series(1,5) AS lv
UNION ALL
SELECT 10213, lv, 2, 'damage',
       (ARRAY[200,210,220,230,240])[lv], 0, 0, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10214, lv, 2, 'damage', 130, 0, 0, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10215, lv, 2, 'damage', 150, 0, 0, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10216, lv, 2, 'damage', 170, 0, 0, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10217, lv, 2, 'damage',
       (ARRAY[185,190,195,200,210])[lv], 0, 0, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10217, lv, 3, 'damage_mp_max', 0, 20 + lv * 10, 0, 'target', TRUE
  FROM generate_series(1,5) AS lv;

-- 毒雾精确对应官方中毒状态 22..26（60 秒，总伤害 360/450/570/720/900）；
-- 封印与冻气同样选取持续时间逐项相等的官方状态等级。
INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec)
SELECT 10214, lv, 1, 1001, 21 + lv, 0
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10215, lv, 1, 1003, 2 + lv, 0
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10216, lv, 1, 1004, 2 * lv, 0
  FROM generate_series(1,5) AS lv;

-- 侦测按每枚范围内敌方陷阱独立判定并只向施法者显形；解除陷阱只能选择
-- 已经看见的敌方陷阱。概率全部来自五级技能说明。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, chance_bp, effect_target)
SELECT 10228, lv, 1, 'detect_trap', 3700 + lv * 800, 'caster'
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10218, lv, 1, 'disarm_trap', 4700 + lv * 800, 'target'
  FROM generate_series(1,5) AS lv;

-- D4：鹰眼说明明确“半径 300、现形后暂时无法再次隐身”，但客户端及公开
-- 旧资料都没有留下五级压制时长。这里把逐级增强参数集中保存为
-- 10/15/20/25/30 秒，便于获得旧服实证后只修数据；运行时不写死猜测值。
INSERT INTO game_custom_statuses
    (status_id, status_level, name, description, status_type, duration_sec,
     prevent_invisible)
SELECT 10327, lv, '现形', format('%s秒内无法再次进入隐身状态。', 5 + lv * 5),
       1, 5 + lv * 5, TRUE
  FROM generate_series(1,5) AS lv;

INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, flat_value, effect_target)
SELECT 10327, lv, 1, 'reveal_invisible', 300, 'caster'
  FROM generate_series(1,5) AS lv;

INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec)
SELECT 10327, lv, 1, 10327, lv, 0
  FROM generate_series(1,5) AS lv;

-- D5：雷火弹先结算说明中的物理伤害，真实命中且怪物存活才震退。客户端与
-- 旧服资料只确认五级击退相同、没有留下距离值；100 世界单位作为显式参数
-- 集中落库，后续取得实机证据时只需调整数据。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, power_pct, flat_value,
     effect_target, requires_previous_landed)
SELECT 10223, lv, 1, 'damage', 155 + lv * 5, 0, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10223, lv, 2, 'knockback', 0, 100, 'target', TRUE
  FROM generate_series(1,5) AS lv;

-- E1：四个即死技能都先结算说明中的普通伤害，只有该段真实命中且目标仍
-- 存活时才独立掷即死概率。概率直接来自五级技能说明。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, power_pct, chance_bp,
     effect_target, requires_previous_landed)
SELECT 10123, lv, 1, 'damage',
       (ARRAY[165,175,185,195,205])[lv], 10000, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10414, lv, 1, 'damage',
       (ARRAY[350,360,370,380,390])[lv], 10000, 'target', FALSE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10004, lv, 2, 'instant_death', 0, 300, 'target', TRUE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10123, lv, 2, 'instant_death', 0, 200, 'target', TRUE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10210, lv, 2, 'instant_death', 0, 300 + lv * 200, 'target', TRUE
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10414, lv, 2, 'instant_death', 0, 400, 'target', TRUE
  FROM generate_series(1,5) AS lv;

-- 光盾术复用官方吸收盾 1021/1..5，但即死免疫是技能来源补充的明确语义，
-- 不能污染复用同一状态定义的其它来源。
INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, duration_sec,
     description_override, instant_death_immune)
SELECT 10320, lv, 1, 1021, lv, 0,
       format('吸收伤害%s点；免疫即死效果。', 500 + lv * 200), TRUE
  FROM generate_series(1,5) AS lv;

-- E2：复活术只能选取死亡队友，恢复 5/15/25/35/45% 生命，并取回
-- 60/70/80/90/100% 的本次死亡实际损失。
INSERT INTO game_skill_effects
    (skill_id, skill_level, effect_order, effect_kind, max_resource_pct,
     loss_refund_pct)
SELECT 10305, lv, 1, 'resurrect', 10 * lv - 5, 50 + 10 * lv
  FROM generate_series(1,5) AS lv;

-- E3：两项冻结技能的触发率和持续时间均由技能五级说明完整给出。状态 1004
-- 提供冰冻控制与客户端表现，技能配置覆盖自己的精确时长和独立触发概率。
INSERT INTO game_skill_statuses
    (skill_id, skill_level, status_order, status_id, status_level, chance_bp, duration_sec)
SELECT 10410, lv, 1, 1004, 1, 2500, 15
  FROM generate_series(1,5) AS lv
UNION ALL
SELECT 10411, lv, 1, 1004, 1, 5000, 5
  FROM generate_series(1,5) AS lv
ON CONFLICT (skill_id, skill_level, status_order) DO UPDATE
  SET status_id = EXCLUDED.status_id,
      status_level = EXCLUDED.status_level,
      chance_bp = EXCLUDED.chance_bp,
      duration_sec = EXCLUDED.duration_sec;

-- 客户端原生施放入口使用的最终距离及前置。
UPDATE gamedata.ov_skilldesc SET depend1_id=10307 WHERE skill_id=10311;
UPDATE gamedata.ov_skilldesc SET dist=150 WHERE skill_id BETWEEN 10213 AND 10217 AND skill_level BETWEEN 1 AND 5;

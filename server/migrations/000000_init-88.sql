-- Monster behavior decision, 2026-10-19. Data only; no player rows or history changes.
-- Scope: existing combat templates of levels 1..60 and positive HP.
-- Active: ov_dialog Scene=2; flee: Scene=7. No elite inheritance.
-- Help: approved dialogue-based group-assistance list. Pickup: approved names,
-- including only the two approved human kinds. Detect-hidden is unchanged.
-- Unlisted scoped monsters have these four behaviors disabled. Level >60 is preserved.
ALTER TABLE game_monsters ADD COLUMN IF NOT EXISTS flee BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE game_monsters ADD COLUMN IF NOT EXISTS pickup BOOLEAN NOT NULL DEFAULT FALSE;

-- Milliseconds match the existing AI template time configuration conventions.
ALTER TABLE game_monster_ai_templates ADD COLUMN IF NOT EXISTS flee_hp_percent INTEGER NOT NULL DEFAULT 20 CHECK (flee_hp_percent BETWEEN 1 AND 99);
ALTER TABLE game_monster_ai_templates ADD COLUMN IF NOT EXISTS flee_duration_ms INTEGER NOT NULL DEFAULT 3000 CHECK (flee_duration_ms > 0);
ALTER TABLE game_monster_ai_templates ADD COLUMN IF NOT EXISTS flee_cooldown_ms INTEGER NOT NULL DEFAULT 10000 CHECK (flee_cooldown_ms > 0);

WITH approved(id, name, aggressive, call_help, flee, pickup) AS (VALUES
    (1002, '食人花', TRUE, FALSE, FALSE, FALSE),
    (1003, '地翼魔', TRUE, FALSE, FALSE, FALSE),
    (1005, '蓝鱼怪', FALSE, FALSE, TRUE, FALSE),
    (1006, '海龟', FALSE, FALSE, TRUE, FALSE),
    (1008, '雪女', FALSE, FALSE, TRUE, FALSE),
    (1010, '护卫', TRUE, FALSE, FALSE, FALSE),
    (1011, '刑天', TRUE, FALSE, FALSE, FALSE),
    (1013, '猫妖', FALSE, FALSE, TRUE, FALSE),
    (1014, '山狼', TRUE, FALSE, FALSE, FALSE),
    (1015, '碧蛇', TRUE, FALSE, TRUE, FALSE),
    (1018, '苍狼皇', TRUE, FALSE, FALSE, FALSE),
    (1020, '狐妖', FALSE, FALSE, TRUE, FALSE),
    (1022, '人虎', TRUE, FALSE, FALSE, FALSE),
    (1028, '小牛魔王', TRUE, FALSE, FALSE, FALSE),
    (1031, '小黑狗', FALSE, TRUE, FALSE, FALSE),
    (1032, '小野猪', FALSE, TRUE, TRUE, FALSE),
    (1033, '闪电鼠', FALSE, TRUE, FALSE, FALSE),
    (1035, '跳跳尸', TRUE, TRUE, FALSE, FALSE),
    (1041, '绿蛙兵', FALSE, TRUE, FALSE, FALSE),
    (1042, '小虎怪', TRUE, FALSE, FALSE, FALSE),
    (1043, '小猴子', FALSE, TRUE, FALSE, TRUE),
    (1049, '步行鸟', FALSE, FALSE, TRUE, FALSE),
    (1060, '兔王', FALSE, FALSE, FALSE, TRUE),
    (1062, '木头人', TRUE, FALSE, FALSE, FALSE),
    (1065, '地藏小妖', TRUE, TRUE, TRUE, FALSE),
    (1069, '黄鱼怪', FALSE, FALSE, TRUE, FALSE),
    (1070, '红鱼怪', FALSE, FALSE, TRUE, FALSE),
    (1073, '魔王飞天猪', TRUE, FALSE, TRUE, FALSE),
    (1074, '鬼火', FALSE, FALSE, TRUE, FALSE),
    (1082, '石身人面怪', TRUE, FALSE, FALSE, FALSE),
    (1084, '机关器', TRUE, FALSE, FALSE, FALSE),
    (1085, '不动明王', TRUE, FALSE, FALSE, FALSE),
    (1087, '孟婆汤怪', TRUE, FALSE, FALSE, FALSE),
    (1088, '仙符', TRUE, FALSE, FALSE, FALSE),
    (1089, '笔仙', FALSE, FALSE, TRUE, FALSE),
    (1090, '牛头', TRUE, FALSE, FALSE, FALSE),
    (1091, '马面', TRUE, FALSE, FALSE, FALSE),
    (1092, '黑白无常', TRUE, FALSE, FALSE, FALSE),
    (1093, '双头鬼侍', TRUE, FALSE, FALSE, FALSE),
    (1094, '木制小飞兔', FALSE, TRUE, FALSE, TRUE),
    (1095, '木箱怪', TRUE, FALSE, FALSE, FALSE),
    (1096, '齿轮机关人', TRUE, FALSE, FALSE, FALSE),
    (1097, '铁钳人', TRUE, FALSE, FALSE, FALSE),
    (1098, '轮面怪', TRUE, FALSE, FALSE, FALSE),
    (1099, '小钟怪', TRUE, FALSE, FALSE, FALSE),
    (1100, '巨钟人', TRUE, FALSE, FALSE, FALSE),
    (1113, '大闸蟹', TRUE, FALSE, FALSE, FALSE),
    (1114, '人鱼巡逻', TRUE, FALSE, FALSE, FALSE),
    (1115, '墨鱼仔', TRUE, FALSE, FALSE, FALSE),
    (1116, '人鱼护卫', TRUE, FALSE, FALSE, FALSE),
    (1117, '虾护卫', TRUE, FALSE, FALSE, FALSE),
    (1118, '美人鱼王', TRUE, FALSE, FALSE, FALSE),
    (1119, '三头龙', TRUE, FALSE, FALSE, FALSE),
    (1120, '黄蜂', TRUE, TRUE, FALSE, FALSE),
    (1123, '赤焰毒蛇', TRUE, FALSE, TRUE, FALSE),
    (1128, 'QQ糖', FALSE, TRUE, FALSE, FALSE),
    (1131, '黑蜂', TRUE, TRUE, FALSE, FALSE),
    (1133, '牛头面具', FALSE, TRUE, FALSE, FALSE),
    (1134, '鹿角面具', FALSE, TRUE, FALSE, FALSE),
    (1135, '鹅翅面具', FALSE, TRUE, FALSE, FALSE),
    (1136, '守护面具王', TRUE, TRUE, FALSE, FALSE),
    (1141, '火鸟', TRUE, FALSE, FALSE, FALSE),
    (1143, '飞天猪', TRUE, FALSE, FALSE, FALSE),
    (1144, '飞天猴子', FALSE, FALSE, FALSE, TRUE),
    (1148, '绒绒兔', FALSE, FALSE, FALSE, TRUE),
    (1149, '白兔', FALSE, FALSE, FALSE, TRUE),
    (1150, '粉粉兔', FALSE, FALSE, FALSE, TRUE),
    (1151, '浪漫粉粉兔', FALSE, FALSE, FALSE, TRUE),
    (1153, '苍狼', TRUE, FALSE, FALSE, FALSE),
    (1154, '小虎怪', TRUE, FALSE, FALSE, FALSE),
    (1156, '甜蜜粉粉兔', FALSE, FALSE, FALSE, TRUE),
    (1161, '仙人掌小妖', TRUE, FALSE, FALSE, FALSE),
    (1162, '灵异小妖', TRUE, FALSE, FALSE, FALSE),
    (1163, '沙漠幻蝶', TRUE, TRUE, FALSE, FALSE),
    (1164, '吸血蚊', TRUE, TRUE, FALSE, FALSE),
    (1165, '巨型蜗牛怪', TRUE, FALSE, FALSE, FALSE),
    (1166, '巨食人花', TRUE, FALSE, FALSE, FALSE),
    (1167, '毒蜘蛛', TRUE, FALSE, FALSE, FALSE),
    (1168, '沙虫', TRUE, FALSE, FALSE, FALSE),
    (1169, '喷射沙虫', TRUE, FALSE, FALSE, FALSE),
    (1170, '蚂蚁幼虫', TRUE, FALSE, FALSE, FALSE),
    (1171, '工蚁', TRUE, FALSE, FALSE, FALSE),
    (1172, '兵蚁', TRUE, TRUE, FALSE, FALSE),
    (1173, '巡逻蚁', TRUE, TRUE, FALSE, FALSE),
    (1174, '守卫蚁', TRUE, TRUE, FALSE, FALSE),
    (1175, '骑兵蚁', TRUE, TRUE, FALSE, FALSE),
    (1201, '盗墓贼', TRUE, FALSE, FALSE, TRUE),
    (1202, '干尸', TRUE, FALSE, FALSE, FALSE),
    (1203, '干尸守卫', TRUE, TRUE, FALSE, FALSE),
    (1204, '妖魔女仆', TRUE, FALSE, FALSE, FALSE),
    (1205, '妖魔仆人', TRUE, FALSE, FALSE, FALSE),
    (1206, '亡灵贵族', TRUE, FALSE, FALSE, FALSE),
    (1207, '妃子之魂', TRUE, FALSE, FALSE, FALSE),
    (1208, '死亡之后', TRUE, FALSE, FALSE, FALSE),
    (1209, '死亡之王', TRUE, TRUE, FALSE, FALSE),
    (3010, '机关兔', FALSE, FALSE, FALSE, TRUE),
    (3017, '飞天小猴', FALSE, FALSE, FALSE, TRUE),
    (4001, '坚强的粉粉兔', FALSE, FALSE, FALSE, TRUE),
    (4003, '坚强的绒绒兔', FALSE, FALSE, FALSE, TRUE),
    (4010, '坚强的小猴子', FALSE, FALSE, FALSE, TRUE),
    (4016, '坚强的白兔', FALSE, FALSE, FALSE, TRUE),
    (4022, '凶暴的机关兔', FALSE, FALSE, FALSE, TRUE),
    (4041, '凶暴的木制小飞兔', FALSE, FALSE, FALSE, TRUE),
    (4047, '勇猛的飞天小猴', FALSE, FALSE, FALSE, TRUE),
    (4058, '天魔兔王', FALSE, FALSE, FALSE, TRUE),
    (4061, '勇猛的飞天猴子', FALSE, FALSE, FALSE, TRUE),
    (4141, '凶暴的盗墓贼', FALSE, FALSE, FALSE, TRUE),
    (5037, '木制小飞兔', FALSE, FALSE, FALSE, TRUE),
    (5081, '盗墓贼', FALSE, FALSE, FALSE, TRUE),
    (5096, '机关兔', FALSE, FALSE, FALSE, TRUE),
    (5103, '飞天小猴', FALSE, FALSE, FALSE, TRUE),
    (12176, '红毛剑齿猿', FALSE, FALSE, FALSE, TRUE)
), desired AS (
    SELECT m.id,
           COALESCE(a.aggressive, FALSE) AS aggressive,
           COALESCE(a.call_help, FALSE) AS call_help,
           COALESCE(a.flee, FALSE) AS flee,
           COALESCE(a.pickup, FALSE) AS pickup
    FROM game_monsters m
    LEFT JOIN approved a ON a.id = m.id AND a.name = m.name
    WHERE m.level BETWEEN 1 AND 60 AND m.hp > 0
      AND m.kind IN ('普通', '精英', 'BOSS')
)
UPDATE game_monsters m
SET aggressive = d.aggressive, call_help = d.call_help,
    flee = d.flee, pickup = d.pickup
FROM desired d WHERE d.id = m.id;

-- All approved rows contribute at least one flag. Missing/renamed/out-of-scope
-- approved identities therefore fail these totals instead of silently succeeding.
DO $behavior_guard$
DECLARE
    actual INTEGER[];
BEGIN
    SELECT ARRAY[count(*) FILTER (WHERE aggressive)::INTEGER,
                 count(*) FILTER (WHERE call_help)::INTEGER,
                 count(*) FILTER (WHERE flee)::INTEGER,
                 count(*) FILTER (WHERE pickup)::INTEGER]
    INTO actual
    FROM game_monsters
    WHERE level BETWEEN 1 AND 60 AND hp > 0
      AND kind IN ('普通', '精英', 'BOSS');
    IF actual IS DISTINCT FROM ARRAY[68,23,15,27] THEN
        RAISE EXCEPTION 'Monster behavior approved identity/scope mismatch: expected active/help/flee/pickup 68/23/15/27, got %. Check approved IDs, names, levels, HP and kinds before retrying.', actual;
    END IF;
END
$behavior_guard$;

-- 跨级打低级怪的等级差惩罚参数（定案，见 docs/战斗公式-贴吧调研.md 的等级差章节）。
--
-- 颜色档分界来自客户端实测（fake-fantasy-online TargetNameColor）：
--   diff = 怪物等级 − 玩家等级
--   diff ≥ 10   紫色：经验递减
--   3 ≤ diff ≤ 9 红色：满经验、满掉率
--   0 ≤ diff ≤ 2 黄色：满经验、满掉率
--   −9 ≤ diff ≤ −1 绿色：经验递减（掉率 |diff|>5 才减）
--   diff ≤ −10  灰色：经验继续递减到 0
--
-- 定案数值（用户拍板）：经验每级 −8%（含紫色）；掉率 |diff|≤5 满、
-- 之后每级 −8%、下限 50%（双向）。倍率一律用万分数（bp），
-- 避免掉率 0.00045% 这类小概率被整数百分比抹成 0。
CREATE TABLE IF NOT EXISTS game_level_penalty (
    id              SMALLINT PRIMARY KEY CHECK (id = 1),
    exp_full_above  INT NOT NULL DEFAULT 9,    -- 怪比玩家高 ≤ 此级差：经验满（红色档上限）
    exp_step_bp     INT NOT NULL DEFAULT 800,  -- 经验每级递减万分数（800 = 8%）
    exp_below_start INT NOT NULL DEFAULT 1,    -- 怪比玩家低 ≥ 此级差：经验开始递减（1 = 低1级即绿档）
    drop_full       INT NOT NULL DEFAULT 5,    -- 等级差绝对值 ≤ 此级差：掉率满
    drop_step_bp    INT NOT NULL DEFAULT 800,  -- 掉率每级递减万分数（800 = 8%）
    drop_floor_bp   INT NOT NULL DEFAULT 5000, -- 掉率下限万分数（5000 = 50%），到线后不再递减
    CONSTRAINT level_penalty_exp_full CHECK (exp_full_above >= 0),
    CONSTRAINT level_penalty_exp_below CHECK (exp_below_start >= 1),
    CONSTRAINT level_penalty_drop_full CHECK (drop_full >= 0),
    CONSTRAINT level_penalty_steps CHECK (
        exp_step_bp BETWEEN 0 AND 10000 AND
        drop_step_bp BETWEEN 0 AND 10000 AND
        drop_floor_bp BETWEEN 0 AND 10000)
);

INSERT INTO game_level_penalty(id) VALUES (1)
ON CONFLICT(id) DO NOTHING;

DO $$
DECLARE n INT;
BEGIN
    SELECT count(*) INTO n FROM game_level_penalty WHERE id = 1;
    IF n <> 1 THEN RAISE EXCEPTION 'game_level_penalty 缺少 id=1 配置'; END IF;
END $$;

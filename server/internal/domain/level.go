package domain

// 等级、经验与成长。
//
// 数据出处:
//
//	game_levels          150 级的经验曲线（客户端 .bin 解出）
//	ov_levelup.*_odd/even 每级六维成长, **按等级奇偶取值**
//
// V5 is a continuous 1–100 progression track using per-level exp only.
// Historical exp_accum is corrupt above 60 and is never used or migrated.
const LevelCap = 100
const ProgressionVersion = "progression-v5"

// LevelTable 是经验曲线。
//
// need[L] = 从 L 级升到 L+1 级所需的经验。索引 0 不用。
type LevelTable struct {
	need []int64
}

// NewLevelTable 由 (等级 → 升级所需经验) 建表。level 从 1 开始连续。
func NewLevelTable(need map[int32]int64) *LevelTable {
	maxL := int32(0)
	for l := range need {
		if l > maxL {
			maxL = l
		}
	}
	t := &LevelTable{need: make([]int64, maxL+2)}
	for l, v := range need {
		if l >= 1 {
			t.need[l] = v
		}
	}
	return t
}

// MaxLevel 返回表里能升到的最高等级。
func (t *LevelTable) MaxLevel() int32 {
	if t == nil {
		return 1
	}
	return int32(len(t.need)) - 2
}

// Need 返回从 level 升到 level+1 所需的经验。0 表示升不上去(到顶了或没数据)。
func (t *LevelTable) Need(level int32) int64 {
	if t == nil || level < 1 || int(level) >= len(t.need) {
		return 0
	}
	return t.need[level]
}

// GainResult 是一次加经验的结果。
type GainResult struct {
	Levels                   int32 // 升了几级
	FromLevel                int32
	ToLevel                  int32
	Overflowed               bool // 因为到了等级上限而丢弃了溢出的经验
	AppliedExp, DiscardedExp int64
	Reason                   string // nonempty rejects invalid input without mutating the character
}

// AddExp 给角色加经验并处理连升。
//
// 经验存的是**当前等级内的进度**, 不是累计总量 —— 理由是飞升会把经验清零重来,
// 存累计值的话那一刻要做一次全量迁移。存进度则天然无所谓。
//
// 到达上限之后经验不再累积(Overflowed=true)。留着一个涨不动的数只会让人困惑,
// 而且将来飞升解锁时那笔存量该怎么算是笔糊涂账。
func (c *Character) AddExp(t *LevelTable, gain int64, cap int32) GainResult {
	res, level, progress := settleExperience(c.Level, c.Exp, gain, cap, t.Need)
	if res.Reason == "" {
		c.Level, c.Exp = level, progress
	}
	return res
}

// ── 每级成长 ──

// growth 是每级六维成长, 分奇数级与偶数级两套。
//
// **[实证]** 取自 `ov_levelup` 的 `*_odd` / `*_even` 列, prof=0 的五行模板
// 与 prof=1~5 的实例行完全一致。data 层有集成测试拿真表逐格对。
//
// 奇偶交替是用来做出 1.5 这种小数成长的（剑客的精神 odd=1 / even=2 → 平均 1.5）,
// **服务端必须按等级奇偶取值, 不能取平均** —— 取平均会让 1 级到 2 级少长半点,
// 而属性是整数, 那半点会永久丢失。
var growth = [...]struct{ Odd, Even Base }{
	Warrior: {
		Odd:  Base{STR: 2, VIT: 5, INT: 0, SPI: 1, AGI: 1, DEX: 1},
		Even: Base{STR: 2, VIT: 5, INT: 0, SPI: 1, AGI: 1, DEX: 1},
	},
	Swordsman: {
		Odd:  Base{STR: 2, VIT: 4, INT: 0, SPI: 1, AGI: 1, DEX: 1},
		Even: Base{STR: 2, VIT: 4, INT: 0, SPI: 2, AGI: 1, DEX: 2},
	},
	Assassin: {
		Odd:  Base{STR: 1, VIT: 3, INT: 0, SPI: 1, AGI: 3, DEX: 2},
		Even: Base{STR: 1, VIT: 3, INT: 0, SPI: 1, AGI: 3, DEX: 2},
	},
	Healer: {
		Odd:  Base{STR: 1, VIT: 3, INT: 1, SPI: 2, AGI: 1, DEX: 1},
		Even: Base{STR: 2, VIT: 3, INT: 2, SPI: 2, AGI: 1, DEX: 1},
	},
	Warlock: {
		Odd:  Base{STR: 0, VIT: 3, INT: 2, SPI: 3, AGI: 1, DEX: 1},
		Even: Base{STR: 0, VIT: 3, INT: 2, SPI: 3, AGI: 1, DEX: 1},
	},
}

// GrowthAt 返回升到 level 级时获得的六维成长。
//
// **奇偶看的是"升到几级"**: 升到 2 级用 even, 升到 3 级用 odd。
func GrowthAt(r Race, level int32) Base {
	if !r.Valid() {
		r = Warrior
	}
	g := growth[r]
	if level%2 == 0 {
		return g.Even
	}
	return g.Odd
}

// LevelUpGains 是升一级拿到的东西。
type LevelUpGains struct {
	Base        Base  // 六维成长
	FreePoints  int32 // 自由分配点
	SkillPoints int32 // 战斗技能点
}

// SkillPointsPerLevel 是飞升前每次升级获得的战斗技能点。
// 协议与客户端技能面板都维护独立 skillPoints；生活技能明确不消耗它。
const SkillPointsPerLevel int32 = 1

// ApplyLevelUps 把 from+1 到 to 这几级的成长全部加到角色身上。
//
// 分级累加而不是"按级数乘一次", 因为奇偶两套值不同 —— 连升三级要分别取三次。
func (c *Character) ApplyLevelUps(from, to int32) LevelUpGains {
	var g LevelUpGains
	for lv := from + 1; lv <= to; lv++ {
		add := GrowthAt(c.Race, lv)
		g.Base = g.Base.Add(add)
		g.FreePoints += FreePointsPerLevel
		g.SkillPoints += SkillPointsPerLevel
	}
	// 基准是**升级前**那一级应有的六维。
	// AddExp 已经把 c.Level 推到 to 了, 用 EffectiveBase()(按 c.Level 补齐)
	// 会把 from+1..to 这段成长算两遍。
	c.Base = c.BaseAtLevel(from).Add(g.Base)
	c.FreePoints += g.FreePoints
	c.SkillPoints += g.SkillPoints
	c.SkillPointsKnown = true
	return g
}

package domain

// 怪物模板与刷怪点 —— 静态配置的**词汇**部分。
//
// 放在 domain 而不是 data，是因为游戏层要能说出"一种怪长什么样"这句话，
// 却不该知道它是从 Postgres 读的还是从文件读的。data 负责装满这些结构体。

// MonsterKind 是怪的类别。取值来自 game_monsters.kind（客户端自己的措辞）。
//
// 类别不只是标签，它决定三件事：会不会主动打人、能不能被打死、多久重生。
type MonsterKind uint8

const (
	MonsterNormal MonsterKind = iota // 普通
	MonsterElite                     // 精英战斗类别；是否显示脚下光圈由运行时转换来源独立决定
	MonsterBoss                      // BOSS
	MonsterProp                      // 场景物件。桌椅箱子之类的摆设
	MonsterGather                    // 采集物。生活技能的采集节点
)

// ParseMonsterKind 把 game_monsters.kind 那一列的中文映射成枚举。
// 认不出来的按普通处理 —— 宁可多刷一只普通怪，也别因为一个没见过的词把整张图刷不出来。
func ParseMonsterKind(s string) MonsterKind {
	switch s {
	case "精英":
		return MonsterElite
	case "BOSS":
		return MonsterBoss
	case "场景物件":
		return MonsterProp
	case "采集物":
		return MonsterGather
	}
	return MonsterNormal
}

func (k MonsterKind) String() string {
	switch k {
	case MonsterElite:
		return "精英"
	case MonsterBoss:
		return "BOSS"
	case MonsterProp:
		return "场景物件"
	case MonsterGather:
		return "采集物"
	}
	return "普通"
}

// Hostile 报告这类怪会不会主动找玩家打。
//
// 摆设和采集物永远不会 —— 一张图里 5023 个刷怪点是场景物件，
// 要是它们也会追人，主城会变成修罗场。
func (k MonsterKind) Hostile() bool {
	return k == MonsterNormal || k == MonsterElite || k == MonsterBoss
}

// MonsterDef 是一种怪的模板 —— **"哪种怪"，不是"哪一只"**。
// 对应 game_monsters 的一行；运行时的每一只由 EntityID 标识。
type MonsterDef struct {
	ID      MonsterID
	Name    string
	Kind    MonsterKind
	EliteID MonsterID // 普通怪对应的光圈精英模板；0 表示没有精英版本
	Level   int32
	HP      int32
	Exp     int64
	Sprite  int32 // 模型号，出场包直接用（→ ov_cmon.map_id）
	Stats   Stats
	// ColorProfile 是这只怪掉出的随机装备使用的“染色”（属性条数）档位，
	// 由 kind 经 game_monster_color_profiles 查得。运行时光圈怪用的是精英
	// 模板，因此拿到的也是精英档。
	ColorProfile ColorProfile

	Aggressive    bool  // 是否主动攻击。15 级前的普通怪不主动，变成光圈(精英)后会
	NoAttack      bool  // 是否完全不能攻击；仍可被玩家攻击、死亡和掉落
	NoBasicAttack bool  // 仍会施放技能，但不进行普通攻击
	CallHelp      bool  // 是否呼叫同伴
	ViewDist      int32 // 索敌半径
	TraceDist     int32 // 追击半径。超出只丢目标，怪物留在脱战位置继续活动
	AI            MonsterAIProfile
	Skills        []MonsterSkillRule
	// ImmuneSkills 来自 game_monster_skills.kind='immune'。怪物免疫的是明确的
	// 技能号，不是按伤害类型做模糊推断。
	ImmuneSkills map[SkillID]struct{}
}

// MonsterAIProfile 是数据库驱动的一套怪物行为模板。模板负责“什么时候以及
// 在多远处做决定”；每只怪的技能规则负责“具体选哪一个技能”。
type MonsterAIProfile struct {
	ID string

	ViewDist        int32
	TraceDist       int32
	BasicAttackDist int32
	KeepDist        int32
	HelpRadius      int32

	SkillCheckEvery Tick
	GlobalCooldown  Tick
}

// MonsterSkillRule 是一只怪对某个技能的完整运行规则。原始怪物表只给技能号与
// 等级；概率、冷却、血线和优先级由服务端配置表补齐后才是一条可执行规则。
type MonsterSkillRule struct {
	Slot  int32
	Self  bool
	Skill SkillDef

	Priority     int32
	ChanceBP     int32 // 万分比，10000 = 必定
	Cooldown     Tick
	InitialDelay Tick
	MinHPPct     int32
	MaxHPPct     int32
	// MaxCastsPerLife 限制一次出生生命周期内最多成功释放几次；0 表示不限。
	// 它用于狂暴、超级化等阶段转换技能，不能拿超长冷却冒充一次性机制。
	MaxCastsPerLife int32
	Enabled         bool

	Extra []MonsterSkillExtra
}

func (r MonsterSkillRule) Usable() bool {
	return r.Enabled && r.Skill.Kind.Active() && (r.Skill.Usable() || len(r.Extra) > 0)
}

// MonsterSkillExtra 是直接伤害、治疗和状态之外的技能结果。前三类复用
// SkillDef；这里只表达确实需要场景参与的动作。
type MonsterSkillExtraKind uint8

const (
	MonsterExtraNone MonsterSkillExtraKind = iota
	MonsterExtraDispelBeneficial
	MonsterExtraDispelHarmful
	MonsterExtraClearAggro
	MonsterExtraDespawn
	MonsterExtraSummon
	MonsterExtraRandomStatus
)

type MonsterSkillExtra struct {
	Kind MonsterSkillExtraKind
	// SummonMonster/Count/Lifetime 仅用于召唤；其它动作保持零值。
	SummonMonster MonsterID
	Count         int32
	Lifetime      Tick
	Statuses      []StatusApplication
}

// NewMonsterStats 用单一攻击值组装怪物属性。它保留给测试夹具及只有单值的临时
// 召唤定义；数据库怪物走 NewMonsterStatsRange，使用配置好的攻击上下限。
func NewMonsterStats(atk, def, hit, matk, mdef, atkSpeedMS, moveSpeed int32) Stats {
	return NewMonsterStatsRange(atk, atk, def, hit, matk, mdef, atkSpeedMS, moveSpeed)
}

// NewMonsterStatsRange 由 game_monsters 的攻击区间及其它数值列组装二级属性。
func NewMonsterStatsRange(atkMin, atkMax, def, hit, matk, mdef, atkSpeedMS, moveSpeed int32) Stats {
	return Stats{
		MinAtk: atkMin, MaxAtk: atkMax,
		Def: def, Hit: hit,
		MAtk: matk, MDef: mdef,
		AtkSpeedMS: atkSpeedMS,
		MoveSpeed:  moveSpeed,
	}
}

// SpawnPoint 是地图上的一个刷怪点，来自客户端 `<map>.lst`。
//
// ⚠️ **客户端数据里没有重生间隔。** 45635 个刷怪点、四个键
// （`id` / `index` / `init_pos` / `init_dir`），一个都没有。
// 所以"多久刷一只"完全是服务端的决定，见 game/spawn 的 RespawnTicks。
type SpawnPoint struct {
	ID      int32 // 图内唯一（.lst 里的 id）
	Monster MonsterID
	Pos     Pos
	// Dir 是 .lst 的 init_dir 原值。**单位未知** ——
	// 实测值域 0~155 共 13 种，其中 45416/45635 都是 67。
	// 既不是 0~7 也不是角度，没搞清之前原样透传，不瞎换算。
	Dir int32
}

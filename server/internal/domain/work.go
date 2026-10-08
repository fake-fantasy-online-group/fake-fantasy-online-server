package domain

import "time"

// 打工与耐力。
//
// 数据出处：`game_work`（33 行）+ `game_stamina_rule`（单行），
// 由 migration 0022 建表并导入，规则见 docs/MVP边界.md「打工与耐力」。
//
// **这是从点卡节奏改成免费游戏节奏的地方，不是还原原作。**
// 原版按"占着号挂机"设计：耐力半小时回 2 点、上限 100，打工每 10 分钟扣 2 点 ——
// 100 点能挂 500 分钟（8.3 小时），每 180 秒结算一次 = 一轮 166 次。
//
// 改成：
//
//	耐力      每天回满一次，一次 100 点（不再按半小时滴回）
//	消耗      每分钟 −1 点  → 100 分钟耗尽
//	结算间隔  全部 ÷5：打工 180→36 秒、钓鱼 60→12 秒、挖矿 15→3 秒
//	单次收益  不变
//
// **总量与单次收益都没变**：100 分钟 ÷ 36 秒 = 166 次，与原版 500 分钟 ÷ 180 秒 相同。
// 只是把 8 小时压缩进 100 分钟。`game_work.unit_sec_v` 留着原值好对照。

// WorkID 是打工种类号（game_work.work_id）。
type WorkID int32

// WorkDef 是一种打工的定义。
type WorkDef struct {
	ID    WorkID
	Title string // 「打扫卫生中的」这类状态名
	// WorkType 是 ov_work.work_type。挖矿=10、钓鱼=11；装备与消耗品规则
	// 按工种配置，不能按标题字符串或某一个 work_id 写死。
	WorkType int32
	// RequiredSkill 是开工必须已经学会的生活技能。0 表示不要求技能。
	// 挖矿/钓鱼分别来自 ov_work.skill_id，不能只靠等级放行。
	RequiredSkill SkillID
	// PetSkill/PetSkillAddPct 来自 ov_work.pet_skill_id/pet_skill_add。
	// 100 表示对应宠物辅助技能使工作效率提高 100%（结算速度翻倍）。
	PetSkill       SkillID
	PetSkillAddPct int32

	LevelMin, LevelMax int32
	// UnitSec 结算间隔（秒）。已经是 ÷5 之后的值。
	UnitSec int32

	// 每次结算给的东西。
	Exp   int64
	Honor int64
	Coin  int64
	// CoinProb 给钱的概率，**千分比**（800 = 80%）。
	// 经验和名誉是必给的，只有钱要掷。
	CoinProb int32

	// Items 物品产出。**这是打工的产出大头** ——
	// 88 条产出条目里 81 条是物品，跨 20 个工种；钓鱼更是只给物品。
	Items []WorkItem
	// WeightedPick 决定 Items 怎么掷：
	//
	//	true   掷一次，按 prob 当权重选**一条**（钓鱼：10 个熟练度档，每档概率正好加满 1000）
	//	false  每条独立掷（贩卖物品合计 800、杂务管理合计 1910）
	//
	// 这个标志在派生时就定死了（见 server/derive/work.sql），运行时不按合计猜。
	WeightedPick bool

	// Practise 熟练度区间。钓鱼靠熟练度分档而不是等级 ——
	// 10 档的等级段全是 12~150，真正的区分维度在这里。
	PractiseMin, PractiseMax int32

	// Requirements 是开工及每次结算都必须满足的权威服务端规则。
	// 同一个 Equipment 条目中的 Items 是可替代品，例如普通锄头、精制锄、
	// 三档矿镐都能满足采矿工具这一项。
	Requirements WorkRequirements
}

type WorkRequirementKind uint8

const (
	WorkRequirementOutfit WorkRequirementKind = iota + 1
	WorkRequirementTool
)

type WorkEquipmentRequirement struct {
	Kind  WorkRequirementKind
	Slot  EquipSlot
	Items []ItemID
}

type WorkRequirements struct {
	Equipment []WorkEquipmentRequirement
	// Consumable/ConsumablePerYield 表示每次真实获得产出时消耗的背包物品。
	// 普通打工与挖矿为 0；钓鱼为鱼饵 ×1。
	Consumable         ItemID
	ConsumablePerYield int32
}

func (r WorkRequirements) EquipmentOf(kind WorkRequirementKind) (WorkEquipmentRequirement, bool) {
	for _, req := range r.Equipment {
		if req.Kind == kind {
			return req, true
		}
	}
	return WorkEquipmentRequirement{}, false
}

// WorkItem 是一条物品产出。
type WorkItem struct {
	Item ItemID
	Qty  int32
	// Prob 在逐条独立掷时是千分比；WeightedPick 为 true 时是相对权重。
	// 例如正式服挖矿是铁矿权重 2、其余已解锁矿权重 1，不需要为了凑成
	// 千分比而引入除不尽的舍入误差。
	Prob int32
	// PractiseGainMax 是本条产出一次最多增加的生活技能熟练度。
	// 正式客户端 ov_work 的 res 字段与实服提示共同证明实际增量为 [1,max]
	// 的随机值，不是固定给 max；普通打工为 0。
	PractiseGainMax int32
}

// TotalItemWeight 是全部物品条目的概率之和。权重单选时用它当分母。
func (d WorkDef) TotalItemWeight() int32 {
	var sum int32
	for _, it := range d.Items {
		sum += it.Prob
	}
	return sum
}

// RollItems 掷一次物品产出。roll 传一个 [0,n) 的取数器。
//
// 两种掷法都在这里，**调用方不需要知道自己在钓鱼还是在管杂务**。
func (d WorkDef) RollItems(roll func(n int32) int32) []WorkItem {
	if len(d.Items) == 0 {
		return nil
	}
	if d.WeightedPick {
		total := d.TotalItemWeight()
		if total <= 0 {
			return nil
		}
		// 掷一次，落在哪个区间就是哪条
		x := roll(total)
		for _, it := range d.Items {
			if x < it.Prob {
				return []WorkItem{it}
			}
			x -= it.Prob
		}
		// 概率加满 1000 时走不到这里; 加不满时宁可空手也不要偏向最后一条
		return nil
	}
	var out []WorkItem
	for _, it := range d.Items {
		if roll(1000) < it.Prob {
			out = append(out, it)
		}
	}
	return out
}

// Available 报告某等级能不能干这活。
func (d WorkDef) Available(level int32) bool {
	return level >= d.LevelMin && (d.LevelMax <= 0 || level <= d.LevelMax)
}

// PractiseFits 报告某熟练度落不落在这一档里。
//
// 只有钓鱼这类分档的工种用得上：同一个「钓鱼中的」有 10 行，等级段完全相同，
// 靠 practise 区分。两端都是 0 表示这个工种不分档，恒真。
func (d WorkDef) PractiseFits(practise int32) bool {
	if d.PractiseMin == 0 && d.PractiseMax == 0 {
		return true
	}
	return practise >= d.PractiseMin && practise <= d.PractiseMax
}

// SettleTicks 返回结算间隔对应的帧数。
func (d WorkDef) SettleTicks() Tick {
	if d.UnitSec <= 0 {
		return 0
	}
	return Ticks(int(d.UnitSec) * 1000)
}

// WorkTable 是全部打工定义。
type WorkTable map[WorkID]WorkDef

// ResolveStart 把正式客户端 BeginWork 的参数解析成实际工作档位。
//
// 普通打工传 game_work.work_id；钓鱼/挖矿传生活技能 id（11006/11007）。
// 后两者在 game_work 中各有十个熟练度档，不能拿技能 id 直接查主键，也不能
// 永远固定在第一档。未学习时熟练度按 0 选中第一档，随后由 CanWork 给出
// “未学习技能”的准确拒绝，而不是误报“工作项目不存在”。
func (t WorkTable) ResolveStart(request WorkID, skills Learned) (WorkDef, bool) {
	if d, ok := t[request]; ok {
		return d, true
	}
	skill := SkillID(request)
	practise := skills.LevelOf(skill)
	for _, d := range t {
		if d.RequiredSkill == skill && d.PractiseFits(practise) {
			return d, true
		}
	}
	return WorkDef{}, false
}

// Available 返回某等级能干的所有活。
func (t WorkTable) Available(level int32) []WorkDef {
	var out []WorkDef
	for _, d := range t {
		if d.Available(level) {
			out = append(out, d)
		}
	}
	return out
}

// StaminaRule 是耐力规则（game_stamina_rule 单行配置）。
type StaminaRule struct {
	Max         int32
	RefillDaily bool
	// CostPerMin 打工每分钟消耗多少耐力。
	CostPerMin int32
}

// DefaultStaminaRule 是配置读不到时的兜底，与库里那一行一致。
func DefaultStaminaRule() StaminaRule {
	return StaminaRule{Max: 100, RefillDaily: true, CostPerMin: 1}
}

// StaminaDrainTicks 是掉一点耐力要多久。
//
// 每分钟 −CostPerMin 点，所以一点管 60/CostPerMin 秒。
func (r StaminaRule) StaminaDrainTicks() Tick {
	if r.CostPerMin <= 0 {
		return 0 // 不消耗
	}
	return Ticks(60 * 1000 / int(r.CostPerMin))
}

// RefillStamina 按"每天回满一次"的规则补耐力。
//
// 返回是否补过。判据是**日期变了**，不是"过了 24 小时" ——
// 后者会让每天上线时间越来越晚的人吃亏，而且跨时区讨论起来很麻烦。
//
// day 传当天的日序（把时间折成年内第几天再加年份，见 DayOf）。
func (c *Character) RefillStamina(r StaminaRule, day int32) bool {
	if !r.RefillDaily {
		return false
	}
	if c.StaminaDay == day && c.Stamina > 0 {
		return false
	}
	// 同一天但耐力是 0 也不补 —— 补了就等于每天无限
	if c.StaminaDay == day {
		return false
	}
	c.Stamina = r.Max
	c.StaminaDay = day
	return true
}

// DayOf 把一个时间折成"第几天"。
//
// 用 年×1000 + 年内天序，而不是 Unix 天数 —— 前者在日志里一眼能读出是哪天。
func DayOf(t time.Time) int32 {
	return int32(t.Year())*1000 + int32(t.YearDay())
}

// WorkSession 是一次正在进行的打工。
//
// **不落盘**：下线就停工。存了反而要处理"离线期间算不算工时"这个问题，
// 而那个问题的任何答案都会被人拿去挂机。
type WorkSession struct {
	Def WorkDef
	// NextSettleAt 下次结算的帧。
	NextSettleAt Tick
	// NextDrainAt 下次扣耐力的帧。
	NextDrainAt Tick
	// Settled 已经结算过几次。给日志与展示用。
	Settled int32
}

// WorkReject 是开工被拒的原因。
type WorkReject uint8

const (
	WorkOK WorkReject = iota
	WorkUnknown
	WorkLevelTooLow
	WorkLevelTooHigh
	WorkSkillNotLearned
	WorkNoStamina
	WorkAlreadyWorking
)

// CanWork 判断能不能开工。skillLearned 必须表示技能记录是否存在，
// 不能用熟练度 > 0 代替：刚学会的生活技能可以是 0 熟练度。
func CanWork(d WorkDef, level, stamina int32, skillLearned, working bool) WorkReject {
	if working {
		return WorkAlreadyWorking
	}
	if d.RequiredSkill != 0 && !skillLearned {
		return WorkSkillNotLearned
	}
	if level < d.LevelMin {
		return WorkLevelTooLow
	}
	if d.LevelMax > 0 && level > d.LevelMax {
		return WorkLevelTooHigh
	}
	if stamina <= 0 {
		return WorkNoStamina
	}
	return WorkOK
}

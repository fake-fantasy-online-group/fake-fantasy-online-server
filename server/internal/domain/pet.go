package domain

import "time"

// 宠物。
//
// 数据出处：`ov_petgrow`（504 行 136 列）+ `ov_petlevelexp`（80 行）。
//
// ⚠️ **两条曲线陷阱，都实测过：**
//
//	ov_petlevelexp.exp_total  从 **59 级起全是 2147483647**（int32 饱和），
//	                          只能用 exp_need 自己在 int64 里累加。
//	ov_petgrow 的生命成长列    hp_min/hp_max/vit_hp_add/sp_min/sp_max/int_sp_add
//	                          **504 行全零** —— 生命不是从这几列长出来的。
//
// 生命到底怎么长，见 MaxHPAt 的注释：系数就是角色那个 ×9。

// PetID 是宠物种类号（ov_petgrow.pet_id）。
type PetID int32

// PetDef 是一种宠物的定义。
//
// 136 列里只挑了 MVP 用得上的。没搬进来的大头：坐骑变形、光环、
// 星盘、加点书、交易税 —— 都属于后期养成层，不在 8 大系统的 MVP 里。
type PetDef struct {
	ID   PetID
	Name string
	// Habit is the client food preference code resolved from the species' food item.
	Habit uint8
	// Model 是客户端 0x800f / 0x8036 使用的宠物造型号。它与 PetID 不是
	// 同一套编号（例如海龟 PetID=1006、Model=206）。
	Model int32
	// BlessingSlots 是 pets.json 的“加持能力”；技能效果尚未实现，但配置先
	// 原样保留，不能等实现效果时再从本地 JSON 临时补读。
	BlessingSlots int32

	// ── 捕捉 ──
	// CaptureLevel 0 表示这只**抓不到**（504 只里 486 只如此，
	// 它们靠买/活动/任务获得）。
	CaptureLevel int32
	CaptureTool  ItemID
	// BaseCaptureRate / MaxCaptureRate 捕捉成功率的**值域两端**，百分比。
	// 18/18 只可捕捉宠满足 Max = Base × 1.6 或 1.8，没有第三种比值。
	BaseCaptureRate int32
	MaxCaptureRate  int32

	// ── 初值与成长 ──
	Init   Base
	InitHP int32
	InitSP int32
	// GrowthOdd / GrowthEven 每级六维成长，按等级奇偶取值。
	// 与角色同一套机制（见 level.go 的 growth），504/504 满足 |odd−even| ≤ 1,
	// 其中 321 只两套完全相同 —— 相同的那些不是没数据，是本来就不需要小数成长。
	GrowthOdd, GrowthEven Base
	MaxLevel              int32

	// ── 信赖与饥渴 ──
	InitTrust  int32
	InitStarve int32
	// Starve 是这只宠的饥渴变化速率。504 行里只有两种取值:
	// 476 只是标准四档, 28 只 Delta 全零(永远不饿)。
	Starve StarveRule

	// ── 坐骑 ──
	// HorseBaseSpeed 0 表示不能骑。504 行里 429 行能骑。
	// HorseSpeedAdd 按「坐骑」技能等级增长，HorseSpeedLimit 是封顶。
	// 这组值与角色属性面板的 MoveSpeed 不同单位：它们是客户端场景像素/秒。
	HorseBaseSpeed, HorseSpeedAdd, HorseSpeedLimit int32

	// CanPickUp 这只宠会不会自己捡地上的东西。
	//
	// ⚠️ **列名骗人**：原表这一列叫 `pickup_trust`，看着像个信赖阈值，
	// 但它 504 行只有 0 和 1 两种取值 —— 是个布尔标志，不是门槛。
	// 123 只为 1，**18 只可捕捉宠全在其中**（自己抓的宠都会捡东西）。
	CanPickUp bool

	// RealPet 报告这一行是不是真宠物。
	//
	// `ov_petgrow` 里混着 28 行**不是宠物的东西**：坐骑与「金甲幻化书」
	// （18 本）。它们的判据是**四档饥渴速率全零** —— 而且这 28 行
	// 同时满足 `can_buy=0`、`can_deal=0`、`deal_trust ≠ init_trust−20`，
	// 三个条件 28/28 完全重合，不是巧合。
	RealPet bool

	// InitialSkills 是种族固定携带的技能；Learnable 是升级时可领悟池。
	// 两者来自已经导入 PostgreSQL 的 pets.json 种族配置；运行时不读 JSON。
	InitialSkills []PetSkill
	Learnable     []SkillID
}

// ModelID 返回客户端模型号。没有 pets.json 配置的后期宠物回退到 PetID，
// 只作为数据缺口兜底；正常宠物都应有明确 Model。
func (d PetDef) ModelID() int32 {
	if d.Model > 0 {
		return d.Model
	}
	return int32(d.ID)
}

// Capturable 报告这只宠能不能抓。
func (d PetDef) Capturable() bool { return d.CaptureLevel > 0 }

// Rideable 报告这只宠能不能骑。
func (d PetDef) Rideable() bool { return d.HorseBaseSpeed > 0 }

// RideSpeed 返回 mountSkillLevel 级「坐骑」技能的场景像素速度。
//
// 原表的三列只有一种不丢掉「1 级=基础速度」语义的组合方式：
//
//	min(horse_base_spd + (mountSkillLevel-1) * horse_spd_add, horse_spd_limit)
//
// 独角兽 215/+9/350 与苍狼皇 205/+8/325 都在 16 级精确触顶，
// 与玩家留存记录「坐骑带到 16 最高速度」互相印证。
func (d PetDef) RideSpeed(mountSkillLevel int32) int32 {
	if !d.Rideable() {
		return 0
	}
	if mountSkillLevel < 1 {
		mountSkillLevel = 1
	}
	speed := int64(d.HorseBaseSpeed) + int64(mountSkillLevel-1)*int64(d.HorseSpeedAdd)
	if d.HorseSpeedLimit > 0 && speed > int64(d.HorseSpeedLimit) {
		speed = int64(d.HorseSpeedLimit)
	}
	if speed < 0 {
		return 0
	}
	if speed > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(speed)
}

// GrowthAt 返回升到 level 级时这一级该长的六维。
//
// 与角色一样**按目标等级的奇偶取值，不能取平均** —— 取平均会在整数属性上
// 每两级丢半点，而那半点永久找不回来（理由详见 level.go）。
func (d PetDef) GrowthAt(level int32) Base {
	if level%2 == 1 {
		return d.GrowthOdd
	}
	return d.GrowthEven
}

// BaseAt 返回宠物在 level 级的六维。
//
// 1 级是 Init，之后逐级累加 GrowthAt(该级)。**逐级累加不能用乘法近似** ——
// 奇偶两套成长值不同时，(level−1)/2 的取整会在奇数级上差一整轮。
func (d PetDef) BaseAt(level int32) Base {
	b := d.Init
	if level < 1 {
		return b
	}
	if d.MaxLevel > 0 && level > d.MaxLevel {
		level = d.MaxLevel
	}
	for l := int32(2); l <= level; l++ {
		b = b.Add(d.GrowthAt(l))
	}
	return b
}

// MaxHPAt 返回宠物在 level 级的生命上限。
//
// **[实证] 系数就是角色那个 ×9**：`ov_petgrow` 504 行里 460 行严格满足
// `init_hp = init_vit × 9`（同时 460 行满足 `init_sp = init_spi × 9`）。
// 这是 HPPerVIT/MPPerSPI 这两条系数的**第三个独立来源**，
// 而且样本量比原来的 ov_levelup 十行大两个数量级。
//
// 那 44 个例外全是「物/法」成对的活动宠（刑天·物/刑天·法、吉祥虎(物)/(法)…），
// 用的是同一套模板值：物 393HP/50SP、法 258HP/370SP。
// 不是逐只手调，是两个模板 —— 所以**照抄 init_hp 而不是重算**才对。
//
// 于是一条式子同时覆盖两类：
//
//	MaxHP = InitHP + (当前体质 − 初始体质) × 9
//
// 对那 460 只，它恒等于 `体质 × 9`；对那 44 只，它把手调出来的那段差值
// 一路带到高级 —— 特殊宠该厚一直厚。反过来写成 `体质 × 9` 的话，
// 升一级就把设计者的手调值抹平了，1 级的区别到 2 级就没了。
func (d PetDef) MaxHPAt(level int32) int32 {
	return d.InitHP + (d.BaseAt(level).VIT-d.Init.VIT)*HPPerVIT
}

// MaxSPAt 返回宠物在 level 级的法力上限。同 MaxHPAt，系数是精神那条 ×9。
func (d PetDef) MaxSPAt(level int32) int32 {
	return d.InitSP + (d.BaseAt(level).SPI-d.Init.SPI)*MPPerSPI
}

// ── 捕捉 ──

// CaptureReject 是捕捉被拒的原因。
type CaptureReject uint8

const (
	CaptureOK CaptureReject = iota
	CaptureNotCapturable
	CaptureLevelTooLow
	CaptureNoTool
	CaptureTargetNotWeak
	CaptureBagFull
)

// CanCapture 判断能不能对一只怪下手抓。
//
// hpRatio 是目标当前生命比例（0~1）。
func CanCapture(d PetDef, playerLevel int32, hasTool, bagHasRoom bool, hpRatio float64) CaptureReject {
	if !d.Capturable() {
		return CaptureNotCapturable
	}
	if playerLevel < d.CaptureLevel {
		return CaptureLevelTooLow
	}
	if !hasTool {
		return CaptureNoTool
	}
	if !bagHasRoom {
		return CaptureBagFull
	}
	if hpRatio > CaptureHPThreshold {
		return CaptureTargetNotWeak
	}
	return CaptureOK
}

// CaptureHPThreshold 是"打残了才抓得动"的门槛。
//
// **服务端定** —— 原表只给了成功率的值域两端，没给触发条件。
// 定 0.5 是因为：不设门槛的话满血一发球最划算，捕捉就退化成刷球，
// 而"先打残再抓"是这类系统里唯一有操作感的玩法。
const CaptureHPThreshold = 0.5

// CaptureRate 返回一次捕捉的成功率（百分比，已钳进值域）。
//
// **值域是实证的，插值函数是服务端定的。**
// 表里只有 base 与 max 两端（18/18 满足 max = base × 1.6 或 1.8），
// 中间怎么走没有任何数据 —— `docs/待你确认.md` P3-3 把它挂着当已知缺口。
//
// 这里定成**按目标残血程度线性插值**：满门槛血时取 base，血空时取 max。
// 选线性不是因为它像原版，是因为它是唯一能让玩家从屏幕上估出来的形状；
// 将来真挖到原函数，只要改这一个函数体，值域两端不用动。
func CaptureRate(d PetDef, hpRatio float64) int32 {
	if !d.Capturable() {
		return 0
	}
	if hpRatio < 0 {
		hpRatio = 0
	}
	if hpRatio > CaptureHPThreshold {
		return d.BaseCaptureRate
	}
	// hpRatio 从门槛降到 0 → t 从 0 升到 1
	t := (CaptureHPThreshold - hpRatio) / CaptureHPThreshold
	span := float64(d.MaxCaptureRate - d.BaseCaptureRate)
	rate := float64(d.BaseCaptureRate) + span*t
	return clampRate(int32(rate), d.BaseCaptureRate, d.MaxCaptureRate)
}

func clampRate(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ── 经验 ──

// PetLevelTable 是宠物经验曲线。
//
// 只存**每级所需**，不存累计 —— `ov_petlevelexp.exp_total` 从 59 级起
// 全是 2147483647（int32 饱和），拿它当累计值会在 59 级上凭空多出一道墙。
type PetLevelTable struct {
	need []int64
}

// NewPetLevelTable 由 (等级 → 升到下一级所需经验) 建表。
func NewPetLevelTable(need map[int32]int64) *PetLevelTable {
	maxL := int32(0)
	for l := range need {
		if l > maxL {
			maxL = l
		}
	}
	t := &PetLevelTable{need: make([]int64, maxL+2)}
	for l, v := range need {
		if l >= 1 {
			t.need[l] = v
		}
	}
	return t
}

// Need 返回从 level 升到 level+1 所需的经验。0 表示升不上去。
func (t *PetLevelTable) Need(level int32) int64 {
	if t == nil || level < 1 || int(level) >= len(t.need) {
		return 0
	}
	return t.need[level]
}

// MaxLevel 返回表里能升到的最高等级。
func (t *PetLevelTable) MaxLevel() int32 {
	if t == nil {
		return 1
	}
	return int32(len(t.need)) - 2
}

// PetLevelCap 是 MVP 里宠物的等级上限。
//
// 表里 max_pet_level 给到 80，但**服务端定**：宠物不能超过主人，
// 而主人 60 封顶（LevelCap）。理由是宠物经验来自主人打怪的分成，
// 放开到 80 的话，一个 60 级玩家会挂着一只打得比自己狠的宠 ——
// 那就不是宠物系统，是换了个皮的主角。
const PetLevelCap = LevelCap

// PetCapFor 返回一只宠在某主人等级下的实际等级上限。
func PetCapFor(d PetDef, ownerLevel int32) int32 {
	cap := ownerLevel
	if cap > PetLevelCap {
		cap = PetLevelCap
	}
	if d.MaxLevel > 0 && cap > d.MaxLevel {
		cap = d.MaxLevel
	}
	if cap < 1 {
		cap = 1
	}
	return cap
}

// PetTable 是全部宠物定义。
type PetTable map[PetID]PetDef

// Capturable 返回所有可捕捉的宠。
func (t PetTable) Capturable() []PetDef {
	var out []PetDef
	for _, d := range t {
		if d.Capturable() {
			out = append(out, d)
		}
	}
	return out
}

// ── 玩家养的那一只 ──

// PetInstID 是一只**具体的宠**的数据库主键。
//
// 与 PetID 分开：PetID 说的是"哪种宠"（海龟），PetInstID 说的是"你那只海龟"。
// 两个人各抓一只海龟，PetID 相同、PetInstID 不同。
type PetInstID int64

// NoPetPrefix 表示没有前缀。0 是官方前缀“野蛮”的有效编号，不能拿零值表示无。
const NoPetPrefix int32 = -1

// PetGenderUnknown is kept only while a captured pet has not hatched.
const PetGenderUnknown uint8 = 255

// PetAllocatedUnknown marks legacy pets whose saved base stats contradict their
// deterministic level growth. The client has no unknown-points representation.
const PetAllocatedUnknown int32 = -1

// PetPrefixDef 是孵化时随机得到的前缀定义。
type PetPrefixDef struct {
	ID   int32
	Name string
	// GetRate 是百分比；官方五行合计 43，剩余 57% 即无前缀。
	GetRate int32
	// FreePointsPerLevel 对应 ov_petnickgrow.status_point；再生为 2。
	FreePointsPerLevel int32
	GrowthOdd          Base
	GrowthEven         Base
}

func (d PetPrefixDef) GrowthAt(level int32) Base {
	if level%2 == 1 {
		return d.GrowthOdd
	}
	return d.GrowthEven
}

type PetPrefixTable []PetPrefixDef

func (t PetPrefixTable) Find(id int32) (PetPrefixDef, bool) {
	for _, d := range t {
		if d.ID == id {
			return d, true
		}
	}
	return PetPrefixDef{}, false
}

// PetSkill 是一只宠已经学会的技能及其当前等级。
type PetSkill struct {
	ID    SkillID
	Level int32
}

// PetSkillDef 是宠物技能的展示、等级上限与数据库配置效果。
type PetSkillDef struct {
	Effect      *PetSkillEffect
	ID          SkillID
	Name        string
	Description string
	Icon        int32
	Fight       bool
	// Active 表示技能需要玩家主动使用；它与 PetInstance.ActiveSkill 的
	// “当前正在使用哪一个”是两个不同概念。
	Active     bool
	LearnLevel int32
	MaxLevel   int32
}

type PetSkillTable map[SkillID]PetSkillDef

// PetLearningItem 在使用后保存到宠物身上，只对下一次升级生效。
type PetLearningItem struct {
	Item     ItemID
	Name     string
	Skill    SkillID
	ChanceBP int32
}

// PetLearningBoost 是已经吃下、等待下一次升级结算的药丸效果。
type PetLearningBoost struct {
	Item     ItemID
	Skill    SkillID
	ChanceBP int32
}

// PetRule 是服务端宠物规则。数值由 PostgreSQL game_pet_rule 提供。
type PetRule struct {
	MaxLifeSkills        int32
	MaxFightSkills       int32
	NaturalLearnChanceBP int32
	InitialSkillMin      int32
	InitialSkillMax      int32
	LowTrustRefuseBP     int32
	ZeroTrustRecallSec   int32
	HungerIntervalSec    int32
	TrustHappySec        int32
	TrustContentSec      int32
	DeployHungerAdd      int32
	DeathTrustLoss       int32
	TradeTrustRequired   int32
	TradeTrustLoss       int32
}

func (r PetRule) Valid() bool {
	return r.NaturalLearnChanceBP >= 0 && r.NaturalLearnChanceBP <= 10000 &&
		r.InitialSkillMin >= 0 && r.InitialSkillMax >= r.InitialSkillMin &&
		r.LowTrustRefuseBP >= 0 && r.LowTrustRefuseBP <= 10000 &&
		r.ZeroTrustRecallSec >= 0 &&
		r.HungerIntervalSec > 0 && r.TrustHappySec > 0 && r.TrustContentSec > 0 &&
		r.DeployHungerAdd >= 0 && r.DeathTrustLoss >= 0 &&
		r.TradeTrustRequired >= 0 && r.TradeTrustLoss >= 0
}

// TrustDeltaAt 按当前饥饿值返回信赖变化量与间隔秒数。
func (r PetRule) TrustDeltaAt(hunger int32) (int32, int32) {
	switch {
	case hunger <= 20:
		return 1, r.TrustHappySec
	case hunger <= 50:
		return 1, r.TrustContentSec
	case hunger <= 80:
		return 0, 0
	default:
		return -1, r.TrustHappySec
	}
}

// PetInstance 是玩家养的一只宠。**要落盘。**
//
// 与 WorkSession（不落盘）相反：宠物是花时间抓来养大的资产，
// 掉一只就是掉一段游戏经历，跟掉装备一个性质。
type PetInstance struct {
	ItemUID int64 // stable, globally unique pet inventory identity
	ID      PetInstID
	Slot    int32  // persistent pet-book order; independent from immutable instance identity
	Def     PetID  // 哪种宠
	Name    string // 可改名；空表示用种族名
	Level   int32
	Exp     int64

	// Base 是**当前**六维。出生时 = PetDef.Init，之后逐级加成长。
	// 存快照而不是每次由等级重算 —— 将来加点书/PP 果会往上加，
	// 那些加成不在等级里，重算就把它们抹了。
	Base Base
	// HP/MP 当前值。**落盘** —— 与玩家不同，宠物召回后再放出来
	// 该接着上次的血继续，否则"召回再召唤"就是免费满血。
	HP, MP int32

	Trust  int32 // 忠诚
	Starve int32 // 饱食

	// Hatched=false 是刚捕捉到的未孵化状态。孵化后才公开属性、前缀和技能，
	// 也只有孵化后的宠物可以出战。
	Hatched    bool
	Gender     uint8
	Prefix     int32
	FreePoints int32
	// AllocatedPoints counts ordinary and PP attribute points currently invested.
	AllocatedPoints int32
	// PPAiUsed 是本宠物成功使用宠爱PP果的累计次数，与已分配点数独立。
	PPAiUsed int32
	Skills   []PetSkill
	// ActiveSkill 是当前启用、给主人的主动技能效果来源。
	ActiveSkill SkillID
	// Deployed/Riding 是跨在线生命周期保留的宠物使用状态。场景实体号只在
	// 单次运行时有效，不能落盘；实例上的这两个事实用于重登时重建同一只
	// 出战宠物及其坐骑展示。
	Deployed bool
	Riding   bool
	// RidingSaddle 是本次骑乘的道具来源；0 表示宠物技能骑乘。
	RidingSaddle ItemID
	// PendingLearn 由领悟药丸设置，在下一次真实升级时无论成败都会清掉。
	PendingLearn PetLearningBoost
	// HatchPrefixRatePct 是增强捕捉工具留到孵化时消费的一次性前缀倍率。
	// 0 表示普通100%；未孵化期间必须落盘，避免重登清掉万能绳索效果。
	HatchPrefixRatePct int32
	// TransmogModel/TransmogUntil 是幻化书的临时纯外观覆盖，不改变 Def、属性或技能。
	TransmogModel int32
	TransmogUntil time.Time
}

func (p PetInstance) RenderModel(d PetDef, now time.Time) int32 {
	if p.TransmogModel > 0 && p.TransmogUntil.After(now) {
		return p.TransmogModel
	}
	return d.ModelID()
}

// TotalFreePoints is the current allocatable pool, including invested points.
// A legacy unknown is shown with the old 0/0 brief until its stats are reset.
func (p PetInstance) TotalFreePoints() int32 {
	if p.AllocatedPoints == PetAllocatedUnknown {
		return 0
	}
	return p.FreePoints + p.AllocatedPoints
}

func (p PetInstance) DisplayAllocatedPoints() int32 {
	if p.AllocatedPoints == PetAllocatedUnknown {
		return 0
	}
	return p.AllocatedPoints
}

func (p *PetInstance) ExpireTransmog(now time.Time) bool {
	if p == nil || p.TransmogModel == 0 || p.TransmogUntil.After(now) {
		return false
	}
	p.TransmogModel = 0
	p.TransmogUntil = time.Time{}
	return true
}

// NewPetInstance 由种族定义造一只 1 级的新宠。
func NewPetInstance(d PetDef) PetInstance {
	return PetInstance{
		Def: d.ID, Level: 1, Base: d.Init,
		HP: d.InitHP, MP: d.InitSP,
		Trust: d.InitTrust, Starve: d.InitStarve, Prefix: NoPetPrefix, Gender: PetGenderUnknown,
	}
}

// Skill 返回已学技能的可修改指针。
func (p *PetInstance) Skill(id SkillID) *PetSkill {
	for i := range p.Skills {
		if p.Skills[i].ID == id {
			return &p.Skills[i]
		}
	}
	return nil
}

func (p *PetInstance) Knows(id SkillID) bool { return p.Skill(id) != nil }

// Learn 学会一个新技能。已经会时返回 false，升级由 LevelKnownSkills 负责。
func (p *PetInstance) Learn(id SkillID) bool {
	if id == 0 || p.Knows(id) {
		return false
	}
	p.Skills = append(p.Skills, PetSkill{ID: id, Level: 1})
	for i := len(p.Skills) - 1; i > 0 && p.Skills[i].ID < p.Skills[i-1].ID; i-- {
		p.Skills[i], p.Skills[i-1] = p.Skills[i-1], p.Skills[i]
	}
	return true
}

// LevelKnownSkills 在宠物每升一级时把此前已经学会的技能各升一级，直到各自上限。
// 新技能应在它之后判定，因此刚在本级领悟的技能保持 1 级。
func (p *PetInstance) LevelKnownSkills(defs PetSkillTable) []PetSkill {
	var changed []PetSkill
	for i := range p.Skills {
		d, ok := defs[p.Skills[i].ID]
		if !ok || d.MaxLevel <= 0 || p.Skills[i].Level >= d.MaxLevel {
			continue
		}
		p.Skills[i].Level++
		changed = append(changed, p.Skills[i])
	}
	return changed
}

// CanLearn 报告该技能是否属于本种族的领悟池且尚未学会。
func (p *PetInstance) CanLearn(d PetDef, id SkillID) bool {
	if p.Knows(id) {
		return false
	}
	for _, candidate := range d.Learnable {
		if candidate == id {
			return true
		}
	}
	return false
}

// AddTrust 修改信赖并钳在 [0,100]。
func (p *PetInstance) AddTrust(delta int32) int32 {
	before := p.Trust
	p.Trust += delta
	if p.Trust < 0 {
		p.Trust = 0
	}
	if p.Trust > MaxTrust {
		p.Trust = MaxTrust
	}
	return p.Trust - before
}

// CanTrade/ApplyTrade 固化交易规则。未孵化宠物不受信赖门槛限制且不扣信赖；
// 已孵化宠物必须达到门槛，成功交易后扣除配置值。
func (p PetInstance) CanTrade(rule PetRule) bool {
	return !p.Hatched || p.Trust >= rule.TradeTrustRequired
}

func (p *PetInstance) ApplyTrade(rule PetRule) bool {
	if !p.CanTrade(rule) {
		return false
	}
	if p.Hatched {
		p.AddTrust(-rule.TradeTrustLoss)
	}
	return true
}

// ClonePets 深拷宠物技能切片，供场景快照和内存存储使用。
func ClonePets(in []PetInstance) []PetInstance {
	out := append([]PetInstance(nil), in...)
	for i := range out {
		out[i].Skills = append([]PetSkill(nil), in[i].Skills...)
	}
	return out
}

// DisplayName 返回该显示的名字：改过名就用改的，否则用种族名。
func (p PetInstance) DisplayName(d PetDef) string {
	if p.Name != "" {
		return p.Name
	}
	return d.Name
}

// MaxHP 返回这只宠当前的生命上限。
//
// 走的是 PetDef.MaxHPAt 同一条式子（保留手调差值），
// 但用**实例自己的体质**而不是按等级重算 —— 加点书加的那些点也要算数。
func (p PetInstance) MaxHP(d PetDef) int32 {
	return d.InitHP + (p.Base.VIT-d.Init.VIT)*HPPerVIT
}

// MaxMP 同上，精神那条。
func (p PetInstance) MaxMP(d PetDef) int32 {
	return d.InitSP + (p.Base.SPI-d.Init.SPI)*MPPerSPI
}

// AddExp 给宠物加经验并处理连升。
//
// cap 由调用方按 PetCapFor 算好传进来（宠物不能超过主人）。
// 到顶之后经验不再累积，理由与角色那条相同（见 level.go 的 AddExp）。
func (p *PetInstance) AddExp(d PetDef, t *PetLevelTable, gain int64, cap int32) GainResult {
	return p.AddExpWithPrefix(d, PetPrefixDef{}, t, gain, cap)
}

// AddExpWithPrefix 在基础种族成长之外叠加孵化前缀成长，并发放前缀自由属性点。
func (p *PetInstance) AddExpWithPrefix(d PetDef, prefix PetPrefixDef,
	t *PetLevelTable, gain int64, cap int32) GainResult {
	res := GainResult{FromLevel: p.Level, ToLevel: p.Level}
	if gain <= 0 || t == nil {
		return res
	}
	if p.Level >= cap {
		res.Overflowed = true
		return res
	}
	p.Exp += gain
	for p.Level < cap {
		need := t.Need(p.Level)
		if need <= 0 || p.Exp < need {
			break
		}
		p.Exp -= need
		p.Level++
		// 成长按**升到的那一级**的奇偶取值，与角色同一套
		p.Base = p.Base.Add(d.GrowthAt(p.Level)).Add(prefix.GrowthAt(p.Level))
		p.FreePoints += prefix.FreePointsPerLevel
		res.Levels++
	}
	res.ToLevel = p.Level
	if p.Level >= cap && p.Exp > 0 {
		p.Exp = 0
		res.Overflowed = true
	}
	return res
}

// PetExpShare 是宠物从主人击杀里分到的经验比例（百分比）。
//
// **服务端定** —— 原始数据里没有分成比例（`ov_petgrow` 没有这类列，
// 击杀经验只有怪物那一份）。定 50% 的理由：
// 宠物 80 级曲线比角色 60 级陡得多，给太少就永远跟不上主人，
// 给满又等于挂着宠白拿一倍经验。取一半，宠物大致落后主人几级，
// 正好让"练宠"这件事有事可做但不至于变成必修课。
const PetExpShare = 50

// SharedExp 返回主人拿到 exp 时宠物该分多少。**不从主人那份里扣。**
//
// 扣的话就变成"带宠物练级更慢"，那没人会带宠物 ——
// 而宠物系统的全部意义就是让人带着它。
func SharedExp(exp int64) int64 {
	return exp * PetExpShare / 100
}

// MaxPets matches the ten source-owned Win05.petdlg slots.
const MaxPets = 10

// FindPet 按实例号找一只宠。找不到返回 nil。
//
// 返回**指针**，调用方直接改它就是改角色身上那一份 ——
// 拷贝出去改的话，宠物打怪涨的经验会随拷贝一起丢掉。
func (c *Character) FindPet(id PetInstID) *PetInstance {
	for i := range c.Pets {
		if c.Pets[i].ID == id {
			return &c.Pets[i]
		}
	}
	return nil
}

// ── 饥渴与信赖 ──
//
// 数据出处与口径**全部实证**，来自两个互相印证的来源：
//
//	ov_petgrow          starve_add1~4 / starve_internal1~4 / online_add / online_internal
//	                    504 行只有两种取值(476 只标准 + 28 只全零)，间隔完全一致
//	game_descs 3020~3031 食物描述文本
//
// 两边对得上：`eats1~4_qty` = 3/2/1/5，与描述里的「降低饥渴 3/2/1/5 点」
// **482 行逐个吻合**。这是 [双源]。
//
// ⚠️ **方向别搞反**：这个值叫「饥渴度」，**越高越饿**。
// 食物是「降低饥渴」，所以喂食是往下减。init_starve = 50 是半饱不饿。

// MaxStarve 饥渴度上限。食物描述里的分档写到 100 为止。
const MaxStarve = 100

// StarveTier 是一档饥渴变化速率。
type StarveTier struct {
	// Delta 每次变化多少。**可以是负的** ——
	// 原表用 u8 存负数（255 = −1），解析时已经还原成有符号值。
	Delta int32
	// Interval 多少秒变一次。
	Interval int32
}

// Ticks 返回这一档的触发间隔（帧）。0 表示这一档不生效。
func (t StarveTier) Ticks() Tick {
	if t.Interval <= 0 || t.Delta == 0 {
		return 0
	}
	return Ticks(int(t.Interval) * 1000)
}

// PetState 是宠物当前的状态，决定用哪一档速率。
type PetState uint8

const (
	PetStabled   PetState = iota // 寄养（牧场）。MVP 没有
	PetRecalled                  // 收在宠物栏里
	PetFollowing                 // 放出来跟着走，没在打
	PetFighting                  // 放出来且有攻击目标
)

// StarveRule 是饥渴变化的全套速率。
type StarveRule struct {
	// Tiers 是原表的四档，**按原始下标存**（0 = starve_add1 … 3 = starve_add4）。
	// 不在这里就把它们改名成状态，是因为「哪一档对应哪个状态」是推的，
	// 而这四个数是实证的 —— 把推的部分隔离到 TierFor 一个函数里。
	Tiers [4]StarveTier
	// Online 是在线基础增长（+1 / 4 小时）。
	Online StarveTier
}

// TierFor 返回某个状态该用哪一档。
//
// ⚠️ **[推导·待实测] 这是本文件唯一不确定的地方。**
// 四档的数值是实证的，但原表没有任何列说明「哪一档在什么状态下生效」。
// 依据是这四个数的**结构**：两降两升，每个方向各有一快一慢 ——
//
//	档1  −1/180s   降得最快   → 寄养（最省心的存放方式）
//	档2  −1/360s   降得较慢   → 收在宠物栏里
//	档3  +1/1800s  升得较慢   → 放出来跟着走
//	档4  +1/900s   升得最快   → 放出来打架
//
// 「出力越多饿得越快、放得越好恢复越快」是唯一能让这四个数各就各位的读法。
// 真挖到原始口径的话，改的是这一个函数。
func (r StarveRule) TierFor(st PetState) StarveTier {
	switch st {
	case PetStabled:
		return r.Tiers[0]
	case PetRecalled:
		return r.Tiers[1]
	case PetFollowing:
		return r.Tiers[2]
	case PetFighting:
		return r.Tiers[3]
	}
	return StarveTier{}
}

// DefaultStarveRule 是配置读不到时的兜底，与 476 只宠的真值一致。
func DefaultStarveRule() StarveRule {
	return StarveRule{
		Tiers: [4]StarveTier{
			{Delta: -1, Interval: 180},
			{Delta: -1, Interval: 360},
			{Delta: +1, Interval: 1800},
			{Delta: +1, Interval: 900},
		},
		Online: StarveTier{Delta: +1, Interval: 14400},
	}
}

// PetFood 是一种宠物食物。
type PetFood struct {
	Item ItemID
	Name string
	// Reduce 降低多少饥渴。
	Reduce int32
	// Min/Max 适用的饥渴度区间。**越饿的档降得越少** ——
	// 0~50 降 3、51~75 降 2、76~100 降 1。
	Min, Max int32
	// Trust 附带增加多少信赖。只有无尽淳(3031)有，+2。
	Trust int32
	// Anytime 是否任何时候都能用（无尽淳）。
	Anytime bool
}

// Usable 报告这份食物能不能在当前饥渴度下用。
func (f PetFood) Usable(starve int32) bool {
	if f.Anytime {
		return true
	}
	return starve >= f.Min && starve <= f.Max
}

// StarveRefuseAt 饿到这个程度就不肯出战了。
//
// **服务端定。** 数据里**没有任何一处**说明饥渴度高了会怎样 ——
// 食物描述只讲怎么降，不讲不降会如何。
//
// 但总得有个后果，否则整套饥渴机制就是装饰。选「拒绝出战」而不是
// 「掉属性」或「跑掉」的理由：
//
//	可逆      喂几口就好了，不会永久损失
//	不毁资产  宠物是花时间养大的，让它跑掉是把玩家的投入删掉
//	有代价    饿着就用不了，这个代价足够让人记得喂
//
// 取 MaxStarve 而不是更低的值：低于上限就开始罚，会让人一直在喂食界面里。
const StarveRefuseAt = MaxStarve

// Starving 报告这只宠是不是饿到不能出战了。
func (p PetInstance) Starving() bool { return p.Starve >= StarveRefuseAt }

// Feed 喂一份食物。返回实际降了多少饥渴、加了多少信赖。
//
// 返回 ok=false 表示这一口**白喂** —— 档位不对，或者不饿也没信赖可加。
// **不要静默吞掉**：食物是要钱的，喂下去没效果还扣一份，玩家会以为是 bug。
//
// 判据是"有没有实际效果"而不是"饿不饿"：无尽淳在满饱食时喂下去
// 仍然 +2 信赖，那一口不算白喂。
func (p *PetInstance) Feed(f PetFood) (dStarve, dTrust int32, ok bool) {
	if !f.Usable(p.Starve) {
		return 0, 0, false
	}
	if p.Starve <= 0 && (f.Trust == 0 || p.Trust >= MaxTrust) {
		return 0, 0, false // 不饿, 也没信赖可加 —— 白喂
	}
	before := p.Starve
	p.Starve -= f.Reduce
	if p.Starve < 0 {
		p.Starve = 0
	}
	p.Trust += f.Trust
	if p.Trust > MaxTrust {
		p.Trust = MaxTrust
	}
	return before - p.Starve, f.Trust, true
}

// TradeTrustDrop 交易一只宠要付出的信赖。
//
// **[推导] 476/504 行满足 `deal_trust = can_deal_trust = init_trust − 20`。**
// 两列在 498/504 行里完全相等，取值又恰好是初始值减 20 ——
// 最省的读法是「交易需要的信赖门槛 = 初始 − 20」。
//
// 28 个例外全是上面那批「不是宠物的东西」（can_deal=0，压根不能交易）。
//
// ⚠️ **没有实现。** 玩家间交易不在 MVP 的 8 大系统里（社交只做组队/好友/聊天），
// 这里只把数出来的规律记下来，免得将来重挖一遍。
const TradeTrustDrop = 20

// MaxTrust 信赖上限。
//
// **服务端定**：原表的 init_trust 只有 50 和 100 两种取值，
// 100 出现在活动宠身上 —— 把它当上限是唯一自洽的读法。
const MaxTrust = 100

// AddStarve 按一档速率改一次饥渴度，钳在 [0, MaxStarve]。
func (p *PetInstance) AddStarve(d int32) {
	p.Starve += d
	if p.Starve < 0 {
		p.Starve = 0
	}
	if p.Starve > MaxStarve {
		p.Starve = MaxStarve
	}
}

// PetSkillEffect contains database-authored passive/proc rules. Values are for level one.
type PetSkillEffect struct {
	Kind                       string
	ChanceBP, ChancePerLevelBP int32
	Value, ValuePerLevel       int32
	SourceAttr, SourceStep     int32
	RequireSelected            bool
	// CleanseStatusIDs overrides the legacy single SourceAttr when configured.
	CleanseStatusIDs []int32
}

func (e PetSkillEffect) Chance(level int32) int32 {
	n := int64(e.ChanceBP) + int64(level-1)*int64(e.ChancePerLevelBP)
	if n < 0 {
		return 0
	}
	if n > 10000 {
		return 10000
	}
	return int32(n)
}
func (e PetSkillEffect) Amount(level int32) int32 { return e.Value + (level-1)*e.ValuePerLevel }

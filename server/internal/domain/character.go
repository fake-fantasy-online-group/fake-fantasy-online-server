// Package domain 是纯领域模型层: 只有数据与规则, 无任何 IO/网络/存储依赖, 100% 可单测。
//
// 设计原则(见 docs/架构设计.md §1): 角色/物品/技能等是有行为的领域对象, 不是字节块。
// 协议层负责 领域对象 ↔ 字节 的转换; 本层不认识协议, 只表达游戏概念与规则。
package domain

import (
	"sort"
	"time"
)

// Race 职业/种族，取值与客户端角色数据中的 race 字段一致：0战士、1剑客、2刺客、3药师、4术士。
type Race uint8

const (
	Warrior   Race = 0 // 战士
	Swordsman Race = 1 // 剑客
	Assassin  Race = 2 // 刺客
	Healer    Race = 3 // 药师
	Warlock   Race = 4 // 术士
)

func (r Race) Valid() bool { return r <= Warlock }

// Pos 世界坐标。客户端用 float32, 领域层统一用 float64 表达, 序列化时转换。
type Pos struct {
	MapID int32
	X, Y  float64
}

// Appearance 外观。客户端 head 是发型编号，hair 是发色编号；不是脸型。
type Appearance struct {
	Gender uint8
	Head   uint8 // 发型（客户端 head）
	Hair   uint8 // 发色（客户端 hair）
	// EquipView 是 6 个外观装备槽。客户端那边的名字是
	// apBody / apCap / apBackpack / apWeaponR / apWeaponL / apFace,
	// 字段顺序与客户端角色数据格式一致。
	EquipView [6]uint16
	// 攻击表现。与外观同源, 客户端拿它决定攻击动作和挥砍距离。
	AtkVariant  uint8  // 攻击动作变体
	WeaponCType uint8  // 武器大类
	AtkDist     uint16 // 攻击距离(真实抓包里 75)

	// 公开展示与装备特效偏好。它们与基础脸/发/装备外观一起出现在
	// 0x8003、0x800a、0x8023，必须跟角色一起落盘；否则重登后自己与旁观者
	// 会看到两份不同的外观。
	Title, Aura, SoulFX, TitleFX string
	GlowMode                     uint8
	// GlowModeKnown 区分“真实选择了模式 0”和旧内存构造的零值；持久化角色恒为 true。
	GlowModeKnown   bool
	GlowUnlocked    bool
	EquipFXHideMask int32
}

// HotbarSlotCount 是当前持久化上限。1.3.4 固定 20 格；1.5.8 的 0x1014 在旧
// expanded 字节后再追加 10 格。PostgreSQL 使用可变长数组，因此旧存档自然补零，
// 无需 nullable 字段或 schema 迁移。
const HotbarSlotCount = 30

// HotbarSlot 是快捷栏的一格权威存档。Icon 是物品/技能定义的派生值，不落盘；
// 客户端保存时也只上报 ID 与 Kind。
type HotbarSlot struct {
	ID   int32
	Kind uint8
}

// 快捷栏类别由正式客户端 GameHud 的 KSkill/KPet/KItem 常量与点击分支共同证明。
// 这三个值会原样落盘，服务端不能只靠 ID 空间猜一格属于哪套技能。
const (
	HotbarKindSkill uint8 = iota
	HotbarKindPetSkill
	HotbarKindItem
)

// Look 是渲染任意实体所需的外观数据。玩家看 Race+Appearance, 怪物/NPC 看 ModelID。
// 两者共用一个结构, 是因为出场包对它们用的是同一段字节。
type Look struct {
	ModelID int32 // 怪物/NPC 的模型 id(ov_cmon); 玩家为 0
	Race    Race
	Appearance
}

// Character 是一个玩家角色的完整领域模型。
//
// 当前字段是 A0 骨架所需的最小集(能登录/进世界/存档)。属性系统(力量/HP/MP 等,
// 对应 0x8003/0x8004 里那 166B 属性块)在 A3 战斗里程碑接入, 届时扩展 Stats。
type Character struct {
	ID        int64  // 全局唯一角色 ID(存储分配)
	AccountID int64  // 所属账号
	Slot      int32  // 该账号下的槽位
	Name      string // 角色名(唯一)
	// Race 保存建角时选定的未来职业路线（0..4）。就职前对客户端仍必须表现为
	// 初行者；完成这条路线唯一对应的 400..404 就职任务后才成为该职业。
	Race Race
	// Employed 才是“已经成为对应职业”的权威状态。EmploymentKnown 兼容旧的
	// 内存构造路径：生产角色（建角或 PostgreSQL 加载）恒为 true；老测试夹具
	// 没有这个字段时仍按历史上的已就职角色处理。
	Employed        bool
	EmploymentKnown bool
	Level           int32 // 等级
	Exp             int64 // 经验
	Pos             Pos   // 当前位置
	// SceneInstance 与 Pos.MapID 合起来标识下线时所在的副本实例。
	// 0 表示常驻地图；重登时原实例不存在则改落副本出口。
	SceneInstance uint32
	Base          Base // 六维。零值表示"还没接属性系统", 读的时候走 EffectiveBase()
	// FreePoints 是还没分配的自由点。每级 2 点(ov_levelup.point_add)。
	FreePoints int32
	// BagSlots 背包格数。0 表示按默认值。
	BagSlots int32
	// Skills 是学会的技能: 技能 id → 已学等级。
	Skills Learned
	// SkillPoints 是尚未花掉的战斗技能点。SkillPointsKnown 只区分老的内存构造
	// 路径；PostgreSQL 角色与 NewCharacter 创建的角色始终为 true。
	SkillPoints      int32
	SkillPointsKnown bool
	// Hotbar 与两项本地玩法偏好由客户端上报，但服务端负责持久化并在下次
	// 进入世界时恢复。
	Hotbar         [HotbarSlotCount]HotbarSlot
	HotbarExpanded bool
	SmartCast      bool
	PetViewMask    int32
	// PKMode 是客户端 0..4 的模式偏好。当前服务端尚未开放玩家互伤，
	// 但仍持久化选择并通过 0x8052 恢复客户端 UI 状态。
	PKMode uint8
	// Quests 是任务本: 任务号 → 进度。已完成的也留着, 用来拦重复接。
	Quests QuestLog
	Trial  TrialProgress
	// Money 三种面额分开存, 不折算(见 quest.go 的说明)。
	Money Money
	// Caiyu 是账号钱包在当前在线角色中的投影，数据库唯一来源为 accounts.caiyu。
	// 同账号进入世界受账号租约保护，余额与背包、流水在同一事务提交。
	Caiyu int64
	// Honor 名誉值。273/366 个任务给名誉。
	Honor int64
	// Nianli 念力。特殊修理每件消耗 2 点，在线期间每 10 分钟恢复 4 点，
	// 上限固定为 DefaultNianli。
	Nianli      int32
	NianliKnown bool
	// Stamina 耐力, StaminaDay 是上次回满的日序(见 DayOf)。
	Stamina    int32
	StaminaDay int32
	// Pets 是养的宠物。**跟角色行同事务存** —— 与背包同一个道理,
	// 分开写会留下"宠物升级存了、经验没存"的半截状态。
	Pets []PetInstance

	Appear      Appearance // 外观
	OwnedTitles []string   // 服务端权威的已解锁称号；当前佩戴项在 Appear.Title
	CreatedAt   time.Time
	LastLogin   time.Time

	// Attrs 是发给客户端的属性数组。
	//
	// 长度不固定, **客户端自带长度前缀**(u16 个数 + int32 数组), 所以想发几个
	// 发几个。真实私服抓包里是 35 个, attrs[0] 已确认是等级。
	// 其余下标的含义还没解, 但这已经不是一坨不透明字节了 ——
	// 以前这里是 `StatBlob []byte`, 谁都不知道里面是什么, 长度错一个就出事。
	Attrs []int32
	// SavedStatuses 与经验卡计时只在持久化边界更新；场景运行态由 Entity 持有。
	SavedStatuses                                     []SavedStatus
	ExpBonusPct, PetExpBonusPct                       int32
	ExpBonusRemainingTicks, PetExpBonusRemainingTicks int64
}

// OwnsTitle 报告角色是否已经通过服务端玩法获得这个称号。空字符串表示卸下称号，
// 不属于“拥有的称号”。
func (c *Character) OwnsTitle(title string) bool {
	if c == nil || title == "" {
		return false
	}
	for _, owned := range c.OwnedTitles {
		if owned == title {
			return true
		}
	}
	return false
}

// UnlockTitle 幂等解锁一个称号。切片始终排序，保证存档与 0x804c 快照稳定。
func (c *Character) UnlockTitle(title string) bool {
	if c == nil || title == "" || c.OwnsTitle(title) {
		return false
	}
	c.OwnedTitles = append(c.OwnedTitles, title)
	sort.Strings(c.OwnedTitles)
	return true
}

const (
	// 0..36。参考服对照客户端属性面板确认 attrs[35]/attrs[36] 分别是
	// 物理/魔法暴击率；旧值35只分配到下标34，导致面板永远收不到暴击。
	attributeArraySlots = 37

	// DefaultPlayerMoveSpeed 是客户端角色属性数组 attrs[9] 使用的基础移速。
	// 旧私服抓包角色该位为 100；若留成 0，客户端不会按 0 移动，而是回退到
	// PlayerController.moveSpeed=400，表现为新建角色速度恰好异常地快一倍。
	DefaultPlayerMoveSpeed int32 = 100

	// DefaultNianli 是客户端角色栏的念力当前值与上限。当前玩法尚不消耗念力，
	// 但正式客户端会直接读取 attrs[25]/attrs[26]，不能把未实现误显示成 0/0。
	DefaultNianli int32 = 100
)

// NewCharacter 按建号参数创建一个新角色(应用建号规则)。
func NewCharacter(accountID int64, slot int32, name string, race Race, ap Appearance, spawn Pos) (*Character, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if !race.Valid() {
		return nil, ErrInvalidRace
	}
	// 业务有效域来自 TitleFlow.DrawCharCreate 的实际控件范围；协议物理宽度虽是 U8，
	// 但超出这些值的资源不存在，允许落库会让正常 UI 无法还原角色外观。
	if ap.Gender > 1 {
		return nil, ErrInvalidGender
	}
	if ap.Head < 1 || ap.Head > 8 {
		return nil, ErrInvalidHead
	}
	if ap.Hair < 1 || ap.Hair > 2 {
		return nil, ErrInvalidHair
	}
	now := time.Now()
	base := StartingBase(race)
	stats := DeriveFromBase(base)
	return &Character{
		AccountID:        accountID,
		Slot:             slot,
		Name:             name,
		Race:             race,
		Employed:         false,
		EmploymentKnown:  true,
		Level:            1,
		Base:             base,
		BagSlots:         DefaultBagSlots,
		Pos:              spawn,
		Appear:           ap,
		Skills:           NewBeginnerSkills(),
		SkillPointsKnown: true,
		Nianli:           DefaultNianli,
		NianliKnown:      true,
		Stamina:          DefaultStaminaRule().Max,
		CreatedAt:        now,
		LastLogin:        time.Time{}, // 尚未进入世界；客户端用空串显示“从未登陆”。
		Attrs:            initialCharacterAttrs(base, 1, stats),
	}, nil
}

// EffectiveNianli 兼容未显式构造念力字段的旧内存夹具；生产角色加载与新建
// 都会把 NianliKnown 置真，因此真实的 0 点不会被误补满。
func (c *Character) EffectiveNianli() int32 {
	if c == nil {
		return 0
	}
	if !c.NianliKnown {
		return DefaultNianli
	}
	if c.Nianli < 0 {
		return 0
	}
	if c.Nianli > DefaultNianli {
		return DefaultNianli
	}
	return c.Nianli
}

// SetNianli 写入经过上下限约束的权威念力。
func (c *Character) SetNianli(v int32) {
	if c == nil {
		return
	}
	if v < 0 {
		v = 0
	} else if v > DefaultNianli {
		v = DefaultNianli
	}
	c.Nianli, c.NianliKnown = v, true
}

func initialCharacterAttrs(base Base, level int32, stats Stats) []int32 {
	attrs := make([]int32, attributeArraySlots)
	attrs[0] = level
	attrs[1] = 0 // exp
	attrs[2] = 0 // exp to next level, 建号暂时不预计算
	attrs[3] = stats.MaxHP
	attrs[4] = stats.MaxHP
	attrs[5] = stats.MaxMP
	attrs[6] = stats.MaxMP
	// 常见 UI 指标（按已闭环客户端下标）
	attrs[7] = stats.MaxAtk // 攻击上限；攻击下限在 attrs[34]
	attrs[8] = stats.Def    // 防御
	attrs[9] = DefaultPlayerMoveSpeed
	attrs[10] = base.STR
	attrs[11] = base.VIT
	attrs[12] = base.AGI
	attrs[13] = base.INT
	attrs[14] = base.SPI
	attrs[15] = stats.MAtk
	attrs[16] = stats.MDef
	attrs[17] = stats.Hit
	attrs[21] = base.DEX
	attrs[24] = 0 // free points
	attrs[25] = DefaultNianli
	attrs[26] = DefaultNianli
	attrs[27] = DefaultStaminaRule().Max
	attrs[28] = DefaultStaminaRule().Max
	attrs[34] = stats.MinAtk
	return attrs
}

// Touch 更新登录时间。
func (c *Character) Touch() { c.LastLogin = time.Now() }

// Clone 做一份独立副本。
//
// 存在的理由只有一个: 场景要把角色交给存档队列写库, 而场景下一帧还会继续改它。
// 直接传指针的话, 写库的 goroutine 和场景 goroutine 就在同一个对象上打架 ——
// 那正是这套架构要杜绝的东西。**Attrs 必须真拷**, 浅拷贝共享底层数组等于没拷。
func (c *Character) Clone() *Character {
	if c == nil {
		return nil
	}
	cp := *c
	if c.Attrs != nil {
		cp.Attrs = append([]int32(nil), c.Attrs...)
	}
	cp.OwnedTitles = append([]string(nil), c.OwnedTitles...)
	// map 是引用, 不深拷就还是同一份
	cp.Skills = c.Skills.Clone()
	cp.Quests = c.Quests.Clone()
	cp.Pets = ClonePets(c.Pets)
	cp.SavedStatuses = CloneSavedStatuses(c.SavedStatuses)
	return &cp
}

package domain

// 物品与背包。
//
// 数据出处:
//
//	game_items       12539 件普通物品(名字/价格/等级)
//	game_equipment   9089 件装备(槽位/需求/攻防/六维加成)
//	ov_item.can_pile 能不能堆叠 —— 10629 能 / 1912 不能, **是真数据**
//
// ⚠️ **堆叠上限与背包格数在原作数据里都找不到。**
// `can_pile` 只说"能不能堆", 没说"最多堆几个"; 背包类装备(slot=10, 862 件)
// 的 `max_load` 有 666 件是 0, 看下来是外观槽不是容量。所以那两个数是服务端定的。

// ItemID 是物品/装备的配置 id。普通物品与装备**共用同一个 id 空间** ——
// 掉落表里同一列既可能指向 game_items 也可能指向 game_equipment。
type ItemID int32

// ItemInstanceKind 区分必须拥有唯一物权身份的物品。装备和实体镶嵌卡即使
// 客户端只携带模板号，服务端也始终用 UID 跟踪同一件实例。
type ItemInstanceKind uint8

const (
	ItemInstanceNone ItemInstanceKind = iota
	ItemInstanceEquipment
	ItemInstanceSocketCard
	ItemInstancePet
)

// ItemIconMapping 是 0x804d 的特殊小图标覆盖。普通物品仍由客户端按自身
// 资源加载；这里只下发 ov_item_icon_special 中明确覆盖的实例。
type ItemIconMapping struct {
	Item ItemID
	Icon int32
}

const (
	// MaxStack 是用户确认的背包单格上限；与个人仓库默认配置保持一致。
	// 客户端拆分框仍限制单次输入 99，但不影响同格累计保存 999。
	MaxStack = 999

	// BagPageSlots 是客户端每个背包页的 10×10 固定网格。
	BagPageSlots = 100
	BagTabCount  = 4

	// DefaultBagSlots 是四个独立页签的总持久化格数。
	DefaultBagSlots = BagPageSlots * BagTabCount

	// DurabilityRawPerPoint 是原始装备表耐久与客户端展示耐久的换算比。
	// ov_arm.duration=1200 的小剑在客户端显示 4 点；attack_consume 等消耗列
	// 也使用同一原始口径，不能直接从展示值扣除。
	DurabilityRawPerPoint int32 = 300
)

// EquipDef 是装备特有的部分。
type EquipDef struct {
	Slot     int32 // 1 面 / 2 头 / 3 项链 / 4 单手 / 5 盾 / 6 手套 / 7 戒指 / 8 衣 / 9 鞋 / 10 背包 / 13 双手
	Category int32 // ov_desc.category，装备大类
	Type     int32 // ov_desc.type，武器子类/原始部位枚举
	Tier     int32 // ov_arm.level，精炼配方所用装备等级（不是人物穿戴等级）
	Position int32 // ov_arm.position_，养成配方部位号，防具使用101/102/...而非UI类别
	// ResourceLevel 是 ov_arm.level 的装备资源等级，用于随机属性池；它与
	// LevelReq（人物穿戴等级）不是同一列。
	ResourceLevel int32
	// NoTypeDrop 对应 ov_arm.no_type_drop。只有 false 的模板能进入
	// “物品种类 + 等级区间”随机掉落池；显式 item_id 掉落不受它影响。
	NoTypeDrop  bool
	LevelReq    int32
	RefineLimit int32
	// RefineEffects 按精炼等级索引；启动时把原表百分比按本装备固有属性
	// 折算为固定增量，显示与战斗共用，不含随机词条、镶嵌或套装。
	RefineEffects map[int32]RefineEffect
	SocketLimit   int32
	// Durable 是客户端展示口径的满耐久。ov_arm.duration 以 300 为一个
	// 展示单位；小剑 1200→4、初行者装 5100→17 与真实 0x8006 逐项一致。
	Durable int32
	// AttackDurabilityCostRaw 是 ov_arm.attack_consume 的原始耐久消耗。
	// ov_arm.duration 以 300 为客户端展示的 1 点耐久，因此工作结算时要把
	// 这份原始消耗按 300 折算，不能直接从展示耐久上扣 18/180 点。
	AttackDurabilityCostRaw int32
	// BeHitDurabilityCostRaw/DeathDurabilityCostRaw 分别来自 ov_arm 的
	// be_hit_consume/dead_consume。前者只在装备主人实际受到攻击伤害时累计，
	// 后者在玩家死亡时对全身装备累计。
	BeHitDurabilityCostRaw int32
	DeathDurabilityCostRaw int32
	// RepairDurabilityCostRaw 是普通修理对实例耐久上限的永久损耗；特殊修理
	// 使用 SpecRepairDurabilityCostRaw。当前原服数据分别为 300 与 0。
	RepairDurabilityCostRaw     int32
	SpecRepairDurabilityCostRaw int32
	CanRepair                   bool
	CanSpecialRepair            bool
	DisappearIfZero             bool
	NoLimitDurability           bool

	// Bonus 是装备主表 ov_arm 的固有二级属性
	// (min_atk/max_atk/def/matk/mdef/hit_rate/max_weight)。
	Bonus Stats
	// Base 是装备主表 ov_arm.str/vit/int_/spi/agi/dex 的固有六维。
	// 它不是 str_need/... 穿戴门槛，也不是 game_equip_extra 附加词条。
	Base Base
	// Affixes 是词条(game_equip_extra, 17717 行)。**Stats 里的暴击率与各种抗性
	// 只从这里来** —— 没有词条的话那些字段永远是零。
	Affixes []Affix
	// Need 是穿戴门槛(等级 + 六维 + 性别 + 职业)。
	Need Requirement
	// ProfessionNames 是 ov_arm 的初行者/五职业/五飞升职业开关
	// 对应的客户端展示名。真正穿戴判定使用 Need.Professions，
	// 这份文本列表只用于 Item.desc 工具提示。
	ProfessionNames []string

	// Appearance 是普通装备对 0x800a 基础外观(ap*)的贡献。它来自 ov_desc.avatar，
	// 不是 0x8006 的 avatar 字段：原服抓包里初行者装/小剑无论在背包还是已穿戴，
	// 0x8006 avatar 都是 0，而 0x800a 已分别变成 Body=143 / WeaponR=1。
	//
	// Known=false 表示当前证据不足，重算时必须忽略，不能用物品 id 或槽号猜模型。
	Appearance EquipAppearance
	// Skill 是法宝穿戴期间临时授予的技能。关联必须同时满足
	// ov_arm.binding_skill 与 ov_skilldesc.binding_arm 双向一致；普通装备为 nil。
	Skill *EquipmentSkill
	// Suit 由 PostgreSQL 的 ov_excesuit / ov_card 外键装配，男女款和
	// 单双手款共享逻辑部位，同一部位只能贡献一件。
	Suit *EquipmentSuit
}

// EquipmentSkill 是装备绑定技能在物品界面的展示与授权元数据。
// 技能效果仍由 SkillDef/StatusDef 裁决，不能从这段提示文本直接执行。
type EquipmentSkill struct {
	ID          SkillID
	Name        string
	Description string
}

// AppearancePart 是 0x800a 六个 ap* 槽的数组下标。
// 数值顺序直接对应客户端 CharData.SetAppearance 的参数顺序。
type AppearancePart uint8

const (
	AppearanceBody AppearancePart = iota
	AppearanceCap
	AppearanceBackpack
	AppearanceWeaponR
	AppearanceWeaponL
	AppearanceFace
	appearancePartCount
)

// EquipAppearance 是一件装备已经闭合的基础模型数据。
// AttackKnown 单独存在，因为“weaponr<number> → WeaponR 模型”与武器 type →
// atkVariant 是两条独立映射；未知武器类型不能因为模型已知就顺手猜攻击动作。
type EquipAppearance struct {
	Part  AppearancePart
	Model uint16
	Known bool

	AtkVariant  uint8
	WeaponCType uint8
	AtkDist     uint16
	AttackKnown bool
}

// ItemDef 是一种物品的模板。
type ItemDef struct {
	PetCarrierSpecies  PetID
	HornTier, HornSkin uint8 // PostgreSQL game_horn_items
	GrantPetID         PetID // 来自game_item_pet_eggs；使用后获得未孵化宠物。
	ID                 ItemID
	Name               string
	Description        string // ov_desc.desc_，客户端物品提示的原始说明
	Level              int32  // 使用等级
	UseLevel           int32  // ov_item.level_need；0 表示没有使用等级门槛
	Price              int64
	// SellPrice 是背包 0x8006.sell 与 NPC 回收共同使用的单件价格。
	// 普通物品的真实样本逐项等于买入价的一半向下取整；装备实例的随机词条
	// 溢价无法由当前实例模型表达，因此这里只使用基础装备价格的一半。
	// 原表 sell_price 是售卖开关/下限，不是这笔交易的金额。
	SellPrice int64
	// Weight 是一件物品占用的负重。普通物品来自 ov_item.weight，装备来自
	// ov_arm.weight；0x8006 的当前负重是背包与身上装备逐件相加的结果。
	Weight int32
	// Icon 是 ov_desc.small_map。原始列名误导，它是客户端小图标编号，
	// 用于恢复快捷栏而不是地图编号。
	Icon int32
	// InventoryTab 是 0x8006 里客户端四个物品桶的路由号。
	// 这不是 ov_item.category 的直接拷贝：真实流量已闭合普通 category=0
	// 物品走 1、装备走 2；其余类别尚无样本时 Known=false，场景拒绝构造
	// 半真半假的背包快照。
	InventoryTab      uint8
	InventoryTabKnown bool
	// Stackable 来自 ov_item.can_pile。**是真数据**, 不要按类别猜。
	Stackable bool
	// InstanceKind 非零表示每一件都必须有独立 UID。实体镶嵌卡即使原始
	// can_pile 允许堆叠，也必须拆成独立实例，才能保证镶嵌和分离返回同一张卡。
	InstanceKind ItemInstanceKind
	// ConsumeOnUse 来自 ov_item.use_waste。普通消耗品为 true；无限次回城书
	// 等可重复使用物品为 false，不能再按“所有自用品都扣一件”处理。
	ConsumeOnUse bool
	// CanMail 来自 ov_item/ov_arm.can_mail；实例仍需同时满足未绑定、未锁定。
	CanMail bool
	// CanTrade 来自 ov_item/ov_arm.can_deal；摆摊和玩家交易还需同时检查实例未绑定、未锁定。
	CanTrade bool
	// CooldownSec/CooldownGroup 来自 ov_itemcool。CooldownGroup 是客户端的
	// item_type：同组物品共用一条冷却，使用其中任意一件都会锁住整组。
	// 秒数为 0 表示该物品没有快捷栏冷却。
	CooldownSec   int32
	CooldownGroup int32
	// Equip 非 nil 表示这是装备。
	Equip *EquipDef
	// UseStatuses 来自 ov_item_entry 的 mode=4 状态操作。只装入 self_use、
	// op_type=1、prob=100 且状态与等级都存在的确定性效果。
	UseStatuses []StatusApplication
	// UseRestore 来自 ov_item_entry 中已由描述交叉验证的即时资源恢复：
	// attr 33/35 分别是当前生命/法力，mode 0 是固定值，mode 1 是最大值百分比。
	UseRestore ItemResourceRestore
	// UseRemoveStatuses 是确定性的自用异常解除效果。效果行的 attr 87 表示
	// 删除状态，value 是要删除的状态号。
	UseRemoveStatuses []StatusID
	// UseClearStatuses 表示清除人物当前全部状态。
	UseClearStatuses bool
	// UseReturn 表示 attr=94 的回城效果，目标落点由服务端场景配置决定。
	UseReturn bool
	// UseSkillBook 是物品明确指定的职业技能号。使用后提升一级，书籍本身
	// 替代技能点消耗；职业、人物等级、前置技能和技能上限仍由技能定义裁决。
	UseSkillBook SkillID
	// UseRevive 是死亡面板可消耗的通用原地复活物品。带地图限定但尚未闭合
	// 地图关系的物品不会进入这个集合。
	UseRevive bool
	// UseCityRevive 是死亡面板选择回城时使用的保护道具。它不走普通
	// UseItem；有可用道具时消耗一件并返还本次死亡损失。
	UseCityRevive bool
	// UsePetRestore 是当前出战宠物的即时资源恢复，效果表只使用最大值百分比。
	UsePetRestore ItemResourceRestore
	// UsePetReset 区分保留前缀/天生技能的普通洗髓与回到未孵化状态的高级洗髓。
	UsePetReset PetResetMode
	// CaptureBoost 是万能捕捉绳索提供的捕捉规则覆盖。
	CaptureBoost CaptureToolBoost
	// StatReset 是人物自由属性点回退物品。
	StatReset StatResetKind
	// UseRewards 是固定礼包的全量奖励；只装入无随机概率且奖励 ID 全部闭合的配置。
	UseRewards []RewardItem
	// GachaBox 是概率礼包的开启规则，与 UseRewards 互斥（同一物品只能登记其一）。
	// 原服开箱由服务端脚本处理，脚本主体与概率未随静态数据提供，这里用结构化
	// 配置替代：每条奖励独立判定概率，可同时命中多条，也可能一条不中。
	GachaBox *GachaBoxDef
	// UseCaiyu 是确定性的彩玉兑换数量，使用后直接进入用户共用的彩玉余额。
	UseCaiyu int64
	// UseCopper 是确定性金币/银币道具的总铜币面额，与彩玉互斥。
	UseCopper int64
	// UseDisabledReason 来自 PostgreSQL 全量使用台账，只在物品声明可使用
	// 但尚无已闭合服务端入口时设置。
	UseDisabledReason string
	// UseTitle 是称号石解锁的固定称号名；“自定义称号”是允许任意称号文本的权限标记。
	UseTitle string
	// ExperienceBoost 是脚本型经验卡的明确倍率和持续时间。
	ExperienceBoost ItemExperienceBoost
	// PlayerExperience 按使用前的角色等级查取单颗经验；缺失等级不可消耗。
	PlayerExperience map[int32]int64
	// AvatarFusion 是 0x1036 可融合到真实装备实例上的换形定义。
	AvatarFusion *AvatarFusionDef
	// EquipmentSoul 是 0x1036 kind=1 的装备灵/元素定义。它与外观换形是
	// 两个独立实例层：客户端 EquippedItem 也分别携带 avatar/soulAvatar。
	EquipmentSoul *EquipmentSoulDef
	// DragonFusion 是0x1036 kind=2的龙系列，独立于变装和部位灵。
	DragonFusion *EquipmentSoulDef
	// PetTransmog 是宠物幻化书的临时显示模型。
	PetTransmog *PetTransmogDef
	// PetExperience 是按宠物当前等级发放固定经验的消耗品。
	PetExperience *PetExperienceItem
	// PetAffectionPP 来自宠爱PP果成功率分档表。
	PetAffectionPP *PetPPItem
	// PetRewards 是直接发入宠物栏的固定前缀奖池，权重来自数据库。
	PetRewards []PetItemReward
	// Saddle 来自 pets.json 的 saddles 经审核入库；不依赖宠物坐骑技能。
	Saddle *SaddleDef
	// CardDefaults 只在新卡生成时复制到实例，不参与已拥有物品的属性计算。
	CardDefaults CardInstanceState
	// SocketAffixes 保留模板效果用于兼容非实体卡的配置读取。
	SocketAffixes []Affix
}

type PetExperienceItem struct {
	MaxAboveOwner int32
	ByLevel       map[int32]int64
}

type PetPPBand struct {
	Low, High, SuccessPct int32
}

type PetItemReward struct {
	Pet    PetID
	Prefix int32
	Weight int32
}

// GachaBoxDef 是概率礼包的开启规则。
type GachaBoxDef struct {
	// KeyItem 是单次打开消耗的钥匙物品；0 表示无需钥匙。
	KeyItem ItemID
	// KeyQuantity 是单次打开消耗的钥匙数量。
	KeyQuantity int32
	// MinFreeSlots 是开箱前背包至少剩余的格数（前置限制）；
	// 0 表示不设门槛，仍由开箱时的全量奖励预演兜底（放不下则整体不生效）。
	MinFreeSlots int32
	// Rewards 是独立判定的奖励行。每条按 ChanceBP 掷，可同时命中多条。
	Rewards []GachaBoxReward
}

// GachaBoxReward 是概率礼包的一条奖励。命中后数量在 [MinQty, MaxQty] 均匀随机。
type GachaBoxReward struct {
	Item     ItemID
	MinQty   int32
	MaxQty   int32
	ChanceBP int32 // 万分比概率，1..10000；10000 表示必掉
}

type PetPPItem struct {
	Limit int32
	Bands []PetPPBand
}

func (d PetPPItem) Chance(used int32) (int32, bool) {
	for _, band := range d.Bands {
		if used >= band.Low && used <= band.High {
			return band.SuccessPct, true
		}
	}
	return 0, false
}

func (d PetExperienceItem) Experience(level int32) int64 {
	return d.ByLevel[level]
}

type SaddleDef struct {
	UsePetMaxSpeed   bool
	SpeedBonusBP     int32
	Passengers       int32
	CombinationModel int32
	DistinctPets     bool
	Pets             []PetID
}

func (d SaddleDef) Allows(id PetID) bool {
	for _, allowed := range d.Pets {
		if allowed == id {
			return true
		}
	}
	return false
}

// ItemResourceRestore 是普通自用药品一次使用的即时恢复量。四项可以组合，
// 例如天山雪莲同时恢复 50% 最大生命和 50% 最大法力。
type ItemResourceRestore struct {
	HPFlat int32
	MPFlat int32
	HPPct  int32
	MPPct  int32
}

type PetResetMode uint8

const (
	PetResetNone PetResetMode = iota
	PetResetKeepInnate
	PetResetRehatch
)

type CaptureToolBoost struct {
	Universal      bool
	SuccessRatePct int32
	PrefixRatePct  int32
}

type StatResetKind uint8

const (
	StatResetNone StatResetKind = iota
	StatResetSTR
	StatResetVIT
	StatResetINT
	StatResetSPI
	StatResetAGI
	StatResetDEX
	StatResetAll
)

type ItemExperienceBoost struct {
	PlayerBonusPct int32
	PetBonusPct    int32
	DurationSec    int32
}

type AvatarFusionDef struct {
	Appearance EquipAppearance
	Affixes    []Affix
}

type EquipmentSoulDef struct {
	TargetSlot EquipSlot
	Affixes    []Affix
}

// AcceptsSlot 把客户端单手武器槽定义同时应用到双手武器槽；其余灵必须精确
// 命中装备部位，不能把戒灵镶到项链或衣服上。
func (d *EquipmentSoulDef) AcceptsSlot(slot EquipSlot) bool {
	if d == nil {
		return false
	}
	if d.TargetSlot == SlotWeapon {
		return slot == SlotWeapon || slot == SlotTwoHand
	}
	return slot == d.TargetSlot
}

type PetTransmogDef struct {
	TargetMonster    MonsterID
	TargetName       string
	Model            int32
	DurationSec      int32
	RequireUnmounted bool
}

func (d PetTransmogDef) Valid() bool {
	return d.TargetMonster > 0 && d.TargetName != "" && d.Model > 0 &&
		d.DurationSec > 0 && d.DurationSec <= 24*60*60
}

func (b ItemExperienceBoost) Valid() bool {
	return b.DurationSec > 0 && b.DurationSec <= 24*60*60 &&
		b.PlayerBonusPct >= 0 && b.PlayerBonusPct <= 900 &&
		b.PetBonusPct >= 0 && b.PetBonusPct <= 900 &&
		(b.PlayerBonusPct > 0 || b.PetBonusPct > 0)
}

func (b CaptureToolBoost) Valid() bool {
	return b.Universal && b.SuccessRatePct >= 100 && b.SuccessRatePct <= 1000 &&
		b.PrefixRatePct >= 100 && b.PrefixRatePct <= 1000
}

func (r ItemResourceRestore) Valid() bool {
	return r.HPFlat >= 0 && r.MPFlat >= 0 && r.HPPct >= 0 && r.HPPct <= 100 &&
		r.MPPct >= 0 && r.MPPct <= 100 &&
		(r.HPFlat > 0 || r.MPFlat > 0 || r.HPPct > 0 || r.MPPct > 0)
}

func (r ItemResourceRestore) HPAmount(max int32) int32 {
	return itemRestoreAmount(max, r.HPFlat, r.HPPct)
}

func (r ItemResourceRestore) MPAmount(max int32) int32 {
	return itemRestoreAmount(max, r.MPFlat, r.MPPct)
}

func itemRestoreAmount(max, flat, pct int32) int32 {
	n := int64(flat) + int64(max)*int64(pct)/100
	if n <= 0 {
		return 0
	}
	if n > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(n)
}

// IsEquip 报告这是不是装备。
func (d ItemDef) IsEquip() bool { return d.Equip != nil }

// Stack 是背包里的一格。Item == 0 表示空格。
type Stack struct {
	// UID 是**这一件东西**的唯一标识, 与 ItemID(哪一种)不是一回事。
	//
	// 现在只有装备用得上(每件的耐久不同), 但交易、仓库、摆摊都要靠它来说
	// "我给你的是这一件"。留到那时候再加就得改存档表, 所以现在就带上。
	UID   int64
	Item  ItemID
	Count int32
	// InstanceKind 随 UID 一起从统一实例表恢复。它不由客户端提交，也不靠
	// “有没有耐久”反推，避免卡片实例被当成普通堆叠物复制。
	InstanceKind ItemInstanceKind
	// Durability/MaxDurability 是客户端展示口径的当前值和实例上限。
	// MaxDurability=0 兼容旧内存夹具，由 MaxDurabilityOf 回退到模板满耐久；
	// 生产实例创建和数据库回填后都会显式保存实例上限。
	Durability    int32
	MaxDurability int32
	// DurabilityWearRaw 保存尚不足一个展示点的累计损耗（0..299）。
	// 将余数落盘可让 attack_consume=1 精确地每 300 次扣 1 点，且重登不能
	// 清空进度；同一实例的攻击、受击和死亡损耗共享这份累计值。
	DurabilityWearRaw int32
	// Bound 是这一件物品的永久绑定状态，不是物品模板属性。
	// 真实 0x8006 在背包项中显式下发该字段；未绑定普通装备
	// 通过 0x1007 穿上后，原服装备描述会变为“（绑定）”。
	Bound bool
	// Locked 是玩家主动设置的实例保护。它与 Bound 不同：绑定限制流通，锁定
	// 阻止消耗、出售、丢弃、交易和装备养成，直到用账号安全码解除。
	Locked bool
	// RefineLevel 与 Sockets 是装备实例养成状态；模板只提供上限和配方类别。
	RefineLevel int32
	// SocketCount 是实际已打出的孔数；Sockets 中非零项才是已使用的孔。
	SocketCount uint8
	Sockets     [5]ItemID
	// SocketUIDs 保存实际镶入的卡片实例。Sockets 继续保留模板号用于现有
	// 客户端协议展示；两者必须成对移动、分离，禁止按模板重新造卡。
	SocketUIDs [5]int64
	// Card 与 SocketCards 保存实例属性快照；位置移动不能重新读取模板。
	Card        CardInstanceState
	SocketCards [5]CardInstanceState
	// RolledAffixes 是普通类型掉落在创建装备实例时抽出的属性。CardID 保留
	// 属性池来源，Affix 是已经抽定、此后不再重掷的实际数值。
	RolledAffixCount uint8
	RolledAffixes    [5]InstanceAffix
	WashQuality      uint8
	WashCount        uint8
	WashAffixes      [4]Affix
	// FusedAppearance 是已经消耗并绑定到本装备实例的换形物品 ID。
	FusedAppearance ItemID
	// FusedSoul 是 0x1036 kind=1 已镶入的装备灵/元素物品 ID。它独立参与
	// 属性与 soulAvatar 展示，换形外观不能覆盖它，反之亦然。
	FusedSoul   ItemID
	FusedDragon ItemID
}

// CardEffect 保留卡片的一条原始效果，包含条件效果的类型与概率。
// 属性在卡片生成时确定；镶嵌只是移动实例，不重新执行生成规则。
type CardEffect struct {
	Op          int32
	Attr        int32
	Mode        int32
	Probability int32
	Value       int32
}

// CardInstanceState 使用值类型，背包克隆、交易快照和孔位移动不会共享可写属性。
type CardInstanceState struct {
	Initialized bool
	Count       uint8
	Effects     [8]CardEffect
	Bound       bool
	Locked      bool
}

// StaticAffixes 返回现有装备属性计算器能够处理的无条件属性。
// 其余原始效果仍保存在实例中，不能当作普通属性累加。
func (c CardInstanceState) StaticAffixes() []Affix {
	out := make([]Affix, 0, int(c.Count))
	for i := 0; i < int(c.Count) && i < len(c.Effects); i++ {
		e := c.Effects[i]
		if e.Op == 1 && e.Probability == 100 {
			out = append(out, Affix{Attr: e.Attr, Mode: e.Mode, Value: e.Value})
		}
	}
	return out
}

// InstanceAffix 是装备实例的一条随机属性及其内部卡定义来源。
type InstanceAffix struct {
	CardID int32
	Affix  Affix
}

// Empty 报告这一格是不是空的。
func (s Stack) Empty() bool { return s.Item == 0 || s.Count <= 0 }

// NewStack 从模板创建一个全新的物品实例。已有物权在位置间移动必须使用
// AddStack，不能再次调用本函数，否则会得到新的 UID。
func NewStack(def ItemDef, count int32) Stack {
	if def.ID == 0 || count <= 0 {
		return Stack{}
	}
	if !def.Stackable && count != 1 {
		return Stack{}
	}
	full := durabilityOf(def)
	st := Stack{Item: def.ID, Count: count, Durability: full, MaxDurability: full,
		InstanceKind: def.InstanceKind}
	if def.InstanceKind != ItemInstanceNone {
		st.UID = NewItemInstanceID()
	}
	if def.InstanceKind == ItemInstanceSocketCard {
		st.Card = def.CardDefaults
	}
	return st
}

// IsMiningTool 报告这件物品是否为客户端特判的采矿工具。
//
// GameAssembly.dylib+0x23c92c (NetClient.IsMiningTool) 的完整函数体只比较
// 0x76d 与 0x3aba。客户端对这两件物品跳过“装备后将绑定”
// 确认框，服务端也必须保留它们的未绑定状态。
func IsMiningTool(id ItemID) bool { return id == 0x76d || id == 0x3aba }

// Bag 是一个角色的背包。
//
// 格子是**定长数组**而不是"有多少放多少"的列表 —— 客户端按格号显示,
// 位置本身是玩家能感知的状态(他把药放在第 3 格就该一直在第 3 格)。
type Bag struct {
	slots []Stack
}

// ClientBagSlot converts a client tab-local position into the persisted slot.
func ClientBagSlot(tab uint8, slot int) (int, bool) {
	if tab >= BagTabCount || slot < 0 || slot >= BagPageSlots {
		return 0, false
	}
	return int(tab)*BagPageSlots + slot, true
}

func bagRange(def ItemDef, total int) (start, end int) {
	// Small Bags are retained for isolated tests and legacy temporary states.
	// A real character bag is always four complete pages.
	if total < DefaultBagSlots || !def.InventoryTabKnown {
		return 0, total
	}
	start, ok := ClientBagSlot(def.InventoryTab, 0)
	if !ok {
		return 0, 0
	}
	return start, start + BagPageSlots
}

// ClientBagPosition is the page-local wire coordinate; negative sentinel values stay negative.
func ClientBagPosition(slot int32) int32 {
	if slot < 0 {
		return slot
	}
	return slot % BagPageSlots
}

func (b *Bag) RangeFor(def ItemDef) (int, int) { return bagRange(def, b.Cap()) }
func (b *Bag) FirstEmptyFor(def ItemDef) int {
	first, last := b.RangeFor(def)
	for i := first; i < last; i++ {
		if b.At(i).Empty() {
			return i
		}
	}
	return -1
}
func (b *Bag) AllowsSlot(def ItemDef, slot int) bool {
	first, last := b.RangeFor(def)
	return slot >= first && slot < last
}

// NewBag 建一个 n 格的空背包。n <= 0 用默认值。
func NewBag(n int) *Bag {
	if n <= 0 {
		n = DefaultBagSlots
	}
	return &Bag{slots: make([]Stack, n)}
}

// Clone 做一份独立背包。多阶段任务用它预演“扣任务物品 + 发中间物品/奖励”，
// 全部能成功后才替换权威背包，避免半扣半发。
func (b *Bag) Clone() *Bag {
	if b == nil {
		return nil
	}
	return &Bag{slots: append([]Stack(nil), b.slots...)}
}

// Cap 返回格数。
func (b *Bag) Cap() int {
	if b == nil {
		return 0
	}
	return len(b.slots)
}

// At 返回第 i 格。越界返回空格。
func (b *Bag) At(i int) Stack {
	if b == nil || i < 0 || i >= len(b.slots) {
		return Stack{}
	}
	return b.slots[i]
}

// Set 直接写第 i 格。**给存档加载用**, 业务逻辑请走 Add/RemoveAt。
func (b *Bag) Set(i int, s Stack) bool {
	if b == nil || i < 0 || i >= len(b.slots) {
		return false
	}
	b.slots[i] = s
	return true
}

// Each 遍历非空格。
func (b *Bag) Each(fn func(slot int, s Stack)) {
	if b == nil {
		return
	}
	for i, s := range b.slots {
		if !s.Empty() {
			fn(i, s)
		}
	}
}

// FreeSlots 返回还有几个空格。
func (b *Bag) FreeSlots() int {
	if b == nil {
		return 0
	}
	n := 0
	for _, s := range b.slots {
		if s.Empty() {
			n++
		}
	}
	return n
}

// CountOf 数某种物品一共有多少个。
func (b *Bag) CountOf(id ItemID) int32 {
	if b == nil {
		return 0
	}
	var n int32
	for _, s := range b.slots {
		if s.Item == id {
			n += s.Count
		}
	}
	return n
}

// Add 往背包里放东西, 返回**没放下的数量**(0 表示全放下了)。
//
// 顺序: 先往已有的同种堆里塞满, 再占空格。这样玩家捡药不会把背包塞成一格一瓶。
//
// 部分成功是**有意的**: 捡起一堆 50 个而只放得下 30 个时, 放下 30 个比
// 一个都不放更符合直觉。调用方拿返回值决定剩下的怎么办(留在地上/退回)。
func (b *Bag) Add(def ItemDef, count int32) int32 {
	if b == nil || count <= 0 || def.ID == 0 {
		return count
	}
	start, end := bagRange(def, len(b.slots))
	if start >= end {
		return count
	}
	if !def.Stackable {
		// 不可堆叠的东西一件占一格, 每件都是独立实例
		for i := start; i < end; i++ {
			if count == 0 {
				break
			}
			if b.slots[i].Empty() {
				b.slots[i] = NewStack(def, 1)
				count--
			}
		}
		return count
	}

	// 1. 填已有的堆
	for i := start; i < end; i++ {
		if count == 0 {
			return 0
		}
		s := &b.slots[i]
		if s.Item != def.ID || s.Locked || s.Count >= MaxStack {
			continue
		}
		room := MaxStack - s.Count
		if room > count {
			room = count
		}
		s.Count += room
		count -= room
	}
	// 2. 开新格
	for i := start; i < end; i++ {
		if count == 0 {
			return 0
		}
		if !b.slots[i].Empty() {
			continue
		}
		put := count
		if put > MaxStack {
			put = MaxStack
		}
		b.slots[i] = Stack{Item: def.ID, Count: put}
		count -= put
	}
	return count
}

// AddStack 把一份已有实例放回背包，返回未放下的数量。
//
// 与 Add 的区别是它保留 UID、耐久和绑定状态，供玩家丢弃后重新拾取、仓库和
// 交易使用。实例字段不同的堆绝不能合并；不可堆叠实例也不能为了腾位置复制 UID。
func (b *Bag) AddStack(def ItemDef, incoming Stack) int32 {
	if b == nil || incoming.Empty() || incoming.Item != def.ID {
		return incoming.Count
	}
	start, end := bagRange(def, len(b.slots))
	if start >= end {
		return incoming.Count
	}
	if !def.Stackable || incoming.UID != 0 || incoming.Durability != 0 ||
		incoming.MaxDurability != 0 || incoming.DurabilityWearRaw != 0 || incoming.FusedAppearance != 0 {
		if incoming.Count != 1 {
			return incoming.Count
		}
		for i := start; i < end; i++ {
			if b.slots[i].Empty() {
				b.slots[i] = incoming
				return 0
			}
		}
		return incoming.Count
	}

	left := incoming.Count
	for i := start; i < end; i++ {
		if left == 0 {
			return 0
		}
		dst := &b.slots[i]
		if dst.Item != incoming.Item || dst.UID != 0 || dst.Bound != incoming.Bound ||
			dst.Locked != incoming.Locked ||
			dst.Durability != 0 || dst.MaxDurability != 0 || dst.DurabilityWearRaw != 0 ||
			dst.FusedAppearance != incoming.FusedAppearance ||
			dst.Count >= MaxStack {
			continue
		}
		put := MaxStack - dst.Count
		if put > left {
			put = left
		}
		dst.Count += put
		left -= put
	}
	for i := start; i < end; i++ {
		if left == 0 {
			return 0
		}
		if !b.slots[i].Empty() {
			continue
		}
		put := left
		if put > MaxStack {
			put = MaxStack
		}
		next := incoming
		next.Count = put
		b.slots[i] = next
		left -= put
	}
	return left
}

// RemoveAt 从某一格拿走 n 个。拿不够就一个都不拿 ——
// 半成功的扣除会让"扣物品→给奖励"这类流程出现只扣了一半的中间态。
func (b *Bag) RemoveAt(slot int, n int32) bool {
	if b == nil || slot < 0 || slot >= len(b.slots) || n <= 0 {
		return false
	}
	s := &b.slots[slot]
	if s.Empty() || s.Locked || s.Count < n {
		return false
	}
	s.Count -= n
	if s.Count == 0 {
		*s = Stack{}
	}
	return true
}

// Remove 按物品 id 扣掉 n 个, 可以跨格。不够则一个都不扣。
func (b *Bag) Remove(id ItemID, n int32) bool {
	if b == nil || n <= 0 || b.UsableCountOf(id) < n {
		return false
	}
	for i := range b.slots {
		if n == 0 {
			break
		}
		s := &b.slots[i]
		if s.Item != id || s.Locked {
			continue
		}
		take := s.Count
		if take > n {
			take = n
		}
		s.Count -= take
		n -= take
		if s.Count == 0 {
			*s = Stack{}
		}
	}
	return true
}

// UsableCountOf 返回未锁定、可被业务消耗的数量。CountOf 仍表示玩家实际持有
// 的总数，供任务展示等只读语义使用；真正扣除必须以本方法的口径为准。
func (b *Bag) UsableCountOf(id ItemID) int32 {
	if b == nil {
		return 0
	}
	var n int32
	for _, s := range b.slots {
		if s.Item == id && !s.Locked {
			n += s.Count
		}
	}
	return n
}

// Move 把 from 格挪到 to 格。同种可堆叠的合并, 否则交换。
func (b *Bag) Move(from, to int) bool {
	if b == nil || from == to {
		return false
	}
	if from < 0 || from >= len(b.slots) || to < 0 || to >= len(b.slots) {
		return false
	}
	src, dst := &b.slots[from], &b.slots[to]
	if src.Empty() {
		return false
	}
	// 同种且目标没满: 合并
	if !dst.Empty() && dst.Item == src.Item && dst.Count < MaxStack &&
		src.UID == 0 && dst.UID == 0 && src.Bound == dst.Bound && src.Locked == dst.Locked {
		room := MaxStack - dst.Count
		if room > src.Count {
			room = src.Count
		}
		dst.Count += room
		src.Count -= room
		if src.Count == 0 {
			*src = Stack{}
		}
		return true
	}
	*src, *dst = *dst, *src
	return true
}

func durabilityOf(def ItemDef) int32 {
	if def.Equip == nil {
		return 0
	}
	return def.Equip.Durable
}

// MaxDurabilityOf 返回装备实例当前的耐久上限。旧实例没有独立上限时只在读取
// 视图时回退到模板值；一旦发生磨损或修理，调用方应把返回值写回实例。
func MaxDurabilityOf(st Stack, def ItemDef) int32 {
	if def.Equip == nil || def.Equip.Durable <= 0 {
		return 0
	}
	if st.MaxDurability > 0 && st.MaxDurability <= def.Equip.Durable {
		return st.MaxDurability
	}
	return def.Equip.Durable
}

// EquipmentFunctional 报告装备当前是否应贡献属性与外观。没有耐久概念或明确
// 标记为无限耐久的装备始终有效；其余装备在当前耐久归零后留在槽位，但失去效果。
func EquipmentFunctional(st Stack, def ItemDef) bool {
	if def.Equip == nil {
		return false
	}
	if def.Equip.Durable <= 0 || def.Equip.NoLimitDurability {
		return true
	}
	// 旧测试/内存构造只填模板而不填实例耐久；生产实例始终有显式上限，
	// 因此 MaxDurability=0 可安全地作为“尚未接入实例耐久”的兼容哨兵。
	if st.MaxDurability == 0 && st.Durability == 0 {
		return true
	}
	return st.Durability > 0
}

// ConsumeDurabilityRaw 在一个实例上累计原始耐久损耗。
// changed 包含“只改变余数、展示值未变”的情况，供持久化标脏；displayChanged
// 只在客户端可见耐久变化时为真；broken 表示本次刚好从可用变为 0。
func ConsumeDurabilityRaw(st *Stack, def ItemDef, raw int32) (changed, displayChanged, broken bool) {
	if st == nil || st.Empty() || def.Equip == nil || raw <= 0 ||
		def.Equip.Durable <= 0 || def.Equip.NoLimitDurability || st.Durability <= 0 {
		return false, false, false
	}
	maxDurability := MaxDurabilityOf(*st, def)
	if maxDurability <= 0 {
		return false, false, false
	}
	st.MaxDurability = maxDurability
	if st.Durability > maxDurability {
		st.Durability = maxDurability
		displayChanged = true
	}
	old := st.Durability
	total := int64(st.DurabilityWearRaw) + int64(raw)
	loss := total / int64(DurabilityRawPerPoint)
	st.DurabilityWearRaw = int32(total % int64(DurabilityRawPerPoint))
	if loss >= int64(st.Durability) {
		st.Durability = 0
		st.DurabilityWearRaw = 0
	} else if loss > 0 {
		st.Durability -= int32(loss)
	}
	displayChanged = displayChanged || st.Durability != old
	return true, displayChanged, old > 0 && st.Durability == 0
}

// Snapshot 是一个角色要落盘的全部东西。
//
// 角色行与背包必须**一起存** —— 分开存就会出现"经验存了、物品没存"这种
// 崩溃后的半截状态, 而那正是玩家会拿去申诉的那种。
type Snapshot struct {
	Char      *Character
	Bag       *Bag
	Worn      *EquipSet
	ChangeSet *ChangeSet
	Warehouse *Warehouse
	Wardrobe  *Wardrobe
	Stall     *Stall
}

// Clone 做一份独立副本, 交给写回队列。
func (s Snapshot) Clone() Snapshot {
	out := Snapshot{Char: s.Char.Clone(), Worn: s.Worn.Clone(), Warehouse: s.Warehouse.Clone(),
		ChangeSet: s.ChangeSet.Clone(), Wardrobe: s.Wardrobe.Clone(), Stall: s.Stall.Clone()}
	if s.Bag != nil {
		out.Bag = s.Bag.Clone()
	}
	return out
}

// DropEntry 是掉落表里的一条。
//
// RatePct 是**百分比, 每条独立掷**, 不是瓜分 100%。
// 实测单怪合计能到 3200%, 也存在 rate=100 的必掉项 —— 按"瓜分"理解会全错。
type DropEntry struct {
	// 没有 Choices 时 Item 是固定掉落；有 Choices 时 Item 保存池内一个有效的
	// 代表 id，真正产出仍从 Choices 随机选。候选池在加载时按区间复用，开服后只读。
	Item    ItemID
	Choices []ItemID
	RatePct float64
	// RollAffixes 只对“物品种类 + 装备等级范围”的普通装备候选为 true。
	// 显式 item_id 与 BOSS 固定装备保持模板固有属性，不额外随机。
	RollAffixes bool
}

// Roll 掷这一条掉不掉。r 给 [0,10000) 的整数(万分比精度) ——
// 掉率最低到 0.00045%, 用百分比整数会全被抹成 0。
func (d DropEntry) Roll(r func(n int) int) bool {
	if d.RatePct <= 0 {
		return false
	}
	if d.RatePct >= 100 {
		return true
	}
	const scale = 1_000_000 // 百万分之一, 覆盖到 0.00045% 这个量级
	return int64(r(scale)) < int64(d.RatePct*scale/100)
}

// RollWithMult 掷这一条掉不掉：RatePct 先乘等级差惩罚倍率（万分数）再判定。
//
// multBP=10000 等价于 Roll；multBP≤0 直接不掉。必掉项（RatePct≥100）在
// 惩罚下同样可能跌破必掉线 —— 这正是"越级打怪掉率递减"的意图。
func (d DropEntry) RollWithMult(r func(n int) int, multBP int32) bool {
	if multBP <= 0 || d.RatePct <= 0 {
		return false
	}
	if multBP >= 10000 {
		return d.Roll(r)
	}
	rate := d.RatePct * float64(multBP) / 10000
	if rate <= 0 {
		return false
	}
	if rate >= 100 {
		return true
	}
	const scale = 1_000_000
	return int64(r(scale)) < int64(rate*scale/100)
}

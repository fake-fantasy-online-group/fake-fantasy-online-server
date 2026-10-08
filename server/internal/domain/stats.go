package domain

// 属性体系 —— 六维基础属性 → 二级属性。
// 全部系数与出处见 docs/属性体系.md, 数值改动必须先改文档再改这里。

// Base 是六维基础属性。加点、装备、状态最终都落到这六个数上。
//
// 对应 ov_levelup 的列名 str/vit/int/spi/agi/dex; luk 列 15 行全 0, 本作未使用。
type Base struct {
	STR int32 // 力量
	VIT int32 // 体质
	INT int32 // 智慧
	SPI int32 // 精神
	AGI int32 // 敏捷
	DEX int32 // 灵巧
}

func (b Base) Add(o Base) Base {
	return Base{b.STR + o.STR, b.VIT + o.VIT, b.INT + o.INT, b.SPI + o.SPI, b.AGI + o.AGI, b.DEX + o.DEX}
}

// 角色基础属性换算系数。
//
// ⚠️ 强弱不同, 用的时候要知道自己站在什么地基上:
//
//	×9 两条(体质→生命、精神→法力)是**双来源**: 逗游文章 + ov_levelup 初始值反推,
//	   init_hp/init_vit = 9.000 十个非零行全中。这两条硬。
//	÷2 五条参考逗游公开资料；现有角色样本不能验证该比例，因为样本服调整过属性数值。
const (
	HPPerVIT        = 9   // 1 体质 = 9 生命
	MPPerSPI        = 9   // 1 精神 = 9 法力
	BaseCarryWeight = 500 // 真实新战士样本: STR=12, 面板负重上限=620
	WeightPerSTR    = 10  // 1 力量 = 10 负重

	perTwoSTR = 2 // 2 力量 = 1 攻击
	perTwoAGI = 2 // 2 敏捷 = 1 防御
	perTwoDEX = 2 // 2 灵巧 = 1 命中
	perTwoINT = 2 // 2 智慧 = 1 魔攻
	perTwoSPI = 2 // 2 精神 = 1 魔防
)

// Stats 是参与战斗结算的二级属性 —— 也就是 0x8007 角色面板属性数组里
// 服务端真正会用到的部分。
//
// 每一项都是"基础属性推导 + 装备加成 + 状态加成"的**最终值**。
// 中间过程(哪件装备加了多少)不在这里, 那是 item 层的事。
type Stats struct {
	MaxHP, MaxMP int32
	HPRegen      int32 // 每次回复跳动的量
	MPRegen      int32

	MinAtk, MaxAtk int32 // 物理攻击下限/上限, 伤害在这个区间均匀取
	Def            int32 // **本作的防御走闪避, 不减伤**(docs/战斗公式.md)
	Hit            int32 // 命中
	MAtk, MDef     int32 // 魔攻 / 魔防

	CritRate  int32 // 物理爆击率, 万分比(ov_card_entry attr_id=23 的口径)
	MCritRate int32 // 魔法爆击率, 万分比(attr_id=66)

	AtkSpeedPct int32 // 客户端属性33的加速百分点，负间隔百分比词条取反累计
	AtkSpeedMS  int32 // 攻击间隔毫秒。基准 1500, 装备上的值为负表示提速
	MoveSpeed   int32 // 移速

	PhysResist   int32 // 伤害抗性: 命中后直接从伤害里减掉的定值(attr_id=69)
	MagicResist  int32 // 魔法抗性(attr_id=70)
	StatusResist int32 // 不良状态抗性(attr_id=118)

	MaxWeight int32
}

// DeriveFromBase 由六维推出二级属性的**基础部分**。
// 装备与状态加成由调用方在此之上叠加 —— 本函数是纯函数, 不认识装备。
func DeriveFromBase(b Base) Stats {
	return Stats{
		MaxHP:     b.VIT * HPPerVIT,
		MaxMP:     b.SPI * MPPerSPI,
		MinAtk:    b.STR / perTwoSTR,
		MaxAtk:    b.STR / perTwoSTR,
		Def:       b.AGI / perTwoAGI,
		Hit:       b.DEX / perTwoDEX,
		MAtk:      b.INT / perTwoINT,
		MDef:      b.SPI / perTwoSPI,
		MaxWeight: BaseCarryWeight + b.STR*WeightPerSTR,
	}
}

// AttackIntervalTicks 把攻击速度换成帧数。基准 1500ms = 15 帧。
// 下限 1 帧: 再快也不能一帧连打, 那会让伤害数值失控。
func (s Stats) AttackIntervalTicks() Tick {
	ms := s.AtkSpeedMS
	if ms <= 0 {
		ms = 1500 // ov_arm.attack_speed 的基准值
	}
	if t := Ticks(int(ms)); t > 0 {
		return t
	}
	return 1
}

func (s Stats) PlayerAttackSpeedPercent() int32 {
	speed := int64(100) + int64(s.AtkSpeedPct)
	if speed < 50 {
		return 50
	}
	if speed > 2147483647 {
		return 2147483647
	}
	return int32(speed)
}

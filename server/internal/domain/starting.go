package domain

import "fmt"

// 一级初始六维。
//
// 数据来自 `gamedata.ov_levelup` 里 `prof=0` 的五行模板（客户端权威数据），
// 逐行抄进来的快照。为什么敢抄死：这五行是 2005 年成品客户端的 .bin，永远不会变。
//
// ⚠️ **只有初始值抄在这里，每级成长没有。**
// 成长在 `ov_levelup.{attr}_odd` / `_even`（按等级奇偶取值，不能取平均 ——
// 奇偶交替就是用来做出 1.5 这种小数成长的）。那部分要读表，等 gamedata 加载器接进来。
//
// 校验不变量：**十项全中** `体质×9 = init_hp`、`精神×9 = init_sp` ——
// 这五行本身就是 HPPerVIT / MPPerSPI 两个系数的第二个独立来源。starting_test.go 逐行钉住。

// startingBase 按职业给出一级六维。索引即 Race。
//
// 职业与 `ov_levelup.type` 的对应是 **type = Race + 1**，五个一一对应。
// **[实证]** `ov_skilldesc.prof` 直接给出答案：prof=4 是治愈术/复法术（hurt_type 12 圣）
// 所以是药师；prof=5 是火弹术/火球术（hurt_type 8 火 / 9 冰）所以是术士。
// docs/属性体系.md 此前把这两个写反了，已更正。
var startingBase = [...]Base{
	Warrior:   {STR: 12, VIT: 24, INT: 4, SPI: 4, AGI: 4, DEX: 4},  // type 1，HP 216 / SP 36
	Swordsman: {STR: 8, VIT: 20, INT: 6, SPI: 6, AGI: 8, DEX: 6},   // type 2，HP 180 / SP 54
	Assassin:  {STR: 4, VIT: 16, INT: 6, SPI: 4, AGI: 12, DEX: 12}, // type 3，HP 144 / SP 36
	Healer:    {STR: 6, VIT: 16, INT: 6, SPI: 12, AGI: 4, DEX: 4},  // type 4，HP 144 / SP 108
	Warlock:   {STR: 6, VIT: 12, INT: 12, SPI: 16, AGI: 4, DEX: 4}, // type 5，HP 108 / SP 144
}

// FreePointsPerLevel 每级可自由分配的点数。`ov_levelup.point_add = 2`，十行全中。
const FreePointsPerLevel = 2

// StartingBase 返回某职业一级时的六维。职业非法时按战士处理 ——
// 建号流程会先校验，走到这里说明数据坏了，给个能玩的角色好过给个 0 血的。
func StartingBase(r Race) Base {
	if !r.Valid() {
		return startingBase[Warrior]
	}
	return startingBase[r]
}

// 新角色的两件出生装备。
//
// **[真实流量]** 原服新建的男性角色（0x1003 gender=1）第一次进图时，
// 0x8006 的第 0/1 格分别是 2875 初行者装（男）与 1011 小剑；重量
// 25+12=37、耐久 17/4 也与 ov_arm 精确闭合。2876 是同表紧邻的
// “初行者装（女）”，客户端角色性别线值为 0/1，因此按性别只替换衣服。
const (
	startingSword      ItemID = 1011
	startingBodyMale   ItemID = 2875
	startingBodyFemale ItemID = 2876
)

// IsStartingSword / IsStartingBody 只识别原服建角事务发放的两类出生装备。
// 会话层用它们核对已经成功构造的背包快照，避免仅凭物品名称或角色等级推断
// “初行者装备已经到账”。
func IsStartingSword(id ItemID) bool { return id == startingSword }

func IsStartingBody(id ItemID) bool {
	return id == startingBodyMale || id == startingBodyFemale
}

// NewStartingBag 按原服第一次进图的格序构造出生背包。
//
// defs 必须来自已加载的客户端物品表。缺一条定义就拒绝建角，不能落下一只
// “角色有了、出生物品没了”的半成品；真正的原子边界由 session 的建角事务保证。
func NewStartingBag(gender uint8, defs map[ItemID]ItemDef) (*Bag, error) {
	body := startingBodyFemale
	switch gender {
	case 0:
		// 客户端二值性别 0；对应女款 2876。
	case 1:
		body = startingBodyMale
	default:
		return nil, fmt.Errorf("domain: 无效的新角色性别 %d", gender)
	}

	bag := NewBag(DefaultBagSlots)
	for _, id := range []ItemID{body, startingSword} {
		def, ok := defs[id]
		if !ok || def.Equip == nil {
			return nil, fmt.Errorf("domain: 出生装备 %d 的客户端定义缺失", id)
		}
		if left := bag.Add(def, 1); left != 0 {
			return nil, fmt.Errorf("domain: 出生装备 %d 无法放入背包", id)
		}
	}
	return bag, nil
}

// EffectiveBase 返回角色实际参与结算的六维。
//
// 存在的理由是一个**真实踩过的坑**：老存档里没有六维（建号流程还没接属性系统），
// 直接拿零值去推导会得到「最大生命 = 0 × 9 = 0」—— 玩家一进图就是死的，
// 而且死得很隐蔽：既没报错也没日志，只是所有攻击都因为「攻击者已死」被静静跳过。
func (c *Character) EffectiveBase() Base { return c.BaseAtLevel(c.Level) }

// BaseAtLevel 返回角色在 lv 级时**应有**的六维。
//
// c.Base 非零就直接用它（存档里的真值）；零值才按成长曲线从一级补起。
//
// 零值时必须补到指定等级，不能只使用一级初始值。
//
// 参数是 lv 而不是直接读 c.Level：`AddExp` 会先把 c.Level 推到升级后的值，
// 之后 `ApplyLevelUps` 才加成长。那里若按 c.Level 补齐，这段成长会被算两遍。
func (c *Character) BaseAtLevel(lv int32) Base {
	if c.Base != (Base{}) {
		return c.Base
	}
	return NaturalBaseAtLevel(c.Race, lv)
}

// NaturalBaseAtLevel 返回完全没有分配自由点时的等级自然六维。
func NaturalBaseAtLevel(r Race, lv int32) Base {
	b := StartingBase(r)
	for l := int32(2); l <= lv; l++ {
		b = b.Add(GrowthAt(r, l))
	}
	return b
}

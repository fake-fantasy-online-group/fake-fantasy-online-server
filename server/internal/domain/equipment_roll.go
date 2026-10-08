package domain

// ColorProfile 是装备“染色”档位号，来自 game_equipment_color_profiles。
//
// 装备名与属性行的颜色只由**属性条数**决定，所以一个档位实际决定的是一组
// “这次掉落抽几条随机属性”的权重。档位按怪物类别挑选：精英与 BOSS 用原档，
// 普通怪与副本小怪用只配到两条的普通档。
type ColorProfile uint16

// DefaultColorProfile 是精英与 BOSS 的原档。实体没有显式档位（例如测试夹具
// 直接构造的怪）时也回退到它，避免静默把所有装备产成白板。
const DefaultColorProfile ColorProfile = 1

// WeightedAffixCount 是普通装备随机属性条数的一个权重档。权重是份额，
// 无需凑成 100；例如 60/20/10/5/3/1 会按总份额 99 归一化。
type WeightedAffixCount struct {
	Count  uint8
	Weight int64
}

// EquipmentRollKey 精确对应装备资源等级与 ov_desc.type。Type=0 是同等级
// 所有部位都可用的显式通配配置，不从槽位或名称推导。
type EquipmentRollKey struct {
	Level int32
	Type  int32
}

// EquipmentRollOption 是已经预计算到 PostgreSQL 的一个随机结果份额。
// 运行时只做整数加权抽取，不再计算对数正态密度。
type EquipmentRollOption struct {
	CardID int32
	Affix  Affix
	Weight int64
}

// EquipmentRollTable 是服务启动时加载、之后只读的普通装备随机属性矩阵。
// Counts 按染色档位分组：没有配置的条数权重为 0。
type EquipmentRollTable struct {
	Counts  map[ColorProfile][]WeightedAffixCount
	Options map[EquipmentRollKey][]EquipmentRollOption
}

// Enabled 报告随机属性条数和至少一个数值池都已配置。
func (t EquipmentRollTable) Enabled() bool {
	return len(t.Counts) != 0 && len(t.Options) != 0
}

// CountsFor 返回某档位的条数权重。档位缺失时回退默认档，使没带档位的实体
// （测试夹具、旧存档回放）仍按精英/BOSS 原档抽取。
func (t EquipmentRollTable) CountsFor(profile ColorProfile) []WeightedAffixCount {
	if counts := t.Counts[profile]; len(counts) != 0 {
		return counts
	}
	return t.Counts[DefaultColorProfile]
}

// Roll 为一件普通掉落装备抽取实例属性。相同 attr_id 不重复出现；候选不足时
// 少于抽中的条数，绝不跨部位或跨等级拿不兼容属性补齐。
func (t EquipmentRollTable) Roll(def ItemDef, profile ColorProfile, intn func(int64) int64) []InstanceAffix {
	if !t.Enabled() || def.Equip == nil || intn == nil {
		return nil
	}
	counts := t.CountsFor(profile)
	if len(counts) == 0 {
		return nil
	}
	count := weightedAffixCount(counts, intn)
	count = uint8(min(int(count), max(0, MaxEquipmentAttributes-len(def.Equip.Affixes))))
	if count == 0 {
		return nil
	}
	key := EquipmentRollKey{Level: def.Equip.ResourceLevel, Type: def.Equip.Type}
	options := append([]EquipmentRollOption(nil), t.Options[key]...)
	if def.Equip.Type != 0 {
		options = append(options, t.Options[EquipmentRollKey{Level: def.Equip.ResourceLevel}]...)
	}
	out := make([]InstanceAffix, 0, count)
	used := make(map[int32]struct{}, count)
	for len(out) < int(count) {
		eligible := options[:0]
		var total int64
		for _, option := range options {
			if option.Weight <= 0 {
				continue
			}
			if _, exists := used[option.Affix.Attr]; exists {
				continue
			}
			eligible = append(eligible, option)
			total += option.Weight
		}
		options = eligible
		if total <= 0 {
			break
		}
		roll := intn(total)
		chosen := options[len(options)-1]
		for _, option := range options {
			if roll < option.Weight {
				chosen = option
				break
			}
			roll -= option.Weight
		}
		out = append(out, InstanceAffix{CardID: chosen.CardID, Affix: chosen.Affix})
		used[chosen.Affix.Attr] = struct{}{}
	}
	return out
}

func weightedAffixCount(counts []WeightedAffixCount, intn func(int64) int64) uint8 {
	var total int64
	for _, option := range counts {
		if option.Weight > 0 {
			total += option.Weight
		}
	}
	if total <= 0 {
		return 0
	}
	roll := intn(total)
	for _, option := range counts {
		if option.Weight <= 0 {
			continue
		}
		if roll < option.Weight {
			return option.Count
		}
		roll -= option.Weight
	}
	return 0
}

// BossFixedDropKey 用地图号与怪物模板号明确标识固定装备怪。最终 BOSS 与
// 小 BOSS 都由配置点名；game_monsters.kind 在原始数据里可能仍写“精英”或
// “普通”，所以代码不再用该标签、名称、等级或出生顺序猜测。
type BossFixedDropKey struct {
	MapID   int32
	Monster MonsterID
}

type BossFixedDropChoice struct {
	Item        ItemID
	RatePct     float64
	Weight      int64
	RollAffixes bool
}

type BossFixedDropRule struct {
	Minimum uint8
	Pool    []BossFixedDropChoice
}

type BossFixedDropTable map[BossFixedDropKey]BossFixedDropRule

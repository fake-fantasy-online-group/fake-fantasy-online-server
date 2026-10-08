package domain

import "math"

type RefineEffect struct {
	Bonuses        []Affix
	DisabledReason string
}

func (e *EquipDef) RefineEffectAt(level int32) (RefineEffect, bool) {
	if e == nil || level < 0 {
		return RefineEffect{}, false
	}
	if level == 0 {
		return RefineEffect{}, true
	}
	r, ok := e.RefineEffects[level]
	return r, ok && r.DisabledReason == ""
}

// RefinedIntrinsic 返回加工后的装备白字属性。数值计算与提示共用，
// 只合入已折算的固定加工增量，不包含任何附加词条，也不修改模板。
func (e *EquipDef) RefinedIntrinsic(level int32) (Base, Stats) {
	if e == nil {
		return Base{}, Stats{}
	}
	base, stats := e.Base, e.Bonus
	if refine, ok := e.RefineEffectAt(level); ok {
		var basePct Base
		var pct statPercent
		for _, a := range refine.Bonuses {
			// ResolveRefineBonuses 已把百分比折为本件装备的固定增量。
			applyAffix(a, &base, &basePct, &stats, &pct)
		}
	}
	return base, stats
}

// ResolveRefineBonuses 只以装备主表固有值为基数。整级系数是该等级的
// 总加成，不能再累加前面各级；整数除法与现有属性体系一致。
// 先转成固定词条，避免最小/最大攻击各30%被重复放入共享攻击百分比桶。
func ResolveRefineBonuses(e *EquipDef, effects []Affix) ([]Affix, bool) {
	if e == nil {
		return nil, false
	}
	var out []Affix
	for _, a := range effects {
		attrs := []int32{a.Attr}
		if a.Attr == AttrAtk {
			attrs = []int32{AttrMinAtk, AttrMaxAtk}
		}
		for _, attr := range attrs {
			base, known := equipmentIntrinsicValue(e, attr)
			if !known || (a.Mode != ModeAbsolute && a.Mode != ModePercent) {
				return nil, false
			}
			value := int64(a.Value)
			if a.Mode == ModePercent {
				value = int64(base) * value / 100
			}
			if value < math.MinInt32 || value > math.MaxInt32 {
				return nil, false
			}
			if value != 0 {
				out = append(out, Affix{Attr: attr, Value: int32(value), Mode: ModeAbsolute})
			}
		}
	}
	return out, true
}

func equipmentIntrinsicValue(e *EquipDef, attr int32) (int32, bool) {
	switch attr {
	case AttrSTR:
		return e.Base.STR, true
	case AttrVIT:
		return e.Base.VIT, true
	case AttrINT:
		return e.Base.INT, true
	case AttrSPI:
		return e.Base.SPI, true
	case AttrAGI:
		return e.Base.AGI, true
	case AttrDEX:
		return e.Base.DEX, true
	case AttrMinAtk:
		return e.Bonus.MinAtk, true
	case AttrMaxAtk:
		return e.Bonus.MaxAtk, true
	case AttrMAtk:
		return e.Bonus.MAtk, true
	case AttrDef:
		return e.Bonus.Def, true
	case AttrMDef:
		return e.Bonus.MDef, true
	case AttrHit:
		return e.Bonus.Hit, true
	case AttrMaxHP:
		return e.Bonus.MaxHP, true
	case AttrMaxMP:
		return e.Bonus.MaxMP, true
	case AttrHPRegen:
		return e.Bonus.HPRegen, true
	case AttrMaxWeight:
		return e.Bonus.MaxWeight, true
	case AttrCrit:
		return e.Bonus.CritRate, true
	case AttrMCrit:
		return e.Bonus.MCritRate, true
	case AttrPhysRes:
		return e.Bonus.PhysResist, true
	case AttrMagicRes:
		return e.Bonus.MagicResist, true
	case AttrStatusRes:
		return e.Bonus.StatusResist, true
	// 目前主表加载的固有属性中没有攻速/移速。百分比作用于0仍是0，
	// 不得借用角色默认1500ms/移速作为基数，更不能猜一个提速公式。
	case AttrAtkSpeed:
		return e.Bonus.AtkSpeedMS, true
	case AttrMoveSpeed:
		return e.Bonus.MoveSpeed, true
	}
	return 0, false
}

type RefineRecipe struct {
	FailureDestroys                                 bool
	ConfirmCooldownMS                               int32
	RowNo                                           int32
	CurrentLevel                                    int32
	EquipClass                                      int32
	EquipTier                                       int32
	Cost                                            int64
	RequiredSkill                                   SkillID
	ProficiencyMin, ProficiencyMax, ProficiencyGain int32
	SuccessRate                                     int32
	FailureLevel                                    int32 // 255 表示失败不降级
	ProtectItem                                     ItemID
	Materials                                       []CraftMaterial
}

type RefineTable map[[3]int32]RefineRecipe

func (t RefineTable) Get(class, tier, level int32) (RefineRecipe, bool) {
	r, ok := t[[3]int32{class, tier, level}]
	return r, ok
}

func RefineClass(def ItemDef) int32 {
	if def.Equip == nil {
		return 0
	}
	if def.Equip.Position != 0 {
		return def.Equip.Position
	}
	if def.Equip.Slot == int32(SlotWeapon) || def.Equip.Slot == int32(SlotTwoHand) {
		return def.Equip.Type
	}
	return def.Equip.Category
}

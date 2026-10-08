package domain

// EquipmentSuit 的部件按可替代部位分组，效果保持原始档位及词条顺序。
type EquipmentSuit struct {
	ID      int32
	Name    string
	Groups  [][]ItemID
	Bonuses []EquipmentSuitBonus
}

type EquipmentSuitBonus struct {
	Pieces         int32
	Affix          Affix
	Probability    int32
	Description    string
	DisabledReason string
}

// WornMembers 是展示和计算共用的唯一激活判据。破损装备不贡献件数，
// 背包、备用换装与同部位替代款都不能被多算。
func (s *EquipmentSuit) WornMembers(worn *EquipSet, defs func(ItemID) (ItemDef, bool)) (int32, map[ItemID]bool) {
	active := make(map[ItemID]bool)
	if s == nil || worn == nil || defs == nil {
		return 0, active
	}
	worn.Each(func(_ EquipSlot, st Stack) {
		if def, ok := defs(st.Item); ok && EquipmentFunctional(st, def) {
			active[st.Item] = true
		}
	})
	var count int32
	for _, group := range s.Groups {
		for _, id := range group {
			if active[id] {
				count++
				break
			}
		}
	}
	return count, active
}

// SupportsEquipmentSuitAffix 不把未接入结算的词条伪装成已激活。
func SupportsEquipmentSuitAffix(a Affix) bool {
	if a.Mode != ModeAbsolute && a.Mode != ModePercent {
		return false
	}
	switch a.Attr {
	case AttrSTR, AttrVIT, AttrINT, AttrSPI, AttrAGI, AttrDEX,
		AttrMaxHP, AttrMaxMP, AttrMinAtk, AttrMaxAtk, AttrAtk, AttrDef,
		AttrHit, AttrMAtk, AttrMDef, AttrAtkSpeed, AttrMoveSpeed,
		AttrPhysRes, AttrMagicRes, AttrStatusRes, AttrMaxWeight:
		return true
	case AttrCrit, AttrMCrit, AttrHPRegen:
		return a.Mode == ModeAbsolute
	}
	return false
}

package domain

type AstralDropGrade uint8

const (
	AstralDropOrdinary AstralDropGrade = iota
	AstralDropElite
	AstralDropBoss
	AstralDropRealm
)

// Only new combat-equipment drops are rolled. Old instances never pass through
// this generator; pickup, trade and mail retain their existing whole Stack.
func RollDroppedSocketCount(def ItemDef, grade AstralDropGrade, intn func(int64) int64, rule *LootRuleSet) uint8 {
	if rule == nil || def.Equip == nil || intn == nil {
		return 0
	}
	slot := EquipSlot(def.Equip.Slot)
	if slot < SlotFace || slot > SlotShoe && slot != SlotTwoHand {
		return 0
	}
	cap := max(0, MaxEquipmentAttributes-len(def.Equip.Affixes))
	levelCap := rule.SocketCapacity(def.Equip.LevelReq)
	cap = min(cap, levelCap)
	if cap == 0 {
		return 0
	}

	if grade > AstralDropRealm {
		grade = AstralDropOrdinary
	}
	roll := intn(100)
	if roll < 0 || roll >= 100 {
		return 0
	}
	for n, w := range rule.Grades[grade].SocketWeights {
		if roll < w {
			return uint8(min(n, cap))
		}
		roll -= w
	}
	return 0
}
func (t EquipmentRollTable) RollWithReservedSockets(def ItemDef, profile ColorProfile, sockets uint8, intn func(int64) int64) []InstanceAffix {
	if def.Equip == nil {
		return nil
	}
	budget := max(0, MaxEquipmentAttributes-len(def.Equip.Affixes)-int(sockets))
	if budget == 0 {
		return nil
	}
	rolled := t.Roll(def, profile, intn)
	if len(rolled) > budget {
		rolled = rolled[:budget]
	}
	return rolled
}

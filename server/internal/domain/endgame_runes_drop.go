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
func RollDroppedSocketCount(def ItemDef, grade AstralDropGrade, intn func(int64) int64) uint8 {
	if def.Equip == nil || intn == nil || def.Equip.LevelReq < 10 {
		return 0
	}
	slot := EquipSlot(def.Equip.Slot)
	if slot < SlotFace || slot > SlotShoe && slot != SlotTwoHand {
		return 0
	}
	cap := max(0, MaxEquipmentAttributes-len(def.Equip.Affixes))
	levelCap := 2
	if def.Equip.LevelReq >= 50 {
		levelCap = 5
	} else if def.Equip.LevelReq >= 40 {
		levelCap = 4
	} else if def.Equip.LevelReq >= 20 {
		levelCap = 3
	}
	cap = min(cap, levelCap)
	if cap == 0 {
		return 0
	}
	weights := [4][6]int64{{72, 21, 6, 1, 0, 0}, {45, 25, 18, 9, 3, 0}, {25, 25, 23, 17, 8, 2}, {15, 20, 25, 22, 13, 5}}
	if grade > AstralDropRealm {
		grade = AstralDropOrdinary
	}
	roll := intn(100)
	if roll < 0 || roll >= 100 {
		return 0
	}
	for n, w := range weights[grade] {
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

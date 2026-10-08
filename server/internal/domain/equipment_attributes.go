package domain

const MaxEquipmentAttributes = 5

// NaturalAttributeCount 只计天生固定/随机附加词条，不计基础白字、加工、
// 镶嵌、套装或变装。实际孔位与品质计算共用这一口径。
func NaturalAttributeCount(def ItemDef, st Stack) int {
	if def.Equip == nil {
		return 0
	}
	return len(def.Equip.Affixes) + min(int(st.RolledAffixCount), len(st.RolledAffixes))
}

func UsedEquipmentSockets(st Stack) int {
	n := 0
	for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
		if st.Sockets[i] != 0 {
			n++
		}
	}
	return n
}

func EquipmentAttributeCount(def ItemDef, st Stack) int {
	if def.Equip == nil {
		return 0
	}
	// 打出的每个孔就占一条属性配额，空孔同样计色；镶嵌只填充该孔，
	// 不能再把已镶嵌属性额外计一次。
	return NaturalAttributeCount(def, st) + min(int(st.SocketCount), len(st.Sockets))
}

func EquipmentSocketCapacity(def ItemDef, st Stack) int {
	if def.Equip == nil {
		return 0
	}
	return max(0, MaxEquipmentAttributes-NaturalAttributeCount(def, st))
}

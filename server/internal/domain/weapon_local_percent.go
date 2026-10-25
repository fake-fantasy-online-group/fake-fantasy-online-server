package domain

// weaponAttackPercent retains the owning weapon until its ordinary attribute
// lines have been collected. It never handles set bonuses, learned passives,
// statuses or power mechanisms, even when those happen to grant the same attr.
type weaponAttackPercent struct {
	min, max, magic int32
}

func (p *weaponAttackPercent) add(e *EquipDef, a Affix) bool {
	if e == nil || (EquipSlot(e.Slot) != SlotWeapon && EquipSlot(e.Slot) != SlotTwoHand) || a.Mode != ModePercent {
		return false
	}
	add := func(dst *int32) { *dst = SaturateInt32(int64(*dst) + int64(a.Value)) }
	switch a.Attr {
	case AttrMinAtk:
		add(&p.min)
	case AttrMaxAtk:
		add(&p.max)
	case AttrAtk:
		add(&p.min)
		add(&p.max)
	case AttrMAtk:
		add(&p.magic)
	default:
		return false
	}
	return true
}

func (p weaponAttackPercent) bonuses(def ItemDef, st Stack) Stats {
	if p == (weaponAttackPercent{}) {
		return Stats{}
	}
	// True/secret legendary tiers multiply the weapon's innate base first.
	// Refinement, flat affixes, primary attributes and other local enhancements
	// are independent additions, not part of this percentage's denominator.
	st.RefineLevel = 0
	_, innate := LegendaryIntrinsic(def, st)
	return Stats{
		MinAtk: SaturateInt32(int64(innate.MinAtk) * int64(p.min) / 100),
		MaxAtk: SaturateInt32(int64(innate.MaxAtk) * int64(p.max) / 100),
		MAtk:   SaturateInt32(int64(innate.MAtk) * int64(p.magic) / 100),
	}
}

// Formation extras travel through the scene's passive-affix pipe. Resolve
// their weapon-local percentages before that pipe loses the item's identity.
func resolveWeaponAttackAffixes(def ItemDef, st Stack, affixes []Affix) []Affix {
	var local weaponAttackPercent
	out := make([]Affix, 0, len(affixes)+3)
	for _, a := range affixes {
		if !local.add(def.Equip, a) {
			out = append(out, a)
		}
	}
	bonus := local.bonuses(def, st)
	for _, a := range []Affix{{Attr: AttrMinAtk, Value: bonus.MinAtk}, {Attr: AttrMaxAtk, Value: bonus.MaxAtk}, {Attr: AttrMAtk, Value: bonus.MAtk}} {
		if a.Value != 0 {
			out = append(out, a)
		}
	}
	return out
}

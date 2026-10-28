package domain

import "testing"

func TestSpiritProjectionDynamicReplacesLegacyAndAppearanceAddsOnce(t *testing.T) {
	d, _ := SpiritDefinitionForKind(9)
	state := SpiritState{Kind: 9, AttrCount: 2, Attrs: [4]int32{AttrAtk, AttrMAtk}}
	state.Rolls[0][0], state.Rolls[1][0] = 20, 11
	weapon := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Bonus: Stats{MinAtk: 10, MaxAtk: 20, MAtk: 30}}}
	soul := ItemDef{ID: d.ItemID, EquipmentSoul: &EquipmentSoulDef{TargetSlot: SlotWeapon, Affixes: []Affix{{Attr: AttrAtk, Value: 999}}}}
	worn := NewEquipSet()
	st := Stack{Item: weapon.ID, Count: 1, FusedSoul: d.ItemID, FusedSpirit: SpiritAttachment{Item: d.ItemID, State: state}, SpiritAppearance: 13701}
	worn.Set(SlotWeapon, st)
	input := StatSource{Worn: worn, Defs: defsOf(weapon, soul)}
	_, got := Compute(input)
	requireAttackStats(t, got, 110, 120, 141)
	st.FusedSpirit, st.FusedSoul = SpiritAttachment{}, 0
	worn.Set(SlotWeapon, st)
	_, got = Compute(input)
	requireAttackStats(t, got, 90, 100, 130)
	st.SpiritAppearance, st.FusedSoul = 0, d.ItemID
	worn.Set(SlotWeapon, st)
	_, got = Compute(input)
	requireAttackStats(t, got, 1009, 1019, 30)
}

func TestSpiritProjectionWeaponModelPreservesCombatAppearance(t *testing.T) {
	weapon := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Appearance: EquipAppearance{
		Known: true, Part: AppearanceWeaponR, Model: 100, AttackKnown: true, AtkVariant: 2, WeaponCType: 3, AtkDist: 4,
	}}}
	model := ItemDef{ID: 1077, Equip: &EquipDef{Appearance: EquipAppearance{
		Known: true, Part: AppearanceWeaponR, Model: 222, AttackKnown: true, AtkVariant: 8,
	}}}
	worn := NewEquipSet()
	st := Stack{Item: weapon.ID, Count: 1, SpiritAppearance: 13698}
	worn.Set(SlotWeapon, st)
	got := AppearanceFromEquipment(Appearance{}, worn, defsOf(weapon, model))
	if got.EquipView[AppearanceWeaponR] != 222 || got.AtkVariant != 2 || got.WeaponCType != 3 || got.AtkDist != 4 {
		t.Fatalf("appearance projection changed actual weapon attack metadata: %+v", got)
	}
	if EquipmentSoulAvatar(st) != 1077 {
		t.Fatal("native soul lookup must receive bonus equipment ID")
	}
	st.SpiritAppearance, st.FusedSoul = 0, 123
	if EquipmentSoulAvatar(st) != 123 {
		t.Fatal("legacy soul lookup changed")
	}
}

func TestSpiritProjectionLegacyAttachmentRetainsFixedStats(t *testing.T) {
	soul, _ := SpiritDefinitionForKind(9)
	weapon := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Bonus: Stats{MinAtk: 10, MaxAtk: 20}}}
	fixed := ItemDef{ID: soul.ItemID, EquipmentSoul: &EquipmentSoulDef{TargetSlot: SlotWeapon, Affixes: []Affix{{Attr: AttrAtk, Value: 77}}}}
	st := Stack{Item: weapon.ID, Count: 1, FusedSoul: soul.ItemID, FusedSpirit: SpiritAttachment{Item: soul.ItemID, State: SpiritState{Kind: 9, LegacyFixed: true}}}
	worn := NewEquipSet()
	worn.Set(SlotWeapon, st)
	_, got := Compute(StatSource{Worn: worn, Defs: defsOf(weapon, fixed)})
	requireAttackStats(t, got, 87, 97, 0)
}

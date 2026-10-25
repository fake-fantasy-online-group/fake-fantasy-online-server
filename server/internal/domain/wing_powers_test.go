package domain

import "testing"

func wingPowerDonor(id string) (Stack, ItemDef) {
	p, _ := LegendaryPower(id)
	slot := SlotWeapon
	if p.Category == "防御" {
		slot = SlotBody
	}
	d := ItemDef{ID: 4001, LegendaryEnabled: true, Equip: &EquipDef{Slot: int32(slot), LevelReq: 100, Need: Requirement{Professions: 31}}}
	st := Stack{UID: 42, Item: d.ID, Count: 1, InstanceKind: ItemInstanceEquipment, Legendary: LegendaryInstance{PowerID: id, Tier: LegendaryTierTrue}}
	for i, a := range []int32{AttrSTR, AttrINT, AttrVIT, AttrAGI, AttrDEX, AttrSPI} {
		st.Legendary.Affixes[i] = InstanceAffix{Affix: Affix{Attr: a, Value: 3}}
		st.Legendary.Bounds[i] = LegendaryAffixBounds{Min: 2, Max: 4}
	}
	RegisterLegendaryPowerExecutable(id)
	return st, d
}
func TestWingPowerExtractionSlotsOverwriteAndNoAffixes(t *testing.T) {
	wing := Stack{UID: 9, Item: 20609, Count: 1, InstanceKind: ItemInstanceEquipment, RefineLevel: 3, SocketCount: 1}
	for _, id := range []string{"TY-A01", "TY-D01", "TY-U01"} {
		src, def := wingPowerDonor(id)
		var err error
		wing, err = ExtractWingPower(wing, src, def)
		if err != nil {
			t.Fatal(err)
		}
	}
	if wing.WingPowers.Validate() != nil || wing.Legendary != (LegendaryInstance{}) || wing.RefineLevel != 3 || wing.SocketCount != 1 || wing.RolledAffixCount != 0 {
		t.Fatal("wing became darkgold or lost state", wing)
	}
	before := wing
	src, def := wingPowerDonor("TY-A02")
	wing, err := ExtractWingPower(wing, src, def)
	if err != nil {
		t.Fatal(err)
	}
	if wing.WingPowers[0].PowerID != "TY-A02" || wing.WingPowers[1] != before.WingPowers[1] || wing.WingPowers[2] != before.WingPowers[2] {
		t.Fatal("overwrite touched another category")
	}
	for _, mutate := range []func(*Stack){func(s *Stack) { s.Locked = true }, func(s *Stack) { s.Count = 2 }, func(s *Stack) { s.Legendary.Bounds[0].Max = 0 }, func(s *Stack) { s.Endgame.SourceLevel = 1; s.Legendary.PowerID = "ZS-A01" }, func(s *Stack) { s.Legendary.PowerID = "unknown" }} {
		bad := src
		mutate(&bad)
		if got, err := ExtractWingPower(wing, bad, def); err == nil || got != wing {
			t.Fatal("bad donor changed wing")
		}
	}
	badDef := def
	copyEquip := *def.Equip
	badDef.Equip = &copyEquip
	badDef.Equip.Slot = int32(SlotShield)
	if _, err := ExtractWingPower(wing, src, badDef); err == nil {
		t.Fatal("wrong donor category slot accepted")
	}
}
func TestWingPowerActivationClassDuplicateWearAndClone(t *testing.T) {
	wing := Stack{UID: 20, Item: 20609, Count: 1, InstanceKind: ItemInstanceEquipment, Durability: 10, MaxDurability: 10}
	for _, id := range []string{"ZS-A01", "TY-D01", "TY-U01"} {
		src, d := wingPowerDonor(id)
		var err error
		wing, err = ExtractWingPower(wing, src, d)
		if err != nil {
			t.Fatal(err)
		}
	}
	d := ItemDef{ID: wing.Item, Equip: &EquipDef{Slot: int32(SlotBag), Durable: 10}}
	defs := func(id ItemID) (ItemDef, bool) { return d, id == d.ID }
	ch := &Character{Level: 100, Race: Warrior}
	w := NewEquipSet()
	w.Set(SlotBag, wing)
	if len(ActiveLegendaryPowers(w, defs, ch)) != 3 {
		t.Fatal("three slots inactive")
	}
	ch.Race = Healer
	if len(ActiveLegendaryPowers(w, defs, ch)) != 2 {
		t.Fatal("class restriction lost")
	}
	ch.Race = Warrior
	ch.EmploymentKnown = true
	if len(ActiveLegendaryPowers(w, defs, ch)) != 2 {
		t.Fatal("beginner class power")
	}
	ch.Employed = true
	clone := w.Clone()
	changed := wing
	changed.WingPowers[0] = WingPower{}
	clone.Set(SlotBag, changed)
	if w.At(SlotBag) != wing {
		t.Fatal("shared powers")
	}
	rev := w.EffectsRevision()
	w.Set(SlotBag, changed)
	if w.EffectsRevision() == rev {
		t.Fatal("power overwrite failed effect invalidation")
	}
	wing.Durability = 0
	w.Set(SlotBag, wing)
	if len(ActiveLegendaryPowers(w, defs, ch)) != 0 {
		t.Fatal("broken wing active")
	}
}

func TestWingPowerRejectsDonorClassAndSlot(t *testing.T) {
	wing := Stack{UID: 9, Item: 20609, Count: 1, InstanceKind: ItemInstanceEquipment}
	src, d := wingPowerDonor("ZS-A01")
	d.Equip.Need.Professions = 2
	if _, err := ExtractWingPower(wing, src, d); err == nil {
		t.Fatal("wrong class donor accepted")
	}
	d.Equip.Need.Professions = 1
	d.Equip.Slot = int32(SlotRing)
	if _, err := ExtractWingPower(wing, src, d); err == nil {
		t.Fatal("class power on universal-only donor accepted")
	}
}

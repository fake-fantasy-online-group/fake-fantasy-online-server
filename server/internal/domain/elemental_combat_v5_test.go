package domain

import "testing"

func TestElementalV5GoldenAndCaps(t *testing.T) {
	if got := ElementOutgoingDamage(1000, 2000, 1000, false); got != 1300 {
		t.Fatal(got)
	}
	if got := ElementResistanceDamage(1300, 4000, 1000, false); got != 910 {
		t.Fatal(got)
	}
	if got := ElementMitigationDamage(910, 1000, 500); got != 773 {
		t.Fatal(got)
	}
	r := ResolveElementAbsorb(2000, 2000, 773, 1000, 40, false)
	if r.Damage != 696 || r.Absorbed != 77 || r.Heal != 40 || r.HP != 1344 {
		t.Fatalf("%+v", r)
	}
	if got := ElementResistanceDamage(1000, 10000, 2000, false); got != 250 {
		t.Fatal("overcap resistance clamped before penetration", got)
	}
	if got := ElementResistanceDamage(1000, -100000, 100000, false); got != 1500 {
		t.Fatal(got)
	}
	if got := ElementResistanceDamage(1000, 0, 100000, true); got != 1150 {
		t.Fatal(got)
	}
	if got := ElementOutgoingDamage(1000, 9500, 2000, false); got != 2000 {
		t.Fatal(got)
	}
	if got := ElementOutgoingDamage(1000, 0, 2000, true); got != 1100 {
		t.Fatal(got)
	}
	if got := ElementMitigationDamage(1000, 4400, 1000); got != 550 {
		t.Fatal(got)
	}
	for _, h := range []int32{HurtFire, HurtIce, HurtHoly, HurtDark} {
		if HurtElementIndex(h) < 0 {
			t.Fatal(h)
		}
	}
	if HurtElementIndex(HurtPhysical) >= 0 || HurtElementIndex(HurtNone) >= 0 {
		t.Fatal("untyped/physical gained element")
	}
}
func TestElementAbsorptionV5SingleCommit(t *testing.T) {
	cases := []struct {
		hp, max, damage, bp, quota int32
		pvp                        bool
		want                       ElementAbsorbResult
	}{
		{500, 1000, 1000, 2000, 20, false, ElementAbsorbResult{800, 200, 0, 0}},
		{1500, 2000, 1000, 2000, 40, false, ElementAbsorbResult{800, 200, 40, 740}},
		{1000, 1000, 100, 1000, 20, false, ElementAbsorbResult{90, 10, 10, 920}},
		{1000, 1000, 1, 2000, 20, false, ElementAbsorbResult{1, 0, 0, 999}},
		{1000, 1000, 0, 10000, 20, false, ElementAbsorbResult{HP: 1000}},
		{1000, 1000, 100, 10000, 20, true, ElementAbsorbResult{90, 10, 10, 920}},
		{0, 1000, 100, 10000, 20, false, ElementAbsorbResult{}},
	}
	for _, c := range cases {
		if got := ResolveElementAbsorb(c.hp, c.max, c.damage, c.bp, c.quota, c.pvp); got != c.want {
			t.Fatalf("%+v got%+v", c, got)
		}
	}
	for d := int32(1); d <= 1000; d++ {
		r := ResolveElementAbsorb(2000, 2000, d, 10000, 0, false)
		if r.Damage < 1 || r.Heal != 0 || r.Absorbed+r.Damage != d {
			t.Fatal(r)
		}
	}
}
func TestEquipmentV5DefinitionValidation(t *testing.T) {
	for _, d := range UniqueCatalog() {
		if err := ValidateEquipmentEffects(d.TypedEffects); err != nil {
			t.Fatal(d.ID, err)
		}
	}
	for _, e := range []EquipmentEffect{{Kind: "element_damage_bp", Element: "lightning", Value: 5}, {Kind: "element_absorb_bp", Element: "fire", Value: -1}, {Kind: "proc", Proc: &EquipmentProc{ID: "bad"}}, {Kind: "unknown"}} {
		if ValidateEquipmentEffects([]EquipmentEffect{e}) == nil {
			t.Fatal(e)
		}
	}
}

func TestEquipmentEffectsRevisionV5(t *testing.T) {
	worn := NewEquipSet()
	st := Stack{UID: 10, Item: 1001, Count: 1, Durability: 100, MaxDurability: 100}
	worn.Set(SlotWeapon, st)
	r := worn.EffectsRevision()
	if r == 0 {
		t.Fatal("missing generation")
	}
	st.DurabilityWearRaw = 10
	st.Durability = 99
	worn.Set(SlotWeapon, st)
	if worn.EffectsRevision() != r {
		t.Fatal("ordinary wear invalidated effects")
	}
	st.Durability = 0
	worn.Set(SlotWeapon, st)
	if worn.EffectsRevision() == r {
		t.Fatal("broken gear did not invalidate")
	}
	r = worn.EffectsRevision()
	st.Durability = 100
	worn.Set(SlotWeapon, st)
	if worn.EffectsRevision() == r {
		t.Fatal("repair did not invalidate")
	}
	cp := worn.Clone()
	if cp.EffectsRevision() != worn.EffectsRevision() {
		t.Fatal("clone lost generation")
	}
	cp.Set(SlotWeapon, Stack{})
	if worn.At(SlotWeapon).Empty() {
		t.Fatal("clone shared gear")
	}
}

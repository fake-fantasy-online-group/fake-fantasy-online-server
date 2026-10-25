package domain

import (
	"encoding/json"
	"testing"
)

func legendaryRollFixture(t *testing.T) (ItemDef, EquipmentRollTable) {
	t.Helper()
	def := ItemDef{ID: 1001, LegendaryEnabled: true, Equip: &EquipDef{Slot: int32(SlotWeapon), LevelReq: 100, ResourceLevel: 3, Type: 1, Need: Requirement{Professions: 31}}}
	for _, p := range LegendaryPowers() {
		if LegendaryPowerEligible(p, def) {
			old := legendaryExecutable[p.ID]
			RegisterLegendaryPowerExecutable(p.ID)
			t.Cleanup(func() {
				if !old {
					delete(legendaryExecutable, p.ID)
				}
			})
			break
		}
	}
	table := EquipmentRollTable{Options: map[EquipmentRollKey][]EquipmentRollOption{}}
	for i, attr := range []int32{AttrSTR, AttrINT, AttrVIT, AttrCrit, AttrAtkSpeed, AttrPhysRes} {
		v, m := int32(10), int32(ModeAbsolute)
		if attr == AttrCrit {
			v = 100
		}
		if attr == AttrAtkSpeed {
			v = -5
			m = ModePercent
		}
		table.Options[EquipmentRollKey{3, 1}] = append(table.Options[EquipmentRollKey{3, 1}], EquipmentRollOption{CardID: int32(i + 1), Affix: Affix{Attr: attr, Value: v, Mode: m}, Weight: 1})
	}
	return def, table
}
func TestLegendarySixAffixesTypedBoundsAndPersistence(t *testing.T) {
	def, table := legendaryRollFixture(t)
	for _, high := range []bool{false, true} {
		st := Stack{UID: 1, Item: def.ID, Count: 1, RolledAffixCount: 1, RolledAffixes: [5]InstanceAffix{{Affix: Affix{Attr: AttrDEX, Value: 900}}}}
		rng := func(n int) int {
			if high {
				return n - 1
			}
			return 0
		}
		if !table.ApplyLegendaryLegacy(&st, def, rng) {
			t.Fatal("promotion failed")
		}
		if st.RolledAffixCount != 0 || st.RolledAffixes != [5]InstanceAffix{} || NaturalAttributeCount(def, st) != 6 {
			t.Fatal("legacy extra lines leaked")
		}
		seen := map[int32]bool{}
		for i, a := range st.Legendary.Affixes {
			if seen[a.Affix.Attr] {
				t.Fatal("duplicate")
			}
			seen[a.Affix.Attr] = true
			b := st.Legendary.Bounds[i]
			want := b.Min
			if high {
				want = b.Max
			}
			if a.Affix.Value != want {
				t.Fatal("wrong endpoint")
			}
			switch a.Affix.Attr {
			case AttrCrit:
				if b.Min != 300 || b.Max != 600 {
					t.Fatal("crit basis points changed units")
				}
			case AttrAtkSpeed:
				if b.Min != -30 || b.Max != -15 {
					t.Fatal("negative beneficial bound")
				}
			case AttrPhysRes:
				if b.Min != 30 || b.Max != 60 || a.Affix.Mode != ModeAbsolute {
					t.Fatal("flat reduction treated as percentage")
				}
			}
		}
		raw, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		var loaded Stack
		if err = json.Unmarshal(raw, &loaded); err != nil {
			t.Fatal(err)
		}
		if loaded != st {
			t.Fatal("roundtrip changed immutable rolls")
		}
	}
}
func TestLegendaryPoolUsesResourceLevelTypeAndWildcard(t *testing.T) {
	def, table := legendaryRollFixture(t)
	table.Options[EquipmentRollKey{100, 1}] = []EquipmentRollOption{{CardID: 100, Affix: Affix{Attr: AttrSTR, Value: 99999}, Weight: 1}}
	table.Options[EquipmentRollKey{3, 2}] = table.Options[EquipmentRollKey{100, 1}]
	table.Options[EquipmentRollKey{3, 0}] = []EquipmentRollOption{{CardID: 101, Affix: Affix{Attr: AttrSTR, Value: 20}, Weight: 1}, {CardID: 102, Affix: Affix{Attr: AttrCrit, Value: 900, Mode: ModePercent}, Weight: 1}, {CardID: 103, Affix: Affix{Attr: AttrAtkSpeed, Value: -8, Mode: ModePercent}, Weight: 1}}
	for _, a := range table.LegendaryAffixPool(def) {
		if a.Affix.Attr == AttrSTR && a.Affix.Value != 20 {
			t.Fatal("wrong pool")
		}
		if a.Affix.Attr == AttrCrit && a.Affix.Mode != ModeAbsolute {
			t.Fatal("ignored percent crit admitted")
		}
		if a.Affix.Attr == AttrAtkSpeed && a.Affix.Value != -8 {
			t.Fatal("SQL MAX negative mistake")
		}
	}
	def.Equip.ResourceLevel = 4
	if table.LegendaryReady(def) {
		t.Fatal("cross-level fallback")
	}
}
func TestLegendaryInsufficientPoolAndInvalidRNGAtomic(t *testing.T) {
	def, table := legendaryRollFixture(t)
	st := Stack{UID: 1, Item: def.ID, Count: 1}
	before := st
	if table.ApplyLegendary(&st, def, func(n int) int { return n }) || st != before {
		t.Fatal("invalid rng mutated")
	}
	table.Options[EquipmentRollKey{3, 1}] = table.Options[EquipmentRollKey{3, 1}][:5]
	if table.ApplyLegendary(&st, def, func(int) int { return 0 }) || st != before {
		t.Fatal("short pool promoted")
	}
}
func TestLegendaryLegacyLoadDoesNotInferOrReroll(t *testing.T) {
	var st Stack
	if err := json.Unmarshal([]byte(`{"UID":7,"Item":1001,"Count":1,"RolledAffixCount":1,"RolledAffixes":[{"CardID":12,"Affix":{"Attr":1,"Value":17,"Mode":0}}]}`), &st); err != nil {
		t.Fatal(err)
	}
	if st.Legendary != (LegendaryInstance{}) || st.RolledAffixes[0].Affix.Value != 17 {
		t.Fatal("legacy changed")
	}
}

func TestLegendaryCubeAwakenReforgeTemperAndSalvage(t *testing.T) {
	def, table := legendaryRollFixture(t)
	ctx, req := cubeInputs(CubeAwakenUnique)
	old := ctx.Items[def.ID]
	def.EndgameCatalog = old.EndgameCatalog
	def.InventoryTab = old.InventoryTab
	def.InventoryTabKnown = true
	def.InstanceKind = ItemInstanceEquipment
	def.Equip.Durable = 100
	ctx.Items[def.ID] = def
	ctx.EquipmentRoll = table
	ctx.RecipientClass = LegendaryRecipientClass(legendaryClassMask(LegendaryCandidates(def)[0].Class))
	preview, err := PlanCube(ctx, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Target.Legendary.PowerID != "" {
		t.Fatal("preview rolled")
	}
	plan, err := PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	st := plan.Target
	if st.Legendary.PowerID == "" || st.UID != 42 || st.RefineLevel != 3 || st.FusedAppearance != 999 || st.Endgame.UniqueID != 0 {
		t.Fatalf("promotion lost identity: %+v", st)
	}
	ctx.Bag = plan.Bag
	req.RecipeID = CubeReforgeUnique
	plan, err = PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.Legendary.PowerID != st.Legendary.PowerID || plan.Target.Legendary.Bounds != st.Legendary.Bounds {
		t.Fatal("reforge changed immutable identity/bounds")
	}
	ctx.Bag = plan.Bag
	req.RecipeID = CubeTemperUnique
	plan, err = PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Bag = plan.Bag
	req.RecipeID = CubeSalvageUnique
	plan, err = PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Bag.At(200).Empty() {
		t.Fatal("salvage did not consume instance")
	}
}

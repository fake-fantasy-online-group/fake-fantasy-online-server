package domain

import (
	"fmt"
	"testing"
)

func TestLegendaryClassMaskAcceptsOfficialMultiClass(t *testing.T) {
	if got := legendaryClassMask("战士,剑客,刺客"); got != 1|2|4 {
		t.Fatalf("melee mask %d", got)
	}
	if got := legendaryClassMask("药师,术士"); got != 8|16 {
		t.Fatalf("caster mask %d", got)
	}
	if legendaryClassMask("药师,术士,药师") != 0 || legendaryClassMask("药师,全职业") != 0 || legendaryClassMask("未知") != 0 {
		t.Fatal("invalid lists must be rejected")
	}
	if legendaryClassMask("战士") != 1 || legendaryClassMask("全职业") != 31 {
		t.Fatal("single-class masks changed")
	}
}

func TestLegendaryPowerVisibleToClass(t *testing.T) {
	if !LegendaryPowerVisibleToClass("全职业", "") || !LegendaryPowerVisibleToClass("全职业", "战士") {
		t.Fatal("universal visibility")
	}
	if LegendaryPowerVisibleToClass("药师", "") || LegendaryPowerVisibleToClass("药师", "战士") {
		t.Fatal("healer-only leaked")
	}
	if !LegendaryPowerVisibleToClass("药师", "药师") || !LegendaryPowerVisibleToClass("药师,术士", "术士") || !LegendaryPowerVisibleToClass("药师,术士", "药师") {
		t.Fatal("listed professions must see shared powers")
	}
	if LegendaryPowerVisibleToClass("战士,剑客,刺客", "药师") || LegendaryPowerVisibleToClass("药师,术士", "刺客") {
		t.Fatal("cross-class visibility")
	}
}

func TestClassLimitedFormerUniversalUsesCurrentClassAtRuntime(t *testing.T) {
	def, _ := sourceLevelFixture(t)
	def.Equip.Slot, def.Equip.Need.Professions, def.Equip.LevelReq = int32(SlotShoe), 31, 60
	st := NewStack(def, 1)
	p, ok := LegendaryPower("TY-U05")
	if !ok || p.Class != "药师" {
		t.Fatalf("expected 药师-limited 凝疗 %+v", p)
	}
	st.Endgame.SourceLevel = 1
	st.Legendary = LegendaryInstance{PowerID: p.ID, PowerRollBP: LegendaryPowerRollMinBP}
	if !LegendaryPowerEligibleForItem(p, def, st) {
		t.Fatal("eligible 药师 item should remain eligible at typified source level 1")
	}
	if !LegendaryPowerSourceLevelEligible(p, 1) || !LegendaryPowerSourceLevelEligible(p, p.MinLevel) {
		t.Fatal("typified class-limited powers must keep typified source-level policy")
	}
	zs, _ := LegendaryPower("ZS-A01")
	if LegendaryPowerSourceLevelEligible(zs, 1) || !LegendaryPowerSourceLevelEligible(zs, zs.MinLevel) {
		t.Fatal("profession-specific MinLevel must stay in force")
	}
	worn := NewEquipSet()
	worn.Set(SlotShoe, st)
	defs := func(ItemID) (ItemDef, bool) { return def, true }
	healer := &Character{Race: Healer, Employed: true, EmploymentKnown: true, Level: 60}
	warrior := &Character{Race: Warrior, Employed: true, EmploymentKnown: true, Level: 60}
	if len(ActiveLegendaryPowers(worn, defs, healer)) != 1 {
		t.Fatal("药师 must still activate 凝疗")
	}
	if len(ActiveLegendaryPowers(worn, defs, warrior)) != 0 {
		t.Fatal("战士 must not activate class-limited 凝疗")
	}
}

func TestRecipientAllowsMultiClassOnlyOnWearableBase(t *testing.T) {
	def, _ := sourceLevelFixture(t)
	def.Equip.Slot, def.Equip.Need.Professions = int32(SlotWeapon), 1 // warrior-only weapon
	p, ok := LegendaryPower("TY-D01")
	if !ok {
		t.Fatal("missing TY-D01")
	}
	if p.Class == "全职业" {
		t.Skip("catalog class patch not applied")
	}
	warrior := LegendaryRecipientClass(1)
	assassin := LegendaryRecipientClass(4)
	if !warrior.allows(p) || !assassin.allows(p) {
		t.Fatal("melee classes must allow 坚甲")
	}
	if !warrior.allowsOnItem(p, def) {
		t.Fatal("warrior may receive 坚甲 on warrior weapon")
	}
	if assassin.allowsOnItem(p, def) {
		t.Fatal("assassin must not receive 坚甲 on a warrior-only base")
	}
	def.Equip.Need.Professions = 31
	if !assassin.allowsOnItem(p, def) {
		t.Fatal("assassin may receive 坚甲 on an unrestricted base")
	}
}

func TestTypifiedClassLimitedStayInTypifiedSplitPool(t *testing.T) {
	powers := []LegendaryPowerDefinition{
		{ID: "TY-D01", Class: "战士,剑客,刺客"},
		{ID: "TY-U19", Class: "全职业"},
		{ID: "ZS-A01", Class: "战士"},
	}
	got, ok := chooseLegendaryPowerPool(powers, func(n int) int {
		if n != 2 {
			t.Fatal("split should still be typified vs profession", n)
		}
		return 1
	})
	if !ok || len(got) != 2 {
		t.Fatal("typified class-limited left the typified pool", got, ok)
	}
	for _, p := range got {
		if !typifiedCatalogPower(p.ID) {
			t.Fatal("profession-specific leaked into typified split", p.ID)
		}
	}
	got, ok = chooseLegendaryPowerPool(powers, func(n int) int { return 0 })
	if !ok || len(got) != 1 || got[0].ID != "ZS-A01" {
		t.Fatal("profession split lost ZS-A01", got, ok)
	}
	// Official 术士 at typified source 1 should not consume a profession-pool roll.
	def, _ := sourceLevelFixture(t)
	powers = LegendaryCandidatesForRecipientWithPolicyAtLevel(def, 1, 16, LegendaryPowerPolicySplit)
	if len(powers) == 0 {
		t.Fatal("术士 typified fallback empty")
	}
	for _, p := range powers {
		if !typifiedCatalogPower(p.ID) {
			t.Fatal("profession-specific at typified source 1", p.ID)
		}
		if p.Class != "全职业" && !LegendaryRecipientClass(16).allows(p) {
			t.Fatal("unrelated class typified admitted", p.ID, p.Class)
		}
	}
}
func TestTypifiedUnifiedClassSourceAndWingPolicy(t *testing.T) {
	limited := map[string]string{
		"TY-D01": "战士,剑客,刺客", "TY-D02": "药师,术士", "TY-D07": "战士,剑客,刺客",
		"TY-D08": "药师,术士", "TY-D11": "药师,术士", "TY-U04": "药师", "TY-U05": "药师",
		"TY-U07": "战士,剑客,刺客,术士", "TY-U10": "药师", "TY-U14": "药师,术士", "TY-U16": "药师",
		"TY-U17": "药师", "TY-U18": "药师,术士",
	}
	var typified []LegendaryPowerDefinition
	for _, prefix := range []string{"TY-U", "TY-D"} {
		for i := 1; i <= 20; i++ {
			id := prefix + fmt.Sprintf("%02d", i)
			p, ok := LegendaryPower(id)
			if !ok {
				t.Fatalf("missing typified power %s", id)
			}
			typified = append(typified, p)
			want, isLimited := limited[p.ID]
			if isLimited {
				if p.Class != want {
					t.Fatalf("%s class %q want %q", p.ID, p.Class, want)
				}
			} else if p.Class != "全职业" {
				t.Fatalf("%s should stay 全职业, got %q", p.ID, p.Class)
			}
			if !LegendaryPowerSourceLevelEligible(p, 1) || LegendaryPowerSourceLevelEligible(p, 0) {
				t.Fatalf("%s typified source policy changed (MinLevel=%d)", p.ID, p.MinLevel)
			}
		}
	}
	if len(typified) != 40 || len(limited) != 13 {
		t.Fatalf("expected 40 typified U/D and 13 limited, got %d powers", len(typified))
	}
	zs, ok := LegendaryPower("ZS-A01")
	if !ok || LegendaryPowerSourceLevelEligible(zs, 1) || !LegendaryPowerSourceLevelEligible(zs, zs.MinLevel) {
		t.Fatal("profession MinLevel must stay tight")
	}
	def, _ := sourceLevelFixture(t)
	def.Equip.Slot, def.Equip.Need.Professions, def.Equip.LevelReq = int32(SlotShoe), 31, 60
	healer := &Character{Race: Healer, Employed: true, EmploymentKnown: true, Level: 60}
	warrior := &Character{Race: Warrior, Employed: true, EmploymentKnown: true, Level: 60}
	// Old and new monster-sourced gear share the current class rule; no grandfather branch.
	for _, srcLv := range []int32{1, 60} {
		st := NewStack(def, 1)
		st.Endgame.SourceLevel = srcLv
		st.Legendary = LegendaryInstance{PowerID: "TY-U05", PowerRollBP: LegendaryPowerRollMinBP}
		worn := NewEquipSet()
		worn.Set(SlotShoe, st)
		defs := func(ItemID) (ItemDef, bool) { return def, true }
		if !LegendaryPowerEligibleForItem(typifiedPower(t, "TY-U05"), def, st) {
			t.Fatalf("source %d should not raise typified MinLevel", srcLv)
		}
		if len(ActiveLegendaryPowers(worn, defs, healer)) != 1 || len(ActiveLegendaryPowers(worn, defs, warrior)) != 0 {
			t.Fatalf("source %d class activation mismatch", srcLv)
		}
	}
	// Extracted wings use the same current-class rule.
	wing := Stack{UID: 20, Item: 20609, Count: 1, InstanceKind: ItemInstanceEquipment, Durability: 10, MaxDurability: 10}
	src, donor := wingPowerDonor("TY-U05")
	var err error
	wing, err = ExtractWingPower(wing, src, donor)
	if err != nil {
		t.Fatal(err)
	}
	wingDef := ItemDef{ID: wing.Item, Equip: &EquipDef{Slot: int32(SlotBag), Durable: 10}}
	wingDefs := func(id ItemID) (ItemDef, bool) { return wingDef, id == wingDef.ID }
	worn := NewEquipSet()
	worn.Set(SlotBag, wing)
	if len(ActiveLegendaryPowers(worn, wingDefs, healer)) != 1 || len(ActiveLegendaryPowers(worn, wingDefs, warrior)) != 0 {
		t.Fatal("wing typified class rule diverged from gear")
	}
}

func typifiedPower(t *testing.T, id string) LegendaryPowerDefinition {
	t.Helper()
	p, ok := LegendaryPower(id)
	if !ok {
		t.Fatalf("missing %s", id)
	}
	return p
}

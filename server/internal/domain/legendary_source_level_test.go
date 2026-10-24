package domain

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func sourceLevelFixture(t *testing.T) (ItemDef, EquipmentRollTable) {
	t.Helper()
	def, table := legendaryRollFixture(t)
	def.InstanceKind = ItemInstanceEquipment
	def.Equip.Slot, def.Equip.LevelReq = int32(SlotFace), 18
	def.Equip.Base.STR = 10
	def.Equip.Need = Requirement{Level: 18}
	for _, p := range LegendaryPowers() {
		old := legendaryExecutable[p.ID]
		RegisterLegendaryPowerExecutable(p.ID)
		t.Cleanup(func() {
			if !old {
				delete(legendaryExecutable, p.ID)
			}
		})
	}
	return def, table
}

func TestLegendarySourceLevelCandidateBoundaries(t *testing.T) {
	def, _ := sourceLevelFixture(t)
	aboveWear := false
	for _, p := range LegendaryCandidatesAtLevel(def, 43) {
		if p.MinLevel > 43 || p.Class != "全职业" || (p.Category != "防御" && p.Category != "通用") {
			t.Fatal("invalid face candidate", p.ID)
		}
		aboveWear = aboveWear || p.MinLevel > def.Equip.LevelReq
	}
	if !aboveWear {
		t.Fatal("low-wear face did not receive higher powers from level43 monster")
	}
	p, _ := LegendaryPower("TY-D09") // Actual catalog threshold 42.
	for _, tc := range []struct {
		level int32
		want  bool
	}{{-1, false}, {0, false}, {1, false}, {41, false}, {42, true}, {43, true}, {math.MaxInt32, true}} {
		for _, wear := range []int32{1, 18, 60, 100} {
			def.Equip.LevelReq = wear
			if got := LegendaryPowerEligibleAtLevel(p, def, tc.level); got != tc.want {
				t.Fatalf("source=%d wear=%d got=%v", tc.level, wear, got)
			}
		}
	}
	p.MinLevel = 0
	if LegendaryPowerEligibleAtLevel(p, def, 43) {
		t.Fatal("invalid power threshold admitted")
	}
	for _, source := range []int32{-1, 0, 1} {
		if len(LegendaryCandidatesAtLevel(def, source)) != 0 {
			t.Fatal("invalid/underlevel source admitted", source)
		}
	}
}

func TestLegendarySourceLevelPersistenceActivationAndAffixes(t *testing.T) {
	def, table := sourceLevelFixture(t)
	source := int32(43)
	powers := LegendaryCandidatesAtLevel(def, source)
	wantPower := -1
	for i, p := range powers {
		if p.ID == "TY-D09" {
			wantPower = i
		}
	}
	if wantPower < 0 {
		t.Fatal("threshold42 candidate absent")
	}
	for _, tierRoll := range []int{0, 1, 99} {
		st := NewStack(def, 1)
		calls := 0
		rng := func(n int) int {
			calls++
			if calls == 1 {
				return wantPower
			}
			if calls == 14 {
				return tierRoll
			}
			return n - 1
		}
		if !table.ApplyLegendaryAtLevel(&st, def, source, rng) {
			t.Fatal("promotion failed")
		}
		if st.Endgame.SourceLevel != source || EquipmentRequiredLevel(def, st) != 18 || def.Equip.ResourceLevel != 3 {
			t.Fatal("source contaminated wear/affix level", st)
		}
		raw, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		var restored Stack
		if err = json.Unmarshal(raw, &restored); err != nil || restored != st {
			t.Fatal("roundtrip changed provenance", err)
		}
		if len(LegendaryItemBonuses(def, restored)) != 6 {
			t.Fatal("reloaded high-source low-base affixes disappeared")
		}
		b, _ := LegendaryIntrinsic(def, restored)
		multiplier := int32(1)
		if tierRoll == 0 {
			multiplier = 3
		} else if tierRoll == 1 {
			multiplier = 2
		}
		if b.STR != 10*multiplier {
			t.Fatal("tier intrinsic rejected generation level", b.STR)
		}
		worn := NewEquipSet()
		worn.Set(SlotFace, restored)
		for _, playerLevel := range []int32{17, 18, 43, 100} {
			ch := &Character{Level: playerLevel}
			got := ActiveLegendaryPowers(worn, func(ItemID) (ItemDef, bool) { return def, true }, ch)
			if (len(got) == 1) != (playerLevel >= 18) {
				t.Fatal("power activation confused source with unchanged wear gate", playerLevel, got)
			}
		}
		// Neither a repeated generation request nor reforge can change the origin/power.
		before := restored
		if table.ApplyLegendaryAtLevel(&restored, def, 100, rng) || restored != before {
			t.Fatal("existing item rerolled")
		}
		restored.Legendary.Reforge(func(int) int { return 0 })
		if restored.Endgame.SourceLevel != source || restored.Legendary.PowerID != before.Legendary.PowerID {
			t.Fatal("reforge changed provenance")
		}
	}
}

func TestLegendarySourceLevelAwakeningPreservesOriginAndLegacy(t *testing.T) {
	def, table := sourceLevelFixture(t)
	for _, source := range []int32{0, 12, 43} {
		for _, characterLevel := range []int32{18, 100} {
			ctx, req := cubeInputs(CubeAwakenUnique)
			old := ctx.Items[def.ID]
			def.EndgameCatalog = old.EndgameCatalog
			def.InventoryTab = old.InventoryTab
			def.InventoryTabKnown = true
			def.InstanceKind = ItemInstanceEquipment
			ctx.Items[def.ID] = def
			ctx.EquipmentRoll = table
			ctx.Level = characterLevel
			st := ctx.Bag.At(200)
			st.Endgame.SourceLevel = source
			ctx.Bag.Set(200, st)
			candidates := LegendaryCandidatesAtLevel(def, LegendaryGenerationLevel(def, st))
			ctx.Intn = func(n int) int { return n - 1 }
			before := ctx.Bag.Clone()
			preview, err := PlanCube(ctx, req, false)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, ctx.Bag) || preview.Target.Legendary.PowerID != "" {
				t.Fatal("preview changed old item")
			}
			plan, err := PlanCube(ctx, req, true)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Target.Endgame.SourceLevel != source {
				t.Fatal("awakening invented/replaced monster provenance")
			}
			p, _ := LegendaryPower(plan.Target.Legendary.PowerID)
			if len(candidates) == 0 || p.ID != candidates[len(candidates)-1].ID {
				t.Fatal("awakening used character level", source, characterLevel, p.ID)
			}
		}
	}
	def.Equip.LevelReq = 60
	st := NewStack(def, 1)
	st.Endgame.SourceLevel = 12
	before := st
	if table.ApplyLegendaryAtLevel(&st, def, 43, func(int) int { return 0 }) || st != before {
		t.Fatal("existing provenance overwritten")
	}
	for _, source := range []int32{-1, 0} {
		if table.ApplyLegendaryAtLevel(&st, def, source, func(int) int { return 0 }) || st != before {
			t.Fatal("invalid level mutated item")
		}
	}
}

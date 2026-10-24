package domain

import (
	"encoding/json"
	"math"
	"testing"
)

func TestLegendaryUniversalEightyPowersIgnoreMinimumAtMonsterOne(t *testing.T) {
	def, table := sourceLevelFixture(t)
	def.Equip.Slot = int32(SlotShoe)
	def.Equip.LevelReq = 1
	def.Equip.Need.Level = 1
	powers := LegendaryCandidatesAtLevel(def, 1)
	if len(powers) != 80 {
		t.Fatal("level1 shoe must admit exactly80 universal powers", len(powers))
	}
	categories := map[string]int{}
	for index, p := range powers {
		if p.Class != "全职业" {
			t.Fatal("profession power leaked into level1", p.ID)
		}
		categories[p.Category]++
		ignored := p
		ignored.MinLevel = math.MaxInt32
		if !LegendaryPowerEligibleAtLevel(ignored, def, 1) {
			t.Fatal("universal historical MinLevel still used", p.ID)
		}
		for _, invalid := range []int32{-1, 0} {
			if LegendaryPowerEligibleAtLevel(p, def, invalid) {
				t.Fatal("invalid monster provenance accepted")
			}
		}
		for _, draw := range []int{99, 1, 0} {
			st := NewStack(def, 1)
			calls := 0
			if !table.ApplyLegendaryAtLevel(&st, def, 1, func(n int) int {
				calls++
				if calls == 1 {
					return index
				}
				if calls == 14 {
					return draw
				}
				return n - 1
			}) {
				t.Fatal("universal generation failed", p.ID, draw)
			}
			raw, _ := json.Marshal(st)
			var loaded Stack
			if json.Unmarshal(raw, &loaded) != nil || loaded != st {
				t.Fatal("roundtrip changed item")
			}
			if loaded.Legendary.PowerID != p.ID || loaded.Endgame.SourceLevel != 1 || len(LegendaryItemBonuses(def, loaded)) != 6 {
				t.Fatal("universal low-source bonuses missing", p.ID)
			}
			b, _ := LegendaryIntrinsic(def, loaded)
			factor := int32(1)
			if draw == 1 {
				factor = 2
			} else if draw == 0 {
				factor = 3
			}
			if b.STR != 10*factor {
				t.Fatal("tier source gate still applied", p.ID, draw)
			}
			worn := NewEquipSet()
			worn.Set(SlotShoe, loaded)
			if len(ActiveLegendaryPowers(worn, func(ItemID) (ItemDef, bool) { return def, true }, &Character{Level: 1})) != 1 {
				t.Fatal("universal activation still gated", p.ID)
			}
		}
	}
	if categories["攻击"] != 40 || categories["防御"] != 20 || categories["通用"] != 20 {
		t.Fatal("universal category scope changed", categories)
	}
	def.Equip.Slot = -1
	if len(LegendaryCandidatesAtLevel(def, 100)) != 0 {
		t.Fatal("universal removed slot restrictions")
	}
	def.Equip.Slot = int32(SlotWeapon)
	def.Equip.Need.Professions = 1
	warrior, _ := LegendaryPower("ZS-A01")
	swordsman, _ := LegendaryPower("JK-A01")
	if LegendaryPowerEligibleAtLevel(warrior, def, warrior.MinLevel-1) || !LegendaryPowerEligibleAtLevel(warrior, def, warrior.MinLevel) || LegendaryPowerEligibleAtLevel(swordsman, def, 100) {
		t.Fatal("profession minimum/type restrictions changed")
	}
}

func TestLegendaryUniversalStillRequiresSixNativeAffixes(t *testing.T) {
	def, table := sourceLevelFixture(t)
	table.Options[EquipmentRollKey{Level: 3, Type: 1}] = table.Options[EquipmentRollKey{Level: 3, Type: 1}][:5]
	st := NewStack(def, 1)
	before := st
	if table.ApplyLegendaryAtLevel(&st, def, 1, func(int) int { return 0 }) || st != before {
		t.Fatal("missing native affix was synthesized")
	}
}

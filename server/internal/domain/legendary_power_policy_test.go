package domain

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestLegendarySplitPolicyEveryProfessionAndSlot(t *testing.T) {
	def, table := sourceLevelFixture(t)
	for race := Warrior; race <= Warlock; race++ {
		recipient := LegendaryClassForCharacter(&Character{Race: race, Employed: true, EmploymentKnown: true})
		for _, slot := range []EquipSlot{SlotFace, SlotHead, SlotNeck, SlotWeapon, SlotShield, SlotGlove, SlotRing, SlotBody, SlotShoe, SlotBag, SlotTwoHand} {
			def.Equip.Slot = int32(slot)
			def.Equip.Need = Requirement{Level: 18} // Real unrestricted accessories do not have a profession mask.
			all := LegendaryCandidatesForRecipientWithPolicyAtLevel(def, 100, recipient, LegendaryPowerPolicySplit)
			classes := map[byte]bool{}
			for _, p := range all {
				if !recipient.allows(p) {
					t.Fatal("other profession entered pool", race, slot, p.ID)
				}
				if p.Class == "全职业" {
					if !LegendaryPowerEligibleAtLevel(p, def, 100) {
						t.Fatal("universal slot restriction changed", slot, p.ID)
					}
				} else {
					classes[p.ID[3]] = true
				}
			}
			if len(classes) != 3 {
				t.Fatal("missing class A/D/U category at slot", race, slot, classes)
			}
			for _, categoryRoll := range []int{0, 1} {
				st := NewStack(def, 1)
				calls := 0
				rng := func(n int) int {
					calls++
					if calls == 1 {
						if n != 2 {
							t.Fatal("category must precede pool", n)
						}
						return categoryRoll
					}
					return n - 1
				}
				if !table.ApplyLegendaryForRecipientWithPolicyAtLevel(&st, def, 100, recipient, LegendaryPowerPolicySplit, true, rng) {
					t.Fatal("new roll failed", race, slot)
				}
				power, _ := LegendaryPower(st.Legendary.PowerID)
				if (power.Class == "全职业") != (categoryRoll == 1) {
					t.Fatal("wrong selected category", power.ID)
				}
				if st.Endgame.SourceLevel != 100 || st.Legendary.Validate() != nil || len(LegendaryItemBonuses(def, st)) != 6 {
					t.Fatal("invalid persistent item", st)
				}
				worn := NewEquipSet()
				worn.Set(slot, st)
				defs := func(ItemID) (ItemDef, bool) { return def, true }
				ch := &Character{Race: race, Employed: true, EmploymentKnown: true, Level: 100}
				if len(ActiveLegendaryPowers(worn, defs, ch)) != 1 {
					t.Fatal("new slot power does not activate", race, slot, power.ID)
				}
				ch.Race = (race + 1) % 5
				if got := len(ActiveLegendaryPowers(worn, defs, ch)); (got == 1) != (categoryRoll == 1) {
					t.Fatal("wrong profession activated class power", race, slot, power.ID)
				}
				before := st
				if table.ApplyLegendaryForRecipientWithPolicyAtLevel(&st, def, 100, recipient, LegendaryPowerPolicySplit, true, func(int) int { t.Fatal("owned item rerolled"); return 0 }) || st != before {
					t.Fatal("owned item changed")
				}
			}
		}
	}
}

func TestLegendarySplitPolicyEmptyClassAndRestrictedBases(t *testing.T) {
	def, table := sourceLevelFixture(t)
	for _, recipient := range []LegendaryRecipientClass{0, 3, 32, 16} {
		for _, level := range []int32{1, 100} {
			if recipient == 16 && level == 100 {
				continue
			}
			powers := LegendaryCandidatesForRecipientWithPolicyAtLevel(def, level, recipient, LegendaryPowerPolicySplit)
			if len(powers) == 0 {
				t.Fatal("universal fallback absent")
			}
			for _, p := range powers {
				if p.Class != "全职业" {
					t.Fatal("invalid profession/level admitted", recipient, level, p.ID)
				}
			}
			st := NewStack(def, 1)
			first := true
			if !table.ApplyLegendaryForRecipientWithPolicyAtLevel(&st, def, level, recipient, LegendaryPowerPolicySplit, true, func(n int) int {
				if first {
					first = false
					if n != len(powers) {
						t.Fatal("empty class still consumed category roll", n)
					}
				}
				return 0
			}) {
				t.Fatal("fallback lost awarded legendary")
			}
		}
	}
	def.Equip.Need = Requirement{Professions: 1, ProfessionRestricted: true}
	for _, p := range LegendaryCandidatesForRecipientWithPolicyAtLevel(def, 100, 16, LegendaryPowerPolicySplit) {
		if p.Class != "全职业" {
			t.Fatal("other-class base changed", p.ID)
		}
	}
	for _, policy := range []int32{-1, 2} {
		st := NewStack(def, 1)
		before := st
		if table.ApplyLegendaryForRecipientWithPolicyAtLevel(&st, def, 100, 1, policy, true, func(int) int { t.Fatal("invalid policy drew"); return 0 }) || st != before {
			t.Fatal("unknown policy accepted")
		}
	}
}

func TestLegendarySplitPolicyCategoryIndependentOfPoolSize(t *testing.T) {
	class := LegendaryPowerDefinition{ID: "SS-A01", Class: "术士"}
	uni := LegendaryPowerDefinition{ID: "TY-D01", Class: "全职业"}
	for _, sizes := range [][2]int{{1, 80}, {60, 1}, {2, 7}} {
		var pool []LegendaryPowerDefinition
		for i := 0; i < sizes[0]; i++ {
			pool = append(pool, class)
		}
		for i := 0; i < sizes[1]; i++ {
			pool = append(pool, uni)
		}
		count := [2]int{}
		for roll := 0; roll < 10000; roll++ {
			chosen, ok := chooseLegendaryPowerPool(pool, func(n int) int {
				if n != 2 {
					t.Fatal(n)
				}
				return roll % 2
			})
			if !ok {
				t.Fatal("pool failed")
			}
			if chosen[0].Class == "全职业" {
				count[1]++
			} else {
				count[0]++
			}
		}
		if count != [2]int{5000, 5000} {
			t.Fatal("category depends on pool size", sizes, count)
		}
	}
	for _, bad := range []int{-1, 2} {
		if _, ok := chooseLegendaryPowerPool([]LegendaryPowerDefinition{class, uni}, func(int) int { return bad }); ok {
			t.Fatal("bad RNG accepted")
		}
	}
}

func TestLegendarySplitPolicyLegacyExactRNGAndInstance(t *testing.T) {
	def, table := sourceLevelFixture(t)
	for _, slot := range []EquipSlot{SlotHead, SlotFace, SlotShoe, SlotWeapon} {
		def.Equip.Slot = int32(slot)
		def.Equip.Need.Professions = 31
		for seed := int64(0); seed < 30; seed++ {
			for _, variants := range []bool{false, true} {
				a, b := NewStack(def, 1), NewStack(def, 1)
				b = a
				ra, rb := rand.New(rand.NewSource(seed)), rand.New(rand.NewSource(seed))
				var ba, bb []int
				ia := func(n int) int { ba = append(ba, n); return ra.Intn(n) }
				ib := func(n int) int { bb = append(bb, n); return rb.Intn(n) }
				var old bool
				if variants {
					old = table.ApplyLegendaryForRecipientAtLevel(&a, def, 43, 16, ia)
				} else {
					old = table.ApplyLegendaryLegacyForRecipientAtLevel(&a, def, 43, 16, ia)
				}
				now := table.ApplyLegendaryForRecipientWithPolicyAtLevel(&b, def, 43, 16, 0, variants, ib)
				if old != now || a != b || !reflect.DeepEqual(ba, bb) || ra.Int63() != rb.Int63() {
					t.Fatal("legacy stream changed", slot, seed, variants)
				}
			}
		}
	}
}

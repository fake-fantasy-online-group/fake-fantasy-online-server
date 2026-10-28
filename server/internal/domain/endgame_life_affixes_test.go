package domain

import (
	"encoding/json"
	"math/rand"
	"testing"
)

func lifeFixture(t *testing.T) (Stack, ItemDef, EquipmentRollTable) {
	def, table := legendaryRollFixture(t)
	st := Stack{UID: 1, Item: def.ID, Count: 1, RefineLevel: 15}
	if !table.ApplyLegendaryLegacy(&st, def, func(int) int { return 0 }) {
		t.Fatal("fixture")
	}
	return st, def, table
}
func TestLifeValueEveryGradeFullSupport(t *testing.T) {
	for grade := 1; grade <= 7; grade++ {
		for _, attr := range []int32{AttrSTR, AttrAtkSpeed} {
			for _, high := range []bool{false, true} {
				b := LegendaryAffixBounds{3, 60}
				if attr == AttrAtkSpeed {
					b = LegendaryAffixBounds{-60, -3}
				}
				rng := func(n int) int {
					if high {
						return n - 1
					}
					return 0
				}
				got, err := lifeAffixValue(b, attr, grade, rng)
				want := b.Min
				if high {
					want = b.Max
				}
				if err != nil || got != want {
					t.Fatalf("grade %d attr%d high%v: %d %v", grade, attr, high, got, err)
				}
			}
		}
	}
}
func TestLifeGradeDistributionImprovesWithoutRangeChange(t *testing.T) {
	last := int64(0)
	for grade := 1; grade <= 7; grade++ {
		r := rand.New(rand.NewSource(7))
		sum := int64(0)
		for i := 0; i < 30000; i++ {
			v, e := lifeAffixValue(LegendaryAffixBounds{1, 100}, AttrSTR, grade, r.Intn)
			if e != nil {
				t.Fatal(e)
			}
			sum += int64(v)
		}
		if sum <= last {
			t.Fatalf("grade %d did not improve", grade)
		}
		last = sum
	}
}
func TestLifeTemperOverwriteShuffleAndPersistence(t *testing.T) {
	st, def, _ := lifeFixture(t)
	original := st
	next, err := LifeTemperAffix(st, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if st != original || next.Legendary.TemperedSlot != 1 {
		t.Fatal("input mutated or marker missing")
	}
	a, b := next.Legendary.EffectiveAffix(0)
	if a.Affix.Value != 45 || b.Min != 45 || b.Max != 90 {
		t.Fatalf("bad boost %+v %+v", a, b)
	}
	if LegendaryItemBonuses(def, next)[0] != a.Affix {
		t.Fatal("combat missing temper")
	}
	again, err := LifeTemperAffix(next, func(n int) int { return n - 1 })
	if err != nil {
		t.Fatal(err)
	}
	if again.Legendary.TemperedSlot != 6 || again.Legendary.Affixes != st.Legendary.Affixes || again.Legendary.Bounds != st.Legendary.Bounds {
		t.Fatal("stacking or overwrote raw rolls")
	}
	raw, _ := again.Legendary.EffectiveAffix(0)
	if raw != st.Legendary.Affixes[0] {
		t.Fatal("old bonus remains")
	}
	temperedAttr := again.Legendary.Affixes[5].Affix.Attr
	shuffled, err := LifeShuffleAffixes(again, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if shuffled.Legendary.Affixes[shuffled.Legendary.TemperedSlot-1].Affix.Attr != temperedAttr {
		t.Fatal("temper detached during shuffle")
	}
	for i, a := range again.Legendary.Affixes {
		found := false
		for j, b := range shuffled.Legendary.Affixes {
			if a == b && again.Legendary.Bounds[i] == shuffled.Legendary.Bounds[j] {
				found = true
			}
		}
		if !found {
			t.Fatal("lost paired affix")
		}
	}
	data, _ := json.Marshal(shuffled)
	var restored Stack
	if err = json.Unmarshal(data, &restored); err != nil || restored != shuffled || restored.Legendary.Validate() != nil {
		t.Fatal("durability roundtrip", err)
	}
	for i := 0; i < 100; i++ {
		again, err = LifeTemperAffix(again, func(n int) int { return i % n })
		if err != nil {
			t.Fatal(err)
		}
	}
	if again.Legendary.Affixes != st.Legendary.Affixes {
		t.Fatal("repeat inflated raw value")
	}
}
func TestLifeRerollPreservesOthersAndRejectsInvalid(t *testing.T) {
	st, def, table := lifeFixture(t)
	// Expand pool with one fresh attribute to demonstrate actual type replacement.
	table.Options[EquipmentRollKey{3, 1}] = append(table.Options[EquipmentRollKey{3, 1}], EquipmentRollOption{CardID: 99, Affix: Affix{Attr: AttrDEX, Value: 20}, Weight: 1})
	for slot := 0; slot < 4; slot++ {
		got, err := LifeRerollAffix(st, def, table, slot, 7, map[int32]int{AttrDEX: 99}, func(n int) int { return n - 1 })
		if err != nil {
			t.Fatal(err)
		}
		if got.Legendary.PowerID != st.Legendary.PowerID || got.Legendary.PowerRollBP != st.Legendary.PowerRollBP {
			t.Fatal("power changed")
		}
		for i := range st.Legendary.Affixes {
			if i != slot && (got.Legendary.Affixes[i] != st.Legendary.Affixes[i] || got.Legendary.Bounds[i] != st.Legendary.Bounds[i]) {
				t.Fatal("other slot changed")
			}
		}
		value, err := LifeRerollAffixValue(got, slot, 1, func(int) int { return 0 })
		if err != nil {
			t.Fatal(err)
		}
		if value.Legendary.Affixes[slot].Affix.Attr != got.Legendary.Affixes[slot].Affix.Attr || value.Legendary.Bounds != got.Legendary.Bounds {
			t.Fatal("value wash changed type/bounds")
		}
	}
	for _, slot := range []int{-1, 4, 5, 6} {
		if _, err := LifeRerollAffixValue(st, slot, 1, nil); err == nil {
			t.Fatal("invalid slot accepted")
		}
	}
	for _, grade := range []int{0, 8} {
		if _, err := LifeRerollAffixValue(st, 0, grade, nil); err == nil {
			t.Fatal("invalid grade accepted")
		}
	}
	low := st
	low.RefineLevel = 14
	if _, err := LifeTemperAffix(low, nil); err == nil {
		t.Fatal("+14 allowed")
	}
	if got, err := LifeShuffleAffixes(st, func(n int) int { return n }); err == nil || got != st {
		t.Fatal("bad RNG partially mutated")
	}
	if got, err := LifeRerollAffix(st, def, table, 0, 1, nil, func(n int) int { return -1 }); err == nil || got != st {
		t.Fatal("bad RNG reroll")
	}
	for _, call := range []func() (Stack, error){func() (Stack, error) { return LifeRerollAffix(st, def, table, 0, 1, nil, nil) }, func() (Stack, error) { return LifeRerollAffixValue(st, 0, 1, nil) }, func() (Stack, error) { return LifeShuffleAffixes(st, nil) }, func() (Stack, error) { return LifeTemperAffix(st, nil) }} {
		got, err := call()
		if err != nil || got != st {
			t.Fatal("preflight changed input", err)
		}
	}
}
func TestLifeSecretAndNegativeTemper(t *testing.T) {
	st, _, _ := lifeFixture(t)
	for i, a := range st.Legendary.Affixes {
		if a.Affix.Attr == AttrAtkSpeed {
			st.Legendary.TemperedSlot = uint8(i + 1)
			boost, _ := st.Legendary.EffectiveAffix(i)
			if boost.Affix.Value >= a.Affix.Value {
				t.Fatal("negative bonus weakened")
			}
		}
	}
	st.Legendary.TemperedSlot = 0
	st.Legendary.Tier = LegendaryTierSecret
	for i, a := range st.Legendary.Affixes {
		st.Legendary.Bounds[i] = LegendaryAffixBounds{a.Affix.Value, a.Affix.Value}
	}
	if _, err := LifeTemperAffix(st, nil); err == nil {
		t.Fatal("fixed stats tempered")
	}
	if _, err := LifeRerollAffixValue(st, 0, 7, nil); err == nil {
		t.Fatal("fixed stats washed")
	}
}

func TestLifeCardBiasHigherLevelIncreasesSelection(t *testing.T) {
	st, def, table := lifeFixture(t)
	table.Options[EquipmentRollKey{3, 1}] = append(table.Options[EquipmentRollKey{3, 1}], EquipmentRollOption{CardID: 99, Affix: Affix{Attr: AttrDEX, Value: 20}, Weight: 1})
	counts := []int{}
	for _, level := range []int32{1, 10, 50} {
		cardDef := ItemDef{Level: level, SocketAffixes: []Affix{{Attr: AttrDEX, Value: 1}}}
		bias := LifeCardBias(cardDef, Stack{})
		r := rand.New(rand.NewSource(2))
		count := 0
		for i := 0; i < 1000; i++ {
			got, err := LifeRerollAffix(st, def, table, 0, 1, bias, r.Intn)
			if err != nil {
				t.Fatal(err)
			}
			if got.Legendary.Affixes[0].Affix.Attr == AttrDEX {
				count++
			}
		}
		counts = append(counts, count)
	}
	if counts[0] >= counts[1] || counts[1] >= counts[2] {
		t.Fatal("card bias does not improve", counts)
	}
	owned := Stack{Card: CardInstanceState{Initialized: true, Count: 1, Effects: [8]CardEffect{{Op: 1, Attr: AttrINT, Probability: 100, Value: 5}}}}
	bias := LifeCardBias(ItemDef{Level: 10, SocketAffixes: []Affix{{Attr: AttrDEX, Value: 1}}}, owned)
	if bias[AttrINT] != 10 || bias[AttrDEX] != 0 {
		t.Fatal("owned effects not authoritative")
	}
}

func TestLifeSecretTypeWashKeepsFixedMaximum(t *testing.T) {
	st, def, table := lifeFixture(t)
	st.Legendary.Tier = LegendaryTierSecret
	for i, a := range st.Legendary.Affixes {
		st.Legendary.Bounds[i] = LegendaryAffixBounds{a.Affix.Value, a.Affix.Value}
	}
	table.Options[EquipmentRollKey{3, 1}] = append(table.Options[EquipmentRollKey{3, 1}], EquipmentRollOption{CardID: 99, Affix: Affix{Attr: AttrDEX, Value: 20}, Weight: 1})
	for slot := 0; slot < 4; slot++ {
		got, err := LifeRerollAffix(st, def, table, slot, 1, nil, func(int) int { return 0 })
		if err != nil {
			t.Fatal(err)
		}
		if got.Legendary.Tier != LegendaryTierSecret || got.Legendary.Bounds[slot].Min != got.Legendary.Bounds[slot].Max || got.Legendary.Validate() != nil {
			t.Fatal("secret invariant broken")
		}
	}
}

func TestLifeTrueWashRetainsTierAndTemper(t *testing.T) {
	st, def, table := lifeFixture(t)
	st.Legendary.Tier = LegendaryTierTrue
	for i, b := range st.Legendary.Bounds {
		st.Legendary.Bounds[i] = LegendaryAffixBounds{b.Min * 3 / 2, b.Max * 3 / 2}
		st.Legendary.Affixes[i].Affix.Value = b.Min * 3 / 2
	}
	st.Legendary.TemperedSlot = 1
	got, err := LifeRerollAffix(st, def, table, 0, 7, nil, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if got.Legendary.Tier != LegendaryTierTrue || got.Legendary.TemperedSlot != 1 || got.Legendary.Bounds[0].Min != 45 || got.Legendary.Bounds[0].Max != 90 {
		t.Fatal("true scaling/temper lost", got.Legendary)
	}
}
func TestLifeTemperRejectsOverflowAndInvalidMarker(t *testing.T) {
	st, _, _ := lifeFixture(t)
	st.Legendary.Bounds[0] = LegendaryAffixBounds{1000000000, 2000000000}
	st.Legendary.Affixes[0].Affix.Value = 1000000000
	got, err := LifeTemperAffix(st, func(int) int { return 0 })
	if err == nil || got != st {
		t.Fatal("overflow accepted or input changed")
	}
	st.Legendary.TemperedSlot = 7
	if st.Legendary.Validate() == nil {
		t.Fatal("invalid marker accepted")
	}
}

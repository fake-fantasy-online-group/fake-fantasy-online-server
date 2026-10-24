package domain

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func rollLegendaryTierFixture(t *testing.T, tierDraw int, high bool) (ItemDef, EquipmentRollTable, Stack) {
	t.Helper()
	def, table := legendaryRollFixture(t)
	st := Stack{UID: 7, Item: def.ID, Count: 1}
	calls := 0
	if !table.ApplyLegendary(&st, def, func(n int) int {
		calls++
		if calls == 14 {
			if n != 100 {
				t.Fatal("tier is not one 100-way draw", n)
			}
			return tierDraw
		}
		if high {
			return n - 1
		}
		return 0
	}) {
		t.Fatal("tier generation failed", tierDraw)
	}
	return def, table, st
}

func TestLegendaryTierExactMutuallyExclusiveDistribution(t *testing.T) {
	counts := map[LegendaryTier]int{}
	for n := 0; n < 100; n++ {
		_, _, st := rollLegendaryTierFixture(t, n, false)
		counts[st.Legendary.Tier]++
		want := LegendaryTierNormal
		if n == 0 {
			want = LegendaryTierSecret
		} else if n <= 10 {
			want = LegendaryTierTrue
		}
		if st.Legendary.Tier != want {
			t.Fatal(n, st.Legendary.Tier, want)
		}
	}
	if !reflect.DeepEqual(counts, map[LegendaryTier]int{LegendaryTierNormal: 89, LegendaryTierTrue: 10, LegendaryTierSecret: 1}) {
		t.Fatal(counts)
	}
}

func TestLegendaryTierAffixBoundsUnitsEndpointsAndReforge(t *testing.T) {
	for _, draw := range []int{0, 1, 10, 11, 99} {
		for _, high := range []bool{false, true} {
			def, table, st := rollLegendaryTierFixture(t, draw, high)
			old := Stack{UID: 7, Item: def.ID, Count: 1}
			table.ApplyLegendaryLegacy(&old, def, func(n int) int {
				if high {
					return n - 1
				}
				return 0
			})
			if st.Legendary.PowerID != old.Legendary.PowerID || NaturalAttributeCount(def, st) != 6 || st.SocketCount != 0 || st.RolledAffixCount != 0 {
				t.Fatal("power/affix budget changed")
			}
			for i, a := range st.Legendary.Affixes {
				b := old.Legendary.Bounds[i]
				lo, hi := b.Min, b.Max
				if draw <= 10 {
					lo = int32(int64(lo) * 3 / 2)
					hi = int32(int64(hi) * 3 / 2)
				}
				if draw == 0 {
					if old.Legendary.Affixes[i].Affix.Value < 0 {
						hi = lo
					}
					lo = hi
				}
				want := lo
				if high {
					want = hi
				}
				if a.Affix.Value != want || st.Legendary.Bounds[i] != (LegendaryAffixBounds{lo, hi}) || a.CardID != old.Legendary.Affixes[i].CardID || a.Affix.Attr != old.Legendary.Affixes[i].Affix.Attr || a.Affix.Mode != old.Legendary.Affixes[i].Affix.Mode {
					t.Fatal("scaled endpoint or typed identity", draw, i, a, lo, hi)
				}
			}
			saved := st.Legendary
			changed := st.Legendary.Reforge(func(n int) int { return n - 1 })
			if draw == 0 && (changed || st.Legendary != saved) {
				t.Fatal("secret locked affixes rerolled")
			}
			if st.Legendary.Bounds != saved.Bounds || st.Legendary.Tier != saved.Tier || st.Legendary.PowerID != saved.PowerID {
				t.Fatal("reforge changed identity")
			}
		}
	}
}

func TestLegendaryTierPersistenceLegacyAndInvalidAtomicity(t *testing.T) {
	for _, draw := range []int{0, 1, 11} {
		def, table, st := rollLegendaryTierFixture(t, draw, false)
		raw, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		var loaded Stack
		if err = json.Unmarshal(raw, &loaded); err != nil || loaded != st {
			t.Fatal("round trip", err)
		}
		before := loaded
		if table.ApplyLegendary(&loaded, def, func(int) int { t.Fatal("owned item rerolled"); return 0 }) || loaded != before {
			t.Fatal("existing item upgraded")
		}
		if draw == 11 && strings.Contains(string(raw), "Tier") {
			t.Fatal("ordinary changes legacy JSON shape")
		}
	}
	def, table := legendaryRollFixture(t)
	for _, bad := range []int{-1, 100} {
		st := Stack{UID: 1, Item: def.ID, Count: 1}
		before := st
		calls := 0
		if table.ApplyLegendary(&st, def, func(n int) int {
			calls++
			if calls == 14 {
				return bad
			}
			return 0
		}) || st != before {
			t.Fatal("invalid tier RNG was not atomic")
		}
	}
	_, _, st := rollLegendaryTierFixture(t, 11, false)
	st.Legendary.Tier = "unrecognized"
	if st.Legendary.Validate() == nil {
		t.Fatal("unknown persisted tier accepted")
	}
	st.Legendary.Tier = LegendaryTierSecret
	if st.Legendary.Validate() == nil {
		t.Fatal("unlocked secret accepted")
	}
	// A source too large to scale must fail, never wrap a beneficial value negative.
	table.Options[EquipmentRollKey{3, 1}][0].Affix.Value = math.MaxInt32 / 6
	st = Stack{UID: 1, Item: def.ID, Count: 1}
	before := st
	if table.ApplyLegendary(&st, def, func(int) int { return 0 }) || st != before {
		t.Fatal("overflow mutation")
	}
}

func TestLegendaryTierIntrinsicAggregationDoesNotScaleAffixesRefineOrPowers(t *testing.T) {
	def, _, st := rollLegendaryTierFixture(t, 11, false)
	def.Equip.Base = Base{STR: 2, VIT: 4, INT: 6, SPI: 8, AGI: 10, DEX: 12}
	def.Equip.Bonus = Stats{MinAtk: 20, MaxAtk: 30, Def: 14, Hit: 16, MAtk: 18, MDef: 22, MaxWeight: 24}
	def.Equip.RefineEffects = map[int32]RefineEffect{1: {Bonuses: []Affix{{Attr: AttrMinAtk, Value: 3}, {Attr: AttrSTR, Value: 1}}}}
	st.RefineLevel = 1
	defs := func(id ItemID) (ItemDef, bool) { return def, id == def.ID }
	worn := NewEquipSet()
	worn.Set(SlotWeapon, st)
	base, stats := Compute(StatSource{Level: 100, Worn: worn, Defs: defs})
	originalDef := *def.Equip
	original := st
	for _, tier := range []LegendaryTier{LegendaryTierTrue, LegendaryTierSecret} {
		st = original
		st.Legendary.Tier = tier
		if tier == LegendaryTierSecret {
			for i, a := range st.Legendary.Affixes {
				st.Legendary.Bounds[i] = LegendaryAffixBounds{a.Affix.Value, a.Affix.Value}
			}
		}
		multiplier := int32(2)
		if tier == LegendaryTierSecret {
			multiplier = 3
		}
		b, s := LegendaryIntrinsic(def, st)
		if b.STR != 2*multiplier+1 || s.MinAtk != 20*multiplier+3 || s.MaxAtk != 30*multiplier {
			t.Fatal("intrinsic/refinement scope", b, s)
		}
		worn.Set(SlotWeapon, st)
		actualBase, actualStats := Compute(StatSource{Level: 100, Worn: worn, Defs: defs})
		wantBase := base.Add(Base{STR: 2 * (multiplier - 1), VIT: 4 * (multiplier - 1), INT: 6 * (multiplier - 1), SPI: 8 * (multiplier - 1), AGI: 10 * (multiplier - 1), DEX: 12 * (multiplier - 1)})
		wantStats := addStats(stats, subStats(DeriveFromBase(wantBase), DeriveFromBase(base)))
		for i := int32(1); i < multiplier; i++ {
			wantStats = addStats(wantStats, def.Equip.Bonus)
		}
		if actualBase != wantBase || actualStats != wantStats {
			t.Fatalf("real aggregation tier=%s got %+v/%+v want %+v/%+v", tier, actualBase, actualStats, wantBase, wantStats)
		}
		if !reflect.DeepEqual(*def.Equip, originalDef) || st.Legendary.PowerID != original.Legendary.PowerID {
			t.Fatal("template or power changed")
		}
	}
}

func TestLegendaryTierLiteralPrefixAndDungeonNamesIdempotent(t *testing.T) {
	p := LegendaryPowerDefinition{Name: "双焰"}
	for _, tc := range []struct {
		tier LegendaryTier
		want string
	}{{LegendaryTierNormal, "双焰之剑"}, {LegendaryTierTrue, "真*双焰之剑"}, {LegendaryTierSecret, "秘*双焰之剑"}} {
		for i := 0; i < 3; i++ {
			if got := LegendaryInstanceDisplayName("剑", p, LegendaryInstance{Tier: tc.tier}); got != tc.want {
				t.Fatal(got, tc.want)
			}
		}
	}
	for _, name := range []string{"剑", "真*剑"} {
		if got := prefixedEquipmentName("真*", name); got != "真*剑" {
			t.Fatal("double star", got)
		}
	}
}

func TestLegendaryTierSecretCubePreviewRejectsLockedValues(t *testing.T) {
	def, table, secret := rollLegendaryTierFixture(t, 0, false)
	ctx, req := cubeInputs(CubeReforgeUnique)
	old := ctx.Items[def.ID]
	def.EndgameCatalog = old.EndgameCatalog
	def.InventoryTab = old.InventoryTab
	def.InventoryTabKnown = true
	def.InstanceKind = ItemInstanceEquipment
	def.Equip.Durable = 100
	ctx.Items[def.ID] = def
	ctx.EquipmentRoll = table
	secret.UID = 42
	secret.InstanceKind = ItemInstanceEquipment
	secret.Durability = 100
	secret.MaxDurability = 100
	ctx.Bag.Set(200, secret)
	before := ctx.Bag.Clone()
	for _, execute := range []bool{false, true} {
		for _, recipe := range []int32{CubeReforgeUnique, CubeTemperUnique} {
			req.RecipeID = recipe
			if _, err := PlanCube(ctx, req, execute); err == nil {
				t.Fatal("locked secret action accepted", recipe, execute)
			}
			if !reflect.DeepEqual(ctx.Bag, before) {
				t.Fatal("failed action consumed ingredients")
			}
		}
	}
}

package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func requireAttackStats(t *testing.T, got Stats, min, max, magic int32) {
	t.Helper()
	if got.MinAtk != min || got.MaxAtk != max || got.MAtk != magic {
		t.Fatalf("attack got %d/%d/%d, want %d/%d/%d", got.MinAtk, got.MaxAtk, got.MAtk, min, max, magic)
	}
}

func TestWeaponAttackPercentEveryItemSourceStaysLocal(t *testing.T) {
	for _, slot := range []EquipSlot{SlotWeapon, SlotTwoHand} {
		for _, source := range []string{"fixed", "rolled", "socket", "wash", "appearance", "soul", "dragon"} {
			t.Run(fmt.Sprintf("%d/%s", slot, source), func(t *testing.T) {
				affixes := []Affix{{Attr: AttrAtk, Value: 50, Mode: ModePercent}, {Attr: AttrMAtk, Value: 75, Mode: ModePercent}}
				weapon := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(slot), Base: Base{STR: 20, INT: 10},
					Bonus:         Stats{MinAtk: 11, MaxAtk: 23, MAtk: 17},
					Affixes:       []Affix{{Attr: AttrAtk, Value: 7}, {Attr: AttrMAtk, Value: 9}},
					RefineEffects: map[int32]RefineEffect{1: {Bonuses: []Affix{{Attr: AttrMinAtk, Value: 100}, {Attr: AttrMaxAtk, Value: 200}, {Attr: AttrMAtk, Value: 300}}}},
				}}
				armor := ItemDef{ID: 99002, Equip: &EquipDef{Slot: int32(SlotBody), Bonus: Stats{MinAtk: 101, MaxAtk: 203, MAtk: 307}}}
				shield := ItemDef{ID: 99003, Equip: &EquipDef{Slot: int32(SlotShield), Bonus: Stats{MinAtk: 13, MaxAtk: 19, MAtk: 29}}}
				extra := ItemDef{ID: 99004}
				st := Stack{Item: weapon.ID, Count: 1, RefineLevel: 1}
				worn := NewEquipSet()
				worn.Set(slot, st)
				worn.Set(SlotBody, Stack{Item: armor.ID, Count: 1})
				if slot == SlotWeapon {
					worn.Set(SlotShield, Stack{Item: shield.ID, Count: 1})
				}
				input := StatSource{Base: Base{STR: 200, INT: 400}, Level: 100, Innate: Stats{MinAtk: 500, MaxAtk: 600, MAtk: 700}, Worn: worn, Defs: defsOf(weapon, armor, shield)}
				baselineBase, baseline := Compute(input)
				switch source {
				case "fixed":
					weapon.Equip.Affixes = append(weapon.Equip.Affixes, affixes...)
				case "rolled":
					st.RolledAffixCount = uint8(len(affixes))
					for i, a := range affixes {
						st.RolledAffixes[i] = InstanceAffix{Affix: a}
					}
				case "socket":
					st.SocketCount, st.Sockets[0] = 1, 99999
					st.SocketCards[0] = CardInstanceState{Initialized: true, Count: uint8(len(affixes))}
					for i, a := range affixes {
						st.SocketCards[0].Effects[i] = CardEffect{Op: 1, Probability: 100, Attr: a.Attr, Mode: a.Mode, Value: a.Value}
					}
				case "wash":
					st.WashCount = uint8(len(affixes))
					copy(st.WashAffixes[:], affixes)
				case "appearance":
					extra.AvatarFusion = &AvatarFusionDef{Appearance: EquipAppearance{Part: AppearanceWeaponR}, Affixes: affixes}
					st.FusedAppearance = extra.ID
				case "soul":
					extra.EquipmentSoul = &EquipmentSoulDef{TargetSlot: SlotWeapon, Affixes: affixes}
					st.FusedSoul = extra.ID
				case "dragon":
					extra.DragonFusion = &EquipmentSoulDef{TargetSlot: SlotWeapon, Affixes: affixes}
					st.FusedDragon = extra.ID
				}
				input.Defs = defsOf(weapon, armor, shield, extra)
				worn.Set(slot, st)
				before, _ := json.Marshal(st)
				gotBase, got := Compute(input)
				want := addStats(baseline, Stats{MinAtk: 5, MaxAtk: 11, MAtk: 12})
				if gotBase != baselineBase || got != want {
					t.Fatalf("%s amplified other contributions: got %+v/%+v, want %+v/%+v", source, gotBase, got, baselineBase, want)
				}
				if after, _ := json.Marshal(st); string(after) != string(before) {
					t.Fatal("compute mutated item")
				}
			})
		}
	}
}

func TestWeaponAttackPercentBoundsAndAdditiveRounding(t *testing.T) {
	for _, tc := range []struct {
		name            string
		affixes         []Affix
		min, max, magic int32
	}{
		{"paired", []Affix{{Attr: AttrMinAtk, Value: 30, Mode: ModePercent}, {Attr: AttrMaxAtk, Value: 50, Mode: ModePercent}}, 14, 34, 17},
		{"min_only", []Affix{{Attr: AttrMinAtk, Value: 100, Mode: ModePercent}}, 22, 23, 17},
		{"max_only", []Affix{{Attr: AttrMaxAtk, Value: 100, Mode: ModePercent}}, 11, 46, 17},
		{"combined", []Affix{{Attr: AttrAtk, Value: 50, Mode: ModePercent}, {Attr: AttrMinAtk, Value: 50, Mode: ModePercent}, {Attr: AttrMaxAtk, Value: 50, Mode: ModePercent}}, 22, 46, 17},
		{"negative", []Affix{{Attr: AttrAtk, Value: -50, Mode: ModePercent}, {Attr: AttrMAtk, Value: -50, Mode: ModePercent}}, 6, 12, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Bonus: Stats{MinAtk: 11, MaxAtk: 23, MAtk: 17}, Affixes: tc.affixes}}
			_, got := Compute(StatSource{Worn: wearing(SlotWeapon, def), Defs: defsOf(def)})
			requireAttackStats(t, got, tc.min, tc.max, tc.magic)
		})
	}
	// Item-local percentages cannot invent attack on a zero-base weapon.
	def := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Affixes: []Affix{{Attr: AttrAtk, Value: 1000, Mode: ModePercent}, {Attr: AttrMAtk, Value: 1000, Mode: ModePercent}}}}
	_, zero := Compute(StatSource{Base: Base{STR: 200, INT: 400}, Worn: wearing(SlotWeapon, def), Defs: defsOf(def)})
	requireAttackStats(t, zero, 100, 100, 200)
	var extreme weaponAttackPercent
	extreme.add(def.Equip, Affix{Attr: AttrAtk, Value: math.MaxInt32, Mode: ModePercent})
	extreme.add(def.Equip, Affix{Attr: AttrAtk, Value: math.MaxInt32, Mode: ModePercent})
	def.Equip.Bonus = Stats{MinAtk: math.MaxInt32, MaxAtk: math.MaxInt32}
	requireAttackStats(t, extreme.bonuses(def, Stack{}), math.MaxInt32, math.MaxInt32, 0)
}

func TestWeaponAttackPercentLegendaryTiersUseOwnUnrefinedBase(t *testing.T) {
	def, _, original := rollLegendaryTierFixture(t, 11, false)
	def.Equip.Base = Base{STR: 20, INT: 10}
	def.Equip.Bonus = Stats{MinAtk: 11, MaxAtk: 23, MAtk: 17}
	def.Equip.Affixes = []Affix{{Attr: AttrAtk, Value: 999, Mode: ModePercent}} // replaced on legendary items
	def.Equip.RefineEffects = map[int32]RefineEffect{1: {Bonuses: []Affix{{Attr: AttrMinAtk, Value: 100}, {Attr: AttrMaxAtk, Value: 200}, {Attr: AttrMAtk, Value: 300}}}}
	for _, tier := range []LegendaryTier{LegendaryTierNormal, LegendaryTierTrue, LegendaryTierSecret} {
		t.Run(string(tier), func(t *testing.T) {
			st := original
			st.RefineLevel, st.Legendary.Tier = 1, tier
			for i, a := range []Affix{{Attr: AttrAtk, Value: 40, Mode: ModePercent}, {Attr: AttrMAtk, Value: 60, Mode: ModePercent}, {Attr: AttrMinAtk, Value: 10, Mode: ModePercent}, {Attr: AttrMaxAtk, Value: 20, Mode: ModePercent}, {Attr: AttrSTR, Value: 4}, {Attr: AttrINT, Value: 6}} {
				st.Legendary.Affixes[i] = InstanceAffix{CardID: int32(i + 1), Affix: a}
				st.Legendary.Bounds[i] = LegendaryAffixBounds{Min: a.Value, Max: a.Value}
			}
			if err := st.Legendary.Validate(); err != nil {
				t.Fatal(err)
			}
			mult := int32(1)
			if tier == LegendaryTierTrue {
				mult = 2
			} else if tier == LegendaryTierSecret {
				mult = 3
			}
			raw, _ := json.Marshal(st)
			var loaded Stack
			if err := json.Unmarshal(raw, &loaded); err != nil {
				t.Fatal(err)
			}
			for _, saved := range []Stack{st, loaded} {
				worn := NewEquipSet()
				worn.Set(SlotWeapon, saved)
				base := Base{STR: 200, INT: 400}
				gotBase, got := Compute(StatSource{Base: base, Level: 100, Worn: worn, Defs: defsOf(def)})
				wantBase := base.Add(Base{STR: 20*mult + 4, INT: 10*mult + 6})
				if gotBase != wantBase {
					t.Fatal("weapon percentage amplified primary attributes", gotBase, wantBase)
				}
				derived := DeriveFromBase(wantBase)
				requireAttackStats(t, got, derived.MinAtk+11*mult+100+11*mult*50/100, derived.MaxAtk+23*mult+200+23*mult*60/100, derived.MAtk+17*mult+300+17*mult*60/100)
				if saved != st {
					t.Fatal("read/reload changed persisted percentages or tier")
				}
				worn.Set(SlotWeapon, Stack{})
				_, bare := Compute(StatSource{Base: base, Level: 100, Worn: worn, Defs: defsOf(def)})
				if bare != DeriveFromBase(base) {
					t.Fatal("unequip retained local contribution")
				}
			}
		})
	}
}

func TestWeaponAttackPercentLegendaryRollRangesStayUnchanged(t *testing.T) {
	def, table := legendaryRollFixture(t)
	key := EquipmentRollKey{Level: 3, Type: 1}
	table.Options[key][0].Affix = Affix{Attr: AttrAtk, Value: 10, Mode: ModePercent}
	table.Options[key][1].Affix = Affix{Attr: AttrMAtk, Value: 20, Mode: ModePercent}
	def.Equip.Bonus = Stats{MinAtk: 100, MaxAtk: 200, MAtk: 300}
	for _, draw := range []int{0, 1, 11} {
		for _, high := range []bool{false, true} {
			st := Stack{UID: 7, Item: def.ID, Count: 1}
			calls := 0
			if !table.ApplyLegendary(&st, def, func(n int) int {
				calls++
				if calls == 14 {
					return draw
				}
				if high {
					return n - 1
				}
				return 0
			}) {
				t.Fatal("percentage legendary generation failed")
			}
			found := 0
			for i, a := range st.Legendary.Affixes {
				if a.Affix.Attr != AttrAtk && a.Affix.Attr != AttrMAtk {
					continue
				}
				found++
				lo, hi := int32(30), int32(60) // existing normal 3x..6x percentage range
				if a.Affix.Attr == AttrMAtk {
					lo, hi = 60, 120
				}
				if draw <= 10 {
					lo, hi = lo*3/2, hi*3/2
				}
				if draw == 0 {
					lo = hi
				}
				want := lo
				if high {
					want = hi
				}
				if a.Affix.Mode != ModePercent || a.Affix.Value != want || st.Legendary.Bounds[i] != (LegendaryAffixBounds{Min: lo, Max: hi}) {
					t.Fatalf("percent roll changed: draw%d got %+v/%+v, want %d in %d..%d", draw, a, st.Legendary.Bounds[i], want, lo, hi)
				}
			}
			if found != 2 {
				t.Fatal("physical/magic percentage affix lost")
			}
			before := st
			worn := NewEquipSet()
			worn.Set(SlotWeapon, st)
			Compute(StatSource{Level: 100, Worn: worn, Defs: defsOf(def)})
			if st != before {
				t.Fatal("computation changed percentage rolls")
			}
		}
	}
}

func TestWeaponAttackPercentMixedWeaponSwapCannotBorrowOtherItemBase(t *testing.T) {
	physical := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Bonus: Stats{MinAtk: 100, MaxAtk: 200}, Affixes: []Affix{{Attr: AttrAtk, Value: 50, Mode: ModePercent}, {Attr: AttrMAtk, Value: 75, Mode: ModePercent}}}}
	magic := ItemDef{ID: 99002, Equip: &EquipDef{Slot: int32(SlotWeapon), Bonus: Stats{MAtk: 300}, Affixes: physical.Equip.Affixes}}
	shield := ItemDef{ID: 99003, Equip: &EquipDef{Slot: int32(SlotShield), Bonus: Stats{MinAtk: 40, MaxAtk: 60, MAtk: 80}}}
	worn := wearing(SlotShield, shield)
	input := StatSource{Base: Base{STR: 200, INT: 400}, Worn: worn, Defs: defsOf(physical, magic, shield)}
	worn.Set(SlotWeapon, Stack{Item: physical.ID, Count: 1})
	_, first := Compute(input)
	requireAttackStats(t, first, 290, 460, 280)
	worn.Set(SlotWeapon, Stack{Item: magic.ID, Count: 1})
	_, second := Compute(input)
	requireAttackStats(t, second, 140, 160, 805)
	worn.Set(SlotWeapon, Stack{})
	_, bare := Compute(input)
	requireAttackStats(t, bare, 140, 160, 280)
}

func TestWeaponAttackPercentPreservesGlobalSuitPassiveStatusAndArmor(t *testing.T) {
	weapon := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Bonus: Stats{MinAtk: 100, MaxAtk: 200, MAtk: 300}, Affixes: []Affix{{Attr: AttrAtk, Value: 50, Mode: ModePercent}, {Attr: AttrMAtk, Value: 50, Mode: ModePercent}}}}
	armor := ItemDef{ID: 99002, Equip: &EquipDef{Slot: int32(SlotBody), Affixes: []Affix{{Attr: AttrAtk, Value: 10, Mode: ModePercent}, {Attr: AttrMAtk, Value: 10, Mode: ModePercent}}}}
	weapon.Equip.Suit = &EquipmentSuit{ID: 1, Groups: [][]ItemID{{weapon.ID}}, Bonuses: []EquipmentSuitBonus{{Pieces: 1, Affix: Affix{Attr: AttrAtk, Value: 20, Mode: ModePercent}}, {Pieces: 1, Affix: Affix{Attr: AttrMAtk, Value: 20, Mode: ModePercent}}}}
	worn := wearing(SlotWeapon, weapon)
	worn.Set(SlotBody, Stack{Item: armor.ID, Count: 1})
	statuses := NewStatusSet()
	statuses.Apply(StatusDef{ID: 99001, DurationSec: 60, Affixes: []Affix{{Attr: AttrAtk, Value: 25, Mode: ModePercent}, {Attr: AttrMAtk, Value: 25, Mode: ModePercent}, {Attr: AttrAtk, Value: 7}, {Attr: AttrMAtk, Value: 9}, {Attr: AttrSTR, Value: 20}, {Attr: AttrINT, Value: 40}}}, 0, 1)
	_, got := Compute(StatSource{Base: Base{STR: 200, INT: 400}, Worn: worn, Defs: defsOf(weapon, armor), Passive: []Affix{{Attr: AttrAtk, Value: 30, Mode: ModePercent}, {Attr: AttrMAtk, Value: 30, Mode: ModePercent}}, Status: statuses})
	// Local baseline 250/400/650, then unchanged global +60%, then status
	// +25% of that baseline. Flat status and its primary stats stay independent.
	requireAttackStats(t, got, 400+100+7+10, 640+160+7+10, 1040+260+9+20)
}

func TestWeaponAttackPercentFormationResolvesBeforePassivePipe(t *testing.T) {
	def := ItemDef{ID: 99001, Equip: &EquipDef{Slot: int32(SlotWeapon), Bonus: Stats{MinAtk: 11, MaxAtk: 23, MAtk: 17}}}
	st := Stack{Item: def.ID, Count: 1, Endgame: EndgameItemState{FormationRollBP: 12000}}
	r := AstralRecipe{Affixes: []Affix{{Attr: AttrAtk, Value: 50, Mode: ModePercent}, {Attr: AttrMAtk, Value: 75, Mode: ModePercent}, {Attr: AttrSTR, Value: 20}}, LocalEnhancements: []ItemLocalEnhancement{{Attr: AttrAtk, Percent: 100}}}
	before := append([]Affix(nil), r.Affixes...)
	bonuses := AstralItemBonuses(def, st, r)
	_, got := Compute(StatSource{Base: Base{STR: 200, INT: 400}, Worn: wearing(SlotWeapon, def), Defs: defsOf(def), Passive: bonuses})
	requireAttackStats(t, got, 110+11+5+11, 110+23+11+23, 200+17+12)
	// Formation activation and ordinary item attributes keep independent
	// rounding, just as explicit LocalEnhancements do: 11*50% twice is 5+5.
	def.Equip.Affixes = []Affix{{Attr: AttrAtk, Value: 50, Mode: ModePercent}}
	_, combined := Compute(StatSource{Base: Base{STR: 200, INT: 400}, Worn: wearing(SlotWeapon, def), Defs: defsOf(def), Passive: bonuses})
	requireAttackStats(t, combined, 110+11+5+5+11, 110+23+11+11+23, 200+17+12)
	if !reflect.DeepEqual(r.Affixes, before) {
		t.Fatal("formation authored percentages mutated")
	}
}

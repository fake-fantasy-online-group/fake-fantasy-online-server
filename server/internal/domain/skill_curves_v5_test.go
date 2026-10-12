package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"reflect"
	"testing"
)

// Synthetic reference payloads deliberately do not read v5SkillCurves. They
// test total evaluation/immutability independently of the authored table. The
// focused regressions below use the real original effect chains and conditions.
func curveAnchors(id SkillID) (SkillDef, SkillDef) {
	a := SkillDef{ID: id, Level: 1, Name: "original", Description: "original description", Prof: 1, Kind: SkillSingle, TargetEnemy: true, HurtType: HurtPhysical, Dist: 100, MPCost: 10,
		Effect:   SkillEffect{Kind: EffectDamage, DamagePct: 100},
		Statuses: []StatusApplication{{ID: StatusStun, Level: 1, ChanceBP: 2000, DurationSec: 5, ExtraAffixes: []Affix{{Attr: AttrDef, Value: 10, Mode: ModePercent}}}},
	}
	b := a
	b.Level = 5
	b.MPCost = 30
	b.Statuses = []StatusApplication{{ID: StatusStun, Level: 5, ChanceBP: 4000, DurationSec: 9, ExtraAffixes: []Affix{{Attr: AttrDef, Value: 20, Mode: ModePercent}}}}
	switch id {
	case 10301, 10304:
		a.Effect = SkillEffect{Kind: EffectHeal, HealFlat: 100, HealPctOfMax: 10}
		b.Effect = SkillEffect{Kind: EffectHeal, HealFlat: 200, HealPctOfMax: 30}
	case 10302:
		a.Effect = SkillEffect{Kind: EffectRestoreMP, RestoreMPFlat: 100}
		b.Effect = SkillEffect{Kind: EffectRestoreMP, RestoreMPFlat: 500}
	case 10028, 10127:
		a.Effect = SkillEffect{Kind: EffectTaunt, TauntPower: 1}
		b.Effect = SkillEffect{Kind: EffectTaunt, TauntPower: 5}
	}
	return a, b
}

// Count only unbounded fields actually sent to combat/stat/status consumers.
// Level, UI metadata, bounded probabilities, durations and geometry do not count.
func curveOperationalCarrier(d SkillDef) int64 {
	n := int64(d.ProgressionShieldFlat)
	for _, fx := range d.DirectEffects() {
		n += int64(fx.DamageFlat) + int64(fx.HealFlat) + int64(fx.RestoreMPFlat) + int64(fx.TauntPower)
	}
	for _, m := range d.PassiveModifiers {
		if m.Kind == PassiveSkillDamageFlat || m.Kind == PassiveEffectRestoreFlat {
			n += int64(m.Value)
		}
	}
	for _, m := range d.PassiveStats {
		if m.Mode == ModeAbsolute {
			n += int64(m.Value)
		}
	}
	for _, m := range d.PassiveStatusAffixes {
		if m.Affix.Mode == ModeAbsolute {
			n += int64(m.Affix.Value)
		}
	}
	for _, s := range d.Statuses {
		n += int64(s.ProgressionDamageShield) + int64(s.ProgressionReflectFlat) + int64(s.ProgressionAttackFlat)
		hp := int64(s.ProgressionHPPerTick)
		if hp < 0 {
			hp = -hp
		}
		n += hp
		for _, a := range append(append([]Affix(nil), s.ExtraAffixes...), s.ProgressionAffixes...) {
			if a.Mode == ModeAbsolute {
				n += int64(a.Value)
			}
		}
	}
	return n
}

func TestV5CurveCatalogueExactly149AndEveryRankImprovesOperationalCarrier(t *testing.T) {
	ids := V5SkillCurveIDs()
	if len(ids) != 149 {
		t.Fatalf("authored identities=%d, want149", len(ids))
	}
	expected := []SkillID{}
	for _, start := range []SkillID{10001, 10101, 10201} {
		for id := start; id < start+30; id++ {
			expected = append(expected, id)
		}
	}
	for id := SkillID(10301); id <= 10330; id++ {
		if id != 10323 {
			expected = append(expected, id)
		}
	}
	expected = append(expected, 10401, 10402, 10405, 10408, 10409, 10410, 10411, 10412, 10413, 10414, 10415, 10416, 10417, 10418, 10419, 10420, 10421, 10422, 10423, 10424, 10425, 10426, 10427, 10428, 10429, 10430, 10431, 10432, 10433, 10435)
	if !reflect.DeepEqual(ids, expected) {
		t.Fatalf("catalogue differs from 149 class battle identities")
	}
	for _, id := range []SkillID{0, 10000, 10323, 10403, 11005, SkillFishing, SkillMining, SkillBeginnerBolt, SkillBeginnerGuard} {
		if HasV5SkillCurve(id) {
			t.Errorf("excluded identity %d was admitted", id)
		}
	}
	ids[0] = 0
	if V5SkillCurveIDs()[0] == 0 {
		t.Fatal("catalogue leaked mutable backing slice")
	}
	ranks := []int32{255, 256, 65535, 65536, 1000000}
	for r := int32(1); r <= 101; r++ {
		ranks = append(ranks, r)
	}
	for _, id := range expected {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			a, b := curveAnchors(id)
			for _, r := range ranks {
				x, err := EvaluateV5Skill(a, b, r)
				if err != nil {
					t.Fatal(err)
				}
				y, err := EvaluateV5Skill(a, b, r+1)
				if err != nil {
					t.Fatal(err)
				}
				if y.Level != r+1 || y.FormulaVersion != 5 {
					t.Fatalf("rank %d was clamped or version lost", r)
				}
				if curveOperationalCarrier(y) <= curveOperationalCarrier(x) {
					t.Fatalf("rank %d -> %d has no actual carrier growth: %d -> %d", r, r+1, curveOperationalCarrier(x), curveOperationalCarrier(y))
				}
			}
		})
	}
}

func TestV5CurveExactPriorityAnchorArithmetic(t *testing.T) {
	samples := []struct {
		id                  SkillID
		first, twenty, step int32
	}{
		{10002, 130, 150, 1}, {10021, 145, 165, 1}, {10022, 155, 175, 1}, {10102, 130, 142, 1}, {10104, 150, 170, 1}, {10121, 130, 150, 1}, {10209, 300, 380, 2}, {10213, 200, 240, 2}, {10216, 170, 200, 1}, {10319, 210, 270, 2}, {10401, 208, 240, 2}, {10402, 195, 215, 1}, {10409, 100, 120, 1}, {10412, 200, 232, 1},
	}
	for _, s := range samples {
		a, b := curveAnchors(s.id)
		for _, r := range []int32{1, 2, 19, 20, 21, 100, 101, 65536} {
			d, err := EvaluateV5Skill(a, b, r)
			if err != nil {
				t.Fatal(err)
			}
			fx := d.Effect
			want := new(big.Rat)
			if r <= 20 {
				want.SetFrac64(int64(s.first)*19+int64(s.twenty-s.first)*int64(r-1), 19)
			} else {
				want.SetInt64(int64(s.twenty) + int64(s.step)*int64(r-20))
			}
			actual := new(big.Rat).SetFrac64(fx.DamagePctNumerator, fx.DamagePctDenominator)
			if actual.Cmp(want) != 0 || fx.DamageFlat != r-1 {
				t.Fatalf("%d rank%d got %s +%d want %s +%d", s.id, r, actual, fx.DamageFlat, want, r-1)
			}
		}
	}
	// Independent published worked examples, attack=1000 before defenses.
	a, b := curveAnchors(10401)
	for r, want := range map[int32]int64{20: 2419, 21: 2440, 100: 4099, 101: 4120} {
		d, _ := EvaluateV5Skill(a, b, r)
		fx := d.Effect
		got := 1000*fx.DamagePctNumerator/(100*fx.DamagePctDenominator) + int64(fx.DamageFlat)
		if got != want {
			t.Fatalf("firebolt rank%d=%d want%d", r, got, want)
		}
	}
}

func TestV5CurveHealingStaysHealingAndRetainsFractions(t *testing.T) {
	for _, s := range []struct {
		id                         SkillID
		flat1, flat20, pct1, pct20 int32
	}{{10301, 100, 220, 10, 30}, {10304, 180, 340, 5, 25}} {
		a, b := curveAnchors(s.id)
		a.HurtType = HurtHoly
		b.HurtType = HurtHoly
		for _, r := range []int32{1, 2, 20, 21, 101, 65536} {
			d, err := EvaluateV5Skill(a, b, r)
			if err != nil {
				t.Fatal(err)
			}
			if d.HasDamageEffect() || d.Effect.Kind != EffectHeal {
				t.Fatalf("holy healing became damage: %+v", d.Effect)
			}
			fx := d.Effect
			f := new(big.Rat).SetFrac64(fx.HealFlatNumerator, fx.HealFlatDenominator)
			p := new(big.Rat).SetFrac64(fx.HealPctNumerator, fx.HealPctDenominator)
			wf, wp := new(big.Rat), new(big.Rat)
			if r <= 20 {
				wf.SetFrac64(int64(s.flat1)*19+int64(s.flat20-s.flat1)*int64(r-1), 19)
				wp.SetFrac64(int64(s.pct1)*19+int64(s.pct20-s.pct1)*int64(r-1), 19)
			} else {
				wf.SetInt64(int64(s.flat20) + 4*int64(r-20))
				wp.SetInt64(int64(s.pct20))
			}
			if f.Cmp(wf) != 0 || p.Cmp(wp) != 0 {
				t.Fatalf("heal%d rank%d wrong exact anchors", s.id, r)
			}
		}
	}
	a, b := curveAnchors(10301)
	d, _ := EvaluateV5Skill(a, b, 2)
	// For maxHP=19: exact 19*(10+20/19)/100 +100+120/19=108.415...,
	// versus flooring percentage and flat separately (108.09); store both fractions.
	if d.Effect.HealFlatNumerator%19 == 0 || d.Effect.HealPctNumerator%19 == 0 {
		t.Fatal("rank2 fractional healing was discarded")
	}
}

func TestV5CurveMaintainsRealOrderedEffectAndTrapIdentity(t *testing.T) {
	a := SkillDef{ID: 10104, Level: 1, Kind: SkillSingle, HurtType: HurtPhysical, TargetEnemy: true, RequiredWeaponType: 2,
		Effects: []SkillEffect{{Kind: EffectDamage, DamagePct: 150}, {Kind: EffectRestoreHP, Target: EffectTargetCaster, RequiresPreviousLanded: true, SourceAttackPct: 20}}}
	b := a
	b.Level = 5
	b.Effects = []SkillEffect{{Kind: EffectDamage, DamagePct: 170}, {Kind: EffectRestoreHP, Target: EffectTargetCaster, RequiresPreviousLanded: true, SourceAttackPct: 28}}
	d, err := EvaluateV5Skill(a, b, 101)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Effects) != 2 || d.Effects[1].Kind != EffectRestoreHP || d.Effects[1].Target != EffectTargetCaster || !d.Effects[1].RequiresPreviousLanded || d.Effects[1].SourceAttackPct != 28 || d.RequiredWeaponType != 2 {
		t.Fatalf("life-steal ordering/limits changed: %+v", d)
	}
	a = SkillDef{ID: 10216, Kind: SkillArea, HurtType: HurtPhysical, TargetEnemy: true, Effects: []SkillEffect{{Kind: EffectPlaceTrap, Target: EffectTargetCaster, TrapRadius: 100}, {Kind: EffectDamage, DamagePct: 170}}, Statuses: []StatusApplication{{ID: StatusFreeze, Level: 2}}}
	b = a
	b.Effects = append([]SkillEffect(nil), a.Effects...)
	b.Statuses = []StatusApplication{{ID: StatusFreeze, Level: 10}}
	d, err = EvaluateV5Skill(a, b, 101)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Effects) != 2 || d.Effects[0].Kind != EffectPlaceTrap || d.Effects[0].TrapRadius != 100 || d.Effects[0].Target != EffectTargetCaster || d.Effects[1].DamagePct != 281 || d.Effects[1].DamageFlat != 100 || d.Statuses[0].ID != StatusFreeze || d.Statuses[0].Level != 10 {
		t.Fatalf("freeze trap identity lost: %+v", d)
	}
	a, b = curveAnchors(10209)
	a.RequiresInvisible = true
	b.RequiresInvisible = true
	a.Effect.GuaranteedHit = true
	b.Effect.GuaranteedHit = true
	d, err = EvaluateV5Skill(a, b, 65536)
	if err != nil {
		t.Fatal(err)
	}
	if !d.RequiresInvisible || !d.Effect.GuaranteedHit {
		t.Fatal("backstab lost invisibility/guaranteed-hit conditions")
	}
	a, b = curveAnchors(10021)
	a.Area = SkillAreaTargeting{Shape: SkillAreaRectangle, Center: SkillAreaCenterCaster, Length: 250, Width: 80}
	b.Area = a.Area
	d, err = EvaluateV5Skill(a, b, 65536)
	if err != nil {
		t.Fatal(err)
	}
	if d.Area != a.Area || !d.Area.Valid() {
		t.Fatal("rectangle geometry was swapped or grew without bounds")
	}
}

func TestV5CurveMaintainsPassiveConditionsAndRepairsCappedMaintenance(t *testing.T) {
	a := SkillDef{ID: 10024, Kind: SkillPassive, PassiveModifiers: []PassiveModifier{{Kind: PassiveSkillDurabilityReductionPct, TargetSkill: 10003, Value: 25}}}
	b := a
	b.PassiveModifiers = []PassiveModifier{{Kind: PassiveSkillDurabilityReductionPct, TargetSkill: 10003, Value: 100}}
	x, err := EvaluateV5Skill(a, b, 20)
	if err != nil {
		t.Fatal(err)
	}
	y, err := EvaluateV5Skill(a, b, 21)
	if err != nil {
		t.Fatal(err)
	}
	if x.PassiveModifiers[0].Value != 100 || y.PassiveModifiers[0].Value != 100 || y.PassiveModifiers[1].Kind != PassiveSkillDamageFlat || y.PassiveModifiers[1].TargetSkill != 10003 || y.PassiveModifiers[1].Value != 20 {
		t.Fatal("maintenance cap lost identity or has no same-target carrier")
	}
	for _, s := range []struct {
		id          SkillID
		attr, equip int32
	}{{10106, AttrDef, 105}, {10202, AttrAtk, 8}, {10204, AttrAtk, 32}, {10222, AttrAtk, 32}, {10203, AttrAtk, 8}} {
		a, b = curveAnchors(s.id)
		a.Kind = SkillPassive
		b.Kind = SkillPassive
		d, err := EvaluateV5Skill(a, b, 101)
		if err != nil {
			t.Fatal(err)
		}
		m := d.PassiveStats[len(d.PassiveStats)-1]
		if m.Attr != s.attr || m.RequiredEquipType != s.equip || m.Mode != ModeAbsolute || m.Value != 100 {
			t.Fatalf("equipment condition lost on passive%d: %+v", s.id, m)
		}
	}
	a, b = curveAnchors(10207)
	d, _ := EvaluateV5Skill(a, b, 21)
	m := d.PassiveStatusAffixes[len(d.PassiveStatusAffixes)-1]
	if m.TargetSkill != 10208 || m.Status != StatusStealth || m.Affix != (Affix{Attr: AttrAtk, Value: 20, Mode: ModeAbsolute}) {
		t.Fatal("advanced stealth did not preserve its single target")
	}
}

func TestV5CurveUtilityBoundsAndControlPayload(t *testing.T) {
	for _, id := range []SkillID{10016, 10029, 10128, 10224, 10310, 10324, 10325, 10413, 10428, 10421} {
		a, b := curveAnchors(id)
		a.Effect = SkillEffect{}
		b.Effect = SkillEffect{}
		d, err := EvaluateV5Skill(a, b, 1)
		if err != nil {
			t.Fatal(err)
		}
		if d.HasDamageEffect() {
			t.Fatalf("control%d received unintended base attack at rank1", id)
		}
		d, err = EvaluateV5Skill(a, b, 21)
		if err != nil {
			t.Fatal(err)
		}
		if !d.HasDamageEffect() || d.Effect.DamagePctNumerator != 0 || d.Effect.DamagePctDenominator != 1 || d.Effect.DamageFlat != 20 || !d.Effect.GuaranteedHit {
			t.Fatalf("control%d flat carrier accidentally includes base attack: %+v", id, d.Effect)
		}
		if d.Statuses[0].ChanceBP != 4000 || d.Statuses[0].DurationSec != 9 {
			t.Fatal("control chance/duration escaped authored boundary")
		}
	}
	a := SkillDef{ID: 10305, TargetTeam: true, Effects: []SkillEffect{{Kind: EffectResurrect, HealPctOfMax: 5, LossRefundPct: 60}}}
	b := a
	b.Effects = []SkillEffect{{Kind: EffectResurrect, HealPctOfMax: 45, LossRefundPct: 100}}
	d, err := EvaluateV5Skill(a, b, 65536)
	if err != nil {
		t.Fatal(err)
	}
	if !d.ResurrectOnly() || d.Effect.LossRefundPct != 100 || d.Effect.HealPctOfMax != 45 || d.ProgressionShieldFlat != 262140 {
		t.Fatalf("resurrection bounds/identity incorrect: %+v", d)
	}
	a, b = curveAnchors(10129)
	a.Statuses[0].Level = 1
	b.Statuses[0].Level = 5
	d, err = EvaluateV5Skill(a, b, 101)
	if err != nil {
		t.Fatal(err)
	}
	if d.Statuses[0].Level != 1 || d.Statuses[0].DurationSec != 9 {
		t.Fatal("old detrimental 0->4 attack cap regression returned")
	}
}

func TestV5CurveNoAliasingAndNoSilentOverflow(t *testing.T) {
	a, b := curveAnchors(10306)
	a.Prerequisites = []SkillID{10301}
	b.Prerequisites = []SkillID{10301}
	a.Effects = []SkillEffect{{RemoveStatusIDs: []StatusID{StatusPoison, StatusStun}, DispelChanceBP: 2000}}
	b.Effects = []SkillEffect{{RemoveStatusIDs: []StatusID{StatusPoison, StatusStun}, DispelChanceBP: 10000}}
	a.PassiveCounter = &PassiveCounterAttack{ChanceBP: 4000, DamagePct: 100, RequiredEquipType: 8}
	counter := *a.PassiveCounter
	b.PassiveCounter = &counter
	d, err := EvaluateV5Skill(a, b, 21)
	if err != nil {
		t.Fatal(err)
	}
	d.Prerequisites[0] = 0
	d.Effects[0].RemoveStatusIDs[0] = 0
	d.Statuses[0].ExtraAffixes[0].Value = 999
	d.PassiveCounter.ChanceBP = 0
	if a.Prerequisites[0] != 10301 || a.Effects[0].RemoveStatusIDs[0] != StatusPoison || a.Statuses[0].ExtraAffixes[0].Value != 10 || a.PassiveCounter.ChanceBP != 4000 {
		t.Fatal("evaluation aliased an immutable legacy anchor")
	}
	for _, id := range []SkillID{10401, 10301, 10320, 10306} {
		a, b = curveAnchors(id)
		if _, err := EvaluateV5Skill(a, b, math.MaxInt32); err == nil {
			t.Fatalf("overflowing skill%d silently saturated", id)
		}
	}
	a, b = curveAnchors(10024)
	if d, err := EvaluateV5Skill(a, b, math.MaxInt32); err != nil || d.Level != math.MaxInt32 {
		t.Fatalf("representable high rank was treated as a gameplay cap: %v", err)
	}
	for _, r := range []int32{0, -1, math.MinInt32} {
		a, b = curveAnchors(10401)
		if _, err := EvaluateV5Skill(a, b, r); err == nil {
			t.Fatalf("invalid rank%d accepted", r)
		}
	}
	a, b = curveAnchors(11005)
	if _, err := EvaluateV5Skill(a, b, 1); err == nil {
		t.Fatal("life skill entered battle curve catalogue")
	}
	a, b = curveAnchors(10401)
	b.HurtType = HurtIce
	if _, err := EvaluateV5Skill(a, b, 20); err == nil {
		t.Fatal("mismatched element anchors accepted")
	}
	a, b = curveAnchors(10401)
	b.Effect.ChanceBP = 10001
	if _, err := EvaluateV5Skill(a, b, 20); err == nil {
		t.Fatal("illegal chance anchor silently clamped")
	}
}

func TestV5CurveAddedPassiveEdgesPreserveGraphMaximumThree(t *testing.T) {
	if err := ValidateSkillSynergies(V5SkillSynergies); err != nil {
		t.Fatal(err)
	}
	graph := map[SkillID]map[SkillID]bool{}
	for _, edge := range V5SkillSynergies {
		if graph[edge.Target] == nil {
			graph[edge.Target] = map[SkillID]bool{}
		}
		graph[edge.Target][edge.Source] = true
	}
	for _, id := range V5SkillCurveIDs() {
		a, b := curveAnchors(id)
		d, err := EvaluateV5Skill(a, b, 21)
		if err != nil {
			t.Fatal(err)
		}
		actual := map[SkillID]bool{}
		for _, m := range d.PassiveModifiers {
			if m.TargetSkill != 0 {
				actual[m.TargetSkill] = true
			}
		}
		for _, m := range d.PassiveStatusAffixes {
			if m.TargetSkill != 0 {
				actual[m.TargetSkill] = true
			}
		}
		for target := range actual {
			if !graph[target][id] {
				t.Fatalf("curve%d introduced unapproved new source for target%d", id, target)
			}
			if len(graph[target]) > 3 {
				t.Fatalf("target%d gained fourth source", target)
			}
		}
	}
	// Three distinct native sources already fill blizzard. Fixed carriers must
	// improve their existing edges rather than add a training fourth source.
	want := map[SkillID]bool{10431: true, 10430: true, 10427: true}
	if !reflect.DeepEqual(graph[10412], want) {
		t.Fatalf("blizzard graph=%v want%v", graph[10412], want)
	}
}

// This fixture is exported by the actual data.LoadSkills loader, using fourteen
// read-only SQL queries against a disposable seeded PostgreSQL 17 cluster. It is
// independent of both v5SkillCurves and the synthetic unit fixture above.
func realV5CurveAnchors(t *testing.T) SkillTable {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/daoist-endgame/v5/skill-legacy-anchors.json")
	if err != nil {
		t.Fatal(err)
	}
	var pairs []struct {
		ID    SkillID  `json:"id"`
		First SkillDef `json:"first"`
		Fifth SkillDef `json:"fifth"`
	}
	if err = json.Unmarshal(raw, &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 149 {
		t.Fatalf("real database fixture has%d skills", len(pairs))
	}
	table := SkillTable{}
	for _, p := range pairs {
		if p.ID != p.First.ID || p.ID != p.Fifth.ID || p.First.Level != 1 || p.Fifth.Level != 5 {
			t.Fatalf("corrupt source anchor identity%d", p.ID)
		}
		for _, d := range []SkillDef{p.First, p.Fifth} {
			k := SkillKey{ID: d.ID, Level: d.Level}
			if _, ok := table[k]; ok {
				t.Fatalf("duplicate real anchor%+v", k)
			}
			table[k] = d
		}
	}
	return table
}

func TestV5CurveRealDatabaseAnchorsAll149ContinuousSemantics(t *testing.T) {
	table := realV5CurveAnchors(t)
	ranks := []int32{255, 256, 65535, 65536, 1000000}
	for r := int32(1); r <= 101; r++ {
		ranks = append(ranks, r)
	}
	for _, id := range V5SkillCurveIDs() {
		a, b := table[SkillKey{ID: id, Level: 1}], table[SkillKey{ID: id, Level: 5}]
		for _, r := range ranks {
			x, err := EvaluateV5Skill(a, b, r)
			if err != nil {
				t.Fatal(err)
			}
			y, err := EvaluateV5Skill(a, b, r+1)
			if err != nil {
				t.Fatal(err)
			}
			if curveOperationalCarrier(y) <= curveOperationalCarrier(x) {
				t.Fatalf("real skill%d rank%d->%d does not improve consumed payload", id, r, r+1)
			}
			if x.ID != a.ID || x.Kind != a.Kind || x.Prof != a.Prof || x.TargetEnemy != a.TargetEnemy || x.TargetSelf != a.TargetSelf || x.TargetTeam != a.TargetTeam || x.RequiresInvisible != a.RequiresInvisible || x.RequiredWeaponType != a.RequiredWeaponType || !reflect.DeepEqual(x.Prerequisites, a.Prerequisites) {
				t.Fatalf("real skill%d lost identity or condition at rank%d", id, r)
			}
			original := a.DirectEffects()
			effective := x.DirectEffects()
			if len(effective) < len(original) {
				t.Fatalf("real skill%d removed an effect", id)
			}
			for i, fx := range original {
				fy := effective[i]
				if fx.Kind != fy.Kind || fx.Target != fy.Target || fx.RequiresPreviousLanded != fy.RequiresPreviousLanded || fx.GuaranteedHit != fy.GuaranteedHit || fx.ClearHarmful != fy.ClearHarmful || !reflect.DeepEqual(fx.RemoveStatusIDs, fy.RemoveStatusIDs) {
					t.Fatalf("real skill%d reordered or changed original effect%d", id, i)
				}
			}
			if len(a.Statuses) != len(x.Statuses) {
				t.Fatalf("real skill%d changed state identities", id)
			}
			for i, s := range a.Statuses {
				if s.ID != x.Statuses[i].ID || s.BlockHarmful != x.Statuses[i].BlockHarmful || s.Control != x.Statuses[i].Control || s.InstantDeathImmune != x.Statuses[i].InstantDeathImmune {
					t.Fatalf("real skill%d lost state%d semantics", id, s.ID)
				}
			}
		}
	}
}

func TestV5CurveRealRoarNonordinalStatusDoesNotOvershootThenRegress(t *testing.T) {
	table := realV5CurveAnchors(t)
	for _, id := range []SkillID{10029, 10128} {
		a, b := table[SkillKey{ID: id, Level: 1}], table[SkillKey{ID: id, Level: 5}]
		if a.Statuses[0].Level != 16 || b.Statuses[0].Level != 14 {
			t.Fatal("fixture no longer represents nonordinal original stun table")
		}
		previous := int32(5)
		for r := int32(1); r <= 101; r++ {
			d, err := EvaluateV5Skill(a, b, r)
			if err != nil {
				t.Fatal(err)
			}
			app := d.Statuses[0]
			if app.ID != StatusStun || app.Level != 16 || app.DurationSec < previous || app.DurationSec > 9 {
				t.Fatalf("roar%d rank%d selected nonordinal/worse state: %+v", id, r, app)
			}
			previous = app.DurationSec
		}
	}
}

func TestV5CurveRealCataloguePreflightRejectsMalformedAnchorAtomically(t *testing.T) {
	table := realV5CurveAnchors(t)
	if err := InstallV5SkillProgression(table); err != nil {
		t.Fatalf("genuine database anchors failed install: %v", err)
	}
	for _, id := range V5SkillCurveIDs() {
		if !table.V5Enabled(id) {
			t.Fatalf("real skill%d was not activated", id)
		}
	}
	table = realV5CurveAnchors(t)
	// A late catalogue entry is damaged to detect partial activation of earlier IDs.
	key := SkillKey{ID: 10435, Level: 5}
	d := table[key]
	d.Statuses = nil
	table[key] = d
	if err := InstallV5SkillProgression(table); err == nil {
		t.Fatal("activation accepted malformed last skill anchor")
	}
	for _, id := range V5SkillCurveIDs() {
		if table.V5Enabled(id) {
			t.Fatalf("failed preflight partially enabled skill%d", id)
		}
	}
}

func TestV5CurveStatusCarrierIsAdditiveSeparateFromLegacyOverrides(t *testing.T) {
	a, b := curveAnchors(10307)
	// A future base definition can already carry absolute attack. Original skill
	// overrides remain separate: a progression carrier must not replace either.
	a.Statuses[0].ExtraAffixes = []Affix{{Attr: AttrAtk, Value: 17, Mode: ModeAbsolute}, {Attr: AttrAtk, Value: 10, Mode: ModePercent}}
	b.Statuses[0].ExtraAffixes = []Affix{{Attr: AttrAtk, Value: 17, Mode: ModeAbsolute}, {Attr: AttrAtk, Value: 18, Mode: ModePercent}}
	a.Statuses[0].ProgressionAffixes = []Affix{{Attr: AttrMAtk, Value: 3, Mode: ModeAbsolute}}
	b.Statuses[0].ProgressionAffixes = []Affix{{Attr: AttrMAtk, Value: 3, Mode: ModeAbsolute}}
	d, err := EvaluateV5Skill(a, b, 21)
	if err != nil {
		t.Fatal(err)
	}
	app := d.Statuses[0]
	if !reflect.DeepEqual(app.ExtraAffixes, b.Statuses[0].ExtraAffixes) {
		t.Fatal("new flat carrier changed original same-attribute overrides")
	}
	if !reflect.DeepEqual(app.ProgressionAffixes, []Affix{{Attr: AttrMAtk, Value: 3, Mode: ModeAbsolute}, {Attr: AttrAtk, Value: 20, Mode: ModeAbsolute}}) {
		t.Fatalf("missing separate additive carrier: %+v", app.ProgressionAffixes)
	}
	app.ProgressionAffixes[0].Value = 999
	if a.Statuses[0].ProgressionAffixes[0].Value != 3 || b.Statuses[0].ProgressionAffixes[0].Value != 3 {
		t.Fatal("progression affixes alias immutable source anchors")
	}
}

func TestV5CurveOneShotDamageSurvivesStatStateConsumption(t *testing.T) {
	table := realV5CurveAnchors(t)
	for _, id := range []SkillID{10011, 10130, 10208, 10328, 10329, 10419, 10420} {
		a, b := table[SkillKey{ID: id, Level: 1}], table[SkillKey{ID: id, Level: 5}]
		d, err := EvaluateV5Skill(a, b, 21)
		if err != nil {
			t.Fatal(err)
		}
		step := int32(1)
		if id == 10011 || id == 10130 {
			step = 2
		}
		app := d.Statuses[0]
		if app.ProgressionAttackFlat != 20*step {
			t.Fatalf("one-shot skill%d lacks captured fixed damage: %+v", id, app)
		}
		if len(app.ProgressionAffixes) != 0 {
			t.Fatalf("one-shot skill%d relies on soon-removed actor stats", id)
		}
	}
}

func TestV5CurveRealControlAddedDamagePreservesOriginalHitEligibility(t *testing.T) {
	table := realV5CurveAnchors(t)
	for _, id := range []SkillID{10016, 10029, 10128, 10224, 10310, 10324, 10325, 10413, 10428, 10421} {
		a, b := table[SkillKey{ID: id, Level: 1}], table[SkillKey{ID: id, Level: 5}]
		first, err := EvaluateV5Skill(a, b, 1)
		if err != nil {
			t.Fatal(err)
		}
		next, err := EvaluateV5Skill(a, b, 2)
		if err != nil {
			t.Fatal(err)
		}
		if first.HasDamageEffect() {
			t.Fatalf("legacy control%d wrongly gained rank1 damage", id)
		}
		if len(next.DirectEffects()) == 0 {
			t.Fatalf("control%d lacks authored rank2 carrier", id)
		}
		fx := next.DirectEffects()[len(next.DirectEffects())-1]
		if fx.Kind != EffectDamage || !fx.GuaranteedHit || fx.DamageFlat != 1 || fx.DamagePctNumerator != 0 || fx.DamagePctDenominator != 1 {
			t.Fatalf("control%d now risks losing original status on an added MISS: %+v", id, fx)
		}
	}
}

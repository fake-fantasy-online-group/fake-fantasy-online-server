package domain

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"reflect"
	"testing"
)

// This fixture is the actual read-only loader output for all authored rows. It
// is independently exported from immutable original data, not curve formulas.
func realV5CurveAnchors(t *testing.T) SkillTable {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []SkillDef
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 149*5 {
		t.Fatalf("original rows=%d", len(rows))
	}
	table := SkillTable{}
	for _, d := range rows {
		k := SkillKey{ID: d.ID, Level: d.Level}
		if _, ok := table[k]; ok {
			t.Fatalf("duplicate original row %+v", k)
		}
		table[k] = d
	}
	return table
}

func TestOriginalSkillEveryAuthoredRankExactParity(t *testing.T) {
	skills := realV5CurveAnchors(t)
	original := SkillTable{}
	for k, d := range skills {
		original[k] = d
	}
	if err := InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	for k, want := range original {
		got, ok := skills.Get(k.ID, k.Level)
		if !ok {
			t.Fatalf("missing %+v", k)
		}
		got.FormulaVersion = 0
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("original row %d/%d was changed:\ngot %+v\nwant %+v", k.ID, k.Level, got, want)
		}
	}
}

func TestOriginalSkillEveryRankPreservesScopeAndNeverInventsCarriers(t *testing.T) {
	skills := realV5CurveAnchors(t)
	if err := InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	for _, id := range V5SkillIDs() {
		original := skills[SkillKey{ID: id, Level: 5}]
		for _, rank := range []int32{6, 20, 21, 100, 101, 255, 256, 65535, 65536, 1000000} {
			got, ok := skills.Get(id, rank)
			if !ok {
				t.Fatalf("skill%d rank%d unavailable", id, rank)
			}
			if got.Level != rank || got.FormulaVersion != SkillFormulaVersion {
				t.Fatalf("rank clamped: %+v", got)
			}
			if got.ID != original.ID || got.Kind != original.Kind || got.HurtType != original.HurtType || got.TargetSelf != original.TargetSelf || got.TargetEnemy != original.TargetEnemy || got.TargetTeam != original.TargetTeam || got.RequiresInvisible != original.RequiresInvisible || got.RequiredWeaponType != original.RequiredWeaponType || got.Area != original.Area {
				t.Fatalf("skill%d changed identity", id)
			}
			if got.ProgressionShieldFlat != 0 {
				t.Fatalf("skill%d invented utility shield", id)
			}
			if len(got.DirectEffects()) != len(original.DirectEffects()) || len(got.Statuses) != len(original.Statuses) || len(got.PassiveModifiers) != len(original.PassiveModifiers) || len(got.PassiveStats) != len(original.PassiveStats) || len(got.PassiveStatusAffixes) != len(original.PassiveStatusAffixes) || len(got.PassiveTriggers) != len(original.PassiveTriggers) || len(got.PassiveTriggerEffects) != len(original.PassiveTriggerEffects) {
				t.Fatalf("skill%d changed effect/target count", id)
			}
			for i, fx := range got.DirectEffects() {
				old := original.DirectEffects()[i]
				if fx.Kind != old.Kind || fx.Target != old.Target || fx.RequiresPreviousLanded != old.RequiresPreviousLanded || fx.GuaranteedHit != old.GuaranteedHit || fx.ClearHarmful != old.ClearHarmful || !reflect.DeepEqual(fx.RemoveStatusIDs, old.RemoveStatusIDs) {
					t.Fatalf("skill%d changed ordered effect%d", id, i)
				}
				if old.DamageFlat == 0 && fx.DamageFlat != 0 {
					t.Fatalf("skill%d invented fixed damage", id)
				}
				if fx.ChanceBP < 0 || fx.ChanceBP > 10000 || fx.DispelChanceBP < 0 || fx.DispelChanceBP > 10000 || fx.HealPctOfMax > 100 || fx.LossRefundPct > 100 {
					t.Fatalf("skill%d unbounded chance/recovery", id)
				}
			}
			for i, m := range got.PassiveModifiers {
				old := original.PassiveModifiers[i]
				if m.Kind != old.Kind || m.TargetSkill != old.TargetSkill || m.EffectKind != old.EffectKind {
					t.Fatalf("skill%d expanded passive scope", id)
				}
			}
			for i, m := range got.PassiveStats {
				old := original.PassiveStats[i]
				if m.Attr != old.Attr || m.Mode != old.Mode || m.RequiredEquipType != old.RequiredEquipType {
					t.Fatalf("skill%d changed passive equipment/attribute", id)
				}
			}
			for i, m := range got.PassiveStatusAffixes {
				old := original.PassiveStatusAffixes[i]
				if m.TargetSkill != old.TargetSkill || m.Status != old.Status || m.Affix.Attr != old.Affix.Attr || m.Affix.Mode != old.Affix.Mode {
					t.Fatalf("skill%d changed status passive scope", id)
				}
			}
			for i, m := range got.PassiveTriggers {
				old := original.PassiveTriggers[i]
				if m.TargetSkill != old.TargetSkill || m.Status != old.Status || m.StatusLevel != old.StatusLevel || m.ChanceBP > 10000 || m.DurationSec != old.DurationSec {
					t.Fatalf("skill%d changed trigger identity/bounds", id)
				}
			}
			for i, app := range got.Statuses {
				old := original.Statuses[i]
				if app.ID != old.ID || app.Level != old.Level || app.DurationSec != old.DurationSec || app.ChanceBP > 10000 || app.ProgressionRanks != rank-5 || app.ProgressionAttackFlat != 0 || app.ProgressionDamageShield != 0 || app.ProgressionReflectFlat != 0 || app.ProgressionHPPerTick != 0 || len(app.ProgressionAffixes) != 0 {
					t.Fatalf("skill%d changed status identity/bounds or invented filler: %+v", id, app)
				}
			}
		}
	}
}

func TestOriginalSkillMultiplierAndHealContinuation(t *testing.T) {
	skills := realV5CurveAnchors(t)
	if err := InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	// Values are independently fixed original data: Firebolt 208+8 per rank,
	// backstab300+20, sword130+3, fireball195+5. No 1-to-20 compression.
	for _, sample := range []struct {
		id          SkillID
		first, step int64
	}{{10401, 208, 8}, {10209, 300, 20}, {10102, 130, 3}, {10402, 195, 5}} {
		for _, rank := range []int32{1, 2, 3, 4, 5, 6, 20, 21, 100, 101, 65536, 1000000} {
			d, ok := skills.Get(sample.id, rank)
			if !ok {
				t.Fatal(sample, rank)
			}
			fx := d.DirectEffects()[0]
			got := big.NewRat(int64(fx.DamagePct), 1)
			if fx.DamagePctDenominator > 0 {
				got.SetFrac64(fx.DamagePctNumerator, fx.DamagePctDenominator)
			}
			want := big.NewRat(sample.first+sample.step*int64(rank-1), 1)
			if got.Cmp(want) != 0 || fx.DamageFlat != 0 {
				t.Fatalf("skill%d rank%d=%s +flat%d want%s", sample.id, rank, got, fx.DamageFlat, want)
			}
		}
	}
	for _, id := range []SkillID{10301, 10304} {
		fifth, _ := skills.Get(id, 5)
		sixth, _ := skills.Get(id, 6)
		if sixth.HasDamageEffect() || sixth.Effect.HealFlat <= fifth.Effect.HealFlat || sixth.Effect.HealAmount(1000) <= fifth.Effect.HealAmount(1000) {
			t.Fatalf("healing identity/growth lost skill%d", id)
		}
	}
	// Dragonkill's intrinsic, mana-derived fixed damage is not V5 filler.
	fifth, _ := skills.Get(10105, 5)
	sixth, _ := skills.Get(10105, 6)
	if fifth.Effect.DamageFlat <= 0 || sixth.Effect.DamageFlat <= fifth.Effect.DamageFlat {
		t.Fatal("original dragonkill flat effect was erased")
	}
}

func TestOriginalSkillControlsKeepNonordinalRowsAndNoDamage(t *testing.T) {
	skills := realV5CurveAnchors(t)
	if err := InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	for _, id := range []SkillID{10029, 10128} {
		for i, level := range []int32{16, 11, 12, 13, 14} {
			d, _ := skills.Get(id, int32(i+1))
			if d.Statuses[0].Level != level || d.HasDamageEffect() {
				t.Fatalf("nonordinal original control%d/%d %+v", id, i+1, d)
			}
		}
	}
	for _, id := range []SkillID{10016, 10029, 10128, 10224, 10310, 10324, 10325, 10413, 10428, 10421} {
		for _, rank := range []int32{1, 5, 6, 20, 100, 65536} {
			d, _ := skills.Get(id, rank)
			if d.HasDamageEffect() {
				t.Fatalf("pure control/status skill%d rank%d gained damage", id, rank)
			}
		}
	}
	// Maintenance retains exactly its original durability target. At100% the
	// mechanic is complete; no universal damage carrier is attached to fake growth.
	d, _ := skills.Get(10024, 1000000)
	if len(d.PassiveModifiers) != 1 || d.PassiveModifiers[0].Kind != PassiveSkillDurabilityReductionPct || d.PassiveModifiers[0].TargetSkill != 10003 || d.PassiveModifiers[0].Value != 100 {
		t.Fatalf("maintenance scope %+v", d)
	}
}

func TestOriginalSkillNoAliasingNoMissingRowsNoSilentOverflow(t *testing.T) {
	skills := realV5CurveAnchors(t)
	first, fifth := skills[SkillKey{10401, 1}], skills[SkillKey{10401, 5}]
	if _, err := EvaluateV5Skill(first, fifth, 2); err == nil {
		t.Fatal("guessed missing original rank2")
	}
	if _, err := EvaluateV5Skill(first, fifth, math.MaxInt32); err == nil {
		t.Fatal("overflowing damage silently saturated")
	}
	for _, rank := range []int32{0, -1, math.MinInt32} {
		if _, err := EvaluateV5Skill(first, fifth, rank); err == nil {
			t.Fatal("accepted invalid rank", rank)
		}
	}
	delete(skills, SkillKey{10401, 3})
	if err := InstallV5SkillProgression(skills); err == nil {
		t.Fatal("accepted missing original rank")
	}
	for _, d := range skills {
		if d.FormulaVersion != 0 {
			t.Fatal("failed preflight mutated original table")
		}
	}
	skills = realV5CurveAnchors(t)
	if err := InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	for _, rank := range []int32{1, 5, 6, 21} {
		d, _ := skills.Get(10307, rank)
		d.Statuses[0].ExtraAffixes[0].Value = 999
		d, _ = skills.Get(10307, rank)
		if d.Statuses[0].ExtraAffixes[0].Value == 999 {
			t.Fatal("mutable returned status aliases original")
		}
		d, _ = skills.Get(10001, rank)
		d.PassiveModifiers[0].TargetSkill = 999
		d, _ = skills.Get(10001, rank)
		if d.PassiveModifiers[0].TargetSkill == 999 {
			t.Fatal("mutable passive aliases original")
		}
	}
}

func TestOriginalSkillChanceAndStealthGrowthUncompressedUntilBounds(t *testing.T) {
	skills := realV5CurveAnchors(t)
	if err := InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	// Disarm trap originally grows55/63/71/79/87%; continue to95 then100.
	for rank, want := range map[int32]int32{5: 8700, 6: 9500, 7: 10000, 100: 10000} {
		d, _ := skills.Get(10218, rank)
		if d.Effect.ChanceBP != want {
			t.Fatalf("rank%d disarm chance=%d want%d", rank, d.Effect.ChanceBP, want)
		}
	}
	// Unchanging original50% freeze chance stays50 while original damage grows.
	for _, rank := range []int32{5, 6, 20, 100} {
		d, _ := skills.Get(10411, rank)
		if d.Statuses[0].ChanceBP != 5000 {
			t.Fatalf("rank%d changed fixed original freeze chance", rank)
		}
	}
	for rank, want := range map[int32]int32{5: 40, 6: 45, 10: 65, 16: 95, 17: 100, 100: 100} {
		d, _ := skills.Get(10415, rank)
		if d.PassiveModifiers[0].Value != want {
			t.Fatalf("rank%d casting reduction=%d want%d", rank, d.PassiveModifiers[0].Value, want)
		}
	}

	six, _ := skills.Get(10207, 6)
	if six.PassiveModifiers[0].Value != 12 || six.PassiveModifiers[1].Value != 30 || six.PassiveModifiers[2].Value != 30 {
		t.Fatalf("stealth original slopes compressed: %+v", six.PassiveModifiers)
	}
}

func TestOriginalSkillCostsTimingsAndUtilityEfficiency(t *testing.T) {
	skills := realV5CurveAnchors(t)
	if err := InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	changed := map[string]int{}
	fields := map[string]func(SkillDef) int32{"MPCost": func(d SkillDef) int32 { return d.MPCost }, "PrepareMS": func(d SkillDef) int32 { return d.PrepareMS }, "CooldownMS": func(d SkillDef) int32 { return d.CooldownMS }}
	for _, id := range V5SkillIDs() {
		first, fifth := skills[SkillKey{id, 1}], skills[SkillKey{id, 5}]
		for name, value := range fields {
			a, b := value(first), value(fifth)
			if a != b {
				changed[name]++
			}
			for _, rank := range []int32{1, 2, 3, 4, 5, 6, 20, 21, 100, 65536, 1000000} {
				d, ok := skills.Get(id, rank)
				if !ok {
					t.Fatalf("skill%d rank%d unavailable", id, rank)
				}
				want := int64(a) + (int64(b)-int64(a))*int64(rank-1)/4
				if rank > 5 && fifth.Kind.Active() && !fifth.HasDamageEffect() {
					want = int64(b)
					if name != "MPCost" {
						want = int64(b) * 50 / (50 + int64(rank-5))
					}
					if d.ManaCostReductionRanks != rank-5 {
						t.Fatalf("skill%d/%d utility efficiency missing", id, rank)
					}
				}
				if value(d) != int32(want) {
					t.Fatalf("skill%d rank%d %s=%d want%d", id, rank, name, value(d), want)
				}
			}
		}
	}
	if !reflect.DeepEqual(changed, map[string]int{"MPCost": 112, "PrepareMS": 6, "CooldownMS": 3}) {
		t.Fatalf("original source counts changed: %v", changed)
	}
	for _, sample := range []struct {
		id                       SkillID
		rank, mp, cast, cooldown int32
	}{{10401, 6, 41, 3000, 500}, {10401, 20, 69, 3000, 500}, {10304, 6, 125, 4509, 1000}, {10428, 6, 96, 1000, 31372}} {
		d, _ := skills.Get(sample.id, sample.rank)
		// Sample checks isolate changing fields; untouched timings are audited above.
		if d.MPCost != sample.mp {
			t.Fatalf("skill%d/%d mp=%d want%d", sample.id, sample.rank, d.MPCost, sample.mp)
		}
		if sample.id == 10304 && d.PrepareMS != sample.cast {
			t.Fatalf("group heal cast=%d", d.PrepareMS)
		}
		if sample.id == 10428 && d.CooldownMS != sample.cooldown {
			t.Fatalf("freeze cooldown=%d", d.CooldownMS)
		}
	}
}

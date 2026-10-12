package domain

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
)

func originalStatusScalingTable(t *testing.T) StatusTable {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/daoist-endgame/v5/skill-status-anchors.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []StatusDef
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	table := StatusTable{}
	for _, row := range rows {
		table[StatusKey{ID: row.ID, Level: row.Level}] = row
	}
	return table
}

func TestOriginalSkillStatusesPreserveStoredRows(t *testing.T) {
	table := originalStatusScalingTable(t)
	for key, want := range table {
		got, ok := table.ResolveSkillApplication(StatusApplication{ID: key.ID, Level: key.Level})
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("status %v changed before extension: %+v", key, got)
		}
	}
}

func TestOriginalSkillStatusesContinueExistingEffects(t *testing.T) {
	table := originalStatusScalingTable(t)
	tests := []struct {
		name                string
		id                  StatusID
		first, fifth, extra int32
		value               func(StatusDef) int32
		want                int32
	}{
		{"holy shield uses source's first row", StatusDamageShield, 11, 15, 1, func(d StatusDef) int32 { return d.DamageShield }, 1500},
		{"light shield", StatusDamageShield, 1, 5, 1, func(d StatusDef) int32 { return d.DamageShield }, 1700},
		{"regeneration", 10303, 1, 5, 1, func(d StatusDef) int32 { return d.HPChanges[0].Value }, 34},
		{"poison", StatusPoison, 22, 26, 4, func(d StatusDef) int32 { return d.Affixes[0].Value }, -48},
		{"constant burning grows original damage", 1036, 1, 1, 50, func(d StatusDef) int32 { return d.Affixes[0].Value }, -70},
		{"next basic attack", StatusKillingIntent, 1, 5, 4, func(d StatusDef) int32 { return d.NextBasicAttackPct }, 180},
		{"next spell", StatusRelease, 1, 5, 4, func(d StatusDef) int32 { return d.NextSkillDamagePct }, 160},
		{"duration multiplier follows original uncapped ratio", StatusFocus, 1, 5, 4, func(d StatusDef) int32 { return d.NextStatusDurationPct }, 130},
		{"existing attack buff", 1014, 1, 5, 4, func(d StatusDef) int32 { return d.Affixes[0].Value }, 26},
		{"sprint keeps its duration identity", 10018, 1, 5, 4, func(d StatusDef) int32 { return d.DurationSec }, 21},
		{"control duration stays bounded", StatusStun, 11, 15, 100, func(d StatusDef) int32 { return d.DurationSec }, 15},
		{"control pressure after duration cap", StatusStun, 11, 15, 100, func(d StatusDef) int32 { return d.ControlPower }, 95},
		{"chance bound", 1011, 1, 5, 100, func(d StatusDef) int32 { return d.MagicFullReflectChancePct }, 100},
		{"cast reduction bound", StatusInstantCast, 1, 5, 100, func(d StatusDef) int32 { return d.NextCastTimeReductionPct }, 100},
		{"debuff remains a percentage", StatusSlow, 3, 7, 100, func(d StatusDef) int32 { return d.Affixes[0].Value }, -100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := table.ResolveSkillApplication(StatusApplication{ID: tt.id, Level: tt.fifth, ProgressionFirstLevel: tt.first, ProgressionRanks: tt.extra})
			if !ok || tt.value(got) != tt.want {
				t.Fatalf("want %d, got %+v (resolved=%t)", tt.want, got, ok)
			}
			base, _ := table.Get(tt.id, tt.fifth)
			if got.ID != base.ID || got.Level != base.Level || got.IntervalSec != base.IntervalSec || got.Control != base.Control || got.BreakOnDamage != base.BreakOnDamage || got.BlockHarmful != base.BlockHarmful || got.Invulnerable != base.Invulnerable || got.PoisonOnHitLevel != base.PoisonOnHitLevel {
				t.Fatal("extension changed original mechanics", got, base)
			}
			if got.ProgressionAttackFlat != 0 {
				t.Fatal("invented flat attack carrier", got)
			}
		})
	}
}

func TestOriginalSkillStatusDurationUsesSourceOverridesAndCaps(t *testing.T) {
	table := originalStatusScalingTable(t)
	for _, tt := range []struct {
		name               string
		app                StatusApplication
		duration, pressure int32
	}{
		{"petrify grows before cap", StatusApplication{ID: StatusPetrify, Level: 1, ProgressionFirstLevel: 1, ProgressionRanks: 1, DurationSec: 9, ProgressionFirstDurationSec: 5, ProgressionBaseDurationSec: 9}, 10, 0},
		{"petrify capped", StatusApplication{ID: StatusPetrify, Level: 1, ProgressionFirstLevel: 1, ProgressionRanks: 7, DurationSec: 9, ProgressionFirstDurationSec: 5, ProgressionBaseDurationSec: 9}, 15, 1},
		{"freeze grows before cap", StatusApplication{ID: StatusFreeze, Level: 5, ProgressionFirstLevel: 5, ProgressionRanks: 1, DurationSec: 40, ProgressionFirstDurationSec: 20, ProgressionBaseDurationSec: 40}, 45, 0},
		{"freeze capped", StatusApplication{ID: StatusFreeze, Level: 5, ProgressionFirstLevel: 5, ProgressionRanks: 5, DurationSec: 40, ProgressionFirstDurationSec: 20, ProgressionBaseDurationSec: 40}, 60, 1},
		{"original long silence retained", StatusApplication{ID: StatusSilence, Level: 7, ProgressionFirstLevel: 3, ProgressionRanks: 5}, 34, 5},
		{"constant control below cap has no pressure", StatusApplication{ID: StatusFreeze, Level: 1, ProgressionFirstLevel: 1, ProgressionRanks: 5, DurationSec: 5, ProgressionFirstDurationSec: 5, ProgressionBaseDurationSec: 5}, 5, 0},
		{"constant debuff at cap has pressure", StatusApplication{ID: StatusBlind, Level: 15, ProgressionFirstLevel: 11, ProgressionRanks: 5}, 60, 5},
		{"stealth passive duration bonus", StatusApplication{ID: StatusStealth, Level: 5, ProgressionFirstLevel: 1, ProgressionRanks: 2, DurationSec: 43, ProgressionFirstDurationSec: 30, ProgressionBaseDurationSec: 38}, 47, 0},
		{"unique bonus on continued duration", StatusApplication{ID: StatusDamageShield, Level: 5, ProgressionFirstLevel: 1, ProgressionRanks: 1, DurationSec: 111}, 121, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := table.ResolveSkillApplication(tt.app)
			if !ok || d.DurationSec != tt.duration || d.ControlPower != tt.pressure {
				t.Fatal("duration/pressure mismatch", d.DurationSec, d.ControlPower, "want", tt.duration, tt.pressure)
			}
		})
	}
}

func TestOriginalSkillStatusExtensionDoesNotMutateTablesOrInventEffects(t *testing.T) {
	table := originalStatusScalingTable(t)
	before, _ := json.Marshal(tableAsStatusRows(table))
	got, ok := table.ResolveSkillApplication(StatusApplication{ID: 1014, Level: 5, ProgressionFirstLevel: 1, ProgressionRanks: 100})
	if !ok || got.DamageShield != 0 || got.HPChanges != nil || got.PhysicalReflectFlat != 0 {
		t.Fatal("extension created an unrelated effect", got)
	}
	got.Affixes[0].Value = 999
	after, _ := json.Marshal(tableAsStatusRows(table))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("extension mutated its source table")
	}
	if _, ok := table.ResolveSkillApplication(StatusApplication{ID: 1014, Level: 5, ProgressionFirstLevel: 999, ProgressionRanks: 1}); ok {
		t.Fatal("missing source anchor silently accepted")
	}
}

func tableAsStatusRows(t StatusTable) map[string]StatusDef {
	// Stable JSON comparison without relying on map iteration order.
	out := map[string]StatusDef{}
	for _, row := range t {
		key, _ := json.Marshal(StatusKey{ID: row.ID, Level: row.Level})
		out[string(key)] = row
	}
	return out
}

func TestOriginalSkillStatusResistancePressure(t *testing.T) {
	for _, resist := range []int32{-5, 0, 25, 90, 100, 200} {
		if got := SkillStatusResistChance(resist, 0); got != ResistChance(resist) {
			t.Fatalf("legacy resistance changed: %d => %d", resist, got)
		}
	}
	if got := SkillStatusResistChance(90, 20); got != 45 {
		t.Fatal("20 extra ranks should halve numerical resistance", got)
	}
	if got := SkillStatusResistChance(100, 20); got != 45 {
		t.Fatal("raw numeric100 was never categorical immunity", got)
	}
	if got := SkillStatusResistChance(0, math.MaxInt32); got != 0 {
		t.Fatal("zero resistance changed", got)
	}
	if got, ok := continueStatusValue(1, math.MaxInt32, math.MaxInt32); ok {
		t.Fatal("overflowed effect continuation silently accepted", got)
	}
	table := StatusTable{
		{ID: StatusDamageShield, Level: 1}: {ID: StatusDamageShield, Level: 1, DamageShield: 1},
		{ID: StatusDamageShield, Level: 5}: {ID: StatusDamageShield, Level: 5, DamageShield: math.MaxInt32},
	}
	if _, ok := table.ResolveSkillApplication(StatusApplication{ID: StatusDamageShield, Level: 5, ProgressionFirstLevel: 1, ProgressionRanks: 1}); ok {
		t.Fatal("status resolver accepted overflowed shield")
	}
}

func TestOriginalSkillStatusBoundedEffectsAllowHugeRanks(t *testing.T) {
	table := originalStatusScalingTable(t)
	for _, id := range []StatusID{1011, StatusSlow, StatusFireShield, StatusInstantCast} {
		d, ok := table.ResolveSkillApplication(StatusApplication{ID: id, Level: 5, ProgressionFirstLevel: 1, ProgressionRanks: math.MaxInt32})
		if !ok || d.MagicFullReflectChancePct > 100 || d.PhysicalDamageReductionPct > 100 || d.PhysicalReflectPct > 100 || d.NextCastTimeReductionPct > 100 {
			t.Fatal("bounded effect failed before its representable cap", id, d, ok)
		}
	}
}

func TestConstantSkillStatusMagnitudeAdvancesEveryRepresentableRank(t *testing.T) {
	for _, base := range []int32{-100, -75, -49, -20, -1, 1, 20, 49, 75, 100} {
		previous := base
		for rank := int32(1); rank <= 200; rank++ {
			got, ok := continueStatusMagnitude(base, base, rank)
			if !ok || base > 0 && got <= previous || base < 0 && got >= previous {
				t.Fatalf("base=%d added rank=%d: %d -> %d, valid=%t", base, rank, previous, got, ok)
			}
			previous = got
		}
	}
	if value, ok := continueStatusMagnitude(0, 0, 100); !ok || value != 0 {
		t.Fatal("absent magnitude invented", value, ok)
	}
	if value, ok := continueStatusValue(20, 20, 100); !ok || value != 20 {
		t.Fatal("ordinary constant field changed", value, ok)
	}
	for _, base := range []int32{math.MaxInt32, math.MinInt32} {
		if _, ok := continueStatusMagnitude(base, base, 1); ok {
			t.Fatal("unrepresentable magnitude was accepted", base)
		}
	}
}

func TestOriginalLowConstantPeriodicStatusesAdvanceEveryRank(t *testing.T) {
	table := originalStatusScalingTable(t)
	previous, ok := table.Get(1036, 1)
	if !ok {
		t.Fatal("burning fixture missing")
	}
	for rank := int32(6); rank <= 200; rank++ {
		got, ok := table.ResolveSkillApplication(StatusApplication{ID: 1036, Level: 1, ProgressionFirstLevel: 1, ProgressionRanks: rank - 5})
		if !ok || len(got.Affixes) != len(previous.Affixes) || got.Affixes[0].Value >= previous.Affixes[0].Value {
			t.Fatalf("burning rank %d: damage failed to advance: %+v -> %+v", rank, previous, got)
		}
		if got.IntervalSec != previous.IntervalSec || got.Control != previous.Control {
			t.Fatal("burning timing or mechanics changed", rank)
		}
		previous = got
	}
}

func TestFunctionalSkillStatusesKeepMeaningfulManaProgression(t *testing.T) {
	skills := realV5CurveAnchors(t)
	checked := 0
	for _, id := range V5SkillIDs() {
		first, fifth := skills[SkillKey{ID: id, Level: 1}], skills[SkillKey{ID: id, Level: 5}]
		if !fifth.Kind.Active() || fifth.HasDamageEffect() || fifth.MPCost <= 0 {
			continue
		}
		checked++
		previous := fifth
		for _, rank := range []int32{20, 21, 30, 50, 100} {
			got, err := EvaluateV5Skill(first, fifth, rank)
			if err != nil {
				t.Fatal(id, rank, err)
			}
			if got.MPCost != fifth.MPCost || got.ManaCostReductionRanks != rank-OriginalSkillRankCap {
				t.Fatalf("skill %d rank %d: base mana=%d, efficiency=%d", id, rank, got.MPCost, got.ManaCostReductionRanks)
			}
			FinalizeSkillManaCost(&got)
			if got.MPCost > previous.MPCost || got.MPCost < 1 {
				t.Fatalf("skill %d rank %d: integer mana cost increased or crossed one", id, rank)
			}
			if got.Area != fifth.Area || len(got.DirectEffects()) != len(fifth.DirectEffects()) || len(got.Statuses) != len(fifth.Statuses) {
				t.Fatalf("skill %d rank %d: mana reduction changed scope", id, rank)
			}
			previous = got
		}
	}
	if checked == 0 {
		t.Fatal("no functional skills audited")
	}
}

package domain

import (
	"math"
	"math/rand"
	"testing"
)

// This is the pre-refactor arithmetic, kept in the test to catch changes in
// operation order, truncation, PvP coefficients and caps at the shared boundary.
func priorOutgoing(amount, existingBP, elementBP, payload int32, pvp bool) int32 {
	if pvp {
		elementBP /= 2
	}
	bp := min(int64(10000), max(int64(0), int64(existingBP)+int64(elementBP)))
	amount = int32(max(int64(1), min(int64(math.MaxInt32), int64(amount)*(10000+bp)/10000)))
	if payload > 0 {
		amount = int32(max(int64(1), min(int64(math.MaxInt32), int64(amount)*int64(payload)/10000)))
	}
	return amount
}

// Frozen independent pre-refactor reduction formula. Inputs in the randomized
// test are bounded so direct int64 multiplication cannot overflow; this oracle
// deliberately calls no production reduction, scaling or saturation helper.
func priorIncoming(amount int32, b CombatDamageBuckets) int32 {
	wide := int64(amount) + int64(amount)*int64(max(int32(0), b.DamageTakenIncreasePct))/100
	pierce, cap := b.ResistancePierceBP, int32(3000)
	if b.PVP {
		pierce /= 2
		cap = 1500
	}
	pierce = min(cap, max(int32(0), pierce))
	resist := max(int64(-5000), min(int64(7500), int64(b.ResistanceBP)-int64(pierce)))
	baseline := wide
	wide = wide * (10000 - resist) / 10000
	if resist < 0 {
		baseline = wide
	}
	mitigation := max(int64(0), min(int64(4500), b.MitigationBP))
	wide = wide * (10000 - mitigation) / 10000
	for n := 0; n < b.HalfDamageLayers; n++ {
		wide = (wide + 1) / 2
	}
	reduction := max(int64(0), min(int64(100), int64(b.DamageReductionPct)))
	wide -= wide * reduction / 100
	wide -= max(int64(0), b.FlatReduction)
	wide = max(wide, (baseline+3)/4)
	return int32(max(int64(0), min(int64(math.MaxInt32), wide)))
}

func TestCombatDamageBucketsPreserveExistingArithmetic(t *testing.T) {
	rng := rand.New(rand.NewSource(8912))
	for i := 0; i < 10000; i++ {
		amount := int32(rng.Intn(2000000000) + 1)
		existing, element := int32(rng.Intn(20000)), int32(rng.Intn(20000))
		b := CombatDamageBuckets{DamageIncreaseBP: int64(existing), PVPScaledDamageIncreaseBP: int64(element),
			ResistanceBP: int32(rng.Intn(20000) - 5000), ResistancePierceBP: int32(rng.Intn(10000)),
			MitigationBP: int64(rng.Intn(20000)), DamageTakenIncreasePct: int32(rng.Intn(400)), DamageReductionPct: int32(rng.Intn(150)),
			HalfDamageLayers: rng.Intn(3), FlatReduction: int64(rng.Intn(2000000000)), PVP: i%2 == 0}
		if i%3 == 0 {
			b.PayloadScaleBP = int32(rng.Intn(10000) + 1)
		}
		want := priorOutgoing(amount, existing, element, b.PayloadScaleBP, b.PVP)
		if got := b.OutgoingDamage(amount); got != want {
			t.Fatalf("outgoing sample %d got %d want %d", i, got, want)
		}
		if got := b.IncomingDamage(want); got != priorIncoming(want, b) {
			t.Fatalf("incoming sample %d changed", i)
		}
	}
}
func TestCombatDamageBucketsContributionsAreAdditiveAndSourceIndependent(t *testing.T) {
	// Labels are deliberately absent from the production bucket. Equipment,
	// current conditions and dynamic effects all contribute ordinary numbers.
	contributions := []int64{1200, 500, 800}
	for _, order := range [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}} {
		var b CombatDamageBuckets
		for _, i := range order {
			b.DamageIncreaseBP += contributions[i]
		}
		if got := b.OutgoingDamage(1000); got != 1250 {
			t.Fatal("same bucket compounded or depended on source order", got)
		}
	}
	b := CombatDamageBuckets{DamageIncreaseBP: 3000, PVPScaledDamageIncreaseBP: 4000}
	if got := b.OutgoingDamage(1000); got != 1700 {
		t.Fatal(got)
	}
	b.PVP = true
	if got := b.OutgoingDamage(1000); got != 1500 {
		t.Fatal("PvP source scaling applied twice", got)
	}
	b.DamageIncreaseBP = math.MaxInt32
	if got := b.OutgoingDamage(1000); got != 2000 {
		t.Fatal("compatibility +100% cap changed", got)
	}
	b = CombatDamageBuckets{ResistanceBP: 7500, MitigationBP: 4500, DamageReductionPct: 75, HalfDamageLayers: 2, FlatReduction: 1000000}
	if got := b.IncomingDamage(1000); got != 250 {
		t.Fatal("combined reduction cap changed", got)
	}
}
func TestPeriodicRecoveryBucketsKeepDistinctCadences(t *testing.T) {
	b := make(PeriodicRecoveryBuckets)
	b.AddFlatHP(2000, 3)
	b.AddFlatHP(2000, 7)
	b.AddFlatHP(1500, 11)
	b.AddFlatHP(50, 999)
	b.AddFlatHP(1501, 999)
	b.AddFlatHP(61000, 999)
	b.AddFlatHP(2000, -1)
	if len(b) != 2 || b[2000] != 10 || b[1500] != 11 {
		t.Fatal("periodic recovery scopes mixed", b)
	}
}

func TestCombatPayloadScaleDistinguishesUnsetAndExplicitZero(t *testing.T) {
	if got := (CombatDamageBuckets{}).OutgoingDamage(1000); got != 1000 {
		t.Fatal("unset coefficient changed damage", got)
	}
	if got := (CombatDamageBuckets{PayloadScaleKnown: true}).OutgoingDamage(1000); got != 1 {
		t.Fatal("explicit zero became full damage", got)
	}
	for _, scale := range []int32{1, 3500, 10000} {
		legacy := CombatDamageBuckets{PayloadScaleBP: scale}
		explicit := legacy
		explicit.PayloadScaleKnown = true
		if legacy.OutgoingDamage(12345) != explicit.OutgoingDamage(12345) {
			t.Fatal("positive coefficient changed", scale)
		}
	}
}

func TestCombatDamageBucketScaledContributionsRoundOnlyOnce(t *testing.T) {
	// Equipment 1bp + default aura 1bp must retain the old (1+1)/2 = 1bp.
	b := CombatDamageBuckets{PVP: true, PVPScaledDamageIncreaseBP: 1, ScaledDamageIncreaseNumerator: 5000}
	if got := b.OutgoingDamage(10000); got != 10001 {
		t.Fatal("separate source rounding lost a BP", got)
	}
	b.ScaledDamageIncreaseNumerator = 2000 * 2500
	if got := b.OutgoingDamage(10000); got != 10500 {
		t.Fatal("configured aura PvP scale ignored", got)
	}
}

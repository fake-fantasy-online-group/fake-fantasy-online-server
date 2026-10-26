package domain

import (
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"
)

func TestSkillRoundingStableConservationAndNativeFloor(t *testing.T) {
	for _, n := range []int64{0, 1, 50, 1200, 6800, 10000, int64(math.MaxInt32) * 50} {
		for _, d := range []int64{50, 51, 145, 146, 10000, int64(math.MaxInt32) + 45, (int64(math.MaxInt32) + 45) * 100} {
			var c SkillRoundingCredit
			var sum int64
			for i := int64(1); i <= 100; i++ {
				v, next := RoundSkillCost(n, d, c)
				c = next
				sum += v
				want := int64(0)
				if n > 0 {
					want = (i*max(n, d) + d - 1) / d
				}
				if sum != want || n > 0 && v < 1 || c.Numerator < 0 || c.Denominator > 0 && c.Numerator >= c.Denominator {
					t.Fatal(n, d, i, v, sum, want, c)
				}
			}
		}
	}
}

func TestSkillRoundingRateChangesCannotCreateCredit(t *testing.T) {
	for _, oldD := range []int64{51, 145, 146, int64(math.MaxInt32) + 45, (int64(math.MaxInt32) + 45) * 100} {
		for _, d := range []int64{51, 146, 10000, (int64(math.MaxInt32) + 45) * 100} {
			for _, rem := range []int64{0, 1, oldD / 2, oldD - 1} {
				old := SkillRoundingCredit{rem, oldD}
				v, next := RoundSkillCost(3*d, d, old)
				if v != 3 {
					t.Fatal("integer base discounted", v)
				}
				before := new(big.Rat).SetFrac64(rem, oldD)
				after := new(big.Rat).SetFrac64(next.Numerator, next.Denominator)
				loss := new(big.Rat).Sub(before, after)
				if loss.Sign() < 0 || loss.Cmp(new(big.Rat).SetFrac64(1, d)) >= 0 {
					t.Fatal(old, d, next, loss)
				}
			}
		}
	}
}

func TestSkillRoundingCharacterCloneValidationAndOriginalLevels(t *testing.T) {
	c := &Character{SkillRoundingCredits: map[SkillID]SkillRoundingState{10018: {Mana: SkillRoundingCredit{105, 145}}}}
	copy := c.Clone()
	copy.SkillRoundingCredits[10018] = SkillRoundingState{}
	if c.SkillRoundingCredits[10018].Mana.Numerator != 105 {
		t.Fatal("shared character ledger")
	}
	if err := ValidateSkillRoundingCredits(c.SkillRoundingCredits); err != nil {
		t.Fatal(err)
	}
	for _, states := range []map[SkillID]SkillRoundingState{{999: {}}, {10018: {Mana: SkillRoundingCredit{1, 0}}}, {10018: {Mana: SkillRoundingCredit{-1, 2}}}, {10018: {Cooldown: SkillRoundingCredit{2, 2}}}} {
		if ValidateSkillRoundingCredits(states) == nil {
			t.Fatal("invalid persisted credit accepted", states)
		}
	}
	for _, rank := range []int32{1, 2, 3, 4, 5} {
		d := SkillDef{ID: 10018, Level: rank, MPCost: 24, CooldownMS: 20000}
		before := d
		old := c.SkillRoundingCredits[10018]
		if next := PreviewSkillRounding(&d, old); next != old || !reflect.DeepEqual(d, before) {
			t.Fatal("native level spent credit", rank, d, next)
		}
	}
}

func TestSkillRoundingBurnInstanceCloneSaveAndReplace(t *testing.T) {
	def := StatusDef{ID: 1036, Level: 1, DurationSec: 40, IntervalSec: 2, PeriodicDamageFraction100: 40}
	set := NewStatusSet()
	set.Apply(def, 0, 1)
	due := set.DueTicks(20)
	if len(due) != 1 || due[0].PeriodicDamageTickExtra != 0 {
		t.Fatal(due)
	}
	clone := set.Clone()
	saved := set.SaveForLogout(20, time.Unix(1000, 0), 1)
	restored := RestoreSavedStatuses(saved, 100, time.Unix(1000, 0), 1)
	for _, candidate := range []*StatusSet{set, clone, restored} {
		at := Tick(40)
		if candidate == restored {
			at = 120
		}
		first := candidate.DueTicks(at)
		second := candidate.DueTicks(at + 20)
		if len(first) != 1 || len(second) != 1 || first[0].PeriodicDamageTickExtra != 0 || second[0].PeriodicDamageTickExtra != 1 || second[0].PeriodicDamageRemainder100 != 20 {
			t.Fatal("lost/duplicated instance credit", first, second)
		}
	}
	// Scene processing expires old instances before a later independent cast.
	set.Expire(1000)
	set.Apply(def, 1000, 2)
	if got := set.DueTicks(1020); len(got) != 1 || got[0].PeriodicDamageTickExtra != 0 || got[0].Source != 2 {
		t.Fatal("new source inherited expired credit", got)
	}
	overlay := StatusOverlay{{Existing: 1036, Incoming: 1036}: OverlayMustReplace}
	set.ApplyWithOverlay(def, 1020, 3, overlay)
	if set.Count() != 2 {
		t.Fatal("different DoT sources must coexist even under same-ID replacement")
	}
	due = set.DueTicks(1040)
	if len(due) != 2 || due[0].PeriodicDamageTickExtra != 0 || due[1].PeriodicDamageTickExtra != 0 {
		t.Fatal("instances shared credit", due)
	}
}

func TestSkillRoundingHonorsEquipmentMinimumAtRoundingBoundary(t *testing.T) {
	base := SkillDef{ID: 10018, Level: 101, ManaCostReductionRanks: 96, CooldownMS: 6850, CooldownFractionNumeratorMS: 1000000, CooldownFractionDenominator: 146, EquipmentVariant: &EquipmentSkillVariant{Behavior: VariantInstantCast, Params: EquipmentVariantParams{MinCooldownMS: 6850}}}
	var credit SkillRoundingState
	for i := 0; i < 100; i++ {
		d := base
		credit = PreviewSkillRounding(&d, credit)
		if d.CooldownMS != 6900 {
			t.Fatal("variant minimum was rounded downward", i, d.CooldownMS, credit)
		}
	}
}

package domain

import "testing"

func TestPassiveOverflowRanksV6(t *testing.T) {
	table := realV5CurveAnchors(t)
	for _, rank := range []int32{20, 21, 30, 50, 100} {
		for _, id := range []SkillID{10008, 10024, 10012, 10019, 10020, 10201, 10203, 10220, 10222, 10226, 10229, 10230, 10415, 10416, 10422, 10427, 10429} {
			first, fifth := table[SkillKey{id, 1}], table[SkillKey{id, 5}]
			current, err := EvaluateV5Skill(first, fifth, rank)
			if err != nil {
				t.Fatal(err)
			}
			next, err := EvaluateV5Skill(first, fifth, rank+1)
			if err != nil {
				t.Fatal(err)
			}
			// At each sampled high rank a native magnitude or an explicitly scoped
			// overflow channel increases, without changing the native carrier count.
			gain := false
			for i, m := range current.PassiveModifiers {
				n := next.PassiveModifiers[i]
				gain = gain || n.Value > m.Value || n.ProgressionOverflow > m.ProgressionOverflow
				if (m.Kind == PassiveCastTimeReductionPct && m.Value > 100) || (m.Kind != PassiveCastTimeReductionPct && m.Value > 100) {
					t.Fatalf("%d rank%d impossible chance: %+v", id, rank, m)
				}
			}
			for i, m := range current.PassiveResists {
				n := next.PassiveResists[i]
				gain = gain || n.Pct > m.Pct || n.ProgressionRestoreMP > m.ProgressionRestoreMP
				if m.Pct > 100 {
					t.Fatal(m)
				}
			}
			for i, m := range current.PassiveTriggers {
				n := next.PassiveTriggers[i]
				gain = gain || n.ChanceBP > m.ChanceBP || n.ManaCostReductionRanks > m.ManaCostReductionRanks
			}
			for i, m := range current.PassiveTriggerEffects {
				n := next.PassiveTriggerEffects[i]
				gain = gain || n.Effect.ChanceBP > m.Effect.ChanceBP || n.ManaCostReductionRanks > m.ManaCostReductionRanks
				if n.Effect.KnockbackDistance != m.Effect.KnockbackDistance {
					t.Fatal("knockback range changed")
				}
			}
			for i, m := range current.PassiveStatusAffixes {
				n := next.PassiveStatusAffixes[i]
				gain = gain || n.Affix.Value != m.Affix.Value || n.ManaCostReductionRanks > m.ManaCostReductionRanks
			}
			if current.PassiveCounter != nil {
				gain = gain || next.PassiveCounter.DamagePct > current.PassiveCounter.DamagePct
			}
			if !gain {
				t.Errorf("passive %d rank%d ->%d gives no gain", id, rank, rank+1)
			}
		}
	}
}

package domain

import (
	"math"
	"testing"
)

func TestSkillGrowthPolicyCatalogueAndSingleAttribute(t *testing.T) {
	expected := map[SkillID]struct{ attr, cap int32 }{
		10024: {AttrVIT, 4}, 10222: {AttrVIT, 10}, 10229: {AttrVIT, 11}, 10230: {AttrVIT, 11},
		10314: {AttrINT, 17}, 10415: {AttrINT, 17}, 10416: {AttrINT, 5}, 10329: {AttrINT, 5}, 10420: {AttrINT, 5},
		10306: {AttrSPI, 5}, 10316: {AttrSPI, 5}, 10318: {AttrSPI, 5}, 10433: {AttrSPI, 5}, 10218: {AttrSPI, 7},
	}
	ids := V5SkillIDs()
	if len(ids) != 149 {
		t.Fatal(len(ids))
	}
	for _, id := range ids {
		p, ok := LookupSkillGrowthPolicy(id)
		if !ok {
			t.Fatal("unclassified catalogue member", id)
		}
		want, special := expected[id]
		if !special {
			if p.Attribute != 0 {
				t.Fatal("unapproved attribute", id, p)
			}
			continue
		}
		for _, rank := range []int32{1, want.cap, want.cap + 1, want.cap + 2, 100} {
			attr, value := SkillGrowthAttribute(id, rank)
			if rank <= want.cap {
				if attr != 0 || value != 0 {
					t.Fatal(id, rank, attr, value)
				}
			} else if attr != want.attr || value != rank-want.cap {
				t.Fatal(id, rank, attr, value)
			}
		}
	}
	if _, ok := LookupSkillGrowthPolicy(99999); ok {
		t.Fatal("invented outside-catalogue policy")
	}
	p, _ := LookupSkillGrowthPolicy(10427)
	p.AttackTargets[0] = 99999
	p, _ = LookupSkillGrowthPolicy(10427)
	if p.AttackTargets[0] != 10409 {
		t.Fatal("aliased policy target list")
	}
}

func TestSkillGrowthDamageEnhancesOnlyOriginalPassiveTargets(t *testing.T) {
	table := realV5CurveAnchors(t)
	for id, p := range skillGrowthPolicies {
		if p.AttackCapRank == 0 {
			continue
		}
		original := table[SkillKey{id, 5}]
		native := map[SkillID]bool{}
		for _, v := range original.PassiveModifiers {
			native[v.TargetSkill] = true
		}
		for _, v := range original.PassiveTriggers {
			native[v.TargetSkill] = true
		}
		for _, v := range original.PassiveTriggerEffects {
			native[v.TargetSkill] = true
		}
		for _, v := range original.PassiveStatusAffixes {
			native[v.TargetSkill] = true
		}
		for _, target := range p.AttackTargets {
			if !native[target] {
				t.Fatal("invented attack target", id, target)
			}
			if got := SkillGrowthDamageBonusPct(id, p.AttackCapRank, target); got != 0 {
				t.Fatal(id, got)
			}
			if got := SkillGrowthDamageBonusPct(id, p.AttackCapRank+1, target); got != 2 {
				t.Fatal(id, got)
			}
			if got := SkillGrowthDamageBonusPct(id, p.AttackCapRank+2, target); got != 4 {
				t.Fatal(id, got)
			}
		}
		if SkillGrowthDamageBonusPct(id, 100, 0) != 0 || SkillGrowthDamageBonusPct(id, 100, 10435) != 0 {
			t.Fatal("unrelated damage increased", id)
		}
	}
}

func TestSkillGrowthRangeBoundariesAndNoHardPlateau(t *testing.T) {
	table := realV5CurveAnchors(t)
	for id, p := range skillGrowthPolicies {
		if p.RangeCapRank == 0 {
			continue
		}
		a, b := table[SkillKey{id, 1}], table[SkillKey{id, 5}]
		if b.Kind != SkillArea || b.HasDamageEffect() {
			t.Fatal("bad range policy", id)
		}
		for _, rank := range []int32{p.RangeCapRank, p.RangeCapRank + 1, p.RangeCapRank + 50, p.RangeCapRank + 51, p.RangeCapRank + 1000} {
			got, err := EvaluateV5Skill(a, b, rank)
			if err != nil {
				t.Fatal(err)
			}
			if got.Area.Shape != b.Area.Shape || got.Area.Center != b.Area.Center || got.Area.Angle != b.Area.Angle || got.Dist != b.Dist {
				t.Fatal("scope changed", id, rank)
			}
			if rank == p.RangeCapRank && got.Area != b.Area {
				t.Fatal("range grew early", id)
			}
			if rank > p.RangeCapRank && got.Area.Radius <= b.Area.Radius {
				t.Fatal("range failed to grow", id, rank)
			}
			if rank == p.RangeCapRank+50 && got.Area.Radius != 2*b.Area.Radius {
				t.Fatal("2-percent radius slope", id, got.Area)
			}
			if rank > p.RangeCapRank+50 && got.Area.Radius <= 2*b.Area.Radius {
				t.Fatal("radius hard-capped", id, rank)
			}
			if id == 10327 && rank > p.RangeCapRank && got.DirectEffects()[0].RevealRadius <= b.DirectEffects()[0].RevealRadius {
				t.Fatal("runtime reveal radius missed", got)
			}
		}
	}
	e := v5Evaluator{id: 10028, rank: math.MaxInt32}
	if e.growthRadius(200, math.MaxInt32) <= 400 || e.err != nil {
		t.Fatal("large-rank safe radius", e.err)
	}
	e.growthRadius(math.MaxInt32, math.MaxInt32)
	if e.err == nil {
		t.Fatal("unrepresentable radius silently wrapped")
	}
}

func TestSkillGrowthProtectionAndChargeDuration(t *testing.T) {
	skills, statuses := realV5CurveAnchors(t), originalStatusScalingTable(t)
	for id, p := range skillGrowthPolicies {
		if p.ProtectionCapRank == 0 {
			continue
		}
		a, b := skills[SkillKey{id, 1}], skills[SkillKey{id, 5}]
		resolve := func(rank int32) StatusDef {
			d, err := EvaluateV5Skill(a, b, rank)
			if err != nil {
				t.Fatal(err)
			}
			app := d.Statuses[0]
			st, ok := statuses.ResolveSkillApplication(app)
			if !ok {
				t.Fatal(id, rank, "unresolved")
			}
			return st
		}
		cap, first, next := resolve(p.ProtectionCapRank), resolve(p.ProtectionCapRank+1), resolve(p.ProtectionCapRank+2)
		// Legacy applications use their authored duration override at runtime.
		if p.ProtectionCapRank == 5 && b.Statuses[0].DurationSec > 0 {
			cap.DurationSec = b.Statuses[0].DurationSec
		}
		if first.DurationSec <= cap.DurationSec || next.DurationSec <= first.DurationSec {
			t.Fatal("protection duration frozen", id, cap.DurationSec, first.DurationSec, next.DurationSec)
		}
		high := resolve(100)
		if high.PhysicalDamageReductionPct > 75 || high.MagicDamageReductionPct > 75 {
			t.Fatal("advertised immunity", id, high)
		}
		if id == 10114 && !high.BlockHarmful {
			t.Fatal("lock lost original immunity")
		}
		if id == 10330 && high.PhysicalReflectPct != 100 {
			t.Fatal("reflection wrongly capped at75", high)
		}
		if id == 10435 && first.FreezeAttackerChance != 100 || id == 10435 && first.FreezeAttackerEffectHitBP != 500 {
			t.Fatal("ice retaliation growth", first)
		}
	}
	a, b := skills[SkillKey{10018, 1}], skills[SkillKey{10018, 5}]
	for _, rank := range []int32{6, 20, 100} {
		d, err := EvaluateV5Skill(a, b, rank)
		if err != nil {
			t.Fatal(err)
		}
		st, ok := statuses.ResolveSkillApplication(d.Statuses[0])
		if !ok || st.DurationSec != 13 || st.Affixes[0].Value != 50 || d.ManaCostReductionRanks != rank-5 {
			t.Fatal("charge must grow only efficiency", rank, d, st)
		}
	}
}

func TestSkillGrowthAssassinationPoisonAndUncappedVulnerability(t *testing.T) {
	skills, statuses := realV5CurveAnchors(t), originalStatusScalingTable(t)
	for _, rank := range []int32{6, 20, 100} {
		d, err := EvaluateV5Skill(skills[SkillKey{10210, 1}], skills[SkillKey{10210, 5}], rank)
		if err != nil {
			t.Fatal(err)
		}
		fx := d.DirectEffects()[1]
		if fx.ChanceBP != 1300 || fx.EffectHitBP != (rank-5)*500 || !fx.RequiresPreviousLanded {
			t.Fatal("assassination base odds changed", rank, fx)
		}
		d, err = EvaluateV5Skill(skills[SkillKey{10211, 1}], skills[SkillKey{10211, 5}], rank)
		if err != nil {
			t.Fatal(err)
		}
		st, ok := statuses.ResolveSkillApplication(d.Statuses[0])
		if !ok || st.PoisonOnHitChance != 50 || st.PoisonOnHitLevel != 25 || st.PoisonOnHitFirstLevel != 21 || st.PoisonOnHitProgressionRanks != rank-5 {
			t.Fatal("poison carrier", st)
		}
		tick, ok := statuses.ResolveSkillApplication(StatusApplication{ID: StatusPoison, Level: st.PoisonOnHitLevel, ProgressionFirstLevel: st.PoisonOnHitFirstLevel, ProgressionRanks: st.PoisonOnHitProgressionRanks, ProgressionFirstDurationSec: st.PoisonOnHitFirstDurationSec, ProgressionBaseDurationSec: st.PoisonOnHitBaseDurationSec})
		base, _ := statuses.Get(StatusPoison, 25)
		if !ok || tick.Affixes[0].Value >= base.Affixes[0].Value || tick.Level != base.Level || tick.DurationSec != base.DurationSec || tick.IntervalSec != base.IntervalSec {
			t.Fatal("poison damage did not grow with original timing", base, tick)
		}
	}
	st, ok := statuses.ResolveSkillApplication(StatusApplication{ID: 10012, Level: 5, ProgressionFirstLevel: 1, ProgressionRanks: 50})
	if !ok || st.PhysicalDamageTakenPct <= 100 || st.MagicDamageTakenPct <= 100 {
		t.Fatal("vulnerability capped", st)
	}
}

func TestStealthDoesNotInventSpeedAfterPenaltyReachesZero(t *testing.T) {
	skills, statuses := realV5CurveAnchors(t), originalStatusScalingTable(t)
	d, err := EvaluateV5Skill(skills[SkillKey{10208, 1}], skills[SkillKey{10208, 5}], 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range d.Statuses[0].ExtraAffixes {
		if a.Attr == AttrMoveSpeed && a.Value != 0 {
			t.Fatal("invented stealth speed", a)
		}
	}
	st, ok := statuses.ResolveSkillApplication(d.Statuses[0])
	if !ok {
		t.Fatal("missing stealth")
	}
	for _, a := range st.Affixes {
		if a.Attr == AttrMoveSpeed && a.Value != 0 {
			t.Fatal("invented stored stealth speed", a)
		}
	}
}

func TestPeriodicPassiveBonusAppliesToOriginalSkillRowsWithoutMutation(t *testing.T) {
	table := originalStatusScalingTable(t)
	base, _ := table.Get(1036, 1)
	for _, ranks := range []int32{0, 1, 10} {
		got, ok := table.ResolveSkillApplication(StatusApplication{ID: 1036, Level: 1, ProgressionFirstLevel: 1, ProgressionRanks: ranks, PeriodicDamageBonusPct: 10})
		plain, plainOK := table.ResolveSkillApplication(StatusApplication{ID: 1036, Level: 1, ProgressionFirstLevel: 1, ProgressionRanks: ranks})
		if !ok || !plainOK || got.Affixes[0].Value >= plain.Affixes[0].Value || got.DurationSec != plain.DurationSec || got.IntervalSec != plain.IntervalSec {
			t.Fatal("periodic passive bonus skipped", ranks, plain, got)
		}
	}
	after, _ := table.Get(1036, 1)
	if after.Affixes[0].Value != base.Affixes[0].Value {
		t.Fatal("stored tick mutated")
	}
}

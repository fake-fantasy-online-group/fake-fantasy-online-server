package domain

import "math"

// SkillGrowthPolicy identifies the deliberately scoped continuation after an
// original mechanic is complete. Zero means keep that skill's original curve.
// This table supplements, rather than replaces, the 149 immutable skill rows.
type SkillGrowthPolicy struct {
	Attribute         int32
	AttributeCapRank  int32
	EfficiencyCapRank int32
	RangeCapRank      int32
	ProtectionCapRank int32
	AttackCapRank     int32
	AttackTargets     []SkillID
}

var skillGrowthPolicies = map[SkillID]SkillGrowthPolicy{
	10018: {EfficiencyCapRank: 5},
	10227: {EfficiencyCapRank: 5},
	10432: {EfficiencyCapRank: 5},
	10301: {EfficiencyCapRank: 19},
	10305: {EfficiencyCapRank: 11},
	10309: {EfficiencyCapRank: 10},
	10024: {Attribute: AttrVIT, AttributeCapRank: 4},
	10222: {Attribute: AttrVIT, AttributeCapRank: 10},
	10229: {Attribute: AttrVIT, AttributeCapRank: 11},
	10230: {Attribute: AttrVIT, AttributeCapRank: 11},
	10314: {Attribute: AttrINT, AttributeCapRank: 17},
	10415: {Attribute: AttrINT, AttributeCapRank: 17},
	10416: {Attribute: AttrINT, AttributeCapRank: 5},
	10329: {Attribute: AttrINT, AttributeCapRank: 5},
	10420: {Attribute: AttrINT, AttributeCapRank: 5},
	10306: {Attribute: AttrSPI, AttributeCapRank: 5},
	10316: {Attribute: AttrSPI, AttributeCapRank: 5},
	10318: {Attribute: AttrSPI, AttributeCapRank: 5},
	10433: {Attribute: AttrSPI, AttributeCapRank: 5},
	10218: {Attribute: AttrSPI, AttributeCapRank: 7},
	10028: {RangeCapRank: 5},
	10029: {RangeCapRank: 11},
	10128: {RangeCapRank: 11},
	10413: {RangeCapRank: 6},
	10127: {RangeCapRank: 5},
	10228: {RangeCapRank: 8},
	10304: {RangeCapRank: 20},
	10311: {RangeCapRank: 17},
	10315: {RangeCapRank: 39},
	10321: {RangeCapRank: 23},
	10327: {RangeCapRank: 11},
	10107: {ProtectionCapRank: 23},
	10114: {ProtectionCapRank: 5},
	10417: {ProtectionCapRank: 15},
	10425: {ProtectionCapRank: 20},
	10435: {ProtectionCapRank: 15},
	10330: {ProtectionCapRank: 10},
	10201: {AttackCapRank: 5, AttackTargets: []SkillID{10209}},
	10019: {AttackCapRank: 6, AttackTargets: []SkillID{10017}},
	10020: {AttackCapRank: 17, AttackTargets: []SkillID{10006}},
	10110: {AttackCapRank: 10, AttackTargets: []SkillID{10102}},
	10220: {AttackCapRank: 11, AttackTargets: []SkillID{10210}},
	10226: {AttackCapRank: 19, AttackTargets: []SkillID{10225}},
	10423: {AttackCapRank: 5, AttackTargets: []SkillID{10405}},
	10422: {AttackCapRank: 15, AttackTargets: []SkillID{10421}},
	10427: {AttackCapRank: 9, AttackTargets: []SkillID{10409, 10410, 10411, 10412}},
	10430: {AttackCapRank: 25, AttackTargets: []SkillID{10412}},
}

// LookupSkillGrowthPolicy covers every catalogue member, including skills whose
// uncapped original effect is their entire progression. Returned slices are owned
// by the caller so UI/export code cannot mutate the runtime policy table.
func LookupSkillGrowthPolicy(id SkillID) (SkillGrowthPolicy, bool) {
	if !HasV5SkillCurve(id) {
		return SkillGrowthPolicy{}, false
	}
	p := skillGrowthPolicies[id]
	p.AttackTargets = append([]SkillID(nil), p.AttackTargets...)
	return p, true
}

// SkillGrowthAttribute is one derived point per effective rank after the last
// meaningful original cap. Callers enforce learned/profession eligibility and
// recompute from current ranks; it is never a cast reward or persisted stat.
func SkillGrowthAttribute(id SkillID, effectiveRank int32) (attr, value int32) {
	p := skillGrowthPolicies[id]
	if p.Attribute == 0 || effectiveRank <= p.AttributeCapRank {
		return 0, 0
	}
	return p.Attribute, effectiveRank - p.AttributeCapRank
}

// SkillGrowthDamageBonusPct enhances only the attack already named by a passive.
// It never enhances basic attacks or another skill. Periodic-only attacks apply
// this to their original damage tick rather than inventing direct damage.
func SkillGrowthDamageBonusPct(source SkillID, rank int32, target SkillID) int32 {
	p := skillGrowthPolicies[source]
	if p.AttackCapRank == 0 || rank <= p.AttackCapRank {
		return 0
	}
	for _, id := range p.AttackTargets {
		if target == id {
			return SaturateInt32(2 * int64(rank-p.AttackCapRank))
		}
	}
	return 0
}

const SkillGrowthEffectHitPerRankBP int32 = 500

func skillGrowthEffectHit(ranks int32) int32 {
	return SaturateInt32(int64(max(int32(0), ranks)) * int64(SkillGrowthEffectHitPerRankBP))
}

// A two-percent original-radius increase per rank reaches twice the original
// radius after 50 ranks. Thereafter a continuous square-root curve progressively
// reduces the marginal increase without introducing a hard maximum.
func (e *v5Evaluator) growthRadius(base, ranks int32) int32 {
	if base <= 0 || ranks <= 0 {
		return base
	}
	if ranks <= 50 {
		return e.checked(int64(base)*(50+int64(ranks))/50, "support area radius")
	}
	factor := 2 * math.Sqrt(float64(ranks)/50)
	return e.checked(int64(math.Floor(float64(base)*factor)), "support area radius")
}

func (e *v5Evaluator) applyGrowthPolicy(a, b SkillDef, out *SkillDef) {
	p := skillGrowthPolicies[e.id]
	if p.EfficiencyCapRank > 0 && e.rank > p.EfficiencyCapRank {
		anchor := v5Evaluator{id: e.id, rank: p.EfficiencyCapRank}
		out.MPCost = max(int32(0), anchor.blend(a.MPCost, b.MPCost))
		out.ManaCostReductionRanks = e.rank - p.EfficiencyCapRank
		baseCD := max(int32(0), anchor.blend(a.CooldownMS, b.CooldownMS))
		if baseCD > 0 {
			n, d := int64(baseCD)*50, 50+int64(out.ManaCostReductionRanks)
			out.CooldownMS = int32((n + d - 1) / d)
		}
		if anchor.err != nil {
			e.fail(anchor.err.Error())
		}
	}
	if p.RangeCapRank > 0 && e.rank > p.RangeCapRank && b.Kind == SkillArea && !b.HasDamageEffect() {
		ranks := e.rank - p.RangeCapRank
		out.Radius = e.growthRadius(b.Radius, ranks)
		out.Area.Radius = e.growthRadius(b.Area.Radius, ranks)
		out.Area.Length = e.growthRadius(b.Area.Length, ranks)
		out.Area.Width = e.growthRadius(b.Area.Width, ranks)
		for i := range out.Effects {
			if out.Effects[i].Kind == EffectRevealInvisible {
				out.Effects[i].RevealRadius = e.growthRadius(b.DirectEffects()[i].RevealRadius, ranks)
			}
		}
	}
}

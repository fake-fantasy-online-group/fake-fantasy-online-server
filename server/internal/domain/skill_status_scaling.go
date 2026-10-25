package domain

import "math"

// ResolveSkillApplication resolves the original status row and, only above the
// original five skill ranks, continues its existing numerical effects. Status
// identifiers, levels, intervals and boolean mechanics remain the stored ones.
// Application-specific overrides are applied by the caller after resolution.
func (t StatusTable) ResolveSkillApplication(app StatusApplication) (StatusDef, bool) {
	fifth, ok := t.Get(app.ID, app.Level)
	if !ok {
		return fifth, false
	}
	if app.ProgressionRanks <= 0 {
		if elementalShieldStatus(app.SourceSkill, app.ID) && app.DurationSec > 0 {
			fifth.DurationSec = app.DurationSec
		}
		return applyPeriodicSkillDamageBonus(fifth, app.PeriodicDamageBonusPct)
	}
	first, ok := t.Get(app.ID, app.ProgressionFirstLevel)
	if !ok {
		return StatusDef{}, false
	}
	d := fifth
	ranks := app.ProgressionRanks
	valid := true
	grow := func(a, b int32) int32 {
		value, ok := continueStatusValue(a, b, ranks)
		valid = valid && ok
		return value
	}
	magnitude := func(a, b int32) int32 {
		value, ok := continueStatusMagnitude(a, b, ranks)
		valid = valid && ok
		return value
	}
	pct := func(a, b int32) int32 {
		return int32(min(int64(100), max(int64(0), continuedStatusWide(a, b, ranks, false))))
	}
	reflectPct := func(a, b int32) int32 {
		return int32(min(int64(100), max(int64(0), continuedStatusWide(a, b, ranks, true))))
	}

	d.Affixes = append([]Affix(nil), fifth.Affixes...)
	for i, b := range fifth.Affixes {
		// Pair effects by attribute and mode, never by their incidental ordering.
		for _, a := range first.Affixes {
			if a.Attr != b.Attr || a.Mode != b.Mode {
				continue
			}
			periodic := b.Attr == AttrHPRegen && d.IntervalSec > 0
			v := continuedStatusWide(a.Value, b.Value, ranks, periodic)
			if b.Value == 0 {
				v = 0
			}
			if b.Mode == ModePercent {
				// A percentage penalty cannot become negative underlying stats.
				// Preserve any already-authored exceptional boundary unchanged.
				v = max(min(int64(-100), int64(b.Value)), v)
				if b.Attr == AttrMoveSpeed || b.Attr == AttrAtkSpeed || periodic {
					v = min(max(int64(100), int64(b.Value)), v)
				}
			}
			if v > math.MaxInt32 || v < math.MinInt32 {
				valid = false
			} else {
				d.Affixes[i].Value = int32(v)
			}
			break
		}
	}
	d.HPChanges, ok = continueResourceChanges(first.HPChanges, fifth.HPChanges, ranks)
	valid = valid && ok
	d.MPChanges, ok = continueResourceChanges(first.MPChanges, fifth.MPChanges, ranks)
	valid = valid && ok
	d.PhysicalDamageReductionPct = min(int32(75), pct(first.PhysicalDamageReductionPct, fifth.PhysicalDamageReductionPct))
	d.MagicDamageReductionPct = min(int32(75), pct(first.MagicDamageReductionPct, fifth.MagicDamageReductionPct))
	d.PhysicalDamageTakenPct = grow(first.PhysicalDamageTakenPct, fifth.PhysicalDamageTakenPct)
	d.MagicDamageTakenPct = grow(first.MagicDamageTakenPct, fifth.MagicDamageTakenPct)
	d.PhysicalReflectPct = reflectPct(first.PhysicalReflectPct, fifth.PhysicalReflectPct)
	d.MagicReflectPct = reflectPct(first.MagicReflectPct, fifth.MagicReflectPct)
	d.MagicFullReflectChancePct = pct(first.MagicFullReflectChancePct, fifth.MagicFullReflectChancePct)
	d.PhysicalReflectFlat = magnitude(first.PhysicalReflectFlat, fifth.PhysicalReflectFlat)
	d.MagicReflectFlat = magnitude(first.MagicReflectFlat, fifth.MagicReflectFlat)
	d.DamageShield = magnitude(first.DamageShield, fifth.DamageShield)
	d.StealthDamagePct = grow(first.StealthDamagePct, fifth.StealthDamagePct)
	d.PoisonOnHitChance = pct(first.PoisonOnHitChance, fifth.PoisonOnHitChance)
	if app.SourceSkill == 10211 {
		d.PoisonOnHitChance = fifth.PoisonOnHitChance
		d.PoisonOnHitProgressionRanks = ranks
		d.PoisonOnHitFirstLevel = first.PoisonOnHitLevel
		// Preserve the original poison's duration and tick interval. The runtime
		// resolves its original row with these ranks, never a fabricated level.
		if poison, ok := t.Get(StatusPoison, fifth.PoisonOnHitLevel); ok {
			d.PoisonOnHitFirstDurationSec = poison.DurationSec
			d.PoisonOnHitBaseDurationSec = poison.DurationSec
		}
	}
	d.FreezeAttackerChance = pct(first.FreezeAttackerChance, fifth.FreezeAttackerChance)
	d.NextBasicAttackPct = magnitude(first.NextBasicAttackPct, fifth.NextBasicAttackPct)
	d.NextSkillDamagePct = magnitude(first.NextSkillDamagePct, fifth.NextSkillDamagePct)
	d.NextStatusDurationPct = grow(first.NextStatusDurationPct, fifth.NextStatusDurationPct)
	d.NextCastTimeReductionPct = pct(first.NextCastTimeReductionPct, fifth.NextCastTimeReductionPct)
	d.LifeStealPct = pct(first.LifeStealPct, fifth.LifeStealPct)
	d.LifeStealChance = pct(first.LifeStealChance, fifth.LifeStealChance)
	d.ManaStealPct = pct(first.ManaStealPct, fifth.ManaStealPct)
	d.ManaStealChance = pct(first.ManaStealChance, fifth.ManaStealChance)
	d.MaxHPFloor = grow(first.MaxHPFloor, fifth.MaxHPFloor)
	d.ExperienceBonusPct = grow(first.ExperienceBonusPct, fifth.ExperienceBonusPct)
	d.OutgoingMagicDamageFlat = grow(first.OutgoingMagicDamageFlat, fifth.OutgoingMagicDamageFlat)
	d.MagicDamageReductionFlat = grow(first.MagicDamageReductionFlat, fifth.MagicDamageReductionFlat)
	d.ManaToHealthRate = grow(first.ManaToHealthRate, fifth.ManaToHealthRate)

	firstDuration, baseDuration := first.DurationSec, fifth.DurationSec
	if app.ProgressionFirstDurationSec > 0 {
		firstDuration = app.ProgressionFirstDurationSec
	}
	if app.ProgressionBaseDurationSec > 0 {
		baseDuration = app.ProgressionBaseDurationSec
	}
	// Passive/unique modifiers run after curve evaluation. Their explicit
	// source-duration adjustment is added once to the continued base duration.
	var durationAdjustment int64
	if app.DurationSec > 0 {
		durationAdjustment = int64(app.DurationSec) - int64(baseDuration)
	}
	// Apply source-only control semantics before choosing the duration boundary.
	timing := fifth
	if app.Control != ControlNone {
		timing.Control = app.Control
	}
	timing.BlockHarmful = timing.BlockHarmful || app.BlockHarmful
	d.DurationSec, d.ControlPower = continueSkillStatusDuration(timing, firstDuration, baseDuration, ranks)
	d.EffectHitBP = skillGrowthEffectHit(d.ControlPower)
	if app.SourceSkill == 10018 {
		// The new movement policy freezes charge's rank-five duration/speed.
		d.DurationSec = baseDuration
		d.ControlPower, d.EffectHitBP = 0, 0
	}
	policy, _ := app.Progression.Policy(app.SourceSkill)
	if elementalShieldStatus(app.SourceSkill, app.ID) {
		// Protection chance caps do not cap elemental shield lifetime.
		value := 30 * (int64(ranks) + 5)
		if value > math.MaxInt32 {
			valid = false
		} else {
			d.DurationSec = int32(value)
		}
	} else if policy.ProtectionCapRank > 0 && int64(ranks)+5 > int64(policy.ProtectionCapRank) {
		capRanks := policy.ProtectionCapRank - 5
		atCap, _ := continueSkillStatusDuration(timing, firstDuration, baseDuration, capRanks)
		step := max(int32(1), baseDuration/50)
		next := int64(atCap) + int64(ranks-capRanks)*int64(step)
		if next > math.MaxInt32 {
			valid = false
		} else {
			d.DurationSec = int32(next)
		}
	}
	if app.SourceSkill == 10435 {
		d.FreezeAttackerEffectHitBP = skillGrowthEffectHit(max(int32(0), ranks-10))
	}
	if durationAdjustment != 0 {
		cap := skillStatusDurationCap(timing, baseDuration)
		if policy.ProtectionCapRank > 0 || elementalShieldStatus(app.SourceSkill, app.ID) {
			cap = math.MaxInt32
		}
		value := min(int64(cap), int64(d.DurationSec)+durationAdjustment)
		if value < 1 || value > math.MaxInt32 {
			valid = false
		} else {
			d.DurationSec = int32(value)
		}
	}
	d, ok = applyPeriodicSkillDamageBonus(d, app.PeriodicDamageBonusPct)
	valid = valid && ok
	return d, valid
}

// A high-rank damage-enhancing passive also applies when the target spell is
// still within its original five ranks. Always clone the stored tick slices.
func applyPeriodicSkillDamageBonus(d StatusDef, bonus int32) (StatusDef, bool) {
	if bonus <= 0 || d.IntervalSec <= 0 {
		return d, true
	}
	d.Affixes = append([]Affix(nil), d.Affixes...)
	d.HPChanges = append([]ResourceChange(nil), d.HPChanges...)
	valid := true
	boost := func(v int32, mode int32) int32 {
		if v >= 0 {
			return v
		}
		wide := int64(v) * (100 + int64(bonus))
		n := wide / 100
		if mode == ModeAbsolute {
			d.PeriodicDamageFraction100 += (-wide) % 100
		}
		if n < math.MinInt32 {
			valid = false
			return 0
		}
		return int32(n)
	}
	for i := range d.Affixes {
		if d.Affixes[i].Attr == AttrHPRegen {
			d.Affixes[i].Value = boost(d.Affixes[i].Value, d.Affixes[i].Mode)
		}
	}
	for i := range d.HPChanges {
		d.HPChanges[i].Value = boost(d.HPChanges[i].Value, d.HPChanges[i].Mode)
	}
	return d, valid
}

// Continue the original duration slope before trading further control ranks for
// local effect accuracy. An original duration above a safety boundary is retained.
func continueSkillStatusDuration(d StatusDef, first, fifth, ranks int32) (duration, pressure int32) {
	if first <= 0 || fifth <= 0 || ranks <= 0 {
		return fifth, 0
	}
	harmfulControl := d.Harmful() && d.IntervalSec == 0
	cap := skillStatusDurationCap(d, fifth)
	if first == fifth {
		if harmfulControl {
			pressure = ranks
		}
		return fifth, pressure
	}
	value := int64(fifth) + (int64(fifth)-int64(first))*int64(ranks)/4
	duration = int32(min(int64(cap), max(int64(1), value)))
	if harmfulControl && fifth > first {
		// Ceiling division locates the first added rank that reaches the cap;
		// only ranks after that boundary become resistance pressure.
		numerator := (int64(cap) - int64(fifth)) * 4
		slope := int64(fifth) - int64(first)
		ranksToCap := (numerator + slope - 1) / slope
		pressure = int32(max(int64(0), int64(ranks)-ranksToCap))
	}
	return duration, pressure
}

// SkillApplicationDurationCap is the same original-source boundary used by the
// resolver. Scene-level duration multipliers must respect it as their last step.
// Original ranks return zero so their existing runtime behavior is unchanged.
func (t StatusTable) SkillApplicationDurationCap(app StatusApplication) int32 {
	if app.ProgressionRanks <= 0 {
		return 0
	}
	d, ok := t.Get(app.ID, app.Level)
	if !ok {
		return 0
	}
	base := d.DurationSec
	if app.ProgressionBaseDurationSec > 0 {
		base = app.ProgressionBaseDurationSec
	}
	if app.Control != ControlNone {
		d.Control = app.Control
	}
	d.BlockHarmful = d.BlockHarmful || app.BlockHarmful
	if policy, _ := app.Progression.Policy(app.SourceSkill); policy.ProtectionCapRank > 0 || elementalShieldStatus(app.SourceSkill, app.ID) {
		return math.MaxInt32
	}
	if app.SourceSkill == 10018 {
		return base
	}
	return skillStatusDurationCap(d, base)
}

func skillStatusDurationCap(d StatusDef, fifth int32) int32 {
	harmfulControl := d.Harmful() && d.IntervalSec == 0
	cap := int32(3600)
	if d.Invulnerable || d.BlockHarmful || (!harmfulControl && (d.Control != ControlNone || d.BlockMove)) {
		cap = fifth
	} else if d.Harmful() {
		switch {
		case d.BreakOnDamage && d.Control != ControlNone:
			cap = 60
		case d.Control == ControlSilence || d.CannotAttack || d.AttackCapKnown || statusDisarms(d):
			cap = 20
		case d.Control == ControlStun || d.BlockMove || d.Immobile != 0:
			cap = 15
		default:
			cap = 60
		}
	}
	return max(cap, fifth)
}

func statusDisarms(d StatusDef) bool {
	for _, affix := range d.Affixes {
		if affix.Attr == AttrAtk && affix.Mode == ModePercent && affix.Value <= -100 {
			return true
		}
	}
	return false
}

// Continue the skill's first-to-fifth slope over four skill-rank intervals.
// A zero-valued fifth-rank effect stays absent, and a decreasing effect cannot
// cross through zero and reverse its original meaning.
func continueStatusValue(first, fifth, ranks int32) (int32, bool) {
	value := continuedStatusWide(first, fifth, ranks, false)
	if value > math.MaxInt32 || value < math.MinInt32 {
		return 0, false
	}
	return int32(value), true
}

func continuedStatusWide(first, fifth, ranks int32, constantMagnitude bool) int64 {
	if ranks <= 0 || fifth == 0 {
		return int64(fifth)
	}
	v := int64(fifth) + (int64(fifth)-int64(first))*int64(ranks)/4
	if first == fifth && constantMagnitude {
		// Whole-number status payloads cannot carry a fractional tick or
		// shield point. Preserve the 2% slope where it advances each rank;
		// otherwise give the existing magnitude one native unit per rank.
		// Do not apply this to chances, durations, or absent effects.
		step := max(int64(ranks), absStatusWide(int64(fifth))*int64(ranks)/50)
		if fifth > 0 {
			v = int64(fifth) + step
		} else {
			v = int64(fifth) - step
		}
	}
	if fifth > 0 {
		return max(int64(0), v)
	}
	return min(int64(0), v)
}

func absStatusWide(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func continueStatusMagnitude(first, fifth, ranks int32) (int32, bool) {
	// Only existing damage/healing/shield magnitudes use at least 2% continuation when
	// their legacy slope is zero. Chances, timing and mechanics never use it.
	value := continuedStatusWide(first, fifth, ranks, true)
	if value > math.MaxInt32 || value < math.MinInt32 {
		return 0, false
	}
	return int32(value), true
}

func continueResourceChanges(first, fifth []ResourceChange, ranks int32) ([]ResourceChange, bool) {
	out := append([]ResourceChange(nil), fifth...)
	if len(first) != len(fifth) {
		return out, true
	}
	for i, b := range fifth {
		if first[i].Mode != b.Mode {
			continue
		}
		value := continuedStatusWide(first[i].Value, b.Value, ranks, true)
		if b.Mode == ModePercent {
			value = min(int64(100), max(int64(-100), value))
		}
		if value > math.MaxInt32 || value < math.MinInt32 {
			return nil, false
		}
		out[i].Value = int32(value)
	}
	return out, true
}

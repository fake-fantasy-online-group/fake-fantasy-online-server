package domain

// ChallengeMonsterBehavior changes movement and disengagement only. Native AI,
// spells, passive movement locks, immunities and all combat stats stay intact.
// It is idempotent because trial templates are also used by legacy rifts/clones.
func ChallengeMonsterBehavior(d MonsterDef) MonsterDef {
	if d.challengeBehaviorApplied {
		return d
	}
	d.challengeBehaviorApplied = true
	d.Stats.MoveSpeed = SaturateInt32(int64(max(int32(0), d.Stats.MoveSpeed)) * 3 / 2)
	trace := d.AI.TraceDist
	if d.AI.ID == "" {
		trace = d.TraceDist
	}
	if trace <= 0 && d.Kind.Hostile() {
		trace = 600
	}
	trace = SaturateInt32(int64(trace) * 3)
	d.AI.TraceDist, d.TraceDist = trace, trace
	return d
}

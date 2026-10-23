package domain

// LegendarySkillModifiers is one additive equipment bucket per parameter.
// TransformSkill determines structure first; BaseSkill supplies flat additions;
// PrepareSkill fills these fields before one final application. Applied values
// preserve the accepted snapshot and permit target-specific additions without
// multiplying the complete bonus twice.
type LegendarySkillModifiers struct {
	UniversalSingleCircle                                    bool
	LifeStealBP                                              int32
	TrapRadiusBP, PrepareReductionBP, HealingBP, RestoreMPBP int32
	ManaDrainBP                                              int32
	DamageBP, RadiusBP, AngleBP, WidthBP, LengthBP           int32
	NativeManaReductionBP                                    int32
	StatusPowerBP                                            int32
	ManaReductionBP, CooldownReductionBP                     int32
	ConservationDamagePerReductionBP                         int32
	AppliedDamageBP                                          int32
	GeometryInherited                                        bool
	Applied                                                  bool
}

// These modifiers belong to the existing native status instance. They never
// create another periodic effect, extend duration or change the source owner.
type LegendaryStatusModifiers struct {
	DurationBP, SlowMagnitudeBP, DefenseLossReductionBP, AttackSlowReductionBP int32
	EffectHitBonusBP, LifeStealBP, PoisonChancePct                             int32
	PeriodicCritChanceBP, PeriodicCritBonusBP                                  int32
	PeriodicCanCrit                                                            bool
	MoveSpeedBonusPct, DurationBonusMS                                         int32
	DamageTickBP, HealingTickBP, ShieldBP, BuffMagnitudeBP                     int32
	ControlDurationReductionBP, SlowMagnitudeReductionBP                       int32
	Applied                                                                    bool
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Ground fields stay ground fields: one pulse per owner/power/target/second,
// never a second attached DOT. Leaving the area stops damage. Because there is
// no target-owned remainder, cashout/spread cannot consume/copy this source.
// Growth, outgoing periodic pool and duration are frozen once at release and
// each pulse crosses the ordinary periodic defense/shield path once.
func (s *Scene) synergyGroundSnapshot(c *legendaryContext, amount, seconds int32) *domain.StatusDef {
	d := domain.StatusDef{ID: legendaryMeleeStatusID(c, "ground-dot"), Level: 1,
		Name: c.Power.Name, Desc: c.Power.Effect, TypeKnown: true, Type: domain.StatusBad,
		DurationSec: seconds, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -amount, Mode: domain.ModeAbsolute}},
		CombatCausalPath: c.effectPath(), LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID, SkillApplied: true, SourceSkillID: int32(c.Skill),
		Icon: legendaryMeleeIcon(c), IconKnown: true, SynergyDOTVersion: 1, SynergyDOTNew: true,
		SynergyDOTPhysical: c.Def.HurtType == domain.HurtPhysical || c.Def.HurtType == domain.HurtNone,
		SynergyDOTHurtType: c.Def.HurtType}
	s.legendaryCasterStatusApplying(c.Owner.ID, nil, &d)
	s.legendaryFinalizeStatus(&d)
	// Native damage already consumed the source critical roll once at release.
	d.LegendaryModifiers.PeriodicCanCrit = false
	return &d
}

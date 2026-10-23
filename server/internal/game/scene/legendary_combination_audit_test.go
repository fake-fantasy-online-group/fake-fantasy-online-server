package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestAuditChildInheritsShieldEfficiency(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-A16")
	d := domain.SkillDef{ID: 10011, Kind: domain.SkillSingle, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{ID: 999, DamageShield: 100, DurationSec: 10}, s.tick, p.ID)
	before := m.HP
	if !c.ChildSkill(m.ID, d, legendaryChildOptions{TargetOnly: true, SuppressGeneration: true}) {
		t.Fatal("child rejected")
	}
	if got := before - m.HP; got != 50 {
		t.Fatalf("child HP damage=%d want50: TY-A16 should consume 100 shield as 50 base damage", got)
	}
}

func TestAuditChildReceivingControlReduction(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-D09")
	s.legendaryState().depth = 1
	s.legendaryState().child = &legendaryChildFrame{Depth: 1}
	d := domain.StatusDef{ID: 998, DurationSec: 5, BlockMove: true}
	if !s.legendaryStatusReceiving(p, m, &d) {
		t.Fatal("unexpected rejection")
	}
	s.legendaryFinalizeStatus(&d)
	if d.LegendaryDurationMS != 3500 {
		t.Fatalf("child control duration=%d want3500", d.LegendaryDurationMS)
	}
}

func TestAuditChildScaleIncludesFlatDamage(t *testing.T) {
	_, _, m, c := universalTestContext(t, "TY-A16")
	d := domain.SkillDef{ID: 10011, Kind: domain.SkillSingle, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, DamageFlat: 100, GuaranteedHit: true}}}
	before := m.HP
	if !c.ChildSkill(m.ID, d, legendaryChildOptions{ScaleBP: 8000, TargetOnly: true, SuppressGeneration: true}) {
		t.Fatal("child rejected")
	}
	if got := before - m.HP; got != 160 {
		t.Fatalf("80%% child damage=%d want160 from 100 attack +100 flat", got)
	}
}

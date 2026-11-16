package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func bpMobilityDashEvent(t *testing.T, s *Scene, id domain.EntityID) event.EntityMoved {
	t.Helper()
	for _, o := range s.outbox {
		if e, ok := o.ev.(event.EntityMoved); ok && e.ID == id && e.Dash {
			return e
		}
	}
	t.Fatal("no same-map dash event", id)
	return event.EntityMoved{}
}

func bpMobilityPlace(s *Scene, e *entity.Entity, x float64) {
	s.aoi.Move(e, domain.Pos{MapID: e.Pos.MapID, X: x, Y: 100})
}

func TestBuffPassLanceChargeDashStunAndWindow(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{10017: 1, 10002: 1}, "ZS-U03")
	bpMobilityPlace(s, p, 100)
	bpMobilityPlace(s, m, 400)
	d, _ := s.skills.Get(10017, 1)
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Dist < 500 {
		t.Fatal("charge range", d.Dist)
	}
	st := domain.Stats{MoveSpeed: 100}
	s.legendaryPassiveStats(p, &st)
	if st.MoveSpeed != 100 {
		t.Fatal("old move-speed effect survived the replacement", st.MoveSpeed)
	}
	lance := domain.SkillDef{ID: 10002, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	before := lance
	s.legendaryPrepareSkill(p, &before, m.ID)
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10017}, m.ID, []domain.EntityID{m.ID})
	e := bpMobilityDashEvent(t, s, p.ID)
	if p.Pos.X >= m.Pos.X || m.Pos.X-p.Pos.X > 41 || e.DurationMS < dashMinMS || e.DurationMS > dashMaxMS {
		t.Fatal("charge did not land at the target's side", p.Pos, m.Pos, e.DurationMS)
	}
	if !s.dashControlImmune(p) {
		t.Fatal("charger not control-immune during the dash")
	}
	if m.Status == nil || m.Status.Controlled() != domain.ControlStun {
		t.Fatal("charge did not stun the target")
	}
	after := lance
	s.legendaryPrepareSkill(p, &after, m.ID)
	if after.LegendaryModifiers.DamageBP-before.LegendaryModifiers.DamageBP != 5000 {
		t.Fatal("lance window +50% missing", before.LegendaryModifiers.DamageBP, after.LegendaryModifiers.DamageBP)
	}
	bpMobilityPlace(s, m, p.Pos.X+700)
	from := p.Pos
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10017}, m.ID, []domain.EntityID{m.ID})
	if p.Pos != from {
		t.Fatal("charged beyond 500")
	}
}

func TestBuffPassVacuumLungeBehindTarget(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{10117: 1}, "JK-U01")
	bpMobilityPlace(s, p, 100)
	bpMobilityPlace(s, m, 400)
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10117}, m.ID, []domain.EntityID{m.ID})
	bpMobilityDashEvent(t, s, p.ID)
	if p.Pos.X <= m.Pos.X || p.Pos.X-m.Pos.X > 41 {
		t.Fatal("lunge did not land behind the target", p.Pos, m.Pos)
	}
	st := domain.Stats{MoveSpeed: 100}
	s.legendaryPassiveStats(p, &st)
	if st.MoveSpeed != 100 {
		t.Fatal("old move-speed effect survived the replacement")
	}
}

func TestBuffPassPhantomDistanceAndNextSpell(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10432: 1, 10401: 1}, "SS-U01")
	orig, _ := s.skills.Get(10432, 1)
	d := orig
	s.legendaryPrepareSkill(p, &d, 0)
	if d.Dist != specialScale(orig.Dist, 150) {
		t.Fatal("phantom distance +50%", orig.Dist, d.Dist)
	}
	spell := domain.SkillDef{ID: 10401, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	base := spell
	s.legendaryPrepareSkill(p, &base, m.ID)
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10432}, 0, nil)
	boosted := spell
	s.legendaryPrepareSkill(p, &boosted, m.ID)
	if boosted.LegendaryModifiers.DamageBP-base.LegendaryModifiers.DamageBP != 3000 {
		t.Fatal("next spell +30% missing", base.LegendaryModifiers.DamageBP, boosted.LegendaryModifiers.DamageBP)
	}
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10401}, m.ID, []domain.EntityID{m.ID})
	spent := spell
	s.legendaryPrepareSkill(p, &spent, m.ID)
	if spent.LegendaryModifiers.DamageBP != base.LegendaryModifiers.DamageBP {
		t.Fatal("next-spell bonus not consumed")
	}
}

func TestBuffPassPurifyRescueDashAndAllyGuard(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10306: 1}, "YS-U07")
	join(t, s, 2, 102, "ally", 500, 100)
	ally := s.players[2]
	bpMobilityPlace(s, p, 100)
	p.Player.Party, ally.Player.Party = 1, 1
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10306}, ally.ID, []domain.EntityID{ally.ID})
	bpMobilityDashEvent(t, s, p.ID)
	if ally.Pos.X-p.Pos.X > 41 || p.Pos.X >= ally.Pos.X {
		t.Fatal("rescue dash did not land beside the ally", p.Pos, ally.Pos)
	}
	if ally.Status.DamageReductionPct(false) < 25 || ally.Status.DamageReductionPct(true) < 25 {
		t.Fatal("cleansed ally lacks the 25% guard")
	}
	ally.Player.Party = 2
	bpMobilityPlace(s, ally, p.Pos.X+300)
	from := p.Pos
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10306}, ally.ID, []domain.EntityID{ally.ID})
	if p.Pos != from {
		t.Fatal("rescue dash to a non-party player")
	}
}

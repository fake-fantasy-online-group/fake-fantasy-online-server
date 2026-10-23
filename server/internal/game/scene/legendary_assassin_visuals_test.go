package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strings"
	"testing"
)

func assassinCustomEvents(s *Scene) (hits []event.SkillHitEffect, ground []event.GroundEffect) {
	for _, p := range s.outbox {
		switch e := p.ev.(type) {
		case event.SkillHitEffect:
			if strings.HasPrefix(e.Effect, "LP_") {
				hits = append(hits, e)
			}
		case event.GroundEffect:
			if strings.HasPrefix(e.Effect, "LP_") {
				ground = append(ground, e)
			}
		}
	}
	return
}
func TestLegendaryAssassinVisualSpreadCommittedOncePerRecipient(t *testing.T) {
	s, p, dead := legendaryAssassinFixture(t, "CK-A08")
	recipients := map[domain.EntityID]bool{}
	for i := 0; i < 7; i++ {
		v := putMonster(s, domain.EntityID(910+i), 10000, domain.Stats{}, float64(140+i), 100)
		v.Status = domain.NewStatusSet()
		s.monsters[v.ID] = v
	}
	poison := domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true, DurationSec: 10, IntervalSec: 2, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	s.applyStatusDef(dead, poison, p.ID)
	dead.HP = 0
	s.outbox = nil
	s.legendaryAssassinDeath(dead)
	hits, ground := assassinCustomEvents(s)
	if len(hits) != 5 || len(ground) != 0 {
		t.Fatalf("spread hits=%v ground=%v", hits, ground)
	}
	for _, h := range hits {
		if h.Effect != "LP_CK_A08_01" || recipients[h.Target] || !s.entities[h.Target].Status.Has(domain.StatusPoison) {
			t.Fatalf("invalid recipient flash %+v", h)
		}
		recipients[h.Target] = true
	}
	for id, target := range s.entities {
		if target.Alive() && target.Status != nil && target.Status.Has(domain.StatusPoison) && !recipients[id] {
			t.Fatalf("committed recipient %d missing flash", id)
		}
	}
	s.outbox = nil
	s.legendaryAssassinDeath(dead)
	hits, ground = assassinCustomEvents(s)
	if len(hits)+len(ground) != 0 {
		t.Fatal("same poison propagated visual twice")
	}
	c := assassinContext(s, p, dead, 10211, 1)
	legendaryAssassinCopyPoison(&c, dead, domain.Active{Def: poison, ExpireAt: s.tick})
	hits, _ = assassinCustomEvents(s)
	if len(hits) != 0 {
		t.Fatal("expired poison flashed")
	}
}
func TestLegendaryAssassinVisualCleanseOnlyWhenRemoved(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-D08")
	c := assassinContext(s, p, m, 10211, 1)
	s.outbox = nil
	legendaryAssassinCleanPoison(&c)
	hits, _ := assassinCustomEvents(s)
	if len(hits) != 0 {
		t.Fatal("empty cleanse flashed")
	}
	s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusPoison, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad}, m.ID)
	s.outbox = nil
	legendaryAssassinCleanPoison(&c)
	hits, ground := assassinCustomEvents(s)
	if len(hits) != 1 || hits[0].Target != p.ID || hits[0].Effect != "LP_CK_D08_01" || len(ground) != 0 {
		t.Fatalf("cleanse %+v %+v", hits, ground)
	}
	s.outbox = nil
	legendaryAssassinCleanPoison(&c)
	hits, _ = assassinCustomEvents(s)
	if len(hits) != 0 {
		t.Fatal("duplicate cleanse flash")
	}
}
func TestLegendaryAssassinVisualPalmCenterLifetimeAndEligibility(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A26")
	center := p.Pos
	s.outbox = nil
	d := domain.SkillDef{ID: 10212, Kind: domain.SkillSingle, TargetEnemy: true}
	s.legendaryAssassinTheftAttempt(p, m, d, false)
	hits, ground := assassinCustomEvents(s)
	if len(hits)+len(ground) != 0 {
		t.Fatal("ineligible theft flashed")
	}
	s.legendaryAssassinTheftAttempt(p, m, d, true)
	hits, ground = assassinCustomEvents(s)
	if len(hits) != 0 || len(ground) != 1 {
		t.Fatalf("palm visuals %v %v", hits, ground)
	}
	for _, pending := range s.outbox {
		if _, ok := pending.ev.(event.ChildSkillReleased); ok {
			t.Fatal("synthetic palm duplicated native theft art")
		}
	}
	g := ground[0]
	if g.Effect != "LP_CK_A26_01" || g.Owner != p.ID || g.Pos != center || g.MapID != center.MapID || g.DurationMS != 0 || g.Mode != 0 {
		t.Fatalf("palm must be one immediate caster-centered flash: %+v", g)
	}
}
func TestLegendaryAssassinVisualPullRequiresActualMovement(t *testing.T) {
	for _, near := range []bool{false, true} {
		s, p, m := legendaryAssassinFixture(t, "CK-A28")
		if !near {
			m.Pos.X = 300
			s.aoi.Move(m, m.Pos)
		}
		d := domain.SkillDef{ID: 10225, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		s.legendaryPrepareSkill(p, &d, m.ID)
		s.outbox = nil
		assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
		hits, ground := assassinCustomEvents(s)
		if near {
			if len(hits)+len(ground) != 0 {
				t.Fatal("unmoved target flashed")
			}
			continue
		}
		if len(hits) != 1 || hits[0].Target != m.ID || hits[0].Effect != "LP_CK_A28_01" || len(ground) != 0 {
			t.Fatalf("pull visuals %+v %+v", hits, ground)
		}
	}
}

func TestLegendaryAssassinVisualSpreadWithoutLivingRecipients(t *testing.T) {
	s, p, dead := legendaryAssassinFixture(t, "CK-A08")
	s.applyStatusDef(dead, domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true, DurationSec: 10, IntervalSec: 2, TypeKnown: true, Type: domain.StatusBad}, p.ID)
	dead.HP = 0
	s.outbox = nil
	s.legendaryAssassinDeath(dead)
	hits, ground := assassinCustomEvents(s)
	if len(hits)+len(ground) != 0 {
		t.Fatalf("corpse-only spread emitted visuals: %v %v", hits, ground)
	}
}

func TestLegendaryAssassinVisualPullImmuneTarget(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A28")
	m.Pos.X = 300
	s.aoi.Move(m, m.Pos)
	m.Status.Apply(domain.StatusDef{ID: 990001, DurationSec: 10, SuppressHarmful: true}, s.tick, m.ID)
	d := domain.SkillDef{ID: 10225, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	s.outbox = nil
	assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
	hits, ground := assassinCustomEvents(s)
	if m.Pos.X != 300 || len(hits)+len(ground) != 0 {
		t.Fatalf("immune pull produced motion/flash: x=%v hit=%v ground=%v", m.Pos.X, hits, ground)
	}
}
func TestLegendaryUniversalPresentationNeverResurrectsOldWorldStages(t *testing.T) {
	for _, power := range domain.LegendaryPowers() {
		if !strings.HasPrefix(power.ID, "TY-") {
			continue
		}
		t.Run(power.ID, func(t *testing.T) {
			s, p, m, _ := universalTestContext(t, power.ID)
			d := domain.SkillDef{ID: 123, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 100, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			s.outbox = nil
			s.legendaryPrepareSkill(p, &d, m.ID)
			assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
			hits, ground := assassinCustomEvents(s)
			if len(hits)+len(ground) != 0 {
				t.Fatalf("native-only power emitted independent world stage: %v %v", hits, ground)
			}
		})
	}
}

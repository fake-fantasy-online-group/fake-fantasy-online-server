package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"testing"
)

func TestSynergySpreadUncappedEveryInRadiusAndTerminal(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-A10")
	const targets = 256
	for i := 0; i < targets; i++ {
		putMonster(s, domain.EntityID(1000+i), 100000, domain.Stats{}, 140+float64(i%30), 100+float64(i/30))
	}
	outside := putMonster(s, 9999, 100000, domain.Stats{}, 1000, 1000)
	d := domain.StatusDef{ID: 1900099221, Level: 1, TypeKnown: true, Type: domain.StatusBad, SourceSkillID: 10002, IntervalSec: 1, DurationSec: 6, SynergyDOTVersion: 1, HPChanges: []domain.ResourceChange{{Value: -100}}}
	m.Status.Apply(d, s.tick, p.ID)
	var original domain.Active
	m.Status.Each(func(a domain.Active) { original = a })
	synergySpread(c, original)
	for i := 0; i < targets; i++ {
		e := s.entities[domain.EntityID(1000+i)]
		n := 0
		e.Status.Each(func(a domain.Active) {
			n++
			if !a.Def.SynergyDOTCopy || a.ExpireAt != original.ExpireAt || synergyTickPower(a, s.tick, e.MaxHP) != 50 {
				t.Fatalf("target %d diluted or invalid copy: %+v", i, a)
			}
			c.Target = e
			synergySpread(c, a)
		})
		if n != 1 {
			t.Fatalf("eligible target %d lost or recursive: %d", i, n)
		}
	}
	if outside.Status != nil && outside.Status.Has(d.ID) {
		t.Fatal("outside radius copied")
	}
	s.tick = original.ExpireAt + 1
	s.stepStatus()
	for i := 0; i < targets; i++ {
		if s.entities[domain.EntityID(1000+i)].Status.Has(d.ID) {
			t.Fatal("copy survived expiry")
		}
	}
}

func TestSynergySkillProcOneDrawAllNativeHitTargetsNoDilution(t *testing.T) {
	s, _, m, c := universalTestContext(t, "TY-A37")
	m.HP = 100000
	m.MaxHP = 100000
	c.Skill = 10002
	c.Blow = combat.Blow{Origin: domain.DamageNormalOrigin}
	c.Def = domain.SkillDef{ID: 10002}
	var amount int32
	for root := uint64(1); root < 100; root++ {
		c.RootID = root
		c.NowMS = int64(root) * 5000
		before := m.HP
		synergySkillProc(c)
		amount = before - m.HP
		if amount > 0 {
			break
		}
	}
	if amount <= 0 {
		t.Fatal("no admitted probability roll")
	}
	for i := 0; i < 256; i++ {
		e := putMonster(s, domain.EntityID(1000+i), 100000, domain.Stats{}, 140, 100)
		c.Target = e
		before := e.HP
		for hit := 0; hit < 12; hit++ {
			synergySkillProc(c)
		}
		if before-e.HP != amount {
			t.Fatalf("target %d lost, diluted or repeated per segment: %d want %d", i, before-e.HP, amount)
		}
	}
}

func TestSynergyCashoutUncappedAreaEachReceivesFullPayload(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-A01")
	m.HP = 100000
	m.MaxHP = 100000
	c.Skill = 10002
	c.RootID = 1
	c.Blow = combat.Blow{Origin: domain.DamageNormalOrigin}
	c.Def = domain.SkillDef{ID: 10002}
	for i := 0; i < 256; i++ {
		putMonster(s, domain.EntityID(1000+i), 100000, domain.Stats{}, 140, 100)
	}
	outside := putMonster(s, 9999, 100000, domain.Stats{}, 1000, 1000)
	d := domain.StatusDef{ID: 1900099222, Level: 1, SourceSkillID: 10002, IntervalSec: 1, DurationSec: 6, SynergyDOTVersion: 1, HPChanges: []domain.ResourceChange{{Value: -100}}}
	m.Status.Apply(d, s.tick, p.ID)
	synergyCashout(c)
	synergyCashout(c)
	for i := 0; i < 256; i++ {
		e := s.entities[domain.EntityID(1000+i)]
		if loss := int32(100000) - e.HP; loss != 210 {
			t.Fatalf("target %d cashout lost/diluted/recursive: %d", i, loss)
		}
	}
	if outside.HP != 100000 {
		t.Fatal("outside radius cashed out")
	}
}

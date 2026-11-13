package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
	"testing"
)

func TestSynergyNewProfessionSnapshots(t *testing.T) {
	for _, id := range []string{"ZS-A01", "JK-A01"} {
		t.Run(id, func(t *testing.T) {
			s, p, m, c := universalTestContext(t, id)
			c.Skill = 10002
			if id == "JK-A01" {
				c.Skill = 10102
			}
			c.RootID = 77
			c.Blow.Origin = domain.DamageNormalOrigin
			synergyProfessionDOT(c)
			synergyProfessionDOT(c)
			var a domain.Active
			count := 0
			m.Status.Each(func(x domain.Active) { a = x; count++ })
			if count != 1 || !a.Def.SynergyDOTPhysical || a.Def.Name == "" || a.Def.Icon <= 0 {
				t.Fatalf("snapshot %+v count%d", a.Def, count)
			}
			want := int64(300)
			if id == "JK-A01" {
				want = 360
			}
			if got := classSetPoisonRemaining(a, s.tick, m.MaxHP); got != want {
				t.Fatalf("base %d want%d", got, want)
			}
			oldNext := a.NextTickAt
			c.RootID++
			s.tick += domain.Ticks(100)
			synergyProfessionDOT(c)
			m.Status.Each(func(x domain.Active) {
				if x.NextTickAt != oldNext {
					t.Fatal("recast reset tick")
				}
			})
			_ = p
		})
	}
}
func TestSynergyGrowthEquipmentAndSharedPool(t *testing.T) {
	s, p, m, c := universalTestContext(t, "ZS-A01")
	warriorEquip(t, s, p, "TY-A18", domain.SlotRing)
	warriorEquip(t, s, p, "TY-A22", domain.SlotGlove)
	c.Skill = 10002
	c.RootID = 1
	c.Blow.Origin = domain.DamageNormalOrigin
	synergyProfessionDOT(c)
	var a domain.Active
	m.Status.Each(func(x domain.Active) { a = x })
	if a.Def.DurationSec != 12 || a.Def.LegendaryModifiers.DamageTickBP != 20000 {
		t.Fatalf("modifiers %+v", a.Def)
	}
	if got := synergyFrozenBalance(a, s.tick, m.MaxHP); got != 2400 {
		t.Fatalf("growth+amp+duration %d", got)
	}
	a.Def.LegendaryModifiers.DamageTickBP = 100000
	if got := s.legendaryPeriodicAmount(a, -100); got != -600 {
		t.Fatalf("pool cap %d", got)
	}
	a.Def.SynergyDOTVersion = 0
	if got := s.legendaryPeriodicAmount(a, -100); got != -100 {
		t.Fatalf("legacy equipment upgraded %d", got)
	}
}
func TestSynergyPhysicalTickAvoidsMagicDefense(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "ZS-A01")
	m.Stats.MDef = 10000
	m.Stats.PhysResist = 10
	d := domain.StatusDef{ID: 1900012345, IntervalSec: 1, DurationSec: 6, SynergyDOTVersion: 1, SynergyDOTPhysical: true, HPChanges: []domain.ResourceChange{{Value: -100}}}
	before := m.HP
	s.tickStatus(m, domain.Active{Source: p.ID, Def: d})
	if before-m.HP != 90 {
		t.Fatalf("physical loss%d", before-m.HP)
	}
	b := s.legendaryPeriodicBlow(domain.Active{Def: d})
	if b.Magic || b.HurtType != domain.HurtPhysical {
		t.Fatal("physical type not propagated")
	}
}
func TestSynergySpreadTerminalAndExpiry(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-A10")
	for i := 0; i < 8; i++ {
		putMonster(s, domain.EntityID(1000+i), 10000, domain.Stats{}, 140+float64(i), 100)
	}
	d := domain.StatusDef{ID: 1900012345, Level: 1, TypeKnown: true, Type: domain.StatusBad, SourceSkillID: 10002, IntervalSec: 1, DurationSec: 6, SynergyDOTVersion: 1, HPChanges: []domain.ResourceChange{{Value: -100}}}
	m.Status.Apply(d, s.tick, p.ID)
	var a domain.Active
	m.Status.Each(func(x domain.Active) { a = x })
	synergySpread(c, a)
	n := 0
	for i := 0; i < 8; i++ {
		target := s.entities[domain.EntityID(1000+i)]
		target.Status.Each(func(x domain.Active) {
			n++
			if !x.Def.SynergyDOTCopy || x.ExpireAt != a.ExpireAt || synergyTickPower(x, s.tick, target.MaxHP) != 50 {
				t.Fatal("invalid copy")
			}
			c.Target = target
			synergySpread(c, x)
		})
	}
	if n != 6 {
		t.Fatalf("spread targets%d", n)
	}
	s.tick = a.ExpireAt + 1
	s.stepStatus()
	for i := 0; i < 8; i++ {
		if s.entities[domain.EntityID(1000+i)].Status.Count() != 0 {
			t.Fatal("copy survived expiry")
		}
	}
}
func TestSynergyCashoutConservesBalanceAndDoesNotRecurse(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-A01")
	c.Skill = 10002
	c.RootID = 1
	c.Blow = combat.Blow{Origin: domain.DamageNormalOrigin}
	c.Def = domain.SkillDef{ID: 10002}
	d := domain.StatusDef{ID: 1900012345, Level: 1, SourceSkillID: 10002, IntervalSec: 1, DurationSec: 6, SynergyDOTVersion: 1, HPChanges: []domain.ResourceChange{{Value: -100}}}
	m.Status.Apply(d, s.tick, p.ID)
	before := m.HP
	synergyCashout(c)
	synergyCashout(c)
	remaining := int64(0)
	m.Status.Each(func(a domain.Active) { remaining = synergyFrozenBalance(a, s.tick, m.MaxHP) })
	if remaining != 390 || before-m.HP != 210 {
		t.Fatalf("balance remaining%d dealt%d", remaining, before-m.HP)
	}
	c.RootID++
	c.IsBasic = true
	synergyCashout(c)
	if before-m.HP != 210 {
		t.Fatal("basic cashed out")
	}
}
func TestSynergyOverflowAndQualifyingBoundary(t *testing.T) {
	if synergyScale64(math.MaxInt64, 60000, 10000) != math.MaxInt64 {
		t.Fatal("positive overflow")
	}
	if synergyScale64(math.MinInt64, 60000, 10000) != math.MinInt64 {
		t.Fatal("negative overflow")
	}
	d := domain.StatusDef{SourceSkillID: 10002, IntervalSec: 1, DurationSec: 6, HPChanges: []domain.ResourceChange{{Value: -5, Mode: domain.ModePercent}}}
	if synergyQualifyingDOT(d) {
		t.Fatal("percent HP drain upgraded")
	}
	d.HPChanges[0].Mode = domain.ModeAbsolute
	d.SourceSkillID = 0
	if synergyQualifyingDOT(d) {
		t.Fatal("unattributed hazard upgraded")
	}
}
func TestSynergySelectiveDefenseSharedFinalBudget(t *testing.T) {
	b := domain.CombatDamageBuckets{DamageReductionPct: 60, SelectiveReductionBP: 7000}
	if n := b.IncomingDamage(1000); n != 250 {
		t.Fatalf("combined cap=%d", n)
	}
	b.DamageReductionPct = 0
	if n := b.IncomingDamage(1000); n != 300 {
		t.Fatalf("selective pool=%d", n)
	}
	s, p, m, c := universalTestContext(t, "TY-D05")
	c.Blow.Area = true
	if n := universalSynergyIncoming(c, 1000); n != 450 {
		t.Fatalf("pve reduction%d", n)
	}
	_ = s
	_ = p
	_ = m
	_ = event.DamageNormal
}

func TestSynergyHealerDOTAndVisualContract(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A03")
	c.Skill = 10319
	synergyProfessionDOT(c)
	count := 0
	c.Target.Status.Each(func(a domain.Active) {
		count++
		if a.Def.Name != "灼光" || a.Def.Icon <= 0 || a.Def.SynergyDOTPhysical {
			t.Fatal("healer presentation/type")
		}
		if got := classSetPoisonRemaining(a, s.tick, c.Target.MaxHP); got != 420 {
			t.Fatalf("healer total %d", got)
		}
		fx, name, _ := s.statusVisual(a.Def)
		if fx != "光系聚气" || name != "灼光" {
			t.Fatalf("visual %s/%s", fx, name)
		}
	})
	if count != 1 {
		t.Fatalf("healer instances%d", count)
	}
}
func TestSynergyPVPQuarterAndDefenseCoefficient(t *testing.T) {
	s, p, _, c := universalTestContext(t, "TY-D05")
	join(t, s, 2, 102, "pvp", 130, 100)
	q := s.players[2]
	q.Stats = domain.Stats{}
	q.HP = 10000
	q.MaxHP = 10000
	d := domain.StatusDef{ID: 1900012345, IntervalSec: 1, DurationSec: 6, SynergyDOTVersion: 1, SynergyDOTNew: true, SynergyDOTPhysical: true, HPChanges: []domain.ResourceChange{{Value: -1000}}}
	before := q.HP
	s.tickStatus(q, domain.Active{Source: p.ID, Def: d})
	if before-q.HP != 250 {
		t.Fatalf("pvp tick%d", before-q.HP)
	}
	c.Target = q
	c.Blow.Area = true
	if n := universalSynergyIncoming(c, 1000); n != 670 {
		t.Fatalf("pvp reduction%d", n)
	}
}
func TestSynergyRecastKeepsSpreadDeadline(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A10")
	d := domain.StatusDef{ID: 1900012345, IntervalSec: 1, DurationSec: 6, SourceSkillID: 10002, HPChanges: []domain.ResourceChange{{Value: -100}}, SynergySpreadNext: 1000}
	m.Status.Apply(d, s.tick, p.ID)
	d.SynergySpreadNext = 0
	m.Status.Apply(d, s.tick+1, p.ID)
	m.Status.Each(func(a domain.Active) {
		if a.Def.SynergySpreadNext != 1000 {
			t.Fatal("recast bypassed spread ICD")
		}
	})
}
func TestSynergySkillProcBudgetPerCastAndICD(t *testing.T) {
	_, _, m, c := universalTestContext(t, "TY-A37")
	c.Skill = 10002
	c.Blow.Origin = domain.DamageNormalOrigin
	successes := 0
	for i := uint64(1); i <= 100; i++ {
		c.RootID = i
		c.NowMS = int64(i) * 5000
		before := m.HP
		synergySkillProc(c)
		after := m.HP
		for j := 0; j < 10; j++ {
			synergySkillProc(c)
		}
		if m.HP != after {
			t.Fatal("multi-hit or group rerolled same cast")
		}
		if after < before {
			successes++
		}
	}
	if successes == 0 || successes == 100 {
		t.Fatalf("proc probability successes%d", successes)
	}
}

func TestSynergyPVPSnapshotSurvivesDisconnectedSource(t *testing.T) {
	s, p, _, _ := universalTestContext(t, "TY-D05")
	join(t, s, 2, 102, "pvp-offline", 130, 100)
	q := s.players[2]
	q.Stats = domain.Stats{}
	q.HP = 10000
	q.MaxHP = 10000
	source := p.ID
	delete(s.entities, p.ID)
	d := domain.StatusDef{ID: 1900012345, IntervalSec: 1, DurationSec: 6, SynergyDOTVersion: 1, SynergyDOTNew: true, SynergyDOTPVP: true, SynergyDOTPhysical: true, HPChanges: []domain.ResourceChange{{Value: -1000}}}
	before := q.HP
	s.tickStatus(q, domain.Active{Source: source, Def: d})
	if before-q.HP != 250 {
		t.Fatalf("offline pvp loss%d", before-q.HP)
	}
}

// Consume the actual domain scheduler, including exact-total/fractional budgets.
func synergyScheduledTickForTest(s *Scene, e *entity.Entity, id domain.StatusID) {
	var next domain.Tick
	e.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == id {
			next = a.NextTickAt
		}
	})
	s.tick = next
	for _, a := range e.Status.DueTicks(s.tick) {
		if a.Def.ID == id {
			s.tickStatus(e, a)
		}
	}
}

func TestSynergyGroundGrowthPoolDurationAndOverlap(t *testing.T) {
	s, p, m, c := universalTestContext(t, "ZS-A14")
	warriorEquip(t, s, p, "TY-A18", domain.SlotRing)
	warriorEquip(t, s, p, "TY-A22", domain.SlotGlove)
	warriorEquip(t, s, p, "TY-A37", domain.SlotHead)
	warriorEquip(t, s, p, "TY-U02", domain.SlotNeck)
	p.MP = 0
	c.Skill = 10006
	c.RootID = 1
	c.Center = p.Pos
	c.Def = domain.SkillDef{ID: 10006, HurtType: domain.HurtPhysical, TargetEnemy: true, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Radius: 100, Center: domain.SkillAreaCenterCaster}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	warriorQuakeZone(c)
	c.RootID = 2
	warriorQuakeZone(c)
	st := c.State(0, "quake-1")
	if st.ExpiresMS != 6000 || st.GroundDOT.LegendaryModifiers.DamageTickBP != 20000 {
		t.Fatal("field not frozen through generic pool", st)
	}
	before := m.HP
	for i := int64(1); i <= 6; i++ {
		s.tick = domain.Tick(i * 1000 / domain.TickMS)
		c.NowMS = i * 1000
		warriorQuakeTick(c)
	}
	if before-m.HP != 1200 || p.MP != 0 {
		t.Fatal("overlap/proc budget", before-m.HP, p.MP)
	}
	n := 0
	m.Status.EachEffective(func(domain.Active) { n++ })
	if n != 0 {
		t.Fatal("ground also attached a target DOT")
	}
	for _, o := range s.outbox {
		if e, ok := o.ev.(event.DamageDealt); ok && (e.Origin != domain.DamageProcOrigin || e.ProcDepth != 1) {
			t.Fatal("ground periodic provenance", e)
		}
	}
	s.tick = domain.Tick(7000 / domain.TickMS)
	c.NowMS = 7000
	hp := m.HP
	warriorQuakeTick(c)
	if m.HP != hp {
		t.Fatal("pulse after deadline")
	}
}
func TestSynergyGroundLeavingAreaStopsPulse(t *testing.T) {
	s, p, m, c := universalTestContext(t, "ZS-A14")
	warriorEquip(t, s, p, "TY-A18", domain.SlotRing)
	c.Skill = 10006
	c.RootID = 1
	c.Center = p.Pos
	c.Def = domain.SkillDef{ID: 10006, HurtType: domain.HurtPhysical, Area: domain.SkillAreaTargeting{Radius: 100}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	warriorQuakeZone(c)
	s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 1000, Y: 1000})
	s.tick = domain.Tick(1000 / domain.TickMS)
	c.NowMS = 1000
	hp := m.HP
	warriorQuakeTick(c)
	if m.HP != hp {
		t.Fatal("ground followed departed target")
	}
}

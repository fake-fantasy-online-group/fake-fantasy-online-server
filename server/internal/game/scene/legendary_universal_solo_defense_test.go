package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/rand"
	"testing"
)

// The native, proc and child rows are damage-consumer unit boundaries. They do
// not assert currently unavailable player-versus-player gameplay.
func TestLegendaryUniversalSoloIncoming(t *testing.T) {
	for _, id := range []string{"TY-D05", "TY-D06", "TY-D07", "TY-D08", "TY-D10", "TY-D11", "TY-D17", "TY-D19"} {
		for _, kind := range []string{"physical", "magic", "area", "basic", "critical", "proc", "child", "dot", "reflection", "self"} {
			t.Run(id+"/"+kind, func(t *testing.T) {
				s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{})
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
				p.MP = 1000
				if id == "TY-D17" {
					p.Status.Apply(domain.StatusDef{ID: 1034, SourceSkillID: 10326, DurationSec: 10, BlockMove: true, Type: domain.StatusBad, TypeKnown: true}, s.tick, m.ID)
				}
				b := combat.Blow{SkillID: 10002, GuaranteedHit: true, SkillPct: 100}
				switch kind {
				case "magic":
					b.Magic = true
				case "area":
					b.Area = true
				case "basic":
					b.SkillID = 0
				case "critical":
					b.GuaranteedCrit = true
				case "proc":
					b.Origin = domain.DamageProcOrigin
					b.ProcDepth = 1
				case "child":
					b.Origin = domain.DamageProcOrigin
					b.ProcDepth = 1
					b.LegendaryChild = true
				}
				want := int32(1000)
				if kind == "critical" {
					want = 2000
				}
				direct := kind != "dot" && kind != "reflection" && kind != "self"
				switch id {
				case "TY-D05":
					if kind == "area" {
						want = 750
					}
				case "TY-D06":
					if kind == "basic" {
						want = 750
					}
				case "TY-D07":
					if direct && kind != "magic" {
						want = want * 80 / 100
					}
				case "TY-D08":
					if kind == "magic" {
						want = 800
					}
				case "TY-D10":
					if kind == "critical" {
						want = 1400
					}
				case "TY-D17":
					if direct {
						want = want * 75 / 100
					}
				case "TY-D19":
					if kind == "dot" {
						want = 750
					}
				}
				spent := int32(0)
				if id == "TY-D11" && kind != "self" {
					spent = want * 30 / 100
					want -= spent
				}
				before := p.HP
				s.outbox = nil
				switch kind {
				case "dot":
					s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: domain.StatusPoison, SourceSkillID: 10214, Type: domain.StatusBad, TypeKnown: true, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
				case "reflection":
					s.emitReflectedDamage(m, p, 1000, false)
				case "self":
					p.Stats.MinAtk = 1000
					p.Stats.MaxAtk = 1000
					s.resolveStrikeStatuses(p, p, &combat.HitTracker{}, b)
				default:
					s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
				}
				if before-p.HP != want || 1000-p.MP != spent {
					t.Fatalf("actual HP/MP %d/%d want%d/%d", before-p.HP, 1000-p.MP, want, spent)
				}
			})
		}
	}
}

func TestLegendaryUniversalSoloManaOrder(t *testing.T) {
	for _, kind := range []string{"direct", "dot", "reflection"} {
		for _, mp := range []int32{0, 20, 1000} {
			t.Run(fmt.Sprintf("TY-D11/%s/mp=%d", kind, mp), func(t *testing.T) {
				s, p, m := universalSoloFixture(t, "TY-D11", domain.Warrior, domain.Learned{})
				p.Stats = domain.Stats{}
				p.MP = mp
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				p.Status.Apply(domain.StatusDef{ID: 10320, SourceSkillID: 10320, DurationSec: 10, DamageShield: 100, PhysicalDamageReductionPct: 20}, s.tick, p.ID)
				s.refreshStats(p)
				p.Stats = domain.Stats{}
				p.HP = p.MaxHP
				p.MP = mp
				hp := p.HP
				s.outbox = nil
				switch kind {
				case "dot":
					s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: domain.StatusPoison, Type: domain.StatusBad, TypeKnown: true, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
				case "reflection":
					s.emitReflectedDamage(m, p, 1000, false)
				default:
					s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true})
				}
				// Preserve the native untyped periodic adapter, then mana and shield once.
				reduced := int32(800)
				spent := min(mp, reduced*30/100)
				want := reduced - spent - 100
				if hp-p.HP != want || mp-p.MP != spent || universalShieldValue(p) != 0 {
					t.Fatalf("order hp%d mp%d shield%d want%d/%d/0", hp-p.HP, mp-p.MP, universalShieldValue(p), want, spent)
				}
				if len(s.attacks) != 0 || len(s.casts) != 0 {
					t.Fatal("absorption generated action loop")
				}
			})
		}
	}
}

func TestLegendaryUniversalSoloStatusLifetime(t *testing.T) {
	for _, tc := range []struct {
		id       string
		status   domain.StatusID
		duration int
		receive  bool
	}{
		{"TY-A13", 1034, 14000, false}, {"TY-A22", domain.StatusPoison, 15000, false}, {"TY-A22", 1036, 15000, false},
		{"TY-D09", 1034, 7000, true}, {"TY-D18", 10320, 14000, false}, {"TY-D20", domain.StatusPoison, 6000, true}, {"TY-D20", 1036, 6000, true}, {"TY-U11", 10321, 15000, false},
	} {
		t.Run(fmt.Sprintf("%s/status=%d", tc.id, tc.status), func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{})
			dst, src := m, p
			d := domain.StatusDef{ID: tc.status, SourceSkillID: 10002, DurationSec: 10, Type: domain.StatusBad, TypeKnown: true, ChanceBP: 10000, ChanceExplicit: true}
			if tc.status == 1034 {
				d.BlockMove = true
			}
			if tc.status == domain.StatusPoison || tc.status == 1036 {
				d.IntervalSec = 1
				d.HPChanges = []domain.ResourceChange{{Value: -10}}
			}

			if tc.status == 10320 {
				d.DamageShield = 100
				d.Type = domain.StatusGood
				dst = p
			}
			if tc.status == 10321 {
				d.Type = domain.StatusGood
				d.PhysicalDamageReductionPct = 10
				dst = p
			}
			if tc.receive {
				dst, src = p, m
			}
			if !s.applyStatusDef(dst, d, src.ID) {
				t.Fatal("status rejected")
			}
			a := universalSoloActive(t, dst, tc.status)
			if a.ExpireAt-s.tick != domain.Ticks(tc.duration) || a.Source != src.ID {
				t.Fatalf("committed lifetime/source %+v", a)
			}
			if tc.status == 1034 && canMove(dst) {
				t.Fatal("control did not block movement")
			}
			if tc.status == domain.StatusPoison || tc.status == 1036 {
				before := dst.HP
				s.tickStatus(dst, a)
				if before-dst.HP != 10 || a.Def.IntervalSec != 1 {
					t.Fatal("duration power altered tick damage/interval")
				}
			}

			s.tick = a.ExpireAt - 1
			s.stepStatus()
			if !dst.Status.Has(tc.status) {
				t.Fatal("expired early")
			}
			s.tick++
			s.stepStatus()
			if dst.Status.Has(tc.status) {
				t.Fatal("did not expire")
			}
			if tc.status == 1034 && !canMove(dst) {
				t.Fatal("control persisted after expiration")
			}
		})
	}
	for _, id := range []string{"TY-A13", "TY-D09"} {
		t.Run(id+"/immunity_and_noncontrol", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{})
			dst, src := m, p
			if id == "TY-D09" {
				dst, src = p, m
			}
			dst.Status.Apply(domain.StatusDef{ID: 10114, DurationSec: 10, BlockHarmful: true}, s.tick, dst.ID)
			d := domain.StatusDef{ID: 1034, SourceSkillID: 10326, DurationSec: 10, BlockMove: true, Type: domain.StatusBad, TypeKnown: true}
			if s.applyStatusDef(dst, d, src.ID) || dst.Status.Has(1034) {
				t.Fatal("control bypassed immunity")
			}
			dst.Status.Remove(10114)
			d.ID = 190981
			d.BlockMove = false
			if !s.applyStatusDef(dst, d, src.ID) {
				t.Fatal("ordinary debuff missing")
			}
			if universalSoloActive(t, dst, d.ID).ExpireAt-s.tick != domain.Ticks(10000) {
				t.Fatal("noncontrol lifetime changed")
			}
		})
	}
	t.Run("TY-U11/exclusions", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U11", domain.Healer, domain.Learned{})
		for _, d := range []domain.StatusDef{{ID: 10320, SourceSkillID: 10320, DurationSec: 10, DamageShield: 100}, {ID: 10303, SourceSkillID: 10303, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: 10}}}} {
			if !s.applyStatusDef(p, d, p.ID) {
				t.Fatal("native status missing")
			}
			a := universalSoloActive(t, p, d.ID)
			start := s.tick
			if a.ExpireAt-start != domain.Ticks(10000) {
				t.Fatal("excluded status duration changed", a.ExpireAt-start)
			}
			s.tick = start + domain.Ticks(10000) - 1
			s.stepStatus()
			if !p.Status.Has(d.ID) {
				t.Fatal("excluded status expired early")
			}
			s.tick = start + domain.Ticks(10000)
			s.stepStatus()
			if p.Status.Has(d.ID) {
				t.Fatal("shield/hot extended by beneficial duration")
			}
		}
	})
}

func TestLegendaryUniversalSoloPeriodic(t *testing.T) {
	for _, tc := range []struct {
		id          string
		status      domain.StatusID
		delta, want int32
		receive     bool
	}{
		{"TY-A18", domain.StatusPoison, -100, -200, false}, {"TY-A27", 1036, -100, -200, false},
		{"TY-A28", domain.StatusPoison, -100, -200, false}, {"TY-A29", 1036, -100, -200, false},
		{"TY-U05", 10303, 100, 160, false}, {"TY-D12", 10303, 100, 140, true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{})
			src, dst := p, m
			p.Stats.CritRate = 10000
			p.Stats.MCritRate = 10000
			if tc.delta > 0 {
				dst = p
			}
			if tc.receive {
				src = m
			}
			d := domain.StatusDef{ID: tc.status, SourceSkillID: 10303, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: tc.delta}}, TypeKnown: true, Type: domain.StatusGood}
			if tc.delta < 0 {
				d.Type = domain.StatusBad
			}
			if !s.applyStatusDef(dst, d, src.ID) {
				t.Fatal("periodic producer rejected")
			}
			a := universalSoloActive(t, dst, tc.status)
			if a.ExpireAt-s.tick != domain.Ticks(10000) || a.NextTickAt-s.tick != domain.Ticks(1000) {
				t.Fatal("amplification altered duration/interval")
			}
			p.Stats.CritRate = 0
			p.Stats.MCritRate = 0
			dst.MaxHP = 10000
			dst.HP = 5000
			for i := 0; i < 3; i++ {
				before := dst.HP
				s.outbox = nil
				s.tickStatus(dst, a)
				if dst.HP-before != tc.want {
					t.Fatalf("committed tick %d want%d", dst.HP-before, tc.want)
				}
				if tc.delta < 0 {
					n := 0
					for _, o := range s.outbox {
						if e, ok := o.ev.(event.DamageDealt); ok {
							n++
							if e.Origin != domain.DamageDOTOrigin {
								t.Fatal("periodic generated attack")
							}
						}
					}
					if n != 1 {
						t.Fatal("duplicate periodic damage", n)
					}
				}
			}
			n := 0
			dst.Status.EachEffective(func(x domain.Active) {
				if x.Def.ID == tc.status {
					n++
				}
			})
			if n != 1 || len(s.casts) != 0 || len(s.attacks) != 0 {
				t.Fatal("extra state or attack")
			}
			// Power-produced periodic payloads must not be amplified recursively.
			d.ID = 190982
			d.LegendaryPowerID = tc.id
			d.HPChanges = []domain.ResourceChange{{Value: tc.delta}}
			dst.HP = 5000
			s.tickStatus(dst, domain.Active{Def: d, Source: src.ID})
			if dst.HP-5000 != tc.delta {
				t.Fatal("recursive periodic amplification")
			}
		})
	}
}

func TestLegendaryUniversalSoloShield(t *testing.T) {
	for _, id := range []string{"TY-D13", "TY-D16"} {
		for _, self := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/self=%v", id, self), func(t *testing.T) {
				s, p, m := universalSoloFixture(t, id, domain.Healer, domain.Learned{})
				src, dst := p, p
				if id == "TY-D13" && !self {
					src = m
				}
				if id == "TY-D16" && !self {
					dst = m
				}
				d := domain.StatusDef{ID: 10320, SourceSkillID: 10320, DurationSec: 10, DamageShield: 100, TypeKnown: true, Type: domain.StatusGood}
				if !s.applyStatusDef(dst, d, src.ID) {
					t.Fatal("shield missing")
				}
				want := int32(100)
				if id == "TY-D16" {
					want = 160
				} else if !self {
					want = 140
				}
				if universalShieldValue(dst) != want {
					t.Fatal("initial shield", universalShieldValue(dst), want)
				}
				if !s.applyStatusDef(dst, d, src.ID) || universalShieldValue(dst) != want {
					t.Fatal("native shield overwrite became additive")
				}
				a := universalSoloActive(t, dst, d.ID)
				if a.Source != src.ID || a.ExpireAt-s.tick != domain.Ticks(10000) {
					t.Fatal("shield source/lifetime changed")
				}
				count := 0
				dst.Status.EachEffective(func(x domain.Active) {
					if x.Def.ID == d.ID {
						count++
					}
				})
				if count != 1 {
					t.Fatal("duplicate shield")
				}
				dst.HP = dst.MaxHP
				hp := dst.HP
				attacker := m
				if dst == m {
					attacker = p
				}
				attacker.Stats.MinAtk = 200
				attacker.Stats.MaxAtk = 200
				s.resolveStrikeStatuses(attacker, dst, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true})
				if hp-dst.HP != 200-want || universalShieldValue(dst) != 0 {
					t.Fatal("shield not consumed once", dst.HP)
				}
			})
		}
	}
}

func TestLegendaryUniversalSoloStatusAccuracy(t *testing.T) {
	for _, tc := range []struct {
		name         string
		base, resist int32
		seed         int64
		want         bool
	}{{"bonus_lands", 5000, 0, 1, true}, {"base_still_matters", 1000, 0, 1, false}, {"resistance_still_matters", 5000, 100, 1, false}, {"zero_base", 0, 0, 1, false}} {
		t.Run("TY-A15/"+tc.name, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-A15", domain.Warrior, domain.Learned{})
			s.rng = rand.New(rand.NewSource(tc.seed))
			m.Stats.StatusResist = tc.resist
			d := domain.StatusDef{ID: 1034, SourceSkillID: 10326, DurationSec: 10, BlockMove: true, TypeKnown: true, Type: domain.StatusBad, ChanceExplicit: true, ChanceBP: tc.base}
			got := s.applyStatusDef(m, d, p.ID)
			if got != tc.want || m.Status.Has(d.ID) != tc.want {
				t.Fatalf("contest result%v want%v", got, tc.want)
			}
		})
	}
}

func TestLegendaryUniversalSoloPeriodicProvenance(t *testing.T) {
	for _, tc := range []struct {
		id                 string
		powerPayload, crit bool
		hp, mp             int32
	}{{"TY-D11", true, false, 700, 300}, {"TY-D19", true, false, 750, 0}, {"TY-D10", false, true, 1400, 0}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{})
			p.Stats = domain.Stats{}
			p.MP = 1000
			d := domain.StatusDef{ID: domain.StatusPoison, SourceSkillID: 10214, Type: domain.StatusBad, TypeKnown: true, HPChanges: []domain.ResourceChange{{Value: -1000}}}
			if tc.powerPayload {
				d.LegendaryPowerID = "source-periodic-payload"
			}
			if tc.crit {
				d.LegendaryModifiers.PeriodicCanCrit = true
				d.LegendaryModifiers.PeriodicCritChanceBP = 10000
			}
			before := p.HP
			s.tickStatus(p, domain.Active{Source: m.ID, Def: d})
			if before-p.HP != tc.hp || 1000-p.MP != tc.mp {
				t.Fatalf("periodic source classification HP/MP%d/%d want%d/%d", before-p.HP, 1000-p.MP, tc.hp, tc.mp)
			}
		})
	}
}

package scene

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Solo means exactly one equipped legendary, not one ID mentioned in a test.
// Native learned skills/statuses and ordinary equipment are the carrier. These
// tests assert committed HP/MP, targets, status lifetime, or admission results;
// prepared modifier fields are never the sole evidence.
func universalSoloFixture(t *testing.T, id string, race domain.Race, learned domain.Learned) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s, p, m := nativeRepairFixture(t, race, learned, id)
	got := domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)
	if len(got) != 1 || got[0].Power.ID != id {
		t.Fatalf("not solo %s: %+v", id, got)
	}
	s.rng = rand.New(rand.NewSource(1))
	return s, p, m
}
func universalSoloAttack(t *testing.T, s *Scene, p, m *entity.Entity, b combat.Blow) {
	t.Helper()
	s.onAttack(Attack{ID: p.ID, Target: m.ID, Blow: b})
	if len(s.attacks) != 1 {
		t.Fatalf("basic rejected: %+v", s.outbox)
	}
	s.stepCombatAt(s.now().Add(time.Second), false)
	if len(s.attacks) != 0 {
		t.Fatal("basic produced extra attacks")
	}
}
func universalSoloActive(t *testing.T, e *entity.Entity, id domain.StatusID) domain.Active {
	t.Helper()
	var out domain.Active
	e.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == id {
			out = a
		}
	})
	if out.Def.ID != id {
		t.Fatalf("status %d not committed", id)
	}
	return out
}
func universalSoloDamage(t *testing.T, s *Scene, target domain.EntityID, skill domain.SkillID, want int32, count int) {
	t.Helper()
	got, n := assassinRegressionDamage(s, target, int32(skill))
	if got != want || n != count {
		t.Fatalf("damage %d/%d want %d/%d", got, n, want, count)
	}
}

func TestLegendaryUniversalSoloGeometry(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		x, y  float64
		want  int32
	}{
		{"TY-A02", 10005, 300, 100, 95}, {"TY-A03", 10006, 300, 100, 130},
		{"TY-A04", 10022, 300, 100, 155}, {"TY-A06", 10005, 200, 135, 95},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{tc.skill: 1})
			added := putMonster(s, 901, 10000, domain.Stats{}, tc.x, tc.y)
			var bx, by float64
			switch tc.id {
			case "TY-A02":
				bx, by = 341, 100
			case "TY-A03":
				bx, by = 326, 100
			case "TY-A04":
				bx, by = 311, 100
			case "TY-A06":
				bx, by = 200, 141
			}
			outside := putMonster(s, 902, 10000, domain.Stats{}, bx, by)
			cx, cy := float64(200), float64(151)
			if tc.id == "TY-A06" {
				cx, cy = 251, 100
			}
			if tc.id == "TY-A03" {
				cx, cy = 150, 200
			} // outside unchanged 120-degree cone
			cross := putMonster(s, 903, 10000, domain.Stats{}, cx, cy)
			before := p.MP
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: m.ID})
			universalSoloDamage(t, s, m.ID, tc.skill, tc.want, 1)
			universalSoloDamage(t, s, added.ID, tc.skill, tc.want, 1)
			if outside.HP != 10000 || before-p.MP != d.MPCost {
				t.Fatal("out of geometry target or duplicate cost", outside.HP, before-p.MP)
			}
			if (tc.id == "TY-A02" || tc.id == "TY-A03" || tc.id == "TY-A06") && cross.HP != 10000 {
				t.Fatal("unchanged dimension widened")
			}
		})
	}
	t.Run("TY-A04/cone_angle", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A04", domain.Warrior, domain.Learned{10006: 1})
		side := putMonster(s, 901, 10000, domain.Stats{}, 100+100*math.Cos(75*math.Pi/180), 100+100*math.Sin(75*math.Pi/180))
		far := putMonster(s, 902, 10000, domain.Stats{}, 100+100*math.Cos(85*math.Pi/180), 100+100*math.Sin(85*math.Pi/180))
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10006, Target: m.ID})
		universalSoloDamage(t, s, side.ID, 10006, 130, 1)
		if far.HP != 10000 {
			t.Fatal("cone exceeded 168 degrees")
		}
	})
	t.Run("TY-A05", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A05", domain.Warrior, domain.Learned{10009: 1})
		sides := []*entity.Entity{}
		for i := 0; i < 6; i++ {
			sides = append(sides, putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i*15), 100))
		}
		out := putMonster(s, 907, 10000, domain.Stats{}, 251, 100)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10009, Target: m.ID})
		if m.HP == m.MaxHP || !m.Status.Has(10009) {
			t.Fatal("primary native damage/debuff lost")
		}
		dmg := m.MaxHP - m.HP
		for i, e := range sides {
			want := true
			if (e.HP < 10000) != want {
				t.Fatalf("nearest cap side%d hp%d", i, e.HP)
			}
			if want && (10000-e.HP != dmg || !e.Status.Has(10009)) {
				t.Fatal("native secondary damage/status changed")
			}
		}
		if out.HP != 10000 {
			t.Fatal("radius escaped")
		}
	})
	for _, tc := range []struct {
		id         string
		main, side int32
	}{{"TY-A34", 279, 155}, {"TY-A36", 155, 217}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{10022: 1})
			side := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10022, Target: m.ID})
			universalSoloDamage(t, s, m.ID, 10022, tc.main, 1)
			universalSoloDamage(t, s, side.ID, 10022, tc.side, 1)
		})
	}
}

func TestLegendaryUniversalSoloBasic(t *testing.T) {
	t.Run("TY-A01", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A01", domain.Assassin, domain.Learned{})
		p.Look.AtkDist = 100
		p.HP = 1000
		p.MP = 0
		p.Status.Apply(domain.StatusDef{ID: 1018, SourceSkillID: 10211, DurationSec: 120, PoisonOnHitChance: 100, PoisonOnHitLevel: 21, SourceEffectHitKnown: true, LifeStealPct: 10, LifeStealChance: 100, ManaStealPct: 10, ManaStealChance: 100}, s.tick, p.ID)
		sides := []*entity.Entity{}
		for i := 0; i < 6; i++ {
			sides = append(sides, putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i*8), 100))
		}
		behind := putMonster(s, 907, 10000, domain.Stats{}, 80, 100)
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
		universalSoloDamage(t, s, m.ID, 0, 100, 1)
		for i, e := range sides {
			want := int32(70)
			if 10000-e.HP != want {
				t.Fatalf("cap/nearest side%d damage%d", i, 10000-e.HP)
			}
			if !e.Status.Has(domain.StatusPoison) {
				t.Fatal("native poison missing")
			}
		}
		if p.HP != 1052 || p.MP != 52 || behind.HP != 10000 || !m.Status.Has(domain.StatusPoison) {
			t.Fatalf("fan leech/geometry hp%d mp%d behind%d", p.HP, p.MP, behind.HP)
		}
	})
	for _, id := range []string{"TY-A09", "TY-A37", "TY-A39"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{})
			p.Look.AtkDist = 100
			want := int32(175)
			b := combat.Blow{GuaranteedHit: true}
			if id == "TY-A09" {
				p.Stats.Hit = 0
				m.Stats.Def = 100000
				p.Stats.CritRate = 10000
				b.GuaranteedHit = false
				want = 200
			}
			if id == "TY-A39" {
				st := p.Player.Worn.At(domain.SlotWeapon)
				it := s.items[st.Item]
				it.Equip.Bonus.Def = 100
				s.items[st.Item] = it
				want = 120
			}
			universalSoloAttack(t, s, p, m, b)
			universalSoloDamage(t, s, m.ID, 0, want, 1)
		})
	}
	t.Run("TY-A35", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A35", domain.Warrior, domain.Learned{})
		st := p.Player.Worn.At(domain.SlotWeapon)
		it := s.items[st.Item]
		it.Equip.Appearance = domain.EquipAppearance{Part: domain.AppearanceWeaponR, Known: true, AttackKnown: true, AtkDist: 100}
		s.items[st.Item] = it
		s.refreshAppearance(p)
		s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 240, Y: 100})
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
		universalSoloDamage(t, s, m.ID, 0, 100, 1)
		beforeReady := p.CapturePlayerRuntime(s.tick).AttackReadyIn
		assertLook := func(want uint16) {
			t.Helper()
			n := 0
			for _, o := range s.outbox {
				if e, ok := o.ev.(event.AppearanceChanged); ok && e.Who == p.ID {
					n++
					if e.Appearance.AtkDist != want {
						t.Fatalf("published distance%d want%d", e.Appearance.AtkDist, want)
					}
				}
			}
			if n != 1 {
				t.Fatal("appearance output count", n)
			}
		}
		s.outbox = nil
		s.refreshAppearance(p)
		assertLook(150)
		st.Legendary = domain.LegendaryInstance{}
		p.Player.Worn.Set(domain.SlotWeapon, st)
		s.outbox = nil
		s.refreshAppearance(p)
		assertLook(100)
		if p.CapturePlayerRuntime(s.tick).AttackReadyIn != beforeReady || p.Player.Char.Appear.AtkDist != 100 || p.UsesRangedWeapon() {
			t.Fatal("reach altered native identity/cadence")
		}

	})
	t.Run("TY-U02", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			mp, want int32
			miss     bool
		}{{"success", 0, 10, false}, {"cap", 9995, 10000, false}, {"miss", 0, 0, true}} {
			t.Run(tc.name, func(t *testing.T) {
				s, p, m := universalSoloFixture(t, "TY-U02", domain.Warrior, domain.Learned{})
				p.Look.AtkDist = 100
				p.MP = tc.mp
				b := combat.Blow{GuaranteedHit: true}
				if tc.miss {
					p.Stats.Hit = 0
					m.Stats.Def = 100000
					b.GuaranteedHit = false
				}
				universalSoloAttack(t, s, p, m, b)
				if p.MP != tc.want {
					t.Fatalf("mp%d want%d", p.MP, tc.want)
				}
			})
		}
	})
	t.Run("TY-A10", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A10", domain.Warrior, domain.Learned{})
		s.refreshStats(p)
		p.Look.AtkDist = 100
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
		ready := p.CapturePlayerRuntime(s.tick).AttackReadyIn
		if ready <= 0 {
			t.Fatal("cadence missing")
		}
		s.tick += ready - 1
		s.onAttack(Attack{ID: p.ID, Target: m.ID})
		if len(s.attacks) != 0 {
			t.Fatal("attacked before shortened cooldown")
		}
		s.tick++
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
		base := p.Stats
		p.Player.Worn = domain.NewEquipSet()
		s.refreshStats(p)
		if base.AttackIntervalTicks() >= p.Stats.AttackIntervalTicks() {
			t.Fatal("effective attack interval not shortened")
		}
	})
}

func TestLegendaryUniversalSoloActiveDamage(t *testing.T) {
	for _, tc := range []struct {
		id    string
		race  domain.Race
		skill domain.SkillID
		want  int32
	}{
		{"TY-A08", domain.Warrior, 10002, 208}, {"TY-A25", domain.Warrior, 10002, 195}, {"TY-A26", domain.Warlock, 10409, 150},
		{"TY-A38", domain.Warrior, 10002, 530}, {"TY-A40", domain.Warrior, 10002, 182},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, tc.race, domain.Learned{tc.skill: 1})
			if tc.id == "TY-A25" || tc.id == "TY-A26" {
				st := p.Player.Worn.At(domain.SlotWeapon)
				it := s.items[st.Item]
				it.Equip.Bonus = domain.Stats{MaxAtk: 100, MinAtk: 100, MAtk: 100}
				s.items[st.Item] = it
			}
			if tc.id == "TY-A40" {
				p.Status.Apply(domain.StatusDef{ID: 10320, SourceSkillID: 10320, DurationSec: 60, DamageShield: 100}, s.tick, p.ID)
			}
			before := p.MP
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: m.ID})
			universalSoloDamage(t, s, m.ID, tc.skill, tc.want, 1)
			if before-p.MP != d.MPCost {
				t.Fatal("extra mana consumption")
			}
			if tc.id == "TY-A40" && universalShieldValue(p) != 100 {
				t.Fatal("offensive bonus consumed shield")
			}
		})
	}
	for _, tc := range []struct {
		id                 string
		crit, resist, want int32
		magic              bool
	}{{"TY-A12", 0, 100, 30, false}, {"TY-A12", 0, 100, 30, true}, {"TY-A33", 10000, 0, 250, false}} {
		t.Run(fmt.Sprintf("%s/magic=%v", tc.id, tc.magic), func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{})
			p.Stats.CritRate = tc.crit
			p.Stats.MCritRate = tc.crit
			m.Stats.PhysResist = tc.resist
			m.Stats.MagicResist = tc.resist
			end := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
			ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 10002, SkillPct: 100, GuaranteedHit: true, Magic: tc.magic})
			end()
			if ev.Amount != tc.want || m.HP != m.MaxHP-tc.want || m.Stats.PhysResist != tc.resist || m.Stats.MagicResist != tc.resist {
				t.Fatalf("strike amount%d want%d", ev.Amount, tc.want)
			}
		})
	}
	t.Run("TY-A16", func(t *testing.T) {
		for _, tc := range []struct{ shield, want, left int32 }{{100, 50, 0}, {300, 0, 100}, {1, 99, 0}, {0, 100, 0}} {
			t.Run(fmt.Sprint(tc.shield), func(t *testing.T) {
				s, p, m := universalSoloFixture(t, "TY-A16", domain.Warrior, domain.Learned{})
				if tc.shield > 0 {
					m.Status.Apply(domain.StatusDef{ID: 10320, SourceSkillID: 10320, DurationSec: 60, DamageShield: tc.shield}, s.tick, m.ID)
				}
				ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 10002, SkillPct: 100, GuaranteedHit: true})
				if ev.Amount != tc.want || m.MaxHP-m.HP != tc.want || universalShieldValue(m) != tc.left {
					t.Fatalf("shield/hp %d/%d want%d/%d", universalShieldValue(m), m.MaxHP-m.HP, tc.left, tc.want)
				}
			})
		}
	})
	t.Run("TY-A19", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A19", domain.Warrior, domain.Learned{10013: 1})
		m.MP = 1000
		m.MaxMP = 1000
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10013, Target: m.ID})
		if m.MP != 892 {
			t.Fatalf("native attack drain=%d", 1000-m.MP)
		}
		universalSoloDamage(t, s, m.ID, 10013, 100, 1)
	})
}

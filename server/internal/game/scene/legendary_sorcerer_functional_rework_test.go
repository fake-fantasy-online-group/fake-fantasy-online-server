package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestSorcererFunctionalWindows(t *testing.T) {
	for _, power := range []string{"SS-U04", "SS-U07", "SS-U10"} {
		t.Run(power, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, power)
			h := legendaryHandlers[power]
			c.Skill = legendaryIceBurst
			if power == "SS-U10" {
				c.Skill = 10432
			}
			check := func() int32 {
				d := domain.SkillDef{ID: 10401, Dist: 300, TargetEnemy: true}
				if power == "SS-U04" {
					stats := domain.Stats{MoveSpeed: 100}
					h.PassiveStats(c, &stats)
					return stats.MoveSpeed
				}
				h.PrepareSkill(c, &d)
				if power == "SS-U07" {
					return d.LegendaryModifiers.ManaReductionBP
				}
				return d.Dist
			}
			base := check()
			if power == "SS-U10" {
				h.Teleported(c, domain.Pos{})
			} else {
				h.RootFinished(c)
			}
			want := int32(180)
			if power == "SS-U07" {
				want = 4500
			}
			if power == "SS-U10" {
				want = 600
			}
			if got := check(); got != want {
				t.Fatalf("window=%d want%d baseline%d", got, want, base)
			}
			delete(c.Owner.Player.Char.Skills, c.Skill)
			if check() != base {
				t.Fatal("unlearned window survived")
			}
			c.Owner.Player.Char.Skills[c.Skill] = 1
			if check() != base {
				t.Fatal("relearning resurrected window")
			}
			_ = s
		})
	}
}
func TestSorcererFunctionalEscapeActualCast(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run(map[bool]string{false: "without", true: "equipped"}[on], func(t *testing.T) {
			s, p, _ := casterIndependentFixture(t, "SS-U06", domain.Learned{10432: 1}, on)
			// Pin chance for the positive success-path test; the power itself does not
			// increase native teleport chance or grant a skill.
			d, _ := s.skills.Get(10432, 1)
			d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
			for i := range d.Effects {
				d.Effects[i].ChanceBP = 10000
			}
			d.Effect = d.Effects[0]
			s.skills[domain.SkillKey{ID: 10432, Level: 1}] = d
			s.refreshStats(p)
			p.MP = p.MaxMP
			p.Status.Apply(domain.StatusDef{ID: domain.StatusStun, Level: 1, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun, DurationSec: 10}, s.tick, 900)
			from := p.Pos
			to := from
			to.X += 40
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
			sharedTargetFinishCasts(t, s)
			if on {
				if p.Pos == from || p.Status.Has(domain.StatusStun) {
					t.Fatal("learned escape failed", p.Pos, p.Status.Has(domain.StatusStun))
				}
			} else if p.Pos != from || !p.Status.Has(domain.StatusStun) {
				t.Fatal("baseline escaped control")
			}
		})
	}
}
func TestSorcererFunctionalGroupStealFreshDedup(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U02")
	s.refreshStats(c.Owner)
	c.Owner.MP = c.Owner.MaxMP
	for i := 0; i < 12; i++ {
		m := putMonster(s, domain.EntityID(910+i), 10000, domain.Stats{}, 130+float64(i), 100)
		m.Status = domain.NewStatusSet()
		m.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, m.ID)
		m.Status.AbsorbDamage(90)
	}
	c.Target.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, c.Target.ID)
	immune := putMonster(s, 950, 10000, domain.Stats{}, 160, 100)
	immune.Status = domain.NewStatusSet()
	immune.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, Permanent: true, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, immune.ID)
	nativeRepairCast(t, s, c.Owner, 10433, c.Target.ID)
	for i := 0; i < 12; i++ {
		if s.entities[domain.EntityID(910+i)].Status.Has(1021) {
			t.Fatal("group count capped", i)
		}
	}
	count := 0
	c.Owner.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1021 {
			count++
			if a.ShieldLeft != 100 || a.ExpireAt != s.tick+domain.Ticks(20000) {
				t.Fatal("not full fresh shield", a.ShieldLeft, a.ExpireAt)
			}
		}
	})
	if count != 1 {
		t.Fatal("stacked shields", count)
	}
	if !immune.Status.Has(1021) {
		t.Fatal("permanent unstealable removed")
	}
}
func TestSorcererFunctionalOwnBurnAnyKill(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U05")
	c.Owner.MP = 0
	c.Target.Status.Apply(domain.StatusDef{ID: 1036, Level: 1, DurationSec: 10}, s.tick, c.Owner.ID)
	c.Target.HP = 0
	c.Hit.Flag = event.DamageFatal
	h := legendaryHandlers["SS-U05"]
	h.AfterHit(c)
	h.AfterHit(c)
	if c.Owner.MP != c.Owner.MaxMP/10 {
		t.Fatal("kill reward not exactly once", c.Owner.MP)
	}
}

func TestSorcererIceStrideActualTickExpiryAndUnequip(t *testing.T) {
	for _, mode := range []string{"expires", "unequipped"} {
		t.Run(mode, func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-U04", domain.Learned{10410: 1}, true)
			s.refreshStats(p)
			p.MP = p.MaxMP
			base := s.playerMoveSpeedPX(p)
			nativeRepairCast(t, s, p, 10410, m.ID)
			boosted := s.playerMoveSpeedPX(p)
			if boosted <= base {
				t.Fatal("successful native ice burst did not activate movement", base, boosted)
			}
			s.tick += domain.Ticks(7900)
			s.stepLegendaryPowers()
			if s.playerMoveSpeedPX(p) != boosted {
				t.Fatal("window expired before eight seconds")
			}
			if mode == "unequipped" {
				p.Player.Worn = &domain.EquipSet{}
				s.pruneLegendaryOwner(p)
				s.refreshStats(p)
			} else {
				// Tick must refresh the cached live stats, not merely hide a
				// status or wait until another cast causes a stat rebuild.
				s.tick += domain.Ticks(200)
			}
			s.stepLegendaryPowers()
			if got := s.playerMoveSpeedPX(p); got != base {
				t.Fatalf("movement remained boosted after %s: got%d want%d", mode, got, base)
			}
			for k, st := range s.legendaryState().states {
				if k.Owner == p.ID && k.Power == "SS-U04" && k.Phase == "ice-stride" && st.ExpiresMS != 0 {
					t.Fatal("expired/removed movement window retained", k, st)
				}
			}
			s.stepLegendaryPowers()
			if s.playerMoveSpeedPX(p) != base {
				t.Fatal("second tick resurrected movement window")
			}
		})
	}
}

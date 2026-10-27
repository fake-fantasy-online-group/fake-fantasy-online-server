package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"testing"
)

func TestAssassinFunctionalWindows(t *testing.T) {
	for _, tc := range []struct {
		id             string
		trigger, query domain.SkillID
		bp             int32
	}{{"CK-U01", 10208, 10205, 4500}, {"CK-U07", 10213, 10214, 5500}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, tc.id)
			c := assassinContext(s, p, m, tc.trigger, 1)
			d := domain.SkillDef{ID: tc.query}
			legendaryAssassinUtilityPrepare(&c, &d)
			if d.LegendaryModifiers.ManaReductionBP != 0 {
				t.Fatal("free baseline")
			}
			legendaryAssassinUtilityRelease(&c)
			d = domain.SkillDef{ID: tc.query}
			legendaryAssassinUtilityPrepare(&c, &d)
			if d.LegendaryModifiers.ManaReductionBP != tc.bp {
				t.Fatal("window", d.LegendaryModifiers)
			}
			c.NowMS += 1000
			legendaryAssassinUtilityRelease(&c)
			if c.State(0, "utility-window").ExpiresMS != c.NowMS+10000 {
				t.Fatal("stacked window")
			}
			delete(p.Player.Char.Skills, tc.trigger)
			d = domain.SkillDef{ID: tc.query}
			legendaryAssassinUtilityPrepare(&c, &d)
			if d.LegendaryModifiers.ManaReductionBP != 0 {
				t.Fatal("respec retained window")
			}
			p.Player.Char.Skills[tc.trigger] = 1
			c.NowMS += 10000
			d = domain.SkillDef{ID: tc.query}
			legendaryAssassinUtilityPrepare(&c, &d)
			if d.LegendaryModifiers.ManaReductionBP != 0 {
				t.Fatal("expired window")
			}
		})
	}
}
func TestAssassinFunctionalDefense(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-D07")
	c := assassinContext(s, p, m, 10219, 1)
	if legendaryAssassinNativeDefense(&c, 100) != 100 {
		t.Fatal("free baseline")
	}
	legendaryAssassinDefenseRelease(&c)
	if legendaryAssassinNativeDefense(&c, 100) != 65 {
		t.Fatal("released guard")
	}
	c.NowMS += 8000
	if legendaryAssassinNativeDefense(&c, 100) != 100 {
		t.Fatal("expired guard")
	}
	delete(p.Player.Char.Skills, 10219)
	legendaryAssassinDefenseRelease(&c)
	if legendaryAssassinNativeDefense(&c, 100) != 100 {
		t.Fatal("unlearned guard")
	}
}
func TestAssassinFunctionalTrapStealth(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-U08")
	c := assassinContext(s, p, m, 10213, 1)
	p.Status.Apply(domain.StatusDef{ID: domain.StatusStealth, Invisible: true, DurationSec: 10, SourceSkillID: 10208, BreakOnAttack: true}, s.tick, p.ID)
	for i := 0; i < 5; i++ {
		d := domain.SkillDef{ID: 10213}
		legendaryAssassinStealthTrapPrepare(&c, &d)
		if d.PreserveInvisibleChanceBP != 10000 {
			t.Fatal("trap preservation", i)
		}
		legendaryAssassinStealthTrapAccepted(&c)
	}
	delete(p.Player.Char.Skills, 10208)
	d := domain.SkillDef{ID: 10213}
	legendaryAssassinStealthTrapPrepare(&c, &d)
	if d.PreserveInvisibleChanceBP != 0 {
		t.Fatal("unlearned preservation")
	}
	s.prepareBasicAttackStatuses(p, &combat.Blow{})
	if entityInvisible(p) {
		t.Fatal("basic attack preserved stealth")
	}
}
func TestAssassinFunctionalExitGuard(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-D01")
	c := assassinContext(s, p, m, 10208, 1)
	s.legendaryAssassinVisibility(p, true, false)
	if legendaryAssassinNativeDefense(&c, 100) != 100 {
		t.Fatal("old stealth granted exit window")
	}
	d := domain.StatusDef{ID: domain.StatusStealth, Invisible: true, SourceSkillID: 10208}
	legendaryAssassinDefenseCommitted(&c, &d)
	s.legendaryAssassinVisibility(p, true, false)
	if legendaryAssassinNativeDefense(&c, 100) != 65 {
		t.Fatal("exit guard missing")
	}
	s.clearLegendaryOwner(p.ID)
	if legendaryAssassinNativeDefense(&c, 100) != 100 {
		t.Fatal("death/relog retained guard")
	}
}

func TestAssassinFunctionalDetectionLifecycle(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-U04")
	s.refreshStats(p)
	base := p.Stats.MoveSpeed
	c := assassinContext(s, p, m, 10228, 1)
	legendaryAssassinUtilityRelease(&c)
	if p.Stats.MoveSpeed != base*160/100 {
		t.Fatalf("window speed %d base %d", p.Stats.MoveSpeed, base)
	}
	s.tick += domain.Ticks(10000)
	s.stepLegendaryPowers()
	if p.Stats.MoveSpeed != base {
		t.Fatal("speed persisted after expiry", p.Stats.MoveSpeed, base)
	}
	c = assassinContext(s, p, m, 10228, 2)
	legendaryAssassinUtilityRelease(&c)
	delete(p.Player.Char.Skills, 10228)
	s.refreshStats(p)
	if p.Stats.MoveSpeed != base {
		t.Fatal("speed persisted after respec")
	}
	p.Player.Char.Skills[10228] = 1
	c = assassinContext(s, p, m, 10228, 3)
	legendaryAssassinUtilityRelease(&c)
	p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		st.Legendary = domain.LegendaryInstance{}
		p.Player.Worn.Set(slot, st)
	})
	s.refreshStats(p)
	if p.Stats.MoveSpeed != base {
		t.Fatal("speed persisted after unequip")
	}
}

func TestAssassinFunctionalNativeTrapRelease(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1, 10214: 1}, "CK-U07")
	baseBomb, ok := s.skills.Get(10213, 1)
	if !ok {
		t.Fatal("bomb missing")
	}
	baseMist, ok := s.skills.Get(10214, 1)
	if !ok {
		t.Fatal("mist missing")
	}
	bomb := assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
	if bomb.MPCost != baseBomb.MPCost {
		t.Fatal("first trap received retrospective discount", bomb.MPCost, baseBomb.MPCost)
	}
	mist := assassinRegressionCast(t, s, p, UseSkill{Skill: 10214, At: &m.Pos})
	if mist.MPCost != (baseMist.MPCost*45+99)/100 {
		t.Fatal("native trap release failed window", mist.MPCost, baseMist.MPCost)
	}
}

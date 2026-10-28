package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestLegendaryHealerFunctionalCleanseEscort(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-U07")
	h := legendaryHealerCleanseEscort()
	c.Skill = 10306
	s.refreshStats(c.Owner)
	c.Owner.MP = 0
	c.PaidMP = 100
	h.RootFinished(c) // Empty cleanse is still a successful release.
	var applied domain.Active
	count := 0
	c.Owner.Status.Each(func(a domain.Active) {
		if a.Def.ID == legendaryHealerEscortStatus {
			applied = a
			count++
		}
	})
	if count != 1 || applied.Def.DurationSec != 8 || applied.Def.PhysicalDamageReductionPct != 25 || applied.Def.MagicDamageReductionPct != 25 || applied.Def.Affixes[0].Value != 60 {
		t.Fatalf("wrong escort: count=%d status=%+v", count, applied)
	}
	if c.Owner.MP != 0 {
		t.Fatal("obsolete empty-cleanse refund survived")
	}
	h.RootFinished(c)
	c.RootID++
	h.RootFinished(c)
	count = 0
	c.Owner.Status.Each(func(a domain.Active) {
		if a.Def.ID == legendaryHealerEscortStatus {
			count++
		}
	})
	if count != 1 {
		t.Fatalf("refresh stacked %d copies", count)
	}
	delete(c.Owner.Player.Char.Skills, 10306)
	h.EquipmentChanged(c)
	if c.Owner.Status.Has(legendaryHealerEscortStatus) {
		t.Fatal("respec retained escort")
	}
	c.RootID++
	h.RootFinished(c)
	if c.Owner.Status.Has(legendaryHealerEscortStatus) {
		t.Fatal("unlearned cleanse granted escort")
	}
	c.Owner.Player.Char.Skills[10306] = 1
	c.IsChild = true
	h.RootFinished(c)
	if !c.Owner.Status.Has(legendaryHealerEscortStatus) {
		t.Fatal("distinct generated cleanse did not grant learned escort")
	}
	c.IsChild = false
	c.RootID++
	h.RootFinished(c)
	c.Owner.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
	s.pruneLegendaryOwner(c.Owner)
	if c.Owner.Status.Has(legendaryHealerEscortStatus) {
		t.Fatal("unequip retained escort")
	}
}

func TestLegendaryHealerFunctionalFarClinic(t *testing.T) {
	_, c := legendaryCasterFixture(t, "YS-U08")
	h := legendaryHealerFarClinic()
	base := domain.SkillDef{ID: 10301, TargetTeam: true, Dist: 100}
	d := base
	h.PrepareSkill(c, &d)
	if d.Dist != 100 || d.LegendaryModifiers.ManaReductionBP != 0 {
		t.Fatal("baseline free clinic")
	}
	c.Skill = 10327
	h.CastReleased(c)
	d = base
	h.PrepareSkill(c, &d)
	if d.Dist != 500 || d.LegendaryModifiers.ManaReductionBP != 4500 {
		t.Fatalf("wrong clinic %+v", d)
	}
	c.NowMS += 1000
	h.CastReleased(c)
	if got := c.State(0, "far-clinic").ExpiresMS; got != c.NowMS+12000 {
		t.Fatalf("refresh did not replace duration: %d", got)
	}
	d = domain.SkillDef{ID: 10327}
	h.PrepareSkill(c, &d)
	if d.Dist != 600 || !d.LegendaryRevealPoint {
		t.Fatal("reveal point lost")
	}
	d = domain.SkillDef{ID: 10319, TargetEnemy: true, Dist: 100}
	h.PrepareSkill(c, &d)
	if d.Dist != 100 || d.LegendaryModifiers.ManaReductionBP != 0 {
		t.Fatal("enemy spell got friendly clinic")
	}
	c.NowMS += 12000
	d = base
	h.PrepareSkill(c, &d)
	if d.Dist != 100 {
		t.Fatal("expired clinic remains")
	}
	h.CastReleased(c)
	delete(c.Owner.Player.Char.Skills, 10327)
	d = base
	h.PrepareSkill(c, &d)
	if d.Dist != 100 {
		t.Fatal("respec clinic remains")
	}
	h.Tick(c)
	if c.State(0, "far-clinic").ExpiresMS != 0 {
		t.Fatal("respec window not pruned")
	}
	c.Owner.Player.Char.Skills[10327] = 1
	c.IsChild = true
	h.CastReleased(c)
	if c.State(0, "far-clinic").ExpiresMS != c.NowMS+12000 {
		t.Fatal("distinct generated reveal did not grant learned clinic")
	}
}

func TestLegendaryHealerFunctionalRejuvenationGuard(t *testing.T) {
	_, c := legendaryCasterFixture(t, "YS-D05")
	h := legendaryHealerRejuvenationGuard()
	if h.BeforeIncoming(c, 100) != 100 {
		t.Fatal("free guard without rejuvenation")
	}
	c.Owner.Status.Apply(domain.StatusDef{ID: 199981, Level: 1, DurationSec: 30, SourceSkillID: 10303}, c.Scene.tick, c.Owner.ID)
	if h.BeforeIncoming(c, 100) != 75 {
		t.Fatal("guard still requires actual windup")
	}
	delete(c.Owner.Player.Char.Skills, 10303)
	if h.BeforeIncoming(c, 100) != 100 {
		t.Fatal("unlearned rejuvenation grants guard")
	}
}

func TestLegendaryHealerFunctionalHealingPrismBatchDeduplicates(t *testing.T) {
	s, p, first := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, "YS-A23", "YS-A01", "YS-A03")
	p.HP = 1
	s.aoi.Move(first, domain.Pos{MapID: p.Pos.MapID, X: 230, Y: 100})
	primaries := []domain.EntityID{first.ID}
	for i, x := range []float64{250, 270, 290, 310} {
		m := healerSoloMonster(s, domain.EntityID(901+i), x, 100)
		primaries = append(primaries, m.ID)
	}
	// Outside the heal's 250 radius, but within all five bolts' 180-radius
	// prisms: this target must receive one secondary, not five copies.
	secondary := healerSoloMonster(s, 905, 400, 100)
	outside := healerSoloMonster(s, 906, 600, 100)
	s.outbox = nil
	nativeRepairCast(t, s, p, 10301, p.ID)
	if p.HP != 1101 {
		t.Fatalf("batch sacrificed original healing: %d", p.HP)
	}
	for _, id := range primaries {
		damage, count := healerSoloCount(s, id)
		if damage != 1760 || count != 1 {
			t.Fatalf("primary %d received %d damage in %d hits; want one full 1760 hit", id, damage, count)
		}
	}
	if damage, count := healerSoloCount(s, secondary.ID); damage != 1056 || count != 1 {
		t.Fatalf("overlapping secondary received %d damage in %d hits; want one 60%% 1056 hit", damage, count)
	}
	if damage, count := healerSoloCount(s, outside.ID); damage != 0 || count != 0 {
		t.Fatalf("batch recurred beyond its range: damage=%d hits=%d", damage, count)
	}
}

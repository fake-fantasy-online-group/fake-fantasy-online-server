package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestTypifiedReductionIsIsolatedFromSelectiveFloor(t *testing.T) {
	s, p, m := universalSoloFixture(t, "TY-D05", domain.Warrior, domain.Learned{})
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
	p.HP, p.MaxHP = 10000, 10000
	before := p.HP
	s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true, Area: true, SkillPct: 100})
	if got := before - p.HP; got != 400 {
		t.Fatalf("typified area 60%% got %d want 400 (must not inherit the 25%% floor)", got)
	}
}

func TestTypifiedGeneratedShieldClearsOnLeaveAndSwap(t *testing.T) {
	s, p, _ := universalSoloFixture(t, "TY-U13", domain.Warrior, domain.Learned{10002: 1})
	p.MaxHP, p.HP = 1000, 1000
	if p.Status == nil {
		p.Status = domain.NewStatusSet()
	}
	p.Status.Apply(domain.StatusDef{
		ID: typifiedStatusID("TY-U13"), Level: 1, Name: "cast shield", TypeKnown: true, Type: domain.StatusGood,
		DurationSec: 5, DamageShield: 250, LegendaryPowerID: "TY-U13", LegendarySourceUID: 991,
	}, s.tick, p.ID)
	if universalShieldValue(p) != 250 {
		t.Fatalf("U13 typified shield not applied: %d", universalShieldValue(p))
	}
	typifiedClearHeldGeneratedShields(s, p)
	if universalShieldValue(p) != 0 {
		t.Fatalf("typified shield survived leave/swap clear: %d", universalShieldValue(p))
	}
}

func TestTypifiedHealRestoreDoNotRetrigger(t *testing.T) {
	s, p, m := universalSoloFixture(t, "TY-U04", domain.Healer, domain.Learned{10301: 1})
	p.MaxMP, p.MP = 1000, 500
	p.MaxHP, p.HP = 1000, 1000
	m.MaxHP, m.HP = 2000, 1000
	m.Player = nil
	// Direct typified heal/restore helpers must be a no-op once already on the typified resource path.
	cs := s.legendaryContextsFor(p, m, combat.Blow{SkillID: 10301}, event.DamageDealt{Dst: m.ID}, true)
	if len(cs) != 1 {
		t.Fatalf("missing U04 context: %d", len(cs))
	}
	c := cs[0]
	c.CausalPath = typifiedResourcePath("TY-U04")
	if typifiedHeal(&c, m, 100) != 0 || typifiedRestoreMP(&c, p, 100) != 0 {
		t.Fatal("typified resource path failed to block recursive heal/mana")
	}
}

func TestTypifiedWrongClassDoesNotActivate(t *testing.T) {
	s, p, _ := universalSoloFixture(t, "TY-U05", domain.Healer, domain.Learned{})
	if n := len(domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)); n != 1 {
		t.Fatalf("凝疗 should activate on a 药师: %d", n)
	}
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.EmploymentKnown, p.Player.Char.Employed = true, true
	if n := len(domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)); n != 0 {
		t.Fatalf("药师-limited 凝疗 activated on a warrior: %d", n)
	}
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestUniqueRestorationIndependentTraitAncestry(t *testing.T) {
	for _, prior := range []string{"", "unique:kill_mend:0", "unique:kill_mana:0", "unique:kill-restoration"} {
		t.Run(prior, func(t *testing.T) {
			s, p, _ := legendaryMeleeFixture(t, "ZS-A03")
			cached := s.equipmentEffects(p)
			cached.traits = []domain.UniqueTrait{{Kind: "kill_mend", PowerBP: 500}, {Kind: "kill_mana", PowerBP: 500}}
			s.equipmentState().cache[p.ID] = cached
			p.MaxHP, p.MaxMP, p.HP, p.MP = 1000, 1000, 500, 500
			m := putMonster(s, 900, 100, domain.Stats{}, 110, 100)
			if prior != "" {
				defer s.withCombatCausalPath([]string{s.combatEffectKey(p.ID, prior)})()
			}
			s.afterUniqueStrike(p, m, event.DamageDealt{Src: p.ID, Dst: m.ID, Amount: 100, Flag: event.DamageFatal})
			wantHP, wantMP := int32(550), int32(550)
			if prior == "unique:kill_mend:0" || prior == "unique:kill-restoration" {
				wantHP = 500
			}
			if prior == "unique:kill_mana:0" || prior == "unique:kill-restoration" {
				wantMP = 500
			}
			if p.HP != wantHP || p.MP != wantMP {
				t.Fatalf("got %d/%d want %d/%d", p.HP, p.MP, wantHP, wantMP)
			}
		})
	}
}

func TestUniqueHealConsumerKillAllowsDistinctManaProducer(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	cached := s.equipmentEffects(p)
	cached.traits = []domain.UniqueTrait{{Kind: "kill_mend", PowerBP: 500}, {Kind: "kill_mana", PowerBP: 500}}
	s.equipmentState().cache[p.ID] = cached
	p.MaxHP, p.MaxMP, p.HP, p.MP = 1000, 1000, 500, 500
	old := legendaryHandlers["ZS-A03"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"] = old })
	calls := 0
	legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", Healed: func(c *legendaryContext, before, amount int32) {
		calls++
		c.Scene.afterUniqueStrike(p, m, event.DamageDealt{Src: p.ID, Dst: m.ID, Amount: 100, Flag: event.DamageFatal})
	}}
	s.afterUniqueStrike(p, m, event.DamageDealt{Src: p.ID, Dst: m.ID, Amount: 100, Flag: event.DamageFatal})
	// Original kill heals once; its healing consumer creates a distinct kill,
	// whose mana trait remains eligible. Same heal ancestry cannot repeat.
	if calls != 1 || p.HP != 550 || p.MP != 600 {
		t.Fatalf("calls%d hp%d mp%d", calls, p.HP, p.MP)
	}
	if len(s.legendaryState().causalPath) != 0 {
		t.Fatal("scope leaked")
	}
}

func TestUnifiedSetRejectedOwnerPreservesNeutralMultiplier(t *testing.T) {
	s := newTestScene(t, nil)
	if got := s.classSetMeleeCastReleased(nil, &domain.SkillDef{}); got != 10000 {
		t.Fatalf("neutral multiplier=%d", got)
	}
}

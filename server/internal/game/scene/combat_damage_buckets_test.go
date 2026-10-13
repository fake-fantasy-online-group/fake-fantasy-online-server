package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
)

func TestCombatDamageBucketOffenseSourceSubstitutionAndLiveCondition(t *testing.T) {
	for _, source := range []string{"equipment", "condition", "aura"} {
		t.Run(source, func(t *testing.T) {
			s := newCombatScene(t)
			join(t, s, 1, 100, "source", 100, 100)
			p := s.players[1]
			p.Stats = domain.Stats{MAtk: 1000}
			p.HP, p.MaxHP = 10000, 10000
			target := putMonster(s, 900, 100000, domain.Stats{}, 110, 100)
			c := s.equipmentEffects(p)
			switch source {
			case "equipment":
				c.effects = []domain.ActiveEquipmentEffect{{SourceUID: 991, Item: 999991, Effect: domain.EquipmentEffect{Kind: "element_damage_bp", Element: "fire", Value: 1000}}}
				c.stats = domain.AggregateEquipmentCombat(c.effects)
				s.equipmentState().cache[p.ID] = c
			case "condition":
				c.traits = []domain.UniqueTrait{{Kind: "healthy_assault", PowerBP: 1000, ThresholdPct: 50}}
				s.equipmentState().cache[p.ID] = c
			case "aura":
				cacheTestAura(s, p, 991, 1000)
				aura := s.equipmentState().cache[p.ID].effects[0].Effect.Aura
				aura.Modifier, aura.Element = "element_damage_bp", "fire"
				s.stepEquipmentAuras()
			}
			strike := func() int32 {
				ev, _, _ := s.resolveStrikeStatuses(p, target, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
				return ev.Amount
			}
			if got := strike(); got != 1100 {
				t.Fatal("same 10% bonus changed with source", source, got)
			}
			// A dynamic condition or live source withdrawal affects the next bucket
			// assembly; it never mutates Stats or the global skill/base definition.
			if source == "condition" {
				p.HP = 1000
			} else {
				delete(s.equipmentState().cache, p.ID)
			}
			if got := strike(); got != 1000 {
				t.Fatal("inactive source remained in bucket", source, got)
			}
			if p.Stats.MAtk != 1000 {
				t.Fatal("modifier rewrote native attack stats")
			}
		})
	}
}

func TestCombatDamageBucketDefenseSingleLayerCompatibility(t *testing.T) {
	for _, source := range []string{"equipment", "condition", "status", "aura"} {
		t.Run(source, func(t *testing.T) {
			s := newCombatScene(t)
			join(t, s, 1, 100, "target", 100, 100)
			p := s.players[1]
			p.Stats = domain.Stats{}
			p.HP, p.MaxHP = 10000, 10000
			attacker := putMonster(s, 900, 100000, domain.Stats{MAtk: 1000}, 110, 100)
			c := s.equipmentEffects(p)
			switch source {
			case "equipment":
				c.effects = []domain.ActiveEquipmentEffect{{SourceUID: 991, Item: 999991, Effect: domain.EquipmentEffect{Kind: "element_mitigation_bp", Element: "fire", Value: 1000}}}
				c.stats = domain.AggregateEquipmentCombat(c.effects)
				s.equipmentState().cache[p.ID] = c
			case "condition":
				c.traits = []domain.UniqueTrait{{Kind: "healthy_guard", PowerBP: 1000, ThresholdPct: 50}}
				s.equipmentState().cache[p.ID] = c
			case "status":
				p.Status.Apply(domain.StatusDef{ID: 99999, DurationSec: 30, MagicDamageReductionPct: 10}, s.tick, p.ID)
			case "aura":
				cacheTestAura(s, p, 991, 1000)
				aura := s.equipmentState().cache[p.ID].effects[0].Effect.Aura
				aura.Modifier, aura.Element = "element_resist_bp", "fire"
				s.stepEquipmentAuras()
			}
			ev, _, _ := s.resolveStrikeStatuses(attacker, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
			if ev.Amount != 900 || p.HP != 9100 {
				t.Fatal("single 10% layer changed with source", source, ev.Amount, p.HP)
			}
		})
	}
}

func TestCombatDamageBucketGuardAndAuraShareMitigationCap(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "target", 100, 100)
	p := s.players[1]
	p.Stats = domain.Stats{}
	p.HP, p.MaxHP = 10000, 10000
	attacker := putMonster(s, 900, 10000, domain.Stats{MinAtk: 1000, MaxAtk: 1000}, 110, 100)
	c := s.equipmentEffects(p)
	c.traits = []domain.UniqueTrait{{Kind: "physical_guard", PowerBP: 3000}}
	s.equipmentState().cache[p.ID] = c
	cacheTestAura(s, p, 991, 3000)
	s.stepEquipmentAuras()
	ev, _, _ := s.resolveStrikeStatuses(attacker, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	// Guard + aura add into one 45%-capped bucket: 1000 * (1-.45)=550.
	// Applying 30% independently twice would give 490 and is prohibited here.
	if ev.Amount != 550 {
		t.Fatal("same mitigation bucket compounded or escaped its cap", ev.Amount)
	}
	p.Status.Apply(domain.StatusDef{ID: 99999, DurationSec: 30, PhysicalDamageReductionPct: 30}, s.tick, p.ID)
	ev, _, _ = s.resolveStrikeStatuses(attacker, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	// Native status reduction deliberately remains a separate multiplicative
	// layer in the retained math; it is not interchangeable with equipment BP.
	if ev.Amount != 385 {
		t.Fatal("separate native status layer was flattened into equipment bucket", ev.Amount)
	}
}

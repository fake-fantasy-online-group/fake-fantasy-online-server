package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestLegendaryCompositionPhasesIndependentOfIDOrder(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	s.items[900002] = domain.ItemDef{ID: 900002, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1}}}
	p.Player.Worn.Set(domain.SlotBody, domain.Stack{UID: 992, Item: 900002, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: "ZS-A04"}})
	a, b := legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"] = a, b })
	legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if c.Skill != 10015 {
			t.Errorf("modifier saw original skill%d", c.Skill)
		}
		scaleVariantDamage(d, 200)
	}}
	legendaryHandlers["ZS-A04"] = legendaryHandler{revision: "synergy-v1", TransformSkill: func(c *legendaryContext, d *domain.SkillDef) { d.ID = 10015 }, BaseSkill: func(c *legendaryContext, d *domain.SkillDef) { d.Effects[0].BaseDamageFlat += 50 }}
	def := domain.SkillDef{ID: 10011, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &def, m.ID)
	if def.Effects[0].BaseDamageFlat != 100 {
		t.Fatalf("base modified out of order: %+v", def.Effects[0])
	}
}
func TestLegendaryChildDistinctProducerInheritsModifiersWithoutCycle(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	s.items[900002] = domain.ItemDef{ID: 900002, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1}}}
	p.Player.Worn.Set(domain.SlotBody, domain.Stack{UID: 992, Item: 900002, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: "ZS-A04"}})
	a, b := legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"] = a, b })
	def := domain.SkillDef{ID: 10011, HurtType: domain.HurtPhysical, Dist: 500, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
	generators := 0
	generate := func(c *legendaryContext) { generators++; c.ChildSkill(m.ID, def, legendaryChildOptions{}) }
	legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", SkillResolved: generate}
	legendaryHandlers["ZS-A04"] = legendaryHandler{revision: "synergy-v1", SkillResolved: generate, ModifyDamage: func(c *legendaryContext, n int32) int32 { return n * 2 }}
	c := s.legendaryContexts(p, m, combat.Blow{SkillID: 10011}, event.DamageDealt{})[0]
	before := m.HP
	if !c.ChildSkill(m.ID, def, legendaryChildOptions{}) {
		t.Fatal("child rejected")
	}
	if generators != 1 {
		t.Fatalf("distinct producer/cycle guard count=%d", generators)
	}
	if before-m.HP != 400 {
		t.Fatalf("modifier inheritance damage=%d want400", before-m.HP)
	}
	if s.legendaryState().child != nil || s.legendaryState().depth != 0 {
		t.Fatal("lineage scope leaked")
	}
}

func TestLegendaryPreviewUsesDetachedStateAndNoActionBoundaries(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	control, _, _ := legendaryMeleeFixture(t, "ZS-A03")
	old := legendaryHandlers["ZS-A03"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"] = old })
	p.MP = 100
	hp, mp, targetHP := p.HP, p.MP, m.HP
	seq := s.equipmentState().sequence
	legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if !c.Preview {
			t.Fatal("projection did not set preview context")
		}
		c.State(0, "preview").Stacks++
		c.Damage(m, 100, false, false)
		c.RefundMP(100)
		c.ChildSkill(m.ID, *d, legendaryChildOptions{})
		c.Visual("stage-01", m)
		c.Ready(1000)
		d.Dist += 50
	}}
	for i := 0; i < 2; i++ {
		d := domain.SkillDef{ID: 10001, Kind: domain.SkillSingle, Dist: 100, TargetEnemy: true, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
		s.legendaryPreviewSkill(p, &d)
		if d.Dist != 150 {
			t.Fatalf("preview%d distance%d", i, d.Dist)
		}
	}
	if p.HP != hp || p.MP != mp || m.HP != targetHP || s.equipmentState().sequence != seq || len(s.legendaryState().states) != 0 || len(s.legendaryState().cooldowns) != 0 {
		t.Fatal("projection mutated live action state")
	}
	if s.rng.Intn(1000000) != control.rng.Intn(1000000) {
		t.Fatal("projection consumed RNG")
	}
	p.HP = 0
	d := domain.SkillDef{ID: 10001, Dist: 100}
	s.legendaryPreviewSkill(p, &d)
	if d.Dist != 150 {
		t.Fatal("dead player lost equipment metadata")
	}
}

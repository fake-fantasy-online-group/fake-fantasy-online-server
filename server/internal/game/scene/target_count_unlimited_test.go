package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestTargetCountUnlimitedChildAndRoot(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	for i := 0; i < 300; i++ {
		putMonster(s, domain.EntityID(1000+i), 100000, domain.Stats{}, 140, 100)
	}
	outside := putMonster(s, 2000, 100000, domain.Stats{}, 1000, 1000)
	dead := putMonster(s, 2001, 0, domain.Stats{}, 140, 100)
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	if got := len(c.Enemies(p.Pos, 150, 0)); got != 301 {
		t.Fatalf("unlimited helper got %d", got)
	}
	def := domain.SkillDef{ID: 10011, Kind: domain.SkillArea, Dist: 500, TargetEnemy: true, HurtType: domain.HurtPhysical, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 150}, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
	ids := s.resolveTargets(p, def, m.ID, nil)
	ids = s.legendarySelectTargets(p, def, m.ID, ids)
	if len(ids) != 301 {
		t.Fatalf("native legendary selection truncated: %d", len(ids))
	}
	if !c.ChildSkill(m.ID, def, legendaryChildOptions{SuppressGeneration: true}) {
		t.Fatal("child rejected")
	}
	for i := 0; i < 300; i++ {
		if s.entities[domain.EntityID(1000+i)].HP == 100000 {
			t.Fatalf("child dropped target %d", i)
		}
	}
	if outside.HP != 100000 || dead.HP != 0 {
		t.Fatal("range/dead predicate changed")
	}
	root := &equipmentRoot{src: p.ID, id: 77}
	s.equipmentState().root = root
	c.RootID = 77
	for i := 0; i < 300; i++ {
		dst := s.entities[domain.EntityID(1000+i)]
		c.Damage(dst, 10, false, false)
		s.collectEquipmentHit(p, dst, combat.Blow{SkillID: 10011}, event.DamageDealt{Amount: 1})
	}
	if len(root.legendaryDamage) != 300 || len(root.hits) != 300 {
		t.Fatalf("root truncated %d payloads/%d hits", len(root.legendaryDamage), len(root.hits))
	}
	s.equipmentState().root = nil
}

func TestTargetCountUnlimitedSelections(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	ids := []domain.EntityID{m.ID}
	for i := 0; i < 12; i++ {
		e := putMonster(s, domain.EntityID(1000+i), 100000, domain.Stats{}, 140, 100)
		ids = append(ids, e.ID)
	}
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	c.Primary = m.ID
	c.Def.LegendaryBasic = true
	if got := universalBasicTargets(&c, ids); len(got) != len(ids) || got[0] != m.ID {
		t.Fatal("fan headcount or primary changed")
	}
	c.Def.LegendaryBasic = false
	c.Def.LegendaryModifiers.UniversalSingleCircle = true
	c.Def.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: 120}
	if got := universalCircleTargets(&c, ids); len(got) != len(ids) || got[0] != m.ID {
		t.Fatal("circle headcount or primary changed")
	}
	def := domain.SkillDef{SpecialWeaponTargetLimit: 2, EquipmentVariant: &domain.EquipmentSkillVariant{MaxTargets: 2}}
	if got := s.limitSpecialWeaponTargets(p, def, ids, m.ID); len(got) != len(ids) {
		t.Fatal("legacy weapon headcount applied")
	}
	if got := s.limitVariantTargets(p, def, ids, m.ID); len(got) != len(ids) {
		t.Fatal("legacy variant headcount applied")
	}
	specialCircle(&def, 150, domain.SkillAreaCenterTarget, 5)
	if def.SpecialWeaponTargetLimit != 0 {
		t.Fatal("new circle retained headcount")
	}
}

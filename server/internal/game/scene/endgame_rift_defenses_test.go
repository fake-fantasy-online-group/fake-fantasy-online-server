package scene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
)

func TestRiftMonsterDisabledDefensesScopedAndRestored(t *testing.T) {
	defenses := []domain.StatusDef{
		{Invulnerable: true}, {PhysicalDamageReductionPct: 100}, {MagicDamageReductionPct: 100},
		{PhysicalReflectPct: 100}, {MagicReflectPct: 100}, {PhysicalReflectFlat: 999},
		{MagicReflectFlat: 999, ReflectFlatAsMagic: true}, {MagicFullReflectChancePct: 100},
	}
	for i, def := range defenses {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			s, m, p, _ := riftEliteFixture(t, 3)
			if def.ID == 0 {
				def.ID = 890001
			}
			def.Level, def.DurationSec = 1, 30
			if s.applyStatusDef(m, def, m.ID) {
				t.Fatal("rift monster received disabled defense")
			}
			if !s.applyStatusDef(p, def, p.ID) {
				t.Fatal("player defense changed")
			}
			p.Status.RemoveWhere(func(domain.StatusDef) bool { return true })
			// Bypass admission to model a legacy saved status or copied clone status.
			m.Status.Apply(def, s.tick, m.ID)
			p.HP, p.MaxHP = 10000, 10000
			for _, magic := range []bool{false, true} {
				before := m.HP
				ev, raw, full := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{FlatDamage: 100, GuaranteedHit: true, Magic: magic})
				s.afterStrikeStatuses(p, m, ev, raw, full)
				if m.HP >= before || full != 0 || p.HP != 10000 {
					t.Fatalf("defense still active: %+v full=%d playerHP=%d", ev, full, p.HP)
				}
			}
			m.Status.Apply(def, s.tick, m.ID)
			s.stepStatus()
			if m.Status.Has(def.ID) {
				t.Fatal("saved defense survived status tick")
			}
			s.rift = nil
			if !s.applyStatusDef(m, def, m.ID) {
				t.Fatal("ordinary map defense changed")
			}
		})
	}
}

func TestRiftGoldenGenerationAndSavedRulesCompatibility(t *testing.T) {
	s, m, _, now := riftEliteFixture(t, 0)
	original, _ := json.Marshal(s.currentRiftRules())
	seenGolden := false
	for class := int32(1); class <= 3; class++ {
		if class == 3 {
			m.Monster.TypeID = 1011
		}
		for seed := uint64(0); seed < 256; seed++ {
			s.registerEndgameRiftAbilities(m, class, 0, seed, 1)
			ids := s.riftMonsterAbilityIDs(m.ID)
			if len(ids) != int(class) {
				t.Fatal("elite ability allocation changed", class, seed, ids)
			}
			unique := map[int32]bool{}
			for _, id := range ids {
				if unique[id] {
					t.Fatal("duplicate ability", ids)
				}
				unique[id] = true
				seenGolden = seenGolden || id == 0
			}
		}
	}
	if !seenGolden || !reflect.DeepEqual(s.riftMonsterAbilityIDs(m.ID), []int32{0, 3, 6}) {
		t.Fatal("golden/guardian abilities not restored")
	}
	s.rift.abilities[m.ID].pattern = []endgameRiftAbility{0, 3}
	s.eliteSnapshot().Casters[s.rift.abilities[m.ID].eliteKey].Ready[0] = *now
	s.stepRiftElite(*now)
	if !m.Status.Has(riftGoldenStatus) || s.riftGoldenUntil(m.ID, *now) != *now+5000 ||
		!reflect.DeepEqual(s.riftMonsterAbilityIDs(m.ID), []int32{0, 3}) || len(s.riftMonsterAffixes(m.ID)) != 2 {
		t.Fatal("saved golden pattern or metadata not restored")
	}
	after, _ := json.Marshal(s.currentRiftRules())
	if string(original) != string(after) {
		t.Fatal("saved rules mutated")
	}
}

func TestRiftNativeDefenseSkillAndReflectionBoundary(t *testing.T) {
	s, m, p, _ := riftEliteFixture(t, 3)
	d := domain.StatusDef{ID: 890001, Level: 1, DurationSec: 30, Invulnerable: true}
	s.statuses = domain.StatusTable{{ID: d.ID, Level: 1}: d}
	rule := domain.MonsterSkillRule{Self: true, Enabled: true, Skill: domain.SkillDef{Statuses: []domain.StatusApplication{{ID: d.ID, Level: 1}}}}
	if !s.disabledRiftMonsterSkill(m, rule) {
		t.Fatal("native self buff not filtered")
	}
	s.startMonsterSkill(m, rule, []domain.EntityID{m.ID})
	if len(s.casts) != 0 || m.Status.Has(d.ID) {
		t.Fatal("native cast admitted")
	}
	hp := p.HP
	s.emitReflectedDamage(m, p, 100, true)
	if p.HP != hp {
		t.Fatal("rift monster reflection emitted")
	}
	hp = m.HP
	s.emitReflectedDamage(p, m, 100, true)
	if m.HP >= hp {
		t.Fatal("player reflection disabled")
	}
	s.rift = nil
	if s.disabledRiftMonsterSkill(m, rule) {
		t.Fatal("ordinary native skill disabled")
	}
	hp = p.HP
	s.emitReflectedDamage(m, p, 100, true)
	if p.HP >= hp {
		t.Fatal("ordinary reflection disabled")
	}
}

// The merged dungeon DOT path calls Scaled directly instead of the legacy
// wrapper. Legacy saved rift defenses must be cleared on that path too.
func TestRiftDisabledDefensesOnScaledSecondaryPath(t *testing.T) {
	s, m, _, _ := riftEliteFixture(t, 3)
	m.Status = domain.NewStatusSet()
	defense := domain.StatusDef{ID: 890002, Level: 1, DurationSec: 30,
		Invulnerable: true, PhysicalReflectPct: 100, DamageReductionCapPct: 95}
	m.Status.Apply(defense, s.tick, m.ID)
	if got := s.mitigateSecondaryStatusDamageScaled(m, false, 100, false, 10000); got != 100 {
		t.Fatalf("rift DOT still blocked by legacy defense: %d", got)
	}
	if m.Status.Has(defense.ID) {
		t.Fatal("legacy rift defense survived scaled path")
	}
	s.rift = nil
	m.Status.Apply(defense, s.tick, m.ID)
	if got := s.mitigateSecondaryStatusDamageScaled(m, false, 100, false, 15000); got != 0 {
		t.Fatalf("ordinary dungeon immunity changed: %d", got)
	}
	if !m.Status.Has(defense.ID) {
		t.Fatal("ordinary defense was removed")
	}
	m.Status.RemoveWhere(func(domain.StatusDef) bool { return true })
	if got := s.mitigateSecondaryStatusDamageScaled(m, false, 100, false, 15000); got != 150 {
		t.Fatalf("dungeon secondary multiplier lost: %d", got)
	}
}

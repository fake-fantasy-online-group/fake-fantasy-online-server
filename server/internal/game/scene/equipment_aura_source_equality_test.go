package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestEquipmentAuraSourceEqualityPreservesCadenceAndDefinitionChanges(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "source", 100, 100)
	p := s.players[1]
	cacheTestAura(s, p, 991, 1200)
	s.stepEquipmentAuras()
	rt := s.equipmentState()
	if len(rt.auraSources) != 1 || len(rt.auraSchedules) != 1 || len(rt.auras[p.ID]) != 1 {
		t.Fatal("first source did not initialize")
	}
	first := rt.auraSources[0]
	next := rt.nextAuraScan
	expiry := rt.auras[p.ID][first.effect.Family].expires
	s.tick++
	s.stepEquipmentAuras()
	if rt.nextAuraScan != next || rt.auras[p.ID][first.effect.Family].expires != expiry {
		t.Fatal("equal source renewed its schedule before the configured cadence")
	}
	// A definition change must refresh immediately even before the old scan deadline.
	aura := rt.cache[p.ID].effects[0].Effect.Aura
	aura.Value++
	s.stepEquipmentAuras()
	if len(rt.auraSchedules) != 1 || rt.auras[p.ID][aura.Family].effect.Value != aura.Value {
		t.Fatal("definition change did not replace the source grant")
	}
	if _, ok := rt.auraSchedules[first]; ok {
		t.Fatal("definition change retained old schedule")
	}
	// Removing the final source must clear schedules and grants immediately.
	delete(rt.cache, p.ID)
	s.stepEquipmentAuras()
	if len(rt.auraSchedules) != 0 || len(rt.auras) != 0 || rt.auraSources != nil || rt.nextAuraScan != 0 {
		t.Fatal("final source removal retained aura state")
	}
}

func TestEquipmentAuraSourceEqualityPreservesNilDistinction(t *testing.T) {
	s := newCombatScene(t)
	rt := s.equipmentState()
	rt.auraSources = []equipmentAuraSource{}
	rt.nextAuraScan = s.tick + domain.Ticks(1000)
	rt.auraSchedules = map[equipmentAuraSource]equipmentAuraSchedule{
		{owner: 999, uid: 1}: {next: rt.nextAuraScan},
	}
	s.stepEquipmentAuras()
	if rt.auraSources != nil || len(rt.auraSchedules) != 0 || rt.nextAuraScan != 0 {
		t.Fatal("nil source transition failed to clear stale schedule")
	}
}

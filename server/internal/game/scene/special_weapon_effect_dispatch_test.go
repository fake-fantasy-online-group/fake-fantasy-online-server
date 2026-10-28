package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestSpecialWeaponUnifiedDispatchOneProducerAcrossTwoInstances(t *testing.T) {
	s, p, _ := specialWeaponActors(t)
	a, _ := domain.RuntimeSpecialWeaponEffect(1323, 4, "heal", true)
	b := a
	a.SourceUID, b.SourceUID = 101, 202
	a.Item, b.Item = 1800001323, 1800001323
	count := 0
	if !s.dispatchEquipmentEffect(p, a, func() {
		count++
		if s.dispatchEquipmentEffect(p, b, func() { count++ }) {
			t.Fatal("same effect on second instance reentered")
		}
		modifier := b
		modifier.Producer = false
		if !s.dispatchEquipmentEffect(p, modifier, func() { count++ }) {
			t.Fatal("pure modifier rejected")
		}
		other, _ := domain.RuntimeSpecialWeaponEffect(1325, 4, "heal", true)
		if !s.dispatchEquipmentEffect(p, other, func() { count++ }) {
			t.Fatal("distinct weapon effect rejected")
		}
	}) {
		t.Fatal("root denied")
	}
	if count != 3 {
		t.Fatalf("callback count=%d", count)
	}
	if !s.dispatchEquipmentEffect(p, b, func() { count++ }) || count != 4 {
		t.Fatal("sibling/root incorrectly suppressed")
	}
}

func TestSpecialWeaponUnifiedModifierDelegatesExactlyOnce(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 2)
	p.Player.Char.Skills = domain.Learned{10102: 1}
	specialWeaponEquip(t, s, p, 1021)
	// An inconsistent persisted dual-slot duplicate must not invoke both the old
	// and unified source paths or multiply the same definition twice.
	duplicate := p.Player.Worn.At(domain.SlotWeapon)
	duplicate.UID++
	p.Player.Worn.Set(domain.SlotTwoHand, duplicate)
	s.invalidateCombatAttributes(p)
	original, ok := s.skills.Get(10102, 1)
	if !ok {
		t.Fatal("skill")
	}
	want := original
	applySpecialWeaponDefinition(1021, &want)
	got := original
	s.applySpecialWeaponModifiers(p, &got, p.Player.Char.Skills)
	if got.Effect.DamagePct != want.Effect.DamagePct || got.SpecialWeaponBaseID != 1021 {
		t.Fatalf("old+new duplicate or skipped: got=%+v want=%+v", got.Effect, want.Effect)
	}
	denied := original
	s.applySpecialWeaponModifiers(p, &denied, domain.Learned{})
	if denied.SpecialWeaponBaseID != 0 {
		t.Fatal("registry learned skill without hard rank")
	}
}

func TestSpecialWeaponUnifiedNilDefinitionsAreInert(t *testing.T) {
	s, p, m := specialWeaponActors(t)
	before := m.HP
	s.specialWeaponBeforeHit(p, m, nil)
	s.specialWeaponAim(p, nil, m.ID)
	s.applySpecialWeaponModifiers(p, nil, domain.Learned{})
	if m.HP != before {
		t.Fatal("nil skill changed target")
	}
}

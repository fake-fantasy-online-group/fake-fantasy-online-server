package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
)

func TestClassSetPiecesUsesFunctionalGroupedEquipment(t *testing.T) {
	s, _ := equipScene(t)
	p := s.entities[1]
	p.Player.Char.Race = domain.Assassin
	p.Player.Char.Level = 40
	suit := &domain.EquipmentSuit{ID: 1, Groups: [][]domain.ItemID{{90001, 90002}, {90003}, {90004}}}
	for i, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotHead, domain.SlotBody, domain.SlotShoe} {
		id := domain.ItemID(90001 + i)
		need := domain.Requirement{}
		if i == 3 {
			need.Level = 90
		}
		s.items[id] = domain.ItemDef{ID: id, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100, Suit: suit, Need: need}}
		p.Player.Worn.Set(slot, domain.Stack{Item: id, Count: 1, Durability: 100, MaxDurability: 100})
	}
	if got := s.classSetPieces(p, 1); got != 2 {
		t.Fatalf("grouped functional count=%d, want2", got)
	}
	st := p.Player.Worn.At(domain.SlotBody)
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotBody, st)
	if got := s.classSetPieces(p, 1); got != 1 {
		t.Fatalf("broken item counted: %d", got)
	}
	p.Player.Char.Race = domain.Warrior
	if got := s.classSetPieces(p, 1); got != 0 {
		t.Fatalf("wrong class activated: %d", got)
	}
	if got := s.classSetPieces(p, 6); got != 0 {
		t.Fatalf("unregistered suit activated: %d", got)
	}
}

func TestClassSetPiecesNilInputs(t *testing.T) {
	s, _ := equipScene(t)
	for _, p := range []*entity.Entity{nil, {}, {Player: &entity.Player{}}} {
		if got := s.classSetPieces(p, 1); got != 0 {
			t.Fatalf("invalid player=%d", got)
		}
	}
	if got := s.classSetPiecesInWorn(s.entities[1], nil, 1); got != 0 {
		t.Fatalf("nil worn=%d", got)
	}
}

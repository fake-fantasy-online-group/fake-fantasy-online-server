package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// classSetPieces resolves the same functional equipment as the normal stat
// pipeline. Never call it while that pipeline is being prepared; callers there
// must pass their already-resolved worn set to classSetPiecesInWorn.
func (s *Scene) classSetPieces(p *entity.Entity, suitID int32) int32 {
	if s == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return 0
	}
	return s.classSetPiecesInWorn(p, s.functionalWorn(p), suitID)
}

func (s *Scene) classSetPiecesInWorn(p *entity.Entity, worn *domain.EquipSet, suitID int32) int32 {
	if s == nil || p == nil || p.Player == nil || p.Player.Char == nil || worn == nil {
		return 0
	}
	var race domain.Race
	switch suitID {
	case 1:
		race = domain.Assassin
	case 2:
		race = domain.Healer
	case 3:
		race = domain.Warlock
	case 4:
		race = domain.Swordsman
	case 5:
		race = domain.Warrior
	default:
		return 0
	}
	if p.Player.Char.Race != race {
		return 0
	}
	var suit *domain.EquipmentSuit
	worn.Each(func(_ domain.EquipSlot, stack domain.Stack) {
		if suit != nil {
			return
		}
		def, ok := s.itemDef(stack.Item)
		if ok && def.Equip != nil && def.Equip.Suit != nil && def.Equip.Suit.ID == suitID && domain.EquipmentFunctional(stack, def) {
			suit = def.Equip.Suit
		}
	})
	if suit == nil {
		return 0
	}
	pieces, _ := suit.WornMembers(worn, s.itemDef)
	return pieces
}

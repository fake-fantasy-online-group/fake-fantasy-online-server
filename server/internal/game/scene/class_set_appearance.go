package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// The approved A1371 asset contains its own head and equipment. Only the public
// projection changes: character appearance, wardrobe and actual equipment stay
// authoritative and unmodified, so saving or removing the set cannot persist a
// transformed body. The client suppresses its separate head for this body ID.
func (s *Scene) classSetAppearance(p *entity.Entity, base domain.Appearance) domain.Appearance {
 s.legendaryModifyAppearance(p,&base)
	if s.classSetPieces(p, classSetWarrior) >= classSetSecondSkillPieces {
		base.EquipView = [6]uint16{1371, 0, 0, 0, 0, 0}
		base.AtkVariant = 0
	}
	return base
}

func (s *Scene) refreshClassSetAppearance(p *entity.Entity) {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return
	}
	next := s.classSetAppearance(p, p.Player.Char.Appear)
	if next != p.Look.Appearance {
		p.Look.Appearance = next
		s.emit(event.AppearanceChanged{Who: p.ID, Appearance: next})
	}
}

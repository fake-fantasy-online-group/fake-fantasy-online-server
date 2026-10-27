package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func init() {
	registerLegendarySynergyHandler("TY-A35", legendaryHandler{ModifyAppearance: func(c *legendaryContext, look *domain.Appearance) {
		look.AtkDist = uint16(min(int32(65535), universalScaleBP(int32(look.AtkDist), c.RollValue("benefit", 5000))))
	}})
}

// Appearance projection runs during login, death and equipment changes. It must
// not use alive-only combat contexts or mutate the persisted native appearance.
func (s *Scene) legendaryModifyAppearance(owner *entity.Entity, look *domain.Appearance) {
	if owner == nil || owner.Player == nil || owner.Player.Char == nil || look == nil {
		return
	}
	for _, p := range domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char) {
		h, ok := legendaryHandlers[p.Power.ID]
		if !ok || !legendaryHandlerCurrent(p.Power.ID) || h.ModifyAppearance == nil {
			continue
		}
		c := legendaryContext{Scene: s, Owner: owner, Power: p.Power, PowerRollBP: p.PowerRollBP, SourceUID: p.SourceUID}
		h.ModifyAppearance(&c, look)
	}
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) legendaryCasterMoved(e *entity.Entity, from, to domain.Pos, voluntary, teleport bool) {

}
func (s *Scene) legendaryCasterTeleported(e *entity.Entity, from domain.Pos, skill domain.SkillID) {
	if e.Pos == from {
		return
	}
	s.forEachLegendary(e, func(c *legendaryContext) {
		c.Skill = skill
		if h := legendaryHandlers[c.Power.ID].Teleported; h != nil {
			h(c, from)
		}
	})
}

func init() {

}
func legendaryCasterSpeed(c *legendaryContext, pct, seconds int32) {
	id := domain.StatusID(1811000000)
	if c.Power.ID == "SS-U09" {
		id++
	}
	c.Scene.applyStatusDef(c.Owner, domain.StatusDef{LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID, ID: id, Level: 1, Name: legendaryCasterNativeName(c), Icon: legendaryCasterIcon(c), IconKnown: true, Desc: c.Power.Effect, TypeKnown: true, Type: domain.StatusGood, DurationSec: seconds, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: pct}}}, c.Owner.ID)
}

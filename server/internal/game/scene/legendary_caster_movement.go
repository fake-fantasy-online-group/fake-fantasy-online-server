package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) legendaryCasterMoved(e *entity.Entity, from, to domain.Pos, voluntary, teleport bool) {

}
func (s *Scene) legendaryCasterTeleported(e *entity.Entity, from domain.Pos, skill domain.SkillID, sources ...*domain.CombatSourceSnapshotV5) {
	if e.Pos == from {
		return
	}
	var source *domain.CombatSourceSnapshotV5
	if len(sources) > 0 {
		source = sources[0]
	}
	for _, c := range s.legendaryContextsFor(e, nil, combat.Blow{SkillID: int32(skill), SourceOffense: source}, event.DamageDealt{}, true) {
		if h := legendaryHandlers[c.Power.ID].Teleported; h != nil {
			h(&c, from)
		}
	}
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

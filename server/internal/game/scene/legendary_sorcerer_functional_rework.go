package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func legendarySorcererManual(c *legendaryContext, skills ...domain.SkillID) bool {
	if c.IsChild || c.Blow.ProcDepth != 0 || c.Owner == nil || !c.Owner.Alive() || c.Owner.Player == nil {
		return false
	}
	for _, id := range skills {
		if c.Skill == id && c.Owner.Player.Char.Skills[id] > 0 {
			return true
		}
	}
	return false
}
func legendarySorcererIceStride() legendaryHandler {
	return legendaryHandler{RootFinished: func(c *legendaryContext) {
		if legendarySorcererManual(c, legendaryIceBurst, legendaryBlizzard) {
			st := c.State(0, "ice-stride")
			st.ExpiresMS = c.NowMS + 8000
			st.Value = int64(c.Skill)
			st.Name, st.Description, st.IconID, st.Good = c.Power.Name, c.Power.Effect, legendaryCasterIcon(c), true
			c.Scene.refreshEntityStats(c.Owner)
			c.Scene.emitAttributesIfPlayer(c.Owner)
		}
	}, PassiveStats: func(c *legendaryContext, stats *domain.Stats) {
		if legendarySorcererWindow(c, "ice-stride", 0) {
			stats.MoveSpeed = specialScale(stats.MoveSpeed, 100+c.RollValue("move_pct", 80))
		}
	}, Tick: func(c *legendaryContext) {
		st := c.State(0, "ice-stride")
		if st.ExpiresMS != 0 && !legendarySorcererWindow(c, "ice-stride", 0) {
			c.DeleteState(0, "ice-stride")
			c.Scene.refreshEntityStats(c.Owner)
			c.Scene.emitAttributesIfPlayer(c.Owner)
		}
	}}
}
func legendarySorcererWindow(c *legendaryContext, key string, skill domain.SkillID) bool {
	st := c.State(0, key)
	if skill == 0 {
		skill = domain.SkillID(st.Value)
	}
	if c.Owner == nil || !c.Owner.Alive() || c.Owner.Player == nil || c.Owner.Player.Char.Skills[skill] <= 0 {
		c.DeleteState(0, key)
		return false
	}
	return st.ExpiresMS > c.NowMS
}
func legendarySorcererManaReserve() legendaryHandler {
	return legendaryHandler{RootFinished: func(c *legendaryContext) {
		if legendarySorcererManual(c, legendaryFireball, legendaryIceBurst, legendaryFlame, legendaryBlizzard) {
			st := c.State(0, "mana-reserve")
			st.ExpiresMS = c.NowMS + 10000
			st.Value = int64(c.Skill)
			st.Name, st.Description, st.IconID, st.Good = c.Power.Name, c.Power.Effect, legendaryCasterIcon(c), true
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if legendarySorcererWindow(c, "mana-reserve", 0) {
			d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 4500)
		}
	}}
}
func legendarySorcererFarCast() legendaryHandler {
	return legendaryHandler{Teleported: func(c *legendaryContext, from domain.Pos) {
		if c.Skill == 10432 && c.Owner.Player.Char.Skills[10432] > 0 {
			st := c.State(0, "far-cast")
			st.ExpiresMS = c.NowMS + 12000
			st.Name, st.Description, st.IconID, st.Good = c.Power.Name, c.Power.Effect, legendaryCasterIcon(c), true
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if legendarySorcererWindow(c, "far-cast", 10432) && d.TargetEnemy && d.Dist > 0 && d.Area.Center != domain.SkillAreaCenterCaster {
			d.Dist = specialScale(d.Dist, 100+c.RollValue("range_pct", 100))
		}
	}}
}
func (s *Scene) legendarySorcererMayEscape(e *entity.Entity, skill domain.SkillID) bool {
	if skill != 10432 || e == nil || !e.Alive() || e.Player == nil || e.Player.RideAnchor != 0 || e.Player.Char.Skills[10432] <= 0 {
		return false
	}
	for _, c := range s.legendaryContexts(e, nil, combat.Blow{}, event.DamageDealt{}) {
		if c.Power.ID == "SS-U06" {
			return true
		}
	}
	return false
}
func legendarySorcererControlBreak() legendaryHandler {
	return legendaryHandler{Teleported: func(c *legendaryContext, from domain.Pos) {
		if c.Skill != 10432 || c.Owner.Player.Char.Skills[10432] <= 0 || c.Owner.Pos == from || c.Owner.Status == nil {
			return
		}
		c.Scene.statusesRemoved(c.Owner, c.Owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
			return d.Harmful() && (d.Control != domain.ControlNone || d.BlockMove || d.CannotAttack || legendaryCasterSlow(d))
		}))
	}}
}

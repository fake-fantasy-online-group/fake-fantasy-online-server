package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
)

// The two side placements inherit the already prepared native definition.
// generate=false is explicit child lineage: children cannot generate placements.
func (s *Scene) legendaryAssassinTrapPlaced(owner, trap *entity.Entity, generate bool) {
	s.forEachLegendary(owner, func(c *legendaryContext) {
		c.Skill = trap.Trap.Def.ID
		c.Def = trap.Trap.Def
		if h := legendaryHandlers[c.Power.ID].TrapPlaced; h != nil {
			h(c, trap, generate)
		}
	})
	if (trap.Trap.Def.ID == 10213 && legendaryAssassinHas(s, owner, "CK-A11")) || (trap.Trap.Def.ID == 10214 && legendaryAssassinHas(s, owner, "CK-A14")) {
		s.triggerAssassinTrap(trap, false)
	}
}

func legendaryAssassinTriplePlacement(c *legendaryContext, trap *entity.Entity, generate bool) {
	s, owner := c.Scene, c.Owner
	if trap.Trap.Def.ID == 10213 && generate {
		dx, dy := trap.Pos.X-owner.Pos.X, trap.Pos.Y-owner.Pos.Y
		length := math.Hypot(dx, dy)
		if length == 0 {
			dx, dy, length = 1, 0, 1
		}
		// Balance default: side spacing 75% of native blast radius. Reverse the
		// known radius modifiers rather than multiplying a prepared definition again.
		nativeRadius := float64(trap.Trap.Radius) * 10000 / float64(10000+trap.Trap.Def.LegendaryModifiers.RadiusBP)
		for _, sign := range []float64{-1, 1} {
			pos := domain.Pos{MapID: trap.Pos.MapID, X: trap.Pos.X - sign*dy/length*nativeRadius*.75, Y: trap.Pos.Y + sign*dx/length*nativeRadius*.75}
			s.placeAssassinTrap(owner, trap.Trap.Def, pos, trap.Trap.Radius, trap.Trap.StatusDurationPct, false, trap.Trap.SourceOffense)
		}
	}
}

// Native trap hit path: one damage/status application per victim, with the
// accepted offense snapshot. No delayed echo or remote chain detonation.
func (s *Scene) triggerAssassinTrap(trap *entity.Entity, _ bool) {
	if trap == nil || trap.Trap == nil || s.traps[trap.ID] != trap {
		return
	}
	owner := s.entities[trap.Trap.Owner]
	if owner == nil || !owner.Alive() {
		s.removeTrap(trap.ID)
		return
	}
	targets := s.trapTargets(trap)
	payload := trapPayload(trap.Trap.Def)
	s.removeTrap(trap.ID)
	offense := trap.Trap.EquipmentOffense
	currentOffense := s.equipmentOffense(owner)
	for i := range offense.Elements {
		offense.Elements[i].DamageBP = min(offense.Elements[i].DamageBP, currentOffense.Elements[i].DamageBP)
		offense.Elements[i].PierceBP = min(offense.Elements[i].PierceBP, currentOffense.Elements[i].PierceBP)
	}
	primary := domain.EntityID(0)
	if len(targets) > 0 {
		primary = targets[0]
	}
	finish := s.beginEquipmentRoot(owner.ID, primary, offense)
	if trap.Trap.LegendaryParentRootID != 0 {
		s.equipment.root.id = trap.Trap.LegendaryParentRootID
	}
	s.equipment.root.def = payload
	s.equipment.root.targets = targets
	s.equipment.root.source = trap.Trap.SourceOffense
	if trap.Trap.SourceOffense != nil {
		s.equipment.root.currentSource = s.captureCombatSourceV5(owner, entityInvisible(owner))
	}
	if trap.Trap.AttackStatsSnapshot {
		s.equipment.root.attackStats = lesserTrapAttackStats(trap.Trap.AttackStats, owner.Stats)
		s.equipment.root.attackSnapshot = true
	}
	for _, id := range targets {
		target := s.entities[id]
		if target != nil && target.Alive() {
			s.applySkill(owner, target, payload, trap.Trap.StatusDurationPct, true)
		}
	}
	finish()
}

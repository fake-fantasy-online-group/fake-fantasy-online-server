package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
)

// The two side placements inherit the already prepared native definition.
// generate=false is explicit child lineage: children cannot generate placements.
func (s *Scene) legendaryAssassinTrapPlaced(owner, trap *entity.Entity, generate bool) {
	for _, c := range s.legendaryContexts(owner, nil, combat.Blow{SourceOffense: trap.Trap.SourceOffense}, event.DamageDealt{}) {
		c.Skill = trap.Trap.Def.ID
		c.Def = trap.Trap.Def
		if h := legendaryHandlers[c.Power.ID].TrapPlaced; h != nil {
			h(&c, trap, generate)
		}
	}
	if (trap.Trap.Def.ID == 10213 && legendaryAssassinHas(s, owner, "CK-A11", trap.Trap.SourceOffense)) || (trap.Trap.Def.ID == 10214 && legendaryAssassinHas(s, owner, "CK-A14", trap.Trap.SourceOffense)) {
		s.triggerAssassinTrap(trap, false)
	}
}

func legendaryAssassinTriplePlacement(c *legendaryContext, trap *entity.Entity, generate bool) {
	s, owner := c.Scene, c.Owner
	if trap.Trap.Def.ID == 10213 && generate {
		for _, pos := range legendaryAssassinSidePlacements(owner.Pos, trap.Pos, trap.Trap.Radius, trap.Trap.Def) {
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
			// A point-placed trap has no explicitly selected primary victim.
			local := s.legendaryTargetDefinition(owner, payload, target, 0, targets)
			s.applySkill(owner, target, local, trap.Trap.StatusDurationPct, true)
		}
	}
	finish()
}

// Both persistent traps and enhanced-weapon instant bombs use this same native
// side spacing. Prepared radius bonuses must not enlarge the spacing again.
func legendaryAssassinSidePlacements(owner, at domain.Pos, radius int32, def domain.SkillDef) []domain.Pos {
	dx, dy := at.X-owner.X, at.Y-owner.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		dx, dy, length = 1, 0, 1
	}
	nativeRadius := float64(radius) * 10000 / float64(10000+def.LegendaryModifiers.RadiusBP+def.LegendaryModifiers.TrapRadiusBP)
	var positions []domain.Pos
	for _, sign := range []float64{-1, 1} {
		positions = append(positions, domain.Pos{MapID: at.MapID, X: at.X - sign*dy/length*nativeRadius*.75, Y: at.Y + sign*dx/length*nativeRadius*.75})
	}
	return positions
}

package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Flee is an explicit data-driven trait, not an inference from monster level/kind.
// Defaults are server design values: <=20% HP, three seconds, ten-second cooldown.
// Direction search is bounded; an enclosed monster resumes fighting immediately.
func (s *Scene) stepMonsterFlee(m *entity.Entity, target *ai.Target, p ai.Params) bool {
	if s.defs == nil || m == nil || m.Monster == nil {
		return false
	}
	d, ok := s.defs.Def(m.Monster.TypeID)
	if !ok || d.Level > 60 || !d.Flee || m.Monster.NoAttack {
		return false
	}
	state := m.Monster
	if target == nil || !target.Alive || sqDist(m.Pos, target.Pos) > p.TraceDist*p.TraceDist || p.MoveSpeed <= 0 || !canMove(m) {
		state.FleeUntil = 0
		return false
	}
	if state.FleeUntil != 0 && s.tick >= state.FleeUntil {
		state.FleeUntil = 0
		s.clearMonsterChasePath(state)
		s.emit(event.EntityMoved{ID: m.ID, To: m.Pos, Snap: true})
	}
	if state.FleeUntil == 0 {
		hp := d.AI.FleeHPPercent
		if hp <= 0 {
			hp = 20
		}
		if hp > 100 {
			hp = 100
		}
		if m.MaxHP <= 0 || int64(m.HP)*100 > int64(m.MaxHP)*int64(hp) || s.tick < state.NextFleeAt {
			return false
		}
		duration, cooldown := d.AI.FleeDurationTicks, d.AI.FleeCooldownTicks
		if duration <= 0 {
			duration = 30
		}
		if cooldown <= 0 {
			cooldown = 100
		}
		state.FleeUntil = s.tick + duration
		state.NextFleeAt = s.tick + max(duration, cooldown)
		state.Roaming = false
		s.clearMonsterChasePath(state)
		s.monsterSpeak(m, domain.MonsterSceneRunaway)
	}
	dx, dy := m.Pos.X-target.Pos.X, m.Pos.Y-target.Pos.Y
	if dx == 0 && dy == 0 {
		dx = 1
	}
	length := math.Hypot(dx, dy)
	dx /= length
	dy /= length
	// Side steps may escape corners; never choose a step toward the threat.
	for _, rotation := range [...][2]float64{{1, 0}, {math.Sqrt2 / 2, math.Sqrt2 / 2}, {math.Sqrt2 / 2, -math.Sqrt2 / 2}, {0, 1}, {0, -1}} {
		c, sn := rotation[0], rotation[1]
		dest := m.Pos
		dest.X += (dx*c - dy*sn) * 100
		dest.Y += (dx*sn + dy*c) * 100
		to := ai.StepToward(m.Pos, dest, p.MoveSpeed)
		if to != m.Pos && s.straightPathWalkable(m.Pos, to) && sqDist(to, target.Pos) >= sqDist(m.Pos, target.Pos) {
			s.moveEntity(m, to)
			return true
		}
	}
	state.FleeUntil = 0
	s.clearMonsterChasePath(state)
	s.emit(event.EntityMoved{ID: m.ID, To: m.Pos, Snap: true})
	return false
}

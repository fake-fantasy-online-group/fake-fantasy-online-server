package scene

import (
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) placeTrap(owner *entity.Entity, def domain.SkillDef, at domain.Pos, radius int32, statusDurationPct int32) {
	if owner == nil || (owner.Player == nil && owner.Monster == nil) || s.alloc == nil || radius <= 0 {
		return
	}
	trap := &entity.Entity{
		ID: s.alloc.Trap(), Kind: domain.KindTrap, Name: def.Name, Pos: at,
		Trap: &entity.Trap{
			Owner: owner.ID, Def: def, Radius: radius, StatusDurationPct: statusDurationPct,
			VisibleTo: map[domain.EntityID]struct{}{owner.ID: {}},
		},
	}
	s.entities[trap.ID] = trap
	s.traps[trap.ID] = trap
	s.aoi.Enter(trap)
	// 敌方默认看不见；主人立即走正式 0x800f/SpawnTrap 分支看到落点。
	s.emitTo(owner.ID, s.spawnEvent(trap))
}

// stepTraps 在怪物本帧移动之后检查触发。放置发生在更后的 stepCasts，因而新放
// 的陷阱至少稳定存在一帧，不会在同一次施法结算中“先出现又消失”。
func (s *Scene) stepTraps() {
	if len(s.traps) == 0 {
		return
	}
	ids := make([]domain.EntityID, 0, len(s.traps))
	for id := range s.traps {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		trap := s.traps[id]
		if trap == nil || trap.Trap == nil {
			s.removeTrap(id)
			continue
		}
		owner := s.entities[trap.Trap.Owner]
		if owner == nil || !owner.Alive() {
			s.removeTrap(id)
			continue
		}
		targets := s.trapTargets(trap)
		if len(targets) == 0 {
			continue
		}
		payload := trapPayload(trap.Trap.Def)
		for _, targetID := range targets {
			target := s.entities[targetID]
			if target == nil || !target.Alive() {
				continue
			}
			_ = s.applySkill(owner, target, payload, trap.Trap.StatusDurationPct, true)
		}
		s.removeTrap(id)
	}
}

func (s *Scene) trapTargets(trap *entity.Entity) []domain.EntityID {
	if trap == nil || trap.Trap == nil {
		return nil
	}
	r2 := float64(trap.Trap.Radius) * float64(trap.Trap.Radius)
	ids := make([]domain.EntityID, 0)
	owner := s.entities[trap.Trap.Owner]
	if owner == nil {
		return nil
	}
	if owner.Kind == domain.KindMonster {
		// 地面陷阱按坐标触发，不需要先建立怪物仇恨，所以隐身目标仍可被命中。
		// 隐身并非不可攻击：已起手的攻击和其他玩家的攻击同样能正常结算；
		// 普通怪物只是会清掉旧仇恨，且不能重新索敌。
		for id, target := range s.players {
			if target != nil && target.Alive() && sqDist(trap.Pos, target.Pos) <= r2 {
				ids = append(ids, id)
			}
		}
	} else {
		for id, target := range s.monsters {
			if target == nil || target.Monster == nil || !target.Alive() ||
				!target.Monster.Kind.Hostile() || sqDist(trap.Pos, target.Pos) > r2 {
				continue
			}
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func trapPayload(def domain.SkillDef) domain.SkillDef {
	effects := make([]domain.SkillEffect, 0, len(def.DirectEffects()))
	for _, effect := range def.DirectEffects() {
		if effect.Kind != domain.EffectPlaceTrap {
			effects = append(effects, effect)
		}
	}
	def.Effects = effects
	def.Effect = domain.SkillEffect{}
	if len(effects) > 0 {
		def.Effect = effects[0]
	}
	return def
}

func (s *Scene) detectTraps(detector *entity.Entity, radius, chanceBP int32) {
	if detector == nil || radius <= 0 || chanceBP <= 0 {
		return
	}
	r2 := float64(radius) * float64(radius)
	for _, trap := range s.traps {
		if trap == nil || trap.Trap == nil || trap.Trap.Owner == detector.ID ||
			trap.Trap.VisibleToPlayer(detector.ID) || sqDist(detector.Pos, trap.Pos) > r2 {
			continue
		}
		if chanceBP < 10000 && int32(s.rng.Intn(10000)) >= chanceBP {
			continue
		}
		trap.Trap.VisibleTo[detector.ID] = struct{}{}
		s.emitTo(detector.ID, s.spawnEvent(trap))
	}
}

func (s *Scene) disarmTrap(detector *entity.Entity, trapID domain.EntityID, chanceBP int32) {
	trap := s.traps[trapID]
	if detector == nil || trap == nil || trap.Trap == nil ||
		!trap.Trap.VisibleToPlayer(detector.ID) || chanceBP <= 0 {
		return
	}
	if chanceBP < 10000 && int32(s.rng.Intn(10000)) >= chanceBP {
		return
	}
	s.removeTrap(trapID)
}

func (s *Scene) removeOwnerTraps(owner domain.EntityID) {
	ids := make([]domain.EntityID, 0)
	for id, trap := range s.traps {
		if trap != nil && trap.Trap != nil && trap.Trap.Owner == owner {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		s.removeTrap(id)
	}
}

func (s *Scene) removeTrap(id domain.EntityID) {
	trap := s.traps[id]
	if trap == nil {
		return
	}
	if trap.Trap != nil {
		for viewer := range trap.Trap.VisibleTo {
			s.emitTo(viewer, event.EntityDespawned{ID: id, Kind: domain.KindTrap, Reason: event.DespawnRemoved})
		}
	}
	s.aoi.Leave(trap)
	delete(s.entities, id)
	delete(s.traps, id)
}

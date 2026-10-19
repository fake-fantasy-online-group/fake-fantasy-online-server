package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const monsterPickupRadius = 150.0
const monsterPickupCapacity = 8
const monsterPickupScanTicks domain.Tick = 10

// tickMonsterPickup runs only on the scene actor, after combat acquisition. It
// transfers ownership of an existing entity; no inventory, UID or affix reroll.
func (s *Scene) tickMonsterPickup(m *entity.Entity) bool {
	if m == nil || m.Monster == nil || !m.Alive() {
		return false
	}
	mm := m.Monster
	if mm.Target != 0 || !canAct(m) || s.rift != nil || mm.Summoner != 0 || s.defs == nil {
		mm.PickupTarget = 0
		return false
	}
	def, ok := s.defs.Def(mm.TypeID)
	if !ok || !def.Pickup || def.Level > 60 || m.Level > 60 || def.Kind == domain.MonsterProp || def.Kind == domain.MonsterGather {
		return false
	}
	// Expired objects follow the original ground lifetime even while carried.
	kept := mm.CarriedLoot[:0]
	for _, e := range mm.CarriedLoot {
		if e.Drop != nil && e.Drop.ExpireAt > s.tick {
			kept = append(kept, e)
		}
	}
	for i := len(kept); i < len(mm.CarriedLoot); i++ {
		mm.CarriedLoot[i] = nil
	}
	mm.CarriedLoot = kept
	if len(kept) >= monsterPickupCapacity {
		mm.PickupTarget = 0
		return false
	}
	target := s.ground[mm.PickupTarget]
	if !s.monsterCanPickUp(target) || sqDist(m.Pos, target.Pos) > monsterPickupRadius*monsterPickupRadius {
		target = nil
		mm.PickupTarget = 0
	}
	if target == nil {
		if s.tick < mm.NextPickupScan {
			return false
		}
		mm.NextPickupScan = s.tick + monsterPickupScanTicks + domain.Tick(uint64(m.ID)%3)
		best := monsterPickupRadius * monsterPickupRadius
		// AOI neighbourhood bounds candidate work; no scene-wide ground scan.
		s.aoi.AroundPos(m.Pos, func(e *entity.Entity) {
			if !s.monsterCanPickUp(e) {
				return
			}
			d := sqDist(m.Pos, e.Pos)
			if d <= best && (target == nil || d < best || e.ID < target.ID) {
				target, best = e, d
			}
		})
		if target == nil {
			return false
		}
		mm.PickupTarget = target.ID
	}
	if !s.straightPathWalkable(m.Pos, target.Pos) {
		mm.PickupTarget = 0
		return false
	}
	mm.Roaming = false
	if sqDist(m.Pos, target.Pos) > pickUpRange*pickUpRange {
		if !canMove(m) {
			return false
		}
		s.moveEntity(m, ai.StepToward(m.Pos, target.Pos, aiParams(m).MoveSpeed))
		return true
	}
	// Revalidate the authoritative ground map: a player/another monster may have
	// acquired it earlier in this actor turn. No pointer survives outside actor.
	if !s.monsterCanPickUp(target) {
		mm.PickupTarget = 0
		return false
	}
	s.removeLoot(target.ID, event.DespawnPickedUp)
	mm.CarriedLoot = append(mm.CarriedLoot, target)
	mm.PickupTarget = 0
	s.monsterSpeak(m, domain.MonsterScenePick)
	return true
}

func (s *Scene) monsterCanPickUp(e *entity.Entity) bool {
	if e == nil || e.Kind != domain.KindDrop || e.Drop == nil || s.ground[e.ID] != e || s.entities[e.ID] != e {
		return false
	}
	d := e.Drop
	if d.Count <= 0 || d.ExpireAt <= s.tick || (d.Owner != 0 && s.tick < d.OwnerUntil) || d.Stack.Bound || d.Stack.Locked {
		return false
	}
	def, ok := s.items[d.Item]
	// Unknown, non-tradable and quest-only items are deliberately not eligible.
	if !ok || !def.CanTrade || !def.InventoryTabKnown || (def.InventoryTab != 1 && def.InventoryTab != 2) {
		return false
	}
	if !d.Stack.Empty() && (d.Stack.Item != d.Item || d.Stack.Count != d.Count) {
		return false
	}
	if s.monsterPickupQuestItems == nil {
		s.monsterPickupQuestItems = make(map[domain.ItemID]bool)
		for _, q := range s.quests {
			if q.RecoveryItem != 0 {
				s.monsterPickupQuestItems[q.RecoveryItem] = true
			}
			for _, step := range q.Steps {
				for _, goal := range step.Collect {
					if len(goal.Sources) > 0 {
						s.monsterPickupQuestItems[goal.Item] = true
					}
				}
				for _, goal := range step.Give {
					s.monsterPickupQuestItems[goal.Item] = true
				}
			}
		}
	}
	return !s.monsterPickupQuestItems[d.Item]
}

// Release is idempotent. Original IDs are retained unless reused; original TTL
// callbacks remain scheduled; expired carried items are never resurrected.
func (s *Scene) releaseMonsterCarriedLoot(m *entity.Entity) {
	if m == nil || m.Monster == nil {
		return
	}
	carried := m.Monster.CarriedLoot
	m.Monster.CarriedLoot = nil
	m.Monster.PickupTarget = 0
	for _, e := range carried {
		if e == nil || e.Drop == nil || e.Drop.ExpireAt <= s.tick {
			continue
		}
		if s.entities[e.ID] != nil || s.ground[e.ID] != nil {
			// Allocator segments may wrap. Never overwrite an existing entity.
			if s.alloc == nil {
				m.Monster.CarriedLoot = append(m.Monster.CarriedLoot, e)
				continue
			}
			for {
				e.ID = s.alloc.Drop()
				if s.entities[e.ID] == nil && s.ground[e.ID] == nil {
					break
				}
			}
			id := e.ID
			s.timer.after(s.tick, e.Drop.ExpireAt-s.tick, func() {
				if s.entities[id] == e {
					s.removeLoot(id, event.DespawnTimeout)
				}
			})
		}
		e.Pos = m.Pos
		s.entities[e.ID], s.ground[e.ID] = e, e
		s.aoi.Enter(e)
		s.emit(s.spawnEvent(e)) // visibility return, not a freshly rolled reward
	}
}

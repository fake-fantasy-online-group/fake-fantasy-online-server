package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	classSetWarrior   int32          = 5
	classSetSwordsman int32          = 4
	classSetTaunt     domain.SkillID = 10127
	classSetWhirlwind domain.SkillID = 10121
)

type classSetUnruly struct {
	owner   *entity.Entity
	stacks  int32
	expires domain.Tick
}

// Apply the transformation to the final derived values, not affix percentages:
// both the panel and source snapshots consume this same computed bundle.
func (s *Scene) classSetMeleeStats(p *entity.Entity, worn *domain.EquipSet, stats domain.Stats) domain.Stats {
	if s.classSetPiecesInWorn(p, worn, classSetWarrior) < 6 {
		return stats
	}
	stats.MaxHP = clampI32(int64(stats.MaxHP) * 2)
	stats.MinAtk = clampI32(int64(stats.MinAtk) * 160 / 100)
	stats.MaxAtk = clampI32(int64(stats.MaxAtk) * 160 / 100)
	stats.MoveSpeed = clampI32(int64(stats.MoveSpeed) * 120 / 100)
	return stats
}

func (s *Scene) classSetDragonActive(p *entity.Entity) bool {
	return p != nil && p.Alive() && s.classSetPieces(p, classSetWarrior) >= 6
}

// Integer HP costs round down, with a minimum of one when affordable. A living
// actor at one HP keeps that last point; paying a resource is never a damage hit.
func classSetBloodCost(hp int32) int32 {
	if hp <= 1 {
		return 0
	}
	return min(hp-1, max(int32(1), hp/10))
}

func (s *Scene) classSetPaySkillCost(p *entity.Entity, def domain.SkillDef) {
	if s.classSetDragonActive(p) {
		p.HP -= classSetBloodCost(p.HP)
	} else {
		p.MP -= def.MPCost
	}
}

func (s *Scene) classSetMissingHealthDamage(p *entity.Entity, damage int32) int32 {
	if damage <= 0 || p == nil || !p.Alive() || p.MaxHP <= 0 || s.classSetPieces(p, classSetWarrior) < 4 {
		return damage
	}
	missing := max(int64(0), int64(p.MaxHP)-int64(p.HP))
	return clampI32(int64(damage) + int64(damage)*missing/int64(p.MaxHP))
}

func (s *Scene) classSetUnrulyStacks(p *entity.Entity) int32 {
	rt := s.equipmentState()
	if p == nil {
		return 0
	}
	state, ok := rt.unruly[p.ID]
	if !ok {
		return 0
	}
	if state.owner != p || !p.Alive() || state.expires <= s.tick || s.classSetPieces(p, classSetSwordsman) < 4 {
		delete(rt.unruly, p.ID)
		return 0
	}
	return state.stacks
}

// Called once on actual cast release, never per hit or on rejected requests.
// A canceled chant does not grant or consume a stack. Refreshing taunt refreshes
// the whole stack group's thirty-second lifetime.
func (s *Scene) classSetMeleeCastReleased(p *entity.Entity, def *domain.SkillDef) int32 {
	if p == nil || def == nil || !p.Alive() {
		return 10000
	}
	stacks := s.classSetUnrulyStacks(p)
	if def.ID == classSetTaunt && s.classSetPieces(p, classSetSwordsman) >= 4 {
		rt := s.equipmentState()
		if rt.unruly == nil {
			rt.unruly = make(map[domain.EntityID]classSetUnruly)
		}
		rt.unruly[p.ID] = classSetUnruly{owner: p, stacks: min(int32(10), stacks+1), expires: s.tick + domain.Ticks(30000)}
		s.pushStatusSnapshots(p)
	}
	if def.ID == classSetWhirlwind && stacks > 0 && s.classSetPieces(p, classSetSwordsman) >= 6 {
		rt := s.equipmentState()
		state := rt.unruly[p.ID]
		state.stacks--
		if state.stacks == 0 {
			delete(rt.unruly, p.ID)
		} else {
			rt.unruly[p.ID] = state
		}
		s.pushStatusSnapshots(p)
		return 20000
	}
	return 10000
}

// Set multipliers remain independent from the legacy equipment mitigation cap.
func (s *Scene) classSetMeleeIncomingDamage(p *entity.Entity, amount int32) int32 {
	if amount <= 0 {
		return amount
	}
	if s.classSetDragonActive(p) {
		amount = clampI32(int64(amount) * 150 / 100)
	}
	return clampI32(int64(amount) * (100 - int64(s.classSetUnrulyStacks(p))*5) / 100)
}

func (s *Scene) classSetDragonSteal(p *entity.Entity, ev event.DamageDealt) {
	if ev.Amount <= 0 || ev.Flag.Has(event.DamageMiss) || !s.classSetDragonActive(p) {
		return
	}
	amount := clampI32(int64(ev.Amount) * 20 / 100)
	if amount > 0 {
		s.emit(combat.Restore(p, p, amount, ev.SkillID))
		s.emitAttributesIfPlayer(p)
	}
}

func (s *Scene) clearClassSetMelee(id domain.EntityID) {
	if s.equipment != nil {
		delete(s.equipment.unruly, id)
	}
}

// The stable identifier is server-owned; the image, when present, is the
// existing learned taunt skill's icon, never an invented resource identifier.
const classSetUnrulyViewID int32 = 2104004

func (s *Scene) classSetUnrulyPresentation(p *entity.Entity) []statusPresentation {
	stacks := s.classSetUnrulyStacks(p)
	if stacks == 0 {
		return nil
	}
	state := s.equipmentState().unruly[p.ID]
	remain := nonnegativeI32((state.expires - s.tick).Millis())
	icon := int32(0)
	if def, ok := s.skills.Get(classSetTaunt, 1); ok {
		icon = def.Icon
	}
	name := fmt.Sprintf("不羁（%d层）", stacks)
	desc := fmt.Sprintf("受到的伤害减少%d%%。旋风剑可消耗一层强化本次伤害。", stacks*5)
	return []statusPresentation{{name: name, desc: desc,
		icon: event.StatusIconView{Status: classSetUnrulyViewID, Icon: icon, Beneficial: true, RemainMS: remain},
		buff: event.BuffView{SkillID: classSetUnrulyViewID, Icon: icon, Name: name, Desc: desc, Beneficial: true, RemainSec: (remain + 999) / 1000}}}
}

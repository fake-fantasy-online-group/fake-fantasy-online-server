package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) captureClassSetRuntime(p *entity.Entity) entity.ClassSetRuntime {
	var r entity.ClassSetRuntime
	if p == nil || !p.Alive() {
		return r
	}
	if n := s.classSetUnrulyStacks(p); n > 0 {
		r.UnrulyStacks = n
		r.UnrulyFor = s.equipment.unruly[p.ID].expires - s.tick
	}
	if s.equipment == nil {
		return r
	}
	if st := s.equipment.classSetCasters[p.ID]; st != nil {
		if s.classSetPieces(p, 3) >= classSetFirstSkillPieces {
			r.FireFor = classSetRemaining(st.fireUntil, s.tick)
			r.IceFor = classSetRemaining(st.iceUntil, s.tick)
		}
		if s.classSetPieces(p, 2) >= classSetFirstSkillPieces && st.nextMana != 0 {
			r.ManaScheduled = true
			r.ManaReadyIn = classSetRemaining(st.nextMana, s.tick)
		}
	}
	return r
}

func (s *Scene) restoreClassSetRuntime(p *entity.Entity, r entity.ClassSetRuntime) {
	if p == nil || !p.Alive() {
		return
	}
	if r.UnrulyStacks > 0 && r.UnrulyFor > 0 && s.classSetPieces(p, classSetSwordsman) >= classSetFirstSkillPieces {
		rt := s.equipmentState()
		if rt.unruly == nil {
			rt.unruly = make(map[domain.EntityID]classSetUnruly)
		}
		rt.unruly[p.ID] = classSetUnruly{owner: p, stacks: min(int32(10), r.UnrulyStacks), expires: s.tick + r.UnrulyFor}
	}
	if s.classSetPieces(p, 3) >= classSetFirstSkillPieces && (r.FireFor > 0 || r.IceFor > 0) {
		st := s.casterSetState(p)
		if r.FireFor > 0 {
			st.fireUntil = s.tick + r.FireFor
		}
		if r.IceFor > 0 {
			st.iceUntil = s.tick + r.IceFor
		}
	}
	if s.classSetPieces(p, 2) >= classSetFirstSkillPieces && r.ManaScheduled {
		// nextMana=0 is the existing uninitialized sentinel; defer a due tick by
		// one frame only if the destination itself is still at tick zero.
		s.casterSetState(p).nextMana = max(domain.Tick(1), s.tick+r.ManaReadyIn)
	}
}

func classSetRemaining(deadline, now domain.Tick) domain.Tick {
	if deadline <= now {
		return 0
	}
	return deadline - now
}

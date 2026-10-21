package scene

import (
	"math"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// These clocks are scene-local. Removing a set, dying, or leaving a scene clears
// its grants; logging back in cannot accumulate offline mana regeneration.
type classSetCasterState struct {
	fireUntil, iceUntil domain.Tick
	nextMana            domain.Tick
}
type classSetCastArea struct {
	shape      domain.SkillAreaTargeting
	center     domain.Pos
	aimX, aimY float64
}

func (s *Scene) casterSetState(p *entity.Entity) *classSetCasterState {
	rt := s.equipmentState()
	if rt.classSetCasters == nil {
		rt.classSetCasters = make(map[domain.EntityID]*classSetCasterState)
	}
	st := rt.classSetCasters[p.ID]
	if st == nil {
		st = &classSetCasterState{}
		rt.classSetCasters[p.ID] = st
	}
	return st
}

func (s *Scene) stepCasterSets() {
	rt := s.equipmentState()
	for id := range rt.classSetCasters {
		p := s.players[id]
		if p == nil || !p.Alive() {
			delete(rt.classSetCasters, id)
		}
	}
	for _, p := range s.players {
		if !p.Alive() {
			continue
		}
		mage := s.classSetPieces(p, 3) >= 4
		healer := s.classSetPieces(p, 2) >= 4
		st := rt.classSetCasters[p.ID]
		if !mage && !healer {
			delete(rt.classSetCasters, p.ID)
			continue
		}
		if st == nil {
			st = s.casterSetState(p)
		}
		if !mage {
			st.fireUntil, st.iceUntil = 0, 0
		}
		if !healer {
			st.nextMana = 0
			continue
		}
		if st.nextMana == 0 {
			st.nextMana = s.tick + domain.Ticks(3000)
			continue
		}
		if s.tick < st.nextMana {
			continue
		}
		st.nextMana = s.tick + domain.Ticks(3000)
		if s.entityMapLoading(p) {
			continue
		}
		amount := int32(int64(p.MaxMP) * 3 / 100)
		p.MP = min(p.MaxMP, clampI32(int64(p.MP)+int64(amount)))
		s.emitAttributesIfPlayer(p)
	}
}

func (s *Scene) startCasterSetCast(src *entity.Entity, def domain.SkillDef) {
	if s.equipmentState().classSetTriggered != 0 || s.classSetPieces(src, 3) < 4 {
		return
	}
	st := s.casterSetState(src)
	switch def.HurtType {
	case domain.HurtFire:
		st.iceUntil = s.tick + domain.Ticks(3000)
	case domain.HurtIce:
		st.fireUntil = s.tick + domain.Ticks(3000)
	}
}

// Apply after the existing outgoing buckets, not to the spell coefficient:
// +50% elemental damage must not amplify the target's native magic defense.
func (s *Scene) casterSetOutgoingDamage(src *entity.Entity, hurtType int32, amount int32) int32 {
	if amount <= 0 || s.classSetPieces(src, 3) < 4 {
		return amount
	}
	st := s.equipmentState().classSetCasters[src.ID]
	if st == nil {
		return amount
	}
	if hurtType == domain.HurtFire && st.fireUntil > s.tick || hurtType == domain.HurtIce && st.iceUntil > s.tick {
		return clampI32(int64(amount) * 3 / 2)
	}
	return amount
}

// Copy the nominal healing once, BEFORE the recipient's missing-HP cap. This
// helper never calls applySkill, so the copy cannot produce another copy.
func (s *Scene) casterSetCopyHeal(src, dst *entity.Entity, amount int32, skill domain.SkillID) {
	if src == nil || dst == nil || src == dst || !src.Alive() || !dst.Alive() ||
		s.equipmentState().classSetTriggered != 0 || s.classSetPieces(src, 2) < 4 || !sameParty(src, dst) || amount <= 0 {
		return
	}
	s.emit(combat.Restore(src, src, amount, int32(skill)))
	s.emitAttributesIfPlayer(src)
}

// Snapshot the accepted cast's geometry. The opposite payload must not expand
// around every AoE victim, nor lose its center when the primary victim dies.
func (s *Scene) captureCasterSetArea(src *entity.Entity, def domain.SkillDef, target domain.EntityID, at *domain.Pos) *classSetCastArea {
	if s.classSetPieces(src, 2) < 6 {
		return nil
	}
	a := &classSetCastArea{shape: def.Area, center: src.Pos}
	targetPos := src.Pos
	if t := s.entities[target]; t != nil {
		targetPos = t.Pos
	}
	if def.Kind != domain.SkillArea || !def.Area.Valid() {
		a.shape = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: 200}
		a.center = targetPos
	} else if def.Area.Center == domain.SkillAreaCenterTarget {
		a.center = targetPos
		if at != nil {
			a.center = *at
		}
	}
	if a.shape.Shape == domain.SkillAreaRectangle || a.shape.Shape == domain.SkillAreaCone {
		aim := targetPos
		if at != nil {
			aim = *at
		}
		if at == nil && target == 0 {
			a.aimX, a.aimY = skillFacingUnit(src.Dir)
		} else {
			a.aimX, a.aimY = aim.X-src.Pos.X, aim.Y-src.Pos.Y
			length := math.Hypot(a.aimX, a.aimY)
			if length > 0 {
				a.aimX /= length
				a.aimY /= length
			}
		}
	}
	return a
}

func (s *Scene) casterSetPayload(src *entity.Entity, id domain.SkillID) (domain.SkillDef, bool) {
	if s.skills == nil || src.Player == nil || src.Player.Char == nil {
		return domain.SkillDef{}, false
	}
	rank, _ := s.skillLevelForUse(src, id)
	if rank <= 0 {
		return domain.SkillDef{}, false
	}
	def, ok := s.skills.Get(id, rank)
	if !ok || !def.Usable() {
		return domain.SkillDef{}, false
	}
	s.applyResolvedSkillModifiers(src, &def, src.Player.Char.Skills, s.rawSkillLevels(src))
	// Generated payloads are immediate, single-target and free. Use the resolved
	// learned spell's potency, but not equipment variants that create more casts.
	def.EquipmentVariant = nil
	def.Kind, def.Area, def.Radius = domain.SkillSingle, domain.SkillAreaTargeting{}, 0
	def.MPCost, def.CooldownMS, def.PrepareMS = 0, 0, 0
	return def, true
}

func (s *Scene) emitCasterSetPayload(src, dst *entity.Entity, def domain.SkillDef) {
	if !src.Alive() || !dst.Alive() || !s.validTarget(src, dst, def) {
		return
	}
	rt := s.equipmentState()
	rt.classSetTriggered++
	defer func() { rt.classSetTriggered-- }()
	s.emit(event.SkillCastChanged{Caster: src.ID, Skill: def.ID, Phase: event.SkillCastRelease, Target: dst.ID})
	s.applySkill(src, dst, def, 100, false)
}

func (s *Scene) finishCasterSetCast(src *entity.Entity, c pendingCast, hits []domain.EntityID) {
	if src == nil || !src.Alive() || s.equipmentState().classSetTriggered != 0 {
		return
	}
	if s.classSetPieces(src, 3) >= 6 && c.def.HasDamageEffect() {
		var id domain.SkillID
		switch c.def.HurtType {
		case domain.HurtFire:
			id = 10401
		case domain.HurtIce:
			id = 10409
		}
		if id != 0 {
			if def, ok := s.casterSetPayload(src, id); ok {
				seen := make(map[domain.EntityID]bool, len(hits))
				for _, tid := range hits {
					if seen[tid] {
						continue
					}
					seen[tid] = true
					if dst := s.entities[tid]; dst != nil {
						s.emitCasterSetPayload(src, dst, def)
					}
				}
			}
		}
	}
	if s.classSetPieces(src, 2) < 6 || c.classSetArea == nil {
		return
	}
	damage := c.def.HasDamageEffect()
	heal := false
	for _, e := range c.def.DirectEffects() {
		if e.Kind == domain.EffectHeal || e.Kind == domain.EffectRestoreHP {
			heal = true
		}
	}
	var id domain.SkillID
	if damage {
		id = 10301
	} else if heal {
		id = 10319
	} else {
		return
	}
	def, ok := s.casterSetPayload(src, id)
	if !ok {
		return
	}
	a := c.classSetArea
	var ids []domain.EntityID
	s.aoi.AroundRadius(a.center, skillAreaSearchRadius(a.shape), func(dst *entity.Entity) {
		if dst.Alive() && s.validTarget(src, dst, def) && skillAreaContains(a.shape, a.center, a.aimX, a.aimY, dst.Pos) {
			ids = append(ids, dst.ID)
		}
	})
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, tid := range ids {
		if dst := s.entities[tid]; dst != nil {
			s.emitCasterSetPayload(src, dst, def)
		}
	}
}

func (s *Scene) pruneCasterSetState(p *entity.Entity, worn *domain.EquipSet) {
	if s.equipment == nil || p == nil {
		return
	}
	st := s.equipment.classSetCasters[p.ID]
	if st == nil {
		return
	}
	mage := s.classSetPiecesInWorn(p, worn, 3) >= 4
	healer := s.classSetPiecesInWorn(p, worn, 2) >= 4
	if !mage && !healer {
		delete(s.equipment.classSetCasters, p.ID)
		return
	}
	if !mage {
		st.fireUntil, st.iceUntil = 0, 0
	}
	if !healer {
		st.nextMana = 0
	}
}

package scene

import (
	"math"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// These clocks are scene-local and rebased through ClassSetRuntime on map transfer.
// Removing a set, dying, or logging out clears grants; no offline mana accumulates.
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
			if p != nil {
				s.pushStatusSnapshots(p)
			}
		}
	}
	for _, p := range s.players {
		if !p.Alive() {
			continue
		}
		mage := s.classSetPieces(p, 3) >= classSetFirstSkillPieces
		healer := s.classSetPieces(p, 2) >= classSetFirstSkillPieces
		st := rt.classSetCasters[p.ID]
		if !mage && !healer {
			delete(rt.classSetCasters, p.ID)
			if st != nil {
				s.pushStatusSnapshots(p)
			}
			continue
		}
		if st == nil {
			st = s.casterSetState(p)
		}
		oldFire, oldIce := st.fireUntil, st.iceUntil
		if !mage || st.fireUntil <= s.tick {
			st.fireUntil = 0
		}
		if !mage || st.iceUntil <= s.tick {
			st.iceUntil = 0
		}
		if oldFire != st.fireUntil || oldIce != st.iceUntil {
			s.pushStatusSnapshots(p)
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
		s.dispatchEquipmentEffect(p, domain.RuntimeBuiltinEquipmentEffect("set:periodic-mana", true), func() {
			amount := int32(int64(p.MaxMP) * 3 / 100)
			before := p.MP
			p.MP = min(p.MaxMP, clampI32(int64(p.MP)+int64(amount)))
			s.legendaryCasterManaRestored(p, p, domain.SkillDef{}, p.MP-before, max(int32(0), amount-(p.MP-before)))
			s.emitAttributesIfPlayer(p)
		})
	}
}

func (s *Scene) startCasterSetCastPayload(src *entity.Entity, def domain.SkillDef) {
	if s.classSetPieces(src, 3) < classSetFirstSkillPieces {
		return
	}
	st := s.casterSetState(src)
	switch def.HurtType {
	case domain.HurtFire:
		st.iceUntil = s.tick + domain.Ticks(3000)
	case domain.HurtIce:
		st.fireUntil = s.tick + domain.Ticks(3000)
	default:
		return
	}
	s.pushStatusSnapshots(src)
}

// Apply after the existing outgoing buckets, not to the spell coefficient:
// +50% elemental damage must not amplify the target's native magic defense.
func (s *Scene) casterSetOutgoingDamagePayload(src *entity.Entity, hurtType int32, amount int32) int32 {
	if amount <= 0 || s.classSetPieces(src, 3) < classSetFirstSkillPieces {
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
		s.combatEffectActive(src.ID, "set:healing-copy") || s.classSetPieces(src, 2) < classSetFirstSkillPieces || !sameParty(src, dst) || amount <= 0 {
		return
	}
	s.dispatchEquipmentEffect(src, domain.RuntimeBuiltinEquipmentEffect("set:healing-copy", true), func() {
		s.restoreHealthWithReactions(src, src, amount, skill)
	})
}

// Snapshot the accepted cast's geometry. The opposite payload must not expand
// around every AoE victim, nor lose its center when the primary victim dies.
func (s *Scene) captureCasterSetArea(src *entity.Entity, def domain.SkillDef, target domain.EntityID, at *domain.Pos) *classSetCastArea {
	if s.classSetPieces(src, 2) < classSetSecondSkillPieces {
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
	s.legendaryPrepareSkill(src, &def)
	def.Kind, def.Area, def.Radius = domain.SkillSingle, domain.SkillAreaTargeting{}, 0
	def.MPCost, def.CooldownMS, def.PrepareMS = 0, 0, 0
	return def, true
}

func (s *Scene) emitCasterSetPayload(src, dst *entity.Entity, def domain.SkillDef) {
	if !src.Alive() || !dst.Alive() || !s.validTarget(src, dst, def) {
		return
	}
	s.dispatchEquipmentEffect(src, domain.RuntimeBuiltinEquipmentEffect("set:opposite-spell", true), func() {
		rt := s.equipmentState()
		rt.classSetTriggered++
		defer func() { rt.classSetTriggered-- }()
		s.emit(event.SkillCastChanged{Caster: src.ID, Skill: def.ID, Phase: event.SkillCastRelease, Target: dst.ID})
		s.applySkill(src, dst, def, 0, false)
	})
}

func (s *Scene) finishCasterSetCast(src *entity.Entity, c pendingCast, hits []domain.EntityID) {
	if src == nil || !src.Alive() || s.combatEffectActive(src.ID, "set:opposite-spell") {
		return
	}
	if s.classSetPieces(src, 3) >= classSetSecondSkillPieces && c.def.HasDamageEffect() {
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
	if s.classSetPieces(src, 2) < classSetSecondSkillPieces || c.classSetArea == nil {
		return
	}
	damage := c.def.HasDamageEffect()
	heal := false
	if root := s.equipmentState().root; root != nil && root.src == src.ID {
		heal = root.classSetHealingStatus
	}
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
	mage := s.classSetPiecesInWorn(p, worn, 3) >= classSetFirstSkillPieces
	healer := s.classSetPiecesInWorn(p, worn, 2) >= classSetFirstSkillPieces
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

// Classification follows a successful, source-owned application rather than a
// spell ID or its description. A pure stat buff and a resisted/blocked HoT do
// not become healing casts. Only the original cast root is marked; status ticks
// never call finishCasterSetCast and cannot emit another reciprocal bolt.
func (s *Scene) recordCasterSetHealingStatus(src, dst *entity.Entity, status domain.StatusDef) {
	rt := s.equipmentState()
	if rt.classSetTriggered != 0 || rt.root == nil || rt.root.src != src.ID || !status.Periodic() || status.Harmful() || s.classSetPieces(src, 2) < classSetSecondSkillPieces {
		return
	}
	var hp int64
	for _, a := range status.Affixes {
		if a.Attr == domain.AttrHPRegen {
			hp += int64(resourceDelta(a.Value, a.Mode, dst.MaxHP))
		}
	}
	for _, a := range status.HPChanges {
		hp += int64(resourceDelta(a.Value, a.Mode, dst.MaxHP))
	}
	if hp > 0 {
		rt.root.classSetHealingStatus = true
	}
}

// Dedicated server-owned UI IDs reuse the original fireball/ice-arrow icons.
// They have no world effect: this projects the existing damage clocks only.
const (
	classSetFireViewID int32 = 2103004
	classSetIceViewID  int32 = 2103005
)

func (s *Scene) classSetCasterPresentation(p *entity.Entity) []statusPresentation {
	if p == nil || !p.Alive() || s.classSetPieces(p, 3) < classSetFirstSkillPieces || s.equipment == nil {
		return nil
	}
	st := s.equipment.classSetCasters[p.ID]
	if st == nil {
		return nil
	}
	var out []statusPresentation
	for _, v := range []struct {
		id         int32
		skill      domain.SkillID
		until      domain.Tick
		name, desc string
	}{
		{classSetFireViewID, 10401, st.fireUntil, "火系增伤", "火系伤害提高50%。"},
		{classSetIceViewID, 10409, st.iceUntil, "冰系增伤", "冰系伤害提高50%。"},
	} {
		if v.until <= s.tick {
			continue
		}
		remain := nonnegativeI32((v.until - s.tick).Millis())
		icon := int32(0)
		if def, ok := s.skills.Get(v.skill, 1); ok {
			icon = def.Icon
		}
		out = append(out, statusPresentation{name: v.name, desc: v.desc,
			icon: event.StatusIconView{Status: v.id, Icon: icon, Beneficial: true, RemainMS: remain},
			buff: event.BuffView{SkillID: v.id, Icon: icon, Name: v.name, Desc: v.desc, Beneficial: true, RemainSec: (remain + 999) / 1000}})
	}
	return out
}

func (s *Scene) casterSetOutgoingDamage(src *entity.Entity, hurtType int32, amount int32) int32 {
	out := amount
	s.dispatchEquipmentEffect(src, domain.RuntimeBuiltinEquipmentEffect("set:element-window", false), func() { out = s.casterSetOutgoingDamagePayload(src, hurtType, amount) })
	return out
}

func (s *Scene) startCasterSetCast(src *entity.Entity, def domain.SkillDef) {
	s.dispatchEquipmentEffect(src, domain.RuntimeBuiltinEquipmentEffect("set:element-window", false), func() { s.startCasterSetCastPayload(src, def) })
}

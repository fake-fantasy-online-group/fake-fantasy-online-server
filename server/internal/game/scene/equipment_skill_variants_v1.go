package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
	"time"
)

func (s *Scene) applyEquipmentSkillVariant(p *entity.Entity, def *domain.SkillDef, hard domain.Learned) {
	if p == nil || p.Player == nil || !p.Player.EndgameSkillVariants || hard[def.ID] <= 0 {
		return
	}
	var candidates []domain.ActiveEquipmentEffect
	for _, a := range s.equipmentEffects(p).effects {
		v, ok := domain.EquipmentSkillVariantByID(a.Effect.VariantID)
		if a.Effect.Kind == "skill_variant" && ok && v.SkillID == def.ID && v.Profession == def.Prof {
			candidates = append(candidates, a)
		}
	}
	// Stable source choice: duplicates never multiply. First release only assigns
	// one reviewed primary form per skill; future conflicting IDs fail closed.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].SourceUID < candidates[j].SourceUID })
	if len(candidates) == 0 {
		return
	}
	a := candidates[0]
	for _, b := range candidates[1:] {
		if b.Effect.VariantID != a.Effect.VariantID {
			return
		}
	}
	v, _ := domain.EquipmentSkillVariantByID(a.Effect.VariantID)
	v.SourceUID, v.Item = a.SourceUID, a.Item
	v.EquipmentRevision = p.Player.Worn.EffectsRevision()
	def.EquipmentVariant = &v
	switch v.ID {
	case "sword_gale_fan":
		def.Kind = domain.SkillArea
		def.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 90}
		def.Radius = 100
		def.GroundEffect = false
	case "warrior_piercing_lance":
		def.Kind = domain.SkillArea
		def.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Length: 240, Width: 48}
		def.Radius = 0
		def.GroundEffect = false
		scaleVariantDamage(def, 80)
	case "warlock_flash_fire":
		def.PrepareMS = 0
		def.CooldownMS = max(def.CooldownMS, 1200)
	}
}
func scaleVariantDamage(def *domain.SkillDef, percent int64) {
	def.Effects = append([]domain.SkillEffect(nil), def.DirectEffects()...)
	for i := range def.Effects {
		if def.Effects[i].Kind == domain.EffectDamage {
			domain.ScaleSkillDamageFraction(&def.Effects[i], percent, 100)
			def.Effects[i].DamageFlat = domain.SaturateInt32(int64(def.Effects[i].DamageFlat) * percent / 100)
		}
	}
}
func variantTargetDef(def domain.SkillDef, index int) domain.SkillDef {
	if def.EquipmentVariant != nil && def.EquipmentVariant.ID == "sword_gale_fan" && index > 0 {
		scaleVariantDamage(&def, 60)
		def.Statuses = nil
		effects := def.Effects[:0]
		for _, effect := range def.Effects {
			if effect.Kind != domain.EffectKnockback {
				effects = append(effects, effect)
			}
		}
		def.Effects = effects
	}
	return def
}
func (s *Scene) limitVariantTargets(caster *entity.Entity, def domain.SkillDef, ids []domain.EntityID, primary domain.EntityID) []domain.EntityID {
	if def.EquipmentVariant == nil {
		return ids
	}
	// Prefer the explicit legal primary, then nearest entities and stable IDs.
	sort.SliceStable(ids, func(i, j int) bool {
		if ids[i] == primary {
			return true
		}
		if ids[j] == primary {
			return false
		}
		a, b := s.entities[ids[i]], s.entities[ids[j]]
		da, db := sqDist(caster.Pos, a.Pos), sqDist(caster.Pos, b.Pos)
		if da != db {
			return da < db
		}
		return ids[i] < ids[j]
	})
	n := int(def.EquipmentVariant.MaxTargets)
	if n > 0 && len(ids) > n {
		ids = ids[:n]
	}
	return ids
}

// Stored delayed healing is finite, target-bounded, non-recursive and cancelled
// on any equipment revision. Re-equipping the same UID cannot revive a deposit.
type variantMend struct {
	source, target domain.EntityID
	uid            int64
	revision       uint64
	amount         int32
	due            time.Time
}

func (s *Scene) startVariantMend(src, dst *entity.Entity, def domain.SkillDef, amount int32) int32 {
	rt := s.equipmentState()
	if def.ID == 10301 {
		delete(rt.variantMends, dst.ID)
	}
	if def.EquipmentVariant == nil || def.EquipmentVariant.ID != "healer_guarded_mend" {
		return amount
	}
	if rt.variantMends == nil {
		rt.variantMends = map[domain.EntityID]variantMend{}
	}
	delete(rt.variantMends, dst.ID)
	budget := min(max(int32(0), dst.MaxHP-dst.HP), amount)
	if budget > 0 {
		rt.variantMends[dst.ID] = variantMend{src.ID, dst.ID, def.EquipmentVariant.SourceUID, def.EquipmentVariant.EquipmentRevision, budget / 2, s.now().Add(time.Second)}
	}
	return int32(int64(budget) * 70 / 100)
}
func (s *Scene) stepVariantMends(now time.Time) {
	rt := s.equipmentState()
	for id, m := range rt.variantMends {
		src, dst := s.entities[m.source], s.entities[m.target]
		valid := src != nil && src.Player != nil && src.Player.EndgameSkillVariants && s.equipmentCombatReady(src) && src.Alive() && dst != nil && dst.Alive() && src.Player.Worn.EffectsRevision() == m.revision
		found := false
		if valid {
			for _, a := range s.equipmentEffects(src).effects {
				if a.SourceUID == m.uid && a.Effect.VariantID == "healer_guarded_mend" {
					found = true
					break
				}
			}
		}
		if !valid || !found {
			delete(rt.variantMends, id)
			continue
		}
		if now.Before(m.due) {
			continue
		}
		delete(rt.variantMends, id)
		def, ok := s.skills.Get(10301, 1)
		if !ok || !s.validTarget(src, dst, def) || !s.equipmentCombatReady(dst) {
			continue
		}
		s.emit(combat.Restore(src, dst, m.amount, 10301))
		s.emitAttributesIfPlayer(dst)
	}
}
func variantDisplayLines(def domain.SkillDef, lines []string) []string {
	if v := def.EquipmentVariant; v != nil {
		return append([]string{fmt.Sprintf("装备形态·%s（来源物品%d）：%s", v.Name, v.Item, v.Description)}, lines...)
	}
	return lines
}

// Reuse the already paid trap payload. Its own placement snapshot is retained;
// the projectile never supplies a new rank or a second proc root to the bomb.
func (s *Scene) detonateVariantBomb(src, dst *entity.Entity, def domain.SkillDef, impact ...domain.Pos) {
	if def.EquipmentVariant == nil || def.EquipmentVariant.ID != "assassin_bomb_fuse" || src == nil || dst == nil {
		return
	}
	positiveHit := false
	if root := s.equipmentState().root; root != nil {
		for _, hit := range root.hits {
			if hit.dst == dst.ID && hit.ev.Amount > 0 {
				positiveHit = true
				break
			}
		}
	}
	if !positiveHit {
		return
	}
	at := dst.Pos
	if len(impact) > 0 {
		at = impact[0]
	}
	var chosen *entity.Entity
	best := float64(160 * 160)
	for _, t := range s.traps {
		if t == nil || t.Trap == nil || t.Trap.Owner != src.ID || t.Trap.Def.ID != 10213 {
			continue
		}
		d := sqDist(t.Pos, at)
		if d > best {
			continue
		}
		if chosen == nil || d < best || t.ID < chosen.ID {
			chosen, best = t, d
		}
	}
	if chosen == nil {
		return
	}
	if chosen.Trap.Def.FormulaVersion == domain.SkillFormulaVersion {
		current, ok := s.lesserPersistentSkill(src, chosen.Trap.Def)
		if !ok {
			s.removeTrap(chosen.ID)
			return
		}
		chosen.Trap.Def = current
		for _, fx := range current.DirectEffects() {
			if fx.Kind == domain.EffectPlaceTrap {
				chosen.Trap.Radius = min(chosen.Trap.Radius, fx.TrapRadius)
			}
		}
	}
	chosen.Trap.Radius = max(chosen.Trap.Radius, 160)
	targets := s.trapTargets(chosen)
	if len(targets) > 6 {
		targets = targets[:6]
	}
	// Consume before settlement. Trap damage is marked and cannot proc equipment.
	s.removeTrap(chosen.ID)
	rt := s.equipmentState()
	previous := rt.root
	offense := chosen.Trap.EquipmentOffense
	currentOffense := s.equipmentOffense(src)
	for i := range offense.Elements {
		offense.Elements[i].DamageBP = min(offense.Elements[i].DamageBP, currentOffense.Elements[i].DamageBP)
		offense.Elements[i].PierceBP = min(offense.Elements[i].PierceBP, currentOffense.Elements[i].PierceBP)
	}
	rt.root = &equipmentRoot{source: chosen.Trap.SourceOffense, currentSource: s.captureCombatSourceV5(src, entityInvisible(src)), src: src.ID, offense: offense, attackStats: lesserTrapAttackStats(chosen.Trap.AttackStats, src.Stats), attackSnapshot: true}
	defer func() { rt.root = previous }()
	payload := trapPayload(chosen.Trap.Def)
	for _, id := range targets {
		target := s.entities[id]
		if target != nil && target.Alive() {
			s.applySkill(src, target, payload, chosen.Trap.StatusDurationPct, true)
		}
	}
}

// Display dimensions are taken after compatible expansion, never copied from
// the unmodified catalog prose when the actual accepted geometry is larger.
func finalizeEquipmentSkillVariant(def *domain.SkillDef) {
	if def.EquipmentVariant == nil {
		return
	}
	v := *def.EquipmentVariant
	switch v.ID {
	case "sword_gale_fan":
		v.Description = fmt.Sprintf("罡气剑改为%d度、半径%d的近身扇面，最多4敌；首目标全额伤害与震退，其余60%%伤害且不震退。", def.Area.Angle, def.Area.Radius)
	case "warrior_piercing_lance":
		v.Description = fmt.Sprintf("飞翔之枪改为向前%d距离、%d宽的贯穿枪线，最多4敌，每敌80%%伤害。", def.Area.Length, def.Area.Width)
	}
	def.EquipmentVariant = &v
}

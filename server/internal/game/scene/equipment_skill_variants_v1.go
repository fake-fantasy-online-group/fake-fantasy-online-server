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
		v, ok := s.endgameCatalog.Variant(a.Effect.VariantID)
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
	v, _ := s.endgameCatalog.Variant(a.Effect.VariantID)
	v.SourceUID, v.Item = a.SourceUID, a.Item
	v.EquipmentRevision = p.Player.Worn.EffectsRevision()
	applyEquipmentSkillVariantDefinition(def, v)
}

func applyEquipmentSkillVariantDefinition(def *domain.SkillDef, v domain.EquipmentSkillVariant) {
	def.EquipmentVariant = &v
	switch v.Behavior {
	case domain.VariantAreaFan, domain.VariantAreaLine:
		def.Kind = domain.SkillArea
		def.Area = v.Params.Area
		def.Radius = v.Params.Area.Radius
		def.GroundEffect = false
		scaleVariantDamage(def, int64(v.Params.DamagePercent))
	case domain.VariantInstantCast:
		def.PrepareMS = 0
		def.CooldownMS = max(def.CooldownMS, v.Params.MinCooldownMS)
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
	v := def.EquipmentVariant
	if v == nil || index <= 0 || (v.Behavior != domain.VariantAreaFan && v.Behavior != domain.VariantAreaLine) {
		return def
	}
	scaleVariantDamage(&def, int64(v.Params.SecondaryDamagePercent))
	if v.Params.SecondaryNoStatuses {
		def.Statuses = nil
	}
	if v.Params.SecondaryNoKnockback {
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
	return ids
}

// Stored delayed healing is finite, target-bounded, non-recursive and cancelled
// on any equipment revision. Re-equipping the same UID cannot revive a deposit.
type variantMend struct {
	source, target domain.EntityID
	uid            int64
	revision       uint64
	amount         int32
	skill          domain.SkillID
	variantID      string
	due            time.Time
}

func (s *Scene) startVariantMend(src, dst *entity.Entity, def domain.SkillDef, amount int32) int32 {
	if s.legendaryCasterSecondaryHeal(src, dst, def.ID) {
		return amount
	}
	rt := s.equipmentState()
	if old, ok := rt.variantMends[dst.ID]; ok && old.skill == def.ID {
		delete(rt.variantMends, dst.ID)
	}
	if def.EquipmentVariant == nil || def.EquipmentVariant.Behavior != domain.VariantDelayedHeal {
		return amount
	}
	if rt.variantMends == nil {
		rt.variantMends = map[domain.EntityID]variantMend{}
	}
	delete(rt.variantMends, dst.ID)
	budget := min(max(int32(0), dst.MaxHP-dst.HP), amount)
	if budget > 0 {
		v := def.EquipmentVariant
		rt.variantMends[dst.ID] = variantMend{source: src.ID, target: dst.ID, uid: v.SourceUID, revision: v.EquipmentRevision,
			amount: domain.SaturateInt32(int64(budget) * int64(v.Params.DelayedHealPercent) / 100), skill: def.ID, variantID: v.ID,
			due: s.now().Add(time.Duration(v.Params.DelayMS) * time.Millisecond)}
	}
	return int32(int64(budget) * int64(def.EquipmentVariant.Params.ImmediateHealPercent) / 100)
}
func (s *Scene) stepVariantMends(now time.Time) {
	rt := s.equipmentState()
	for id, m := range rt.variantMends {
		src, dst := s.entities[m.source], s.entities[m.target]
		valid := src != nil && src.Player != nil && src.Player.EndgameSkillVariants && s.equipmentCombatReady(src) && src.Alive() && dst != nil && dst.Alive() && src.Player.Worn.EffectsRevision() == m.revision
		found := false
		if valid {
			for _, a := range s.equipmentEffects(src).effects {
				if a.SourceUID == m.uid && a.Effect.VariantID == m.variantID {
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
		def, ok := s.skills.Get(m.skill, 1)
		if !ok || !s.validTarget(src, dst, def) || !s.equipmentCombatReady(dst) {
			continue
		}
		s.emit(combat.Restore(src, dst, m.amount, int32(m.skill)))
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
	if def.EquipmentVariant == nil || def.EquipmentVariant.Behavior != domain.VariantDetonateOwnedTrap || src == nil || dst == nil {
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
	params := def.EquipmentVariant.Params
	best := float64(params.TrapSearchRadius) * float64(params.TrapSearchRadius)
	for _, t := range s.traps {
		if t == nil || t.Trap == nil || t.Trap.Owner != src.ID || t.Trap.Def.ID != params.TrapSkillID {
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
	chosen.Trap.Radius = max(chosen.Trap.Radius, params.TrapMinRadius)
	targets := s.trapTargets(chosen)
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
	switch v.Behavior {
	case domain.VariantAreaFan:
		primary := fmt.Sprintf("%d%%伤害", v.Params.DamagePercent)
		if v.Params.DamagePercent == 100 {
			primary = "全额伤害"
		}
		v.Description = fmt.Sprintf("%s改为%d度、半径%d的近身扇面；首目标%s，其余%g%%伤害", def.Name, def.Area.Angle, def.Area.Radius, primary, float64(v.Params.DamagePercent)*float64(v.Params.SecondaryDamagePercent)/100)
		if v.Params.SecondaryNoStatuses {
			v.Description += "且不附加状态"
		}
		if v.Params.SecondaryNoKnockback {
			v.Description += "且不震退"
		}
		v.Description += "。"
	case domain.VariantAreaLine:
		v.Description = fmt.Sprintf("%s改为向前%d距离、%d宽的贯穿枪线，每敌%d%%伤害。", def.Name, def.Area.Length, def.Area.Width, v.Params.DamagePercent)
	}
	def.EquipmentVariant = &v
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func uniquePercent(value, maximum int32) int32 {
	if maximum <= 0 {
		return 0
	}
	return int32(max(int64(0), min(int64(100), int64(value)*100/int64(maximum))))
}
func (s *Scene) uniqueTraits(p *entity.Entity) []domain.UniqueTrait {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return nil
	}
	return s.equipmentEffects(p).traits
}
func uniqueMonsterElite(p *entity.Entity) bool {
	return p != nil && p.Monster != nil && (p.Monster.Kind == domain.MonsterElite || p.Monster.Kind == domain.MonsterBoss)
}
func uniqueStrikeContext(src, dst *entity.Entity, blow combat.Blow, ev event.DamageDealt, beforeHP int32) domain.UniqueContext {
	return domain.UniqueContext{SkillID: domain.SkillID(blow.SkillID), HPPercent: uniquePercent(src.HP, src.MaxHP), MPPercent: uniquePercent(src.MP, src.MaxMP),
		TargetHPPercent: uniquePercent(beforeHP, dst.MaxHP), Magic: ev.Flag.Has(event.DamageMagic), Critical: ev.Flag.Has(event.DamageCrit),
		TargetElite: uniqueMonsterElite(dst), Invisible: blow.StartedInvisible || entityInvisible(src), Trap: blow.Trap,
		Area: blow.Area || blow.Trap, Skill: blow.SkillID != 0, HurtType: blow.HurtType}
}
func (s *Scene) uniqueHealing(src, dst *entity.Entity, amount int32, def domain.SkillDef) int32 {
	source, current := s.combatSourceV5(src)
	x := sourceUniqueEffectsV5(source, current, domain.UniqueContext{SkillID: def.ID, HPPercent: uniquePercent(src.HP, src.MaxHP),
		TargetHPPercent: uniquePercent(dst.HP, dst.MaxHP), Healing: true, Ally: src.ID != dst.ID, Skill: true})
	return domain.ApplyUniqueBP(amount, x.HealingBP)
}
func (s *Scene) applyUniqueSkillModifiers(p *entity.Entity, def *domain.SkillDef) {
	if p == nil || def == nil {
		return
	}
	c := domain.UniqueContext{SkillID: def.ID, HPPercent: uniquePercent(p.HP, p.MaxHP), MPPercent: uniquePercent(p.MP, p.MaxMP), Skill: true,
		Magic: def.HurtType != domain.HurtNone && def.HurtType != domain.HurtPhysical, Area: def.Area.Valid() || def.Radius > 0}
	for _, e := range def.DirectEffects() {
		c.Healing = c.Healing || e.Kind == domain.EffectHeal || e.Kind == domain.EffectRestoreHP
		c.Trap = c.Trap || e.Kind == domain.EffectPlaceTrap
	}
	x := domain.EvaluateUniqueTraits(s.uniqueTraits(p), c)
	def.MPCost = domain.ApplyUniqueBP(def.MPCost, -x.ManaCostReductionBP)
	def.PrepareMS = domain.ApplyUniqueBP(def.PrepareMS, -x.CastReductionBP)
	def.Radius = domain.ApplyUniqueBP(def.Radius, x.AreaExpansionBP)
	def.Area.Radius = domain.ApplyUniqueBP(def.Area.Radius, x.AreaExpansionBP)
	def.Area.Length = domain.ApplyUniqueBP(def.Area.Length, x.AreaExpansionBP)
	def.Area.Width = domain.ApplyUniqueBP(def.Area.Width, x.AreaExpansionBP)
	if x.TrapExpansionBP > 0 {
		def.Effect.TrapRadius = domain.ApplyUniqueBP(def.Effect.TrapRadius, x.TrapExpansionBP)
		def.Effects = append([]domain.SkillEffect(nil), def.Effects...)
		for i := range def.Effects {
			def.Effects[i].TrapRadius = domain.ApplyUniqueBP(def.Effects[i].TrapRadius, x.TrapExpansionBP)
		}
	}
}

// Called only at the positive, resolved fatal-hit boundary. Reflected/status
// damage and the later onDeath lifecycle do not recurse into these powers.
func (s *Scene) afterUniqueStrike(src, dst *entity.Entity, ev event.DamageDealt) {
	if src == nil || dst == nil || src.Player == nil || !src.Alive() || dst.Kind != domain.KindMonster || ev.Amount <= 0 || !ev.Flag.Has(event.DamageFatal) || ev.Flag.Has(event.DamageMiss) {
		return
	}
	x := domain.EvaluateUniqueTraits(s.uniqueTraits(src), domain.UniqueContext{})
	oldHP, oldMP := src.HP, src.MP
	src.HP = int32(min(int64(src.MaxHP), int64(src.HP)+int64(src.MaxHP)*int64(x.KillHealBP)/10000))
	src.MP = int32(min(int64(src.MaxMP), int64(src.MP)+int64(src.MaxMP)*int64(x.KillManaBP)/10000))
	if src.HP != oldHP || src.MP != oldMP {
		s.emitAttributesIfPlayer(src)
	}
}

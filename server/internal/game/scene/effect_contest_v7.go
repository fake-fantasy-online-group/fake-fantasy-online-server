package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// sourceEffectHitBP uses the accepted combat root rather than gear at impact.
// Persistent roots (traps) provide both cast and current snapshots and take the
// lesser rating, so neither casting naked nor removing gear improves the payload.
func (s *Scene) sourceEffectHitBP(source *entity.Entity) int32 {
	if source == nil {
		return 0
	}
	cast, current := s.combatSourceV5(source)
	hit := cast.Stats.EffectHitBP
	if current != nil {
		hit = min(hit, current.Stats.EffectHitBP)
	}
	return hit
}

func (s *Scene) rollEffectProbability(chance float64) bool {
	return chance >= 1 || chance > 0 && s.rng.Float64() < chance
}

func (s *Scene) statusEffectLands(target *entity.Entity, def domain.StatusDef, sourceID domain.EntityID) bool {
	base := domain.EffectiveStatusChanceBP(def.ChanceBP, def.ChanceExplicit)
	if !def.Harmful() {
		return s.rollEffectProbability(domain.EffectSuccessProbability(base, 0, 0))
	}
	hit := int64(def.EffectHitBP) + int64(s.sourceEffectHitBP(s.entities[sourceID]))
	if def.SourceEffectHitKnown {
		hit = int64(def.EffectHitBP) + int64(def.SourceEffectHitBP)
	}
	resist := (int64(target.Stats.StatusResist) + int64(s.passiveStatusResistPct(target, def.ID))) * 100
	chance := domain.EffectSuccessProbability(base, hit, resist)
	if chance >= 1 {
		return true
	}
	if chance <= 0 {
		return false
	}
	draw := s.rng.Float64()
	if draw < chance {
		return true
	}
	// Classify using the same random draw: success at zero resistance but failure
	// with resistance is a resist; a base-chance miss is not a resisted effect.
	// The gameplay result is one draw regardless of the notification outcome.
	if resist > 0 && draw < domain.EffectSuccessProbability(base, hit, 0) {
		s.emitTo(target.ID, event.StatusResisted{Who: target.ID, Status: int32(def.ID)})
	}
	return false
}

func (s *Scene) instantDeathLands(source, target *entity.Entity, skill domain.SkillID, effect domain.SkillEffect) bool {
	if skill != 10210 {
		return effect.ChanceBP >= 10000 || int32(s.rng.Intn(10000)) < effect.ChanceBP
	}
	// Assassination is deliberately the only direct-effect exception. Death has
	// no status ID; use general resistance without borrowing a control resist.
	return s.rollEffectProbability(domain.EffectSuccessProbability(effect.ChanceBP,
		int64(s.sourceEffectHitBP(source))+int64(effect.EffectHitBP), int64(target.Stats.StatusResist)*100))
}

func (s *Scene) resolveSkillStatusDef(skill domain.SkillDef, app domain.StatusApplication, durationBonus int32) (domain.StatusDef, bool) {
	if app.SourceSkill == 0 {
		app.SourceSkill = skill.ID
	}
	def, ok := s.statuses.ResolveSkillApplication(app)
	if !ok {
		return domain.StatusDef{}, false
	}
	if app.DurationSec > 0 && app.ProgressionRanks <= 0 {
		def.DurationSec = app.DurationSec
	}
	def.ChanceBP, def.ChanceExplicit = app.ChanceBP, app.ChanceExplicit
	def.EffectHitBP = domain.SaturateInt32(int64(def.EffectHitBP) + int64(app.EffectHitBP))
	def.Affixes = overrideStatusAffixes(def.Affixes, app.ExtraAffixes)
	applyProgressionStatus(app, &def)
	stampSkillStatusSnapshot(&def, skill)
	def.StealthDamagePct = app.StealthDamagePct
	def.PhysicalDamageTakenPct = domain.SaturateInt32(int64(def.PhysicalDamageTakenPct) + int64(app.PhysicalDamageTakenPct))
	if app.Description != "" {
		def.Desc = app.Description
	}
	def.InstantDeathImmune = app.InstantDeathImmune
	def.BlockHarmful = def.BlockHarmful || app.BlockHarmful
	if app.Control != domain.ControlNone {
		def.Control = app.Control
	}
	if durationBonus > 0 {
		def.DurationSec = domain.SaturateInt32(int64(def.DurationSec) * (100 + int64(durationBonus)) / 100)
	}
	if cap := s.statuses.SkillApplicationDurationCap(app); cap > 0 {
		def.DurationSec = min(def.DurationSec, cap)
	}
	if app.ID == domain.StatusStealth {
		def.Invisible = true
	}
	applyStatusSource(&def, skill.Icon, int32(skill.ID))
	specialWeaponStatus(skill, &def)
	s.legendaryInheritStatusModifiers(skill,&def)
	return def, true
}

func (s *Scene) applyCombatTriggeredStatus(holder, target *entity.Entity, id domain.StatusID) bool {
	var carrier domain.StatusDef
	var chance, level int32
	var totalFreezeChance int64
	holder.Status.EachEffective(func(active domain.Active) {
		candidate := active.Def
		if id == domain.StatusPoison {
			if candidate.PoisonOnHitChance > chance || candidate.PoisonOnHitChance == chance && candidate.PoisonOnHitProgressionRanks > carrier.PoisonOnHitProgressionRanks {
				chance, level, carrier = candidate.PoisonOnHitChance, candidate.PoisonOnHitLevel, candidate
			}
		} else {
			totalFreezeChance += int64(candidate.FreezeAttackerChance)
			if candidate.FreezeAttackerChance > chance {
				chance, level, carrier = candidate.FreezeAttackerChance, 1, candidate
			}
		}
	})
	if id == domain.StatusFreeze {
		// Preserve the existing aggregate counter-freeze trigger probability.
		chance = int32(min(int64(100), max(int64(0), totalFreezeChance)))
	}
	if chance <= 0 || level <= 0 {
		return false
	}
	app := domain.StatusApplication{ID: id, Level: level, ChanceBP: domain.SaturateInt32(int64(chance) * 100), ChanceExplicit: true}
	if id == domain.StatusPoison {
		app.ProgressionRanks = carrier.PoisonOnHitProgressionRanks
		app.ProgressionFirstLevel = carrier.PoisonOnHitFirstLevel
		app.ProgressionFirstDurationSec = carrier.PoisonOnHitFirstDurationSec
		app.ProgressionBaseDurationSec = carrier.PoisonOnHitBaseDurationSec
	}
	def, ok := s.statuses.ResolveSkillApplication(app)
	if !ok {
		return false
	}
	def.ChanceBP, def.ChanceExplicit = app.ChanceBP, true
	def.EffectHitBP = carrier.EffectHitBP
	if id == domain.StatusFreeze {
		def.EffectHitBP = domain.SaturateInt32(int64(def.EffectHitBP) + int64(carrier.FreezeAttackerEffectHitBP))
	}
	def.SourceEffectHitBP = s.sourceEffectHitBP(holder)
	if carrier.SourceEffectHitKnown {
		def.SourceEffectHitBP = min(def.SourceEffectHitBP, carrier.SourceEffectHitBP)
	} else {
		// Old saved carriers predate effect accuracy and therefore had no
		// accepted accuracy bonus. Recasting records a proper new snapshot.
		def.SourceEffectHitBP = min(def.SourceEffectHitBP, int32(0))
	}
	def.SourceEffectHitKnown = true
	applyStatusSource(&def, carrier.Icon, carrier.SourceSkillID)
	s.prepareClassSetPoison(holder, carrier, &def)
	return s.applyStatusDef(target, def, holder.ID)
}

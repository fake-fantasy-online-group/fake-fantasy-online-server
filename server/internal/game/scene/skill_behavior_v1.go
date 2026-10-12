package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// applyResolvedSkillModifiers is shared by actual casting and typed/UI views.
// It never consumes a status or mutates the immutable source SkillTable row.
func (s *Scene) applyResolvedSkillModifiers(p *entity.Entity, def *domain.SkillDef, hard, raw domain.Learned) {
	s.applyTrainingSynergiesWithHard(p, def, hard)
	s.applyPassiveSkillModifiersWithLevels(p, def, raw)
	def.UniqueInvisibleStart = entityInvisible(p)
	s.applyEquipmentSkillVariant(p, def, hard)
	s.applyUniqueSkillModifiers(p, def)
	finalizeEquipmentSkillVariant(def)
}

func (s *Scene) skillBehaviorIdentity(id domain.SkillID) *domain.SkillBehaviorContract {
	// Both resource histories identify 10212 as theft; the existing server row
	// is an unresolved damage placeholder. Never export that false identity.
	if id == 10212 {
		return nil
	}
	def, ok := s.skills[domain.SkillKey{ID: id, Level: 1}]
	if !ok {
		return nil
	}
	kind := map[domain.SkillKind]string{domain.SkillSingle: "single", domain.SkillArea: "area", domain.SkillPassive: "passive", domain.SkillSpecial: "special"}[def.Kind]
	prerequisites := append([]domain.SkillID{}, def.Prerequisites...)
	level := def.LevelNeed
	if identity, known := domain.V5SkillIdentity(id); known {
		level = identity.UnlockLevel
		prerequisites = append([]domain.SkillID{}, identity.Prerequisites...)
	}
	return &domain.SkillBehaviorContract{Name: def.Name, Icon: def.Icon, Profession: def.Prof, Kind: kind, UnlockLevel: level, PrerequisiteSkillIDs: prerequisites}
}

func (s *Scene) skillBehaviorPayload(p *entity.Entity, def domain.SkillDef) *domain.SkillBehaviorPayload {
	out := &domain.SkillBehaviorPayload{Variant: def.EquipmentVariant, MPCost: def.MPCost, PrepareMS: def.PrepareMS, CooldownMS: def.CooldownMS, CastDistance: def.Dist, Usable: def.Usable(), RequiresInvisible: def.RequiresInvisible, RequiredWeaponType: def.RequiredWeaponType,
		InputMode: "none", TargetSelf: def.TargetSelf, TargetEnemy: def.TargetEnemy, TargetTeam: def.TargetTeam && !def.HasDamageEffect(), AllowsDeadTarget: def.HasResurrectEffect(), DeadTargetOnly: def.ResurrectOnly(), GroundEffect: def.GroundEffect,
		Area: domain.SkillBehaviorArea{Shape: "none", Center: "none"}}
	// Preview only. Actual acceptance consumes the original status exactly once.
	if p != nil && p.Status != nil && def.PrepareMS > 0 {
		preview := p.Status.Clone()
		_, _, reduction, _ := preview.ConsumeAttackModifiers(true, def.HasDamageEffect(), len(def.Statuses) > 0, true)
		out.PrepareMS = skillPrepareAfterReduction(def.PrepareMS, reduction)
	}
	shape := map[domain.SkillAreaShape]string{domain.SkillAreaNone: "none", domain.SkillAreaRectangle: "rectangle", domain.SkillAreaCircle: "circle", domain.SkillAreaCone: "cone"}
	center := map[domain.SkillAreaCenter]string{domain.SkillAreaCenterNone: "none", domain.SkillAreaCenterCaster: "caster", domain.SkillAreaCenterTarget: "target"}
	if def.Area.Valid() {
		out.Area = domain.SkillBehaviorArea{Shape: shape[def.Area.Shape], Center: center[def.Area.Center], Radius: def.Area.Radius, Length: def.Area.Length, Width: def.Area.Width, Angle: def.Area.Angle}
	}
	if !def.Usable() {
		return out
	}
	if effect, special := specialSkillEffect(def); special {
		switch effect.Kind {
		case domain.EffectTeleportPoint:
			out.InputMode, out.PointRequired = "point", true
			out.TargetSelf, out.TargetEnemy, out.TargetTeam = true, false, false
		case domain.EffectTeleportTarget:
			out.InputMode = "entity"
			out.TargetSelf, out.TargetEnemy, out.TargetTeam = false, true, true
		case domain.EffectPlaceTrap:
			out.InputMode, out.SelfFallback = "point", true
			out.Area = domain.SkillBehaviorArea{Shape: "circle", Center: "target", Radius: effect.TrapRadius}
		case domain.EffectDetectTrap:
			out.InputMode, out.SelfFallback = "self", true
			out.Area = domain.SkillBehaviorArea{Shape: "circle", Center: "caster", Radius: def.Radius}
		case domain.EffectDisarmTrap:
			out.InputMode = "trapEntity"
		case domain.EffectRevealInvisible:
			out.InputMode, out.SelfFallback = "self", true
			out.Area = domain.SkillBehaviorArea{Shape: "circle", Center: "caster", Radius: effect.RevealRadius}
		}
		return out
	}
	if def.Area.Valid() {
		switch def.Area.Center {
		case domain.SkillAreaCenterCaster:
			if def.Area.Shape == domain.SkillAreaCircle {
				out.InputMode, out.SelfFallback = "self", true
			} else {
				out.InputMode = "direction"
			}
		case domain.SkillAreaCenterTarget:
			out.InputMode = "point"
		}
		return out
	}
	if def.GroundEffect && !skillCenteredOnCaster(def) {
		out.InputMode = "point"
		return out
	}
	if def.TargetSelf && !def.TargetEnemy && !def.TargetTeam {
		out.InputMode, out.SelfFallback = "self", true
		return out
	}
	out.InputMode = "entity"
	out.SelfFallback = def.TargetSelf && !def.TargetEnemy
	return out
}

// Shared with one-shot status consumption; use int64 for high-rank preparation.
func skillPrepareAfterReduction(prepare, reduction int32) int32 {
	reduction = min(int32(100), max(int32(0), reduction))
	return prepare - int32(int64(prepare)*int64(reduction)/100)
}

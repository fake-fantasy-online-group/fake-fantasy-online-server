package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
)

func (s *Scene) captureLegendaryChildPreparation(owner *entity.Entity, source *domain.CombatSourceSnapshotV5) {
	needs := false
	for _, power := range source.LegendaryPowers {
		switch power.Power.ID {
		// Include non-ChildSkill consumers: native-radius partitions, learned
		// passive checks, granted buff ranks, and field/dispel numerical bases.
		// Every same-action legendaryCasterSkill read after admission must use
		// these ranks; later independent casts capture their own source.
		case "SS-A04", "SS-A06", "SS-A08", "SS-A09", "SS-A13", "SS-A17", "SS-A21", "SS-A24", "SS-A25", "SS-A27", "SS-A28", "SS-A30",
			"YS-A07", "YS-A22", "YS-A23", "YS-A24", "YS-A28", "YS-U06", "CK-A26":
			needs = true
		}
	}
	if !needs {
		return
	}
	p := &domain.LegendaryChildPreparationSnapshot{MaximumMP: owner.MaxMP, Hard: owner.Player.Char.Skills.Clone(), Raw: s.rawSkillLevels(owner), SpecialWeapon: s.specialWeapon(owner)}
	p.Equipped, p.Bare = universalEquipmentContribution(&legendaryContext{Scene: s, Owner: owner})
	if owner.Player.EndgameSkillVariants {
		for _, a := range s.equipmentEffects(owner).effects {
			if a.Effect.Kind != "skill_variant" {
				continue
			}
			if v, ok := s.endgameCatalog.Variant(a.Effect.VariantID); ok {
				v.SourceUID, v.Item, v.EquipmentRevision = a.SourceUID, a.Item, owner.Player.Worn.EffectsRevision()
				p.Variants = append(p.Variants, v)
			}
		}
		sort.SliceStable(p.Variants, func(i, j int) bool { return p.Variants[i].SourceUID < p.Variants[j].SourceUID })
	}
	source.ChildPreparation = p
}

// Child preparation is pure and free. Do not read new gear or new soft ranks
// during a parent windup, nor temporarily change the live actor to fake a source.
func (s *Scene) prepareLegendaryChildNative(owner *entity.Entity, def *domain.SkillDef, source *domain.CombatSourceSnapshotV5) {
	if source == nil || source.ChildPreparation == nil {
		s.applyResolvedSkillModifiers(owner, def, owner.Player.Char.Skills, s.rawSkillLevels(owner))
		return
	}
	p := source.ChildPreparation
	s.applyTrainingSynergiesWithLevels(owner, def, p.Hard, p.Raw)
	s.applyPassiveSkillModifiersWithLevels(owner, def, p.Raw)
	def.SnapshotHardRanks, def.SnapshotRawRanks = p.Hard, p.Raw
	def.UniqueInvisibleStart = source.Invisible
	if p.Hard[def.ID] > 0 {
		var selected *domain.EquipmentSkillVariant
		conflict := false
		for i := range p.Variants {
			v := &p.Variants[i]
			if v.SkillID != def.ID || v.Profession != def.Prof {
				continue
			}
			if selected == nil {
				selected = v
			} else if selected.ID != v.ID {
				conflict = true
			}
		}
		if selected != nil && !conflict {
			applyEquipmentSkillVariantDefinition(def, *selected)
		}
	}
	context := domain.UniqueContext{SkillID: def.ID, HPPercent: source.HPPercent, MPPercent: source.MPPercent, Skill: true,
		Magic: def.HurtType != domain.HurtNone && def.HurtType != domain.HurtPhysical, Area: def.Area.Valid() || def.Radius > 0}
	for _, e := range def.DirectEffects() {
		context.Healing = context.Healing || e.Kind == domain.EffectHeal || e.Kind == domain.EffectRestoreHP
		context.Trap = context.Trap || e.Kind == domain.EffectPlaceTrap
	}
	x := domain.EvaluateUniqueTraits(source.Traits, context)
	applyUniqueSkillDefinition(def, x, true)
	finalizeEquipmentSkillVariant(def)
	if p.Hard[def.ID] > 0 {
		applySpecialWeaponDefinition(p.SpecialWeapon, def)
	}
	domain.FinalizeSkillManaCost(def)
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) captureCombatSourceV5(src *entity.Entity, invisible bool) *domain.CombatSourceSnapshotV5 {
	out := &domain.CombatSourceSnapshotV5{
		Stats: src.Stats, Equipment: s.equipmentOffense(src),
		Traits:    append([]domain.UniqueTrait(nil), s.uniqueTraits(src)...),
		HPPercent: uniquePercent(src.HP, src.MaxHP), MPPercent: uniquePercent(src.MP, src.MaxMP),
		Invisible: invisible,
	}
	if src.Status != nil {
		out.StatusPresent = true
		out.PhysicalFlat = src.Status.OutgoingDamageFlat(false)
		out.MagicFlat = src.Status.OutgoingDamageFlat(true)
	}
	if src.Player == nil {
		return out
	}
	pet := s.entities[src.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || !pet.Alive() {
		return out
	}
	inst := pet.Pet.Inst
	bonus := s.petCombatSkillLevelBonus(inst)
	for _, learned := range inst.Skills {
		d, ok := s.petSkills[learned.ID]
		if !ok || d.Effect == nil || learned.Level <= 0 {
			continue
		}
		e := d.Effect
		if e.Kind != "physical_extra" && e.Kind != "magic_extra" || e.RequireSelected && inst.ActiveSkill != learned.ID {
			continue
		}
		level := learned.Level
		if d.Fight {
			level += bonus
		}
		if d.MaxLevel > 0 {
			level = min(level, d.MaxLevel)
		}
		var stat int32
		switch e.SourceAttr {
		case domain.AttrSTR:
			stat = inst.Base.STR
		case domain.AttrINT:
			stat = inst.Base.INT
		case domain.AttrAGI:
			stat = inst.Base.AGI
		case domain.AttrVIT:
			stat = inst.Base.VIT
		case domain.AttrSPI:
			stat = inst.Base.SPI
		}
		if e.SourceStep <= 0 {
			continue
		}
		out.PetExtras = append(out.PetExtras, domain.PetOffenseSnapshotV5{PetID: inst.ID, SkillID: learned.ID, Magic: e.Kind == "magic_extra", ChanceBP: e.Chance(level), Amount: clampI32(int64(stat/e.SourceStep) * int64(e.Amount(level)))})
	}
	return out
}

func sourceUniqueContextV5(c domain.UniqueContext, source *domain.CombatSourceSnapshotV5) domain.UniqueContext {
	c.HPPercent, c.MPPercent, c.Invisible = source.HPPercent, source.MPPercent, source.Invisible
	return c
}

// Persistent sources retain only powers present in both snapshots. Evaluate
// source predicates separately, but evaluate target predicates at impact.
func sourceUniqueEffectsV5(source, current *domain.CombatSourceSnapshotV5, c domain.UniqueContext) domain.UniqueEffects {
	castContext := sourceUniqueContextV5(c, source)
	if current == nil {
		return domain.EvaluateUniqueTraits(source.Traits, castContext)
	}
	currentContext := sourceUniqueContextV5(c, current)
	var allowed []domain.UniqueTrait
	for _, a := range source.Traits {
		if domain.EvaluateUniqueTraits([]domain.UniqueTrait{a}, castContext) == (domain.UniqueEffects{}) {
			continue
		}
		var power int32
		for _, b := range current.Traits {
			if a.Kind != b.Kind || a.SkillID != b.SkillID || domain.EvaluateUniqueTraits([]domain.UniqueTrait{b}, currentContext) == (domain.UniqueEffects{}) {
				continue
			}
			power = max(power, min(a.PowerBP, b.PowerBP))
		}
		if power > 0 {
			a.PowerBP = power
			allowed = append(allowed, a)
		}
	}
	return domain.EvaluateUniqueTraits(allowed, castContext)
}

func (s *Scene) combatSourceV5(src *entity.Entity) (source, current *domain.CombatSourceSnapshotV5) {
	if root := s.equipmentState().root; root != nil && root.src == src.ID && root.source != nil {
		return root.source, root.currentSource
	}
	return s.captureCombatSourceV5(src, entityInvisible(src)), nil
}

func (s *Scene) sourcePetExtraV5(source, current *domain.CombatSourceSnapshotV5, magic bool) int32 {
	var total int64
	for _, a := range source.PetExtras {
		if a.Magic != magic {
			continue
		}
		if current != nil {
			chance, amount := int32(0), int32(0)
			for _, b := range current.PetExtras {
				if a.PetID == b.PetID && a.SkillID == b.SkillID && a.Magic == b.Magic {
					chance = max(chance, min(a.ChanceBP, b.ChanceBP))
					amount = max(amount, min(a.Amount, b.Amount))
				}
			}
			a.ChanceBP, a.Amount = chance, amount
		}
		// Preserve the native per-hit roll, including one independent roll for
		// every matching pet skill on every AOE target.
		if int32(s.rng.Intn(10000)) < a.ChanceBP {
			total += int64(a.Amount)
		}
	}
	return clampI32(total)
}

func sourceStatusFlatV5(source, current *domain.CombatSourceSnapshotV5, magic bool) int32 {
	value := source.PhysicalFlat
	if magic {
		value = source.MagicFlat
	}
	if current != nil {
		other := current.PhysicalFlat
		if magic {
			other = current.MagicFlat
		}
		value = min(value, other)
	}
	return value
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"slices"
)

// Half-damage passives keep their native 50% result after their chance caps.
// Further ranks grant constitution through the normal attribute pipeline.
func (s *Scene) passiveHalfDamage(_ *entity.Entity, _ domain.PassiveModifierKind, incoming int32) int32 {
	return domain.SaturateInt32((int64(incoming) + 1) / 2)
}

// computeSkillGrowthStats uses the same equipped/status pipeline for both totals.
// The difference includes legitimate percentage amplification of growth attributes,
// so none of their extra HP/MP can leak into refreshStats' resource restoration.
func (s *Scene) computeSkillGrowthStats(p *entity.Entity, worn *domain.EquipSet) (domain.Base, domain.Stats, domain.Stats) {
	return s.computeSkillGrowthStatsResolved(p, worn, newEquipmentRequirementResolution())
}

func (s *Scene) computeSkillGrowthStatsResolved(p *entity.Entity, worn *domain.EquipSet, resolution *equipmentRequirementResolution) (domain.Base, domain.Stats, domain.Stats) {
	ch := p.Player.Char
	preview := *p
	player := *p.Player
	player.Worn = worn
	preview.Player = &player
	src := domain.StatSource{
		Base: ch.EffectiveBase(), Level: ch.Level, Innate: characterInnateStats(ch),
		Worn: worn, Defs: s.itemDef, Passive: s.passiveStatAffixesResolved(&preview, resolution), Status: p.Status,
	}
	base, stats := domain.Compute(src)
	without := stats
	withoutGrowth := s.passiveStatAffixesIncludingGrowth(&preview, resolution, false)
	if !slices.Equal(src.Passive, withoutGrowth) {
		src.Passive = withoutGrowth
		_, without = domain.Compute(src)
	}
	stats = s.classSetMeleeStats(p, worn, stats)
	without = s.classSetMeleeStats(p, worn, without)
	return base, stats, domain.Stats{
		MaxHP: domain.SaturateInt32(int64(stats.MaxHP) - int64(without.MaxHP)),
		MaxMP: domain.SaturateInt32(int64(stats.MaxMP) - int64(without.MaxMP)),
	}
}

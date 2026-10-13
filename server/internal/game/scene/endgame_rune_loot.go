package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) astralDropGrade(dead *entity.Entity) domain.AstralDropGrade {
	if dead == nil || dead.Monster == nil {
		return domain.AstralDropOrdinary
	}
	if _, ok := s.bossFixedDrops[domain.BossFixedDropKey{MapID: s.id.MapID, Monster: dead.Monster.TypeID}]; ok {
		return domain.AstralDropBoss
	}
	switch dead.Monster.Kind {
	case domain.MonsterBoss:
		return domain.AstralDropBoss
	case domain.MonsterElite:
		return domain.AstralDropElite
	}
	return domain.AstralDropOrdinary
}

// At most one supplemental rune per level20+ combat kill; original drop-table
// limits and level penalty remain intact. The published rule weights select
// which rune drops without changing the overall occurrence rate.
func (s *Scene) rollAstralRuneLoot(dead *entity.Entity, killer domain.EntityID, slot int) {
	if dead == nil || dead.Monster == nil || s.defs == nil || !dead.Monster.Kind.Hostile() {
		return
	}
	rule := s.lootRules.Active()
	if rule == nil {
		return
	}
	md, ok := s.defs.Def(dead.Monster.TypeID)
	if !ok || md.Level < rule.MinRuneLevel {
		return
	}
	if _, ok := s.itemDef(domain.FirstAstralRune); !ok {
		return
	}
	entry := rule.RuneDrop(s.astralDropGrade(dead))
	if !entry.RollWithMult(s.rng.Intn, s.dropMultBP(killer, dead.Monster.TypeID)) {
		return
	}
	id, ok := rule.RollRune(s.rng.Intn)
	if !ok {
		return
	}
	def, ok := s.itemDef(id)
	if !ok {
		return
	}
	s.spawnGeneratedLoot(def, domain.NewStack(def, 1), s.monsterLootPosition(dead.Pos, slot), killer)
}

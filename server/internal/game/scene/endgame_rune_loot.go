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
// limits remain intact. All28 are equiprobable, subject to existing level penalty.
func (s *Scene) rollAstralRuneLoot(dead *entity.Entity, killer domain.EntityID, slot int) {
	if dead == nil || dead.Monster == nil || s.defs == nil || !dead.Monster.Kind.Hostile() {
		return
	}
	md, ok := s.defs.Def(dead.Monster.TypeID)
	if !ok || md.Level < 20 {
		return
	}
	if _, ok := s.itemDef(domain.FirstAstralRune); !ok {
		return
	}
	rate := 0.6
	switch s.astralDropGrade(dead) {
	case domain.AstralDropElite:
		rate = 8
	case domain.AstralDropBoss:
		rate = 20
	}
	if !(domain.DropEntry{RatePct: rate}).RollWithMult(s.rng.Intn, s.dropMultBP(killer, dead.Monster.TypeID)) {
		return
	}
	def, ok := s.itemDef(domain.FirstAstralRune + domain.ItemID(s.rng.Intn(28)))
	if !ok {
		return
	}
	s.spawnGeneratedLoot(def, domain.NewStack(def, 1), s.monsterLootPosition(dead.Pos, slot), killer)
}

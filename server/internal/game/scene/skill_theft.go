package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// theftEntries uses only the monster's loaded original drop table. Boss minimum
// rewards, supplemental runes, and conditional quest rewards are kill pathways,
// not extra theft pools. Invalid runtime configuration fails closed.
func (s *Scene) theftEntries(monster domain.MonsterID) []domain.DropEntry {
	var out []domain.DropEntry
	for _, entry := range s.lootTable[monster] {
		if entry.DungeonOnly && s.dungeon == nil {
			continue
		}
		if domain.TheftDropWeight(entry) == 0 {
			continue
		}
		if len(entry.Choices) == 0 {
			if _, ok := s.items[entry.Item]; !ok || !s.lootRules.Active().AllowsCombatMaterial(entry.Item) {
				continue
			}
		} else {
			valid := true
			for _, id := range entry.Choices {
				if _, ok := s.items[id]; !ok || !s.lootRules.Active().AllowsCombatMaterial(id) {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
		}
		out = append(out, entry)
	}
	return out
}

// This check is used at admission and immediately before settlement. The marker
// lives on the monster instance, so another player, reconnection, or a return to
// the map cannot reset it. Only a new monster life creates another opportunity.
func (s *Scene) theftTargetEligible(src, dst *entity.Entity, def domain.SkillDef, effect domain.SkillEffect) bool {
	if src == nil || src.Kind != domain.KindPlayer || src.Player == nil || s.players[src.ID] != src || !src.Alive() ||
		dst == nil || dst.Kind != domain.KindMonster || dst.Monster == nil || s.entities[dst.ID] != dst || !dst.Alive() ||
		!def.TargetEnemy || !dst.Monster.Kind.Hostile() || dst.Monster.Summoner != 0 ||
		effect.TheftMaxSuccesses != 1 || dst.Monster.TheftSuccesses >= effect.TheftMaxSuccesses ||
		s.alloc == nil || s.entityMapLoading(src) || src.Pos.MapID != dst.Pos.MapID {
		return false
	}
	if def.Dist > 0 && sqDist(src.Pos, dst.Pos) > float64(def.Dist)*float64(def.Dist) {
		return false
	}
	if s.defs != nil {
		if monsterDef, ok := s.defs.Def(dst.Monster.TypeID); ok {
			if _, immune := monsterDef.ImmuneSkills[def.ID]; immune {
				return false
			}
		}
	}
	return len(s.theftEntries(dst.Monster.TypeID)) > 0
}

// stealMonsterItem runs only on the scene actor. Probability is checked once;
// after that, one weighted row yields one generated ground stack. No bag write,
// asynchronous save, or deferred callback can duplicate this result.
func (s *Scene) stealMonsterItem(src, dst *entity.Entity, def domain.SkillDef, effect domain.SkillEffect) bool {
	if !s.theftTargetEligible(src, dst, def, effect) {
		return false
	}
	if effect.ChanceBP <= 0 || effect.ChanceBP < 10000 && int32(s.rng.Intn(10000)) >= effect.ChanceBP {
		s.emitTo(src.ID, event.ServerNotice{Who: src.ID, Text: "偷窃失败。"})
		return false
	}
	entry, ok := domain.PickTheftDrop(s.theftEntries(dst.Monster.TypeID), s.rng.Int63n)
	if !ok {
		return false
	}
	id := entry.Item
	if len(entry.Choices) > 0 {
		id = entry.Choices[s.rng.Intn(len(entry.Choices))]
	}
	item := s.items[id]
	stack := domain.NewStack(item, 1)
	profile := dst.Monster.ColorProfile
	if profile == 0 {
		profile = domain.DefaultColorProfile
	}
	// A separate ordinal keeps theft and a same-tick kill from sharing equipment
	// random streams. The normal generation and MF rules themselves are unchanged.
	s.rollNaturalEquipmentV5(item, &stack, dst, -1, profile, entry.RollAffixes, s.freezeLootContextV5(src.ID))
	before := len(s.ground)
	s.spawnGeneratedLoot(item, stack, dst.Pos, src.ID)
	if len(s.ground) != before+1 {
		return false
	}
	dst.Monster.TheftSuccesses++
	s.emitTo(src.ID, event.ServerNotice{Who: src.ID, Text: fmt.Sprintf("偷窃成功：%s ×1 已掉落在怪物脚下。", itemDisplayName(item, stack))})
	return true
}

package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// The only natural creation gate for true equipment. The exact original drop
// must already have rolled; this never adds unrelated gear to a monster's pool.
func (s *Scene) dungeonLootItem(item domain.ItemID, monster domain.MonsterID) (domain.ItemDef, bool) {
	def, ok := s.items[item]
	if !ok {
		return def, false
	}
	if def.HardDungeonOnly {
		return def, false
	} // Never consume direct hard IDs from inherited/generic loot.
	if s.nativeDifficultyDungeon() && s.dungeonDifficulty == domain.DungeonHard {
		if hardID := domain.HardDungeonDropVariant(item, s.id.MapID, monster); hardID != 0 {
			hard, exists := s.items[hardID]
			return hard, exists && hard.HardDungeonOnly && hard.DungeonBaseID == item
		}
	}
	return def, true
}

// Augment a per-kill view, not the shared monster table: reused templates in
// Rift/activity maps never inherit these newly assigned temple drops.
func (s *Scene) withLostTempleAncientDrops(entries []domain.DropEntry, monster domain.MonsterID) []domain.DropEntry {
	if !s.nativeDifficultyDungeon() {
		return entries
	}
	added := domain.AdditionalLostTempleDrops(s.id.MapID, monster)
	if len(added) == 0 {
		return entries
	}
	seen := map[domain.ItemID]bool{}
	for _, e := range entries {
		seen[e.Item] = true
		for _, id := range e.Choices {
			seen[id] = true
		}
	}
	out := append([]domain.DropEntry(nil), entries...)
	for _, e := range added {
		if !seen[e.Item] {
			out = append(out, e)
			seen[e.Item] = true
		}
	}
	return out
}

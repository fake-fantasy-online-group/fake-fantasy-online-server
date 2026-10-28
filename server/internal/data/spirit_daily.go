package data

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"sort"
)

// Only configured persistent world spawns qualify. Dungeon boss templates,
// scripted summons and Rift monsters cannot enter the target pool.
func SpiritDailyTargets(spawns SpawnTable, defs spawn.MapDefs, dungeons domain.DungeonTable, maps *MapCatalog, trials ...*domain.TrialRules) []domain.SpiritDailyTarget {
	out := []domain.SpiritDailyTarget{}
	seen := map[domain.MonsterID]bool{}
	ids := []int32{}
	for id := range spawns {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, mapID := range ids {
		trialMap := false
		for _, trial := range trials {
			if trial.IsMap(mapID) {
				trialMap = true
			}
		}
		if trialMap {
			continue
		}
		if _, ok := dungeons[mapID]; ok {
			continue
		}
		if mapID <= 0 {
			continue
		}
		for _, p := range spawns[mapID] {
			d, ok := defs.Def(p.Monster)
			if !ok || d.Kind != domain.MonsterBoss || p.ID <= 0 || seen[p.Monster] {
				continue
			}
			name := "野外"
			if maps != nil {
				if m, ok := maps.ByID(mapID); ok {
					name = m.Name
				}
			}
			seen[p.Monster] = true
			out = append(out, domain.SpiritDailyTarget{Monster: p.Monster, MapID: mapID, Name: d.Name, MapName: name})
		}
	}
	return out
}

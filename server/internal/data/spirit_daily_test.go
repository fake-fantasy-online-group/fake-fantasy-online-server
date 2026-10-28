package data

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func TestSpiritDailyWorldSpawnPool(t *testing.T) {
	defs := spawn.MapDefs{1: {Kind: domain.MonsterBoss}, 2: {Kind: domain.MonsterNormal}, 3: {Kind: domain.MonsterBoss}, 4: {Kind: domain.MonsterBoss}}
	points := SpawnTable{1: {{ID: 1, Monster: 1}, {ID: 2, Monster: 2}, {ID: 0, Monster: 4}}, 2: {{ID: 3, Monster: 3}}, 3: {{ID: 4, Monster: 1}}}
	got := SpiritDailyTargets(points, defs, domain.DungeonTable{2: {}}, nil)
	if len(got) != 1 || got[0].Monster != 1 || got[0].MapID != 1 {
		t.Fatalf("not actual distinct wild bosses: %+v", got)
	}
}

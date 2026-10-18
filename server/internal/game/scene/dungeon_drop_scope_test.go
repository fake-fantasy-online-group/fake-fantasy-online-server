package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestDungeonSupplementalDropsRespectSceneScope(t *testing.T) {
	for _, dungeon := range []bool{false, true} {
		name := "field"
		if dungeon {
			name = "dungeon"
		}
		t.Run(name, func(t *testing.T) {
			entries := []domain.DropEntry{{Item: 掉落牛肉面.ID, RatePct: 100, DungeonOnly: true}, {Item: 掉落树枝.ID, RatePct: 100}}
			s, mid := lootScene(t, entries)
			if dungeon {
				s.dungeon = &domain.DungeonDef{ID: 1}
			}
			s.rollLoot(s.entities[mid], 0)
			want := 1
			if dungeon {
				want = 2
			}
			if len(s.ground) != want {
				t.Fatalf("ground=%d want%d", len(s.ground), want)
			}
			if got := s.theftEntries(必掉的怪.ID); len(got) != want {
				t.Fatalf("theft=%+v want%d rows", got, want)
			}
			if !dungeon {
				for _, e := range s.ground {
					if e.Drop.Item != 掉落树枝.ID {
						t.Fatal("dungeon addition leaked into field")
					}
				}
			}
			if !s.lootTable[必掉的怪.ID][0].DungeonOnly || s.lootTable[必掉的怪.ID][0].RatePct != 100 {
				t.Fatal("shared table mutated")
			}
		})
	}
}

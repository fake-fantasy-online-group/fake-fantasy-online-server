package scene

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// These values are the field template and six native direct-item drop rows from
// init-32.sql, independently matched to the original monster recovery data.
// init-39.sql also appends the same-level 5019 donor at double rates because
// 1041 appears in pw0200. Include its five ordinary direct-item rows, tagged
// dungeon-only so field kills cannot inherit that supplemental pool;
// equipment category pools and explicit donor weapons need richer item fixtures.
// This exercises ordinary kill settlement, not a rift or a live database.
func TestGreenFrogFieldKillRewardAudit(t *testing.T) {
	frog := domain.MonsterDef{ID: 1041, Name: "绿蛙兵", Kind: domain.MonsterNormal, Level: 19, HP: 569, Exp: 131, Sprite: 241, Stats: domain.NewMonsterStats(23, 8, 57, 23, 8, 1500, 120)}
	entries := []domain.DropEntry{
		{Item: 4162, RatePct: 28.758707}, {Item: 3382, RatePct: 1.514169},
		{Item: 3002, RatePct: 7.694213}, {Item: 3004, RatePct: 7.694213},
		{Item: 25118, RatePct: 0.15}, {Item: 3128, RatePct: 10},
		{DungeonOnly: true, Item: 4126, RatePct: 40}, {DungeonOnly: true, Item: 3034, RatePct: 60},
		{DungeonOnly: true, Item: 3035, RatePct: 60}, {DungeonOnly: true, Item: 3205, RatePct: 4},
		{DungeonOnly: true, Item: 3206, RatePct: 4},
	}
	for _, tc := range []struct {
		level int32
		exp   int64
	}{{19, 131}, {30, 15}, {31, 5}, {32, 0}} {
		t.Run(fmt.Sprint(tc.level), func(t *testing.T) {
			drops := 0
			for seed := int64(0); seed < 32; seed++ {
				s, mid := rewardScene(t, frog)
				s.rng = rand.New(rand.NewSource(seed))
				s.alloc = domain.NewEntityAlloc()
				s.penalty = 定案惩罚()
				s.items = map[domain.ItemID]domain.ItemDef{}
				for _, entry := range entries {
					s.items[entry.Item] = domain.ItemDef{ID: entry.Item, Name: "field frog item", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
				}
				s.lootTable = map[domain.MonsterID][]domain.DropEntry{1041: entries}
				sink := join(t, s, 1, 100, "field-frog-audit", 100, 100)
				s.step()
				sink.take()
				p := s.entities[1]
				p.Player.Char.Level = tc.level
				p.Level = tc.level
				oneShot(s)
				s.exec(Attack{ID: 1, Target: mid})
				s.step()
				if s.entities[mid].Alive() {
					t.Fatal("field frog did not die")
				}
				ev, ok := firstOf[event.ExpGained](sink.take())
				if !ok || ev.Delta != tc.exp || p.Player.Char.Exp != tc.exp {
					t.Fatalf("seed %d: exp event %+v present=%v total=%d want=%d", seed, ev, ok, p.Player.Char.Exp, tc.exp)
				}
				for id, drop := range s.ground {
					if drop.Drop == nil || drop.Drop.Owner != p.ID || s.entities[id] != drop {
						t.Fatalf("invalid corpse-drop identity or ownership: %+v", drop)
					}
					for _, entry := range entries {
						if entry.DungeonOnly && entry.Item == drop.Drop.Item {
							t.Fatal("field frog received dungeon-only supplemental drop")
						}
					}
					if _, exists := s.items[drop.Drop.Item]; !exists {
						t.Fatal("ground drop missing item definition")
					}
				}
				drops += len(s.ground)
			}
			if drops == 0 {
				t.Fatal("32 deterministic ordinary kills produced no direct-item drops")
			}
			t.Logf("32 field kills: XP each=%d, direct-item drops=%d", tc.exp, drops)
		})
	}
}

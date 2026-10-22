package scene

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"testing"
	"time"
)

func TestDungeonTransportRealOpenChooseNormalAndHard(t *testing.T) {
	for _, tc := range []struct{ list, source, target int32 }{{127, 19, 23800}, {110, 32, 24400}, {110, 32, 26800}, {125, 7, 24500}, {115, 7, 26900}, {117, 7, 30600}} {
		for _, difficulty := range []domain.DungeonDifficulty{domain.DungeonNormal, domain.DungeonHard} {
			rig := &dungeonRig{fr: map[domain.SceneID]chan time.Time{}}
			def := domain.DungeonDef{ID: domain.DungeonID(tc.target/100 - 100), Name: "verified", Enter: domain.Pos{MapID: tc.target, X: 500, Y: 500}, Exit: domain.Pos{MapID: tc.source, X: 100, Y: 100}, TimeoutSec: 900}
			tbl := domain.DungeonTable{tc.target: def}
			lists := domain.TransportTable{tc.list: {ID: tc.list, SourceMapID: tc.source, Destinations: []domain.TransportDestination{{Name: "verified", Pos: def.Enter, Dungeon: true}}}}
			rig.r = NewRouter(func(id domain.SceneID) (*Scene, error) {
				ch := make(chan time.Time)
				rig.put(id, ch)
				cfg := Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: ch, Dungeons: tbl, Transports: lists, Alloc: domain.NewEntityAlloc()}
				if id.Persistent() {
					cfg.NPCs = []domain.NPCSpawn{{Name: "entry", Sprite: 1, Trans: tc.list, Pos: domain.Pos{MapID: tc.source, X: 100, Y: 100}}}
				} else {
					cfg.Dungeon = &def
				}
				return New(cfg), nil
			}, nil)
			ctx, cancel := context.WithCancel(context.Background())
			rig.r.Start(ctx)
			t.Cleanup(cancel)
			town, _ := rig.r.Ensure(domain.SceneID{MapID: tc.source})
			sink := &fakeSink{}
			town.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: sink, Party: 1, Char: &domain.Character{ID: 100, Name: "selector", Pos: domain.Pos{MapID: tc.source, X: 100, Y: 100}}})
			rig.step(t, town, func(*Scene) {})
			town.Post(ChooseTransport{ID: 1, List: tc.list, Index: 1})
			rig.step(t, town, func(s *Scene) {
				if s.players[1] == nil || len(rig.instances(tc.target)) != 0 {
					t.Fatal("unopened hard menu admitted", tc)
				}
			})
			sink.take()
			town.Post(OpenTransport{ID: 1, List: tc.list})
			rig.step(t, town, func(*Scene) {})
			found := false
			for _, e := range sink.take() {
				if menu, ok := e.(event.TransportDestinations); ok {
					found = true
					if len(menu.Destinations) != 2 || menu.Destinations[0].Difficulty != domain.DungeonNormal || menu.Destinations[1].Difficulty != domain.DungeonHard {
						t.Fatal(tc, menu)
					}
				}
			}
			if !found {
				t.Fatal("native menu not emitted", tc)
			}
			town.Post(ChooseTransport{ID: 1, List: tc.list, Index: int32(difficulty)})
			rig.step(t, town, func(*Scene) {})
			instances := rig.instances(tc.target)
			if len(instances) != 1 {
				t.Fatal("choose did not allocate instance", tc, instances)
			}
			dungeon, _ := rig.r.Ensure(instances[0])
			rig.step(t, dungeon, func(s *Scene) {
				if s.players[1] == nil || s.dungeonDifficulty != difficulty || !s.nativeDifficultyDungeon() {
					t.Fatal("wrong instance or difficulty", tc, difficulty)
				}
			})
		}
	}
}

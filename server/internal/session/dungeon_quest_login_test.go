package session

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"testing"
	"time"
)

func TestQuestDungeonLoginRechecksLoadedUnfinishedHistory(t *testing.T) {
	for _, pair := range [][2]int32{{29200, 355}, {29400, 356}, {29900, 359}, {30000, 361}} {
		for _, state := range []int{-1, 0, 1, 2} {
			t.Run(fmt.Sprintf("%d-state%d", pair[0], state), func(t *testing.T) {
				rig := newAuthRig(t)
				_, ch := rig.mkAccountWithChar("login", "returner")
				def := domain.DungeonDef{ID: domain.DungeonID(pair[0]), Solo: true, Enter: domain.Pos{MapID: pair[0], X: 100, Y: 100}, Exit: domain.Pos{MapID: 7, X: 900, Y: 900}}
				table := domain.DungeonTable{pair[0]: def}
				router := scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
					return scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: make(chan time.Time), Dungeons: table, Dungeon: &def}), nil
				}, nil)
				ctx, cancel := context.WithCancel(context.Background())
				router.Start(ctx)
				t.Cleanup(func() { cancel(); router.Drain(time.Second) })
				saved, err := router.OpenDungeonInstance(def, def.Owner(domain.CharID(ch.ID), 0))
				if err != nil {
					t.Fatal(err)
				}
				ch.Pos = def.Enter
				ch.SceneInstance = saved.Instance
				ch.Quests = domain.QuestLog{}
				if state >= 0 {
					ch.Quests[domain.QuestID(pair[1])] = domain.QuestEntry{State: domain.QuestState(state)}
				}
				if err := rig.st.SaveSnapshot(ctx, domain.Snapshot{Char: ch, Bag: domain.NewBag(10), Worn: domain.NewEquipSet()}); err != nil {
					t.Fatal(err)
				}
				// Same order as onEnterWorld: saved character, full quest log, login scene.
				loaded, err := rig.st.CharByName(ctx, ch.Name)
				if err != nil {
					t.Fatal(err)
				}
				loaded.Quests, err = rig.st.LoadQuests(ctx, ch.ID)
				if err != nil {
					t.Fatal(err)
				}
				rig.sess.deps.Router = router
				rig.sess.deps.Dungeons = table
				got, err := rig.sess.resolveLoginScene(ctx, loaded, 0)
				if err != nil {
					t.Fatal(err)
				}
				allowed := state == 0 || state == 1
				if allowed {
					if got != saved || loaded.SceneInstance != saved.Instance {
						t.Fatal("unfinished task cannot resume existing owned instance", got)
					}
				} else {
					if got != (domain.SceneID{MapID: 7}) || loaded.Pos != def.Exit || loaded.SceneInstance != 0 {
						t.Fatal("completed/missing quest resumed dungeon", state, got)
					}
					persisted, err := rig.st.CharByName(ctx, ch.Name)
					if err != nil || persisted.Pos != def.Exit || persisted.SceneInstance != 0 {
						t.Fatal("safe return was not saved", persisted, err)
					}
				}
			})
		}
	}
}

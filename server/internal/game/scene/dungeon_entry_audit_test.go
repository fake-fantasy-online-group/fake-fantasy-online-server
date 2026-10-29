package scene

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

// Approved task-only entry rule: three loaded solo story maps (355 remains held) accept unfinished tasks,
// including Deliver, but reject missing/abandoned/finished. Ordinary farm maps
// stay unrestricted; the special trial retains its stricter Active-only rule.
func TestDungeonQuestEntryStateMatrix(t *testing.T) {
	for _, target := range []struct {
		mapID       int32
		quest       domain.QuestID
		solo, trial bool
	}{
		{29200, 355, true, false}, {29400, 356, true, false}, {29900, 359, true, false}, {30000, 361, true, false},
		{20701, 211, false, false}, {21100, 233, false, true},
	} {
		for _, state := range []struct {
			name    string
			present bool
			value   domain.QuestState
		}{
			{"unaccepted-or-abandoned", false, 0}, {"active", true, domain.QuestActive},
			{"deliverable", true, domain.QuestDeliver}, {"finished", true, domain.QuestFinished},
		} {
			t.Run(fmt.Sprintf("map%d-%s", target.mapID, state.name), func(t *testing.T) {
				s := newTestScene(t, newFakeSaver())
				join(t, s, 1, 100, "audit", 100, 100)
				p := s.players[1]
				p.Player.Char.Level = 50
				p.Player.Char.Quests = domain.QuestLog{}
				if state.present {
					p.Player.Char.Quests[target.quest] = domain.QuestEntry{State: state.value}
				}
				before := domain.QuestLog{}
				for id, q := range p.Player.Char.Quests {
					before[id] = q
				}
				if !target.solo {
					p.Player.Party = 1
				}
				def := domain.DungeonDef{ID: domain.DungeonID(target.mapID), Name: "audit", Solo: target.solo,
					Enter: domain.Pos{MapID: target.mapID, X: 100, Y: 100}, Exit: domain.Pos{MapID: 7, X: 100, Y: 100}}
				s.dungeons = domain.DungeonTable{target.mapID: def}
				if target.trial {
					s.trial = &domain.TrialRules{Quest: 233, MinLevel: 12, MaxLevel: 60,
						Maps: map[int32][]domain.TrialSpawn{target.mapID: {{Point: domain.SpawnPoint{ID: 1}}}}}
				}
				r := NewRouter(func(id domain.SceneID) (*Scene, error) {
					return New(Config{ID: id,
						EndgameCatalog: catalogfixture.EndgameCatalog(), Frames: make(chan time.Time), Dungeons: s.dungeons}), nil
				}, nil)
				ctx, cancel := context.WithCancel(context.Background())
				r.Start(ctx)
				t.Cleanup(cancel)
				s.router = r
				id, err := s.openPlayerDungeon(p, def)
				restricted := domain.DungeonEntryQuest(target.mapID) != 0
				want := !restricted && !target.trial || state.present && (target.trial && state.value == domain.QuestActive || restricted && (state.value == domain.QuestActive || state.value == domain.QuestDeliver))
				if (err == nil) != want {
					t.Fatalf("current gate allow=%v, instance=%v err=%v", want, id, err)
				}
				if want && (id.Persistent() || id.MapID != target.mapID) {
					t.Fatalf("not an instance: %v", id)
				}
				if !reflect.DeepEqual(before, p.Player.Char.Quests) {
					t.Fatal("entry mutated quest state")
				}
			})
		}
	}
}

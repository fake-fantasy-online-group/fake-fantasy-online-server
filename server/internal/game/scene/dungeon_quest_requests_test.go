package scene

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"testing"
	"time"
)

func TestQuestDungeonDirectRequestsCannotBypassAdmission(t *testing.T) {
	for _, pair := range [][2]int32{{29400, 356}, {29900, 359}, {30000, 361}} {
		for _, finished := range []bool{false, true} {
			for _, instance := range []uint32{0, 123} {
				s := newTestScene(t, newFakeSaver())
				sink := join(t, s, 1, 100, "direct", 100, 100)
				p := s.players[1]
				p.Pos.MapID = s.id.MapID
				p.Player.Char.Quests = domain.QuestLog{}
				if finished {
					p.Player.Char.Quests[domain.QuestID(pair[1])] = domain.QuestEntry{State: domain.QuestFinished}
				}
				before := p.Pos
				sink.take()
				if s.onTeleport(Teleport{ID: 1, To: domain.SceneID{MapID: pair[0], Instance: instance}, At: domain.Pos{MapID: pair[0], X: 10, Y: 20}}) {
					t.Fatal("unauthorized direct transfer consumed", pair, finished, instance)
				}
				s.flush()
				if p.Pos != before || s.players[1] != p || countOf[event.Rejected](sink.take()) != 1 {
					t.Fatal("direct request bypassed task guard", pair, finished, instance)
				}
				if s.onEnterDungeon(EnterDungeon{ID: 1, MapID: pair[0]}) {
					t.Fatal("unauthorized explicit EnterDungeon", pair)
				}
			}
		}
	}
}

func TestQuestDungeonTransportRechecksWithoutIndexRemap(t *testing.T) {
	for _, after := range []int{-1, int(domain.QuestDeliver), int(domain.QuestFinished)} {
		s := newTestScene(t, newFakeSaver())
		sink := join(t, s, 1, 100, "menu", 100, 100)
		p := s.players[1]
		p.Pos.MapID = s.id.MapID
		p.Player.Char.Quests = domain.QuestLog{359: {State: domain.QuestActive}}
		def := domain.DungeonDef{ID: 199, Solo: true, Name: "月牙塔", Enter: domain.Pos{MapID: 29900, X: 100, Y: 100}, Exit: domain.Pos{MapID: s.id.MapID, X: 100, Y: 100}}
		s.dungeons = domain.DungeonTable{29900: def}
		s.transports = domain.TransportTable{110: {ID: 110, SourceMapID: s.id.MapID, Destinations: []domain.TransportDestination{
			{Index: 0, Name: "ordinary", Pos: domain.Pos{MapID: 24400, X: 100, Y: 100}, Dungeon: true},
			{Index: 1, Name: "task", Pos: def.Enter, Dungeon: true},
			{Index: 2, Name: "unrelated", Pos: domain.Pos{MapID: 26800, X: 100, Y: 100}, Dungeon: true},
		}}}
		s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, NPC: &entity.NPCInfo{Trans: 110}}
		r := NewRouter(func(id domain.SceneID) (*Scene, error) {
			return New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: make(chan time.Time), Dungeons: s.dungeons, Dungeon: &def}), nil
		}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		r.Start(ctx)
		t.Cleanup(cancel)
		s.router = r
		s.onOpenTransport(OpenTransport{ID: 1, List: 110})
		s.flush()
		menu, ok := firstOf[event.TransportDestinations](sink.take())
		if !ok || len(menu.Destinations) != 3 || menu.Destinations[1].Index != 1 {
			t.Fatal("wire rows filtered or shifted", menu)
		}
		if after < 0 {
			p.Player.Char.Quests.Abandon(359)
		} else {
			p.Player.Char.Quests[359] = domain.QuestEntry{State: domain.QuestState(after)}
		}
		s.onChooseTransport(ChooseTransport{ID: 1, List: 110, Index: 1})
		allowed := after == int(domain.QuestDeliver)
		if (s.players[1] == nil) != allowed {
			t.Fatal("selection did not recheck original target against current state", after)
		}
		if !allowed && p.Player.TransportList != 0 {
			t.Fatal("rejected choice was not one-shot")
		}
	}
}

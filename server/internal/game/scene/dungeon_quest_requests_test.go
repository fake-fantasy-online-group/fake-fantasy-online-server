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
	for _, pair := range [][2]int32{{29200, 355}, {29400, 356}, {29900, 359}, {30000, 361}} {
		for _, finished := range []bool{false, true} {
			for _, instance := range []uint32{0, 123} {
				s := newTestScene(t, newFakeSaver())
				sink := join(t, s, 1, 100, "direct", 100, 100)
				p := s.players[1]
				p.Pos.MapID = s.id.MapID
				def := domain.DungeonDef{ID: domain.DungeonID(pair[0]), Solo: true, Enter: domain.Pos{MapID: pair[0], X: 100, Y: 100}, Exit: p.Pos}
				s.dungeons = domain.DungeonTable{pair[0]: def}
				r := NewRouter(func(id domain.SceneID) (*Scene, error) {
					return New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: make(chan time.Time), Dungeons: s.dungeons, Dungeon: &def}), nil
				}, nil)
				ctx, cancel := context.WithCancel(context.Background())
				r.Start(ctx)
				t.Cleanup(cancel)
				s.router = r
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
				s.flush()
				sink.take()
				// Native clients may already have switched locally before 0x100a.
				// The rejection must restore the authoritative source, not leave
				// the character on a task-only map or recurse through the gate.
				if !s.onTeleport(Teleport{ID: 1, To: domain.SceneID{MapID: pair[0]}, At: def.Enter, ClientMapReady: true}) || s.players[1] != p || p.Pos != before {
					t.Fatal("unauthorized map-ready failed to restore source", pair)
				}
			}
		}
	}
}

func TestQuestDungeonTransportRechecksWithoutIndexRemap(t *testing.T) {
	for _, after := range []int{-1, int(domain.QuestDeliver), int(domain.QuestFinished)} {
		s := newTestScene(t, newFakeSaver())
		s.id = domain.SceneID{MapID: 32}
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
		if !ok || len(menu.Destinations) != 5 || menu.Destinations[1].Index != 1 || menu.Destinations[3].Pos.MapID != 24400 || menu.Destinations[4].Pos.MapID != 26800 || menu.Destinations[3].Difficulty != domain.DungeonHard {
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

// A story quest mentioning a repeatable equipment dungeon is not an admission
// rule. An older completed character can still lead a newcomer with no quest.
func TestQuestDungeonPolicyKeepsFarmPartyCarryWithoutQuest(t *testing.T) {
	rig := &dungeonRig{fr: map[domain.SceneID]chan time.Time{}}
	def := domain.DungeonDef{ID: 144, Name: "天魔洞窟之九天蛰眠之龙", Enter: domain.Pos{MapID: 24400, X: 100, Y: 100}, Exit: domain.Pos{MapID: 32, X: 100, Y: 100}}
	table := domain.DungeonTable{24400: def}
	roster := func(id domain.CharID) domain.DungeonParty {
		if id != 100 && id != 200 {
			return domain.DungeonParty{}
		}
		return domain.DungeonParty{ID: 9, Leader: 100, MaxLevel: 100, Members: []string{"mentor", "newcomer"}}
	}
	rig.r = NewRouter(func(id domain.SceneID) (*Scene, error) {
		frames := make(chan time.Time)
		rig.put(id, frames)
		cfg := Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: frames, Dungeons: table, DungeonParty: roster, Alloc: domain.NewEntityAlloc()}
		if !id.Persistent() {
			cfg.Dungeon = &def
		}
		return New(cfg), nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	rig.r.Start(ctx)
	t.Cleanup(cancel)
	town, _ := rig.r.Ensure(domain.SceneID{MapID: 32})
	for _, member := range []struct {
		id    domain.EntityID
		char  int64
		level int32
		name  string
		log   domain.QuestLog
	}{{1, 100, 100, "mentor", domain.QuestLog{309: {State: domain.QuestFinished}}}, {2, 200, 30, "newcomer", domain.QuestLog{}}} {
		town.Post(Enter{ID: member.id, Bag: domain.NewBag(10), Sink: &fakeSink{}, Party: 9, Char: &domain.Character{ID: member.char, Name: member.name, Level: member.level, Pos: def.Exit, Quests: member.log}})
	}
	rig.step(t, town, func(*Scene) {})
	town.Post(EnterDungeon{ID: 1, MapID: 24400})
	rig.step(t, town, func(*Scene) {})
	ids := rig.instances(24400)
	if len(ids) != 1 {
		t.Fatal("completed mentor cannot create normal farm instance", ids)
	}
	dungeon, _ := rig.r.Ensure(ids[0])
	rig.step(t, dungeon, func(s *Scene) {
		if s.players[1] == nil {
			t.Fatal("mentor never admitted")
		}
	})
	town.Post(EnterDungeon{ID: 2, MapID: 24400})
	rig.step(t, town, func(*Scene) {})
	rig.step(t, dungeon, func(s *Scene) {
		if s.players[2] == nil || s.players[1] == nil {
			t.Fatal("newcomer without related quest cannot join completed mentor")
		}
	})
	if len(rig.instances(24400)) != 1 {
		t.Fatal("party split into separate farm instances")
	}
}

func TestQuestDungeonRetained355ReceiptUsesRealSavedStates(t *testing.T) {
	for _, state := range []int{-1, 0, 1, 2, 3} {
		s := newTestScene(t, newFakeSaver())
		sink := join(t, s, 1, 100, "retained", 100, 100)
		p := s.players[1]
		s.quests = domain.QuestTable{} // The disabled definition was removed, not the log.
		p.Player.Char.Quests = domain.QuestLog{}
		if state >= 0 {
			p.Player.Char.Quests[355] = domain.QuestEntry{State: domain.QuestState(state)}
		}
		sink.take()
		s.pushQuestLog(p)
		s.flush()
		evt, ok := firstOf[event.ActiveQuests](sink.take())
		if !ok {
			t.Fatal("missing native task receipt")
		}
		allowed := state == 0 || state == 1
		if (len(evt.List) == 1) != allowed || domain.CanEnterQuestDungeon(29200, p.Player.Char.Quests) != allowed {
			t.Fatal("receipt/admission disagreement", state, evt)
		}
		if allowed && (evt.List[0].ID != 355 || evt.List[0].Ready != (state == 1)) {
			t.Fatal("changed saved task state", evt)
		}
		if (len(evt.Done) == 1) != (state == 2) {
			t.Fatal("finished history omitted or forged", state, evt)
		}
		before, existed := p.Player.Char.Quests[355]
		s.onAcceptQuest(AcceptQuest{ID: p.ID, Quest: 355})
		s.onCompleteQuest(CompleteQuest{ID: p.ID, Quest: 355})
		s.flush()
		if countOf[event.Rejected](sink.take()) != 2 {
			t.Fatal("receipt projection restored a disabled accept/reward path", state)
		}
		after, remains := p.Player.Char.Quests[355]
		if existed != remains || before.State != after.State || before.Progress != after.Progress {
			t.Fatal("disabled accept/complete mutated the saved record", state)
		}
	}
}

func TestQuestDungeonMixedListKeepsOriginalAppendedHardIndex(t *testing.T) {
	for _, chosen := range []struct{ index, target int32 }{{3, 24400}, {4, 26800}} {
		rig := &dungeonRig{fr: map[domain.SceneID]chan time.Time{}}
		defs := domain.DungeonTable{}
		for _, mid := range []int32{24400, 29900, 26800} {
			defs[mid] = domain.DungeonDef{ID: domain.DungeonID(mid/100 - 100), Solo: mid == 29900, Name: "mixed", Enter: domain.Pos{MapID: mid, X: 500, Y: 500}, Exit: domain.Pos{MapID: 32, X: 100, Y: 100}, TimeoutSec: 900}
		}
		destinations := []domain.TransportDestination{}
		for i, mid := range []int32{24400, 29900, 26800} {
			destinations = append(destinations, domain.TransportDestination{Index: int32(i), Name: "mixed", Pos: defs[mid].Enter, Dungeon: true})
		}
		lists := domain.TransportTable{110: {ID: 110, SourceMapID: 32, Destinations: destinations}}
		rig.r = NewRouter(func(id domain.SceneID) (*Scene, error) {
			frames := make(chan time.Time)
			rig.put(id, frames)
			cfg := Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: frames, Dungeons: defs, Transports: lists, Alloc: domain.NewEntityAlloc()}
			if id.Persistent() {
				cfg.NPCs = []domain.NPCSpawn{{Name: "entry", Sprite: 1, Trans: 110, Pos: domain.Pos{MapID: 32, X: 100, Y: 100}}}
			} else {
				def := defs[id.MapID]
				cfg.Dungeon = &def
			}
			return New(cfg), nil
		}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		rig.r.Start(ctx)
		t.Cleanup(cancel)
		town, _ := rig.r.Ensure(domain.SceneID{MapID: 32})
		sink := &fakeSink{}
		town.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: sink, Party: 1, Char: &domain.Character{ID: 100, Name: "selector", Pos: domain.Pos{MapID: 32, X: 100, Y: 100}, Quests: domain.QuestLog{}}})
		rig.step(t, town, func(*Scene) {})
		// Task row1 is not eligible, but remains in the wire list. Choosing it must
		// reject, never drift into either ordinary row or an appended hard variant.
		town.Post(OpenTransport{ID: 1, List: 110})
		rig.step(t, town, func(*Scene) {})
		town.Post(ChooseTransport{ID: 1, List: 110, Index: 1})
		rig.step(t, town, func(s *Scene) {
			if s.players[1] == nil {
				t.Fatal("ineligible task row admitted")
			}
		})
		if len(rig.instances(24400))+len(rig.instances(29900))+len(rig.instances(26800)) != 0 {
			t.Fatal("task denial drifted into another destination")
		}
		sink.take()
		town.Post(OpenTransport{ID: 1, List: 110})
		rig.step(t, town, func(*Scene) {})
		menu, ok := firstOf[event.TransportDestinations](sink.take())
		if !ok || len(menu.Destinations) != 5 || menu.Destinations[chosen.index].Pos.MapID != chosen.target {
			t.Fatal("original hard indices changed", menu)
		}
		town.Post(ChooseTransport{ID: 1, List: 110, Index: chosen.index})
		rig.step(t, town, func(*Scene) {})
		ids := rig.instances(chosen.target)
		if len(ids) != 1 {
			t.Fatal("original hard index failed", chosen, ids)
		}
		dungeon, _ := rig.r.Ensure(ids[0])
		rig.step(t, dungeon, func(s *Scene) {
			if s.players[1] == nil || s.dungeonDifficulty != domain.DungeonHard {
				t.Fatal("wrong target/difficulty after hidden task row", chosen)
			}
		})
		if len(rig.instances(29900)) != 0 {
			t.Fatal("hard selection entered task map")
		}
	}
}

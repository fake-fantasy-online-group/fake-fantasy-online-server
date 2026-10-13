package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

func TestNativeTrialMenuCommandsReachReadyTerminalState(t *testing.T) {
	var data struct {
		Rules    domain.TrialRules           `json:"rules"`
		Dungeons domain.DungeonTable         `json:"dungeons"`
		NPCs     map[int32][]domain.NPCSpawn `json:"npcs"`
	}
	bytes, err := os.ReadFile("testdata/native_trial_routes.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(bytes, &data); err != nil {
		t.Fatal(err)
	}
	rules := &data.Rules
	// Keep real rule/route identities, but sample one normal and one boss spawn;
	// this is a lifecycle/command test, not a repeated density/balance benchmark.
	rules.NormalCount, rules.BossCount = 1, 1
	var entrance domain.TrialEntrance
	for _, e := range rules.Entrances {
		if e.ClaimMap == 61 {
			entrance = e
		}
	}
	def := data.Dungeons[21100]
	if entrance.EntryMap != 41 || entrance.Price != 5000 || def.Exit != (domain.Pos{MapID: 7, X: 7424, Y: 5922}) {
		t.Fatal("fixture does not match audited production entry/exit")
	}
	var mu sync.Mutex
	frames := map[domain.SceneID]chan time.Time{}
	alloc := domain.NewEntityAlloc()
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 4103, Name: "trial snapshot drop", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Price: 2}
	router := scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		frame := make(chan time.Time)
		mu.Lock()
		frames[id] = frame
		mu.Unlock()
		cfg := scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: frame, Alloc: alloc, Dungeons: data.Dungeons, Trial: rules, Quests: domain.QuestTable{}, Items: map[domain.ItemID]domain.ItemDef{4103: item}, NPCs: data.NPCs[id.MapID]}
		if !id.Persistent() {
			copy := data.Dungeons[id.MapID]
			cfg.Dungeon = &copy
		}
		if len(cfg.NPCs) == 0 {
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 999, Name: "snapshot NPC", Sprite: 630, Pos: domain.Pos{MapID: id.MapID, X: 100, Y: 100}}}
		}
		return scene.New(cfg), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	router.Start(ctx)
	t.Cleanup(func() { cancel(); router.Drain(time.Second) })
	step := func(id domain.SceneID, fn func(*scene.Scene)) {
		t.Helper()
		mu.Lock()
		frame := frames[id]
		mu.Unlock()
		if frame == nil {
			t.Fatalf("no actor %v", id)
		}
		select {
		case frame <- time.Now():
		case <-time.After(time.Second):
			t.Fatalf("actor stopped %v", id)
		}
		done := make(chan struct{})
		if !router.Post(id, scene.Inspect{Fn: fn, Done: done}) {
			t.Fatalf("inspect rejected %v", id)
		}
		select {
		case frame <- time.Now():
		case <-time.After(time.Second):
			t.Fatalf("actor failed to inspect %v", id)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("inspect timed out %v", id)
		}
	}
	origin := domain.SceneID{MapID: entrance.ClaimMap}
	if _, err := router.Ensure(origin); err != nil {
		t.Fatal(err)
	}
	out := &capSink{}
	ch := &domain.Character{ID: 100, Name: "trial command traveler", Level: rules.MinLevel, Pos: domain.Pos{MapID: origin.MapID, X: 100, Y: 100}, Money: domain.Money{Silver: 10}}
	s := &Session{deps: Deps{Router: router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: origin, char: ch, sink: out}
	router.Post(origin, scene.Enter{ID: 1, Char: ch, Party: 9, Sink: &eventSink{out: out, log: slog.Default(), sess: s, observer: 1}})
	step(origin, func(*scene.Scene) {})
	send := func(command string) { s.OnPacket(0x1006, protocol.NewW(0).Str(command).Bytes()[2:]) }
	npc := func(name string) { s.OnPacket(0x1032, protocol.NewW(0).Str(name).Bytes()[2:]) }
	teleportCount := func() int { return mapReviewWireCount(out, protocol.SCTeleport) }
	send("trialgo")
	step(origin, func(*scene.Scene) {})
	if teleportCount() != 0 {
		t.Fatal("trialgo without source NPC context traveled")
	}
	npc(entrance.ClaimNPC)
	step(origin, func(*scene.Scene) {})
	send("trialtask")
	send("trialtask")
	step(origin, func(sc *scene.Scene) {
		if !sc.EntityAt(1).Player.Char.Quests.Active(rules.Quest) {
			t.Fatal("actual trialtask did not accept the task")
		}
	})
	// The real fee rejection must leave source ownership and the task untouched.
	step(origin, func(sc *scene.Scene) { sc.EntityAt(1).Player.Char.Money = domain.Money{} })
	send("trialgo")
	step(origin, func(*scene.Scene) {})
	if teleportCount() != 0 {
		t.Fatal("insufficient trialgo balance still traveled")
	}
	step(origin, func(sc *scene.Scene) { sc.EntityAt(1).Player.Char.Money = domain.Money{Silver: 10} })
	send("trialgo")
	send("trialgo")
	step(origin, func(*scene.Scene) {})
	guard := domain.SceneID{MapID: entrance.EntryMap}
	step(guard, func(*scene.Scene) {})
	if teleportCount() != 1 {
		t.Fatal("duplicate trialgo did not produce one authoritative transfer")
	}
	s.mu.Lock()
	awaiting := s.awaitMapAck
	s.mu.Unlock()
	if !awaiting {
		t.Fatal("trialgo released before first ready")
	}
	s.onMove(nativeReady(guard.MapID, int32(entrance.EntryPos.X), int32(entrance.EntryPos.Y)))
	step(guard, func(sc *scene.Scene) {
		p := sc.EntityAt(1)
		if p.Pos != entrance.EntryPos {
			t.Fatalf("trialgo landing=%v want=%v", p.Pos, entrance.EntryPos)
		}
		if got := p.Player.Char.Money; got != (domain.Money{Silver: 5}) {
			t.Fatalf("trialgo fee charged incorrectly/duplicated: %+v", got)
		}
	})
	assertNativeOwner(t, s, guard)
	send("trial")
	step(guard, func(*scene.Scene) {})
	if teleportCount() != 1 {
		t.Fatal("trial without guard context entered instance")
	}
	npc(entrance.EntryNPC)
	step(guard, func(*scene.Scene) {})
	step(guard, func(sc *scene.Scene) { sc.EntityAt(1).Player.Party = 0 })
	send("trial")
	step(guard, func(*scene.Scene) {})
	if teleportCount() != 1 {
		t.Fatal("trial without party entered")
	}
	step(guard, func(sc *scene.Scene) {
		p := sc.EntityAt(1)
		p.Player.Party = 9
		delete(p.Player.Char.Quests, rules.Quest)
	})
	send("trial")
	step(guard, func(*scene.Scene) {})
	if teleportCount() != 1 {
		t.Fatal("trial without active quest entered")
	}
	step(guard, func(sc *scene.Scene) {
		sc.EntityAt(1).Player.Char.Quests[rules.Quest] = domain.QuestEntry{State: domain.QuestActive}
	})
	send("trial")
	send("trial")
	step(guard, func(*scene.Scene) {})
	s.mu.Lock()
	private := s.scene
	s.mu.Unlock()
	if private.MapID != 21100 || private.Persistent() {
		t.Fatalf("trial did not create correct private owner %v", private)
	}
	step(private, func(*scene.Scene) {})
	if teleportCount() != 2 {
		t.Fatal("trial did not emit exactly one instance teleport")
	}
	// Populate a real peer and drop while the traveler's visibility barrier is
	// closed; the sole first-ready snapshot must include those plus the NPC/monsters.
	router.Post(private, scene.Enter{ID: 2, Char: &domain.Character{ID: 200, Name: "trial witness", Level: rules.MinLevel, Pos: domain.Pos{MapID: private.MapID, X: 100, Y: 100}}, Party: 9, Sink: &nullSink{}})
	step(private, func(sc *scene.Scene) { sc.EntityAt(2).Player.Bag.Set(0, domain.Stack{Item: 4103, Count: 1}) })
	router.Post(private, scene.DropBagItem{ID: 2, Tab: 1, Slot: 0, Item: 4103, At: domain.Pos{MapID: private.MapID, X: 100, Y: 100}})
	step(private, func(*scene.Scene) {})
	before := map[uint16]int{}
	for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
		before[op] = mapReviewWireCount(out, op)
	}
	s.onMove(nativeReady(private.MapID, int32(def.Enter.X)+1, int32(def.Enter.Y)))
	step(private, func(*scene.Scene) {})
	for op, n := range before {
		if mapReviewWireCount(out, op) != n {
			t.Fatalf("wrong trial landing released %04x snapshot", op)
		}
	}
	s.onMove(nativeReady(private.MapID, int32(def.Enter.X), int32(def.Enter.Y)))
	step(private, func(*scene.Scene) {})
	assertNativeOwner(t, s, private)
	for op, n := range before {
		want := 1
		if op == protocol.SCEntitySpawn {
			want = 2
		}
		if got := mapReviewWireCount(out, op) - n; got != want {
			t.Fatalf("trial first-ready %04x delta=%d want=%d", op, got, want)
		}
	}
	send("trialleave")
	send("trialleave")
	step(private, func(*scene.Scene) {})
	exit := domain.SceneID{MapID: def.Exit.MapID}
	step(exit, func(*scene.Scene) {})
	if teleportCount() != 3 {
		t.Fatal("duplicate trialleave did not produce one public exit")
	}
	s.mu.Lock()
	awaiting = s.awaitMapAck
	s.mu.Unlock()
	if !awaiting {
		t.Fatal("trial exit opened before build ready")
	}
	s.onMove(nativeReady(exit.MapID, int32(def.Exit.X), int32(def.Exit.Y)))
	step(exit, func(sc *scene.Scene) {
		if p := sc.EntityAt(1); p == nil || p.Pos != def.Exit {
			t.Fatalf("trial exit lost authoritative landing %v", p)
		}
	})
	assertNativeOwner(t, s, exit)
	send("trialleave")
	step(exit, func(*scene.Scene) {})
	if teleportCount() != 3 {
		t.Fatal("trialleave from public exit spuriously transferred")
	}
}

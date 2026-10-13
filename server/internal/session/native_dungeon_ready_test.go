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
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

func TestNativeDungeonReadyThreeFloorsAndNondefaultReturnLandings(t *testing.T) {
	// Extracted from production LoadDungeons/LoadTeleports on migrations 0..65,
	// not an invented shared landing: the reverse links differ from Enter points.
	var config struct {
		Dungeons domain.DungeonTable         `json:"dungeons"`
		Portals  map[int32][]domain.Teleport `json:"portals"`
	}
	raw, err := os.ReadFile("testdata/native_dungeon_routes.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	frames := map[domain.SceneID]chan time.Time{}
	alloc := domain.NewEntityAlloc()
	router := scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		frame := make(chan time.Time)
		mu.Lock()
		frames[id] = frame
		mu.Unlock()
		cfg := scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: frame, Dungeons: config.Dungeons, Alloc: alloc}
		if !id.Persistent() {
			def := config.Dungeons[id.MapID]
			cfg.Dungeon = &def
		}
		for _, p := range config.Portals[id.MapID] {
			cfg.Portals = append(cfg.Portals, domain.NewPortal(p))
		}
		cfg.NPCs = []domain.NPCSpawn{{NPCID: 1, Name: "floor NPC", Sprite: 630, Pos: domain.Pos{MapID: id.MapID, X: 100, Y: 100}}}
		defs := spawn.MapDefs{1001: domain.MonsterDef{ID: 1001, Name: "floor entity", Kind: domain.MonsterProp, Sprite: 101, Level: 1, HP: 100}}
		cfg.Spawner, _ = spawn.New([]domain.SpawnPoint{{ID: 1, Monster: 1001, Pos: domain.Pos{MapID: id.MapID, X: 140, Y: 140}}}, defs, alloc)
		cfg.Defs = defs
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
			t.Fatalf("actor did not advance %v", id)
		}
		done := make(chan struct{})
		if !router.Post(id, scene.Inspect{Fn: fn, Done: done}) {
			t.Fatalf("actor rejected inspect %v", id)
		}
		select {
		case frame <- time.Now():
		case <-time.After(time.Second):
			t.Fatalf("actor did not inspect %v", id)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("actor inspection timed out %v", id)
		}
	}
	origin := domain.SceneID{MapID: 32}
	if _, err := router.Ensure(origin); err != nil {
		t.Fatal(err)
	}
	out := &capSink{}
	ch := &domain.Character{ID: 100, Name: "native dungeon traveler", Level: 60, Pos: domain.Pos{MapID: 32, X: 13417, Y: 5020}}
	s := &Session{deps: Deps{Router: router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: origin, char: ch, sink: out}
	router.Post(origin, scene.Enter{ID: 1, Char: ch, Party: 9, Sink: &eventSink{out: out, log: slog.Default(), sess: s, observer: 1}})
	step(origin, func(*scene.Scene) {})
	out.mu.Lock()
	out.pkts = nil
	out.mu.Unlock()
	sequence := []domain.Pos{config.Dungeons[20801].Enter, {MapID: 20802, X: 2098, Y: 3212}, {MapID: 20803, X: 9077, Y: 3414}, {MapID: 20802, X: 6348, Y: 3200}, {MapID: 20801, X: 3274, Y: 4073}, {MapID: 32, X: 13335, Y: 4921}}
	var instance uint32
	source := origin
	for i, at := range sequence {
		ready := nativeReady(at.MapID, int32(at.X), int32(at.Y))
		s.onMove(ready)
		s.onMove(ready)
		step(source, func(*scene.Scene) {})
		s.mu.Lock()
		target := s.scene
		s.mu.Unlock()
		if at.MapID != 32 {
			if target.Persistent() {
				t.Fatalf("floor %d lost private ownership", at.MapID)
			}
			if instance == 0 {
				instance = target.Instance
			} else if instance != target.Instance {
				t.Fatalf("floor %d changed instance %d to %d", at.MapID, instance, target.Instance)
			}
		} else if !target.Persistent() {
			t.Fatal("exit retained private instance")
		}
		step(target, func(sc *scene.Scene) {
			if e := sc.EntityAt(1); e == nil || e.Pos != at {
				t.Fatalf("step%d target=%v got=%v want=%v", i, target, e, at)
			}
		})
		assertNativeOwner(t, s, target)
		if got := mapReviewWireCount(out, protocol.SCTeleport); got != 0 {
			t.Fatalf("native floor step%d emitted %d teleports", i, got)
		}
		for _, op := range []uint16{protocol.SCNPCSpawn, protocol.SCEntitySpawn} {
			if got := mapReviewWireCount(out, op); got != i+1 {
				t.Fatalf("step%d snapshot %04x=%d", i, op, got)
			}
		}
		source = target
	}
}

package session

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// Real Session/Router regressions: source and destination actors advance only on explicit frames.
func mapReviewWireCount(sink *capSink, op uint16) int {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	n := 0
	for _, p := range sink.pkts {
		if len(p) >= 2 && binary.LittleEndian.Uint16(p) == op {
			n++
		}
	}
	return n
}
func mapReviewSession(t *testing.T) (*twoMapRig, *Session, *capSink) {
	r := newTwoMapRig(t)
	r.enter(1, 1, 100, "source")
	r.enter(285, 2, 200, "target witness")
	sink := &capSink{}
	s := &Session{deps: Deps{Router: r.router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 1}, char: &domain.Character{ID: 100, Name: "source"}, sink: sink}
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
		sc.EntityAt(1).Player.Sink = &eventSink{out: sink, log: slog.Default(), sess: s, observer: 1}
	}, Done: make(chan struct{})})
	r.step(1)
	return r, s, sink
}
func TestClientMapReadyClientFirstSingleReady(t *testing.T) {
	r, s, sink := mapReviewSession(t)
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(1)
	r.step(285)
	s.mu.Lock()
	waiting := s.awaitMapAck
	s.mu.Unlock()
	tp, entities := mapReviewWireCount(sink, protocol.SCTeleport), mapReviewWireCount(sink, protocol.SCRemotePlayer)
	t.Logf("client-first single ready: Teleport=%d target entity batches=%d waiting=%v", tp, entities, waiting)
	if waiting || tp != 0 || entities != 1 {
		t.Fatal("normal client-first did not complete exactly once")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(285)
	t.Logf("after duplicate entered=1: target entity batches=%d", mapReviewWireCount(sink, protocol.SCRemotePlayer))
	if mapReviewWireCount(sink, protocol.SCRemotePlayer) != 2 {
		t.Fatal("expected existing same-map ready to cause redundant snapshot")
	}
}
func TestClientMapReadyServerFirstSingleReady(t *testing.T) {
	r, s, sink := mapReviewSession(t)
	r.router.Post(domain.SceneID{MapID: 1}, scene.Teleport{ID: 1, To: domain.SceneID{MapID: 285}, At: domain.Pos{MapID: 285, X: 100, Y: 100}})
	r.step(1)
	r.step(285)
	if mapReviewWireCount(sink, protocol.SCTeleport) != 1 || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 0 {
		t.Fatal("server-first must emit one teleport before any target entities")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(285)
	s.mu.Lock()
	waiting := s.awaitMapAck
	s.mu.Unlock()
	t.Logf("server-first single ready: Teleport=%d target entity batches=%d waiting=%v", mapReviewWireCount(sink, protocol.SCTeleport), mapReviewWireCount(sink, protocol.SCRemotePlayer), waiting)
	if waiting || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 1 {
		t.Fatal("first ready did not release sole first snapshot")
	}
}
func TestClientMapReadyClientReadyRacesQueuedServerTeleport(t *testing.T) {
	r, s, sink := mapReviewSession(t)
	// Existing source-scene teleport is queued but not executed. This is the same queue
	// ordering produced by an earlier movement/NPC command that starts transfer.
	r.router.Post(domain.SceneID{MapID: 1}, scene.Teleport{ID: 1, To: domain.SceneID{MapID: 285}, At: domain.Pos{MapID: 285, X: 100, Y: 100}})
	// Session still owns map 1, so this is posted to map 1 as ClientMapReady Teleport.
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(1)
	r.step(285)
	s.mu.Lock()
	waiting := s.awaitMapAck
	s.mu.Unlock()
	t.Logf("queued-server/client-ready collision after first ready: Teleport=%d target entity batches=%d waiting=%v", mapReviewWireCount(sink, protocol.SCTeleport), mapReviewWireCount(sink, protocol.SCRemotePlayer), waiting)
	if !waiting || mapReviewWireCount(sink, protocol.SCTeleport) != 1 || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 0 {
		t.Fatal("expected first ready to have been posted to retired source owner")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(285)
	s.mu.Lock()
	waiting = s.awaitMapAck
	s.mu.Unlock()
	t.Logf("after required late-teleport ready: Teleport=%d target entity batches=%d waiting=%v", mapReviewWireCount(sink, protocol.SCTeleport), mapReviewWireCount(sink, protocol.SCRemotePlayer), waiting)
	if waiting || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 1 {
		t.Fatal("late ready must release the first, not second, snapshot")
	}
}

func TestClientMapReadyPortalMoveThenClientReady(t *testing.T) {
	r := &twoMapRig{t: t, frames: map[int32]chan time.Time{1: make(chan time.Time), 285: make(chan time.Time)}}
	r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		cfg := scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: r.frames[id.MapID]}
		if id.MapID == 1 {
			cfg.Portals = []domain.Portal{domain.NewPortal(domain.Teleport{ProcID: 1, To: domain.Pos{MapID: 285, X: 100, Y: 100}, Area: domain.Polygon{{X: 115, Y: 105}, {X: 125, Y: 105}, {X: 125, Y: 115}, {X: 115, Y: 115}}})}
		}
		return scene.New(cfg), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	r.router.Start(ctx)
	t.Cleanup(func() { cancel(); r.router.Drain(time.Second) })
	r.enter(1, 1, 100, "source")
	r.enter(285, 2, 200, "target witness")
	sink := &capSink{}
	s := &Session{deps: Deps{Router: r.router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 1}, char: &domain.Character{ID: 100, Name: "source"}, sink: sink}
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
		sc.EntityAt(1).Player.Sink = &eventSink{out: sink, log: slog.Default(), sess: s, observer: 1}
	}, Done: make(chan struct{})})
	r.step(1)
	// Both actual network handlers execute before the source scene's next frame.
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(1)
	r.step(285)
	s.mu.Lock()
	waiting := s.awaitMapAck
	s.mu.Unlock()
	t.Logf("real portal movement then first ready: Teleport=%d target entity batches=%d waiting=%v", mapReviewWireCount(sink, protocol.SCTeleport), mapReviewWireCount(sink, protocol.SCRemotePlayer), waiting)
	if waiting || mapReviewWireCount(sink, protocol.SCTeleport) != 0 || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 1 {
		t.Fatal("first ready was lost or client was asked to rebuild an already built portal target")
	}
	s.mu.Lock()
	receipt := s.clientMapReady
	s.mu.Unlock()
	if receipt != nil {
		t.Fatal("portal handoff retained a reusable source receipt")
	}
}

func TestClientMapReadyReceiptExactSourceAndTarget(t *testing.T) {
	source := domain.SceneID{MapID: 1, Instance: 17}
	target := domain.SceneID{MapID: 285}
	landing := domain.Pos{MapID: 285, X: 100, Y: 200}
	for _, tc := range []struct {
		name           string
		edit           func(*Session, *domain.SceneID, *domain.SceneID, *domain.Pos, *domain.EntityID, *bool)
		ready, handoff bool
	}{
		{name: "matching ordinary portal", ready: true, handoff: true},
		{name: "unrelated same target NPC transfer", edit: func(_ *Session, _ *domain.SceneID, _ *domain.SceneID, _ *domain.Pos, _ *domain.EntityID, p *bool) {
			*p = false
		}, handoff: true},
		{name: "other source instance", edit: func(_ *Session, f *domain.SceneID, _ *domain.SceneID, _ *domain.Pos, _ *domain.EntityID, _ *bool) {
			f.Instance++
		}},
		{name: "other entity", edit: func(_ *Session, _ *domain.SceneID, _ *domain.SceneID, _ *domain.Pos, id *domain.EntityID, _ *bool) {
			*id = 99
		}},
		{name: "source epoch replaced", edit: func(s *Session, _ *domain.SceneID, _ *domain.SceneID, _ *domain.Pos, _ *domain.EntityID, _ *bool) {
			s.mapEpoch++
		}, handoff: true},
		{name: "private target instance", edit: func(_ *Session, _ *domain.SceneID, to *domain.SceneID, _ *domain.Pos, _ *domain.EntityID, _ *bool) {
			to.Instance = 18
		}, handoff: true},
		{name: "different destination", edit: func(_ *Session, _ *domain.SceneID, to *domain.SceneID, at *domain.Pos, _ *domain.EntityID, _ *bool) {
			to.MapID = 3
			at.MapID = 3
		}, handoff: true},
		{name: "landing correction", edit: func(_ *Session, _ *domain.SceneID, _ *domain.SceneID, at *domain.Pos, _ *domain.EntityID, _ *bool) {
			at.X++
		}, handoff: true},
		{name: "fractional landing not rounded into confirmation", edit: func(_ *Session, _ *domain.SceneID, _ *domain.SceneID, at *domain.Pos, _ *domain.EntityID, _ *bool) {
			at.Y += 0.25
		}, handoff: true},
		{name: "same logical map different instance", edit: func(_ *Session, _ *domain.SceneID, to *domain.SceneID, at *domain.Pos, _ *domain.EntityID, _ *bool) {
			to.MapID = 1
			to.Instance = 18
			at.MapID = 1
		}, handoff: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Session{stage: StageInGame, entity: 1, scene: source, mapEpoch: 7, log: slog.Default()}
			s.registerClientMapReadyLocked(1, source, 285, 100, 200)
			from, to, at, id, portal := source, target, landing, domain.EntityID(1), true
			if tc.edit != nil {
				tc.edit(s, &from, &to, &at, &id, &portal)
			}
			called, gotReady := false, false
			ok := s.transferSceneWithClientReady(id, from, to, at, portal, 0, false, func(ready bool) bool { called = true; gotReady = ready; return true })
			if ok != tc.handoff || called != tc.handoff || gotReady != tc.ready {
				t.Fatalf("handoff=%v called=%v ready=%v", ok, called, gotReady)
			}
			if called && s.clientMapReady != nil {
				t.Fatal("handoff left receipt reusable")
			}
		})
	}
}

func TestClientMapReadyFailureAndCancellationRetireReceipt(t *testing.T) {
	source, target := domain.SceneID{MapID: 1}, domain.SceneID{MapID: 285}
	landing := domain.Pos{MapID: 285, X: 100, Y: 200}
	t.Run("failed atomic handoff", func(t *testing.T) {
		s := &Session{stage: StageInGame, entity: 1, scene: source, mapEpoch: 7, log: slog.Default()}
		serial := s.registerClientMapReadyLocked(1, source, 285, 100, 200)
		sink := &eventSink{sess: s}
		if s.transferSceneWithClientReady(1, source, target, landing, true, 0, false, func(ready bool) bool {
			if !ready {
				t.Fatal("fixture did not match")
			}
			return false
		}) {
			t.Fatal("failed handoff succeeded")
		}
		if s.scene != source || s.mapEpoch != 7 || s.awaitMapAck || s.clientMapReady != nil || sink.ClientMapReadyCurrent(1, source, serial) {
			t.Fatal("failed handoff altered ownership or retained receipt")
		}
	})
	t.Run("same scene authoritative load fences queued ready", func(t *testing.T) {
		s := &Session{stage: StageInGame, entity: 1, scene: source, mapEpoch: 7}
		serial := s.registerClientMapReadyLocked(1, source, 285, 100, 200)
		sink := &eventSink{sess: s}
		epoch, ok := sink.BeginMapLoad(1, source)
		if !ok || epoch != 8 || s.clientMapReady != nil || sink.ClientMapReadyCurrent(1, source, serial) {
			t.Fatal("new load did not invalidate source ready")
		}
	})
	t.Run("newer receipt survives older retire", func(t *testing.T) {
		s := &Session{stage: StageInGame, entity: 1, scene: source, mapEpoch: 7}
		old := s.registerClientMapReadyLocked(1, source, 285, 100, 200)
		current := s.registerClientMapReadyLocked(1, source, 3, 100, 200)
		s.retireClientMapReady(old)
		if old == current || s.clientMapReady == nil || s.clientMapReady.serial != current {
			t.Fatal("old command retired newer ready")
		}
	})
	t.Run("disconnect clears ready", func(t *testing.T) {
		r, s, _ := mapReviewSession(t)
		s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
		s.OnClose("test disconnect before source frame")
		r.step(1)
		s.mu.Lock()
		pending, stage := s.clientMapReady, s.stage
		s.mu.Unlock()
		if pending != nil || stage != StageClosed {
			t.Fatal("disconnect retained a receipt")
		}
		if got := r.posOf(1, 1); got.MapID != 0 {
			t.Fatal("disconnected source entity survived")
		}
	})
}

func TestClientMapReadyCannotOpenFollowingTransfer(t *testing.T) {
	source, target := domain.SceneID{MapID: 1}, domain.SceneID{MapID: 285}
	s := &Session{stage: StageInGame, entity: 1, scene: source, mapEpoch: 7, log: slog.Default()}
	s.registerClientMapReadyLocked(1, source, 285, 100, 200)
	first := false
	if !s.transferSceneWithClientReady(1, source, target, domain.Pos{MapID: 285, X: 100, Y: 200}, true, 0, false, func(ready bool) bool { first = ready; return true }) || !first {
		t.Fatal("first transfer did not inherit ready")
	}
	sink := &eventSink{sess: s}
	if !sink.CompleteMapLoad(1, target, 8) {
		t.Fatal("could not settle first target")
	}
	secondReady := true
	if !s.transferSceneWithClientReady(1, target, source, domain.Pos{MapID: 1, X: 100, Y: 200}, true, 0, false, func(ready bool) bool { secondReady = ready; return true }) || secondReady || !s.awaitMapAck || s.mapEpoch != 9 {
		t.Fatal("first receipt opened the following transfer")
	}
}

func TestClientMapReadyFinalCommitRevalidatesDirectReceipt(t *testing.T) {
	from, to := domain.SceneID{MapID: 1}, domain.SceneID{MapID: 285}
	s := &Session{stage: StageInGame, entity: 1, scene: from, mapEpoch: 7, log: slog.Default()}
	serial := s.registerClientMapReadyLocked(1, from, 285, 100, 200)
	sink := &eventSink{sess: s}
	if !sink.ClientMapReadyCurrent(1, from, serial) {
		t.Fatal("initial actor validation failed")
	}
	// Network receipt replacement occurs after actor validation but before commit.
	s.registerClientMapReadyLocked(1, from, 3, 300, 400)
	called := false
	if s.transferSceneWithClientReady(1, from, to, domain.Pos{MapID: 285, X: 100, Y: 200}, false, serial, true, func(bool) bool { called = true; return true }) || called {
		t.Fatal("stale direct-command flag bypassed final ownership validation")
	}
	if s.scene != from || s.mapEpoch != 7 || s.awaitMapAck || s.clientMapReady != nil {
		t.Fatal("rejected commit changed ownership or left a reusable receipt")
	}
}

func readyPortalFixture(t *testing.T, populated, failTarget bool) (*twoMapRig, *Session, *capSink) {
	return readyPortalFixtureWithMailboxes(t, populated, failTarget, 0, 0)
}

func readyPortalFixtureWithMailboxes(t *testing.T, populated, failTarget bool, sourceMailbox, targetMailbox int) (*twoMapRig, *Session, *capSink) {
	t.Helper()
	r := &twoMapRig{t: t, frames: map[int32]chan time.Time{1: make(chan time.Time), 285: make(chan time.Time)}}
	alloc := domain.NewEntityAlloc()
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 4103, Name: "receipt-test drop", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Price: 2}
	r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		if failTarget && id.MapID == 285 {
			return nil, errors.New("forced target construction failure")
		}
		cfg := scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: r.frames[id.MapID], Alloc: alloc, Items: map[domain.ItemID]domain.ItemDef{item.ID: item}}
		cfg.Mailbox = targetMailbox
		if id.MapID == 1 {
			cfg.Mailbox = sourceMailbox
			cfg.Portals = []domain.Portal{domain.NewPortal(domain.Teleport{ProcID: 1, To: domain.Pos{MapID: 285, X: 100, Y: 100}, Area: domain.Polygon{{X: 115, Y: 105}, {X: 125, Y: 105}, {X: 125, Y: 115}, {X: 115, Y: 115}}})}
		}
		if populated && id.MapID == 285 {
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 1, Name: "receipt-test npc", Sprite: 630, Script: "test", Pos: domain.Pos{MapID: 285, X: 120, Y: 120}}}
			defs := spawn.MapDefs{1001: domain.MonsterDef{ID: 1001, Name: "receipt-test monster", Kind: domain.MonsterProp, Sprite: 101, Level: 1, HP: 100}}
			cfg.Spawner, _ = spawn.New([]domain.SpawnPoint{{ID: 1, Monster: 1001, Pos: domain.Pos{MapID: 285, X: 140, Y: 140}}}, defs, alloc)
			cfg.Defs = defs
		}
		return scene.New(cfg), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	r.router.Start(ctx)
	t.Cleanup(func() { cancel(); r.router.Drain(time.Second) })
	r.enter(1, 1, 100, "source")
	if !failTarget {
		if populated {
			r.enter(285, 2, 200, "target witness")
			r.router.Post(domain.SceneID{MapID: 285}, scene.Inspect{Fn: func(sc *scene.Scene) { sc.EntityAt(2).Player.Bag.Set(0, domain.Stack{Item: item.ID, Count: 1}) }, Done: make(chan struct{})})
			r.step(285)
			r.router.Post(domain.SceneID{MapID: 285}, scene.DropBagItem{ID: 2, Tab: 1, Slot: 0, Item: item.ID, At: domain.Pos{MapID: 285, X: 100, Y: 100}})
			r.step(285)
		} else if _, err := r.router.Ensure(domain.SceneID{MapID: 285}); err != nil {
			t.Fatal(err)
		}
	}
	sink := &capSink{}
	s := &Session{deps: Deps{Router: r.router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 1}, char: &domain.Character{ID: 100, Name: "source"}, sink: sink}
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
		sc.EntityAt(1).Player.Sink = &eventSink{out: sink, log: slog.Default(), sess: s, observer: 1}
	}, Done: make(chan struct{})})
	r.step(1)
	return r, s, sink
}

func TestClientMapReadyPortalFirstSnapshotContainsConfiguredVisibleKinds(t *testing.T) {
	r, s, sink := readyPortalFixture(t, true, false)
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	ready := protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100}
	s.onMove(ready)
	s.onMove(ready) // duplicate before the same source command is consumed
	r.step(1)
	r.step(285)
	for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
		if got := mapReviewWireCount(sink, op); got != 1 {
			t.Fatalf("snapshot opcode %04x count=%d, want exactly one", op, got)
		}
	}
	if got := mapReviewWireCount(sink, protocol.SCTeleport); got != 0 {
		t.Fatalf("prebuilt target was asked to rebuild: %d", got)
	}
	if got := r.posOf(1, 1); got.MapID != 0 {
		t.Fatal("source still owns transferred player")
	}
	if got := r.posOf(285, 1); got.MapID != 285 || got.X != 100 || got.Y != 100 {
		t.Fatalf("target ownership/landing: %+v", got)
	}
	s.mu.Lock()
	pending, awaiting := s.clientMapReady, s.awaitMapAck
	s.mu.Unlock()
	if pending != nil || awaiting {
		t.Fatal("first snapshot did not complete source receipt and target barrier")
	}
	// Advancing again with no extra ready cannot trigger a hidden resynchronization.
	r.step(285)
	for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
		if got := mapReviewWireCount(sink, op); got != 1 {
			t.Fatalf("unexpected later refresh %04x count=%d", op, got)
		}
	}
}

func TestClientMapReadyPortalEmptyTargetNeedsNoSecondReady(t *testing.T) {
	r, s, sink := readyPortalFixture(t, false, false)
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(1)
	r.step(285)
	s.mu.Lock()
	waiting := s.awaitMapAck
	s.mu.Unlock()
	if waiting || mapReviewWireCount(sink, protocol.SCTeleport) != 0 || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 0 {
		t.Fatal("empty target was left waiting or unnecessarily teleported")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 110, Y: 110})
	r.step(285)
	if pos := r.posOf(285, 1); pos.X != 110 || pos.Y != 110 {
		t.Fatal("empty target did not restore normal movement")
	}
}

func TestClientMapReadyPortalCorrectedLandingRetiresReceipt(t *testing.T) {
	r, s, sink := readyPortalFixture(t, false, false)
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 101, Y: 100})
	r.step(1)
	r.step(285)
	s.mu.Lock()
	waiting, pending := s.awaitMapAck, s.clientMapReady
	s.mu.Unlock()
	if !waiting || pending != nil || mapReviewWireCount(sink, protocol.SCTeleport) != 1 {
		t.Fatal("mismatched landing confirmed an authoritative correction")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(285)
	s.mu.Lock()
	waiting = s.awaitMapAck
	s.mu.Unlock()
	if waiting {
		t.Fatal("corrected target could not be acknowledged normally")
	}
}

func TestClientMapReadyPortalFailedTargetClosesWithoutReusableReceipt(t *testing.T) {
	r, s, sink := readyPortalFixture(t, false, true)
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(1)
	sink.mu.Lock()
	closed := sink.closed
	sink.mu.Unlock()
	s.mu.Lock()
	pending, sc, waiting := s.clientMapReady, s.scene, s.awaitMapAck
	s.mu.Unlock()
	if !closed || pending != nil || sc.MapID != 1 || waiting {
		t.Fatal("failed target did not preserve source save binding and retire ready")
	}
	if got := r.posOf(1, 1); got.MapID != 0 {
		t.Fatal("failed handoff left mutable player in source")
	}
}

func TestClientMapReadyDisconnectBetweenHandoffAndTargetEnter(t *testing.T) {
	r, s, _ := readyPortalFixture(t, false, false)
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(1)
	s.OnClose("disconnect after handoff before target Enter")
	r.step(285)
	if got := r.posOf(285, 1); got.MapID != 0 {
		t.Fatal("target retained disconnected transferred player")
	}
	s.mu.Lock()
	pending, stage := s.clientMapReady, s.stage
	s.mu.Unlock()
	if pending != nil || stage != StageClosed {
		t.Fatal("closed connection retained pending source ready")
	}
}

func TestClientMapReadySameMapNewInstanceNeverInheritsCrossMapReceipt(t *testing.T) {
	source, target := domain.SceneID{MapID: 1}, domain.SceneID{MapID: 1, Instance: 9}
	frames := map[domain.SceneID]chan time.Time{source: make(chan time.Time), target: make(chan time.Time)}
	router := scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		ch, ok := frames[id]
		if !ok {
			return nil, errors.New("unexpected scene")
		}
		return scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: ch}), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	router.Start(ctx)
	t.Cleanup(func() { cancel(); router.Drain(time.Second) })
	step := func(sc domain.SceneID) {
		t.Helper()
		frames[sc] <- time.Now()
		done := make(chan struct{})
		if !router.Post(sc, scene.Inspect{Fn: func(*scene.Scene) {}, Done: done}) {
			t.Fatal("post scene barrier")
		}
		frames[sc] <- time.Now()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("scene barrier timed out")
		}
	}
	for _, sc := range []domain.SceneID{source, target} {
		if _, err := router.Ensure(sc); err != nil {
			t.Fatal(err)
		}
	}
	sink := &capSink{}
	s := &Session{deps: Deps{Router: router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: source, char: &domain.Character{ID: 100, Name: "source"}, sink: sink}
	router.Post(source, scene.Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "source", Level: 10, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Sink: &eventSink{out: sink, log: slog.Default(), sess: s, observer: 1}})
	step(source)
	router.Post(target, scene.Enter{ID: 2, Char: &domain.Character{ID: 200, Name: "private witness", Level: 10, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Sink: nullSink{}})
	step(target)
	sink.mu.Lock()
	sink.pkts = nil
	sink.mu.Unlock()
	router.Post(source, scene.Teleport{ID: 1, To: target, At: domain.Pos{MapID: 1, X: 100, Y: 100}})
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	step(source)
	step(target)
	s.mu.Lock()
	waiting, current, pending := s.awaitMapAck, s.scene, s.clientMapReady
	s.mu.Unlock()
	if !waiting || current != target || pending != nil || mapReviewWireCount(sink, protocol.SCTeleport) != 1 || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 0 {
		t.Fatal("cross-map receipt opened same-map new instance")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 1, X: 100, Y: 100})
	step(target)
	s.mu.Lock()
	waiting = s.awaitMapAck
	s.mu.Unlock()
	if waiting || mapReviewWireCount(sink, protocol.SCRemotePlayer) != 1 {
		t.Fatal("new private instance did not receive its own first ready/snapshot")
	}
}

func TestClientMapReadySettledSameMapRebuildKeepsAuthoritativePosition(t *testing.T) {
	r, s, sink := mapReviewSession(t)
	r.enter(1, 3, 300, "same-map witness")
	sink.mu.Lock()
	sink.pkts = nil
	sink.mu.Unlock()
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 1, X: 9999, Y: 9999})
	r.step(1)
	if got := r.posOf(1, 1); got.X != 100 || got.Y != 100 {
		t.Fatal("rebuild confirmation changed server position")
	}
	if mapReviewWireCount(sink, protocol.SCRemotePlayer) != 1 || mapReviewWireCount(sink, protocol.SCTeleport) != 0 {
		t.Fatal("genuine same-map rebuild lost its normal full snapshot")
	}
}

func TestClientMapReadyBetweenHandoffAndTargetEnter(t *testing.T) {
	r, s, sink := readyPortalFixture(t, false, false)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	if !r.router.Post(domain.SceneID{MapID: 285}, scene.Inspect{Fn: func(*scene.Scene) { close(entered); <-release }, Done: finished}) {
		t.Fatal("target block post")
	}
	select {
	case r.frames[285] <- time.Now():
	case <-entered:
	}
	<-entered // target has passed its handoff drain and is executing mailbox work
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	r.step(1) // actual portal hands player to target's handoff queue while target is busy
	s.mu.Lock()
	sc, waiting := s.scene, s.awaitMapAck
	s.mu.Unlock()
	if sc.MapID != 285 || !waiting {
		t.Fatal("source did not hand off")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	close(release)
	<-finished
	r.step(285)
	s.mu.Lock()
	waiting = s.awaitMapAck
	s.mu.Unlock()
	t.Logf("ordered first-ready after handoff/before target Enter: awaiting=%v teleport=%d snapshots=%d", waiting, mapReviewWireCount(sink, protocol.SCTeleport), mapReviewWireCount(sink, protocol.SCRemotePlayer))
	if waiting {
		t.Fatal("first ready was consumed before the pending target Enter and lost")
	}
}

// Network calls remain FIFO in every schedule. Only actor execution is moved
// across those calls, including a target actor already inside its mailbox loop.
func TestClientMapReadyFIFOActorScheduleMatrix(t *testing.T) {
	for _, phase := range []string{"before_source", "after_handoff", "after_target_enter", "inside_target_mailbox"} {
		for _, duplicates := range []bool{false, true} {
			name := phase
			if duplicates {
				name += "_duplicate_before_consumption"
			}
			t.Run(name, func(t *testing.T) {
				r, s, sink := readyPortalFixture(t, true, false)
				var release, blocked, finished chan struct{}
				if phase == "inside_target_mailbox" {
					release, blocked, finished = make(chan struct{}), make(chan struct{}), make(chan struct{})
					r.router.Post(domain.SceneID{MapID: 285}, scene.Inspect{Fn: func(*scene.Scene) { close(blocked); <-release }, Done: finished})
					select {
					case r.frames[285] <- time.Now():
					case <-blocked:
					}
					<-blocked
				}
				s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
				if phase != "before_source" {
					r.step(1)
				}
				if phase == "after_target_enter" {
					r.step(285)
				}
				ready := protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100}
				s.onMove(ready)
				if duplicates {
					s.onMove(ready)
				}
				if phase == "before_source" {
					r.step(1)
				}
				if release != nil {
					close(release)
					<-finished
				}
				r.step(285)
				s.mu.Lock()
				waiting, sc, pending := s.awaitMapAck, s.scene, s.clientMapReady
				s.mu.Unlock()
				if waiting || sc != (domain.SceneID{MapID: 285}) || pending != nil {
					t.Fatalf("ownership not settled: %v %v", sc, waiting)
				}
				for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
					if n := mapReviewWireCount(sink, op); n != 1 {
						t.Fatalf("first entity kind %04x repeated/lost: %d", op, n)
					}
				}
				expectedTeleport := 1
				if phase == "before_source" {
					expectedTeleport = 0
				}
				if n := mapReviewWireCount(sink, protocol.SCTeleport); n != expectedTeleport {
					t.Fatalf("teleport=%d want%d", n, expectedTeleport)
				}
			})
		}
	}
}

func TestClientMapReadySourceMailboxRejectClosesAndRetires(t *testing.T) {
	r, s, sink := readyPortalFixtureWithMailboxes(t, false, false, 2, 0)
	release := blockReadyActor(t, r, 1)
	for i := 0; i < 2; i++ {
		if !r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(*scene.Scene) {}, Done: make(chan struct{})}) {
			t.Fatal("cannot fill source mailbox")
		}
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	sink.mu.Lock()
	closed := sink.closed
	sink.mu.Unlock()
	s.mu.Lock()
	pending, sc := s.clientMapReady, s.scene
	s.mu.Unlock()
	if !closed || pending != nil || sc.MapID != 1 {
		t.Fatal("rejected ready was silently left live")
	}
	release()
	readyStepFullMailbox(t, r, 1)
	s.OnClose("source mailbox rejected ready")
	r.step(1)
	if p := r.posOf(1, 1); p.MapID != 0 {
		t.Fatal("rejected-connection cleanup left player")
	}
}

func TestClientMapReadyTargetFullMailboxCannotOvertakeHandoff(t *testing.T) {
	r, s, sink := readyPortalFixtureWithMailboxes(t, false, false, 0, 2)
	release := blockReadyActor(t, r, 285)
	occupied := make(chan struct{})
	if !r.router.Post(domain.SceneID{MapID: 285}, scene.Inspect{Fn: func(*scene.Scene) {}, Done: make(chan struct{})}) {
		t.Fatal("cannot fill first target slot")
	}
	if !r.router.Post(domain.SceneID{MapID: 285}, scene.Inspect{Fn: func(*scene.Scene) {}, Done: occupied}) {
		t.Fatal("cannot fill target mailbox")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	r.step(1)
	// Target handoff has a distinct non-dropping queue despite its full mailbox.
	done := make(chan struct{})
	go func() {
		s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
		close(done)
	}()
	release()
	readyStepFullMailbox(t, r, 285)
	<-occupied
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ready lifecycle did not gain target mailbox capacity")
	}
	readyStepFullMailbox(t, r, 285)
	s.mu.Lock()
	waiting := s.awaitMapAck
	s.mu.Unlock()
	if waiting || mapReviewWireCount(sink, protocol.SCTeleport) != 1 {
		t.Fatal("full mailbox lost/duplicated native first readiness")
	}
	if p := r.posOf(285, 1); p.MapID != 285 {
		t.Fatal("handoff owner missing behind full mailbox")
	}
}

func TestClientMapReadyOldClosedConnectionCannotAffectReconnectedOwner(t *testing.T) {
	r, old, sink := readyPortalFixture(t, false, false)
	old.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	old.OnClose("close before queued ready consumes source")
	r.step(1)
	// Reconnect uses a fresh connection and fresh entity, as the real allocator does.
	r.enter(1, 10, 100, "reconnected")
	fresh := &Session{deps: old.deps, log: slog.Default(), stage: StageInGame, entity: 10, scene: domain.SceneID{MapID: 1}, char: &domain.Character{ID: 100, Name: "reconnected"}, sink: &capSink{}}
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
		sc.EntityAt(10).Player.Sink = &eventSink{out: fresh.sink, log: slog.Default(), sess: fresh, observer: 10}
	}, Done: make(chan struct{})})
	r.step(1)
	// In-flight old socket work arrives after close, still in its own FIFO order.
	old.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	old.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(1)
	r.step(285)
	fresh.mu.Lock()
	sc, waiting, pending := fresh.scene, fresh.awaitMapAck, fresh.clientMapReady
	fresh.mu.Unlock()
	if sc.MapID != 1 || waiting || pending != nil {
		t.Fatal("old connection changed fresh ownership")
	}
	if p := r.posOf(1, 10); p.X != 100 || p.Y != 100 {
		t.Fatal("old connection moved reconnected entity")
	}
	if mapReviewWireCount(sink, protocol.SCTeleport) != 0 {
		t.Fatal("closed connection unexpectedly executed a delayed teleport")
	}
}

// Unlike the generic rig step, this barrier is allowed to wait for a deliberately
// saturated test mailbox. Frames, not sleeps, establish command completion.
func readyStepFullMailbox(t *testing.T, r *twoMapRig, mapID int32) {
	t.Helper()
	done := make(chan struct{})
	posted := make(chan bool, 1)
	go func() {
		posted <- r.router.PostLifecycle(domain.SceneID{MapID: mapID}, scene.Inspect{Fn: func(*scene.Scene) {}, Done: done})
	}()
	r.frames[mapID] <- time.Now()
	select {
	case ok := <-posted:
		if !ok {
			t.Fatal("full mailbox barrier rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("full mailbox barrier stalled")
	}
	r.frames[mapID] <- time.Now()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full mailbox barrier not executed")
	}
}

func TestClientMapReadyQueuedAToBToCNeedsEachOwnReady(t *testing.T) {
	r, s, sink := readyPortalFixture(t, true, false)
	r.frames[3] = make(chan time.Time)
	// Server-driven A->B reaches its load barrier before client readiness.
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	r.step(1)
	r.step(285)
	r.router.Post(domain.SceneID{MapID: 285}, scene.Teleport{ID: 1, To: domain.SceneID{MapID: 3}, At: domain.Pos{MapID: 3, X: 200, Y: 200}})
	r.step(285)
	s.mu.Lock()
	sc, epochB := s.scene, s.mapEpoch
	s.mu.Unlock()
	if sc.MapID != 285 {
		t.Fatal("queued C executed before B was ready")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(285)
	r.step(3)
	s.mu.Lock()
	sc, waiting, epochC := s.scene, s.awaitMapAck, s.mapEpoch
	s.mu.Unlock()
	if sc.MapID != 3 || !waiting || epochC <= epochB {
		t.Fatal("C did not acquire its own load barrier")
	}
	for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
		if n := mapReviewWireCount(sink, op); n != 1 {
			t.Fatalf("B first snapshot incomplete/repeated %04x:%d", op, n)
		}
	}
	// A late duplicate B ready cannot open C, nor can B's internal receipt epoch.
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.router.PostLifecycle(domain.SceneID{MapID: 3}, scene.MapReady{ID: 1, Scene: domain.SceneID{MapID: 3}, Epoch: epochB})
	r.step(3)
	s.mu.Lock()
	waiting = s.awaitMapAck
	s.mu.Unlock()
	if !waiting {
		t.Fatal("old B ready opened C")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 3, X: 200, Y: 200})
	r.step(3)
	s.mu.Lock()
	waiting = s.awaitMapAck
	s.mu.Unlock()
	if waiting || mapReviewWireCount(sink, protocol.SCTeleport) != 2 {
		t.Fatal("C failed its own first ready")
	}
	if r.posOf(1, 1).MapID != 0 || r.posOf(285, 1).MapID != 0 || r.posOf(3, 1).MapID != 3 {
		t.Fatal("A/B/C retained wrong ownership")
	}
}

type readyBarrierWireSink struct {
	entered, release chan struct{}
	once             sync.Once
}

func (b *readyBarrierWireSink) Send(p []byte) {
	if len(p) >= 2 && binary.LittleEndian.Uint16(p) == protocol.SCEntityDespawn {
		b.once.Do(func() { close(b.entered); <-b.release })
	}
}
func (*readyBarrierWireSink) Close()           {}
func (*readyBarrierWireSink) SessionID() int64 { return 99 }

func TestClientMapReadyReplacementDuringSourceTransferFlush(t *testing.T) {
	r, s, sink := readyPortalFixture(t, false, false)
	r.enter(1, 3, 300, "source observer")
	barrier := &readyBarrierWireSink{entered: make(chan struct{}), release: make(chan struct{})}
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
		sc.EntityAt(3).Player.Sink = &eventSink{out: barrier, log: slog.Default(), observer: 3}
	}, Done: make(chan struct{})})
	r.step(1)
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	select {
	case r.frames[1] <- time.Now():
	case <-barrier.entered:
	}
	// This real source flush is after actor validation but before ownership commit.
	select {
	case <-barrier.entered:
	case <-time.After(time.Second):
		t.Fatal("source never reached transfer flush")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 3, X: 200, Y: 200})
	close(barrier.release)
	r.step(1)
	sink.mu.Lock()
	closed := sink.closed
	sink.mu.Unlock()
	s.mu.Lock()
	sc, waiting, pending := s.scene, s.awaitMapAck, s.clientMapReady
	s.mu.Unlock()
	if !closed || sc.MapID != 1 || waiting || pending != nil {
		t.Fatal("new FIFO ready replacement was ignored at final transfer commit")
	}
	if r.posOf(285, 1).MapID != 0 {
		t.Fatal("stale client-ready transferred player to wrong target")
	}
	s.OnClose("stale source transfer safely closed")
	r.step(1)
}

func TestClientMapReadyRejectedRedundantSourcePostCannotCloseNewOwner(t *testing.T) {
	r, s, sink := readyPortalFixtureWithMailboxes(t, false, false, 2, 0)
	from := domain.SceneID{MapID: 1}
	ready := protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100}
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	// Split onMove at its actual unlock-before-Post seam: an earlier actor movement
	// is allowed to execute here while the network handler is descheduled.
	s.mu.Lock()
	receipt := s.registerClientMapReadyLocked(1, from, ready.MapID, ready.X, ready.Y)
	s.mu.Unlock()
	r.step(1)
	r.step(285)
	release := blockReadyActor(t, r, 1)
	for i := 0; i < 2; i++ {
		if !r.router.Post(from, scene.Inspect{Fn: func(*scene.Scene) {}, Done: make(chan struct{})}) {
			t.Fatal("cannot saturate retired source mailbox")
		}
	}
	// Resume the real handler suffix. Its now-redundant source command is rejected.
	s.postClientMapReady(1, from, ready, receipt)
	sink.mu.Lock()
	closed := sink.closed
	sink.mu.Unlock()
	s.mu.Lock()
	sc, waiting, pending := s.scene, s.awaitMapAck, s.clientMapReady
	s.mu.Unlock()
	if closed || sc.MapID != 285 || waiting || pending != nil {
		t.Fatal("rejected stale source post closed/corrupted successful destination owner")
	}
	release()
	readyStepFullMailbox(t, r, 1)
	if p := r.posOf(285, 1); p.MapID != 285 {
		t.Fatal("valid target owner missing")
	}
}

// Block the actor in an actual mailbox command before saturating its queue.
// This removes any dependence on whether the preceding rig frame has returned.
func blockReadyActor(t *testing.T, r *twoMapRig, mapID int32) func() {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if !r.router.Post(domain.SceneID{MapID: mapID}, scene.Inspect{Fn: func(*scene.Scene) { close(entered); <-release }, Done: make(chan struct{})}) {
		t.Fatal("block actor command rejected")
	}
	select {
	case r.frames[mapID] <- time.Now():
	case <-entered:
	}
	<-entered
	return unblock
}

func TestClientMapReadyDuplicatePendingReceiptDoesNotCloseOnFullMailbox(t *testing.T) {
	r, s, sink := readyPortalFixtureWithMailboxes(t, true, false, 2, 0)
	release := blockReadyActor(t, r, 1)
	if !r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(*scene.Scene) {}, Done: make(chan struct{})}) {
		t.Fatal("fill source slot")
	}
	ready := protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100}
	s.onMove(ready) // consumes the second and final mailbox slot
	s.onMove(ready) // same pending receipt must not attempt a second Post/Close
	sink.mu.Lock()
	closed := sink.closed
	sink.mu.Unlock()
	if closed {
		t.Fatal("duplicate pending ready closed connection with its first command already queued")
	}
	release()
	readyStepFullMailbox(t, r, 1)
	r.step(285)
	s.mu.Lock()
	waiting, sc := s.awaitMapAck, s.scene
	s.mu.Unlock()
	if waiting || sc.MapID != 285 || mapReviewWireCount(sink, protocol.SCTeleport) != 0 {
		t.Fatal("single queued prebuilt ready did not settle")
	}
	for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
		if n := mapReviewWireCount(sink, op); n != 1 {
			t.Fatalf("duplicate pending ready repeated/lost first snapshot %04x:%d", op, n)
		}
	}
}

package session

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

// The production session handlers and actor/router generate these wire packets.
// The client trace test reuses this fixture rather than predicting server output.
func nativePortalFixture(t *testing.T, clientOwned, sameMap bool) (*twoMapRig, *Session, *capSink) {
	return nativePortalFixtureOptions(t, clientOwned, sameMap, false, false)
}

func nativePortalFixtureOptions(t *testing.T, clientOwned, sameMap, empty, failTarget bool) (*twoMapRig, *Session, *capSink) {
	t.Helper()
	r := &twoMapRig{t: t, frames: map[int32]chan time.Time{1: make(chan time.Time), 285: make(chan time.Time), 286: make(chan time.Time)}}
	alloc := domain.NewEntityAlloc()
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 4103, Name: "native-gate drop", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Price: 2}
	landing := domain.Pos{MapID: 285, X: 100, Y: 100}
	if sameMap {
		landing = domain.Pos{MapID: 1, X: 200, Y: 200}
	}
	r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		if failTarget && id.MapID == 285 {
			return nil, errors.New("native target unavailable")
		}
		cfg := scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: r.frames[id.MapID], Alloc: alloc, Items: map[domain.ItemID]domain.ItemDef{item.ID: item}}
		if id.MapID == 1 || id.MapID == 285 {
			to := landing
			if id.MapID == 285 {
				to = domain.Pos{MapID: 286, X: 100, Y: 100}
			}
			cfg.Portals = []domain.Portal{domain.NewPortal(domain.Teleport{ProcID: 1, ClientOwned: clientOwned, To: to, ToDir: 67, Area: domain.Polygon{{X: 115, Y: 105}, {X: 125, Y: 105}, {X: 125, Y: 115}, {X: 115, Y: 115}}})}
		}
		if !empty && !sameMap && id.MapID == 1 {
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 9, Name: "source-native npc", Sprite: 630, Script: "test", Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}}
		}
		if !empty && id.MapID == landing.MapID {
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 1, Name: "native-gate npc", Sprite: 630, Script: "test", Pos: domain.Pos{MapID: id.MapID, X: 220, Y: 220}}}
			defs := spawn.MapDefs{1001: domain.MonsterDef{ID: 1001, Name: "native-gate monster", Kind: domain.MonsterProp, Sprite: 101, Level: 1, HP: 100}}
			cfg.Spawner, _ = spawn.New([]domain.SpawnPoint{{ID: 1, Monster: 1001, Pos: domain.Pos{MapID: id.MapID, X: 240, Y: 240}}}, defs, alloc)
			cfg.Defs = defs
		}
		return scene.New(cfg), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	r.router.Start(ctx)
	t.Cleanup(func() { cancel(); r.router.Drain(time.Second) })
	r.enter(1, 1, 100, "source")
	if !failTarget {
		if !empty {
			r.enter(landing.MapID, 2, 200, "target witness")
			r.router.Post(domain.SceneID{MapID: landing.MapID}, scene.Inspect{Fn: func(sc *scene.Scene) { sc.EntityAt(2).Player.Bag.Set(0, domain.Stack{Item: item.ID, Count: 1}) }, Done: make(chan struct{})})
			r.step(landing.MapID)
			r.router.Post(domain.SceneID{MapID: landing.MapID}, scene.DropBagItem{ID: 2, Tab: 1, Slot: 0, Item: item.ID, At: domain.Pos{MapID: landing.MapID, X: 100, Y: 100}})
			r.step(landing.MapID)
		} else if _, err := r.router.Ensure(domain.SceneID{MapID: landing.MapID}); err != nil {
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

func nativeReady(mapID, x, y int32) protocol.Request {
	return protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: mapID, X: x, Y: y}
}
func assertNativeSnapshot(t *testing.T, sink *capSink, n int) {
	t.Helper()
	for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
		if got := mapReviewWireCount(sink, op); got != n {
			t.Fatalf("snapshot %04x count=%d want=%d", op, got, n)
		}
	}
}
func assertNativeOwner(t *testing.T, s *Session, sc domain.SceneID) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scene != sc || s.awaitMapAck || s.clientMapReady != nil {
		t.Fatalf("owner=%v awaiting=%v receipt=%+v want=%v", s.scene, s.awaitMapAck, s.clientMapReady, sc)
	}
}

func TestNativePortalFIFOReadyDriven(t *testing.T) {
	for _, sameMap := range []bool{false, true} {
		for _, phase := range []string{"ready_before_source_frame", "ready_after_source_frame", "ready_after_more_source_frames", "ready_without_final_move"} {
			for _, duplicate := range []bool{false, true} {
				t.Run(fmt.Sprintf("same_%v/%s/duplicate_%v", sameMap, phase, duplicate), func(t *testing.T) {
					r, s, sink := nativePortalFixture(t, true, sameMap)
					target, x, y := int32(285), int32(100), int32(100)
					if sameMap {
						target, x, y = 1, 200, 200
					}
					if phase != "ready_without_final_move" {
						s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
					}
					if phase == "ready_after_source_frame" || phase == "ready_after_more_source_frames" {
						r.step(1)
					}
					if phase == "ready_after_more_source_frames" {
						for i := 0; i < 4; i++ {
							r.step(1)
						}
					}
					// Regardless of how early the server consumes Move, it never creates the
					// opposing 803a that could cross an already-built client's upstream ready.
					if got := mapReviewWireCount(sink, protocol.SCTeleport); got != 0 {
						t.Fatalf("before ready: teleport=%d", got)
					}
					assertNativeOwner(t, s, domain.SceneID{MapID: 1})
					s.onMove(nativeReady(target, x, y))
					if duplicate {
						s.onMove(nativeReady(target, x, y))
					}
					r.step(1)
					if !sameMap {
						r.step(target)
					}
					assertNativeOwner(t, s, domain.SceneID{MapID: target})
					if got := mapReviewWireCount(sink, protocol.SCTeleport); got != 0 {
						t.Fatalf("after ready: teleport=%d", got)
					}
					assertNativeSnapshot(t, sink, 1)
					if pos := r.posOf(target, 1); pos != (domain.Pos{MapID: target, X: float64(x), Y: float64(y)}) {
						t.Fatalf("landing=%+v", pos)
					}
					if !sameMap && r.posOf(1, 1).MapID != 0 {
						t.Fatal("source still owns player")
					}
					r.step(target)
					assertNativeSnapshot(t, sink, 1)
				})
			}
		}
	}
}

func TestNativePortalNoReadyKeepsSourceAndAllowsExit(t *testing.T) {
	r, s, sink := nativePortalFixture(t, true, false)
	// Includes startup inside the polygon, integer rounding into it, and a local
	// resource/build rejection: no entered=1 means no transfer was requested.
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
	r.step(1)
	for i := 0; i < 3; i++ {
		s.onMove(protocol.Request{Kind: protocol.ReqSavePos, MapID: 1, X: 120, Y: 110})
		r.step(1)
	}
	assertNativeOwner(t, s, domain.SceneID{MapID: 1})
	if mapReviewWireCount(sink, protocol.SCTeleport) != 0 {
		t.Fatal("movement forced a client-owned gate")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 110, Y: 100})
	r.step(1)
	if pos := r.posOf(1, 1); pos.X != 110 || pos.Y != 100 {
		t.Fatalf("failed build/source exit left wrong owner: %+v", pos)
	}
}

func TestNativePortalSameMapRebuildAndUnknownLandingDoNotMove(t *testing.T) {
	r, s, sink := nativePortalFixture(t, true, true)
	for _, at := range []protocol.Request{nativeReady(1, 999, 999), nativeReady(1, 201, 200)} {
		s.onMove(at)
		r.step(1)
		if p := r.posOf(1, 1); p.X != 100 || p.Y != 100 {
			t.Fatalf("unconfigured ready moved player: %+v", p)
		}
	}
	assertNativeSnapshot(t, sink, 2)
	s.onMove(nativeReady(1, 200, 200))
	r.step(1)
	s.mu.Lock()
	epoch := s.mapEpoch
	s.mu.Unlock()
	s.onMove(nativeReady(1, 200, 200))
	r.step(1)
	s.mu.Lock()
	after := s.mapEpoch
	s.mu.Unlock()
	if after != epoch {
		t.Fatal("already-landed rebuild repeated teleport side effects")
	}
	assertNativeSnapshot(t, sink, 4)
	if mapReviewWireCount(sink, protocol.SCTeleport) != 0 {
		t.Fatal("same-map ready forced a rebuild")
	}
}

func TestNativePortalServerOwnedAndExplicitTransfersStillDriveClient(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		for _, sameMap := range []bool{false, true} {
			t.Run(fmt.Sprintf("explicit_%v/same_%v", explicit, sameMap), func(t *testing.T) {
				r, s, sink := nativePortalFixture(t, explicit, sameMap)
				target, x, y := int32(285), int32(100), int32(100)
				if sameMap {
					target, x, y = 1, 200, 200
				}
				if explicit {
					r.router.Post(domain.SceneID{MapID: 1}, scene.Teleport{ID: 1, To: domain.SceneID{MapID: target}, At: domain.Pos{MapID: target, X: float64(x), Y: float64(y)}})
				} else {
					s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
				}
				r.step(1)
				if !sameMap {
					r.step(target)
				}
				if mapReviewWireCount(sink, protocol.SCTeleport) != 1 {
					t.Fatal("server-initiated transfer lost native teleport")
				}
				assertNativeSnapshot(t, sink, 0)
				s.onMove(nativeReady(target, x, y))
				r.step(target)
				assertNativeSnapshot(t, sink, 1)
				assertNativeOwner(t, s, domain.SceneID{MapID: target})
			})
		}
	}
}

func TestNativePortalEmptyFailedDisconnectedAndFollowingTransfer(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		r, s, sink := nativePortalFixtureOptions(t, true, false, true, false)
		s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
		r.step(1)
		s.onMove(nativeReady(285, 100, 100))
		r.step(1)
		r.step(285)
		assertNativeOwner(t, s, domain.SceneID{MapID: 285})
		if mapReviewWireCount(sink, protocol.SCTeleport) != 0 {
			t.Fatal("empty target teleported")
		}
	})
	t.Run("failed target", func(t *testing.T) {
		r, s, sink := nativePortalFixtureOptions(t, true, false, true, true)
		s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
		r.step(1)
		s.onMove(nativeReady(285, 100, 100))
		r.step(1)
		sink.mu.Lock()
		closed := sink.closed
		sink.mu.Unlock()
		if !closed {
			t.Fatal("failed handoff left a client on unavailable destination")
		}
		s.mu.Lock()
		receipt := s.clientMapReady
		binding := s.scene
		s.mu.Unlock()
		if receipt != nil || binding.MapID != 1 {
			t.Fatal("failed handoff retained receipt or wrong persisted binding")
		}
	})
	t.Run("disconnect before ready consumption", func(t *testing.T) {
		r, s, sink := nativePortalFixture(t, true, false)
		s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
		r.step(1)
		s.onMove(nativeReady(285, 100, 100))
		s.OnClose("native disconnect")
		r.step(1)
		r.step(285)
		if r.posOf(1, 1).MapID != 0 || r.posOf(285, 1).MapID != 0 {
			t.Fatal("closed player retained an owner")
		}
		assertNativeSnapshot(t, sink, 0)
	})
	t.Run("A to B to C", func(t *testing.T) {
		r, s, sink := nativePortalFixture(t, true, false)
		s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
		r.step(1)
		s.onMove(nativeReady(285, 100, 100))
		r.step(1)
		r.step(285)
		assertNativeSnapshot(t, sink, 1)
		s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
		r.step(285)
		assertNativeOwner(t, s, domain.SceneID{MapID: 285})
		s.onMove(nativeReady(286, 100, 100))
		r.step(285)
		r.step(286)
		assertNativeOwner(t, s, domain.SceneID{MapID: 286})
		if r.posOf(1, 1).MapID != 0 || r.posOf(285, 1).MapID != 0 || r.posOf(286, 1).MapID != 286 {
			t.Fatal("consecutive gates lost unique owner")
		}
		if mapReviewWireCount(sink, protocol.SCTeleport) != 0 {
			t.Fatal("native consecutive gates emitted forced teleport")
		}
	})
}

// Real source NPC snapshot and another player's later drop can both be on the
// downlink after the local target has been built but before its ready arrives.
func nativePortalLateMaplessPackets(t *testing.T) ([][]byte, map[uint32]uint16) {
	t.Helper()
	r, s, sink := nativePortalFixture(t, true, false)
	r.enter(1, 3, 300, "source bystander")
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) { sc.EntityAt(3).Player.Bag.Set(0, domain.Stack{Item: 4103, Count: 1}) }, Done: make(chan struct{})})
	r.step(1)
	sink.mu.Lock()
	sink.pkts = nil
	sink.mu.Unlock()
	// A legitimate preceding same-scene snapshot may still be in downlink flight.
	s.onMove(nativeReady(1, 100, 100))
	r.step(1)
	r.router.Post(domain.SceneID{MapID: 1}, scene.DropBagItem{ID: 3, Tab: 1, Slot: 0, Item: 4103, At: domain.Pos{MapID: 1, X: 100, Y: 100}})
	r.step(1)
	old := map[uint32]uint16{}
	sink.mu.Lock()
	for _, p := range sink.pkts {
		if len(p) < 6 {
			continue
		}
		op := binary.LittleEndian.Uint16(p)
		if op == protocol.SCGroundSpawn || op == protocol.SCNPCSpawn {
			old[binary.LittleEndian.Uint32(p[2:])] = op
		}
	}
	sink.mu.Unlock()
	if len(old) != 2 {
		t.Fatalf("source fixtures must publish NPC and drop, got %v", old)
	}
	s.onMove(nativeReady(285, 100, 100))
	r.step(1)
	r.step(285)
	assertNativeOwner(t, s, domain.SceneID{MapID: 285})
	sink.mu.Lock()
	defer sink.mu.Unlock()
	packets := make([][]byte, len(sink.pkts))
	for i, p := range sink.pkts {
		packets[i] = append([]byte(nil), p...)
	}
	return packets, old
}

func TestNativePortalMaplessSourceRowsRetiredBeforeTargetSnapshot(t *testing.T) {
	packets, old := nativePortalLateMaplessPackets(t)
	rows := map[uint32]uint16{}
	targetSpawns := 0
	for _, p := range packets {
		if len(p) < 6 {
			continue
		}
		op, id := binary.LittleEndian.Uint16(p), binary.LittleEndian.Uint32(p[2:])
		switch op {
		case protocol.SCNPCSpawn, protocol.SCGroundSpawn:
			if _, source := old[id]; !source {
				for sourceID := range old {
					if _, remains := rows[sourceID]; remains {
						t.Fatalf("source ID %d remains when target %d spawns", sourceID, id)
					}
				}
				targetSpawns++
			}
			rows[id] = op
		case protocol.SCNPCRemove, protocol.SCGroundRemove:
			sourceKind, source := old[id]
			if !source {
				t.Fatalf("removal unexpectedly targets non-source ID %d", id)
			}
			if (sourceKind == protocol.SCNPCSpawn) != (op == protocol.SCNPCRemove) {
				t.Fatal("wrong native kind-specific removal")
			}
			delete(rows, id)
		}
	}
	if len(rows) != 2 || targetSpawns != 2 {
		t.Fatalf("final mapless rows=%v target spawns=%d", rows, targetSpawns)
	}
	for id := range old {
		if _, ok := rows[id]; ok {
			t.Fatalf("source ID %d survived target snapshot", id)
		}
	}
}

func TestNativePortalSameMapCommitRejectsStaleBinding(t *testing.T) {
	for _, mutation := range []string{"epoch", "instance", "entity", "waiting", "closed"} {
		t.Run(mutation, func(t *testing.T) {
			s := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 1}, mapEpoch: 7}
			switch mutation {
			case "epoch":
				s.mapEpoch++
			case "instance":
				s.scene.Instance = 5
			case "entity":
				s.entity = 2
			case "waiting":
				s.awaitMapAck = true
			case "closed":
				s.stage = StageConnected
			}
			sink := &eventSink{sess: s}
			if sink.ConfirmClientGateReady(1, domain.SceneID{MapID: 1}, 7) {
				t.Fatal("stale same-map gate committed")
			}
		})
	}
}

func TestNativePortalVillageMenuKeepsClientFirstWithoutGeometry(t *testing.T) {
	// Actual DialogUI.Travel NoviceVillages. These menu callbacks build locally;
	// no geometric portal, OpenTeleport request, or final movement packet exists.
	for _, at := range []domain.Pos{{MapID: 61, X: 6923, Y: 1763}, {MapID: 37, X: 5978, Y: 2432}, {MapID: 62, X: 4863, Y: 3529}, {MapID: 38, X: 4965, Y: 3681}} {
		t.Run(fmt.Sprint(at.MapID), func(t *testing.T) {
			r := &twoMapRig{t: t, frames: map[int32]chan time.Time{285: make(chan time.Time), at.MapID: make(chan time.Time)}}
			r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
				return scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: r.frames[id.MapID]}), nil
			}, slog.Default())
			ctx, cancel := context.WithCancel(context.Background())
			r.router.Start(ctx)
			t.Cleanup(func() { cancel(); r.router.Drain(time.Second) })
			r.enter(285, 1, 100, "new character on island")
			r.enter(at.MapID, 2, 200, "village witness")
			out := &capSink{}
			s := &Session{deps: Deps{Router: r.router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 285}, sink: out, char: &domain.Character{ID: 100}}
			r.router.Post(domain.SceneID{MapID: 285}, scene.Inspect{Fn: func(sc *scene.Scene) {
				sc.EntityAt(1).Player.Sink = &eventSink{out: out, log: slog.Default(), sess: s, observer: 1}
			}, Done: make(chan struct{})})
			r.step(285)
			s.onMove(nativeReady(at.MapID, int32(at.X), int32(at.Y)))
			s.onMove(nativeReady(at.MapID, int32(at.X), int32(at.Y)))
			r.step(285)
			r.step(at.MapID)
			assertNativeOwner(t, s, domain.SceneID{MapID: at.MapID})
			if got := r.posOf(at.MapID, 1); got != at {
				t.Fatalf("village landing=%+v want=%+v", got, at)
			}
			if mapReviewWireCount(out, protocol.SCTeleport) != 0 || mapReviewWireCount(out, protocol.SCRemotePlayer) != 1 {
				t.Fatal("village menu required forced/duplicate load")
			}
		})
	}
}

func TestNativePortalPrivateSameMapReadyRemainsResynchronization(t *testing.T) {
	id := domain.SceneID{MapID: 1, Instance: 77}
	frames := make(chan time.Time)
	router := scene.NewRouter(func(sc domain.SceneID) (*scene.Scene, error) {
		return scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: sc, Frames: frames, Portals: []domain.Portal{domain.NewPortal(domain.Teleport{ClientOwned: true, To: domain.Pos{MapID: 1, X: 200, Y: 200}, Area: domain.Polygon{{X: 115, Y: 105}, {X: 125, Y: 105}, {X: 125, Y: 115}, {X: 115, Y: 115}}})}}), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	router.Start(ctx)
	t.Cleanup(func() { cancel(); router.Drain(time.Second) })
	out := &capSink{}
	s := &Session{deps: Deps{Router: router, Log: slog.Default()}, log: slog.Default(), stage: StageInGame, entity: 1, scene: id, sink: out}
	step := func(fn func(*scene.Scene)) {
		done := make(chan struct{})
		if !router.Post(id, scene.Inspect{Fn: fn, Done: done}) {
			t.Fatal("inspect rejected")
		}
		select {
		case frames <- time.Now():
		case <-done:
		}
		<-done
	}
	if !router.Post(id, scene.Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "private", Level: 10, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Sink: &eventSink{out: out, log: slog.Default(), sess: s, observer: 1}}) {
		t.Fatal("private enter")
	}
	step(func(*scene.Scene) {})
	s.onMove(nativeReady(1, 200, 200))
	step(func(sc *scene.Scene) {
		if p := sc.EntityAt(1).Pos; p.X != 100 || p.Y != 100 {
			t.Fatalf("private resync incorrectly used public native gate: %+v", p)
		}
	})
	s.mu.Lock()
	epoch := s.mapEpoch
	s.mu.Unlock()
	if epoch != 0 {
		t.Fatalf("private rebuild advanced epoch=%d", epoch)
	}
}

func TestNativePortalGMForceRejectsCrossedDifferentLandingReady(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			r, s, sink := nativePortalFixture(t, true, same)
			target := int32(285)
			if same {
				target = 1
			}
			s.account = &store.Account{ID: 1, GMLevel: store.GMApp}
			s.deps.GMLanding = func(mapID int32) (domain.Pos, bool) { return domain.Pos{MapID: mapID, X: 300, Y: 300}, mapID == target }
			done := make(chan struct{})
			go func() {
				s.OnPacket(0x1006, protocol.NewW(0).Str(fmt.Sprintf("goto %d 300 300", target)).Bytes()[2:])
				close(done)
			}()
			timeout := time.After(time.Second)
		loop:
			for {
				select {
				case <-done:
					break loop
				case r.frames[1] <- time.Now():
				case <-timeout:
					t.Fatal("GM command did not finish")
				}
			}
			r.step(target)
			if mapReviewWireCount(sink, protocol.SCTeleport) != 1 {
				t.Fatal("actual GM request did not emit authoritative teleport")
			}
			// The earlier client-owned build's ready crosses the still unread GM803a.
			s.OnPacket(0x100a, protocol.NewW(0).I32(target).I32(200).I32(200).U8(1).Bytes()[2:])
			r.step(target)
			s.mu.Lock()
			waiting := s.awaitMapAck
			s.mu.Unlock()
			if !waiting {
				t.Fatal("stale other-landing ready released the target")
			}
			assertNativeSnapshot(t, sink, 0)
			// Once the real forced landing has built, its single ready releases all kinds.
			s.OnPacket(0x100a, protocol.NewW(0).I32(target).I32(300).I32(300).U8(1).Bytes()[2:])
			r.step(target)
			assertNativeOwner(t, s, domain.SceneID{MapID: target})
			assertNativeSnapshot(t, sink, 1)
		})
	}
}

func TestNativePortalForcedReadyUsesNativeIntegerFloatRoundTrip(t *testing.T) {
	for _, at := range []domain.Pos{{MapID: 285, X: 200.75, Y: 301.99}, {MapID: 285, X: 16777217, Y: 123.999999}, {MapID: 285, X: -200.75, Y: -301.99}} {
		t.Run(fmt.Sprintf("%g_%g", at.X, at.Y), func(t *testing.T) {
			r, s, sink := nativePortalFixture(t, true, false)
			r.router.Post(domain.SceneID{MapID: 1}, scene.Teleport{ID: 1, To: domain.SceneID{MapID: 285}, At: at})
			r.step(1)
			r.step(285)
			// Decode the real encoder's I32 fields, as the original client does,
			// then apply its Single storage and ReportPos integer conversion.
			var packet []byte
			sink.mu.Lock()
			for _, p := range sink.pkts {
				if len(p) >= 2 && binary.LittleEndian.Uint16(p) == protocol.SCTeleport {
					packet = append([]byte(nil), p...)
					break
				}
			}
			sink.mu.Unlock()
			if len(packet) != 14 {
				t.Fatalf("unexpected teleport wire %x", packet)
			}
			x := int32(float32(int32(binary.LittleEndian.Uint32(packet[6:10]))))
			y := int32(float32(int32(binary.LittleEndian.Uint32(packet[10:14]))))
			s.onMove(nativeReady(285, x+1, y))
			r.step(285)
			s.mu.Lock()
			waiting := s.awaitMapAck
			s.mu.Unlock()
			if !waiting {
				t.Fatal("wrong integer landing released snapshot")
			}
			s.onMove(nativeReady(285, x, y))
			r.step(285)
			assertNativeOwner(t, s, domain.SceneID{MapID: 285})
			assertNativeSnapshot(t, sink, 1)
		})
	}
}

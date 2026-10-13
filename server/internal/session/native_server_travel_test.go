package session

import (
	"context"
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

func nativeServerTravelFixture(t *testing.T, transport bool, itemID domain.ItemID) (*twoMapRig, *Session, *capSink, domain.Pos, int32) {
	t.Helper()
	source := int32(919)
	at := domain.Pos{MapID: 58, X: 7715, Y: 5323}
	if transport {
		source = 4
		at = domain.Pos{MapID: 21, X: 9201, Y: 1953}
	}
	r := &twoMapRig{t: t, frames: map[int32]chan time.Time{source: make(chan time.Time), at.MapID: make(chan time.Time)}}
	alloc := domain.NewEntityAlloc()
	items := map[domain.ItemID]domain.ItemDef{
		3381:  {EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 3381, Name: "回城卷轴", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, UseReturn: true, ConsumeOnUse: true},
		10025: {EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 10025, Name: "无限次回城书", InventoryTab: 1, InventoryTabKnown: true, UseReturn: true},
		4103:  {EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 4103, Name: "ground witness", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Price: 2},
	}
	r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		cfg := scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: r.frames[id.MapID], Alloc: alloc, Items: items, Home: scene.ReviveAt{Scene: domain.SceneID{MapID: at.MapID}, Pos: at}, Revive: scene.ReviveAt{Scene: domain.SceneID{MapID: at.MapID}, Pos: at}}
		if id.MapID == source && transport {
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 3, Name: "野外传送点", Sprite: 856, Script: "N856_004", Trans: 6, Pos: domain.Pos{MapID: 4, X: 5446, Y: 2797}}}
			cfg.Transports = domain.TransportTable{6: {ID: 6, SourceMapID: 4, Destinations: []domain.TransportDestination{{Index: 0, Name: "下古岳平原", Pos: at}}}}
		}
		if id.MapID == at.MapID {
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 1, Name: "target NPC", Sprite: 630, Pos: domain.Pos{MapID: id.MapID, X: 100, Y: 100}}}
			defs := spawn.MapDefs{1001: domain.MonsterDef{ID: 1001, Name: "target entity", Kind: domain.MonsterProp, Sprite: 101, Level: 1, HP: 100}}
			cfg.Spawner, _ = spawn.New([]domain.SpawnPoint{{ID: 1, Monster: 1001, Pos: domain.Pos{MapID: id.MapID, X: 140, Y: 140}}}, defs, alloc)
			cfg.Defs = defs
		}
		return scene.New(cfg), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	r.router.Start(ctx)
	t.Cleanup(func() { cancel(); r.router.Drain(time.Second) })
	r.enter(source, 1, 100, "traveler")
	r.enter(at.MapID, 2, 200, "target witness")
	r.router.Post(domain.SceneID{MapID: at.MapID}, scene.Inspect{Fn: func(sc *scene.Scene) { sc.EntityAt(2).Player.Bag.Set(0, domain.Stack{Item: 4103, Count: 1}) }, Done: make(chan struct{})})
	r.step(at.MapID)
	r.router.Post(domain.SceneID{MapID: at.MapID}, scene.DropBagItem{ID: 2, Tab: 1, Slot: 0, Item: 4103, At: domain.Pos{MapID: at.MapID, X: 100, Y: 100}})
	r.step(at.MapID)
	out := &capSink{}
	s := &Session{deps: Deps{Router: r.router, Log: slog.Default(), Store: store.NewMemory(catalogfixture.EndgameCatalog())}, log: slog.Default(), stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: source}, char: &domain.Character{ID: 100}, sink: out}
	r.router.Post(domain.SceneID{MapID: source}, scene.Inspect{Fn: func(sc *scene.Scene) {
		p := sc.EntityAt(1)
		p.Player.Sink = &eventSink{out: out, log: slog.Default(), sess: s, observer: 1}
		if itemID != 0 {
			p.Player.Bag.Set(clientBagSlot(1, 0), domain.Stack{Item: itemID, Count: 1})
		}
	}, Done: make(chan struct{})})
	r.step(source)
	return r, s, out, at, source
}

func TestNativePortalWildernessTransportActualOpenChoosePackets(t *testing.T) {
	r, s, out, at, source := nativeServerTravelFixture(t, true, 0)
	// Both opcodes are the production decoder/handler route. No synthetic Teleport.
	s.OnPacket(0x1072, protocol.NewW(0).I32(6).I32(0).Bytes()[2:])
	r.step(source)
	if mapReviewWireCount(out, protocol.SCTeleport) != 0 {
		t.Fatal("choose without an open menu teleported")
	}
	s.OnPacket(0x1071, protocol.NewW(0).I32(6).Bytes()[2:])
	r.step(source)
	s.OnPacket(0x1072, protocol.NewW(0).I32(6).I32(99).Bytes()[2:])
	r.step(source)
	if mapReviewWireCount(out, protocol.SCTeleport) != 0 {
		t.Fatal("invalid destination teleported")
	}
	for i := 0; i < 2; i++ {
		s.OnPacket(0x1072, protocol.NewW(0).I32(6).I32(0).Bytes()[2:])
	}
	r.step(source)
	r.step(at.MapID)
	if mapReviewWireCount(out, protocol.SCTeleport) != 1 {
		t.Fatal("transport menu did not create exactly one forced transfer")
	}
	assertNativeSnapshot(t, out, 0)
	s.OnPacket(0x100a, protocol.NewW(0).I32(at.MapID).I32(int32(at.X) + 1).I32(int32(at.Y)).U8(1).Bytes()[2:])
	r.step(at.MapID)
	assertNativeSnapshot(t, out, 0)
	s.OnPacket(0x100a, protocol.NewW(0).I32(at.MapID).I32(int32(at.X)).I32(int32(at.Y)).U8(1).Bytes()[2:])
	r.step(at.MapID)
	assertNativeOwner(t, s, domain.SceneID{MapID: at.MapID})
	assertNativeSnapshot(t, out, 1)
	if pos := r.posOf(at.MapID, 1); pos != at {
		t.Fatalf("wrong configured transport landing %v", pos)
	}
}

func TestNativePortalReturnItemsActualUsePacket(t *testing.T) {
	for _, itemID := range []domain.ItemID{3381, 10025} {
		t.Run(map[domain.ItemID]string{3381: "consumable scroll", 10025: "unlimited book"}[itemID], func(t *testing.T) {
			r, s, out, at, source := nativeServerTravelFixture(t, false, itemID)
			payload := protocol.NewW(0).I32(int32(itemID)).U8(1).I32(0).Bytes()[2:]
			s.OnPacket(0x1015, payload)
			s.OnPacket(0x1015, payload)
			r.step(source)
			r.step(at.MapID)
			if mapReviewWireCount(out, protocol.SCTeleport) != 1 {
				t.Fatal("actual item use did not transfer exactly once")
			}
			assertNativeSnapshot(t, out, 0)
			s.onMove(nativeReady(at.MapID, int32(at.X), int32(at.Y)))
			r.step(at.MapID)
			assertNativeOwner(t, s, domain.SceneID{MapID: at.MapID})
			assertNativeSnapshot(t, out, 1)
			done := make(chan struct{})
			r.router.Post(domain.SceneID{MapID: at.MapID}, scene.Inspect{Fn: func(sc *scene.Scene) {
				count := sc.EntityAt(1).Player.Bag.CountOf(itemID)
				want := int32(0)
				if itemID == 10025 {
					want = 1
				}
				if count != want {
					t.Errorf("item count=%d want=%d", count, want)
				}
			}, Done: done})
			r.step(at.MapID)
			<-done
			// Same-map reuse of the infinite book is a real new server operation. The
			// consumed scroll is rejected, not a duplicate map-load compensator.
			s.OnPacket(0x1015, payload)
			r.step(at.MapID)
			if itemID == 3381 {
				if mapReviewWireCount(out, protocol.SCTeleport) != 1 {
					t.Fatal("missing consumed scroll teleported")
				}
				return
			}
			if mapReviewWireCount(out, protocol.SCTeleport) != 2 {
				t.Fatal("same-map book use lost authoritative teleport")
			}
			s.onMove(nativeReady(at.MapID, int32(at.X), int32(at.Y)))
			r.step(at.MapID)
			assertNativeSnapshot(t, out, 2)
		})
	}
}

func TestNativePortalDeathReturnActualRevivePacket(t *testing.T) {
	r, s, out, at, source := nativeServerTravelFixture(t, false, 0)
	// Alive requests cannot start a return; only the actual dead-state entry can.
	s.OnPacket(0x1018, protocol.NewW(0).U8(0).Bytes()[2:])
	r.step(source)
	if mapReviewWireCount(out, protocol.SCTeleport) != 0 {
		t.Fatal("alive revive returned to town")
	}
	r.router.Post(domain.SceneID{MapID: source}, scene.Inspect{Fn: func(sc *scene.Scene) { sc.EntityAt(1).HP = 0 }, Done: make(chan struct{})})
	r.step(source)
	s.OnPacket(0x1018, protocol.NewW(0).U8(0).Bytes()[2:])
	s.OnPacket(0x1018, protocol.NewW(0).U8(0).Bytes()[2:])
	r.step(source)
	r.step(at.MapID)
	if mapReviewWireCount(out, protocol.SCTeleport) != 1 {
		t.Fatal("dead revive did not issue exactly one native return")
	}
	assertNativeSnapshot(t, out, 0)
	s.onMove(nativeReady(at.MapID, int32(at.X), int32(at.Y)))
	r.step(at.MapID)
	assertNativeOwner(t, s, domain.SceneID{MapID: at.MapID})
	assertNativeSnapshot(t, out, 1)
	if pos := r.posOf(at.MapID, 1); pos != at {
		t.Fatalf("wrong town return %v", pos)
	}
}

func TestNativePortalGMIdenticalLandingRestoresAfterSecondRealBuildReady(t *testing.T) {
	r, s, out := nativePortalFixture(t, true, false)
	s.account = &store.Account{ID: 1, GMLevel: store.GMApp}
	s.deps.GMLanding = func(mapID int32) (domain.Pos, bool) { return domain.Pos{MapID: mapID, X: 100, Y: 100}, mapID == 285 }
	done := make(chan struct{})
	go func() { s.OnPacket(0x1006, protocol.NewW(0).Str("goto 285 100 100").Bytes()[2:]); close(done) }()
	limit := time.After(time.Second)
loop:
	for {
		select {
		case <-done:
			break loop
		case r.frames[1] <- time.Now():
		case <-limit:
			t.Fatal("GM command did not finish")
		}
	}
	r.step(285)
	ready := protocol.NewW(0).I32(285).I32(100).I32(100).U8(1).Bytes()[2:]
	// Its first ready belongs to a simultaneous local gate. Identical wire map/
	// coordinates cannot identify which operation produced it; GM is a real second
	// requested transfer, not the removed server response to one ordinary Move.
	s.OnPacket(0x100a, ready)
	r.step(285)
	assertNativeSnapshot(t, out, 1)
	// Actual original coroutine rebuild after late GM803a sends this second ready.
	// Its ordinary same-map resync must restore all kinds even if snapshot1 arrived
	// while switching. No compensating 78ce2b0 ACK or new opcode is required.
	s.OnPacket(0x100a, ready)
	r.step(285)
	assertNativeOwner(t, s, domain.SceneID{MapID: 285})
	assertNativeSnapshot(t, out, 2)
	if mapReviewWireCount(out, protocol.SCTeleport) != 1 || r.posOf(1, 1).MapID != 0 || r.posOf(285, 1) != (domain.Pos{MapID: 285, X: 100, Y: 100}) {
		t.Fatal("independent same-landing forced transfer did not recover uniquely")
	}
}

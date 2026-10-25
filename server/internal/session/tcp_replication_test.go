package session

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	gateway "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/net"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

type replicationTCPHandler struct {
	keys  wire.Keys
	ready chan struct{}
}

func (h *replicationTCPHandler) OnHandshake([]byte) (wire.Keys, error) {
	close(h.ready)
	return h.keys, nil
}
func (h *replicationTCPHandler) OnPacket(uint16, []byte) {}
func (h *replicationTCPHandler) OnClose(string)          {}

type replicationTCPAccepted struct {
	conn  *gateway.Conn
	ready chan struct{}
}

// Uses the exported gateway listener/factory, not a replacement Send queue or a
// test-only production hook. The only synthetic boundary is authentication: the
// handler returns fixed test keys after the real 260-byte transport handshake.
func startReplicationTCPGateway(t *testing.T) (string, <-chan replicationTCPAccepted, wire.Keys) {
	return startReplicationTCPGatewayWithWriteWait(t, 250*time.Millisecond)
}

func startReplicationTCPGatewayWithWriteWait(t *testing.T, writeWait time.Duration) (string, <-chan replicationTCPAccepted, wire.Keys) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	keys := wire.Keys{AES: make([]byte, 32), MAC: make([]byte, 32)}
	for i := range keys.AES {
		keys.AES[i] = byte(i + 1)
		keys.MAC[i] = byte(255 - i)
	}
	accepted := make(chan replicationTCPAccepted, 8)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := gateway.NewServer(addr, func(out gateway.Sink) gateway.Handler {
		ready := make(chan struct{})
		accepted <- replicationTCPAccepted{conn: out.(*gateway.Conn), ready: ready}
		return &replicationTCPHandler{keys: keys, ready: ready}
	}, gateway.Options{Log: logger, OutBuffer: 1024, WriteWait: writeWait, ShutdownWait: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(2 * time.Second):
			t.Error("gateway did not stop")
		}
	})
	return addr, accepted, keys
}

type replicationTCPReader struct {
	client             net.Conn
	mu                 sync.Mutex
	monsters           map[uint32]int
	frames             int
	err                error
	teleports          int
	currentMap         int32
	staleAfterTeleport int
	npcAfterTeleport   int
}

func (r *replicationTCPReader) count() (int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.monsters), r.frames, r.err
}
func (r *replicationTCPReader) read(c net.Conn, keys wire.Keys, slow bool) {
	var opener wire.Opener
	var seq uint64
	for {
		body, err := wire.ReadFrame(c)
		if err != nil {
			r.mu.Lock()
			r.err = err
			r.mu.Unlock()
			return
		}
		plain, err := opener.OpenInPlace(&keys, body)
		if err == nil && (len(plain) < 10 || binary.LittleEndian.Uint64(plain) != seq) {
			err = fmt.Errorf("sequence %d mismatch", seq)
		}
		if err != nil {
			r.mu.Lock()
			r.err = err
			r.mu.Unlock()
			return
		}
		seq++
		op := binary.LittleEndian.Uint16(plain[8:10])
		payload := plain[10:]
		r.mu.Lock()
		r.frames++
		if op == protocol.SCEntitySpawn && len(payload) >= 4 {
			r.monsters[binary.LittleEndian.Uint32(payload)]++
			if r.currentMap != 0 && len(payload) >= 11 {
				nameLen := int(binary.LittleEndian.Uint16(payload[9:11]))
				mapAt := 36 + nameLen
				if len(payload) < mapAt+4 || int32(binary.LittleEndian.Uint32(payload[mapAt:mapAt+4])) != r.currentMap {
					r.staleAfterTeleport++
				}
			}
		}
		if op == protocol.SCNPCSpawn && r.currentMap != 0 {
			r.npcAfterTeleport++
			if bytes.Contains(payload, []byte("source-npc")) {
				r.staleAfterTeleport++
			}
		}
		if op == protocol.SCTeleport {
			r.teleports++
			if len(payload) >= 4 {
				r.currentMap = int32(binary.LittleEndian.Uint32(payload[:4]))
			}
		}
		r.mu.Unlock()
		if slow && seq%16 == 0 {
			time.Sleep(time.Millisecond)
		}
	}
}
func dialReplicationTCP(t *testing.T, addr string, accepted <-chan replicationTCPAccepted, keys wire.Keys, read bool, slow bool) (*gateway.Conn, *replicationTCPReader) {
	t.Helper()
	var c net.Conn
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err = net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if !read {
		_ = c.(*net.TCPConn).SetReadBuffer(4096)
	}
	if _, err = c.Write(make([]byte, 260)); err != nil {
		t.Fatal(err)
	}
	var a replicationTCPAccepted
	select {
	case a = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("connection factory did not run")
	}
	select {
	case <-a.ready:
	case <-time.After(time.Second):
		t.Fatal("handshake did not finish")
	}
	r := &replicationTCPReader{client: c, monsters: make(map[uint32]int)}
	if read {
		go r.read(c, keys, slow)
	}
	return a.conn, r
}

func TestSceneSessionTCPBoundedDenseInitialSnapshot(t *testing.T) {
	for _, total := range []int{1160, 4097} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			addr, accepted, keys := startReplicationTCPGateway(t)
			// The larger synthetic scene uses long but protocol-valid names to exceed
			// OS TCP buffering, proving a truly non-reading peer blocks its writer while
			// other peers continue. The 1160 case uses ordinary-sized monster names.
			name := "dense monster"
			if total > 3072 {
				name = strings.Repeat("dense", 410)
			}
			defs := spawn.MapDefs{1001: domain.MonsterDef{ID: 1001, Name: name, Kind: domain.MonsterProp, Sprite: 101, Level: 1, HP: 100}}
			points := make([]domain.SpawnPoint, total)
			for i := range points {
				points[i] = domain.SpawnPoint{ID: int32(i + 1), Monster: 1001, Pos: domain.Pos{MapID: 28603, X: 4384, Y: 6688}}
			}
			alloc := domain.NewEntityAlloc()
			sp, skipped := spawn.New(points, defs, alloc)
			if skipped != 0 {
				t.Fatal(skipped)
			}
			frames := make(chan time.Time)
			logger := slog.Default() // Keep actor recovery diagnostics visible for this integration test.
			sc := scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: 28603}, Frames: frames, Alloc: alloc, Spawner: sp, Defs: defs, Log: logger})
			ctx, cancel := context.WithCancel(context.Background())
			sceneDone := make(chan struct{})
			go func() { sc.Run(ctx); close(sceneDone) }()
			t.Cleanup(func() {
				cancel()
				select {
				case <-sceneDone:
				case <-time.After(2 * time.Second):
					t.Error("scene did not stop")
				}
			})
			peers := 5
			conns := make([]*gateway.Conn, peers)
			readers := make([]*replicationTCPReader, peers)
			for i := 0; i < peers; i++ {
				read := total == 1160 || i != 4
				conns[i], readers[i] = dialReplicationTCP(t, addr, accepted, keys, read, i == 3)
				ch := &domain.Character{ID: int64(i + 1), Name: fmt.Sprint("peer", i), Level: 3, Pos: domain.Pos{MapID: 28603, X: 4384, Y: 6688}}
				sessionChar := *ch
				owner := &Session{stage: StageInGame, entity: domain.EntityID(i + 1), scene: domain.SceneID{MapID: 28603}, char: &sessionChar, sink: conns[i], log: logger, deps: Deps{Store: store.NewMemory(catalogfixture.EndgameCatalog()), Log: logger}}
				if !sc.Post(scene.Enter{ID: domain.EntityID(i + 1), Char: ch, Sink: &eventSink{out: conns[i], log: logger, observer: domain.EntityID(i + 1), sess: owner}}) {
					t.Fatal("enter rejected")
				}
			}
			ticks := 0
			maxTick := time.Duration(0)
			start := time.Now()
			deadline := start.Add(20 * time.Second)
			for {
				ready := true
				for i, r := range readers {
					if total > 3072 && i == 4 {
						continue
					}
					n, _, err := r.count()
					if err != nil {
						t.Fatalf("peer%d read: %v", i, err)
					}
					if n != total {
						ready = false
					}
					if conns[i].Closed() {
						t.Fatalf("healthy peer%d closed", i)
					}
				}
				stalledClosed := total == 1160 || conns[4].Closed()
				if ready && stalledClosed {
					break
				}
				if time.Now().After(deadline) {
					for i, r := range readers {
						n, f, e := r.count()
						t.Logf("peer%d monsters=%d frames=%d closed=%v err=%v", i, n, f, conns[i].Closed(), e)
					}
					t.Fatal("dense snapshot did not finish")
				}
				done := make(chan struct{})
				if !sc.Post(scene.Inspect{Fn: func(*scene.Scene) {}, Done: done}) {
					t.Fatal("inspect rejected")
				}
				before := time.Now()
				select {
				case frames <- time.Now():
				case <-time.After(time.Second):
					t.Fatal("one client blocked scene tick")
				}
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("scene actor did not respond")
				}
				elapsed := time.Since(before)
				if elapsed > maxTick {
					maxTick = elapsed
				}
				ticks++
				time.Sleep(time.Millisecond)
			}
			for i, r := range readers {
				if total > 3072 && i == 4 {
					continue
				}
				r.mu.Lock()
				for id, n := range r.monsters {
					if n != 1 {
						t.Errorf("peer%d duplicate monster%d: %d", i, id, n)
					}
				}
				r.mu.Unlock()
			}
			t.Logf("monsters=%d peers=%d ticks=%d maxActorResponse=%v duration=%v stalledClosed=%v", total, peers, ticks, maxTick, time.Since(start), conns[4].Closed())
		})
	}
}

func TestSceneSessionTCPRejectsCapturedOldEpoch(t *testing.T) {
	addr, accepted, keys := startReplicationTCPGateway(t)
	conn, reader := dialReplicationTCP(t, addr, accepted, keys, false, false)
	owner := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 28603}, mapEpoch: 4}
	sink := &eventSink{out: conn, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ev := event.EntitySpawned{ID: 100, Kind: domain.KindMonster, HP: 20, MaxHP: 20, Pos: domain.Pos{MapID: 28603}}
	packets := protocol.Encode(1, ev)
	binding, current := sink.emissionBinding()
	if !current {
		t.Fatal("missing original binding")
	}
	// Retain the exact encoded state a previous congested attempt would leave.
	sink.staged = &stagedEmission{packets: packets, bytes: len(packets[0]), atomic: true, binding: binding, transportEpoch: conn.PacedEpoch()}
	owner.mu.Lock()
	owner.mapEpoch++
	owner.scene.MapID = 28604
	owner.mu.Unlock()
	if sink.TryEmit(ev) {
		t.Fatal("old scene batch accepted after new binding")
	}
	marker := wire.Encode(protocol.SCTeleport, protocol.NewW(0).I32(28604).I32(100).I32(100).Bytes()[2:])
	conn.Send(marker)
	_ = reader.client.SetReadDeadline(time.Now().Add(time.Second))
	body, err := wire.ReadFrame(reader.client)
	if err != nil {
		t.Fatal(err)
	}
	var opener wire.Opener
	plain, err := opener.OpenInPlace(&keys, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != 8+len(marker) || binary.LittleEndian.Uint64(plain) != 0 || !bytes.Equal(plain[8:], marker) {
		t.Fatal("stale entity reached actual TCP before current-epoch marker")
	}
	sink.CancelPendingEmit()
}

func TestSceneSessionTCPRejectsCapturedOldTransportEpoch(t *testing.T) {
	addr, accepted, keys := startReplicationTCPGateway(t)
	conn, reader := dialReplicationTCP(t, addr, accepted, keys, false, false)
	owner := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 28603}, mapEpoch: 4}
	sink := &eventSink{out: conn, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ev := event.EntitySpawned{ID: 100, Kind: domain.KindMonster, HP: 20, MaxHP: 20, Pos: domain.Pos{MapID: 28603}}
	packets := protocol.Encode(1, ev)
	binding, current := sink.emissionBinding()
	if !current {
		t.Fatal("missing original binding")
	}
	// Retain the exact encoded state a previous congested attempt would leave.
	sink.staged = &stagedEmission{packets: packets, bytes: len(packets[0]), atomic: true, binding: binding, transportEpoch: conn.PacedEpoch()}
	if _, ok := conn.CancelPacedEpoch(conn.PacedEpoch()); !ok {
		t.Fatal("transport epoch retirement failed")
	}
	if sink.TryEmit(ev) {
		t.Fatal("old scene batch accepted after new binding")
	}
	marker := wire.Encode(protocol.SCTeleport, protocol.NewW(0).I32(28604).I32(100).I32(100).Bytes()[2:])
	conn.Send(marker)
	_ = reader.client.SetReadDeadline(time.Now().Add(time.Second))
	body, err := wire.ReadFrame(reader.client)
	if err != nil {
		t.Fatal(err)
	}
	var opener wire.Opener
	plain, err := opener.OpenInPlace(&keys, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != 8+len(marker) || binary.LittleEndian.Uint64(plain) != 0 || !bytes.Equal(plain[8:], marker) {
		t.Fatal("stale entity reached actual TCP before current-epoch marker")
	}
	sink.CancelPendingEmit()
}

func TestSceneSessionTCPTransferCancelsInitialSnapshot(t *testing.T) {
	addr, accepted, keys := startReplicationTCPGateway(t)
	conn, reader := dialReplicationTCP(t, addr, accepted, keys, true, false)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const source, target int32 = 28603, 28604
	r := &twoMapRig{t: t, frames: map[int32]chan time.Time{source: make(chan time.Time), target: make(chan time.Time)}}
	alloc := domain.NewEntityAlloc()
	defs := spawn.MapDefs{1001: domain.MonsterDef{ID: 1001, Name: "dense transfer monster", Kind: domain.MonsterProp, Sprite: 101, Level: 1, HP: 100}}
	r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		cfg := scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: r.frames[id.MapID], Alloc: alloc, Log: logger}
		if id.MapID == source {
			points := make([]domain.SpawnPoint, 4097)
			for i := range points {
				points[i] = domain.SpawnPoint{ID: int32(i + 1), Monster: 1001, Pos: domain.Pos{MapID: source, X: 4384, Y: 6688}}
			}
			cfg.Spawner, _ = spawn.New(points, defs, alloc)
			cfg.Defs = defs
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 1, Name: "source-npc", Sprite: 630, Script: "test", Pos: domain.Pos{MapID: source, X: 4384, Y: 6688}}}
		} else {
			cfg.NPCs = []domain.NPCSpawn{{NPCID: 2, Name: "target-npc", Sprite: 630, Script: "test", Pos: domain.Pos{MapID: target, X: 100, Y: 100}}}
		}
		return scene.New(cfg), nil
	}, logger)
	ctx, cancel := context.WithCancel(context.Background())
	r.router.Start(ctx)
	t.Cleanup(func() { cancel(); r.router.Drain(time.Second) })
	ch := &domain.Character{ID: 1, Name: "traveler", Level: 3, Pos: domain.Pos{MapID: source, X: 4384, Y: 6688}}
	sessionChar := *ch
	owner := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: source}, char: &sessionChar, sink: conn, log: logger, deps: Deps{Router: r.router, Store: store.NewMemory(catalogfixture.EndgameCatalog()), Log: logger}}
	es := &eventSink{out: conn, observer: 1, sess: owner, log: logger}
	if !r.router.Post(domain.SceneID{MapID: source}, scene.Enter{ID: 1, Char: ch, Sink: es}) {
		t.Fatal("enter rejected")
	}
	r.step(source)
	deadline := time.Now().Add(time.Second)
	for {
		n, _, err := reader.count()
		if err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			if n >= 4097 {
				t.Fatal("initial snapshot unexpectedly already complete")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial slice never arrived")
		}
		time.Sleep(time.Millisecond)
	}
	if !r.router.Post(domain.SceneID{MapID: source}, scene.Teleport{ID: 1, To: domain.SceneID{MapID: target}, At: domain.Pos{MapID: target, X: 100, Y: 100}}) {
		t.Fatal("teleport rejected")
	}
	for i := 0; i < 4; i++ {
		r.step(source)
	}
	r.step(target)
	owner.mu.Lock()
	bound, waiting := owner.scene, owner.awaitMapAck
	owner.mu.Unlock()
	if bound.MapID != target || !waiting {
		t.Fatalf("target handshake state=%v waiting=%v", bound, waiting)
	}
	owner.onMove(nativeReady(target, 100, 100))
	r.step(target)
	for i := 0; i < 4; i++ {
		r.step(source)
		r.step(target)
	}
	deadline = time.Now().Add(time.Second)
	for {
		reader.mu.Lock()
		tp, npc, stale := reader.teleports, reader.npcAfterTeleport, reader.staleAfterTeleport
		err := reader.err
		reader.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if tp == 1 && npc == 1 {
			if stale != 0 {
				t.Fatalf("%d old world frames crossed target teleport", stale)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("teleports=%d targetNPCs=%d stale=%d", tp, npc, stale)
		}
		time.Sleep(time.Millisecond)
	}
	assertNativeOwner(t, owner, domain.SceneID{MapID: target})
	if conn.Closed() {
		t.Fatal("healthy transfer connection closed")
	}
}

// The scene producer's actual Rift/AOI semantics are tested in scene. This test
// independently proves the complete global Aura payload survives the production
// eventSink, native schema, encryption, TCP and existing page reassembly without
// silently trimming it to the set of spawned/local entities.
func TestSessionTCPGlobalRiftAurasAndNativePages(t *testing.T) {
	for _, paged := range []bool{false, true} {
		t.Run(fmt.Sprint("paged=", paged), func(t *testing.T) {
			addr, accepted, keys := startReplicationTCPGateway(t)
			conn, reader := dialReplicationTCP(t, addr, accepted, keys, false, false)
			// This client reads complete global pages promptly (not the stalled-peer fixture).
			if err := reader.client.(*net.TCPConn).SetReadBuffer(512 << 10); err != nil {
				t.Fatal(err)
			}
			owner := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 28603}, mapEpoch: 1, endgameV1: true}
			sink := &eventSink{out: conn, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			response := domain.EndgameResponse{Kind: "rift.state", OK: true, RequestID: "deidentified-global-state", Revision: "15", Rift: &domain.EndgameRiftView{RunID: "synthetic-dense", Revision: "15", Phase: "active", MapID: 28603, Seed: "123", GeneratorVersion: 6, ProgressMax: 1000}}
			for i := 0; i < 1160; i++ {
				aura := domain.EndgameRiftAuraView{EntityID: strconv.Itoa(i + 100), Class: 0}
				if paged {
					aura.Class = 3
					aura.AbilityIDs = []int32{0, 3, 6}
					aura.GoldenUntilMS = 123456
					aura.OriginalName = strings.Repeat("synthetic monster ", 4)
					aura.Affixes = []string{"first", "second", "third"}
				}
				response.Rift.Auras = append(response.Rift.Auras, aura)
			}
			original, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			pages, err := domain.PageEndgameRift(response, "synthetic-transfer")
			if err != nil {
				t.Fatal(err)
			}
			if paged && len(pages) < 2 || !paged && len(pages) != 1 {
				t.Fatalf("unexpected pages %d bytes%d", len(pages), len(original))
			}
			for _, page := range pages {
				ev := event.EndgameMessage{Who: 1, Message: page}
				deadline := time.Now().Add(time.Second)
				for !sink.TryEmit(ev) {
					if conn.Closed() || time.Now().After(deadline) {
						t.Fatal("global page admission failed")
					}
					time.Sleep(time.Millisecond)
				}
			}
			var opener wire.Opener
			var reassembled []byte
			for i := range pages {
				_ = reader.client.SetReadDeadline(time.Now().Add(3 * time.Second))
				body, err := wire.ReadFrame(reader.client)
				if err != nil {
					t.Fatal(err)
				}
				plain, err := opener.OpenInPlace(&keys, body)
				if err != nil {
					t.Fatal(err)
				}
				if len(plain) < 14 || binary.LittleEndian.Uint64(plain) != uint64(i) {
					t.Fatal("bad native page frame")
				}
				packet := plain[8:]
				if !bytes.Equal(packet, protocol.Encode(1, event.EndgameMessage{Who: 1, Message: pages[i]})[0]) {
					t.Fatal("TCP changed native encoded page")
				}
				if binary.LittleEndian.Uint16(packet[2:4]) != domain.EndgameSchema {
					t.Fatal("schema changed")
				}
				n := int(binary.LittleEndian.Uint16(packet[4:6]))
				if n > domain.EndgameMaxJSONBytes || len(packet) != 6+n {
					t.Fatal("native JSON frame budget changed")
				}
				var got domain.EndgameResponse
				if err := json.Unmarshal(packet[6:], &got); err != nil {
					t.Fatal(err)
				}
				if got.Kind == "rift.page" {
					if got.Chunk != int32(i) || got.Chunks != int32(len(pages)) || got.TotalBytes != int32(len(original)) {
						t.Fatal("page envelope changed")
					}
					part, err := base64.StdEncoding.DecodeString(got.Data)
					if err != nil {
						t.Fatal(err)
					}
					reassembled = append(reassembled, part...)
				} else {
					reassembled = append(reassembled, packet[6:]...)
				}
			}
			if !bytes.Equal(reassembled, original) {
				t.Fatal("global state was filtered or changed")
			}
			var decoded domain.EndgameResponse
			if err := json.Unmarshal(reassembled, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Rift == nil || len(decoded.Rift.Auras) != 1160 {
				t.Fatal("full Aura population lost")
			}
			t.Logf("auras=%d rawBytes=%d nativeFrames=%d", len(decoded.Rift.Auras), len(original), len(pages))
		})
	}
}

func TestSessionTCPImmediateGlobalBurstPreservesLegacyFIFO(t *testing.T) {
	for _, slow := range []bool{false, true} {
		t.Run(fmt.Sprint("slow=", slow), func(t *testing.T) {
			addr, accepted, keys := startReplicationTCPGatewayWithWriteWait(t, 10*time.Second)
			conn, reader := dialReplicationTCP(t, addr, accepted, keys, false, false)
			if err := reader.client.(*net.TCPConn).SetReadBuffer(512 << 10); err != nil {
				t.Fatal(err)
			}
			owner := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 28603}, mapEpoch: 1, endgameV1: true}
			sink := &eventSink{out: conn, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			events := globalBudgetFixture(t)
			var expected [][]byte
			total := 0
			for _, e := range events {
				for _, p := range protocol.Encode(1, e) {
					if len(p) > wire.MaxFrame-64 {
						t.Fatal("fixture exceeds native frame budget")
					}
					expected = append(expected, p)
					total += len(p)
				}
			}
			done := make(chan error, 1)
			go func() {
				var opener wire.Opener
				for i, p := range expected {
					if slow {
						time.Sleep(2 * time.Millisecond)
					}
					_ = reader.client.SetReadDeadline(time.Now().Add(5 * time.Second))
					body, err := wire.ReadFrame(reader.client)
					if err != nil {
						done <- err
						return
					}
					plain, err := opener.OpenInPlace(&keys, body)
					if err != nil {
						done <- err
						return
					}
					if len(plain) < 8 || binary.LittleEndian.Uint64(plain) != uint64(i) || !bytes.Equal(plain[8:], p) {
						done <- fmt.Errorf("global frame%d changed or overtook", i)
						return
					}
				}
				done <- nil
			}()
			started := time.Now()
			for _, e := range events {
				sink.Emit(e)
			} // original immediate non-world/control path
			admission := time.Since(started)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("global burst incomplete")
			}
			if conn.Closed() {
				t.Fatal("healthy global-only burst closed")
			}
			t.Logf("globalFrames=%d innerBytes=%d immediateAdmission=%v total=%v", len(expected), total, admission, time.Since(started))
		})
	}
}

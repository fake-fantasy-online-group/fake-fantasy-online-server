package session

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// These are actual production Session/Router/eventSink payloads. The client test
// replays each FIFO batch after its real iterator emits the corresponding ready;
// it must not invent a teleport or compensating second snapshot.
type nativePortalWireTrace struct {
	Name           string     `json:"name"`
	LocalFirst     bool       `json:"local_first"`
	MapID          int32      `json:"map_id"`
	X              int32      `json:"x"`
	Y              int32      `json:"y"`
	ExpectedBuilds int        `json:"expected_builds"`
	FinalMapID     int32      `json:"final_map_id"`
	Batches        [][][]byte `json:"batches"`
}

func nativeTraceSince(sink *capSink, offset *int) [][]byte {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	packets := make([][]byte, 0, len(sink.pkts)-*offset)
	for _, p := range sink.pkts[*offset:] {
		packets = append(packets, append([]byte(nil), p...))
	}
	*offset = len(sink.pkts)
	return packets
}

func TestNativePortalExportClientIteratorTrace(t *testing.T) {
	var traces []nativePortalWireTrace
	for _, kind := range []string{"client_gate", "client_gate_no_final_move", "client_gate_late_source_ground", "server_only_gate", "server_active", "server_active_then_same_landing"} {
		for _, same := range []bool{false, true} {
			if kind == "client_gate_late_source_ground" && same {
				continue // Same scene retains this item; it is not stale source data.
			}
			name := fmt.Sprintf("%s_same_%v", kind, same)
			t.Run(name, func(t *testing.T) {
				lateGround := kind == "client_gate_late_source_ground"
				local := kind == "client_gate" || kind == "client_gate_no_final_move" || lateGround
				r, s, sink := nativePortalFixture(t, local, same)
				mapID, x, y := int32(285), int32(100), int32(100)
				if same {
					mapID, x, y = 1, 200, 200
				}
				offset := 0
				if lateGround {
					r.enter(1, 3, 300, "source bystander")
					r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
						sc.EntityAt(3).Player.Bag.Set(0, domain.Stack{Item: 4103, Count: 1})
					}, Done: make(chan struct{})})
					r.step(1)
				}
				nativeTraceSince(sink, &offset) // fixture setup is not this transition's downlink
				active := kind == "server_active" || kind == "server_active_then_same_landing"
				teleport := scene.Teleport{ID: 1, To: domain.SceneID{MapID: mapID}, At: domain.Pos{MapID: mapID, X: float64(x), Y: float64(y)}}
				if active {
					r.router.Post(domain.SceneID{MapID: 1}, teleport)
				} else if lateGround {
					r.router.Post(domain.SceneID{MapID: 1}, scene.DropBagItem{ID: 3, Tab: 1, Slot: 0, Item: 4103, At: domain.Pos{MapID: 1, X: 100, Y: 100}})
				} else if kind != "client_gate_no_final_move" {
					s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 110})
				}
				r.step(1)
				if !same {
					r.step(mapID)
				}
				trace := nativePortalWireTrace{Name: name, LocalFirst: local, MapID: mapID, X: x, Y: y, ExpectedBuilds: 1, FinalMapID: mapID}
				trace.Batches = append(trace.Batches, nativeTraceSince(sink, &offset))
				for _, p := range trace.Batches[0] {
					op := binary.LittleEndian.Uint16(p)
					if local && op == protocol.SCTeleport {
						t.Fatal("client-owned gate emitted a forced teleport before ready")
					}
					if op == protocol.SCRemotePlayer || op == protocol.SCNPCSpawn || op == protocol.SCEntitySpawn || op == protocol.SCGroundSpawn && !lateGround {
						t.Fatalf("snapshot %04x preceded client's first ready", op)
					}
				}
				s.onMove(nativeReady(mapID, x, y))
				r.step(1)
				if !same {
					r.step(mapID)
				}
				assertNativeOwner(t, s, domain.SceneID{MapID: mapID})
				if lateGround {
					for _, op := range []uint16{protocol.SCRemotePlayer, protocol.SCNPCSpawn, protocol.SCEntitySpawn, protocol.SCGroundSpawn} {
						want := 1
						if op == protocol.SCGroundSpawn {
							want = 2
						}
						// Earlier bystander entry was excluded from the trace offset.
						if op == protocol.SCRemotePlayer {
							want = 2
						}
						if got := mapReviewWireCount(sink, op); got != want {
							t.Fatalf("late ground snapshot %04x=%d want%d", op, got, want)
						}
					}
				} else {
					assertNativeSnapshot(t, sink, 1)
				}
				if kind == "server_active_then_same_landing" {
					// This really is a second server operation with identical coordinates.
					// Its new 803a follows the first snapshot, still within the first fade.
					r.router.Post(domain.SceneID{MapID: mapID}, teleport)
					r.step(mapID)
					trace.Batches = append(trace.Batches, nativeTraceSince(sink, &offset))
					s.onMove(nativeReady(mapID, x, y))
					r.step(mapID)
					assertNativeOwner(t, s, domain.SceneID{MapID: mapID})
					assertNativeSnapshot(t, sink, 2)
					trace.ExpectedBuilds = 2
				}
				trace.Batches = append(trace.Batches, nativeTraceSince(sink, &offset))
				traces = append(traces, trace)
				t.Logf("production wire trace %s: ready=%d batches=%d teleport=%d first_entity_kinds=4", name, trace.ExpectedBuilds, len(trace.Batches), mapReviewWireCount(sink, protocol.SCTeleport))
			})
		}
	}
	t.Run("client_gate_late_source_npc_and_ground", func(t *testing.T) {
		packets, old := nativePortalLateMaplessPackets(t)
		boundary := -1
		for i, p := range packets {
			if len(p) >= 6 {
				if op, source := old[binary.LittleEndian.Uint32(p[2:])]; source && op == binary.LittleEndian.Uint16(p) {
					boundary = i + 1 // helper emits its final source drop before submitting target ready
				}
			}
		}
		if boundary < 1 || boundary == len(packets) {
			t.Fatal("source/target trace boundary missing")
		}
		traces = append(traces, nativePortalWireTrace{Name: "client_gate_late_source_npc_and_ground", LocalFirst: true, MapID: 285, X: 100, Y: 100, ExpectedBuilds: 1, FinalMapID: 285, Batches: [][][]byte{packets[:boundary], packets[boundary:]}})
	})
	if path := os.Getenv("FFO_MAP_TRACE_PATH"); path != "" {
		data, err := json.MarshalIndent(struct {
			SourceCommit string                  `json:"source_commit"`
			SourceSHA256 string                  `json:"source_sha256"`
			Traces       []nativePortalWireTrace `json:"traces"`
		}{os.Getenv("FFO_MAP_TRACE_SOURCE_COMMIT"), os.Getenv("FFO_MAP_TRACE_SOURCE_SHA256"), traces}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

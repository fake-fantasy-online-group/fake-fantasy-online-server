package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// This is deliberately a real, running Router/Scene with a full bounded mailbox.
// A nil router, a stopped scene, or a fabricated EndgameResponse would miss the
// actual congestion path used by a fresh reconnect's capability negotiation.
func TestRiftHelloBusyFromFullMailbox(t *testing.T) {
	out := &capSink{}
	frames := make(chan time.Time)
	router := scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		return scene.New(scene.Config{ID: id, Frames: frames, Mailbox: 1}), nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	router.Start(ctx)
	s := New(out, Deps{Router: router})
	s.stage, s.entity, s.scene = StageInGame, 900000, domain.SceneID{MapID: 7}

	// Optional cross-package fixture replay is explicitly sourced from the actual
	// seeded private Enter above. eventSink, rather than this test, creates the
	// 8033/8003 bootstrap and rebinds the Session from the captured SelfEntered.
	dir := os.Getenv("RIFT_CONTEXT_BUSY_FIXTURE_DIR")
	if dir != "" {
		data, err := os.ReadFile(filepath.Join(dir, "fresh-reconnect-self-entered.json"))
		if err != nil {
			t.Fatalf("run TestRiftFreshReconnectHasNoContextUntilSuccessfulHello with the same fixture directory first: %v", err)
		}
		var entered event.SelfEntered
		if err := json.Unmarshal(data, &entered); err != nil {
			t.Fatal(err)
		}
		if entered.Char == nil || entered.Scene.Persistent() {
			t.Fatal("fixture is not a captured private SelfEntered")
		}
		s.char = entered.Char
		(&eventSink{out: out, log: s.log, sess: s, observer: entered.ID}).Emit(entered)
		if s.scene != entered.Scene || s.entity != entered.ID {
			t.Fatal("event sink did not rebind from the real private entry")
		}
	}

	enteredBarrier, release, barrierDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		cancel()
		if err := router.Drain(time.Second); err != nil {
			t.Error(err)
		}
	})
	if !router.Post(s.scene, scene.Inspect{Fn: func(*scene.Scene) { close(enteredBarrier); <-release }, Done: barrierDone}) {
		t.Fatal("blocking inspect was not queued")
	}
	select {
	case frames <- time.Unix(1000, 0):
	case <-enteredBarrier:
	case <-time.After(2 * time.Second):
		t.Fatal("actor frame did not start")
	}
	select {
	case <-enteredBarrier:
	case <-time.After(2 * time.Second):
		t.Fatal("actor did not enter inspect barrier")
	}
	queuedDone := make(chan struct{})
	if !router.Post(s.scene, scene.Inspect{Fn: func(*scene.Scene) {}, Done: queuedDone}) {
		t.Fatal("ordinary bounded mailbox did not accept its one slot")
	}
	if router.Post(s.scene, scene.Inspect{Fn: func(*scene.Scene) {}, Done: make(chan struct{})}) {
		t.Fatal("ordinary mailbox is not genuinely full")
	}

	out.mu.Lock()
	bootstrapCount := len(out.pkts)
	out.mu.Unlock()
	for attempt := 1; attempt <= 3; attempt++ {
		req := domain.EndgameRequest{Op: "hello", RequestID: fmt.Sprintf("rift-reconnect-busy-%d", attempt), Capabilities: []string{domain.EndgameV1, domain.EndgameRiftPortalsV1}}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		payload := protocol.NewW(0).U16(domain.EndgameSchema).Str(string(data)).Bytes()[2:]
		decoded, ok := protocol.DecodeEndgame(payload)
		if !ok || decoded.Op != "hello" || decoded.RequestID != req.RequestID {
			t.Fatal("real hello decoder rejected fixture")
		}
		s.OnPacket(protocol.OpEndgame, payload)
		out.mu.Lock()
		var packets [][]byte
		for _, packet := range out.pkts[bootstrapCount:] {
			if binary.LittleEndian.Uint16(packet) == protocol.SCEndgame {
				packets = append(packets, packet)
			}
		}
		out.mu.Unlock()
		if len(packets) != 2*attempt {
			t.Fatalf("hello %d did not synchronously produce admission-pending then busy: %d", attempt, len(packets))
		}
		responses := nativeStatsResponses(t, packets)
		admission := responses[len(responses)-2]
		if admission.Kind != "hello" || admission.RequestID != req.RequestID || admission.OK || admission.Error != "pending" || len(admission.Capabilities) != 0 || len(admission.Result) != 0 {
			t.Fatalf("missing ordered transport admission before busy: %+v", admission)
		}
		response := responses[len(responses)-1]
		if response.Kind != "hello" || response.RequestID != req.RequestID || response.OK || response.Error != "busy" || len(response.Capabilities) != 0 || len(response.Result) != 0 {
			t.Fatalf("busy hello unexpectedly supplied ownership: %+v", response)
		}
		if !bytes.Equal(packets[len(packets)-1], protocol.EncodeEndgame(response)) {
			t.Fatal("captured packet differs from production EncodeEndgame")
		}
		if dir != "" {
			packet := packets[len(packets)-1]
			for name, content := range map[string][]byte{
				fmt.Sprintf("hello-admission-pending-%d.bin", attempt):         packets[len(packets)-2],
				fmt.Sprintf("hello-admission-pending-%d.payload.bin", attempt): packets[len(packets)-2][2:],
				fmt.Sprintf("hello-busy-%d.bin", attempt):                      packet,
				fmt.Sprintf("hello-busy-%d.payload.bin", attempt):              packet[2:],
				fmt.Sprintf("hello-request-%d.payload.bin", attempt):           payload,
			} {
				if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if attempt == 1 {
				if err := os.WriteFile(filepath.Join(dir, "hello-busy.bin"), packet, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if !s.endgameEnabled() || s.growthReady() || s.awaitMapAck {
		t.Fatal("busy transport negotiation changed world/map readiness unexpectedly")
	}
	select {
	case <-queuedDone:
		t.Fatal("scene was not held throughout all three hello attempts")
	default:
	}
	if dir != "" {
		out.mu.Lock()
		packets := append([][]byte(nil), out.pkts...)
		out.mu.Unlock()
		if len(packets) < 8 || binary.LittleEndian.Uint16(packets[0]) != protocol.SCPartySnapshot || binary.LittleEndian.Uint16(packets[1]) != protocol.SCSelfData {
			t.Fatal("real event sink did not emit 8033 then 8003 before hello")
		}
		var fifo bytes.Buffer
		for i, packet := range packets {
			if err := binary.Write(&fifo, binary.LittleEndian, uint32(len(packet))); err != nil {
				t.Fatal(err)
			}
			fifo.Write(packet)
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("fifo-%02d-%04x.bin", i, binary.LittleEndian.Uint16(packet))), packet, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "session-fifo.u32le.bin"), fifo.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("three genuine full-mailbox hello attempts returned admission-pending then busy without capabilities/context; bootstrap packets=%d, scene=%v", bootstrapCount, s.scene)
}

// Distinguish slow storage from an actor that has not executed accepted work.
// This uses the existing live-player Session/Router rig. While another actor
// command is held and ordinary mailbox capacity remains, Session queues all
// three requests and must acknowledge supported transport immediately. Once execution
// resumes, the unresolved Load produces pending on retries, as in the scene
// test. No public/private identity is inferred from this transport-only case.
func TestRiftHelloAcceptedMailboxReportsPendingBeforeActorRuns(t *testing.T) {
	r := newNativeStatsRig(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	if !r.router.Post(r.s.scene, scene.Inspect{Fn: func(*scene.Scene) { close(entered); <-release }, Done: done}) {
		t.Fatal("blocking actor command not queued")
	}
	select {
	case r.frames <- time.Unix(1000, 0):
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("actor did not start")
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("actor did not reach barrier")
	}
	for attempt := 1; attempt <= 3; attempt++ {
		req := domain.EndgameRequest{Op: "hello", RequestID: fmt.Sprintf("rift-reconnect-queued-%d", attempt), Capabilities: []string{domain.EndgameV1, domain.EndgameRiftPortalsV1}}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		r.s.OnPacket(protocol.OpEndgame, protocol.NewW(0).U16(domain.EndgameSchema).Str(string(data)).Bytes()[2:])
		packets := r.take()
		responses := nativeStatsResponses(t, packets)
		if len(responses) != 1 || responses[0].Kind != "hello" || responses[0].RequestID != req.RequestID || responses[0].OK || responses[0].Error != "pending" || len(responses[0].Capabilities) != 0 || len(responses[0].Result) != 0 {
			t.Fatalf("queued hello %d needs immediate admission-pending, without ownership: %+v", attempt, responses)
		}
		if dir := os.Getenv("RIFT_CONTEXT_BUSY_FIXTURE_DIR"); dir != "" {
			for _, packet := range packets {
				if binary.LittleEndian.Uint16(packet) == protocol.SCEndgame {
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("queued-hello-pending-%d.payload.bin", attempt)), packet[2:], 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	t.Log("all three Session.OnPacket hello requests immediately emitted pending before the blocked actor could process them")
	once.Do(func() { close(release) })
	r.sync()
	responses := nativeStatsResponses(t, r.take())
	if len(responses) != 2 {
		t.Fatalf("resumed actor should report pending for the two retries: %+v", responses)
	}
	for i, response := range responses {
		if response.Kind != "hello" || response.Error != "pending" || response.OK || response.RequestID != fmt.Sprintf("rift-reconnect-queued-%d", i+2) {
			t.Fatalf("resumed actor response is not a pending retry: %+v", response)
		}
	}
	r.backend.loads <- domain.EndgameLoadResult{}
	r.sync()
	packets := r.take()
	responses = nativeStatsResponses(t, packets)
	if len(responses) != 3 || responses[0].Kind != "hello" || !responses[0].OK || responses[0].RequestID != "rift-reconnect-queued-1" || responses[1].Kind != "rift.context" || responses[2].Kind != "snapshot" {
		t.Fatalf("resumed backend did not finish the original queued hello: %+v", responses)
	}
	if dir := os.Getenv("RIFT_CONTEXT_BUSY_FIXTURE_DIR"); dir != "" {
		for _, packet := range packets {
			if binary.LittleEndian.Uint16(packet) != protocol.SCEndgame {
				continue
			}
			response := nativeStatsResponses(t, [][]byte{packet})[0]
			if err := os.WriteFile(filepath.Join(dir, "queued-"+response.Kind+"-success.payload.bin"), packet[2:], 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	r.s.OnPacket(protocol.OpEndgame, protocol.NewW(0).U16(domain.EndgameSchema).Str(`{"op":"snapshot","requestId":"not-a-hello"}`).Bytes()[2:])
	r.sync()
	responses = nativeStatsResponses(t, r.take())
	if len(responses) != 1 || responses[0].Kind != "snapshot" || !responses[0].OK {
		t.Fatalf("non-hello request acquired an admission response: %+v", responses)
	}
}

func TestRiftHelloAdmissionDoesNotBypassValidationOrWorldGate(t *testing.T) {
	valid := protocol.NewW(0).U16(domain.EndgameSchema).Str(`{"op":"hello","requestId":"admission-validation","capabilities":["endgame.v1","endgame.rift-portals.v1"]}`).Bytes()[2:]
	for _, tc := range []struct {
		name      string
		stage     Stage
		loading   bool
		payload   []byte
		wantError string
	}{
		{name: "connected", stage: StageConnected, payload: valid},
		{name: "authed", stage: StageAuthed, payload: valid},
		{name: "closed", stage: StageClosed, payload: valid},
		{name: "map-loading", stage: StageInGame, loading: true, payload: valid},
		{name: "non-hello", stage: StageInGame, payload: protocol.NewW(0).U16(domain.EndgameSchema).Str(`{"op":"snapshot"}`).Bytes()[2:]},
		{name: "malformed", stage: StageInGame, payload: []byte{1, 0, 255}},
		{name: "unsupported", stage: StageInGame, payload: protocol.NewW(0).U16(domain.EndgameSchema).Str(`{"op":"hello","requestId":"admission-validation","capabilities":["endgame.rift-portals.v1"]}`).Bytes()[2:], wantError: "unsupported_capability"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := &capSink{}
			s := New(out, Deps{})
			s.stage, s.awaitMapAck = tc.stage, tc.loading
			s.OnPacket(protocol.OpEndgame, tc.payload)
			responses := nativeStatsResponses(t, out.pkts)
			if tc.wantError == "" {
				if len(out.pkts) != 0 {
					t.Fatalf("unauthorized or invalid hello leaked a response: %+v", responses)
				}
			} else if len(responses) != 1 || responses[0].Error != tc.wantError || responses[0].OK || len(responses[0].Capabilities) != 0 || len(responses[0].Result) != 0 {
				t.Fatalf("unsupported hello acquired new admission behavior: %+v", responses)
			}
			if s.endgameV1 || s.growthV5 {
				t.Fatal("invalid hello enabled transport or gameplay")
			}
		})
	}
}

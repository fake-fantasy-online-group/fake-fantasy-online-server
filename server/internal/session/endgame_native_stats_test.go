package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// Exercise the real packet entry, router, scene actor, event gate and wire
// encoder together. A direct dispatchEndgame test cannot catch a Session
// handler that acknowledges hello itself and never reaches the scene.
type nativeStatsBackend struct{ loads chan domain.EndgameLoadResult }

func (b *nativeStatsBackend) Load(int64) <-chan domain.EndgameLoadResult { return b.loads }
func (*nativeStatsBackend) LoadReceipt(int64, string) <-chan domain.EndgameReceiptLoadResult {
	panic("hello must not load a mutation receipt")
}
func (*nativeStatsBackend) LoadRun(string) <-chan domain.EndgameRunLoadResult {
	panic("hello must not load a run")
}
func (*nativeStatsBackend) Commit(domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	panic("hello must not write storage")
}

type nativeStatsRig struct {
	t       *testing.T
	s       *Session
	out     *capSink
	router  *scene.Router
	frames  chan time.Time
	backend *nativeStatsBackend
}

func newNativeStatsRig(t *testing.T, results ...domain.EndgameLoadResult) *nativeStatsRig {
	t.Helper()
	r := &nativeStatsRig{t: t, out: &capSink{}, frames: make(chan time.Time), backend: &nativeStatsBackend{loads: make(chan domain.EndgameLoadResult, 8)}}
	for _, result := range results {
		r.backend.loads <- result
	}
	items := map[domain.ItemID]domain.ItemDef{}
	for _, def := range domain.AstralRuneItems(7) {
		items[def.ID] = def
	}
	def := domain.ItemDef{ID: 999001, Name: "测试剑", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment,
		Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), SocketLimit: 5, Durable: 100, Bonus: domain.Stats{MinAtk: 100, MaxAtk: 200}}}
	items[def.ID] = def
	st := domain.NewStack(def, 1)
	st.UID, st.Endgame.FormationRollBP = 81, 12000
	recipe, _ := domain.AstralRecipeByID(11)
	st.SocketCount = uint8(len(recipe.Runes))
	for i, id := range recipe.Runes {
		st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = id, int64(100+i), items[id].CardDefaults
	}
	if _, ok := domain.MatchAstralRecipe(def, st); !ok {
		t.Fatal("invalid formation fixture")
	}
	bag := domain.NewBag(domain.DefaultBagSlots)
	bag.Set(200, st)
	worn := domain.NewEquipSet()
	st.UID = 82
	worn.Set(domain.SlotWeapon, st)
	r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		return scene.New(scene.Config{ID: id, Frames: r.frames, Items: items, EndgameBackend: r.backend}), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	r.router.Start(ctx)
	t.Cleanup(func() {
		select {
		case r.backend.loads <- domain.EndgameLoadResult{}:
		default:
		}
		cancel()
		r.router.Drain(time.Second)
	})
	r.s = New(r.out, Deps{Router: r.router})
	r.s.stage, r.s.entity, r.s.scene = StageInGame, 1, domain.SceneID{MapID: 1}
	r.s.char = &domain.Character{ID: 100, Name: "甲", Race: domain.Warrior, Level: 60, Pos: domain.Pos{MapID: 1}}
	if !r.router.Post(r.s.scene, scene.Enter{ID: 1, Char: r.s.char, Bag: bag, Worn: worn,
		Sink: &eventSink{out: r.out, log: r.s.log, sess: r.s, observer: 1}}) {
		t.Fatal("enter not queued")
	}
	r.sync()
	r.take()
	return r
}
func (r *nativeStatsRig) frame() {
	r.t.Helper()
	select {
	case r.frames <- time.Now():
	case <-time.After(2 * time.Second):
		r.t.Fatal("scene frame timed out")
	}
}
func (r *nativeStatsRig) inspect(fn func(*scene.Scene)) {
	r.t.Helper()
	done := make(chan struct{})
	if !r.router.Post(r.s.scene, scene.Inspect{Fn: fn, Done: done}) {
		r.t.Fatal("inspect not queued")
	}
	r.frame()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		r.t.Fatal("scene inspect timed out")
	}
}
func (r *nativeStatsRig) sync() {
	// A second frame starts only after the previous frame's flush has finished.
	r.frame()
	r.inspect(func(*scene.Scene) {})
}
func (r *nativeStatsRig) take() [][]byte {
	r.out.mu.Lock()
	defer r.out.mu.Unlock()
	packets := r.out.pkts
	r.out.pkts = nil
	return packets
}
func nativeStatsHello(native bool) []byte {
	caps := []string{domain.EndgameV1}
	if native {
		caps = append(caps, domain.EndgameNativeStatsV1)
	}
	b, _ := json.Marshal(domain.EndgameRequest{Op: "hello", RequestID: "native-stats-wire-hello", Capabilities: caps})
	return protocol.NewW(0).U16(domain.EndgameSchema).Str(string(b)).Bytes()[2:]
}
func (r *nativeStatsRig) hello(native bool) [][]byte {
	r.s.OnPacket(protocol.OpEndgame, nativeStatsHello(native))
	r.sync()
	return r.take()
}
func nativeStatsResponses(t *testing.T, packets [][]byte) []domain.EndgameResponse {
	t.Helper()
	var responses []domain.EndgameResponse
	for _, packet := range packets {
		if binary.LittleEndian.Uint16(packet) != protocol.SCEndgame {
			continue
		}
		if len(packet) < 6 || int(binary.LittleEndian.Uint16(packet[4:])) != len(packet)-6 {
			t.Fatal("invalid sidecar packet")
		}
		var response domain.EndgameResponse
		if err := json.Unmarshal(packet[6:], &response); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
	return responses
}
func hasNativeStats(response domain.EndgameResponse) bool {
	for _, capability := range response.Capabilities {
		if capability == domain.EndgameNativeStatsV1 {
			return true
		}
	}
	return false
}
func assertNativeStatsHandshake(t *testing.T, packets [][]byte, native, refresh bool) {
	t.Helper()
	responses := nativeStatsResponses(t, packets)
	if len(responses) != 2 || responses[0].Kind != "hello" || !responses[0].OK || responses[1].Kind != "snapshot" || !responses[1].OK || hasNativeStats(responses[0]) != native {
		t.Fatalf("expected exactly one authoritative hello then snapshot (native=%v): %+v", native, responses)
	}
	inventory, hello, snapshot := -1, -1, -1
	for i, packet := range packets {
		op := binary.LittleEndian.Uint16(packet)
		if op == protocol.SCInventory || op == protocol.SCInventoryDelta {
			inventory = i
		}
		if op == protocol.SCEndgame {
			if hello < 0 {
				hello = i
			} else {
				snapshot = i
			}
		}
	}
	if refresh && (inventory < 0 || inventory >= hello || hello >= snapshot) {
		t.Fatalf("native inventory must precede capability echo and sidecar: inventory=%d hello=%d snapshot=%d", inventory, hello, snapshot)
	}
	if !refresh {
		return
	}
	packet := packets[inventory]
	if native {
		// Same flat/global scope sums 3 + 18 once for each bag/equipped item.
		for _, line := range []string{"\x034力量 +21\n", "\x034攻击 +3\n", "\x034攻击 +42%\n", "\x034攻击强化 +110%（本件）\n", "\x030攻击 210~420\n"} {
			if bytes.Count(packet, []byte(line)) != 2 {
				t.Fatalf("bag/equipped merged/local/global line missing or repeated: %q in %q", line, packet)
			}
		}
		if bytes.Contains(packet, []byte("力量 +3\n")) || bytes.Contains(packet, []byte("力量 +18\n")) {
			t.Fatal("unmerged rune or recipe value leaked into native tooltip")
		}
	} else if bytes.Count(packet, []byte("力量 +3\n")) != 2 || bytes.Contains(packet, []byte("力量 +21\n")) {
		t.Fatal("legacy client did not regain legacy descriptions")
	}
}

func TestEndgameNativeStatsThroughSessionWire(t *testing.T) {
	t.Run("first-native-hello", func(t *testing.T) {
		r := newNativeStatsRig(t, domain.EndgameLoadResult{})
		assertNativeStatsHandshake(t, r.hello(true), true, true)
	})
	r := newNativeStatsRig(t, domain.EndgameLoadResult{})
	assertNativeStatsHandshake(t, r.hello(false), false, false)
	assertNativeStatsHandshake(t, r.hello(true), true, true)
	assertNativeStatsHandshake(t, r.hello(true), true, false) // unchanged native inventory may be delta-suppressed
	assertNativeStatsHandshake(t, r.hello(false), false, true)
	assertNativeStatsHandshake(t, r.hello(true), true, true)
}
func TestEndgameNativeStatsSessionRetriesLoadAndRefreshFailures(t *testing.T) {
	for _, failure := range []string{"storage", "busy"} {
		t.Run(failure, func(t *testing.T) {
			results := []domain.EndgameLoadResult{{}}
			if failure == "storage" {
				results = []domain.EndgameLoadResult{{Err: errors.New("injected load failure")}, {}}
			}
			r := newNativeStatsRig(t, results...)
			if failure == "busy" {
				r.inspect(func(sc *scene.Scene) { sc.EntityAt(1).Player.Bag.Set(201, domain.Stack{Item: 987654, Count: 1}) })
			}
			packets := r.hello(true)
			responses := nativeStatsResponses(t, packets)
			if len(responses) != 1 || responses[0].Kind != "hello" || responses[0].OK || responses[0].Error != failure || len(responses[0].Capabilities) != 0 {
				t.Fatalf("failed hello must reach the client without capability confirmation: %+v", responses)
			}
			r.inspect(func(sc *scene.Scene) {
				p := sc.EntityAt(1)
				if p.Player.EndgameNativeStats {
					t.Error("failed hello enabled native formatting")
				}
				if failure == "busy" {
					p.Player.Bag.Set(201, domain.Stack{})
				}
			})
			assertNativeStatsHandshake(t, r.hello(true), true, true)
		})
	}
}
func TestEndgameNativeStatsSessionPendingHelloDoesNotConfirmEarly(t *testing.T) {
	r := newNativeStatsRig(t)
	if packets := nativeStatsResponses(t, r.hello(true)); len(packets) != 0 {
		t.Fatalf("hello confirmed before storage finished: %+v", packets)
	}
	responses := nativeStatsResponses(t, r.hello(true))
	if len(responses) != 1 || responses[0].Error != "pending" || responses[0].OK || len(responses[0].Capabilities) != 0 {
		t.Fatalf("pending response was hidden or advertised success: %+v", responses)
	}
	r.backend.loads <- domain.EndgameLoadResult{}
	r.sync()
	assertNativeStatsHandshake(t, r.take(), true, true)
}

func TestEndgameNativeStatsSessionFailedRehelloRetainsRenderer(t *testing.T) {
	r := newNativeStatsRig(t, domain.EndgameLoadResult{})
	assertNativeStatsHandshake(t, r.hello(true), true, true)
	r.inspect(func(sc *scene.Scene) { sc.EntityAt(1).Player.Bag.Set(201, domain.Stack{Item: 987654, Count: 1}) })
	responses := nativeStatsResponses(t, r.hello(true))
	if len(responses) != 1 || responses[0].OK || responses[0].Error != "busy" {
		t.Fatalf("unexpected failed rehello: %+v", responses)
	}
	r.inspect(func(sc *scene.Scene) {
		if !sc.EntityAt(1).Player.EndgameNativeStats {
			t.Error("failed rehello lost previously accepted renderer")
		}
		sc.EntityAt(1).Player.Bag.Set(201, domain.Stack{})
	})
	assertNativeStatsHandshake(t, r.hello(true), true, false)
}

func TestEndgameNativeStatsSessionRejectsOptionalCapabilityWithoutBase(t *testing.T) {
	r := newNativeStatsRig(t, domain.EndgameLoadResult{})
	r.s.OnPacket(protocol.OpEndgame, protocol.NewW(0).U16(domain.EndgameSchema).Str(`{"op":"hello","capabilities":["endgame.native-stats.v1"]}`).Bytes()[2:])
	r.sync()
	responses := nativeStatsResponses(t, r.take())
	if len(responses) != 1 || responses[0].OK || responses[0].Error != "unsupported_capability" || r.s.endgameEnabled() {
		t.Fatalf("optional capability bypassed base transport opt-in: %+v", responses)
	}
	r.inspect(func(sc *scene.Scene) {
		if sc.EntityAt(1).Player.EndgameNativeStats {
			t.Error("unsupported hello reached native renderer")
		}
	})
}

func TestEndgameNativeStatsSessionRejectsMalformedRehelloWithoutRevokingState(t *testing.T) {
	r := newNativeStatsRig(t, domain.EndgameLoadResult{})
	assertNativeStatsHandshake(t, r.hello(true), true, true)
	r.s.OnPacket(protocol.OpEndgame, protocol.NewW(0).U16(domain.EndgameSchema).Str(`{"op":"hello","capabilities":["endgame.native-stats.v1"]}`).Bytes()[2:])
	r.sync()
	responses := nativeStatsResponses(t, r.take())
	if len(responses) != 1 || responses[0].OK || responses[0].Error != "unsupported_capability" || !r.s.endgameEnabled() {
		t.Fatalf("rejected rehello revoked accepted transport: %+v", responses)
	}
	r.inspect(func(sc *scene.Scene) {
		if !sc.EntityAt(1).Player.EndgameNativeStats {
			t.Error("rejected rehello revoked accepted renderer")
		}
	})
	assertNativeStatsHandshake(t, r.hello(true), true, false)
}

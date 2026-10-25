package session

import (
	"bytes"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

type stagedTestSink struct {
	capacity int
	packets  [][]byte
	closed   bool
	calls    int
}

func (b *stagedTestSink) Send(p []byte)    { b.packets = append(b.packets, p) }
func (b *stagedTestSink) SessionID() int64 { return 1 }
func (b *stagedTestSink) Close()           { b.closed = true }
func (b *stagedTestSink) Closed() bool     { return b.closed }
func (b *stagedTestSink) TrySendBatch(p [][]byte) bool {
	b.calls++
	if b.closed || len(p) > b.capacity-len(b.packets) {
		return false
	}
	b.packets = append(b.packets, p...)
	return true
}
func stagedSink(b *stagedTestSink) *eventSink {
	return &eventSink{out: b, observer: 1, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestStagedEmitSpawnAtomicRetryAndCancel(t *testing.T) {
	for _, kind := range []domain.EntityKind{domain.KindPlayer, domain.KindPet} {
		b := &stagedTestSink{capacity: 1}
		s := stagedSink(b)
		e := event.EntitySpawned{ID: 10, Kind: kind, Owner: 1, Name: "dependent", HP: 20, MaxHP: 20}
		want := protocol.Encode(1, e)
		if len(want) != 2 {
			t.Fatalf("expected dependent frame for %v", kind)
		}
		if s.TryEmit(e) || len(b.packets) != 0 || s.PendingEmitStarted() {
			t.Fatal("spawn partially accepted")
		}
		retained := s.staged
		if s.TryEmit(e) || s.staged != retained {
			t.Fatal("retry re-encoded or accepted")
		}
		s.CancelPendingEmit()
		if s.staged != nil {
			t.Fatal("cancel retained encoding")
		}
		e.ID = 11
		want = protocol.Encode(1, e)
		b.capacity = 2
		if !s.TryEmit(e) {
			t.Fatal("new spawn rejected")
		}
		if len(b.packets) != len(want) {
			t.Fatal("canceled entity leaked")
		}
		for i := range want {
			if !bytes.Equal(b.packets[i], want[i]) {
				t.Fatalf("packet %d differs", i)
			}
		}
	}
}

func TestStagedEmitLargeGlobalPrefixBudget(t *testing.T) {
	b := &stagedTestSink{capacity: 8}
	s := stagedSink(b)
	packets := make([][]byte, 3100)
	for i := range packets {
		packets[i] = []byte{byte(i), byte(i >> 8)}
	}
	callback := 0
	s.staged = &stagedEmission{packets: packets, bytes: len(packets) * 2, after: []func(){func() { callback++ }}}
	total := 0
	for s.staged != nil {
		complete := s.TryEmit(event.EntityMoved{ID: 9}) // preencoded global head, not re-encoded
		if len(b.packets) > 8 {
			t.Fatal("per-call transport capacity exceeded")
		}
		for _, p := range b.packets {
			if (int(p[0]) | (int(p[1]) << 8)) != total {
				t.Fatalf("packet order %d", total)
			}
			total++
		}
		b.packets = nil
		if !complete && (!s.PendingEmitStarted() || callback != 0) {
			t.Fatal("partial completion semantics")
		}
	}
	if total != 3100 || callback != 1 {
		t.Fatalf("total=%d callbacks=%d", total, callback)
	}
}

func TestStagedEmitByteBudgetAndOversizedSingle(t *testing.T) {
	b := &stagedTestSink{capacity: 100}
	s := stagedSink(b)
	p := make([][]byte, 40)
	for i := range p {
		p[i] = make([]byte, 8192)
	}
	s.staged = &stagedEmission{packets: p, bytes: 40 * 8192}
	if s.TryEmit(event.EntityMoved{}) || len(b.packets) != 8 {
		t.Fatal("64KiB prefix budget violated")
	}
	s.CancelPendingEmit()
	b.packets = nil
	s.staged = &stagedEmission{packets: [][]byte{make([]byte, 100000), make([]byte, 2)}, bytes: 100002}
	if s.TryEmit(event.EntityMoved{}) || len(b.packets) != 1 {
		t.Fatal("large single packet was not admitted alone")
	}
	b.packets = nil
	if !s.TryEmit(event.EntityMoved{}) || len(b.packets) != 1 {
		t.Fatal("large prefix did not complete")
	}
}

func TestStagedCaptureBoundsAndDeferredEffects(t *testing.T) {
	c := &emissionCapture{}
	c.Send(make([]byte, maxStagedEventBytes))
	c.Send([]byte{1})
	if !c.overflow || c.packets != nil || c.bytes != 0 {
		t.Fatal("capture byte cap")
	}
	c = &emissionCapture{}
	for i := 0; i < maxStagedEventFrames+1; i++ {
		c.Send(nil)
	}
	if !c.overflow || c.packets != nil {
		t.Fatal("capture frame cap")
	}
	b := &stagedTestSink{capacity: 0}
	s := stagedSink(b)
	after := 0
	s.capturing = true
	s.afterEmit(func() { after++ })
	s.capturing = false
	if after != 0 || len(s.afterCapture) != 1 {
		t.Fatal("effect ran before admission")
	}
	s.staged = &stagedEmission{packets: [][]byte{{1, 2}}, bytes: 2, after: s.afterCapture}
	s.afterCapture = nil
	if s.TryEmit(event.EntityMoved{}) || after != 0 {
		t.Fatal("backpressure ran deferred effect")
	}
	s.CancelPendingEmit()
	if after != 0 {
		t.Fatal("cancellation ran effect")
	}
}

func TestStagedEmitEncodingCapturedOnce(t *testing.T) {
	b := &stagedTestSink{capacity: 0}
	s := stagedSink(b)
	e := event.InventorySnapshot{Who: 1, Items: []event.InventoryItemView{{Item: 1, Name: "initial", BagIndex: 0}}}
	if s.TryEmit(e) {
		t.Fatal("zero capacity accepted")
	}
	cached := s.inventory
	pending := s.staged
	if cached == nil || pending == nil {
		t.Fatal("inventory was not encoded")
	}
	if s.TryEmit(e) || s.inventory != cached || s.staged != pending {
		t.Fatal("retry encoded/diffed state twice")
	}
	b.capacity = 100
	if !s.TryEmit(e) {
		t.Fatal("retry did not drain")
	}
	if s.staged != nil || len(b.packets) == 0 {
		t.Fatal("empty completion")
	}
}

func TestStagedCancelInvalidatesPresentationBaselines(t *testing.T) {
	b := &stagedTestSink{capacity: 0}
	s := stagedSink(b)
	s.warehouse = &event.WarehouseSnapshot{}
	s.pet = &event.PetSnapshot{}
	e := event.InventorySnapshot{Who: 1, Items: []event.InventoryItemView{{Item: 1, Name: "initial", BagIndex: 0}}}
	if s.TryEmit(e) {
		t.Fatal("zero capacity accepted")
	}
	s.CancelPendingEmit()
	if s.inventory != nil || s.warehouse != nil || s.pet != nil {
		t.Fatal("canceled baseline retained")
	}
	b.capacity = 100
	if !s.TryEmit(e) {
		t.Fatal("replacement snapshot rejected")
	}
	want := protocol.Encode(1, e)
	if len(want) != len(b.packets) {
		t.Fatal("replacement was not full inventory")
	}
	for i := range want {
		if !bytes.Equal(want[i], b.packets[i]) {
			t.Fatal("replacement inventory differs")
		}
	}
}

func TestStagedEncodingPanicRestoresTransport(t *testing.T) {
	b := &stagedTestSink{capacity: 100}
	s := stagedSink(b)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected injected nil-logger panic")
			}
		}()
		s.log = nil // inject a panic after encoding while capture is installed
		s.TryEmit(event.EntityMoved{ID: 1})
	}()
	if s.out != b || s.capturing || s.afterCapture != nil || !b.closed {
		t.Fatal("panic left capture installed or transport live")
	}
}

func TestStagedEmitRejectsReplacedSessionEpoch(t *testing.T) {
	for _, change := range []string{"epoch", "scene", "entity", "selection", "closed"} {
		t.Run(change, func(t *testing.T) {
			b := &stagedTestSink{capacity: 0}
			s := stagedSink(b)
			owner := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 7}, mapEpoch: 12}
			s.sess = owner
			e := event.EntitySpawned{ID: 10, Kind: domain.KindMonster, HP: 20, MaxHP: 20, Pos: domain.Pos{MapID: 7}}
			if s.TryEmit(e) {
				t.Fatal("zero capacity accepted")
			}
			owner.mu.Lock()
			switch change {
			case "epoch":
				owner.mapEpoch++
			case "scene":
				owner.scene.MapID++
			case "entity":
				owner.entity++
			case "selection":
				owner.stage = StageAuthed
			case "closed":
				owner.stage = StageClosed
			}
			owner.mu.Unlock()
			b.capacity = 10
			if s.TryEmit(e) || len(b.packets) != 0 {
				t.Fatal("stale captured spawn crossed binding fence")
			}
			s.CancelPendingEmit()
		})
	}
}

type gatedStagedSink struct {
	stagedTestSink
	entered chan struct{}
	release chan struct{}
}

func (b *gatedStagedSink) TrySendBatch(p [][]byte) bool {
	close(b.entered)
	<-b.release
	return b.stagedTestSink.TrySendBatch(p)
}

func TestStagedAdmissionSerializesWithBindingChange(t *testing.T) {
	b := &gatedStagedSink{stagedTestSink: stagedTestSink{capacity: 10}, entered: make(chan struct{}), release: make(chan struct{})}
	owner := &Session{stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 7}, mapEpoch: 1}
	s := &eventSink{out: b, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sent := make(chan bool, 1)
	go func() { sent <- s.TryEmit(event.EntitySpawned{ID: 2, Kind: domain.KindMonster}) }()
	<-b.entered
	changed := make(chan struct{})
	go func() { owner.mu.Lock(); owner.mapEpoch++; owner.mu.Unlock(); close(changed) }()
	select {
	case <-changed:
		t.Fatal("binding changed inside atomic transport admission")
	case <-time.After(10 * time.Millisecond):
	}
	close(b.release)
	if !<-sent {
		t.Fatal("original epoch's admitted spawn failed")
	}
	<-changed
	if len(b.packets) != 1 {
		t.Fatal("wrong packet count")
	}
}

func TestStagedWorldCancellationPreservesControlCaches(t *testing.T) {
	b := &stagedTestSink{capacity: 0}
	s := stagedSink(b)
	inventory, warehouse, pet := &event.InventorySnapshot{}, &event.WarehouseSnapshot{}, &event.PetSnapshot{}
	s.inventory, s.warehouse, s.pet = inventory, warehouse, pet
	if s.TryEmit(event.EntitySpawned{ID: 10, Kind: domain.KindMonster}) {
		t.Fatal("zero capacity accepted")
	}
	s.CancelPendingEmit()
	if s.inventory != inventory || s.warehouse != warehouse || s.pet != pet {
		t.Fatal("world-only pressure invalidated unrelated control baselines")
	}
}

type epochTestSink struct {
	stagedTestSink
	epoch  uint64
	purges int
}

func (b *epochTestSink) PacedEpoch() uint64 { return b.epoch }
func (b *epochTestSink) TrySendBatchEpoch(p [][]byte, epoch uint64) bool {
	if epoch != b.epoch {
		return false
	}
	return b.TrySendBatch(p)
}
func (b *epochTestSink) CancelPacedEpoch(epoch uint64) (uint64, bool) {
	if epoch != b.epoch {
		return b.epoch, false
	}
	b.epoch++
	b.purges++
	b.packets = nil
	return b.epoch, true
}
func TestStagedRetryCancellationDoesNotRetireAcceptedWorld(t *testing.T) {
	b := &epochTestSink{stagedTestSink: stagedTestSink{capacity: 1}, epoch: 1}
	owner := &Session{stage: StageInGame, entity: 1, mapEpoch: 3}
	s := &eventSink{out: b, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	e := event.EntitySpawned{ID: 10, Kind: domain.KindMonster, HP: 20, MaxHP: 20}
	if !s.TryEmit(e) {
		t.Fatal("first spawn rejected")
	}
	e.ID++
	if s.TryEmit(e) {
		t.Fatal("second spawn should be pending")
	}
	s.CancelPendingEmit()
	if b.epoch != 1 || b.purges != 0 || len(b.packets) != 1 {
		t.Fatal("ordinary retry cancellation purged accepted world")
	}
	s.CancelReplicationEpoch()
	if b.epoch != 2 || b.purges != 1 || len(b.packets) != 0 || s.staged != nil {
		t.Fatal("reset did not retire transport epoch")
	}
	if !s.TryEmit(e) {
		t.Fatal("new epoch spawn rejected")
	}
}

func TestSameMapGateAdvanceReencodesRefusedRemoveWithoutPurging(t *testing.T) {
	b := &epochTestSink{stagedTestSink: stagedTestSink{capacity: 1}, epoch: 9}
	sc := domain.SceneID{MapID: 7}
	owner := &Session{stage: StageInGame, entity: 1, scene: sc, mapEpoch: 12}
	s := &eventSink{out: b, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	first := event.EntityDespawned{ID: 10, Kind: domain.KindMonster, Reason: event.DespawnOutOfSight}
	second := first
	second.ID++
	if !s.TryEmit(first) {
		t.Fatal("first reconciliation removal rejected")
	}
	accepted := append([]byte(nil), b.packets[0]...)
	if s.TryEmit(second) || s.staged == nil || s.staged.binding.epoch != 12 {
		t.Fatal("missing refused old-binding removal")
	}
	if s.ConfirmClientGateReady(1, sc, 11) || s.staged == nil {
		t.Fatal("stale gate changed pending head")
	}
	if !s.ConfirmClientGateReady(1, sc, 12) {
		t.Fatal("current gate rejected")
	}
	if owner.mapEpoch != 13 || s.staged != nil {
		t.Fatal("gate left stale captured binding")
	}
	if b.purges != 0 || b.epoch != 9 || len(b.packets) != 1 || !bytes.Equal(b.packets[0], accepted) {
		t.Fatal("gate purged accepted ordered removal")
	}
	b.capacity = 2
	if !s.TryEmit(second) {
		t.Fatal("preserved removal head cannot re-encode after gate")
	}
	want := protocol.Encode(1, second)
	if len(b.packets) != 2 || len(want) != 1 || !bytes.Equal(b.packets[1], want[0]) {
		t.Fatal("reconciliation FIFO changed")
	}
}

// Models accepted-but-unwritten transport storage. Actual Conn filtering/FIFO
// and ciphertext sequencing are covered by net's paced epoch tests.
func TestEpochPurgeRespawnRetainsFullDefinitionsAndControlCaches(t *testing.T) {
	b := &epochTestSink{stagedTestSink: stagedTestSink{capacity: 32}, epoch: 1}
	owner := &Session{stage: StageInGame, entity: 1, mapEpoch: 3, groundPresentationV1: true}
	s := &eventSink{out: b, observer: 1, sess: owner, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	inventory, warehouse, pet := &event.InventorySnapshot{}, &event.WarehouseSnapshot{}, &event.PetSnapshot{}
	s.inventory, s.warehouse, s.pet = inventory, warehouse, pet
	events := []event.EntitySpawned{
		{ID: 10, Kind: domain.KindNPC, Name: "same NPC", NPC: &event.NPCView{Sprite: "npc601", Script: "fixture", Portrait: "portrait", Label: "vendor", Sell: 2, Trans: 3}},
		{ID: 11, Kind: domain.KindDrop, Name: "same item", Ground: &event.GroundItemView{Item: 4103, Count: 2, Owner: 1, ProtectionMS: 1000, Quality: 4, UniqueID: 71, DropToken: 1234567}},
		{ID: 12, Kind: domain.KindPet, Owner: 1, Name: "owned pet", HP: 20, MaxHP: 20},
		{ID: 13, Kind: domain.KindPlayer, Name: "stall owner", Stall: &event.StallOwnerChanged{Owner: 13, Name: "stall"}},
	}
	var want [][]byte
	for _, e := range events {
		expected := protocol.Encode(1, e)
		if e.Kind == domain.KindDrop {
			expected = [][]byte{protocol.GroundSpawnPresentation(1, e)}
		}
		if (e.Kind == domain.KindPet || e.Kind == domain.KindPlayer) && len(expected) != 2 {
			t.Fatal("missing atomic dependent tail fixture")
		}
		want = append(want, expected...)
		if !s.TryEmit(e) {
			t.Fatal("original spawn rejected")
		}
	}
	if len(b.packets) != len(want) {
		t.Fatal("initial frames missing")
	}
	s.CancelReplicationEpoch()
	if len(b.packets) != 0 {
		t.Fatal("old unwritten frames retained")
	}
	for _, e := range events {
		if !s.TryEmit(e) {
			t.Fatal("new world respawn rejected")
		}
	}
	if len(b.packets) != len(want) {
		t.Fatal("respawn frame missing")
	}
	for i := range want {
		if !bytes.Equal(b.packets[i], want[i]) {
			t.Fatalf("definition or dependent tail changed at frame %d", i)
		}
	}
	if s.inventory != inventory || s.warehouse != warehouse || s.pet != pet {
		t.Fatal("world retirement invalidated business presentation caches")
	}
}

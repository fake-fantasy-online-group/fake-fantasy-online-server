package session

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

type nativeInlayBackend struct {
	plans   chan domain.EndgameCommitPlan
	results chan domain.EndgameCommitResult
}

func (*nativeInlayBackend) Load(int64) <-chan domain.EndgameLoadResult {
	ch := make(chan domain.EndgameLoadResult, 1)
	ch <- domain.EndgameLoadResult{}
	return ch
}
func (*nativeInlayBackend) LoadReceipt(int64, string) <-chan domain.EndgameReceiptLoadResult {
	ch := make(chan domain.EndgameReceiptLoadResult, 1)
	ch <- domain.EndgameReceiptLoadResult{}
	return ch
}
func (*nativeInlayBackend) LoadRun(string) <-chan domain.EndgameRunLoadResult {
	panic("native inlay must not load a rift")
}
func (b *nativeInlayBackend) Commit(p domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	b.plans <- p
	return b.results
}

func newNativeInlayRig(t *testing.T) (*nativeStatsRig, *nativeInlayBackend) {
	t.Helper()
	r := &nativeStatsRig{t: t, out: &capSink{}, frames: make(chan time.Time)}
	backend := &nativeInlayBackend{plans: make(chan domain.EndgameCommitPlan, 8), results: make(chan domain.EndgameCommitResult, 8)}
	items := map[domain.ItemID]domain.ItemDef{}
	for _, def := range astralRuneItemsFixture(7) {
		items[def.ID] = def
	}
	def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999001, Name: "星阵剑", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), SocketLimit: 5, Durable: 100}}
	items[def.ID] = def
	gear := domain.NewStack(def, 1)
	gear.UID = 81
	gear.SocketCount = 2
	bag := domain.NewBag(domain.DefaultBagSlots)
	bag.Set(200, gear)
	for i, id := range []domain.ItemID{domain.FirstAstralRune, domain.FirstAstralRune + 5} {
		rune := domain.NewStack(items[id], 1)
		rune.UID = int64(91 + i)
		bag.Set(100+i, rune)
	}
	r.router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		return scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: r.frames, Items: items, Skills: sessionSkillTableFixture(t), EndgameBackend: backend}), nil
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	r.router.Start(ctx)
	t.Cleanup(func() {
		select {
		case backend.results <- domain.EndgameCommitResult{Err: errors.New("test teardown")}:
		default:
		}
		cancel()
		r.router.Drain(time.Second)
	})
	r.s = New(r.out, Deps{Router: r.router})
	r.s.stage, r.s.entity, r.s.scene = StageInGame, 1, domain.SceneID{MapID: 1}
	r.s.char = &domain.Character{ID: 100, Name: "甲", Race: domain.Warrior, Level: 60, Pos: domain.Pos{MapID: 1}, Skills: domain.Learned{}}
	if !r.router.Post(r.s.scene, scene.Enter{ID: 1, Char: r.s.char, Bag: bag, Worn: domain.NewEquipSet(), Sink: &eventSink{out: r.out, log: r.s.log, sess: r.s, observer: 1}}) {
		t.Fatal("enter failed")
	}
	r.sync()
	r.take()
	return r, backend
}
func nativeInlayPacket(hole byte, runeSlot int32) []byte {
	return protocol.NewW(0).U8(0).U8(hole).U8(1).I32(runeSlot).U8(0).U8(2).I32(0).Bytes()[2:]
}
func nativeInlayPlan(t *testing.T, b *nativeInlayBackend) domain.EndgameCommitPlan {
	t.Helper()
	select {
	case p := <-b.plans:
		return p
	default:
		t.Fatal("native furnace failed to reach durable commit")
		return domain.EndgameCommitPlan{}
	}
}
func nativeInlaySocketIDs(t *testing.T, packets [][]byte) []int32 {
	t.Helper()
	var ids []int32
	for _, p := range packets {
		if len(p) < 2 || binary.LittleEndian.Uint16(p) != protocol.SCSocketInfo {
			continue
		}
		if len(p) < 12 {
			t.Fatal("truncated native socket reply")
		}
		count := int(p[11])
		if len(p) < 12+4*count {
			t.Fatal("truncated socket IDs")
		}
		ids = nil
		for i := 0; i < count; i++ {
			ids = append(ids, int32(binary.LittleEndian.Uint32(p[12+4*i:])))
		}
	}
	return ids
}

// End-to-end: original client bytes -> Session page-local slot translation ->
// Scene actor/atomic receipt -> native notice and SocketInfo bytes, without hello.
func TestNativeFurnaceAstralWireAtomicRoundTrip(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "storage_retry"}[fail], func(t *testing.T) {
			r, b := newNativeInlayRig(t)
			r.s.OnPacket(0x1029, protocol.NewW(0).U8(0).U8(0).U8(0).U8(0).U8(2).I32(0).Bytes()[2:])
			r.sync()
			if ids := nativeInlaySocketIDs(t, r.take()); len(ids) != 2 || ids[0] != 0 || ids[1] != 0 {
				t.Fatal("native preview did not return two empty holes", ids)
			}
			r.s.OnPacket(0x102a, nativeInlayPacket(0, 0))
			r.sync()
			plan := nativeInlayPlan(t, b)
			if plan.Snapshot.Bag.At(200).SocketUIDs[0] != 91 || !plan.Snapshot.Bag.At(100).Empty() || plan.Snapshot.Char.Money != plan.Before.Char.Money {
				t.Fatal("page-local slot translation or rune consumption wrong")
			}
			for _, id := range nativeInlaySocketIDs(t, r.take()) {
				if id != 0 {
					t.Fatal("candidate published before storage")
				}
			}
			r.s.OnPacket(0x102a, nativeInlayPacket(0, 0))
			r.sync()
			if len(b.plans) != 0 {
				t.Fatal("repeated native packet passed inventory lease")
			}
			if fail {
				b.results <- domain.EndgameCommitResult{Err: errors.New("storage unavailable")}
				r.sync()
				if ids := nativeInlaySocketIDs(t, r.take()); len(ids) != 2 || ids[0] != 0 {
					t.Fatal("storage failure leaked socket")
				}
				r.s.OnPacket(0x102a, nativeInlayPacket(0, 0))
				r.sync()
				retry := nativeInlayPlan(t, b)
				if retry.Receipt.RequestID != plan.Receipt.RequestID || retry.Receipt.Digest != plan.Receipt.Digest {
					t.Fatal("native retry lost receipt identity")
				}
				plan = retry
			}
			b.results <- domain.EndgameCommitResult{State: plan.State, Receipt: plan.Receipt}
			r.sync()
			packets := r.take()
			if ids := nativeInlaySocketIDs(t, packets); len(ids) != 2 || ids[0] != int32(domain.FirstAstralRune) || ids[1] != 0 {
				t.Fatal("native success failed socket refresh", ids)
			}
			if r.s.endgameEnabled() || len(nativeStatsResponses(t, packets)) != 0 {
				t.Fatal("native furnace required or leaked custom UI transport")
			}
			r.s.OnPacket(0x102a, nativeInlayPacket(1, 1))
			r.sync()
			second := nativeInlayPlan(t, b)
			st := second.Snapshot.Bag.At(200)
			if st.SocketUIDs[0] != 91 || st.SocketUIDs[1] != 92 || second.ExpectedRevision != 1 {
				t.Fatal("native second hole/revision wrong")
			}
			if _, ok := domain.MatchAstralRecipe(domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999001, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon)}}, st); !ok {
				t.Fatal("native hole order did not form array")
			}
			b.results <- domain.EndgameCommitResult{State: second.State, Receipt: second.Receipt}
			r.sync()
			if ids := nativeInlaySocketIDs(t, r.take()); len(ids) != 2 || ids[1] != int32(domain.FirstAstralRune+5) {
				t.Fatal("second socket byte update missing", ids)
			}
			r.s.OnPacket(0x102a, nativeInlayPacket(0, 0))
			r.sync()
			if len(b.plans) != 0 {
				t.Fatal("late native packet consumed twice")
			}
		})
	}
}

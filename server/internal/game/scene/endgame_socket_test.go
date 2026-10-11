package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"reflect"
	"testing"
)

type astralTestBackend struct {
	result  chan domain.EndgameCommitResult
	plan    domain.EndgameCommitPlan
	receipt *domain.EndgameReceipt
	commits int
}

func (b *astralTestBackend) Load(int64) <-chan domain.EndgameLoadResult {
	ch := make(chan domain.EndgameLoadResult, 1)
	ch <- domain.EndgameLoadResult{}
	return ch
}
func (b *astralTestBackend) LoadReceipt(int64, string) <-chan domain.EndgameReceiptLoadResult {
	ch := make(chan domain.EndgameReceiptLoadResult, 1)
	ch <- domain.EndgameReceiptLoadResult{Receipt: b.receipt}
	return ch
}
func (b *astralTestBackend) LoadRun(string) <-chan domain.EndgameRunLoadResult {
	ch := make(chan domain.EndgameRunLoadResult, 1)
	ch <- domain.EndgameRunLoadResult{}
	return ch
}
func (b *astralTestBackend) Commit(p domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	b.plan = p
	b.commits++
	b.result = make(chan domain.EndgameCommitResult, 1)
	return b.result
}
func astralTake(s *Scene, sink *fakeSink) []event.Event { s.flush(); return sink.take() }
func astralSceneFixture(t *testing.T) (*Scene, *fakeSink, *astralTestBackend) {
	t.Helper()
	s := newTestScene(t, nil)
	s.items = map[domain.ItemID]domain.ItemDef{}
	for _, d := range domain.AstralRuneItems(7) {
		s.items[d.ID] = d
	}
	for _, d := range domain.CubeMaterialDefinitions() {
		s.items[d.ID] = d
	}
	s.items[999001] = domain.ItemDef{ID: 999001, Name: "星阵测试剑", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), LevelReq: 50, Durable: 100}}
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	p.Player.Char.Level = 60
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.Money = moneyFromInventory(100000)
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	target := domain.NewStack(s.items[999001], 1)
	target.UID = 42
	target.SocketCount = 2
	p.Player.Bag.Set(200, target)
	r := domain.NewStack(s.items[domain.FirstAstralRune], 1)
	r.UID = 51
	p.Player.Bag.Set(100, r)
	p.Player.Bag.Set(102, domain.Stack{Item: domain.CubeStarMarrow, Count: 1})
	p.Player.Bag.Set(103, domain.Stack{Item: domain.CubePurifiedDust, Count: 1})
	b := &astralTestBackend{}
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = true
	astralTake(s, sink)
	return s, sink, b
}
func TestAstralSocketCommitReplayAndStorageFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit_replay", true: "rollback"}[fail], func(t *testing.T) {
			s, sink, b := astralSceneFixture(t)
			p := s.players[1]
			req := domain.EndgameRequest{Op: "socket.insert", RequestID: "astral-insert-00001", ExpectedRevision: "0", TargetUID: "42", RuneUID: "51", Socket: 0}
			before := p.Player.Bag.Clone()
			s.onEndgameSocket(p, req)
			if b.commits != 1 || !p.Player.TradeBusy || !reflect.DeepEqual(before, p.Player.Bag) {
				t.Fatal("candidate leaked before durable success")
			}
			if b.plan.Snapshot.Bag.At(200).SocketUIDs[0] != 51 || !b.plan.Snapshot.Bag.At(100).Empty() {
				t.Fatal("candidate wrong")
			}
			if fail {
				b.result <- domain.EndgameCommitResult{Err: errors.New("injected storage failure")}
				s.finishEndgame(false)
				if !reflect.DeepEqual(before, p.Player.Bag) || p.Player.TradeBusy {
					t.Fatal("failure lost card")
				}
				return
			}
			b.receipt = &b.plan.Receipt
			b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			s.finishEndgame(false)
			astralTake(s, sink)
			if p.Player.Bag.At(200).SocketUIDs[0] != 51 || p.Player.TradeBusy {
				t.Fatal("commit unpublished")
			}
			s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
			s.finishEndgame(false)
			if b.commits != 1 {
				t.Fatal("replay duplicated mutation")
			}
			found := false
			for _, e := range astralTake(s, sink) {
				if m, ok := e.(event.EndgameMessage); ok && m.Message.OK && m.Message.RequestID == req.RequestID {
					found = true
				}
			}
			if !found {
				t.Fatal("missing receipt replay")
			}
		})
	}
}
func TestAstralRemoveUsesFreedMaterialSlot(t *testing.T) {
	s, _, b := astralSceneFixture(t)
	p := s.players[1]
	plan, err := domain.PlanAstralInsert(p.Player.Bag, p.Player.Worn, s.itemDef, 42, 51, 0, p.Player.Char)
	if err != nil {
		t.Fatal(err)
	}
	p.Player.Bag = plan.Bag
	for i := 100; i < 200; i++ {
		if p.Player.Bag.At(i).Empty() {
			p.Player.Bag.Set(i, domain.Stack{Item: 998001, Count: 1})
		}
	}
	before := p.Player.Bag.Clone()
	money, _ := inventoryMoney(p.Player.Char.Money)
	s.onEndgameSocket(p, domain.EndgameRequest{Op: "socket.remove", RequestID: "astral-remove-00001", ExpectedRevision: "0", TargetUID: "42", Socket: 0})
	if b.commits != 1 || !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatal("non-atomic remove")
	}
	after, _ := inventoryMoney(b.plan.Snapshot.Char.Money)
	if after != money-2500 || b.plan.Snapshot.Bag.CountOf(domain.CubeStarMarrow) != 0 || b.plan.Snapshot.Bag.CountOf(domain.CubePurifiedDust) != 0 || b.plan.Snapshot.Bag.At(200).Sockets[0] != 0 {
		t.Fatal("wrong economics")
	}
	n := 0
	b.plan.Snapshot.Bag.Each(func(_ int, st domain.Stack) {
		if st.UID == 51 {
			n++
		}
	})
	if n != 1 {
		t.Fatal("wrong UID count", n)
	}
}
func TestAstralNativeInlayRejectsRandomPath(t *testing.T) {
	s, sink, b := astralSceneFixture(t)
	p := s.players[1]
	s.sockets = domain.SocketTable{}
	before := p.Player.Bag.Clone()
	s.onInlayItem(InlayItem{ID: p.ID, TargetKind: 0, Tab: 2, BagSlot: 200, Hole: 0, RuneTab: 1, RuneBagSlot: 100})
	if !reflect.DeepEqual(before, p.Player.Bag) || b.commits != 0 {
		t.Fatal("legacy route mutated rune")
	}
	if _, ok := firstOf[event.ServerNotice](astralTake(s, sink)); !ok {
		t.Fatal("missing guidance")
	}
}
func TestAstralFormationChangesRealCombatStats(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	p.Player.Char.Base = domain.Base{STR: 200, VIT: 100, INT: 100, SPI: 100, AGI: 100, DEX: 100}
	plan, err := domain.PlanAstralInsert(p.Player.Bag, p.Player.Worn, s.itemDef, 42, 51, 0, p.Player.Char)
	if err != nil {
		t.Fatal(err)
	}
	tail := domain.NewStack(s.items[domain.FirstAstralRune+5], 1)
	tail.UID = 52
	plan.Bag.Set(101, tail)
	plan, err = domain.PlanAstralInsert(plan.Bag, plan.Worn, s.itemDef, 42, 52, 1, p.Player.Char)
	if err != nil {
		t.Fatal(err)
	}
	p.Player.Bag = plan.Bag
	p.Player.Bag.Set(200, domain.Stack{})
	p.Player.Worn.Set(domain.SlotWeapon, plan.Target)
	s.refreshStats(p)
	complete := p.Stats.MinAtk
	st := plan.Target
	st.Sockets[0], st.Sockets[1] = st.Sockets[1], st.Sockets[0]
	st.SocketUIDs[0], st.SocketUIDs[1] = st.SocketUIDs[1], st.SocketUIDs[0]
	st.SocketCards[0], st.SocketCards[1] = st.SocketCards[1], st.SocketCards[0]
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.refreshStats(p)
	// v2 引火入锋 adds STR4 then attack18%; reverse order retains only the two rune snapshots.
	if complete != 125 || p.Stats.MinAtk != 104 {
		t.Fatalf("wrong active/reversed damage stats %d/%d", complete, p.Stats.MinAtk)
	}
}
func TestAstralDroppedHolesReserveAttributeBudget(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 999002, RatePct: 100, RollAffixes: true}})
	s.items[999002] = domain.ItemDef{ID: 999002, Name: "空白星剑", InstanceKind: domain.ItemInstanceEquipment, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), LevelReq: 50, Durable: 100}}
	s.equipmentRoll = domain.EquipmentRollTable{Counts: map[domain.ColorProfile][]domain.WeightedAffixCount{1: {{Count: 5, Weight: 1}}}, Options: map[domain.EquipmentRollKey][]domain.EquipmentRollOption{{}: {{Affix: domain.Affix{Attr: domain.AttrSTR, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrINT, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrVIT, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrAGI, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrDEX, Value: 2}, Weight: 1}}}}
	s.entities[mid].Monster.Kind = domain.MonsterBoss
	for i := 0; i < 100; i++ {
		s.rollLoot(s.entities[mid], 0)
	}
	seen := map[int64]bool{}
	holes := 0
	for _, e := range s.ground {
		st := e.Drop.Stack
		if st.Item != 999002 {
			continue
		}
		if st.UID <= 0 || seen[st.UID] || int(st.SocketCount)+int(st.RolledAffixCount) != 5 {
			t.Fatal("bad generated stack", st)
		}
		seen[st.UID] = true
		if st.SocketCount > 0 {
			holes++
		}
	}
	if len(seen) != 100 || holes == 0 {
		t.Fatal("no real socket drops")
	}
}

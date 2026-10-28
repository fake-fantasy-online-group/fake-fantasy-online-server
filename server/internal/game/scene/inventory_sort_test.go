package scene

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Read real ov_item rows, rather than declaring all test items stackable. These
// ordinary material rows have category/self_use/other_use = 0 (material tab 1).
func sortMaterialFixtures(t *testing.T) map[domain.ItemID]domain.ItemDef {
	t.Helper()
	paths := []string{"../../../migrations/000000_init-29.sql", "../../../migrations/000000_init-30.sql"}
	var raw []byte
	for _, path := range paths {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		raw = append(raw, b...)
	}
	wanted := map[string]bool{"元神结晶": true, "铁矿石": true, "小鱼": true}
	defs := map[domain.ItemID]domain.ItemDef{}
	active := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "INSERT INTO") {
			active = strings.HasPrefix(line, `INSERT INTO "gamedata"."ov_item" `)
		}
		if !active {
			continue
		}
		fields := strings.Split(strings.TrimSpace(line), ", ")
		if len(fields) < 44 {
			continue
		}
		name := strings.Trim(fields[2], "'")
		if !wanted[name] {
			continue
		}
		number := func(i int) int32 {
			n, e := strconv.ParseInt(fields[i], 10, 32)
			if e != nil {
				t.Fatal(e)
			}
			return int32(n)
		}
		if number(4) != 0 || number(42) != 0 || number(43) != 0 {
			t.Fatalf("fixture tab changed: %s", name)
		}
		id := domain.ItemID(number(1))
		defs[id] = domain.ItemDef{ID: id, Name: name, Stackable: number(7) != 0, Weight: number(12), InventoryTab: 1, InventoryTabKnown: true}
	}
	if len(defs) != 3 {
		t.Fatalf("missing SQL fixtures: %v", defs)
	}
	return defs
}

func TestSortBagRealMaterialStacks(t *testing.T) {
	for id, def := range sortMaterialFixtures(t) {
		t.Run(def.Name, func(t *testing.T) {
			s := newTestScene(t, nil)
			s.items = map[domain.ItemID]domain.ItemDef{id: def}
			sink := join(t, s, 1, 100, "sort", 0, 0)
			p := s.players[1]
			p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
			a := domain.NewStack(def, domain.MaxStack-2)
			b := domain.NewStack(def, 5)
			p.Player.Bag.Set(100, a)
			p.Player.Bag.Set(107, b)
			money := p.Player.Char.Money
			s.flush()
			sink.take()
			s.exec(SortBag{ID: 1, Tab: 1})
			if p.Player.Bag.At(100).Count != domain.MaxStack || p.Player.Bag.At(101).Count != 3 || !p.Player.Bag.At(107).Empty() {
				t.Fatal("not consolidated to limit")
			}
			if p.Player.Char.Money != money || p.Player.Bag.CountOf(id) != domain.MaxStack+3 {
				t.Fatal("conservation")
			}
			s.flush()
			snap, ok := firstOf[event.InventorySnapshot](sink.take())
			if !ok || len(snap.Items) != 2 {
				t.Fatalf("missing complete snapshot: %+v", snap)
			}
			before := p.Player.Bag.Clone()
			s.exec(SortBag{ID: 1, Tab: 1})
			for i := 0; i < before.Cap(); i++ {
				if before.At(i) != p.Player.Bag.At(i) {
					t.Fatal("not idempotent")
				}
			}
		})
	}
}

func TestSortBagRuneInterleavedSnapshots(t *testing.T) {
	def := astralRuneItemsFixture(0)[0]
	s := newTestScene(t, nil)
	s.items = map[domain.ItemID]domain.ItemDef{def.ID: def}
	a := domain.NewStack(def, domain.MaxStack-2)
	a.UID = 101
	b := domain.NewStack(def, 4)
	b.UID = 102
	b.Card.Effects[0].Value++
	c := domain.NewStack(def, 5)
	c.UID = 103
	got := s.mergeSortedBagStacks([]domain.Stack{a, b, c})
	wantA, wantC := a, c
	wantA.Count = domain.MaxStack
	wantC.Count = 3
	if !reflect.DeepEqual(got, []domain.Stack{wantA, b, wantC}) {
		t.Fatalf("interleaved rune lost merge or attributes: %+v", got)
	}
	if !reflect.DeepEqual(got, s.mergeSortedBagStacks(got)) {
		t.Fatal("not idempotent")
	}
	for _, mutate := range []func(*domain.Stack){func(x *domain.Stack) { x.Bound = true }, func(x *domain.Stack) { x.Locked = true }, func(x *domain.Stack) { x.Card.Bound = true }, func(x *domain.Stack) { x.Card.Locked = true }, func(x *domain.Stack) { x.Durability = 1 }, func(x *domain.Stack) { x.Endgame.PowerScrollID = "test" }} {
		other := c
		mutate(&other)
		expected := other
		if domain.IsAstralRuneStack(expected) && expected.Bound {
			expected.Card.Bound = true
		}
		if got := s.mergeSortedBagStacks([]domain.Stack{a, other}); !reflect.DeepEqual(got, []domain.Stack{a, expected}) {
			t.Fatal("merged incompatible metadata")
		}
	}
}

func TestSortBagFullPageBoundGroupsAndQueuedRequests(t *testing.T) {
	defs := sortMaterialFixtures(t)
	def := defs[4001]
	s := newTestScene(t, nil)
	s.items = defs
	sink := join(t, s, 1, 100, "sort", 0, 0)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	for i := 100; i < 200; i++ {
		st := domain.NewStack(def, 1)
		st.Bound = i%2 == 0
		p.Player.Bag.Set(i, st)
	}
	// A different page is not a source or destination, even when it is invalid.
	// Snapshot failure below must restore the entire prior plan.
	prior := p.Player.Bag.Clone()
	quote := &entity.RepairQuote{}
	p.Player.RepairQuote = quote
	p.Player.Char.Money.Gold = 1<<63 - 1
	s.exec(SortBag{ID: 1, Tab: 1})
	for i := 0; i < prior.Cap(); i++ {
		if prior.At(i) != p.Player.Bag.At(i) {
			t.Fatal("failed snapshot changed bag")
		}
	}
	if p.Player.RepairQuote != quote {
		t.Fatal("failed snapshot lost quote")
	}
	p.Player.Char.Money.Gold = 0
	sink.take()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !s.Post(SortBag{ID: 1, Tab: 1}) {
				t.Error("queue rejected")
			}
		}()
	}
	wg.Wait()
	s.drain()
	if p.Player.Bag.CountOf(def.ID) != 100 || p.Player.Bag.At(100).Count != 50 || p.Player.Bag.At(101).Count != 50 || p.Player.Bag.At(100).Bound || !p.Player.Bag.At(101).Bound {
		t.Fatal("queued sort duplicated or mixed binding")
	}
	for i := 102; i < 200; i++ {
		if !p.Player.Bag.At(i).Empty() {
			t.Fatal("slots not reclaimed")
		}
	}
}

// Read-only production diagnosis found these five pairs: outer binding true,
// identical effect snapshots, but one card's legacy snapshot_bound was false.
// Counts and item definitions are real; test UIDs are deliberately synthetic.
func TestSortBagLegacyBoundRunePairs(t *testing.T) {
	pairs := map[domain.ItemID][2]int32{1900028005: {2, 1}, 1900028009: {1, 2}, 1900028012: {1, 1}, 1900028021: {1, 2}, 1900028026: {1, 1}}
	matched := 0
	for _, def := range astralRuneItemsFixture(0) {
		counts, ok := pairs[def.ID]
		if !ok {
			continue
		}
		matched++
		t.Run(def.Name, func(t *testing.T) {
			saver := newFakeSaver()
			s := newTestScene(t, saver)
			s.items = map[domain.ItemID]domain.ItemDef{def.ID: def}
			join(t, s, 1, 100, "sort", 0, 0)
			p := s.players[1]
			p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
			a, b := domain.NewStack(def, counts[0]), domain.NewStack(def, counts[1])
			a.Bound = true
			b.Bound = true
			a.Card.Bound = true
			unbound := domain.NewStack(def, 3)
			// Put the legacy false snapshot first as well: outcome cannot depend on UID ordering.
			for _, order := range [][]domain.Stack{{a, b, unbound}, {b, a, unbound}} {
				p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
				for i, st := range order {
					p.Player.Bag.Set(100+i, st)
				}
				s.exec(SortBag{ID: 1, Tab: 1})
				got := p.Player.Bag.At(101)
				if p.Player.Bag.At(100) != unbound || got.Count != counts[0]+counts[1] || !got.Bound || !got.Card.Bound || got.Card.Effects != a.Card.Effects || got.UID != order[0].UID || !p.Player.Bag.At(102).Empty() {
					t.Fatalf("legacy binding not safely merged: %+v", got)
				}
				persisted := saver.lastBag[100]
				if persisted == nil || persisted.At(101) != got || !persisted.At(102).Empty() {
					t.Fatal("scene did not enqueue the consolidated owned stack and card snapshot for persistence")
				}
				first := p.Player.Bag.Clone()
				s.exec(SortBag{ID: 1, Tab: 1})
				for i := 0; i < first.Cap(); i++ {
					if first.At(i) != p.Player.Bag.At(i) {
						t.Fatal("not idempotent")
					}
				}
			}
		})
	}
	if matched != len(pairs) {
		t.Fatalf("real pair fixtures missing: %d", matched)
	}
}

func TestSortBagOtherPagesAndUniqueInstancesUntouched(t *testing.T) {
	defs := sortMaterialFixtures(t)
	s := newTestScene(t, nil)
	s.items = defs
	// Valid consumable and equipment pages provide a snapshot without participating.
	potion := domain.ItemDef{ID: 77, Stackable: true, InventoryTab: 0, InventoryTabKnown: true}
	s.items[77] = potion
	equipment := domain.ItemDef{ID: 78, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Durable: 100}}
	s.items[78] = equipment
	join(t, s, 1, 100, "sort", 0, 0)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	p.Player.Bag.Set(5, domain.NewStack(potion, 2))
	p.Player.Bag.Set(8, domain.NewStack(potion, 3))
	a, b := domain.NewStack(equipment, 1), domain.NewStack(equipment, 1)
	p.Player.Bag.Set(205, a)
	p.Player.Bag.Set(209, b)
	p.Player.Bag.Set(100, domain.NewStack(defs[4001], 2))
	p.Player.Bag.Set(109, domain.NewStack(defs[4001], 3))
	warehouse := domain.NewWarehouse(1, 20, 999)
	p.Player.Warehouse = warehouse
	before := p.Player.Bag.Clone()
	s.exec(SortBag{ID: 1, Tab: 1})
	for i := 0; i < before.Cap(); i++ {
		if (i < 100 || i >= 200) && before.At(i) != p.Player.Bag.At(i) {
			t.Fatal("other page moved")
		}
	}
	if p.Player.Warehouse != warehouse {
		t.Fatal("warehouse touched")
	}
	s.exec(SortBag{ID: 1, Tab: 2})
	if p.Player.Bag.At(200).UID != a.UID || p.Player.Bag.At(201).UID != b.UID || p.Player.Bag.At(200).Count != 1 || p.Player.Bag.At(201).Count != 1 {
		t.Fatal("equipment merged or UID lost")
	}
	// Even a template incorrectly marked stackable must not merge unique scroll/spirit instances.
	for _, kind := range []domain.ItemInstanceKind{domain.ItemInstanceSpirit, domain.ItemInstanceEquipment} {
		def := domain.ItemDef{ID: 88, Stackable: true, InstanceKind: kind}
		s.items[88] = def
		x, y := domain.NewStack(def, 1), domain.NewStack(def, 1)
		if got := s.mergeSortedBagStacks([]domain.Stack{x, y}); !reflect.DeepEqual(got, []domain.Stack{x, y}) {
			t.Fatal("unique instances merged")
		}
	}
}

func TestSortBagRejectsSplitOverflowOfSelectedPage(t *testing.T) {
	defs := sortMaterialFixtures(t)
	s := newTestScene(t, nil)
	s.items = defs
	saver := newFakeSaver()
	s.saver = saver
	join(t, s, 1, 100, "sort", 0, 0)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	st := domain.NewStack(defs[4001], domain.MaxStack*(domain.BagPageSlots+1))
	p.Player.Bag.Set(107, st)
	quote := &entity.RepairQuote{}
	p.Player.RepairQuote = quote
	before := p.Player.Bag
	money := p.Player.Char.Money
	n := saver.count(100)
	s.exec(SortBag{ID: 1, Tab: 1})
	if p.Player.Bag != before || p.Player.Bag.At(107) != st || p.Player.RepairQuote != quote || p.Player.Char.Money != money || saver.count(100) != n {
		t.Fatal("overflow silently truncated selected page or committed a partial plan")
	}
}

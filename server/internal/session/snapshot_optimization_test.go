package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"io"
	"log/slog"
	"math/rand"
	"reflect"
	"testing"
)

type optimizationDiscard struct{}

func (optimizationDiscard) Send([]byte)      {}
func (optimizationDiscard) Close()           {}
func (optimizationDiscard) SessionID() int64 { return 1 }
func BenchmarkOptimizationMove(b *testing.B) {
	for _, n := range []int{1, 100} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			sinks := make([]eventSink, n)
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			for i := range sinks {
				sinks[i] = eventSink{out: optimizationDiscard{}, log: log, observer: domain.EntityID(i + 1)}
			}
			var ev event.Event = event.EntityMoved{ID: 1000}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := range sinks {
					sinks[j].Emit(ev)
				}
			}
		})
	}
}
func BenchmarkOptimizationInventoryDiff(b *testing.B) {
	for _, n := range []int{32, 128} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			old := event.InventorySnapshot{Items: make([]event.InventoryItemView, n), Equipped: make([]event.EquippedItemView, 12)}
			for i := range old.Items {
				old.Items[i] = event.InventoryItemView{BagIndex: int32(i), Name: "item", Desc: "item description", Item: 1}
			}
			for i := range old.Equipped {
				old.Equipped[i] = event.EquippedItemView{Slot: domain.EquipSlot(i), Name: "equipped", Item: 1}
			}
			next := cloneInventory(old)
			next.Money++
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _, _ = inventoryDiff(old, next)
			}
		})
	}
}
func TestOptimizationInventoryEquality(t *testing.T) {
	a := event.InventorySnapshot{Items: []event.InventoryItemView{{Item: 1, Pet: &domain.PetItemInfo{Name: "pet"}}}}
	b := cloneInventory(a)
	b.Items[0].Pet = &domain.PetItemInfo{Name: "pet"}
	_, _, changed, ok := inventoryDiff(a, b)
	if changed || !ok {
		t.Fatal("equal pet values differed")
	}
	b.Items[0].Pet.Name = "new"
	items, _, changed, ok := inventoryDiff(a, b)
	if !changed || !ok || !reflect.DeepEqual(items, b.Items) {
		t.Fatal("changed pet ignored")
	}
}

func inventoryDiffOriginalOptimization(old, next event.InventorySnapshot) ([]event.InventoryItemView,
	[]event.EquippedItemView, bool, bool) {
	oldItems, ok := indexInventoryOriginalOptimization(old.Items)
	if !ok {
		return nil, nil, false, false
	}
	nextItems, ok := indexInventoryOriginalOptimization(next.Items)
	if !ok {
		return nil, nil, false, false
	}
	var items []event.InventoryItemView
	for _, item := range next.Items {
		key := inventorySlot{tab: item.Tab, slot: item.BagIndex}
		if previous, exists := oldItems[key]; !exists || !reflect.DeepEqual(previous, item) {
			items = append(items, item)
		}
	}
	for _, item := range old.Items {
		key := inventorySlot{tab: item.Tab, slot: item.BagIndex}
		if _, exists := nextItems[key]; !exists {
			items = append(items, event.InventoryItemView{
				Tab: item.Tab, BagIndex: item.BagIndex, Slot: domain.EquipSlot(-1),
			})
		}
	}

	oldEquipped, ok := indexEquippedOriginalOptimization(old.Equipped)
	if !ok {
		return nil, nil, false, false
	}
	nextEquipped, ok := indexEquippedOriginalOptimization(next.Equipped)
	if !ok {
		return nil, nil, false, false
	}
	var equipped []event.EquippedItemView
	for _, item := range next.Equipped {
		if previous, exists := oldEquipped[item.Slot]; !exists || !reflect.DeepEqual(previous, item) {
			equipped = append(equipped, item)
		}
	}
	for _, item := range old.Equipped {
		if _, exists := nextEquipped[item.Slot]; !exists {
			equipped = append(equipped, event.EquippedItemView{Slot: item.Slot})
		}
	}
	headerChanged := old.Money != next.Money || old.Caiyu != next.Caiyu || old.Honor != next.Honor ||
		old.Weight != next.Weight || old.MaxWeight != next.MaxWeight
	return items, equipped, headerChanged || len(items) != 0 || len(equipped) != 0, true
}

func indexInventoryOriginalOptimization(items []event.InventoryItemView) (map[inventorySlot]event.InventoryItemView, bool) {
	out := make(map[inventorySlot]event.InventoryItemView, len(items))
	for _, item := range items {
		key := inventorySlot{tab: item.Tab, slot: item.BagIndex}
		if _, exists := out[key]; exists {
			return nil, false
		}
		out[key] = item
	}
	return out, true
}

func indexEquippedOriginalOptimization(items []event.EquippedItemView) (map[domain.EquipSlot]event.EquippedItemView, bool) {
	out := make(map[domain.EquipSlot]event.EquippedItemView, len(items))
	for _, item := range items {
		if _, exists := out[item.Slot]; exists {
			return nil, false
		}
		out[item.Slot] = item
	}
	return out, true
}

func TestOptimizationInventoryDiffParity(t *testing.T) {
	for n := 0; n < 80; n++ {
		old := event.InventorySnapshot{Items: make([]event.InventoryItemView, n), Equipped: make([]event.EquippedItemView, 12)}
		for i := range old.Items {
			old.Items[i] = event.InventoryItemView{BagIndex: int32(i), Item: 1, Name: "x"}
		}
		for i := range old.Equipped {
			old.Equipped[i] = event.EquippedItemView{Slot: domain.EquipSlot(i), Item: 1}
		}
		for variant := 0; variant < 6; variant++ {
			next := cloneInventory(old)
			switch variant {
			case 0:
				next.Money++
			case 1:
				if n > 0 {
					next.Items[0].Count++
				}
			case 2:
				if n > 0 {
					next.Items = next.Items[1:]
				}
			case 3:
				if n > 1 {
					next.Items[1] = next.Items[0]
				}
			case 4:
				next.Equipped[1].Item++
			case 5:
				next.Equipped = next.Equipped[1:]
			}
			a, b, c, d := inventoryDiff(old, next)
			w, x, y, z := inventoryDiffOriginalOptimization(old, next)
			if !reflect.DeepEqual(a, w) || !reflect.DeepEqual(b, x) || c != y || d != z {
				t.Fatalf("parity n=%d variant=%d", n, variant)
			}
		}
	}
}

func warehouseDiffOriginalOptimization(old, next event.WarehouseSnapshot) ([]event.WarehouseItemView, bool, bool) {
	oldItems, ok := indexWarehouseOriginalOptimization(old.Items)
	if !ok {
		return nil, false, false
	}
	nextItems, ok := indexWarehouseOriginalOptimization(next.Items)
	if !ok {
		return nil, false, false
	}
	var items []event.WarehouseItemView
	for _, item := range next.Items {
		key := inventorySlot{tab: item.Tab, slot: item.Slot}
		if previous, exists := oldItems[key]; !exists || !reflect.DeepEqual(previous, item) {
			items = append(items, item)
		}
	}
	for _, item := range old.Items {
		key := inventorySlot{tab: item.Tab, slot: item.Slot}
		if _, exists := nextItems[key]; !exists {
			items = append(items, event.WarehouseItemView{Tab: item.Tab, Slot: item.Slot})
		}
	}
	headerChanged := old.PageCount != next.PageCount || old.Money != next.Money
	return items, headerChanged || len(items) != 0, true
}

func indexWarehouseOriginalOptimization(items []event.WarehouseItemView) (map[inventorySlot]event.WarehouseItemView, bool) {
	out := make(map[inventorySlot]event.WarehouseItemView, len(items))
	for _, item := range items {
		key := inventorySlot{tab: item.Tab, slot: item.Slot}
		if _, exists := out[key]; exists {
			return nil, false
		}
		out[key] = item
	}
	return out, true
}

func TestOptimizationWarehouseDiffParity(t *testing.T) {
	for n := 0; n < 80; n++ {
		old := event.WarehouseSnapshot{Items: make([]event.WarehouseItemView, n)}
		for i := range old.Items {
			old.Items[i] = event.WarehouseItemView{Slot: int32(i), Item: 1, Name: "x"}
		}
		for variant := 0; variant < 4; variant++ {
			next := cloneWarehouse(old)
			switch variant {
			case 0:
				next.Money++
			case 1:
				if n > 0 {
					next.Items[0].Count++
				}
			case 2:
				if n > 0 {
					next.Items = next.Items[1:]
				}
			case 3:
				if n > 1 {
					next.Items[1] = next.Items[0]
				}
			}
			a, b, c := warehouseDiff(old, next)
			x, y, z := warehouseDiffOriginalOptimization(old, next)
			if !reflect.DeepEqual(a, x) || b != y || c != z {
				t.Fatalf("parity n=%d variant=%d", n, variant)
			}
		}
	}
}

// Randomized comparisons use the former value-map implementation as an oracle.
// Small key spaces intentionally exercise duplicate fallback in both inputs.
func TestOptimizationRandomizedDiffParity(t *testing.T) {
	r := rand.New(rand.NewSource(371849))
	inventory := func() event.InventorySnapshot {
		s := event.InventorySnapshot{Money: int64(r.Intn(3)), Items: make([]event.InventoryItemView, r.Intn(20)), Equipped: make([]event.EquippedItemView, r.Intn(8))}
		for i := range s.Items {
			s.Items[i] = event.InventoryItemView{Tab: uint8(r.Intn(3)), BagIndex: int32(r.Intn(10)), Item: domain.ItemID(r.Intn(5)), Count: int32(r.Intn(4)), Name: fmt.Sprint(r.Intn(4)), Bound: r.Intn(2) == 0}
			if r.Intn(3) == 0 {
				s.Items[i].Pet = &domain.PetItemInfo{Name: fmt.Sprint(r.Intn(4)), Skills: []domain.PetItemSkill{{Name: "skill", Level: int32(r.Intn(3))}}}
			}
		}
		for i := range s.Equipped {
			s.Equipped[i] = event.EquippedItemView{Slot: domain.EquipSlot(r.Intn(6)), Item: domain.ItemID(r.Intn(5)), SoulAvatar: int32(r.Intn(4))}
		}
		return s
	}
	warehouse := func() event.WarehouseSnapshot {
		s := event.WarehouseSnapshot{Money: int64(r.Intn(3)), Items: make([]event.WarehouseItemView, r.Intn(20))}
		for i := range s.Items {
			s.Items[i] = event.WarehouseItemView{Tab: uint8(r.Intn(3)), Slot: int32(r.Intn(10)), Item: domain.ItemID(r.Intn(5)), Count: int32(r.Intn(4)), Blocked: r.Intn(2) == 0}
		}
		return s
	}
	for trial := 0; trial < 2000; trial++ {
		a, b := inventory(), inventory()
		beforeA, _ := json.Marshal(a)
		beforeB, _ := json.Marshal(b)
		x, y, c, ok := inventoryDiff(a, b)
		wx, wy, wc, wok := inventoryDiffOriginalOptimization(a, b)
		if !reflect.DeepEqual(x, wx) || !reflect.DeepEqual(y, wy) || c != wc || ok != wok {
			t.Fatalf("inventory trial %d", trial)
		}
		afterA, _ := json.Marshal(a)
		afterB, _ := json.Marshal(b)
		if !bytes.Equal(beforeA, afterA) || !bytes.Equal(beforeB, afterB) {
			t.Fatal("inventory input mutated")
		}
		wa, wb := warehouse(), warehouse()
		ca, cb := cloneWarehouse(wa), cloneWarehouse(wb)
		z, d, valid := warehouseDiff(wa, wb)
		wz, wd, wvalid := warehouseDiffOriginalOptimization(wa, wb)
		if !reflect.DeepEqual(z, wz) || d != wd || valid != wvalid {
			t.Fatalf("warehouse trial %d", trial)
		}
		// cloneWarehouse normalizes empty slices; compare their contents.
		if !reflect.DeepEqual(cloneWarehouse(wa), ca) || !reflect.DeepEqual(cloneWarehouse(wb), cb) {
			t.Fatal("warehouse input mutated")
		}
	}
}

func TestOptimizationInventoryEqualityEveryField(t *testing.T) {
	a := event.InventoryItemView{Pet: &domain.PetItemInfo{Name: "pet", Skills: []domain.PetItemSkill{{Name: "skill", Level: 1}}}}
	for i := 0; i < reflect.TypeOf(a).NumField(); i++ {
		b := a
		f := reflect.ValueOf(&b).Elem().Field(i)
		switch f.Kind() {
		case reflect.Pointer:
			p := *a.Pet
			p.Skills = append([]domain.PetItemSkill(nil), p.Skills...)
			p.Skills[0].Level++
			b.Pet = &p
		case reflect.String:
			f.SetString("changed")
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			f.SetInt(1)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			f.SetUint(1)
		default:
			t.Fatalf("add coverage for %s", f.Kind())
		}
		if equalInventoryItem(a, b) != reflect.DeepEqual(a, b) {
			t.Fatalf("field %s", reflect.TypeOf(a).Field(i).Name)
		}
	}
	b := a
	p := *a.Pet
	p.Skills = append([]domain.PetItemSkill(nil), p.Skills...)
	b.Pet = &p
	if !equalInventoryItem(a, b) {
		t.Fatal("distinct equal pet pointers")
	}
	p.Skills = nil
	a.Pet = &domain.PetItemInfo{Name: "pet"}
	b.Pet = &domain.PetItemInfo{Name: "pet", Skills: []domain.PetItemSkill{}}
	if equalInventoryItem(a, b) {
		t.Fatal("nil and empty pet skills must differ")
	}
	if a.Pet == nil || b.Pet == nil {
		t.Fatal("equality mutated pointers")
	}
}

type optimizationRetainingSink struct{ packets [][]byte }

func (s *optimizationRetainingSink) Send(p []byte)  { s.packets = append(s.packets, p) }
func (*optimizationRetainingSink) Close()           {}
func (*optimizationRetainingSink) SessionID() int64 { return 1 }

func TestOptimizationDebugAndPacketParity(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo} {
		var gotLog, wantLog bytes.Buffer
		opts := &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		}}
		out := &optimizationRetainingSink{}
		s := eventSink{out: out, log: slog.New(slog.NewTextHandler(&gotLog, opts)), observer: 1}
		referenceLog := slog.New(slog.NewTextHandler(&wantLog, opts))
		events := []event.Event{event.EntityMoved{ID: 1000}, event.EntityMoved{ID: 1001}, event.WarehouseSnapshot{Who: 1}}
		var want [][]byte
		for _, ev := range events {
			packets := protocol.Encode(s.observer, ev)
			if len(packets) == 0 {
				referenceLog.Debug("事件无对应下行包", "event", eventName(ev))
			}
			for _, p := range packets {
				referenceLog.Debug("下行", "op", logHex(uint16(p[0])|uint16(p[1])<<8), "字节", len(p), "event", eventName(ev))
				want = append(want, append([]byte(nil), p...))
			}
			s.Emit(ev)
		}
		if gotLog.String() != wantLog.String() {
			t.Fatalf("level %s logging changed:\n%s\nwant:\n%s", level, gotLog.String(), wantLog.String())
		}
		if !reflect.DeepEqual(out.packets, want) {
			t.Fatalf("level %s packet order/bytes/retained ownership changed", level)
		}
	}
}

func BenchmarkOptimizationDiffComparison(b *testing.B) {
	for _, n := range []int{32, 128} {
		for _, reference := range []bool{false, true} {
			b.Run(fmt.Sprintf("items=%d/reference=%v", n, reference), func(b *testing.B) {
				old := event.InventorySnapshot{Items: make([]event.InventoryItemView, n), Equipped: make([]event.EquippedItemView, 12)}
				for i := range old.Items {
					old.Items[i] = event.InventoryItemView{BagIndex: int32(i), Item: 1, Name: "item"}
				}
				for i := range old.Equipped {
					old.Equipped[i] = event.EquippedItemView{Slot: domain.EquipSlot(i), Item: 1}
				}
				next := cloneInventory(old)
				next.Money++
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if reference {
						inventoryDiffOriginalOptimization(old, next)
					} else {
						inventoryDiff(old, next)
					}
				}
			})
		}
	}
}

func TestOptimizationSnapshotPacketParity(t *testing.T) {
	base := event.InventorySnapshot{Who: 1, Items: []event.InventoryItemView{{BagIndex: 0, Item: 1, Count: 2, Name: "first", Slot: -1}, {BagIndex: 2, Item: 2, Count: 1, Name: "second", Slot: -1}}, Equipped: []event.EquippedItemView{{Slot: 0, Item: 3}, {Slot: 1, Item: 4}}}
	for variant := 0; variant < 8; variant++ {
		next := cloneInventory(base)
		switch variant {
		case 0:
			next.Money++
		case 1:
			next.Items[0].Count++
		case 2:
			next.Items = next.Items[1:]
			next.Equipped = next.Equipped[1:]
		case 3:
			next.Items[1] = next.Items[0]
		case 4:
			next.Equipped[1] = next.Equipped[0]
		case 5:
			next.Items[0].Pet = &domain.PetItemInfo{Name: "pet", Skills: []domain.PetItemSkill{{Name: "skill", Level: 1}}}
		case 6:
			next.Items[0], next.Items[1] = next.Items[1], next.Items[0]
		}
		// Derive the exact packet choice and bytes using the unoptimized diff.
		items, equipped, changed, ok := inventoryDiffOriginalOptimization(base, next)
		var want [][]byte
		fallback := !ok
		for _, item := range items {
			fallback = fallback || item.Pet != nil
		}
		if !fallback && changed {
			p := protocol.InventoryDelta(next, items, equipped)
			if p == nil {
				fallback = true
			} else {
				want = [][]byte{p}
			}
		}
		if fallback {
			want = protocol.Encode(1, next)
		}
		cached := cloneInventory(base)
		s := eventSink{observer: 1, inventory: &cached}
		if got := s.inventoryPackets(next); !reflect.DeepEqual(got, want) {
			t.Fatalf("inventory packet variant %d", variant)
		}
		if s.inventory != nil && len(next.Items) > 0 {
			retained := s.inventory.Items[0]
			next.Items[0].Name = "caller changed"
			if s.inventory.Items[0] != retained {
				t.Fatal("cached inventory aliases caller slice")
			}
		}
	}
	baseWarehouse := event.WarehouseSnapshot{Who: 1, PageCount: 1, Items: []event.WarehouseItemView{{Slot: 0, Item: 1, Count: 2}, {Slot: 2, Item: 2, Count: 1}}}
	for variant := 0; variant < 5; variant++ {
		next := cloneWarehouse(baseWarehouse)
		switch variant {
		case 0:
			next.Money++
		case 1:
			next.Items[0].Count++
		case 2:
			next.Items = next.Items[1:]
		case 3:
			next.Items[1] = next.Items[0]
		}
		items, changed, ok := warehouseDiffOriginalOptimization(baseWarehouse, next)
		var want [][]byte
		if ok && changed {
			p := protocol.WarehouseDelta(next, items)
			if p == nil {
				ok = false
			} else {
				want = [][]byte{p}
			}
		}
		if !ok {
			want = protocol.Encode(1, next)
		}
		cached := cloneWarehouse(baseWarehouse)
		s := eventSink{observer: 1, warehouse: &cached}
		if got := s.warehousePackets(next); !reflect.DeepEqual(got, want) {
			t.Fatalf("warehouse packet variant %d", variant)
		}
		if s.warehouse != nil && len(next.Items) > 0 {
			retained := s.warehouse.Items[0]
			next.Items[0].Name = "caller changed"
			if s.warehouse.Items[0] != retained {
				t.Fatal("cached warehouse aliases caller slice")
			}
		}
	}
}

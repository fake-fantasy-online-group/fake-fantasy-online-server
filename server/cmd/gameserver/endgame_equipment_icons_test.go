package main

import (
	"encoding/binary"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func TestRegisterEndgameEquipmentIconsCoverEveryBaseAndPreserveSpecial(t *testing.T) {
	w := endgameWorldFixture()
	// Real ov_desc aliases. These are sprite IDs, never item-table lookups.
	expected := map[domain.ItemID]int32{2101: 2101, 20824: 22090, 22581: 30041, 22593: 40330}
	for id, icon := range expected {
		d := w.items[id]
		d.Icon = icon
		w.items[id] = d
	}
	// Original extended-slot special artwork must retain priority over ov_desc.
	special := domain.ItemIconMapping{Item: 57093, Icon: 8231}
	if _, ok := w.items[special.Item]; !ok {
		t.Fatal("special fixture must be a unique base")
	}
	w.itemIcons = []domain.ItemIconMapping{special}
	if err := registerEndgameItems(&w); err != nil {
		t.Fatal(err)
	}
	got := make(map[domain.ItemID]int32)
	for _, pair := range w.itemIcons {
		if _, duplicate := got[pair.Item]; duplicate {
			t.Fatalf("duplicate mapping %d", pair.Item)
		}
		got[pair.Item] = pair.Icon
	}
	for _, baseID := range domain.UniqueAppearanceBaseIDs() {
		want := w.items[baseID].Icon
		if baseID == special.Item {
			want = special.Icon
		}
		if got[baseID] != want {
			t.Fatalf("base %d slot %d maps to %d, want %d", baseID, w.items[baseID].Equip.Slot, got[baseID], want)
		}
	}
	for id, icon := range expected {
		if got[id] != icon {
			t.Fatalf("boots %d mapped to %d, want %d", id, got[id], icon)
		}
	}
	// Execute the native encoder, not only a count assertion. Full loaded-world
	// special/pet mappings are also counted by the migration-source audit.
	packets := protocol.Encode(1, event.ItemIconMap{Who: 1, Pairs: w.itemIcons})
	if len(packets) != 1 || len(packets[0]) != 4+8*(len(domain.UniqueAppearanceBaseIDs())+32) {
		t.Fatalf("invalid equipment icon packet: %d packets", len(packets))
	}
	if binary.LittleEndian.Uint16(packets[0]) != protocol.SCItemIconMap || int(binary.LittleEndian.Uint16(packets[0][2:])) != len(got) {
		t.Fatal("native icon packet header")
	}
}

func TestRegisterEndgameEquipmentIconsNeverInventsSwordFallback(t *testing.T) {
	w := endgameWorldFixture()
	d := w.items[22581]
	d.Icon = 0
	w.items[22581] = d
	// A missing sprite mapping remains unresolved; category/quality/unique ID is
	// not permission to substitute a sword or another item's lookup identity.
	registerEndgameEquipmentIcons(&w)
	for _, pair := range w.itemIcons {
		if pair.Item == 22581 {
			t.Fatalf("invented mapping: %+v", pair)
		}
	}
	before := len(w.itemIcons)
	registerEndgameEquipmentIcons(&w)
	if len(w.itemIcons) != before {
		t.Fatal("repeated registration duplicated mappings")
	}
}

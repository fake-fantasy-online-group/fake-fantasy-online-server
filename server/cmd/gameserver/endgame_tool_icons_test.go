package main

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func TestRegisterPortableCubeUsesPersistedNativeIcon(t *testing.T) {
	w := endgameWorldFixture()
	const tool domain.ItemID = 1900028301
	// SQL84 selects ov_desc(index=9515, 清神炉).small_map, which is sprite
	// 9509 in SQL28. The source item ID 9515 is NOT the artwork ID.
	w.items[tool] = domain.ItemDef{ID: tool, Name: "周天鼎", Icon: 9509, UseAction: "cube"}
	before := w.items[tool]
	for i := 0; i < 2; i++ {
		if err := registerEndgameItems(&w); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(w.items[tool], before) {
		t.Fatal("icon registration changed the gameplay definition")
	}
	var count int
	for _, pair := range w.itemIcons {
		if pair.Item == tool {
			count++
			if pair.Icon != 9509 {
				t.Fatalf("wrong cauldron sprite: %+v", pair)
			}
		}
	}
	if count != 1 {
		t.Fatalf("portable tool missing/duplicated: %d", count)
	}
	packets := protocol.Encode(1, event.ItemIconMap{Who: 1, Pairs: w.itemIcons})
	if len(packets) != 1 || binary.LittleEndian.Uint16(packets[0]) != protocol.SCItemIconMap {
		t.Fatal("native icon mapping was not encoded")
	}
	var wireCount int
	for offset := 4; offset+8 <= len(packets[0]); offset += 8 {
		if binary.LittleEndian.Uint32(packets[0][offset:]) == uint32(tool) {
			wireCount++
			if binary.LittleEndian.Uint32(packets[0][offset+4:]) != 9509 {
				t.Fatal("wire lost the original sprite alias")
			}
		}
	}
	if wireCount != 1 {
		t.Fatal("portable cube omitted from 0x804d")
	}
}

func TestRegisterToolIconsAreDataDrivenBoundedAndIdempotent(t *testing.T) {
	w := worldData{items: data.ItemDefs{
		300: {ID: 300, Icon: 700, UseAction: "cube"},
		200: {ID: 200, Icon: 500, UseAction: "cube"},
		400: {ID: 400, Icon: 0, UseAction: "cube"},
		500: {ID: 500, Icon: -1, UseAction: "cube"},
		600: {ID: 600, Icon: 900, UseAction: "unrelated"},
	}}
	w.itemIcons = []domain.ItemIconMapping{{Item: 300, Icon: 777}}
	registerEndgameToolIcons(&w)
	registerEndgameToolIcons(&w)
	want := []domain.ItemIconMapping{{Item: 300, Icon: 777}, {Item: 200, Icon: 500}}
	if !reflect.DeepEqual(w.itemIcons, want) {
		t.Fatalf("must preserve explicit mappings, skip missing art and never guess: %#v", w.itemIcons)
	}
}

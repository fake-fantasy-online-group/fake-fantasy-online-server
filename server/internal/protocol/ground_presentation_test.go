package protocol

import (
	"bytes"
	"encoding/binary"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestGroundPresentationOptionalTailAndExactIdentity(t *testing.T) {
	e := event.EntitySpawned{ID: 77, Kind: domain.KindDrop, Name: "暗金", Ground: &event.GroundItemView{Item: 1601, Count: 1, Quality: 4, UniqueID: 1601, FreshDrop: true, DropToken: 9007199254740993}}
	legacy := Encode(1, e)[0]
	extended := GroundSpawnPresentation(1, e)
	if len(extended) != len(legacy)+14 || !bytes.Equal(extended[:len(legacy)], legacy) {
		t.Fatal("legacy prefix changed")
	}
	tail := extended[len(legacy):]
	if tail[0] != 1 || tail[1] != 1 || binary.LittleEndian.Uint32(tail[2:]) != 1601 || binary.LittleEndian.Uint64(tail[6:]) != 9007199254740993 {
		t.Fatal("lost v1 identity", tail)
	}
	e.Ground.FreshDrop = false
	snapshot := GroundSpawnPresentation(1, e)
	if snapshot[len(legacy)+1] != 0 {
		t.Fatal("snapshot marked fresh")
	}
	for _, caps := range []int32{31, 63} {
		r, ok := DecodeClientCaps(NewW(0).I32(caps).Bytes()[2:])
		if !ok || r.ClientCaps != caps {
			t.Fatal("capability not decoded", caps)
		}
	}
	for _, payload := range [][]byte{nil, {31}, {31, 0, 0, 0, 0}} {
		if _, ok := DecodeClientCaps(payload); ok {
			t.Fatal("malformed capability accepted")
		}
	}
}

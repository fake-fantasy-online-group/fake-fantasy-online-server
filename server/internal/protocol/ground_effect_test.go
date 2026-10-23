package protocol

import (
	"bytes"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestGroundEffectWireAndLegacyUnchanged(t *testing.T) {
	e := event.GroundEffect{Owner: 7, MapID: 2, Pos: domain.Pos{X: 120, Y: 240}, Key: "zone", Effect: "FX", DurationMS: 1500, Mode: 1}
	want := NewW(0x80F1).I32(7).I32(2).I32(120).I32(240).Str("zone").Str("FX").I32(1500).U8(1).Bytes()
	got := Encode(7, e)
	if len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("wire %x", got)
	}
	e.Mode = 2
	e.Effect = ""
	if len(Encode(7, e)) != 1 {
		t.Fatal("explicit stop rejected")
	}
	e.Mode = 0
	if len(Encode(7, e)) != 0 {
		t.Fatal("empty play accepted")
	}
	e.Effect = "FX"
	e.DurationMS = -1
	if len(Encode(7, e)) != 0 {
		t.Fatal("negative duration")
	}
	old := Encode(7, event.SkillHitEffect{Target: 7, Effect: "FX"})
	if len(old) != 1 || !bytes.Equal(old[0], NewW(0x8045).I32(7).Str("FX").Bytes()) {
		t.Fatal("legacy changed")
	}
}

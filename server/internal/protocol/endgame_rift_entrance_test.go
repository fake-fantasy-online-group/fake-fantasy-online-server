package protocol

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestRiftEntranceUnsignedEntityRequest(t *testing.T) {
	good := `{"op":"rift.start","requestId":"rift-entrance-0001","expectedRevision":"0","tier":1,"npcEntityId":3221225473}`
	payload := func(s string) []byte { return NewW(OpEndgame).U16(domain.EndgameSchema).Str(s).Bytes()[2:] }
	got, ok := DecodeEndgame(payload(good))
	if !ok || got.NPCEntityID != domain.EntityID(3221225473) {
		t.Fatalf("native unsigned ID lost: %+v %v", got, ok)
	}
	for _, bad := range []string{
		`{"op":"rift.start","requestId":"rift-entrance-0001","expectedRevision":"0","tier":1,"npcEntityId":-1}`,
		`{"op":"rift.start","requestId":"rift-entrance-0001","expectedRevision":"0","tier":1,"npcEntityId":4294967296}`,
		`{"op":"rift.start","requestId":"rift-entrance-0001","expectedRevision":"0","tier":1,"npcEntityId":1.5}`,
		`{"op":"rift.state","npcEntityId":3221225473}`,
		`{"op":"rift.start","requestId":"rift-entrance-0001","expectedRevision":"0","tier":1,"NpcEntityId":3221225473}`,
	} {
		if _, ok := DecodeEndgame(payload(bad)); ok {
			t.Fatal("invalid NPC entity accepted:", bad)
		}
	}
}

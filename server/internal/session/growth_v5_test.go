package session

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"testing"
)

func growthPacket(r domain.EndgameRequest) []byte {
	b, _ := json.Marshal(r)
	return protocol.NewW(0).U16(domain.EndgameSchema).Str(string(b)).Bytes()[2:]
}
func TestGrowthV5SessionHandshakeAndRankWire(t *testing.T) {
	r := newNativeStatsRig(t, domain.EndgameLoadResult{})
	r.s.deps.RequireGrowthV5 = true
	r.s.OnPacket(0x100c, []byte{2, 0, 0, 0})
	res := nativeStatsResponses(t, r.take())
	if len(res) != 1 || res[0].Error != "update_required" {
		t.Fatalf("pre-hello attack accepted: %+v", res)
	}
	r.s.OnPacket(protocol.OpEndgame, growthPacket(domain.EndgameRequest{Op: "hello", Capabilities: []string{domain.EndgameV1, domain.EndgameNativeStatsV1, domain.EndgameGrowthV5}}))
	r.sync()
	res = nativeStatsResponses(t, r.take())
	found := false
	for _, v := range res {
		if v.Kind == "skill.ranks" && v.OK && v.Result["hardCap"] == "20" {
			found = true
		}
	}
	if !found || !r.s.growthReady() {
		t.Fatalf("V5 handshake/rank view missing: %+v", res)
	}
	r.inspect(func(sc *scene.Scene) {
		if !sc.EntityAt(1).Player.EndgameGrowthV5 {
			t.Fatal("scene capability not installed")
		}
	})
	r.s.OnPacket(protocol.OpEndgame, growthPacket(domain.EndgameRequest{Op: "skill.ranks", RequestID: "growth-ranks-request"}))
	r.sync()
	res = nativeStatsResponses(t, r.take())
	found = false
	for _, v := range res {
		if v.Kind == "skill.ranks" && v.RequestID == "growth-ranks-request" && v.OK {
			found = true
		}
	}
	if !found {
		t.Fatalf("actual decoder/router lost skill.ranks: %+v", res)
	}
}

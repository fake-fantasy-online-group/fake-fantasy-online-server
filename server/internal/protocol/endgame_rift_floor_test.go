package protocol

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestRiftAdvanceClosedProtocolAndDifficultyRange(t *testing.T) {
	good := `{"op":"rift.advance","requestId":"rift-floor-advance-01","expectedRevision":"0","runId":"seeded-run","floorIndex":2}`
	r, ok := DecodeEndgame(endgamePayload(good))
	if !ok || r.FloorIndex != 2 || !r.Mutates() {
		t.Fatal("advance identity did not survive decode")
	}
	for _, bad := range []string{
		`{"op":"rift.advance","requestId":"rift-floor-advance-01","expectedRevision":"0","runId":"seeded-run"}`,
		`{"op":"rift.advance","requestId":"rift-floor-advance-01","expectedRevision":"0","floorIndex":2}`,
		`{"op":"rift.advance","requestId":"rift-floor-advance-01","runId":"seeded-run","floorIndex":2}`,
		`{"op":"rift.advance","requestId":"rift-floor-advance-01","expectedRevision":"0","runId":"seeded-run","floorIndex":-1}`,
		`{"op":"rift.start","requestId":"rift-floor-start-001","expectedRevision":"0","tier":151}`,
	} {
		if _, ok := DecodeEndgame(endgamePayload(bad)); ok {
			t.Fatalf("accepted malformed command: %s", bad)
		}
	}
	if _, ok := DecodeEndgame(endgamePayload(`{"op":"rift.start","requestId":"rift-floor-start-001","expectedRevision":"0","tier":150}`)); !ok {
		t.Fatal("tier150 rejected")
	}
	v := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: &domain.EndgameRiftView{Seed: "18446744073709551615", GeneratorVersion: 1, FloorIndex: 2, Tier: 150, MaxTier: 150, UnlockedTier: 150, Telegraphs: []domain.EndgameRiftTelegraphView{{ID: "cast-1", EntityID: "99", Ability: "震环", Shape: "ring", Radius: 240, InnerRadius: 80, RemainingMS: 1400}}}}
	if len(EncodeEndgame(v)) == 0 {
		t.Fatal("new state failed encoding")
	}
}

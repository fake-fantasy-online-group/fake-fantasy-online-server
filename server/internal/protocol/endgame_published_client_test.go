package protocol

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"testing"
)

func TestPublishedClientHelloCompatibility(t *testing.T) {
	b, err := os.ReadFile("testdata/client-1.0.6-hello.payload.bin")
	if err != nil {
		t.Fatal(err)
	}
	r, ok := DecodeEndgame(b)
	if !ok || len(r.Capabilities) != 10 {
		t.Fatalf("published client hello rejected: caps=%d accepted=%v", len(r.Capabilities), ok)
	}
	for _, tc := range []struct {
		name     string
		caps     []string
		accepted bool
	}{
		{"legacy8", r.Capabilities[:8], true},
		{"codex9", r.Capabilities[:9], true},
		{"spirit9", append(append([]string{}, r.Capabilities[:8]...), domain.EndgameSpiritV1), true},
		{"unrelated9", append(append([]string{}, r.Capabilities[:8]...), "unknown"), false},
		{"full10", r.Capabilities, true},
		{"unrelated11", append(append([]string{}, r.Capabilities...), "unknown"), false},
		{"duplicateCodex10", append(append([]string{}, r.Capabilities[:9]...), domain.EndgamePowerCodexV1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := r
			v.Capabilities = tc.caps
			j, _ := json.Marshal(v)
			_, ok := DecodeEndgame(endgamePayload(string(j)))
			if ok != tc.accepted {
				t.Fatalf("accepted=%v want=%v", ok, tc.accepted)
			}
		})
	}
}

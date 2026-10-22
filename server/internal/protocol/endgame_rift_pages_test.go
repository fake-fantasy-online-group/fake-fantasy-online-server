package protocol

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestRiftPagesNativeFramesPreserveCompleteResponse(t *testing.T) {
	v := domain.EndgameResponse{Kind: "rift.start", OK: true, RequestID: "paged-admission-receipt", Revision: "7",
		Rift: &domain.EndgameRiftView{RunID: "dense-run", Revision: "7", Phase: "active", MapID: 7,
			ServerNowMS: 100000, GeneratorVersion: 6, FloorIndex: 1, Tier: 1, ProgressMax: 1000,
			MaxTier: 150, UnlockedTier: 1, Seed: "123", KeyItemID: int32(domain.RiftKeyItemID)}}
	for i := 1; i <= 5000; i++ {
		v.Rift.Auras = append(v.Rift.Auras, domain.EndgameRiftAuraView{EntityID: strconv.Itoa(i), Class: 0})
	}
	raw, _ := json.Marshal(v)
	if len(raw) <= domain.EndgameMaxJSONBytes || EncodeEndgame(v) != nil {
		t.Fatal("fixture must exceed old native frame")
	}
	pages, err := domain.PageEndgameRift(v, "actual-go-wire")
	if err != nil || len(pages) < 2 {
		t.Fatal(err)
	}
	var joined []byte
	for i, page := range pages {
		if page.Kind != "rift.page" || page.Chunk != int32(i) || page.Chunks != int32(len(pages)) || page.TotalBytes != int32(len(raw)) || page.RequestID != "" || page.Rift != nil {
			t.Fatal("invalid transport-only envelope")
		}
		packet := EncodeEndgame(page)
		if len(packet) == 0 || len(packet) > domain.EndgameMaxJSONBytes+6 {
			t.Fatal("page exceeds unchanged wire frame")
		}
		r := newReader(packet[2:])
		if r.u16() != domain.EndgameSchema {
			t.Fatal("changed schema")
		}
		var decoded domain.EndgameResponse
		if json.Unmarshal([]byte(r.str()), &decoded) != nil || !r.done() {
			t.Fatal("malformed packet")
		}
		part, err := base64.StdEncoding.DecodeString(decoded.Data)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, part...)
		if dir := os.Getenv("RIFT_PAGES_FIXTURE_DIR"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("rift-page-%02d.payload.bin", i)), packet[2:], 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !bytes.Equal(joined, raw) {
		t.Fatal("lost or altered complete receipt or aura bytes")
	}
	var roundtrip domain.EndgameResponse
	if json.Unmarshal(joined, &roundtrip) != nil || len(roundtrip.Rift.Auras) != 5000 || roundtrip.RequestID != v.RequestID {
		t.Fatal("truncated population or original receipt")
	}
	t.Logf("5000 actors: %d raw bytes, %d unchanged-schema frames", len(raw), len(pages))
}

func TestRiftPagesResourceBudgetAndLegacySmallFrames(t *testing.T) {
	v := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: &domain.EndgameRiftView{RunID: "run", Revision: "1", Phase: "active"}}
	pages, err := domain.PageEndgameRift(v, "small")
	if err != nil || len(pages) != 1 || pages[0].Kind != v.Kind {
		t.Fatal("small complete responses need no wrapper")
	}
	v.Message = strings.Repeat("x", domain.EndgameRiftSnapshotBytes)
	if _, err = domain.PageEndgameRift(v, "large"); err == nil {
		t.Fatal("unbounded aggregate")
	}
	for _, raw := range []string{
		`{"op":"capabilities","requestId":"caps-1","capabilities":["endgame.v1","endgame.rift-pages.v1"]}`,
		`{"op":"hello","capabilities":["endgame.v1","a","b","c","d","e","f","g"]}`,
	} {
		r, ok := DecodeEndgame(endgamePayload(raw))
		if !ok || r.Mutates() {
			t.Fatalf("rejected read-only negotiation: %s", raw)
		}
	}
	if _, ok := DecodeEndgame(endgamePayload(`{"op":"hello","capabilities":["endgame.v1","a","b","c","d","e","f","g","h"]}`)); ok {
		t.Fatal("unnecessarily changed legacy capability count guard")
	}
}

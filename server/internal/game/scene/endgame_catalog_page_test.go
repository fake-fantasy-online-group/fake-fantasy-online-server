package scene

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestCatalogPageUsesEncodedByteBudget(t *testing.T) {
	r := domain.EndgameResponse{Kind: "catalog.uniques", OK: true, RequestID: strings.Repeat("x", 64)}
	for i := 0; i < 32; i++ {
		r.Items = append(r.Items, domain.EndgameItemView{ItemID: int32(i + 1), Description: strings.Repeat("仙<&", 1000)})
	}
	if !fitEndgameCatalogPage(&r, 7, 80) {
		t.Fatal("could not fit bounded records")
	}
	if len(r.Items) == 0 || len(r.Items) >= 32 || r.NextCursor != strconv.Itoa(7+len(r.Items)) {
		t.Fatalf("invalid shrunk cursor %d/%s", len(r.Items), r.NextCursor)
	}
	r.Revision = "9223372036854775807"
	b, err := json.Marshal(r)
	if err != nil || len(b) > domain.EndgameMaxJSONBytes {
		t.Fatalf("payload=%d err=%v", len(b), err)
	}
	r = domain.EndgameResponse{Items: []domain.EndgameItemView{{Description: strings.Repeat("x", domain.EndgameMaxJSONBytes)}}}
	if fitEndgameCatalogPage(&r, 0, 1) {
		t.Fatal("oversized single entry accepted")
	}
	r = domain.EndgameResponse{}
	if !fitEndgameCatalogPage(&r, 80, 80) || r.NextCursor != "" {
		t.Fatal("empty end page rejected")
	}
}

func TestSnapshotByteChunksPreserveRowsAndHeader(t *testing.T) {
	base := domain.EndgameResponse{Kind: "snapshot", SnapshotID: "1-2-3", OK: true,
		Materials: []domain.EndgameMaterialView{{Name: strings.Repeat("x", 50000)}}}
	items := make([]domain.EndgameItemView, 65)
	for i := range items {
		items[i] = domain.EndgameItemView{UID: strconv.Itoa(i + 1), Description: strings.Repeat("字", 6000)}
	}
	chunks, ok := splitEndgameSnapshot(base, items)
	if !ok || len(chunks) < 2 || len(chunks[0].Items) != 0 {
		t.Fatal("header-only first chunk was not supported")
	}
	n := 0
	for i, c := range chunks {
		if c.Chunk != int32(i) || c.Chunks != int32(len(chunks)) {
			t.Fatal("non-contiguous envelope")
		}
		if i > 0 && (len(c.Materials) != 0 || len(c.Formations) != 0 || c.CombatStats != nil) {
			t.Fatal("header repeated")
		}
		c.Revision = "9223372036854775807"
		b, err := json.Marshal(c)
		if err != nil || len(b) > domain.EndgameMaxJSONBytes {
			t.Fatalf("oversize chunk %d", i)
		}
		for _, item := range c.Items {
			n++
			if item.UID != strconv.Itoa(n) {
				t.Fatal("row lost or reordered")
			}
		}
	}
	if n != len(items) {
		t.Fatal("rows missing")
	}
	if _, ok := splitEndgameSnapshot(domain.EndgameResponse{}, []domain.EndgameItemView{{Description: strings.Repeat("x", domain.EndgameMaxJSONBytes)}}); ok {
		t.Fatal("oversized item accepted")
	}
	if chunks, ok := splitEndgameSnapshot(domain.EndgameResponse{}, nil); !ok || len(chunks) != 1 {
		t.Fatal("empty inventory rejected")
	}
}

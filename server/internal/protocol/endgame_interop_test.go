package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These fixtures are also consumed by client plain-C# tests. Regenerate only by
// explicitly setting ENDGAME_FIXTURE_OUT; normal tests compare committed bytes.
func TestEndgameInteropFixtures(t *testing.T) {
	fixtures := map[string]domain.EndgameResponse{
		"hello":          {Kind: "hello", OK: true, Capabilities: []string{domain.EndgameV1}},
		"snapshot_1":     {Kind: "snapshot", OK: true, Revision: "17", SnapshotID: "snapshot-17", Chunk: 0, Chunks: 2, Items: []domain.EndgameItemView{{UID: "9007199254740993", ItemID: 4001, Location: "bag", Tab: 2, Slot: 25, Revision: "3", UniqueID: 4001, UniqueRollBP: 10123, Name: "天罡·试剑", SocketCount: 2, Sockets: []domain.EndgameSocketView{{Index: 0, ItemID: 1900028001, UID: "9223372036854775807", Name: "角木篆"}, {Index: 1}}}}},
		"snapshot_2":     {Kind: "snapshot", OK: true, Revision: "17", SnapshotID: "snapshot-17", Chunk: 1, Chunks: 2, Items: []domain.EndgameItemView{{UID: "9223372036854775806", ItemID: 1900028002, Location: "bag", Tab: 1, Slot: 7, Revision: "0", Name: "亢金篆"}}},
		"snapshot_empty": {Kind: "snapshot", OK: true, Revision: "18", SnapshotID: "snapshot-18", Chunk: 0, Chunks: 1, Items: []domain.EndgameItemView{}, Formations: []domain.EndgameFormationView{{ID: 1, Name: "青龙宿阵", Channel: "战阵", Runes: []int32{1900028001, 1900028002}, Active: true, Stats: []domain.EndgameStatView{{Attr: 1, Value: 50, Mode: 0}}}}, Materials: []domain.EndgameMaterialView{{ItemID: 1900028201, Count: 12, Name: "星髓"}}},
		"catalog_runes":  {Kind: "catalog.runes", OK: true, CatalogVersion: "daoist-v1", Cursor: "28", NextCursor: "", Runes: []domain.EndgameRuneView{{MinLevel: 12, ItemID: 1900028001, Name: "角木篆", Mansion: "角", Quadrant: "青龙", Description: "可镶嵌的实体星篆", Stats: []domain.EndgameStatView{{Attr: 1, Value: 12, Mode: 0}}}}},
		"receipt":        {Kind: "cube.result", RequestID: "fixture-000000001", OK: true, Revision: "19", Result: map[string]string{"uid": "9007199254740993", "uniqueRollBp": "10123"}},
	}
	out := os.Getenv("ENDGAME_FIXTURE_OUT")
	dir := filepath.Join("testdata", "endgame")
	for name, v := range fixtures {
		t.Run(name, func(t *testing.T) {
			wire := EncodeEndgame(v)
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if out != "" {
				for _, d := range []string{dir, out} {
					if err = os.MkdirAll(d, 0755); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(d, name+".wire.hex"), []byte(hex.EncodeToString(wire)+"\n"), 0644); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(d, name+".json"), raw, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			for ext, want := range map[string][]byte{".wire.hex": wire, ".json": raw} {
				got, err := os.ReadFile(filepath.Join(dir, name+ext))
				if err != nil {
					t.Fatal(err)
				}
				if ext == ".wire.hex" {
					got, err = hex.DecodeString(strings.TrimSpace(string(got)))
					if err != nil {
						t.Fatal(err)
					}
				}
				if !bytes.Equal(got, want) {
					t.Fatal("wire fixture drift", name, ext)
				}
			}
		})
	}
}

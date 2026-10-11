// Synthetic cross-language fixtures, produced by the production server encoder.
package main

import (
	"encoding/hex"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: endgame-fixtures output-directory")
	}
	out := os.Args[1]
	if err := os.MkdirAll(out, 0755); err != nil {
		panic(err)
	}
	fixtures := map[string]domain.EndgameResponse{
		"hello":          {Kind: "hello", OK: true, Revision: "0", Capabilities: []string{domain.EndgameV1}},
		"snapshot-0":     {Kind: "snapshot", OK: true, Revision: "3", SnapshotID: "fixture-v1", Chunk: 0, Chunks: 2, Items: []domain.EndgameItemView{{UID: "9007199254740993", ItemID: 22211, Location: "bag", Tab: 0, Slot: 99, Revision: "2", UniqueID: 22211, UniqueRollBP: 10000, Name: "青冥承露", SocketCount: 1, Sockets: []domain.EndgameSocketView{{Index: 0, ItemID: 1900028001, UID: "9223372036854775806"}}}}},
		"snapshot-1":     {Kind: "snapshot", OK: true, Revision: "3", SnapshotID: "fixture-v1", Chunk: 1, Chunks: 2, Items: []domain.EndgameItemView{{UID: "9007199254740994", ItemID: 22212, Location: "equipped", Slot: 25, Revision: "0", Name: "玄门护佩", SocketCount: 0}}},
		"snapshot-empty": {Kind: "snapshot", OK: true, Revision: "4", SnapshotID: "fixture-empty", Chunks: 1},
		"runes":          {Kind: "catalog.runes", OK: true, CatalogVersion: "fixture", Runes: []domain.EndgameRuneView{{ItemID: 1900028001, Name: "角木蛟·星篆", Quadrant: "东方青龙", Stats: []domain.EndgameStatView{{Attr: 1, Value: 5, Mode: 0}}}}},
		"mutation":       {Kind: "socket.insert", RequestID: "fixture-request-0001", OK: true, Revision: "4", Message: "星篆已镶入"},
		"rift":           {Kind: "rift.state", OK: true, Revision: "4", Rift: &domain.EndgameRiftView{RunID: "rift-fixture-001", Revision: "5", Phase: "active", Tier: 1, Progress: 40, ProgressMax: 10000, RemainingSeconds: 899, Members: []domain.EndgameRiftMemberView{{CharID: "9007199254740993", Level: 50, Eligible: true}}}},
	}
	for name, r := range fixtures {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			panic(err)
		}
		if err = os.WriteFile(filepath.Join(out, name+".json"), append(b, '\n'), 0644); err != nil {
			panic(err)
		}
		wire := protocol.EncodeEndgame(r)
		if wire == nil {
			panic("encode failed")
		}
		if err = os.WriteFile(filepath.Join(out, name+".wire.hex"), []byte(hex.EncodeToString(wire)+"\n"), 0644); err != nil {
			panic(err)
		}
	}
}

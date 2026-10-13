package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"unicode/utf16"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func TestEndgameCatalogFitsClientStatAndTextBounds(t *testing.T) {
	// Paired EndgameData.String defaults to 8192 UTF-16 code units, and
	// EndgameItemView/EndgameFormationView parse at most 64 extra stat rows.
	check := func(id int32, stats []domain.Affix, text ...string) {
		t.Helper()
		if len(stats) > 64 {
			t.Fatal("client stat count exceeded", id, len(stats))
		}
		for _, value := range text {
			if len(utf16.Encode([]rune(value))) > 8192 {
				t.Fatal("client text limit exceeded", id)
			}
		}
	}
	for _, d := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
		check(d.ID, d.Affixes, d.Name, d.Description(), uniqueTraitDescription(d.Traits))
	}
	for _, d := range domain.AstralRecipes(catalogfixture.EndgameCatalog()) {
		check(d.ID, d.Affixes, d.Name, d.Description())
	}
}

func TestEndgameExtraAffixViewsKeepMoreThanSixLines(t *testing.T) {
	var affixes []domain.Affix
	for _, attr := range []int32{1, 3, 5, 7, 9, 13, 15, 17, 19, 34} {
		affixes = append(affixes, domain.Affix{Attr: attr, Value: 20})
	}
	stats := endgameStats(affixes)
	if len(stats) != 10 {
		t.Fatal("long extra-attribute slice was truncated", len(stats))
	}
	response := domain.EndgameResponse{Kind: "snapshot", OK: true, SnapshotID: "long-affixes", Chunks: 1,
		Items:      []domain.EndgameItemView{{UID: "9007199254740993", Stats: stats}},
		Formations: []domain.EndgameFormationView{{ID: 1, Stats: stats}}}
	wire := protocol.EncodeEndgame(response)
	if len(wire) < 6 {
		t.Fatal("extended affix snapshot failed encoding")
	}
	var decoded domain.EndgameResponse
	if err := json.Unmarshal(wire[6:], &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Items[0].Stats, stats) || !reflect.DeepEqual(decoded.Formations[0].Stats, stats) {
		t.Fatal("wire serialization lost extended extra attributes")
	}
}

func TestEndgameFormationCatalogEveryPagePreservesAllExtras(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 0, 0)
	sink.take()
	p := s.players[1]
	s.endgamePlayer(p).loaded = true
	cursor := ""
	seen := map[int32]bool{}
	for page := 0; page < 2; page++ {
		s.sendEndgameCatalog(p, domain.EndgameRequest{Op: "catalog", Catalog: "formations", Cursor: cursor})
		responses := endgameMessages(s, sink)
		if len(responses) != 1 || !responses[0].OK {
			t.Fatal(responses)
		}
		r := responses[0]
		if len(protocol.EncodeEndgame(r)) == 0 || len(r.Formations) > 32 {
			t.Fatal("formation catalog exceeds wire bound")
		}
		for _, item := range r.Formations {
			definition, ok := domain.AstralRecipeByID(item.ID, catalogfixture.EndgameCatalog())
			if !ok || seen[item.ID] || len(item.Stats) < domain.MinBeneficialEndgameExtraAffixes ||
				!reflect.DeepEqual(item.Stats, endgameStats(domain.ResolveVariableAffixes(definition.Affixes, definition.VariableAffixes, 10000))) {
				t.Fatal("formation extras missing or truncated", item.ID)
			}
			seen[item.ID] = true
		}
		cursor = r.NextCursor
	}
	if len(seen) != len(domain.AstralRecipes(catalogfixture.EndgameCatalog())) || cursor != "" {
		t.Fatal("formation pagination incomplete", len(seen), cursor)
	}
}

func TestEndgameWorstFullSnapshotFitsExtendedAffixWireBudget(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 0, 0)
	sink.take()
	p := s.players[1]
	s.endgamePlayer(p).loaded = true
	s.items = map[domain.ItemID]domain.ItemDef{}
	var largest domain.Stack
	maxBytes := 0
	// Use the longest fully populated owned unique, repeated 32 times, rather
	// than assuming the first catalog page is also the largest snapshot page.
	for _, d := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
		s.items[d.BaseID] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: d.BaseID, Name: d.BaseName, InventoryTab: 2,
			InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: d.Slot, Durable: 100,
				Need: domain.Requirement{Level: d.LevelRequirement, Professions: d.Professions}}}
		st := domain.Stack{UID: 9223372036854775000, Item: d.BaseID, Count: 1, Durability: 100,
			MaxDurability: 100, SocketCount: 5, Bound: true, Locked: true,
			Endgame: domain.EndgameItemState{UniqueID: d.ID, UniqueRollBP: domain.UniqueRollMaxBP, Revision: 9223372036854775000}}
		for i := range st.Sockets {
			st.Sockets[i] = domain.FirstAstralRune + domain.ItemID(i)
			st.SocketUIDs[i] = 9223372036854774000 + int64(i)
		}
		view := s.endgameItem(st, "bag", 99)
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > maxBytes {
			maxBytes, largest = len(raw), st
		}
	}
	for _, r := range astralRuneItemsFixture(0) {
		s.items[r.ID] = r
	}
	for _, id := range []domain.ItemID{domain.RiftKeyItemID, domain.CubeStarMarrow, domain.CubePurifiedDust, domain.CubeImmortalEcho} {
		s.items[id] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Name: "完整材料计数"}
	}
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	for i := 0; i < 32; i++ {
		st := largest
		st.UID += int64(i)
		p.Player.Bag.Set(i, st)
	}
	s.sendEndgameSnapshot(p)
	responses := endgameMessages(s, sink)
	if len(responses) != 1 || len(responses[0].Items) != 32 {
		t.Fatal("expected one full snapshot", len(responses))
	}
	r := responses[0]
	// Include all three channel-winner DTOs too, using the largest possible
	// actual formation in each channel and their complete extra attributes.
	for channel := domain.AstralBattle; channel <= domain.AstralSupport; channel++ {
		var largestFormation domain.EndgameFormationView
		largestSize := 0
		for _, f := range domain.AstralRecipes(catalogfixture.EndgameCatalog()) {
			if f.Channel != channel {
				continue
			}
			v := domain.EndgameFormationView{ID: f.ID, Name: f.Name, Channel: f.Channel.String(),
				MinLevel: f.MinLevel, Professions: int32(f.Professions), Active: true,
				Description: f.Description(), Stats: endgameStats(f.Affixes)}
			for _, id := range f.Runes {
				v.Runes = append(v.Runes, int32(id))
			}
			for _, slot := range f.Slots {
				v.Slots = append(v.Slots, int32(slot))
			}
			raw, _ := json.Marshal(v)
			if len(raw) > largestSize {
				largestSize, largestFormation = len(raw), v
			}
		}
		r.Formations = append(r.Formations, largestFormation)
	}
	for i := range r.Materials {
		r.Materials[i].Count = 2147483647
	}
	wire := protocol.EncodeEndgame(r)
	if len(wire) == 0 {
		raw, _ := json.Marshal(r)
		t.Fatalf("full 32-item extended snapshot exceeds %d-byte JSON limit: %d", domain.EndgameMaxJSONBytes, len(raw))
	}
	var decoded domain.EndgameResponse
	if err := json.Unmarshal(wire[6:], &decoded); err != nil {
		t.Fatal(err)
	}
	for i, item := range decoded.Items {
		if item.UID != strconv.FormatInt(largest.UID+int64(i), 10) || len(item.Stats) != len(r.Items[i].Stats) || len(item.Sockets) != 5 {
			t.Fatal("snapshot truncated affixes/sockets or rounded UID", i)
		}
	}
	t.Logf("worst full32 snapshot: %d wire bytes; %d extra lines per unique; %d formations; %d material counts", len(wire), len(r.Items[0].Stats), len(r.Formations), len(r.Materials))
	// Optional integration export uses the exact production response above.
	// Normal tests never rewrite committed fixtures or generated catalogs.
	if out := os.Getenv("ENDGAME_WORST_SNAPSHOT_OUT"); out != "" {
		for suffix, data := range map[string][]byte{".wire.hex": []byte(hex.EncodeToString(wire) + "\n"), ".json": wire[6:]} {
			if err := os.WriteFile(filepath.Join(out, "worst-full32-extra-affixes"+suffix), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

package data

import (
	"context"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestBalanceActualPostgresLoader(t *testing.T) {
	path := os.Getenv("FANTASY_BALANCE_PG_DATA")
	if path == "" {
		t.Skip("FANTASY_BALANCE_PG_DATA not set")
	}
	bin := os.Getenv("PG_BIN")
	if bin == "" {
		t.Fatal("PG_BIN required")
	}
	database := os.Getenv("FANTASY_BALANCE_PG_DATABASE")
	if database == "" {
		database = "postgres"
	}
	q := &v5SingleUserQuerier{bin: bin, path: path, database: database}
	items, err := LoadItems(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	loot, err := LoadLootRules(context.Background(), q, items)
	if err != nil {
		t.Fatal(err)
	}
	rift, err := LoadRiftRules(context.Background(), q, loot, items)
	if err != nil {
		t.Fatal(err)
	}
	var expectedLoot domain.LootRuleSet
	var expectedRift domain.RiftRules
	if err = testfixture.LoadLootRules(&expectedLoot); err != nil {
		t.Fatal(err)
	}
	if err = testfixture.LoadRiftRules(&expectedRift); err != nil {
		t.Fatal(err)
	}
	expectedRift.Loot = &expectedLoot
	legacy, ok := rift.Version(1)
	if !ok || !reflect.DeepEqual(legacy, &expectedRift) {
		t.Fatal("legacy rule snapshot changed")
	}
	if rift.Active().Version == 2 {
		expectedRift.Version = 2
		expectedRift.FloorPopulationMultiplier = 2
	}
	if !reflect.DeepEqual(loot.Active(), &expectedLoot) || !reflect.DeepEqual(rift.Active(), &expectedRift) {
		t.Fatal("actual SQL configuration differs from seed")
	}
	if want := []domain.EndgameRiftEntranceView{{MapID: 7, NPCID: 83, NPC: "八仙使者", Script: "Nbaxian"}}; !reflect.DeepEqual(rift.Entrances(), want) {
		t.Fatalf("actual native rift entrance mismatch: %+v", rift.Entrances())
	}
	npcs, _, err := LoadNPCs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, npc := range npcs[7] {
		if npc.NPCID == 83 {
			found = npc.Name == "八仙使者" && npc.Sprite == 630 && npc.Script == "Nbaxian" && npc.Pos.X == 9917 && npc.Pos.Y == 4734
		}
	}
	if !found {
		t.Fatal("configured entrance did not survive actual native NPC loader")
	}
	// Compare original frozen 885-ID oracle to authoritative loaded ov_arm flags.
	raw, err := os.ReadFile("../domain/native_equipment_drop_bases.json")
	if err != nil {
		t.Fatal(err)
	}
	var ids []domain.ItemID
	if err = json.Unmarshal(raw, &ids); err != nil {
		t.Fatal(err)
	}
	want := map[domain.ItemID]bool{}
	for _, id := range ids {
		want[id] = true
	}
	count := 0
	for id, d := range items {
		got := domain.NativeEquipmentDropEligible(d)
		if got != want[id] {
			t.Fatalf("native drop pool changed %d got%v want%v", id, got, want[id])
		}
		if got {
			count++
		}
	}
	if count != 885 {
		t.Fatal("ordinary scope changed", count)
	}
	t.Logf("actual LoadItems→LootRules/RiftRules: %d items, 28 runes, 4 socket grades, 3 abilities, immutable version1, exact885 native pool", len(items))
}

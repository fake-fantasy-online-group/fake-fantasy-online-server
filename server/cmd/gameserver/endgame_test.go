package main
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"reflect"
	"testing"
)

func endgameWorldFixture() worldData {
	w := worldData{endgameCatalog: catalogfixture.EndgameCatalog(), items: make(data.ItemDefs)}
	var source struct {
		Bases []struct {
			BaseID      domain.ItemID    `json:"base_id"`
			Intrinsic   map[string]int32 `json:"intrinsic"`
			Name        string           `json:"base_name"`
			Level       int32            `json:"wearer_level"`
			Tier        int32            `json:"resource_tier"`
			Slot        int32            `json:"slot"`
			Professions []domain.Race    `json:"professions"`
			Sex         uint8            `json:"sex"`
			Beginner    bool             `json:"beginner_allowed"`
		}
	}
	raw, err := os.ReadFile("../../../docs/daoist-endgame/balance/native_equipment_budget_references.json")
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(raw, &source); err != nil {
		panic(err)
	}
	for _, u := range source.Bases {
		intrinsic := u.Intrinsic
		var mask uint8
		for _, race := range u.Professions {
			mask |= 1 << race
		}
		w.items[u.BaseID] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: u.BaseID, Name: u.Name, Icon: 7, InventoryTabKnown: true, Equip: &domain.EquipDef{Bonus: domain.Stats{MinAtk: intrinsic["min_atk"], MaxAtk: intrinsic["max_atk"], MAtk: intrinsic["matk"], Def: intrinsic["def"], MDef: intrinsic["mdef"]}, Slot: u.Slot, LevelReq: u.Level, ResourceLevel: u.Tier, Need: domain.Requirement{Professions: mask, BeginnerAllowed: u.Beginner, Sex: u.Sex}}}
	}
	for _, d := range endgameItemsFixture() {
		w.items[d.ID] = d
	}
	domain.AttachEndgameCatalog(w.items, w.endgameCatalog)
	return w
}
func TestRegisterEndgameItemsCompleteNoAssetInvention(t *testing.T) {
	w := endgameWorldFixture()
	base := len(w.items)
	if err := registerEndgameItems(&w); err != nil {
		t.Fatal(err)
	}
	if len(w.items) != base || len(w.itemIcons) != base || !w.itemIconsKnown {
		t.Fatal("missing runtime definitions")
	}
	seen := map[domain.ItemID]bool{}
	for _, m := range w.itemIcons {
		d := w.items[m.Item]
		if seen[m.Item] || d.Icon != m.Icon || !d.InventoryTabKnown {
			t.Fatal("bad existing-art mapping", m, d)
		}
		if d.Equip == nil && m.Icon != endgameItemIconFixture(m.Item) {
			t.Fatal("bad added item mapping", m)
		}
		seen[m.Item] = true
	}
	if !w.items[domain.RiftKeyItemID].Stackable || w.items[domain.RiftKeyItemID].CanTrade {
		t.Fatal("key definition")
	}
	for _, r := range domain.AstralRunes(catalogfixture.EndgameCatalog()) {
		if d := w.items[r.Item]; d.InstanceKind != domain.ItemInstanceSocketCard || !d.CardDefaults.Initialized {
			t.Fatal("rune lost real instance semantics")
		}
	}
}
func TestRegisterEndgameRejectsMalformedPersistedDefinition(t *testing.T) {
	w := endgameWorldFixture()
	w.items[domain.RiftKeyItemID] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.RiftKeyItemID, Name: "existing"}
	if registerEndgameItems(&w) == nil {
		t.Fatal("overwrote existing ID")
	}
	if w.items[domain.RiftKeyItemID].Name != "existing" {
		t.Fatal("existing definition overwritten")
	}
}

func TestEndgameIconCatalogDistinctExistingSprites(t *testing.T) {
	// The resource audit checks pixels separately. This guards registration coverage
	// and prevents a regression to the first arbitrary icon in the item table.
	seen := map[int32]domain.ItemID{}
	ids := []domain.ItemID{domain.RiftKeyItemID, domain.CubeStarMarrow, domain.CubePurifiedDust, domain.CubeImmortalEcho}
	for _, r := range domain.AstralRunes(catalogfixture.EndgameCatalog()) {
		ids = append(ids, r.Item)
	}
	for _, id := range ids {
		icon := endgameItemIconFixture(id)
		if icon <= 0 {
			t.Fatalf("missing icon for %d", id)
		}
		if old, ok := seen[icon]; ok {
			t.Fatalf("shared icon %d for %d and %d", icon, old, id)
		}
		seen[icon] = id
	}
	if len(ids) != 32 || len(seen) != 32 {
		t.Fatal("incomplete icon catalog")
	}
	if endgameItemIconFixture(0) != 0 || endgameItemIconFixture(domain.FirstAstralRune-1) != 0 || endgameItemIconFixture(domain.FirstAstralRune+28) != 0 {
		t.Fatal("icon lookup leaked outside catalog")
	}
}

func TestRegisterEndgameIconsDoNotDependOnIncidentalItemOrder(t *testing.T) {
	w := endgameWorldFixture()
	w.items[1] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 1, Icon: 999999}
	if err := registerEndgameItems(&w); err != nil {
		t.Fatal(err)
	}
	if w.items[domain.FirstAstralRune].Icon != int32(26018) {
		t.Fatal("used unrelated first icon")
	}
}

func TestRegisterEndgameRejectsMissingMigrationWithoutCreatingItems(t *testing.T) {
	for _, id := range domain.EndgameBaseItemIDs() {
		w := endgameWorldFixture()
		delete(w.items, id)
		before := len(w.items)
		if err := registerEndgameItems(&w); err == nil {
			t.Fatalf("missing item %d silently synthesized", id)
		}
		if len(w.items) != before || len(w.itemIcons) != 0 {
			t.Fatal("failed registration mutated world")
		}
	}
}
func TestRegisterEndgameUsesPersistedMetadataAndIsIdempotent(t *testing.T) {
	w := endgameWorldFixture()
	d := w.items[domain.RiftKeyItemID]
	d.Name, d.Description, d.Icon, d.Price = "数据库符钥", "数据库说明", 12345, 42
	w.items[d.ID] = d
	for i := 0; i < 2; i++ {
		if err := registerEndgameItems(&w); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(w.items[d.ID], d) {
		t.Fatal("registration rewrote persisted metadata")
	}
	seen := map[domain.ItemID]bool{}
	for _, p := range w.itemIcons {
		if seen[p.Item] {
			t.Fatal("duplicate icon")
		}
		seen[p.Item] = true
		if p.Item == d.ID && p.Icon != d.Icon {
			t.Fatal("code icon overrode database")
		}
	}
}

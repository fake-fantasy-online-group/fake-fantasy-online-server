package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// CubeSnapshotMaterialIDs is shared by startup validation and the actual snapshot
// writer. Materials follow database recipes; catalogue identities are not formulas.
func CubeSnapshotMaterialIDs(rules *CubeRuleTable, catalog *EndgameCatalog, wings ...*WingProgressionRules) []ItemID {
	ids := map[ItemID]bool{RiftKeyItemID: true}
	for _, recipe := range rules.Recipes() {
		for _, m := range recipe.Materials {
			ids[m.ItemID] = true
		}
	}
	// Wing recipes live in their own immutable table, but use the same native
	// cube input contract. Omitting them makes old and new clients see zero
	// owned feathers and reject the drag before a quote reaches the server.
	for _, wingRules := range wings {
		for _, recipe := range wingRules.Recipes() {
			for _, material := range recipe.Materials {
				ids[material.ItemID] = true
			}
		}
	}
	if catalog != nil {
		for _, r := range catalog.Runes() {
			ids[r.Item] = true
		}
	}
	sorted := make([]ItemID, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted
}
func ValidateCubeSnapshotMaterials(items map[ItemID]ItemDef, catalog *EndgameCatalog, rules *CubeRuleTable, wings ...*WingProgressionRules) error {
	if catalog == nil || rules == nil {
		return fmt.Errorf("missing cube snapshot configuration")
	}
	ids := CubeSnapshotMaterialIDs(rules, catalog, wings...)
	if len(ids) > 64 {
		return fmt.Errorf("cube material snapshot exceeds client 64-row limit: %d", len(ids))
	}
	rows := make([]EndgameMaterialView, 0, len(ids))
	for _, id := range ids {
		def, ok := items[id]
		if !ok || !catalogTextValid(def.Name) {
			return fmt.Errorf("invalid cube material presentation %d", id)
		}
		rows = append(rows, EndgameMaterialView{ItemID: int32(id), Name: def.Name, Count: math.MaxInt32})
	}
	wire, err := json.Marshal(rows)
	// The legacy header/item/formation validation remains in force. Material
	// extension stays within this bounded allocation before actors are admitted.
	if err != nil || len(wire) > 16*1024 {
		return fmt.Errorf("cube material snapshot exceeds 16KiB header allocation")
	}
	return nil
}

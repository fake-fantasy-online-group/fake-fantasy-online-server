package domain

import "fmt"

// EndgameBaseItemIDs are stable identifiers, not alternate base definitions.
// Existing inventories and card snapshots retain these same IDs after migration.
func EndgameBaseItemIDs() []ItemID {
	out := make([]ItemID, 0, 32)
	for id := FirstAstralRune; id <= LastAstralRune; id++ {
		out = append(out, id)
	}
	return append(out, RiftKeyItemID, CubeStarMarrow, CubePurifiedDust, CubeImmortalEcho)
}

// ValidateEndgameItemDefinitions fails closed on absent/incompatible database
// configuration. Names, artwork, prices and permissions remain database-owned.
func ValidateEndgameItemDefinitions(items map[ItemID]ItemDef) error {
	for _, id := range EndgameBaseItemIDs() {
		d, ok := items[id]
		if !ok || d.ID != id {
			return fmt.Errorf("endgame item %d missing from database configuration; apply migration 000000_init-59.sql", id)
		}
		if d.Name == "" || d.Description == "" || d.Icon <= 0 || d.Equip != nil || !d.InventoryTabKnown || d.InventoryTab != 1 {
			return fmt.Errorf("endgame item %d has incomplete database base definition", id)
		}
		if id >= FirstAstralRune && id <= LastAstralRune {
			if d.Stackable || d.InstanceKind != ItemInstanceSocketCard || d.Level <= 0 || d.UseLevel != d.Level || !d.CardDefaults.Initialized || d.CardDefaults.Count == 0 {
				return fmt.Errorf("endgame rune %d has incompatible database card identity or level", id)
			}
		} else if !d.Stackable || d.InstanceKind != ItemInstanceNone {
			return fmt.Errorf("endgame material/key %d has incompatible database stack identity", id)
		}
	}
	return nil
}

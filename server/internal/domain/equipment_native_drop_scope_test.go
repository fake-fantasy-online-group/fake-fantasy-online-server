package domain

import (
	"encoding/json"
	"os"
)

// Frozen pre-migration baseline used only by parity tests.
var nativeEquipmentDropBases = func() map[ItemID]bool {
	raw, err := os.ReadFile("native_equipment_drop_bases.json")
	if err != nil {
		panic(err)
	}
	var ids []ItemID
	if err = json.Unmarshal(raw, &ids); err != nil {
		panic(err)
	}
	result := make(map[ItemID]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || result[id] {
			panic("invalid ordinary equipment drop identity")
		}
		result[id] = true
	}
	return result
}()

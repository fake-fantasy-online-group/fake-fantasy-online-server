package domain

import (
	"embed"
	"encoding/json"
)

// Generated independently from ov_arm's actual native drop classifications.
// This ordinary pool is intentionally not the259-entry unique promotion list.
//
//go:embed native_equipment_drop_bases.json
var nativeEquipmentDropSource embed.FS
var nativeEquipmentDropBases = func() map[ItemID]bool {
	raw, err := nativeEquipmentDropSource.ReadFile("native_equipment_drop_bases.json")
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

func NativeEquipmentDropEligible(def ItemDef) bool {
	return def.Equip != nil && !def.Equip.NoTypeDrop && nativeEquipmentDropBases[def.ID]
}

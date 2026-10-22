package domain

// NativeEquipmentDropEligible consumes the original SQL classification loaded
// with the template, keeping the ordinary pool separate from unique promotion.
func NativeEquipmentDropEligible(def ItemDef) bool {
	return !def.HardDungeonOnly && def.Equip != nil && !def.Equip.NoTypeDrop && def.Equip.NativeDropEligible
}

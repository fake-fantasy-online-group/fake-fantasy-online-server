package domain

import "fmt"

const SpiritSealedPrice int64 = 10_000_000 // 10 gold, in copper.
const RiftSpiritRewardVersion int32 = 19

func SpiritShopID(kind uint8) int32 {
	if _, ok := SpiritDefinitionForKind(kind); !ok {
		return 0
	}
	return 6200 + int32(kind)
}
func SpiritDefinitionForShop(shop int32) (SpiritDefinition, bool) {
	for _, def := range SpiritDefinitions() {
		if SpiritShopID(def.Kind) == shop {
			return def, true
		}
	}
	return SpiritDefinition{}, false
}

// Integer rewards stay within [0.9*tier/2,1.1*tier/2]. When the
// interval has no integer (very low odd tiers), round tier/2 up, minimum one.
func SpiritRiftQuantityBounds(tier int32) (int32, int32) {
	if tier < 1 || tier > 150 {
		return 0, 0
	}
	lo, hi := (tier*9+19)/20, tier*11/20
	if lo < 1 {
		lo = 1
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi
}
func (r *RiftRules) validateSpiritRewards(items map[ItemID]ItemDef) error {
	if r.Version < RiftSpiritRewardVersion {
		return nil
	}
	for _, spirit := range SpiritDefinitions() {
		def, ok := items[spirit.EssenceID]
		if !ok || !def.Stackable || def.InstanceKind != ItemInstanceNone {
			return fmt.Errorf("invalid rift spirit essence %d", spirit.EssenceID)
		}
	}
	return nil
}

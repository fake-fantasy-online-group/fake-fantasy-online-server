package domain

import "fmt"

// RiftEquipmentPool is published database configuration, frozen with each run.
// MonsterLevel identifies the native elite drop tier, not equipment item level.
type RiftEquipmentPool struct {
	MonsterLevel   int32
	SourceMonsters []MonsterID
	Items          []ItemID
}

func (r *RiftRules) validateEquipmentPools(items map[ItemID]ItemDef) error {
	if r.EquipmentPools == nil {
		if r.Version >= 7 {
			return fmt.Errorf("missing rift equipment pools")
		}
		return nil
	} // historical snapshots
	if len(r.EquipmentPools) == 0 {
		return fmt.Errorf("empty rift equipment pools")
	}
	last := int32(0)
	for _, p := range r.EquipmentPools {
		if p.MonsterLevel <= last || len(p.SourceMonsters) == 0 || len(p.Items) == 0 {
			return fmt.Errorf("invalid rift equipment tier")
		}
		monsters := map[MonsterID]bool{}
		for _, id := range p.SourceMonsters {
			if id <= 0 || monsters[id] {
				return fmt.Errorf("invalid rift source monster %d", id)
			}
			monsters[id] = true
		}
		last = p.MonsterLevel
		seen := map[ItemID]bool{}
		for _, id := range p.Items {
			if id <= 0 || seen[id] {
				return fmt.Errorf("invalid duplicate rift equipment %d", id)
			}
			seen[id] = true
			if items != nil {
				d, ok := items[id]
				if !ok || d.HardDungeonOnly || d.Equip == nil || d.Equip.LevelReq > 60 || d.Equip.AscensionOnly {
					return fmt.Errorf("invalid rift equipment %d", id)
				}
			}
		}
	}
	return nil
}

// EquipmentPoolForLevel selects the nearest native monster tier containing at
// least one legal item. Ties favor the lower tier. It never scans the item catalog
// or falls back to unrelated equipment when an explicit pool is configured.
func (r *RiftRules) EquipmentPoolForLevel(level int32, eligible func(ItemID) bool) []ItemID {
	var result []ItemID
	best := int64(1 << 62)
	for _, p := range r.EquipmentPools {
		distance := int64(p.MonsterLevel) - int64(level)
		if distance < 0 {
			distance = -distance
		}
		if distance >= best {
			continue
		}
		var candidates []ItemID
		for _, id := range p.Items {
			if eligible(id) {
				candidates = append(candidates, id)
			}
		}
		if len(candidates) > 0 {
			best = distance
			result = candidates
		}
	}
	return result
}

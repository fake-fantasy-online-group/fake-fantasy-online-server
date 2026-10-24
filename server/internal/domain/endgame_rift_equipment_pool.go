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
	if err := r.validateEquipmentSlotFallbacks(items); err != nil {
		return err
	}
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

// Only ordinary native random bases may repair missing slots. In particular,
// explicitly named fixed equipment and ascension/costume bases are not fallback.
func riftSlotFallbackBase(def ItemDef) bool {
	e := def.Equip
	return NativeEquipmentDropEligible(def) && !e.AscensionOnly &&
		(EquipSlot(e.Slot) == SlotFace || EquipSlot(e.Slot) == SlotBag) &&
		e.LevelReq >= 0 && e.LevelReq <= 60 && e.Need.Level >= 0 && e.Need.Level <= 60 &&
		e.ResourceLevel >= 0 && len(e.Affixes) == 0 && !e.UnknownInnateProperties
}

func (r *RiftRules) validateEquipmentSlotFallbacks(items map[ItemID]ItemDef) error {
	if r.Version < 11 {
		if r.EquipmentSlotFallbacks != nil {
			return fmt.Errorf("historical rift cannot add equipment slot fallback")
		}
		return nil
	}
	if len(r.EquipmentSlotFallbacks) == 0 {
		return fmt.Errorf("missing rift equipment slot fallback")
	}
	seen := map[ItemID]bool{}
	slots := map[int32]bool{}
	for _, id := range r.EquipmentSlotFallbacks {
		if id <= 0 || seen[id] {
			return fmt.Errorf("invalid duplicate rift slot fallback %d", id)
		}
		seen[id] = true
		if items != nil {
			def, ok := items[id]
			if !ok || !riftSlotFallbackBase(def) {
				return fmt.Errorf("invalid rift slot fallback %d", id)
			}
			slots[def.Equip.Slot] = true
		}
	}
	if items != nil && len(slots) != 2 {
		return fmt.Errorf("rift slot fallback requires face and backpack bases")
	}
	return nil
}

// FillMissingEquipmentSlots preserves the selected native pool and its order.
// It adds all wearer-legal variants at the highest resource tier, independently
// for each absent slot. Level is the recipient's frozen admission level, not the
// party's encounter level, and is never compared to equipment resource tiers.
// Only IDs frozen in these rules are consulted; old versions never opt in.
func (r *RiftRules) FillMissingEquipmentSlots(pool []ItemID, level int32, items map[ItemID]ItemDef, eligible func(ItemID) bool) []ItemID {
	if r.Version < 11 || len(r.EquipmentSlotFallbacks) == 0 {
		return pool
	}
	occupied := map[EquipSlot]bool{}
	for _, id := range pool {
		if def := items[id]; def.Equip != nil {
			occupied[EquipSlot(def.Equip.Slot)] = true
		}
	}
	for _, slot := range []EquipSlot{SlotFace, SlotBag} {
		if occupied[slot] {
			continue
		}
		best := int32(-1)
		var additions []ItemID
		for _, id := range r.EquipmentSlotFallbacks {
			def := items[id]
			if !riftSlotFallbackBase(def) || EquipSlot(def.Equip.Slot) != slot ||
				def.Equip.LevelReq > level || def.Equip.Need.Level > level || !eligible(id) {
				continue
			}
			tier := def.Equip.ResourceLevel
			if tier > best {
				best, additions = tier, nil
			}
			if tier == best {
				additions = append(additions, id)
			}
		}
		if len(additions) > 0 {
			// Do not mutate the caller's backing slice or a frozen rule pool.
			pool = append(append([]ItemID(nil), pool...), additions...)
		}
	}
	return pool
}

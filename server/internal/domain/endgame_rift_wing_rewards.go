package domain

import "fmt"

// A bounded package may span native inventory stacks without truncation.
const RiftWingMaxRewardCount int32 = 99999

// RiftWingRewardRules references the existing item catalog, never a new currency.
// A milestone is a fixed personal package, independent of the player's wing.
// RepeatBands select exactly the highest reached band, not all lower bands.
// RareFeather is a separate, approved one-roll slot, never a white denomination.
type RiftRareFeatherRule struct {
	Item            ItemID
	Count, ChanceBP int32
}

func (r RiftRareFeatherRule) Valid() bool { return r.Item == 11682 && r.Count == 1 && r.ChanceBP == 50 }
func (r RiftRareFeatherRule) Roll(intn func(int) int) (CubeMaterialDef, bool) {
	if !r.Valid() || intn == nil {
		return CubeMaterialDef{}, false
	}
	roll := intn(10000)
	if roll < 0 || roll >= int(r.ChanceBP) {
		return CubeMaterialDef{}, false
	}
	return CubeMaterialDef{ItemID: r.Item, Count: r.Count}, true
}

type RiftWingRewardRules struct {
	RareFeather *RiftRareFeatherRule `json:",omitempty"`
	Milestones  []RiftWingMilestone
	RepeatBands []RiftWingRepeatBand
}

type RiftWingMilestone struct {
	Tier      int32
	Item      ItemID
	Count     int32
	Materials []CubeMaterialDef `json:",omitempty"`
}

type RiftWingRepeatBand struct {
	MinTier   int32
	Item      ItemID
	Count     int32
	Materials []CubeMaterialDef `json:",omitempty"`
}

func (r *RiftRules) validateWingRewards(items map[ItemID]ItemDef) error {
	if r.WingRewards == nil {
		if r.Version >= 14 {
			return fmt.Errorf("missing rift wing reward policy")
		}
		return nil
	}
	w := r.WingRewards
	if rare := w.RareFeather; rare != nil {
		if !rare.Valid() {
			return fmt.Errorf("unapproved rare feather policy")
		}
		if items != nil {
			d, ok := items[rare.Item]
			if !ok || !d.Stackable || d.HardDungeonOnly || d.Equip != nil || d.InstanceKind != ItemInstanceNone {
				return fmt.Errorf("invalid rare feather template")
			}
		}
	}
	if r.Version < 14 || len(w.Milestones) < 1 || len(w.Milestones) > 32 || len(w.RepeatBands) < 1 || len(w.RepeatBands) > 32 || w.RepeatBands[0].MinTier != 1 {
		return fmt.Errorf("invalid versioned rift wing reward policy")
	}
	validate := func(tier, last int32, item ItemID, count int32, materials []CubeMaterialDef) error {
		if tier <= last || tier > r.MaxTier {
			return fmt.Errorf("invalid rift wing reward tier")
		}
		if len(materials) > 0 && (item != 0 || count != 0) {
			return fmt.Errorf("ambiguous wing package")
		}
		ms := wingRewardMaterials(item, count, materials)
		if len(ms) == 0 || len(ms) > 6 {
			return fmt.Errorf("invalid wing package size")
		}
		seen := map[ItemID]bool{}
		for _, m := range ms {
			if (m.ItemID != 25027 && m.ItemID != 25028 && m.ItemID != 25029) || m.Count < 1 || m.Count > RiftWingMaxRewardCount || seen[m.ItemID] {
				return fmt.Errorf("invalid rift wing material/count")
			}
			seen[m.ItemID] = true
			if items != nil {
				d, ok := items[m.ItemID]
				if !ok || !d.Stackable || d.HardDungeonOnly || d.Equip != nil || d.InstanceKind != ItemInstanceNone {
					return fmt.Errorf("invalid rift wing material %d", m.ItemID)
				}
			}
		}
		return nil
	}
	last := int32(0)
	for _, m := range w.Milestones {
		if err := validate(m.Tier, last, m.Item, m.Count, m.Materials); err != nil {
			return err
		}
		last = m.Tier
	}
	last = 0
	for _, b := range w.RepeatBands {
		if err := validate(b.MinTier, last, b.Item, b.Count, b.Materials); err != nil {
			return err
		}
		last = b.MinTier
	}
	return nil
}

func (r *RiftRules) WingRepeatReward(tier int32) (RiftWingRepeatBand, bool) {
	var selected RiftWingRepeatBand
	if r != nil && r.WingRewards != nil {
		for _, b := range r.WingRewards.RepeatBands {
			if b.MinTier > tier {
				break
			}
			selected = b
		}
	}
	return selected, len(selected.Rewards()) > 0
}

// MissingWingMilestones is evaluated while preparing a character-state CAS,
// not during shared run generation. The caller must atomically persist both
// the returned entitlements and the personal milestone ledger.
func (r *RiftRules) MissingWingMilestones(tier int32, claimed []int32) []RiftWingMilestone {
	if r == nil || r.WingRewards == nil {
		return nil
	}
	seen := make(map[int32]bool, len(claimed))
	for _, tier := range claimed {
		seen[tier] = true
	}
	var missing []RiftWingMilestone
	for _, m := range r.WingRewards.Milestones {
		if m.Tier > tier {
			break
		}
		if !seen[m.Tier] {
			missing = append(missing, m)
		}
	}
	return missing
}

func wingRewardMaterials(item ItemID, count int32, materials []CubeMaterialDef) []CubeMaterialDef {
	if len(materials) > 0 {
		return append([]CubeMaterialDef(nil), materials...)
	}
	if item > 0 && count > 0 {
		return []CubeMaterialDef{{ItemID: item, Count: count}}
	}
	return nil
}
func (m RiftWingMilestone) Rewards() []CubeMaterialDef {
	return wingRewardMaterials(m.Item, m.Count, m.Materials)
}
func (b RiftWingRepeatBand) Rewards() []CubeMaterialDef {
	return wingRewardMaterials(b.Item, b.Count, b.Materials)
}

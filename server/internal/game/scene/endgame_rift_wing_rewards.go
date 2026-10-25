package scene

import (
	"fmt"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (s *Scene) riftWingStacks(item domain.ItemID, count int32) ([]domain.Stack, error) {
	def, ok := s.itemDef(item)
	if !ok || !def.Stackable || def.HardDungeonOnly || def.Equip != nil || def.InstanceKind != domain.ItemInstanceNone || count < 1 || count > domain.RiftWingMaxRewardCount {
		return nil, fmt.Errorf("invalid rift wing material %d", item)
	}
	var stacks []domain.Stack
	for count > 0 {
		n := min(count, domain.MaxStack)
		stack := domain.NewStack(def, n)
		stack.Bound = true
		stacks = append(stacks, stack)
		count -= n
	}
	return stacks, nil
}

// Reserve fixed first-clear packages into this run's recoverable grant together
// with the personal ledger. The existing character revision CAS and run revision
// transaction serialize two runs; a loser reloads, then plans only missing tiers.
// No inventory space or equipped-wing state is consulted at entitlement time.
func (s *Scene) resolveEndgameRiftWingMilestones(d *endgameRiftPersisted, char domain.CharID, profile *endgameRiftProfile) error {
	if d.Run.Rules.WingRewards == nil {
		return nil
	}
	grant := d.Grants[char]
	if grant.WingMilestonesResolved {
		return nil
	}
	member := d.Run.Members[char]
	if member == nil || !member.Eligible || (d.Run.Phase != domain.RiftCompleted && d.Run.Phase != domain.RiftClosed) {
		return fmt.Errorf("rift wing reward requires eligible completion")
	}
	missing := d.Run.Rules.MissingWingMilestones(d.Run.Tier, profile.WingMilestones)
	stacks := make([]domain.Stack, 0, len(missing))
	for _, m := range missing {
		packageStacks, err := s.riftWingPackage(m.Rewards())
		if err != nil {
			return err
		}
		stacks = append(stacks, packageStacks...)
	}
	grant.Items = append(grant.Items, stacks...)
	grant.WingMilestonesResolved = true
	d.Grants[char] = grant
	for _, m := range missing {
		profile.WingMilestones = append(profile.WingMilestones, m.Tier)
	}
	sort.Slice(profile.WingMilestones, func(i, j int) bool { return profile.WingMilestones[i] < profile.WingMilestones[j] })
	return nil
}

func (s *Scene) riftWingPackage(materials []domain.CubeMaterialDef) ([]domain.Stack, error) {
	var out []domain.Stack
	for _, m := range materials {
		stacks, err := s.riftWingStacks(m.ItemID, m.Count)
		if err != nil {
			return nil, err
		}
		out = append(out, stacks...)
	}
	return out, nil
}

// One purpose-separated deterministic roll per eligible character and completed
// run. Tier, MF, party ordering and other reward RNG cannot add rolls or alter it.
// The result becomes an ordinary frozen grant stack and uses existing recovery.
func (s *Scene) riftRareFeatherStacks(char domain.CharID) ([]domain.Stack, error) {
	r := s.rift.data.Run
	if r.Rules.WingRewards == nil || r.Rules.WingRewards.RareFeather == nil {
		return nil, nil
	}
	if r.Phase != domain.RiftCompleted && r.Phase != domain.RiftClosed {
		return nil, nil
	}
	member := r.Members[char]
	if member == nil || !member.Eligible {
		return nil, nil
	}
	rare := *r.Rules.WingRewards.RareFeather
	if !rare.Valid() {
		return nil, fmt.Errorf("unapproved rare feather rule")
	}
	// Validate the template even on a miss; broken configuration cannot erase a hit.
	if _, err := s.riftWingStacks(rare.Item, rare.Count); err != nil {
		return nil, err
	}
	rng := s.riftCumulativeRandom(char, 0, "rare-wing-feather")
	material, ok := rare.Roll(rng.Intn)
	if !ok {
		return nil, nil
	}
	return s.riftWingStacks(material.ItemID, material.Count)
}

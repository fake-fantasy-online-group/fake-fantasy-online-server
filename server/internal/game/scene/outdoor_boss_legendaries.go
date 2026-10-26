package scene

import (
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

const outdoorBossGuaranteedLegendaries = 2

// This is a kill-only addition to the original loot, not a replacement and not
// part of theft's single-item roll. The monster-life marker prevents a repeated
// death settlement from granting these two instances again.
func (s *Scene) rollOutdoorBossLegendaries(dead *entity.Entity, context domain.LootContextV5, startSlot int) int {
	if dead == nil || dead.Monster == nil || dead.Monster.Kind != domain.MonsterBoss || dead.Monster.Summoner != 0 ||
		s.id.Instance != 0 || s.dungeon != nil || s.rift != nil || s.trialRun != nil || s.trial.IsMap(s.id.MapID) || s.alloc == nil {
		return 0
	}
	rule := s.lootRules.Active()
	if rule == nil || rule.OutdoorEquipment == nil || dead.Monster.OutdoorBossLegendariesGranted {
		return 0
	}
	dead.Monster.OutdoorBossLegendariesGranted = true
	pool := s.outdoorBossLegendaryPool(dead, context.RecipientClass)
	if len(pool) == 0 {
		s.log.Warn("野外 BOSS 暗金保底没有合法底材", "monster", dead.Monster.TypeID, "level", dead.Level, "owner", context.Owner)
		return 0
	}
	// Prefer two different bases when possible, but one legal base can still
	// produce two independently identified instances. Duplicate drop rows do not
	// multiply a base's selection weight.
	remaining := append([]domain.ItemID(nil), pool...)
	var stacks [outdoorBossGuaranteedLegendaries]domain.Stack
	for ordinal := range stacks {
		selection := s.lootRandomV5(dead.ID, ordinal, 0x56360001)
		variables := s.lootRandomV5(dead.ID, ordinal, 0x56360002)
		index := selection.Intn(len(remaining))
		def := s.items[remaining[index]]
		st := domain.NewStack(def, 1)
		// Generates six legal affixes, one power and its variant exactly once. Power
		// level eligibility follows the source monster, not the equipment or owner.
		if !s.equipmentRoll.ApplyLegendaryForRecipientWithPolicyAtLevel(&st, def, dead.Level, context.RecipientClass, rule.LegendaryPowerPolicy, true, variables.Intn) {
			s.log.Error("野外 BOSS 暗金保底生成失败", "monster", dead.Monster.TypeID, "item", def.ID, "level", dead.Level)
			return 0 // Do not publish a partial pair or replace it with old Unique.
		}
		stacks[ordinal] = st
		if len(remaining) > 1 {
			remaining = append(remaining[:index], remaining[index+1:]...)
		}
	}
	for ordinal, st := range stacks {
		s.spawnGeneratedLoot(s.items[st.Item], st, s.monsterLootPosition(dead.Pos, startSlot+ordinal), context.Owner)
	}
	return len(stacks)
}

func (s *Scene) outdoorBossLegendaryPool(dead *entity.Entity, recipient ...domain.LegendaryRecipientClass) []domain.ItemID {
	if dead == nil || dead.Monster == nil {
		return nil
	}
	var recipientClass domain.LegendaryRecipientClass
	if len(recipient) > 0 {
		recipientClass = recipient[0]
	}
	var policy int32
	if rule := s.lootRules.Active(); rule != nil {
		policy = rule.LegendaryPowerPolicy
	}
	unique := map[domain.ItemID]bool{}
	add := func(id domain.ItemID) {
		def, ok := s.items[id]
		if ok && domain.NativeEquipmentDropEligible(def) && def.LegendaryEnabled &&
			s.equipmentRoll.LegendaryReady(def) && len(domain.LegendaryCandidatesForRecipientWithPolicyAtLevel(def, dead.Level, recipientClass, policy)) > 0 {
			unique[id] = true
		}
	}
	// Only the target's existing natural table and map-specific fixed equipment
	// pool are eligible. No catalog-wide or higher-level substitute is added.
	for _, entry := range s.lootTable[dead.Monster.TypeID] {
		if entry.DungeonOnly || entry.RatePct <= 0 || entry.RatePct > 100 {
			continue
		}
		if len(entry.Choices) > 0 {
			for _, id := range entry.Choices {
				add(id)
			}
		} else {
			add(entry.Item)
		}
	}
	if fixed, ok := s.bossFixedDrops[domain.BossFixedDropKey{MapID: s.id.MapID, Monster: dead.Monster.TypeID}]; ok && fixed.Minimum > 0 {
		for _, choice := range fixed.Pool {
			if choice.Weight > 0 || choice.RatePct > 0 {
				add(choice.Item)
			}
		}
	}
	pool := make([]domain.ItemID, 0, len(unique))
	for id := range unique {
		pool = append(pool, id)
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i] < pool[j] })
	return pool
}

package scene

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/rand"
	"sort"
)

type endgameRiftIdentity struct {
	Race     domain.Race
	Gender   uint8
	Employed bool
	// Latest authoritative personal MF, retained for eligible disconnected members.
	MagicFind        int64                          `json:",omitempty"`
	RewardLevel      int32                          `json:",omitempty"`
	RewardClass      domain.LegendaryRecipientClass `json:",omitempty"`
	RewardClassKnown bool                           `json:",omitempty"`
}

// Admission profiles are usable for legacy pending rewards, but a missing
// profile must not invent warrior merely because Race's zero value is warrior.
func (identity endgameRiftIdentity) legendaryRecipientClass() domain.LegendaryRecipientClass {
	if identity.RewardClassKnown {
		return identity.RewardClass
	}
	return domain.LegendaryClassForCharacter(&domain.Character{Race: identity.Race, Employed: identity.Employed, EmploymentKnown: true})
}

// Completion constructs exact instance identities once; retries never reroll.
type endgameRiftRewardContext struct {
	Level               int32
	MagicFind           int64
	RecipientClass      domain.LegendaryRecipientClass
	RecipientClassKnown bool
}

func (s *Scene) freezeEndgameRiftRewardContexts() map[domain.CharID]endgameRiftRewardContext {
	contexts := map[domain.CharID]endgameRiftRewardContext{}
	for char, member := range s.rift.data.Run.Members {
		profile := s.rift.data.Profiles[char]
		context := endgameRiftRewardContext{Level: profile.RewardLevel, MagicFind: profile.MagicFind, RecipientClass: profile.legendaryRecipientClass(), RecipientClassKnown: true}
		if context.Level == 0 {
			context.Level = member.Level
		}
		// A commit callback can replace the aggregate before the next tick drains
		// observations. A guardian event in that interval still sees the last value.
		for _, observation := range s.rift.delayedDeaths {
			if observation.CharID == char && observation.ObservedContext != nil {
				context = *observation.ObservedContext
			}
		}
		if owner := s.endgameRewardPlayer(char); owner != nil {
			context.Level, context.MagicFind = owner.Player.Char.Level, s.magicFindV5(owner)
			context.RecipientClass, context.RecipientClassKnown = domain.LegendaryClassForCharacter(owner.Player.Char), true
		}
		if s.rift.data.RewardClasses != nil {
			context.RecipientClass, context.RecipientClassKnown = s.rift.data.RewardClasses[char], true
		}
		contexts[char] = context
	}
	return contexts
}

func (s *Scene) generateEndgameRiftGrants() {
	s.generateEndgameRiftGrantsWithContexts(s.freezeEndgameRiftRewardContexts())
}

func (s *Scene) generateEndgameRiftGrantsWithContexts(contexts map[domain.CharID]endgameRiftRewardContext) {
	d := &s.rift.data
	if d.GrantsReady {
		return
	}
	r := d.Run
	// Capture even if content/provenance prevents this attempt. The exact accepted
	// death classes survive retries, reconnects and persistence, including zero.
	if d.RewardClasses == nil {
		d.RewardClasses = make(map[domain.CharID]domain.LegendaryRecipientClass, len(r.Members))
		for char := range r.Members {
			recipient := d.Profiles[char].legendaryRecipientClass()
			if context := contexts[char]; context.RecipientClassKnown {
				recipient = context.RecipientClass
			}
			d.RewardClasses[char] = recipient
		}
	}
	sourceLevel := s.riftRewardSourceLevel()
	if sourceLevel > 0 {
		d.RewardSourceLevel = sourceLevel
	}
	plan := domain.PlanRiftRewards(r.Tier, r.Timely, r.Rules)
	if sourceLevel <= 0 && (plan.Equipment > 0 || plan.NormalBossEquipment > 0 || r.Rules.CumulativeLegendaryPerTier > 0) {
		// An old paid run has the same right to a real guaranteed promotion.
		// Unknown provenance must not turn its entitlement into ordinary gear.
		s.log.Warn("洞天奖励缺少可核实怪物等级，保留待生成状态", "run", r.RunID)
		return
	}
	// Encounter base level uses the highest frozen admission level, as native rift spawns do.
	poolLevel := int32(0)
	for _, member := range r.Members {
		if member.Level > poolLevel {
			poolLevel = member.Level
		}
	}
	// Validate the entire award before generating any instances. A broken content
	// configuration must not silently turn a guaranteed Legendary into no reward.
	legendaryPools := map[domain.CharID][]domain.ItemID{}
	if r.Rules.CumulativeLegendaryPerTier > 0 {
		for char, member := range r.Members {
			if !member.Eligible {
				continue
			}
			pool := s.riftCumulativeLegendaryPool(r.Rules, poolLevel, member.Level, sourceLevel, d.Profiles[char], d.RewardClasses[char])
			if len(pool) == 0 {
				s.log.Warn("洞天暗金奖励缺少合法底材，保留待生成状态", "run", r.RunID, "character", char)
				return
			}
			legendaryPools[char] = pool
		}
	}
	grants := make(map[domain.CharID]endgameRiftGrant, len(r.Members))
	chars := make([]domain.CharID, 0, len(r.Members))
	for char := range r.Members {
		chars = append(chars, char)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	for _, char := range chars {
		m := r.Members[char]
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-reward/%d/v%d", r.RunID, char, d.Version)))
		rng := s.rng
		if d.Version >= 2 {
			rng = rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(hash[:8]))))
		}
		if !m.Eligible {
			continue
		}
		grant := endgameRiftGrant{}
		rareStacks, err := s.riftRareFeatherStacks(char)
		if err != nil {
			s.log.Warn("洞天稀羽配置无效，保留待生成状态", "run", r.RunID, "error", err)
			return
		}
		grant.Items = append(grant.Items, rareStacks...)
		if reward, ok := r.Rules.WingRepeatReward(r.Tier); ok {
			stacks, err := s.riftWingPackage(reward.Rewards())
			if err != nil {
				s.log.Warn("洞天羽毛配置无效，保留待生成状态", "run", r.RunID, "error", err)
				return
			}
			grant.Items = append(grant.Items, stacks...)
		}
		if r.Rules.CumulativeLegendaryPerTier > 0 {
			count := s.riftCumulativeRandom(char, -1, "count")
			for i, n := int32(0), plan.CumulativeLegendaryCount(count.Intn); i < n; i++ {
				stack := s.rollEndgameRiftCumulativeLegendary(char, i, legendaryPools[char], d.RewardClasses[char])
				if stack.Empty() {
					s.log.Warn("洞天暗金奖励生成失败，保留待生成状态", "run", r.RunID, "character", char, "ordinal", i)
					return
				}
				grant.Items = append(grant.Items, stack)
			}
		}
		var equipment, guaranteedUniques []domain.ItemID
		best, bestUnique := int32(0), int32(0)
		identity := d.Profiles[char]
		character := &domain.Character{Race: identity.Race, Employed: identity.Employed, EmploymentKnown: true}
		for id, def := range s.items {
			if r.Rules.EquipmentPools != nil {
				break
			}
			if def.HardDungeonOnly || def.Equip == nil || def.Equip.LevelReq > m.Level || def.Equip.LevelReq > 60 || def.Equip.AscensionOnly {
				continue
			}
			if !domain.NativeEquipmentDropEligible(def) {
				continue
			}
			need := def.Equip.Need
			if !need.AllowsCharacter(character) {
				continue
			}
			sex := uint8(2)
			if identity.Gender == 1 {
				sex = 1
			}
			if need.Sex != 0 && need.Sex != sex {
				continue
			}
			// A guaranteed unique slot has its own wearer-legal259-base pool.
			// All other slots retain the complete ordinary native pool above.
			if s.riftLegendaryBaseEligibleAtLevel(def, sourceLevel, d.RewardClasses[char]) {
				if def.Equip.LevelReq > bestUnique {
					bestUnique = def.Equip.LevelReq
					guaranteedUniques = nil
				}
				if def.Equip.LevelReq == bestUnique {
					guaranteedUniques = append(guaranteedUniques, id)
				}
			}
			if def.Equip.LevelReq > best {
				best = def.Equip.LevelReq
				equipment = nil
			}
			if def.Equip.LevelReq == best {
				equipment = append(equipment, id)
			}
		}
		if r.Rules.EquipmentPools != nil {
			eligible := func(id domain.ItemID) bool {
				def, ok := s.items[id]
				if !ok || def.HardDungeonOnly || def.Equip == nil || def.Equip.LevelReq > m.Level || def.Equip.LevelReq > 60 || def.Equip.AscensionOnly {
					return false
				}
				sex := uint8(2)
				if identity.Gender == 1 {
					sex = 1
				}
				return def.Equip.Need.AllowsCharacter(character) && (def.Equip.Need.Sex == 0 || def.Equip.Need.Sex == sex)
			}
			equipment = r.Rules.EquipmentPoolForLevel(poolLevel, eligible)
			equipment = r.Rules.FillMissingEquipmentSlots(equipment, m.Level, s.items, eligible)
			for _, id := range equipment {
				if s.riftLegendaryBaseEligibleAtLevel(s.items[id], sourceLevel, d.RewardClasses[char]) {
					guaranteedUniques = append(guaranteedUniques, id)
				}
			}
		}
		sort.Slice(equipment, func(i, j int) bool { return equipment[i] < equipment[j] })
		sort.Slice(guaranteedUniques, func(i, j int) bool { return guaranteedUniques[i] < guaranteedUniques[j] })
		for i := int32(0); i < plan.Equipment && len(equipment) > 0; i++ {
			candidates := equipment
			if r.Tier >= r.Rules.GuaranteedUniqueTier && i == 0 && len(guaranteedUniques) > 0 {
				candidates = guaranteedUniques
			}
			// With no eligible unique, preserve this one ordinary reward slot;
			// never synthesize a base or create an extra compensation item.
			def := s.items[candidates[rng.Intn(len(candidates))]]
			stack := domain.NewStack(def, 1)
			stack.Bound = true
			stack.Endgame.SourceLevel = sourceLevel
			// Explicit named native equipment keeps its original fixed properties.
			if domain.NativeEquipmentDropEligible(def) {
				_, affixes := s.equipmentRoll.RollColorFirstEquipment(def, domain.DefaultColorProfile, 0, r.Rules.Loot, rng.Int63n, nil, rng.Int63n)
				stack.RolledAffixCount = uint8(len(affixes))
				copy(stack.RolledAffixes[:], affixes)
				if (r.Tier >= r.Rules.GuaranteedUniqueTier && i == 0) || rng.Intn(100) < int(plan.UniqueChance) {
					if def.LegendaryEnabled {
						s.equipmentRoll.ApplyLegendaryLegacyForRecipientAtLevel(&stack, def, sourceLevel, d.RewardClasses[char], rng.Intn)
					} else {
						domain.ApplyUnique(&stack, def, rng.Intn)
					}
				}
			}
			grant.Items = append(grant.Items, stack)
		}
		for i := int32(0); i < plan.Rune; i++ {
			id, ok := r.Rules.Loot.RollRune(rng.Intn)
			if !ok {
				continue
			}
			if def, ok := s.itemDef(id); ok {
				stack := domain.NewStack(def, 1)
				stack.Bound = true
				grant.Items = append(grant.Items, stack)
			}
		}
		if def, ok := s.itemDef(r.Rules.DustItem); ok {
			stack := domain.NewStack(def, plan.Dust)
			stack.Bound = true
			grant.Items = append(grant.Items, stack)
		}
		// Fixed boss quantities do not take the level-gap occurrence penalty. Use
		// the same wearer-legal, level-appropriate native pool as ordinary slots,
		// but independent streams leave all historical special/rune/dust rolls intact.
		if plan.NormalBossEquipment > 0 {
			grant.MagicFind = contexts[char].MagicFind
			for i := int32(0); i < plan.NormalBossEquipment && len(equipment) > 0; i++ {
				if stack := s.rollEndgameRiftNormalBossEquipment(char, i, equipment, grant.MagicFind, d.RewardClasses[char]); !stack.Empty() {
					grant.Items = append(grant.Items, stack)
				}
			}
		}
		level := m.Level
		if r.Rules.CompletionLevelExperience {
			if context, ok := contexts[char]; ok {
				level = context.Level
			}
			grant.ExperienceLevel = level
			grant.ExperienceBase = s.trial.BaseExperience(level, s.levels)
		}
		grant.Exp = domain.RiftBossExperience(s.trial.BaseExperience(level, s.levels), r.Tier, r.Rules)
		if d.Version >= 2 {
			grant.ProgressionVersion = domain.ProgressionVersion
			grant.RewardVersion = domain.TrialRewardVersion
			grant.Pet = s.freezeEndgamePetGrant(s.endgameRewardPlayer(char), grant.Exp, r.RunID, char)
		}
		// Final source boundary also covers any future rune/dust/chest reward path
		// which obtains an item from configuration instead of the equipment pool.
		allowedItems := grant.Items[:0]
		for _, stack := range grant.Items {
			if def, ok := s.itemDef(stack.Item); ok && def.HardDungeonOnly {
				continue
			}
			allowedItems = append(allowedItems, stack)
		}
		grant.Items = allowedItems
		grants[char] = grant
	}
	d.Grants = grants
	d.GrantsReady = true
}

func (s *Scene) publishEndgameRewardExperience(p *entity.Entity, previousLevel int32, exp int64) {
	if exp <= 0 {
		return
	}
	s.emitTo(p.ID, s.attributeSnapshot(p))
	s.emitTo(p.ID, event.ExpGained{Who: p.ID, Delta: exp, Total: p.Player.Char.Exp})
	if p.Player.Char.Level > previousLevel {
		s.emit(event.LevelUp{Who: p.ID, Level: p.Player.Char.Level})
		s.emitTo(p.ID, s.skillSnapshot(p))
		s.refreshChangeSetTooltips(p)
	}
}

// Each item/purpose is keyed to the durable run and recipient, not scene tick,
// killer, live loot version or another reward slot's RNG consumption.
func (s *Scene) rollEndgameRiftNormalBossEquipment(char domain.CharID, ordinal int32, pool []domain.ItemID, magicFind int64, recipient ...domain.LegendaryRecipientClass) domain.Stack {
	// Refilter even explicit/cached pools. A stale database pool is not permission
	// to award hard-dungeon equipment in a Rift.
	allowed := make([]domain.ItemID, 0, len(pool))
	for _, id := range pool {
		if def, ok := s.items[id]; ok && !def.HardDungeonOnly && def.Equip != nil {
			allowed = append(allowed, id)
		}
	}
	pool = allowed
	if len(pool) == 0 {
		return domain.Stack{}
	}
	r := s.rift.data.Run
	stream := func(purpose string) lootRandomV5 {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-normal-boss/%d/%d/%d/%s", r.RunID, r.RulesVersion, char, ordinal, purpose)))
		return lootRandomV5{binary.BigEndian.Uint64(hash[:8])}
	}
	item := stream("item")
	quality, values := stream("quality"), stream("values")
	unique, variables := stream("unique"), stream("variables")
	def := s.items[pool[item.Intn(len(pool))]]
	stack := domain.NewStack(def, 1)
	stack.Bound = true
	sourceLevel := s.riftRewardSourceLevel()
	stack.Endgame.SourceLevel = sourceLevel
	if domain.NativeEquipmentDropEligible(def) {
		if r.Rules.CumulativeLegendaryPerTier > 0 {
			// Copy the pinned policy, never the live outdoor selector or shared table.
			rule, table := *r.Rules.Loot, s.equipmentRoll
			table.Counts = map[domain.ColorProfile][]domain.WeightedAffixCount{domain.DefaultColorProfile: rule.OutdoorEquipment.Counts()}
			rule.UniqueBasePPB = rule.OutdoorEquipment.LegendaryBasePPB
			if !def.LegendaryEnabled {
				rule.UniqueBasePPB = 0
			}
			s.rollEquipmentQualityV5(def, &stack, sourceLevel, table, domain.DefaultColorProfile, true, magicFind, &rule, quality.Int63n, values.Int63n, unique.Int63n, variables.Intn, recipient...)
		} else {
			s.rollNaturalEquipmentQualityV5Legacy(def, &stack, sourceLevel, domain.DefaultColorProfile, true, magicFind, r.Rules.Loot, &quality, &values, &unique, &variables, recipient...)
		}
	}
	return stack
}

// Promotion never expands the existing reward base pool or admits true hard-dungeon bases.
func (s *Scene) riftLegendaryBaseEligible(def domain.ItemDef) bool {
	if def.Equip == nil {
		return false
	}
	return s.riftLegendaryBaseEligibleAtLevel(def, def.Equip.LevelReq)
}

func (s *Scene) riftLegendaryBaseEligibleAtLevel(def domain.ItemDef, sourceLevel int32, recipient ...domain.LegendaryRecipientClass) bool {
	if def.HardDungeonOnly || def.Equip == nil || def.Equip.LevelReq > 60 || !domain.NativeEquipmentDropEligible(def) {
		return false
	}
	if def.LegendaryEnabled {
		var recipientClass domain.LegendaryRecipientClass
		if len(recipient) > 0 {
			recipientClass = recipient[0]
		}
		return s.equipmentRoll.LegendaryReady(def) && len(domain.LegendaryCandidatesForRecipientAtLevel(def, sourceLevel, recipientClass)) > 0
	}
	u, ok := domain.UniqueForBase(def.ID, s.endgameCatalog)
	return ok && u.NativeDropEligible
}

// Select only frozen native pools and wearer-legal bases with a real executable
// power and six-affix foundation. No fixed named gear or legacy Unique fallback.
func (s *Scene) riftCumulativeLegendaryPool(rules *domain.RiftRules, poolLevel, wearLevel, sourceLevel int32, identity endgameRiftIdentity, recipient ...domain.LegendaryRecipientClass) []domain.ItemID {
	recipientClass := identity.legendaryRecipientClass()
	if len(recipient) > 0 {
		recipientClass = recipient[0]
	}
	character := &domain.Character{Race: identity.Race, Employed: identity.Employed, EmploymentKnown: true}
	sex := uint8(2)
	if identity.Gender == 1 {
		sex = 1
	}
	if sourceLevel <= 0 {
		return nil
	}
	eligibility := map[domain.ItemID]bool{}
	eligible := func(id domain.ItemID) bool {
		if allowed, found := eligibility[id]; found {
			return allowed
		}
		def, ok := s.items[id]
		allowed := ok && def.LegendaryEnabled && def.Equip != nil && !def.Equip.AscensionOnly &&
			def.Equip.LevelReq <= wearLevel && def.Equip.Need.Level <= wearLevel && def.Equip.Need.AllowsCharacter(character) &&
			(def.Equip.Need.Sex == 0 || def.Equip.Need.Sex == sex) && s.riftLegendaryBaseEligibleAtLevel(def, sourceLevel, recipientClass)
		eligibility[id] = allowed
		return allowed
	}
	pool := rules.EquipmentPoolForLevel(poolLevel, eligible)
	pool = rules.FillMissingEquipmentSlots(pool, wearLevel, s.items, eligible)
	sort.Slice(pool, func(i, j int) bool { return pool[i] < pool[j] })
	return pool
}

func (s *Scene) riftCumulativeRandom(char domain.CharID, ordinal int32, purpose string) lootRandomV5 {
	r := s.rift.data.Run
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-cumulative/%d/%d/%d/%s", r.RunID, r.RulesVersion, char, ordinal, purpose)))
	return lootRandomV5{binary.BigEndian.Uint64(hash[:8])}
}

func (s *Scene) rollEndgameRiftCumulativeLegendary(char domain.CharID, ordinal int32, pool []domain.ItemID, recipient domain.LegendaryRecipientClass) domain.Stack {
	if len(pool) == 0 {
		return domain.Stack{}
	}
	item, values := s.riftCumulativeRandom(char, ordinal, "item"), s.riftCumulativeRandom(char, ordinal, "values")
	def := s.items[pool[item.Intn(len(pool))]]
	stack := domain.NewStack(def, 1)
	stack.Bound = true
	if !s.equipmentRoll.ApplyLegendaryForRecipientAtLevel(&stack, def, s.rift.data.RewardSourceLevel, recipient, values.Intn) {
		return domain.Stack{}
	}
	return stack
}

// Prefer the captured death event. Older pending snapshots can recover the
// exact encounter definition selected by the existing spawn algorithm, never
// an equipment requirement or an arbitrary player-level substitute.
func (s *Scene) riftRewardSourceLevel() int32 {
	d := &s.rift.data
	if d.RewardSourceLevel > 0 {
		return d.RewardSourceLevel
	}
	for id, registered := range d.Run.Monsters {
		if registered != nil && registered.Class == 3 {
			if monster := s.monsters[id]; monster != nil && monster.Level > 0 {
				return monster.Level
			}
		}
	}
	bases := s.riftFloorBaseDefinitions(true)
	if len(bases) == 0 {
		return 0
	}
	if d.Floor != nil {
		return bases[d.Floor.Seed%uint64(len(bases))].Level
	}
	// Floorless historical encounters lack a deterministic identity once gone.
	// Recovery is safe only when all possible real Boss definitions agree.
	level := bases[0].Level
	for _, base := range bases[1:] {
		if base.Level != level {
			return 0
		}
	}
	return level
}

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
	MagicFind   int64 `json:",omitempty"`
	RewardLevel int32 `json:",omitempty"`
}

// Completion constructs exact instance identities once; retries never reroll.
type endgameRiftRewardContext struct {
	Level     int32
	MagicFind int64
}

func (s *Scene) freezeEndgameRiftRewardContexts() map[domain.CharID]endgameRiftRewardContext {
	contexts := map[domain.CharID]endgameRiftRewardContext{}
	for char, member := range s.rift.data.Run.Members {
		profile := s.rift.data.Profiles[char]
		context := endgameRiftRewardContext{Level: profile.RewardLevel, MagicFind: profile.MagicFind}
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
	plan := domain.PlanRiftRewards(r.Tier, r.Timely, r.Rules)
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
		var equipment, guaranteedUniques []domain.ItemID
		best, bestUnique := int32(0), int32(0)
		identity := d.Profiles[char]
		character := &domain.Character{Race: identity.Race, Employed: identity.Employed, EmploymentKnown: true}
		for id, def := range s.items {
			if def.Equip == nil || def.Equip.LevelReq > m.Level {
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
			if unique, ok := domain.UniqueForBase(id, s.endgameCatalog); ok && unique.NativeDropEligible && def.Equip.LevelReq <= 60 {
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
			stack.SocketCount = domain.RollDroppedSocketCount(def, domain.AstralDropRealm, rng.Int63n, r.Rules.Loot)
			affixes := s.equipmentRoll.RollWithReservedSockets(def, domain.DefaultColorProfile, stack.SocketCount, rng.Int63n)
			stack.RolledAffixCount = uint8(len(affixes))
			copy(stack.RolledAffixes[:], affixes)
			if (r.Tier >= r.Rules.GuaranteedUniqueTier && i == 0) || rng.Intn(100) < int(plan.UniqueChance) {
				domain.ApplyUnique(&stack, def, rng.Intn)
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
				grant.Items = append(grant.Items, s.rollEndgameRiftNormalBossEquipment(char, i, equipment, grant.MagicFind))
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
		d.Grants[char] = grant
	}
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
func (s *Scene) rollEndgameRiftNormalBossEquipment(char domain.CharID, ordinal int32, pool []domain.ItemID, magicFind int64) domain.Stack {
	r := s.rift.data.Run
	stream := func(purpose string) lootRandomV5 {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-normal-boss/%d/%d/%d/%s", r.RunID, r.RulesVersion, char, ordinal, purpose)))
		return lootRandomV5{binary.BigEndian.Uint64(hash[:8])}
	}
	item, sockets := stream("item"), stream("sockets")
	quality, values := stream("quality"), stream("values")
	unique, variables := stream("unique"), stream("variables")
	def := s.items[pool[item.Intn(len(pool))]]
	stack := domain.NewStack(def, 1)
	stack.Bound = true
	stack.SocketCount = domain.RollDroppedSocketCount(def, domain.AstralDropBoss, sockets.Int63n, r.Rules.Loot)
	s.rollNaturalEquipmentQualityV5(def, &stack, domain.DefaultColorProfile, true, magicFind, r.Rules.Loot, &quality, &values, &unique, &variables)
	return stack
}

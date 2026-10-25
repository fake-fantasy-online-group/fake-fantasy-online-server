package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) magicFindV5(p *entity.Entity) int64 {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return 0
	}
	// MF reads eligibility and typed effects, never fractional durability. The
	// combat dependency key invalidates this immutable snapshot at break/repair.
	var worn *domain.EquipSet
	if state := s.combatAttributeState(p); state != nil {
		s.prepareCombatAttributes(p, state)
		worn = state.values.FunctionalWorn
	} else {
		worn = s.functionalWorn(p)
	}
	var total int64
	for _, active := range domain.ActiveEquipmentEffects(worn, s.itemDef, p.Player.Char) {
		if active.Effect.Kind == "magic_find_pct" {
			total += int64(active.Effect.Value)
		}
	}
	return max(int64(0), total)
}

// lootOwnerV5 follows stable source ownership rather than selecting an available
// player or a party maximum. A missing final owner retains its stable ID, but
// cannot contribute MF. A malformed owner cycle fails closed.
func (s *Scene) lootOwnerV5(source domain.EntityID) domain.EntityID {
	var visited [16]domain.EntityID
	for depth := range visited {
		if source == 0 {
			return 0
		}
		for _, previous := range visited[:depth] {
			if previous == source {
				return 0
			}
		}
		visited[depth] = source
		e := s.entities[source]
		if e == nil {
			e = s.pets[source]
		}
		if e == nil {
			e = s.traps[source]
		}
		if e == nil || e.Player != nil {
			return source
		}
		switch {
		case e.Pet != nil:
			source = e.Pet.Owner
		case e.Trap != nil:
			source = e.Trap.Owner
		case e.Monster != nil && e.Monster.Summoner != 0:
			source = e.Monster.Summoner
		default:
			return 0
		}
	}
	return 0
}

func (s *Scene) freezeLootContextV5(source domain.EntityID) domain.LootContextV5 {
	owner := s.lootOwnerV5(source)
	var recipient domain.LegendaryRecipientClass
	if p := s.players[owner]; p != nil && p.Player != nil {
		recipient = domain.LegendaryClassForCharacter(p.Player.Char)
	}
	return domain.LootContextV5{Owner: owner, RecipientClass: recipient, MagicFind: s.magicFindV5(s.players[owner]), Version: domain.LootRuleVersionV5}
}

func (s *Scene) periodicDamageOwnerV5(def domain.StatusDef, source domain.EntityID) domain.EntityID {
	damaging := false
	for _, change := range def.HPChanges {
		damaging = damaging || change.Value < 0
	}
	for _, affix := range def.Affixes {
		damaging = damaging || affix.Attr == domain.AttrHPRegen && affix.Value < 0
	}
	if damaging {
		if owner := s.lootOwnerV5(source); owner != 0 {
			return owner
		}
	}
	return source
}

// Each purpose and generated item has its own small, deterministic RNG. Drawing
// extra affixes or succeeding at unique promotion cannot consume the item,
// rune, other-item, or unique-variable streams. Version changes need new domains.
// This deliberately preserves MF=0 distributions, not legacy same-seed sequences.
type lootRandomV5 struct{ state uint64 }

func (r *lootRandomV5) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *lootRandomV5) Int63n(n int64) int64 {
	if n <= 0 {
		panic("invalid v5 loot random bound")
	}
	// Rejection sampling avoids modulo bias for both 99-part legacy weights and
	// the billion-part quantized tables.
	bound := uint64(n)
	threshold := -bound % bound
	for {
		v := r.next()
		if v >= threshold {
			return int64(v % bound)
		}
	}
}

func (r *lootRandomV5) Intn(n int) int { return int(r.Int63n(int64(n))) }

func (s *Scene) lootRandomV5(dead domain.EntityID, ordinal int, purpose uint64) lootRandomV5 {
	seed := lootRandomV5{uint64(s.lootSeed)}
	seed.state ^= uint64(dead)
	seed.state = seed.next() ^ uint64(s.tick)
	seed.state = seed.next() ^ uint64(ordinal)
	seed.state = seed.next() ^ purpose
	return lootRandomV5{seed.next()}
}

func (s *Scene) rollNaturalEquipmentV5(def domain.ItemDef, stack *domain.Stack, dead *entity.Entity, ordinal int, profile domain.ColorProfile, rollAffixes bool, context domain.LootContextV5) {
	if def.Equip == nil || dead == nil || dead.Monster == nil {
		return
	}
	quality := s.lootRandomV5(dead.ID, ordinal, 0x56350001)
	values := s.lootRandomV5(dead.ID, ordinal, 0x56350002)
	unique := s.lootRandomV5(dead.ID, ordinal, 0x56350003)
	variables := s.lootRandomV5(dead.ID, ordinal, 0x56350004)
	rule, table := s.lootRules.Active(), s.equipmentRoll
	// Theft is another drop from the same target and uses the same source rules;
	// its reserved negative ordinal still keeps its random streams independent.
	var outdoor *domain.OutdoorEquipmentRule
	if rule != nil && s.outdoorEquipmentQualitySource(dead, def, rollAffixes) {
		outdoor = rule.OutdoorEquipment
		if dead.Monster.Kind == domain.MonsterNormal {
			outdoor = rule.OutdoorNormalEquipment
		}
	}
	if outdoor != nil {
		// Both are private copies. Never replace the shared profile 1, which is
		// also used by ordinary dungeon and frozen Rift reward paths.
		table.Counts = map[domain.ColorProfile][]domain.WeightedAffixCount{profile: outdoor.Counts()}
		rule.UniqueBasePPB = outdoor.LegendaryBasePPB
		if !def.LegendaryEnabled {
			// A missing six-affix/power foundation keeps the ordinary result;
			// the new outdoor chance must not revive legacy Unique instances.
			rule.UniqueBasePPB = 0
		}
	}
	s.rollEquipmentQualityV5(def, stack, dead.Level, table, profile, rollAffixes, context.MagicFind, rule, quality.Int63n, values.Int63n, unique.Int63n, variables.Intn, context.RecipientClass)
}

func (s *Scene) outdoorEquipmentQualitySource(dead *entity.Entity, def domain.ItemDef, rollAffixes bool) bool {
	return dead != nil && dead.Monster != nil && dead.Monster.Summoner == 0 &&
		(dead.Monster.Kind == domain.MonsterNormal || dead.Monster.Kind == domain.MonsterElite || dead.Monster.Kind == domain.MonsterBoss) &&
		s.id.Instance == 0 && s.dungeon == nil && s.rift == nil && s.trialRun == nil && !s.trial.IsMap(s.id.MapID) &&
		rollAffixes && domain.NativeEquipmentDropEligible(def)
}

// Shared by natural world drops and new normal-boss reward slots. Callers own
// independent deterministic streams and the appropriate frozen loot rules.
func (s *Scene) rollNaturalEquipmentQualityV5(def domain.ItemDef, stack *domain.Stack, sourceLevel int32, profile domain.ColorProfile, rollAffixes bool, magicFind int64, rule *domain.LootRuleSet, quality, values, unique, variables *lootRandomV5, recipient ...domain.LegendaryRecipientClass) {
	s.rollEquipmentQualityV5(def, stack, sourceLevel, s.equipmentRoll, profile, rollAffixes, magicFind, rule, quality.Int63n, values.Int63n, unique.Int63n, variables.Intn, recipient...)
}

// Frozen pre-v12 Rift snapshots retain their pre-variant rolls and RNG stream.
func (s *Scene) rollNaturalEquipmentQualityV5Legacy(def domain.ItemDef, stack *domain.Stack, sourceLevel int32, profile domain.ColorProfile, rollAffixes bool, magicFind int64, rule *domain.LootRuleSet, quality, values, unique, variables *lootRandomV5, recipient ...domain.LegendaryRecipientClass) {
	s.rollEquipmentQualityV5WithVariants(def, stack, sourceLevel, s.equipmentRoll, profile, rollAffixes, magicFind, rule, quality.Int63n, values.Int63n, unique.Int63n, variables.Intn, false, recipient...)
}

func (s *Scene) rollEquipmentQualityV5(def domain.ItemDef, stack *domain.Stack, sourceLevel int32, table domain.EquipmentRollTable, profile domain.ColorProfile, rollAffixes bool, magicFind int64, rule *domain.LootRuleSet, quality, values, unique func(int64) int64, variables func(int) int, recipient ...domain.LegendaryRecipientClass) {
	s.rollEquipmentQualityV5WithVariants(def, stack, sourceLevel, table, profile, rollAffixes, magicFind, rule, quality, values, unique, variables, true, recipient...)
}

func (s *Scene) rollEquipmentQualityV5WithVariants(def domain.ItemDef, stack *domain.Stack, sourceLevel int32, table domain.EquipmentRollTable, profile domain.ColorProfile, rollAffixes bool, magicFind int64, rule *domain.LootRuleSet, quality, values, unique func(int64) int64, variables func(int) int, variants bool, recipient ...domain.LegendaryRecipientClass) {
	if stack == nil || def.Equip == nil {
		return
	}
	// Keep provenance even if promotion misses, so later cube awakening cannot
	// substitute the player's level or the base's unrelated wear requirement.
	if sourceLevel > 0 && stack.Endgame.SourceLevel == 0 {
		stack.Endgame.SourceLevel = sourceLevel
	}
	if rollAffixes {
		_, rolled := table.RollColorFirstEquipment(def, profile, magicFind, rule, quality, nil, values)
		stack.RolledAffixCount = uint8(len(rolled))
		copy(stack.RolledAffixes[:], rolled)
	}
	var recipientClass domain.LegendaryRecipientClass
	if len(recipient) > 0 {
		recipientClass = recipient[0]
	}
	if def.LegendaryEnabled {
		if !def.Equip.NoTypeDrop && def.Equip.NativeDropEligible && unique(domain.LootProbabilityScaleV5) < domain.UniqueProbabilityV5(magicFind, rule) {
			if variants {
				table.ApplyLegendaryForRecipientAtLevel(stack, def, sourceLevel, recipientClass, variables)
			} else {
				table.ApplyLegendaryLegacyForRecipientAtLevel(stack, def, sourceLevel, recipientClass, variables)
			}
		}
		return
	}
	domain.RollNaturalUniqueWithMagicFindV5(stack, def, magicFind, unique, variables, rule)
}

package scene

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func requireRecipientPower(t *testing.T, st domain.Stack, def domain.ItemDef, recipient domain.LegendaryRecipientClass) string {
	t.Helper()
	power, ok := domain.LegendaryPower(st.Legendary.PowerID)
	if !ok || st.Legendary.Validate() != nil || !domain.LegendaryPowerEligibleForItem(power, def, st) {
		t.Fatal("invalid generated legendary", st.Item, st.Legendary.PowerID)
	}
	for _, allowed := range domain.LegendaryCandidatesForRecipientAtLevel(def, st.Endgame.SourceLevel, recipient) {
		if allowed.ID == power.ID {
			return power.Class
		}
	}
	t.Fatal("generated another recipient's power", recipient, power.ID)
	return ""
}

func forceRecipientLegendaryRules(t *testing.T, s *Scene) {
	t.Helper()
	rule := *lootRulesFixture()
	rule.Version, rule.UniqueBasePPB, rule.UniqueMFBoost = 3, domain.LootProbabilityScaleV5, 0
	rule.OutdoorEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{1}, LegendaryBasePPB: domain.LootProbabilityScaleV5}
	rule.OutdoorNormalEquipment = rule.OutdoorEquipment
	var err error
	s.lootRules, err = domain.NewLootRuleTable(3, []domain.LootRuleSet{*lootRulesFixture(), rule}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
}

func TestLegendaryRecipientNativeCrossClassPetTrapSummonAndBossPair(t *testing.T) {
	fixture, _ := outdoorQualityFixture(t)
	def := fixture.items[1608] // Actual level42 warrior-only two-hand base 玄纹龙影.
	if def.Equip.Need.Professions != 1 || !def.LegendaryEnabled {
		t.Fatal("native warrior fixture changed")
	}
	before, _ := json.Marshal(def)
	for _, kind := range []domain.EntityKind{domain.KindPlayer, domain.KindPet, domain.KindTrap, domain.KindMonster} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			s, mid := lootScene(t, []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}})
			s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
			forceRecipientLegendaryRules(t, s)
			join(t, s, 1, 100, "assassin owner", 100, 100)
			join(t, s, 2, 200, "warrior party member", 100, 100)
			s.players[1].Player.Char.Race = domain.Assassin
			s.players[2].Player.Char.Race = domain.Warrior
			source := s.players[1]
			if kind != domain.KindPlayer {
				source = &entity.Entity{ID: 71, Kind: kind, HP: 100}
				switch kind {
				case domain.KindPet:
					source.Pet = &entity.Pet{Owner: 1}
					s.pets[source.ID] = source
				case domain.KindTrap:
					source.Trap = &entity.Trap{Owner: 1}
				case domain.KindMonster:
					source.Monster = &entity.Monster{Summoner: 1}
				}
				s.entities[source.ID] = source
			}
			dead := s.entities[mid]
			dead.Monster.Kind, dead.Level, dead.HP = domain.MonsterBoss, 43, 0
			if kind == domain.KindPlayer {
				s.bossFixedDrops = domain.BossFixedDropTable{{MapID: s.id.MapID, Monster: dead.Monster.TypeID}: {Minimum: 1, Pool: []domain.BossFixedDropChoice{{Item: def.ID, Weight: 1, RollAffixes: true}}}}
			}
			s.onDeath(dead, source.ID)
			if len(s.ground) != 3 {
				t.Fatal("original drop or Boss2 pair missing", len(s.ground))
			}
			for _, drop := range s.ground {
				if drop.Drop.Owner != 1 || drop.Drop.Stack.Item != 1608 || requireRecipientPower(t, drop.Drop.Stack, def, 4) != "全职业" {
					t.Fatal("base transformed, class leaked, or actual source owner lost", drop.Drop)
				}
			}
			after, _ := json.Marshal(s.items[def.ID])
			if string(before) != string(after) {
				t.Fatal("base requirement transformed")
			}
		})
	}
}

func TestLegendaryRecipientTheftUsesThiefNotBaseClass(t *testing.T) {
	fixture, _ := outdoorQualityFixture(t)
	def := fixture.items[1608]
	s, src, dst, _ := theftTestScene(t, 10000)
	s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
	forceRecipientLegendaryRules(t, s)
	s.lootTable[dst.Monster.TypeID] = []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}}
	dst.Monster.Kind, dst.Level = domain.MonsterBoss, 43
	skill := s.skills[domain.SkillKey{ID: 10212, Level: 1}]
	if !s.stealMonsterItem(src, dst, skill, skill.Effects[0]) {
		t.Fatal("theft failed")
	}
	drop := onlyTheftDrop(t, s)
	if drop.Drop.Owner != src.ID || drop.Drop.Stack.Item != def.ID || requireRecipientPower(t, drop.Drop.Stack, def, 4) != "全职业" || dst.Monster.OutdoorBossLegendariesGranted {
		t.Fatal("theft borrowed base class or consumed Boss2")
	}
}

func TestLegendaryRecipientFrozenDeathAndUnknownOwner(t *testing.T) {
	fixture, _ := legendaryDropFixture(t)
	def := fixture.items[1001]
	for _, unknown := range []bool{false, true} {
		s, mid := lootScene(t, []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}})
		s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
		forceRecipientLegendaryRules(t, s)
		join(t, s, 1, 100, "owner", 100, 100)
		s.players[1].Player.Char.Race = domain.Assassin
		if unknown {
			delete(s.players, 1)
			delete(s.entities, 1)
		}
		ctx := s.freezeLootContextV5(1)
		if ctx.Owner != 1 || !unknown && ctx.RecipientClass != 4 || unknown && ctx.RecipientClass != 0 {
			t.Fatal("incorrect frozen context", ctx)
		}
		if !unknown {
			s.players[1].Player.Char.Race = domain.Warrior
			delete(s.players, 1)
			delete(s.entities, 1)
		}
		dead := s.entities[mid]
		dead.Level = 43
		seenOwn := false
		for i := 0; i < 64; i++ {
			s.tick++
			s.rollLootWithContextV5(dead, ctx)
		}
		for _, drop := range s.ground {
			class := requireRecipientPower(t, drop.Drop.Stack, def, ctx.RecipientClass)
			seenOwn = seenOwn || class == "刺客"
			if drop.Drop.Owner != 1 || unknown && class != "全职业" {
				t.Fatal("missing owner borrowed a party profession")
			}
		}
		if !unknown && !seenOwn {
			t.Fatal("frozen own class was lost after disconnect")
		}
	}
}

func TestLegendaryRecipientMixedPartyRiftAllGenerationRoutes(t *testing.T) {
	for _, version := range []int32{11, 12, 13} {
		legacy := version == 11
		s, latest, old := cumulativeRiftFixture(t)
		rules := *latest
		if legacy {
			rules = *old
			rules.UniqueBaseChance, rules.UniquePerTier, rules.UniqueCap = 100, 0, 100
		}
		if version == 13 {
			rules.Version, rules.GuardianHPDivisor = version, 10
			oldHP := domain.RiftScaledStat(10009, 10, 5, 3, true, latest)
			if hp := domain.RiftScaledStat(10009, 10, 5, 3, true, &rules); hp != oldHP/10 {
				t.Fatal("new guardian health rule did not coexist with recipient reward policy", hp, oldHP)
			}
		}
		fixture, def := legendaryDropFixture(t)
		s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
		rules.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 60, Items: []domain.ItemID{def.ID}}}
		rules.EquipmentSlotFallbacks = nil
		loot := *rules.Loot
		loot.UniqueBasePPB = domain.LootProbabilityScaleV5
		loot.OutdoorEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{1}, LegendaryBasePPB: domain.LootProbabilityScaleV5}
		rules.Loot = &loot
		prepareCumulativeRun(s, &rules, fmt.Sprintf("recipient-party-v%d", version), 10, true, 60, 60, 60, 60, 60)
		s.generateEndgameRiftGrants()
		if !s.rift.data.GrantsReady {
			t.Fatal("mixed party grant not ready")
		}
		for char, grant := range s.rift.data.Grants {
			recipient := domain.LegendaryRecipientClass(1 << uint8(char/100-1))
			count, own := 0, false
			for _, st := range grant.Items {
				if st.Item != def.ID {
					continue
				}
				count++
				own = requireRecipientPower(t, st, def, recipient) != "全职业" || own
			}
			plan := domain.PlanRiftRewards(10, true, &rules)
			minimum := int(plan.Equipment + plan.NormalBossEquipment)
			if !legacy {
				minimum += int(plan.GuaranteedLegendaries)
			}
			if count < minimum || !own {
				t.Fatal("lost own-class or normal/special rewards", legacy, char, count, minimum, own)
			}
		}
		// Previously generated (including historical) grants remain byte-for-byte
		// immutable even if the present recipient class and profile now differ.
		before, _ := json.Marshal(s.rift.data.Grants)
		s.rift.data = s.rift.data.clone()
		for char, profile := range s.rift.data.Profiles {
			profile.Race = domain.Warlock
			s.rift.data.Profiles[char] = profile
		}
		s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {RecipientClass: 16, RecipientClassKnown: true}})
		after, _ := json.Marshal(s.rift.data.Grants)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("ready grants rerolled on class change")
		}
	}
}

func TestLegendaryRecipientRiftFailedGenerationFreezesClassesAcrossPersistence(t *testing.T) {
	s, rules, _ := cumulativeRiftFixture(t)
	prepareCumulativeRun(s, rules, "recipient-retry", 1, false, 43)
	delete(s.rift.data.Profiles, 100) // No profile is not an implicit warrior.
	contexts := map[domain.CharID]endgameRiftRewardContext{100: {Level: 43, RecipientClass: 4, RecipientClassKnown: true}}
	items := s.items
	s.items = map[domain.ItemID]domain.ItemDef{}
	s.generateEndgameRiftGrantsWithContexts(contexts)
	if s.rift.data.GrantsReady || s.rift.data.RewardClasses[100] != 4 {
		t.Fatal("failed generation discarded accepted class")
	}
	s.rift.data = s.rift.data.clone()
	s.items = items
	contexts[100] = endgameRiftRewardContext{Level: 43, RecipientClass: 1, RecipientClassKnown: true}
	s.generateEndgameRiftGrantsWithContexts(contexts)
	if !s.rift.data.GrantsReady || s.rift.data.RewardClasses[100] != 4 {
		t.Fatal("retry changed accepted recipient")
	}
	for _, st := range s.rift.data.Grants[100].Items {
		if st.Legendary.PowerID != "" {
			requireRecipientPower(t, st, s.items[st.Item], 4)
		}
	}
}

func TestLegendaryRecipientGroundPickupDoesNotRerollForNewOwner(t *testing.T) {
	fixture, def := legendaryDropFixture(t)
	s, _ := lootScene(t, nil)
	s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
	join(t, s, 1, 100, "assassin origin", 100, 100)
	join(t, s, 2, 200, "warrior picker", 100, 100)
	s.players[1].Player.Char.Race, s.players[2].Player.Char.Race = domain.Assassin, domain.Warrior
	st := domain.NewStack(def, 1)
	// CK sorts before TY, so this produces a genuine assassin-specific power.
	if !s.equipmentRoll.ApplyLegendaryForRecipientAtLevel(&st, def, 43, 4, func(int) int { return 0 }) || requireRecipientPower(t, st, def, 4) != "刺客" {
		t.Fatal("missing class-specific pickup fixture")
	}
	s.spawnGeneratedLoot(def, st, s.players[1].Pos, 1)
	drop := onlyTheftDrop(t, s)
	s.onPickUp(PickUp{ID: 2, Target: drop.ID})
	if s.ground[drop.ID] == nil || s.ground[drop.ID].Drop.Stack != st {
		t.Fatal("other picker bypassed owner protection or changed power")
	}
	s.tick = drop.Drop.OwnerUntil
	s.onPickUp(PickUp{ID: 2, Target: drop.ID})
	if s.ground[drop.ID] != nil {
		t.Fatal("post-protection pickup failed")
	}
	found := false
	s.players[2].Player.Bag.Each(func(_ int, current domain.Stack) {
		if current.UID == st.UID {
			found = true
			if current != st {
				t.Fatal("new owner rerolled existing power")
			}
		}
	})
	if !found {
		t.Fatal("ground instance identity lost")
	}
}

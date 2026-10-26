package scene

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"testing"
)

func splitPowerSceneRules(t *testing.T, s *Scene) {
	t.Helper()
	rule := *lootRulesFixture()
	rule.Version, rule.LegendaryPowerPolicy = 4, domain.LegendaryPowerPolicySplit
	rule.UniqueBasePPB, rule.UniqueMFBoost = domain.LootProbabilityScaleV5, 0
	rule.OutdoorEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{1}, LegendaryBasePPB: domain.LootProbabilityScaleV5}
	rule.OutdoorNormalEquipment = rule.OutdoorEquipment
	var err error
	s.lootRules, err = domain.NewLootRuleTable(4, []domain.LootRuleSet{*lootRulesFixture(), rule}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
}
func splitPowerClass(t *testing.T, st domain.Stack, def domain.ItemDef, recipient domain.LegendaryRecipientClass) string {
	t.Helper()
	power, ok := domain.LegendaryPower(st.Legendary.PowerID)
	if !ok || st.Legendary.Validate() != nil || !domain.LegendaryPowerEligibleForItem(power, def, st) || len(domain.LegendaryItemBonuses(def, st)) != 6 {
		t.Fatal("invalid all-slot legendary", st.Item, st.Legendary.PowerID)
	}
	for _, candidate := range domain.LegendaryCandidatesForRecipientWithPolicyAtLevel(def, st.Endgame.SourceLevel, recipient, domain.LegendaryPowerPolicySplit) {
		if candidate.ID == power.ID {
			return power.Class
		}
	}
	t.Fatal("power did not belong to frozen recipient", recipient, power.ID)
	return ""
}

// Production natural and guaranteed Boss-pair consumers, with an unrestricted face.
func TestLegendaryPowerPolicyNaturalFrozenOwnerAndBossPair(t *testing.T) {
	fixture, def := legendaryDropFixture(t)
	def.Equip.Slot, def.Equip.LevelReq, def.Equip.Need = int32(domain.SlotFace), 1, domain.Requirement{}
	s, mid := lootScene(t, []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}})
	s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
	splitPowerSceneRules(t, s)
	join(t, s, 1, 100, "frozen warlock", 100, 100)
	s.players[1].Player.Char.Race = domain.Warlock
	ctx := s.freezeLootContextV5(1)
	if ctx.RecipientClass != 16 {
		t.Fatal("wrong profession", ctx)
	}
	s.players[1].Player.Char.Race = domain.Warrior
	delete(s.players, 1)
	dead := s.entities[mid]
	dead.Level, dead.Monster.Kind = 43, domain.MonsterNormal
	for i := 0; i < 48; i++ {
		s.tick++
		s.rollLootWithContextV5(dead, ctx)
	}
	counts := map[string]int{}
	for _, drop := range s.ground {
		if drop.Drop.Owner != 1 {
			t.Fatal("lost owner")
		}
		counts[splitPowerClass(t, drop.Drop.Stack, def, 16)]++
	}
	if len(s.ground) != 48 || counts["术士"] == 0 || counts["全职业"] == 0 {
		t.Fatal("natural category/count", counts, len(s.ground))
	}
	seen := map[domain.EntityID]bool{}
	for id := range s.ground {
		seen[id] = true
	}
	dead.Monster.Kind = domain.MonsterBoss
	pairCounts := map[string]int{}
	for life := 0; life < 32; life++ {
		s.tick++
		dead.Monster.OutdoorBossLegendariesGranted = false // Independent simulated Boss life.
		before := len(s.ground)
		if n := s.rollOutdoorBossLegendaries(dead, ctx, 0); n != 2 || len(s.ground) != before+2 {
			t.Fatal("Boss pair lost", n, len(s.ground))
		}
		if n := s.rollOutdoorBossLegendaries(dead, ctx, 0); n != 0 || len(s.ground) != before+2 {
			t.Fatal("Boss pair rerolled", n)
		}
		for id, drop := range s.ground {
			if !seen[id] {
				seen[id] = true
				pairCounts[splitPowerClass(t, drop.Drop.Stack, def, 16)]++
			}
		}
	}
	if pairCounts["术士"] == 0 || pairCounts["全职业"] == 0 {
		t.Fatal("Boss pair consumer missed split policy", pairCounts)
	}
}
func TestLegendaryPowerPolicyTheftAllSlot(t *testing.T) {
	fixture, def := legendaryDropFixture(t)
	def.Equip.Slot, def.Equip.LevelReq, def.Equip.Need = int32(domain.SlotFace), 1, domain.Requirement{}
	s, src, dst, _ := theftTestScene(t, 10000)
	s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
	splitPowerSceneRules(t, s)
	s.lootTable[dst.Monster.TypeID] = []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}}
	dst.Monster.Kind, dst.Level = domain.MonsterBoss, 43
	skill := s.skills[domain.SkillKey{ID: 10212, Level: 1}]
	for i := 0; i < 48; i++ {
		s.tick++
		dst.Monster.TheftSuccesses = 0 // Independent simulated target life.
		if !s.stealMonsterItem(src, dst, skill, skill.Effects[0]) {
			t.Fatal("theft failed", i)
		}
		if s.stealMonsterItem(src, dst, skill, skill.Effects[0]) {
			t.Fatal("same life paid twice")
		}
	}
	counts := map[string]int{}
	for _, drop := range s.ground {
		if drop.Drop.Owner != src.ID || dst.Monster.OutdoorBossLegendariesGranted {
			t.Fatal("theft owner/Boss marker changed")
		}
		counts[splitPowerClass(t, drop.Drop.Stack, def, 4)]++
	}
	if len(s.ground) != 48 || counts["刺客"] == 0 || counts["全职业"] == 0 {
		t.Fatal("theft category/count", counts)
	}
}
func splitPowerRiftFixture(t *testing.T) (*Scene, *domain.RiftRules, *domain.RiftRules, *domain.RiftRules) {
	t.Helper()
	s, v15, v14 := slotFirstRiftFixture(t)
	l1, _ := s.lootRules.Version(1)
	l2, _ := s.lootRules.Version(2)
	l3, _ := s.lootRules.Version(3)
	l4 := *l3
	l4.Version, l4.LegendaryPowerPolicy = 4, domain.LegendaryPowerPolicySplit
	l4.UniqueBasePPB, l4.UniqueMFBoost = domain.LootProbabilityScaleV5, 0
	l4.OutdoorEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{1}, LegendaryBasePPB: domain.LootProbabilityScaleV5}
	fresh := *v15
	fresh.Version, fresh.LootVersion, fresh.Loot = 16, 4, &l4
	var err error
	s.lootRules, err = domain.NewLootRuleTable(4, []domain.LootRuleSet{*l1, *l2, *l3, l4}, s.items)
	if err != nil {
		t.Fatal(err)
	}
	s.riftRules, err = domain.NewRiftRuleTable(16, []domain.RiftRules{*riftRulesFixture(), *v14, *v15, fresh}, s.lootRules, s.items)
	if err != nil {
		t.Fatal(err)
	}
	return s, s.riftRules.Active(), v15, v14
}
func TestLegendaryPowerPolicyRiftBothPathsAndLegacyExactRolls(t *testing.T) {
	s, v16, v15, v14 := splitPowerRiftFixture(t)
	identity := endgameRiftIdentity{Race: domain.Warlock, Gender: 0, Employed: true}
	const recipient domain.LegendaryRecipientClass = 16
	for _, rules := range []*domain.RiftRules{v14, v15, v16} {
		pool := s.riftCumulativeLegendaryPool(rules, 43, 43, 65, identity, recipient)
		if len(pool) == 0 {
			t.Fatal("empty pool", rules.Version)
		}
		prepareCumulativeRun(s, rules, "power-policy-rift", 150, true, 43)
		counts := [2]map[string]int{{}, {}}
		classSlots := [2]map[int32]bool{{}, {}}
		for i := int32(0); i < 256; i++ {
			for path := 0; path < 2; path++ {
				var got domain.Stack
				var selection lootRandomV5
				if path == 0 {
					got = s.rollEndgameRiftCumulativeLegendary(100, i, pool, recipient)
					selection = s.riftCumulativeRandom(100, i, "item")
				} else {
					got = s.rollEndgameRiftNormalBossEquipment(100, i, pool, 0, recipient)
					h := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-normal-boss/%d/%d/%d/item", s.rift.data.Run.RunID, rules.Version, 100, i)))
					selection = lootRandomV5{binary.BigEndian.Uint64(h[:8])}
				}
				var wantID domain.ItemID
				if rules.Version < 15 {
					wantID = pool[selection.Intn(len(pool))]
				} else {
					wantID = slotFirstOracle(s, pool, &selection)
				}
				if got.Item != wantID {
					t.Fatal("changed slot/template", rules.Version, path, i)
				}
				def := s.items[got.Item]
				if rules.Version < 16 {
					if got.Legendary.PowerID != "" {
						requireRecipientPower(t, got, def, recipient)
					}
					if path == 0 {
						values := s.riftCumulativeRandom(100, i, "values")
						want := domain.NewStack(def, 1)
						want.Bound = true
						if !s.equipmentRoll.ApplyLegendaryForRecipientAtLevel(&want, def, 65, recipient, values.Intn) {
							t.Fatal("legacy oracle failed")
						}
						if !reflect.DeepEqual(normalizeRiftStacks([]domain.Stack{got}), normalizeRiftStacks([]domain.Stack{want})) {
							t.Fatal("legacy exact power/affix/tier changed", rules.Version, i)
						}
					} else {
						// Independent pre-policy normal-Boss oracle, including all four
						// purpose streams, color, promotion, affixes and variant draws.
						stream := func(purpose string) lootRandomV5 {
							h := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-normal-boss/%d/%d/%d/%s", s.rift.data.Run.RunID, rules.Version, 100, i, purpose)))
							return lootRandomV5{binary.BigEndian.Uint64(h[:8])}
						}
						q, v, u, powers := stream("quality"), stream("values"), stream("unique"), stream("variables")
						rule, table := *rules.Loot, s.equipmentRoll
						table.Counts = map[domain.ColorProfile][]domain.WeightedAffixCount{domain.DefaultColorProfile: rule.OutdoorEquipment.Counts()}
						rule.UniqueBasePPB = rule.OutdoorEquipment.LegendaryBasePPB
						want := domain.NewStack(def, 1)
						want.Bound, want.Endgame.SourceLevel = true, 65
						_, rolled := table.RollColorFirstEquipment(def, domain.DefaultColorProfile, 0, &rule, q.Int63n, nil, v.Int63n)
						want.RolledAffixCount = uint8(len(rolled))
						copy(want.RolledAffixes[:], rolled)
						if u.Int63n(domain.LootProbabilityScaleV5) < domain.UniqueProbabilityV5(0, &rule) {
							if !table.ApplyLegendaryForRecipientAtLevel(&want, def, 65, recipient, powers.Intn) {
								t.Fatal("legacy normal oracle failed")
							}
						}
						if !reflect.DeepEqual(normalizeRiftStacks([]domain.Stack{got}), normalizeRiftStacks([]domain.Stack{want})) {
							t.Fatal("legacy normal exact stream changed", rules.Version, i)
						}
					}
					continue
				}
				class := splitPowerClass(t, got, def, recipient)
				counts[path][class]++
				if class == "术士" {
					classSlots[path][def.Equip.Slot] = true
				}
			}
		}
		if rules.Version == 16 {
			for path := 0; path < 2; path++ {
				if counts[path]["术士"] < 80 || counts[path]["术士"] > 176 || counts[path]["全职业"] == 0 {
					t.Fatal("category skew", path, counts[path])
				}
				for _, slot := range []domain.EquipSlot{domain.SlotFace, domain.SlotGlove, domain.SlotRing, domain.SlotNeck, domain.SlotShoe, domain.SlotBag} {
					if !classSlots[path][int32(slot)] {
						t.Fatal("missing class on former forbidden slot", path, slot, classSlots[path])
					}
				}
			}
		}
	}
}
func TestLegendaryPowerPolicyRiftPersistedRetryAndReadyFreeze(t *testing.T) {
	s, fresh, _, _ := splitPowerRiftFixture(t)
	prepareCumulativeRun(s, fresh, "power-policy-retry", 150, true, 43)
	delete(s.rift.data.Profiles, 100)
	contexts := map[domain.CharID]endgameRiftRewardContext{100: {Level: 43, RecipientClass: 16, RecipientClassKnown: true}}
	items := s.items
	s.items = map[domain.ItemID]domain.ItemDef{}
	s.generateEndgameRiftGrantsWithContexts(contexts)
	if s.rift.data.GrantsReady || s.rift.data.RewardClasses[100] != 16 {
		t.Fatal("failed award lost class")
	}
	s.items = items
	raw, err := json.Marshal(s.rift.data)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &s.rift.data); err != nil {
		t.Fatal(err)
	}
	if err = s.rift.data.Run.BindRules(s.riftRules); err != nil {
		t.Fatal(err)
	}
	contexts[100] = endgameRiftRewardContext{Level: 43, RecipientClass: 1, RecipientClassKnown: true}
	s.generateEndgameRiftGrantsWithContexts(contexts)
	if !s.rift.data.GrantsReady || s.rift.data.RewardClasses[100] != 16 {
		t.Fatal("retry changed class")
	}
	gear := 0
	for _, st := range s.rift.data.Grants[100].Items {
		if st.Legendary.PowerID != "" {
			gear++
			splitPowerClass(t, st, s.items[st.Item], 16)
		}
	}
	if gear != 25 {
		t.Fatal("cumulative15/normal10 count changed", gear)
	}
	before, _ := json.Marshal(s.rift.data.Grants)
	s.generateEndgameRiftGrantsWithContexts(nil)
	after, _ := json.Marshal(s.rift.data.Grants)
	if string(before) != string(after) {
		t.Fatal("ready grants rerolled")
	}
}

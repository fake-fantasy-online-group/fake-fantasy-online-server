package scene

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func weightedSceneLootFixture(t *testing.T) *domain.LootRuleTable {
	t.Helper()
	legacy := *lootRulesFixture()
	weighted := *lootRulesFixture()
	weighted.Version = 2
	for i := range weighted.RunePool {
		weighted.RuneWeights = append(weighted.RuneWeights, int64(129-4*i))
	}
	table, err := domain.NewLootRuleTable(2, []domain.LootRuleSet{legacy, weighted}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func expectedWeightedRune(roll int) domain.ItemID {
	for i := 0; i < 28; i++ {
		weight := 129 - 4*i
		if roll < weight {
			return domain.FirstAstralRune + domain.ItemID(i)
		}
		roll -= weight
	}
	return 0
}

func TestSupplementalRuneLootUsesPublishedWeights(t *testing.T) {
	for seed := int64(0); seed < 64; seed++ {
		s, mid := lootScene(t, nil)
		for id, d := range balanceItemsFixture() {
			s.items[id] = d
		}
		rule := *weightedSceneLootFixture(t).Active()
		rule.MinRuneLevel = 14
		rule.Grades[0].RuneRateBP = 10000
		var err error
		s.lootRules, err = domain.NewLootRuleTable(2, []domain.LootRuleSet{*lootRulesFixture(), rule}, s.items)
		if err != nil {
			t.Fatal(err)
		}
		s.rng = rand.New(rand.NewSource(seed))
		want := expectedWeightedRune(rand.New(rand.NewSource(seed)).Intn(2100))
		s.rollAstralRuneLoot(s.entities[mid], 0, 0)
		if len(s.ground) != 1 {
			t.Fatalf("seed %d: guaranteed occurrence did not yield one rune", seed)
		}
		for _, e := range s.ground {
			st := e.Drop.Stack
			if st.Item != want || st.UID <= 0 || st.Count != 1 || st.InstanceKind != domain.ItemInstanceSocketCard || !st.Card.Initialized {
				t.Fatalf("seed %d: wrong weighted rune/instance, want %d got %+v", seed, want, st)
			}
		}
	}
}

func TestRiftRuneRewardsUsePinnedWeightsAndPersistExactGrants(t *testing.T) {
	loot := weightedSceneLootFixture(t)
	v1, v2, v3 := *riftRulesFixture(), *riftRulesFixture(), *riftRulesFixture()
	v2.Version, v2.FloorPopulationMultiplier = 2, 2
	v3.Version, v3.FloorPopulationMultiplier, v3.LootVersion = 3, 2, 2
	rules, err := domain.NewRiftRuleTable(3, []domain.RiftRules{v1, v2, v3}, loot, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int32{1, 2, 3} {
		for sample := 0; sample < 8; sample++ {
			s, b, now := riftFixture(t)
			s.lootRules, s.riftRules = loot, rules
			activateTestRift(t, s, b, *now)
			d := &s.rift.data
			d.Version = 2
			d.Run.RunID = fmt.Sprintf("rune-weights-%d-%d", version, sample)
			d.Run.RulesVersion = version
			d.Run.Rules, _ = rules.Version(version)
			d.Run.Timely = true
			d.Run.Phase = domain.RiftCompleted
			for char, m := range d.Run.Members {
				m.Eligible = true
				d.Run.Members[char] = m
			}
			// Isolate rune RNG: no equipment candidate means no earlier draws.
			for id, def := range s.items {
				if def.Equip != nil {
					delete(s.items, id)
				}
			}
			plan := domain.PlanRiftRewards(d.Run.Tier, true, d.Run.Rules)
			if want := domain.PlanRiftRewards(d.Run.Tier, true, riftRulesFixture()); plan != want {
				t.Fatal("weights changed reward quantities")
			}
			hash := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-reward/100/v2", d.Run.RunID)))
			rng := rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(hash[:8]))))
			s.generateEndgameRiftGrants()
			items := d.Grants[100].Items
			if !d.GrantsReady || len(items) != int(plan.Rune)+1 {
				t.Fatal("rune or dust reward count changed")
			}
			for i := 0; i < int(plan.Rune); i++ {
				var want domain.ItemID
				if version == 3 {
					want = expectedWeightedRune(rng.Intn(2100))
				} else {
					want = domain.FirstAstralRune + domain.ItemID(rng.Intn(28))
				}
				st := items[i]
				if st.Item != want || st.UID <= 0 || !st.Bound || st.Count != 1 || !st.Card.Initialized {
					t.Fatalf("version %d sample %d reward %d: want %d got %+v", version, sample, i, want, st)
				}
			}
			record := d.record(77)
			restored, ok := decodeEndgameRift(&record, rules)
			if !ok || !restored.GrantsReady || !reflect.DeepEqual(restored.Grants[100].Items, items) {
				t.Fatal("persist/reload altered already-generated rune instances")
			}
			s.stepEndgameRift()
			s.stepEndgameRift()
			if !reflect.DeepEqual(s.rift.data.Grants[100].Items, items) {
				t.Fatal("completion retry rerolled rune instances")
			}
		}
	}
}

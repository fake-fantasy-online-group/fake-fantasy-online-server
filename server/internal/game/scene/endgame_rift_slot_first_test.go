package scene

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func slotFirstRiftFixture(t *testing.T) (*Scene, *domain.RiftRules, *domain.RiftRules) {
	t.Helper()
	table, s := wingRiftRulesFixture(t)
	old := table.Active()
	enableRareFeather(s, old)
	fresh := *old
	fresh.Version = 15
	var err error
	s.riftRules, err = domain.NewRiftRuleTable(15, []domain.RiftRules{*riftRulesFixture(), *old, fresh}, s.lootRules, s.items)
	if err != nil {
		t.Fatal(err)
	}
	return s, s.riftRules.Active(), old
}

// Independent oracle: group the already-filtered native candidates, with both
// weapon encodings sharing one semantic part and no empty part in the draw.
func slotFirstOracle(s *Scene, pool []domain.ItemID, rng *lootRandomV5) domain.ItemID {
	groups := map[int32][]domain.ItemID{}
	for _, id := range pool {
		slot := s.items[id].Equip.Slot
		if slot == 13 {
			slot = 4
		}
		groups[slot] = append(groups[slot], id)
	}
	keys := make([]int, 0, len(groups))
	for slot := range groups {
		keys = append(keys, int(slot))
	}
	sort.Ints(keys)
	ids := groups[int32(keys[rng.Intn(len(keys))])]
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids[rng.Intn(len(ids))]
}

func TestRiftSlotFirstBothRewardPathsAndLegacyExactSelection(t *testing.T) {
	s, fresh, old := slotFirstRiftFixture(t)
	identity := endgameRiftIdentity{Race: 3, Gender: 1, Employed: true}
	recipient := identity.legendaryRecipientClass()
	for _, rules := range []*domain.RiftRules{old, fresh} {
		pool := s.riftCumulativeLegendaryPool(rules, 43, 43, 65, identity, recipient)
		if len(pool) < 10 {
			t.Fatal("native imbalanced fixture missing", pool)
		}
		prepareCumulativeRun(s, rules, "slot-first-exact-seeds", 150, true, 43)
		counts := [2]map[int32]int{{}, {}}
		const draws = 1800
		for i := int32(0); i < draws; i++ {
			for path := 0; path < 2; path++ {
				var rng lootRandomV5
				if path == 0 {
					h := sha256.Sum256([]byte(fmt.Sprintf("%s/rift-normal-boss/%d/%d/%d/item", s.rift.data.Run.RunID, rules.Version, 100, i)))
					rng = lootRandomV5{binary.BigEndian.Uint64(h[:8])}
				} else {
					rng = s.riftCumulativeRandom(100, i, "item")
				}
				var want domain.ItemID
				if rules.Version < 15 {
					want = pool[rng.Intn(len(pool))]
				} else {
					want = slotFirstOracle(s, pool, &rng)
				}
				roll := func(ids []domain.ItemID) domain.Stack {
					if path == 0 {
						return s.rollEndgameRiftNormalBossEquipment(100, i, ids, 100, recipient)
					}
					return s.rollEndgameRiftCumulativeLegendary(100, i, ids, recipient)
				}
				got := roll(pool)
				if got.Item != want {
					t.Fatalf("v%d path%d ordinal%d got%d want%d", rules.Version, path, i, got.Item, want)
				}
				// A singleton candidate consumes different selection draws but cannot alter
				// the independent quality/power/value streams, including historical rolls.
				if !reflect.DeepEqual(normalizeRiftStacks([]domain.Stack{got}), normalizeRiftStacks([]domain.Stack{roll([]domain.ItemID{want})})) {
					t.Fatal("selection perturbed quality or power RNG")
				}
				slot := s.items[got.Item].Equip.Slot
				if slot == 13 {
					slot = 4
				}
				counts[path][slot]++
			}
		}
		if rules.Version == 15 {
			for path, slots := range counts {
				mean := draws / len(slots)
				for slot, n := range slots {
					if n < mean*7/10 || n > mean*13/10 {
						t.Fatalf("path%d slot%d is not uniformly weighted: %v", path, slot, slots)
					}
				}
			}
		}
	}
}

func TestRiftSlotFirstMixedPartyCountsMFAndPersistedFreeze(t *testing.T) {
	s, rules, old := slotFirstRiftFixture(t)
	for _, pinned := range []*domain.RiftRules{old, rules} {
		prepareCumulativeRun(s, pinned, "slot-first-mixed-freeze", 150, true, 12, 20, 30, 43, 60)
		pending, err := json.Marshal(s.rift.data)
		if err != nil {
			t.Fatal(err)
		}
		var first map[domain.CharID]endgameRiftGrant
		for attempt := 0; attempt < 2; attempt++ {
			if err := json.Unmarshal(pending, &s.rift.data); err != nil {
				t.Fatal(err)
			}
			// Changing the live selector must not change a paid run's pinned algorithm.
			if err := s.rift.data.Run.BindRules(s.riftRules); err != nil {
				t.Fatal(err)
			}
			s.generateEndgameRiftGrantsWithContexts(nil)
			if !s.rift.data.GrantsReady {
				t.Fatal("mixed party grant not generated")
			}
			for char, grant := range s.rift.data.Grants {
				member, profile := s.rift.data.Run.Members[char], s.rift.data.Profiles[char]
				character := &domain.Character{Race: profile.Race, Employed: profile.Employed, EmploymentKnown: true}
				sex := uint8(2)
				if profile.Gender == 1 {
					sex = 1
				}
				gear, special := 0, 0
				for _, st := range grant.Items {
					def := s.items[st.Item]
					if def.Equip == nil {
						continue
					}
					gear++
					if gear <= 15 {
						special++
						if st.Legendary.PowerID == "" {
							t.Fatal("lost guaranteed legendary")
						}
					}
					if def.HardDungeonOnly || def.Equip.AscensionOnly || def.Equip.LevelReq > member.Level || !def.Equip.Need.AllowsCharacter(character) || def.Equip.Need.Sex != 0 && def.Equip.Need.Sex != sex {
						t.Fatalf("illegal wearer base char%d item%d", char, st.Item)
					}
				}
				if gear != 25 || special != 15 || wingGrantCount(grant) != 4 {
					t.Fatal("reward quantities changed", char, gear, special, wingGrantCount(grant))
				}
			}
			if attempt == 0 {
				first = s.rift.data.clone().Grants
			} else {
				for char, grant := range first {
					if !reflect.DeepEqual(normalizeRiftStacks(grant.Items), normalizeRiftStacks(s.rift.data.Grants[char].Items)) {
						t.Fatal("pending retry rerolled", char)
					}
				}
			}
			ready, _ := json.Marshal(s.rift.data)
			s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: 1, MagicFind: 999999}})
			after, _ := json.Marshal(s.rift.data)
			if string(ready) != string(after) {
				t.Fatal("ready grants changed")
			}
		}
	}
}

func TestRiftSlotFirstMFDoesNotChangeSpecialOrOtherRewards(t *testing.T) {
	s, rules, _ := slotFirstRiftFixture(t)
	var baseline []domain.Stack
	observedQualityChange := false
	for _, mf := range []int64{0, 1000000} {
		prepareCumulativeRun(s, rules, "slot-first-mf-streams", 150, true, 43)
		s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: 43, MagicFind: mf}})
		got := normalizeRiftStacks(s.rift.data.Grants[100].Items)
		if !s.rift.data.GrantsReady {
			t.Fatal("no grants")
		}
		if mf == 0 {
			baseline = got
			continue
		}
		if len(got) != len(baseline) {
			t.Fatal("MF changed reward count")
		}
		equipment := 0
		for i, st := range got {
			if st.Item != baseline[i].Item || st.Count != baseline[i].Count {
				t.Fatal("MF changed base or material/rune/feather count")
			}
			if s.items[st.Item].Equip == nil {
				if !reflect.DeepEqual(st, baseline[i]) {
					t.Fatal("MF changed non-equipment reward")
				}
				continue
			}
			equipment++
			if equipment <= 15 && !reflect.DeepEqual(st, baseline[i]) {
				t.Fatal("MF changed special legendary")
			}
			if equipment > 15 && !reflect.DeepEqual(st, baseline[i]) {
				observedQualityChange = true
			}
		}
	}
	if !observedQualityChange {
		t.Fatal("normal boss equipment stopped applying MF")
	}
}

package scene

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestLegendaryShoesNative42And43CandidatesAndNaturalDrops(t *testing.T) {
	s, _, rules, _ := cosmeticHatFixture(t)
	for _, level := range []int32{42, 43} {
		for _, gender := range []uint8{0, 1} {
			var shoes []domain.ItemID
			for _, id := range missingSlotLegalPool(s, rules, level, level, 2, true, gender) {
				if s.items[id].Equip.Slot == int32(domain.SlotShoe) {
					shoes = append(shoes, id)
				}
			}
			sort.Slice(shoes, func(i, j int) bool { return shoes[i] < shoes[j] })
			if !reflect.DeepEqual(shoes, []domain.ItemID{2105, 2106}) {
				t.Fatalf("level %d gender %d shoe pool changed: %v", level, gender, shoes)
			}
			for _, id := range shoes {
				def := s.items[id]
				if !s.riftLegendaryBaseEligible(def) {
					t.Fatalf("native shoe %d cannot become legendary", id)
				}
				categories := map[string]bool{}
				for _, p := range domain.LegendaryCandidates(def) {
					if p.Class != "全职业" {
						t.Fatalf("shoe %d admitted class/over-level power %s", id, p.ID)
					}
					categories[p.Category] = true
				}
				wantCategories := map[string]bool{"攻击": true, "防御": true, "通用": true}
				if !reflect.DeepEqual(categories, wantCategories) {
					t.Fatalf("shoe %d missing power categories: %v", id, categories)
				}
				observed := map[string]bool{}
				for seed := uint64(1); seed <= 128; seed++ {
					st := domain.NewStack(def, 1)
					quality, values := lootRandomV5{seed}, lootRandomV5{seed + 1}
					unique, variables := lootRandomV5{seed + 2}, lootRandomV5{seed + 3}
					// Force the existing independent promotion roll, not its base rate.
					loot := &domain.LootRuleSet{UniqueBasePPB: domain.LootProbabilityScaleV5, UniqueMFDivisor: 250}
					s.rollNaturalEquipmentQualityV5(def, &st, level, domain.DefaultColorProfile, true, 0, loot, &quality, &values, &unique, &variables)
					p, ok := domain.LegendaryPower(st.Legendary.PowerID)
					if !ok || p.Class != "全职业" || !domain.LegendaryPowerEligibleForItem(p, def, st) || st.Legendary.Validate() != nil {
						t.Fatalf("native shoe %d generated invalid legendary: %+v", id, st.Legendary)
					}
					if len(domain.LegendaryItemBonuses(def, st)) != domain.LegendaryAffixCount {
						t.Fatal("shoe lost six legendary affixes", id)
					}
					worn := domain.NewEquipSet()
					worn.Set(domain.SlotShoe, st)
					ch := &domain.Character{Race: 2, Employed: true, EmploymentKnown: true, Level: level}
					active := domain.ActiveLegendaryPowers(worn, s.itemDef, ch)
					if len(active) != 1 || active[0].Power.ID != p.ID {
						t.Fatal("legal shoe power did not activate", id, p.ID)
					}
					view := s.endgameItem(st, "equipped", int32(domain.SlotShoe))
					if view.Legendary == nil || len(view.Legendary.Affixes) != domain.LegendaryAffixCount {
						t.Fatal("shoe legendary presentation missing", id)
					}
					observed[p.Category] = true
				}
				if !reflect.DeepEqual(observed, wantCategories) {
					t.Fatal("natural drops did not produce every shoe power category", id, observed)
				}
			}
		}
	}
}

func TestLegendaryShoesRealRiftRewards(t *testing.T) {
	s, _, rules, _ := cosmeticHatFixture(t)
	for _, level := range []int32{42, 43} {
		for _, gender := range []uint8{0, 1} {
			observed := map[domain.ItemID]map[string]bool{2105: {}, 2106: {}}
			for seed := 0; seed < 256; seed++ {
				run := &domain.RiftRun{RunID: fmt.Sprintf("shoe-l%d-g%d-s%d", level, gender, seed), Tier: 10, Timely: true, RulesVersion: rules.Version, Rules: rules, Members: map[domain.CharID]*domain.RiftMember{100: {CharID: 100, Level: level, Eligible: true}}}
				s.rift = &endgameRiftState{data: endgameRiftPersisted{Version: 2, RewardSourceLevel: level, Run: run, Profiles: map[domain.CharID]endgameRiftIdentity{100: {Race: 2, Gender: gender, Employed: true, RewardLevel: level}}, Grants: map[domain.CharID]endgameRiftGrant{}}}
				contexts := map[domain.CharID]endgameRiftRewardContext{100: {Level: level}}
				s.generateEndgameRiftGrantsWithContexts(contexts)
				grant := s.rift.data.Grants[100]
				for _, st := range grant.Items {
					def := s.items[st.Item]
					if def.Equip == nil || def.Equip.Slot != int32(domain.SlotShoe) || st.Legendary.PowerID == "" {
						continue
					}
					p, ok := domain.LegendaryPower(st.Legendary.PowerID)
					if !ok || p.Class != "全职业" || !domain.LegendaryPowerEligibleForItem(p, def, st) || st.Legendary.Validate() != nil || observed[st.Item] == nil {
						t.Fatal("invalid legendary shoe reward", st.Item, st.Legendary.PowerID)
					}
					observed[st.Item][p.Category] = true
				}
				// An already materialized reward remains immutable on retry.
				s.generateEndgameRiftGrantsWithContexts(contexts)
				if !reflect.DeepEqual(grant, s.rift.data.Grants[100]) {
					t.Fatal("ready grants changed on retry")
				}
			}
			for id, categories := range observed {
				if !reflect.DeepEqual(categories, map[string]bool{"攻击": true, "防御": true, "通用": true}) {
					t.Fatalf("level %d gender %d shoe %d missing real reward categories: %v", level, gender, id, categories)
				}
			}
		}
	}
}

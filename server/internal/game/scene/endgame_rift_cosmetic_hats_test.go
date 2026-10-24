package scene

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Native seeds: migration82 source/drop relation and migration39 affix weights.
// The fixture exports fields consumed by pool eligibility/quality generation.
func cosmeticHatFixture(t *testing.T) (*Scene, *domain.RiftRules, *domain.RiftRules, map[domain.ItemID]bool) {
	t.Helper()
	var f struct {
		Items         []domain.ItemDef
		Pools         []domain.RiftEquipmentPool
		SlotFallbacks []domain.ItemID
		Options       []struct {
			Level, Type, CardID int32
			Affix               domain.Affix
			Weight              int64
		}
	}
	read := func(path string, dst any) {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(raw, dst); e != nil {
			t.Fatal(e)
		}
	}
	read("testdata/rift_native_equipment_pools.json", &f)
	if len(f.Items) != 318 || len(f.Pools) != 60 || len(f.Options) != 15458 {
		t.Fatal("native fixture coverage changed")
	}
	raw, e := os.ReadFile("../../../migrations/000000_init-105.sql")
	if e != nil {
		t.Fatal(e)
	}
	parts := strings.Split(string(raw), "$excluded$")
	if len(parts) != 3 {
		t.Fatal("invalid exclusion manifest")
	}
	var manifest []struct {
		ID   domain.ItemID
		Name string
		Sex  uint8
	}
	if e = json.Unmarshal([]byte(parts[1]), &manifest); e != nil {
		t.Fatal(e)
	}
	excluded := map[domain.ItemID]bool{}
	for _, x := range manifest {
		excluded[x.ID] = true
	}
	s, _, _ := riftFixture(t)
	s.equipmentRoll = domain.EquipmentRollTable{Counts: map[domain.ColorProfile][]domain.WeightedAffixCount{domain.DefaultColorProfile: {{Count: 0, Weight: 60}, {Count: 1, Weight: 20}, {Count: 2, Weight: 10}, {Count: 3, Weight: 5}, {Count: 4, Weight: 3}, {Count: 5, Weight: 1}}}, Options: map[domain.EquipmentRollKey][]domain.EquipmentRollOption{}}
	for _, o := range f.Options {
		k := domain.EquipmentRollKey{Level: o.Level, Type: o.Type}
		s.equipmentRoll.Options[k] = append(s.equipmentRoll.Options[k], domain.EquipmentRollOption{CardID: o.CardID, Affix: o.Affix, Weight: o.Weight})
	}
	for _, d := range f.Items {
		if d.Equip != nil && d.Equip.NativeDropEligible && !d.Equip.NoTypeDrop && s.equipmentRoll.LegendaryReady(d) {
			d.LegendaryEnabled = domain.LegendaryCatalogComplete()
		}
		s.items[d.ID] = d
	}
	for _, x := range manifest {
		d := s.items[x.ID]
		if d.Name != x.Name || d.Equip.LevelReq != 1 || d.Equip.ResourceLevel != 5 || d.Equip.Slot != 2 || d.Equip.Need.Sex != x.Sex {
			t.Fatal("reviewed identity mismatch", x)
		}
	}
	old := riftEliteRulesFixture(t)
	old.Version = 10
	old.FloorLayoutVersion = domain.RiftFloorFullCoverageVersion
	old.GuardianRecentWindow = 3
	old.NormalBossEquipment = 10
	old.EquipmentPools = f.Pools
	raw, _ = json.Marshal(old)
	var fresh domain.RiftRules
	if e = json.Unmarshal(raw, &fresh); e != nil {
		t.Fatal(e)
	}
	fresh.Version = 11
	fresh.EquipmentSlotFallbacks = f.SlotFallbacks
	for i := range fresh.EquipmentPools {
		var kept []domain.ItemID
		for _, id := range fresh.EquipmentPools[i].Items {
			if !excluded[id] {
				kept = append(kept, id)
			}
		}
		if len(kept) == 0 {
			t.Fatal("empty tier", fresh.EquipmentPools[i].MonsterLevel)
		}
		fresh.EquipmentPools[i].Items = kept
	}
	s.riftRules, e = domain.NewRiftRuleTable(11, []domain.RiftRules{*riftRulesFixture(), *old, fresh}, lootRulesTableFixture(), s.items)
	if e != nil {
		t.Fatal(e)
	}
	return s, old, &fresh, excluded
}
func cosmeticLegalPool(s *Scene, r *domain.RiftRules, level int32, gender uint8) []domain.ItemID {
	ch := &domain.Character{Race: 2, Employed: true, EmploymentKnown: true}
	return r.EquipmentPoolForLevel(level, func(id domain.ItemID) bool {
		d := s.items[id]
		return d.Equip != nil && !d.HardDungeonOnly && !d.Equip.AscensionOnly && d.Equip.LevelReq <= level && d.Equip.LevelReq <= 60 && d.Equip.Need.AllowsCharacter(ch) && (d.Equip.Need.Sex == 0 || d.Equip.Need.Sex == gender)
	})
}
func TestRiftCosmeticHatsNativePoolsAndFrozenVersion(t *testing.T) {
	s, old, fresh, excluded := cosmeticHatFixture(t)
	for i, p := range old.EquipmentPools {
		got := fresh.EquipmentPools[i]
		if got.MonsterLevel != p.MonsterLevel || !reflect.DeepEqual(got.SourceMonsters, p.SourceMonsters) {
			t.Fatal("source/tier changed")
		}
		var want []domain.ItemID
		for _, id := range p.Items {
			if !excluded[id] {
				want = append(want, id)
			}
		}
		if !reflect.DeepEqual(got.Items, want) {
			t.Fatal("ordinary gear or order changed")
		}
	}
	for level := int32(1); level <= 65; level++ {
		for _, gender := range []uint8{1, 2} {
			before, after := cosmeticLegalPool(s, old, level, gender), cosmeticLegalPool(s, fresh, level, gender)
			var want []domain.ItemID
			for _, id := range before {
				if !excluded[id] {
					want = append(want, id)
				}
			}
			if !reflect.DeepEqual(after, want) {
				t.Fatalf("level=%d gender=%d candidate drift: %v => %v want %v", level, gender, before, after, want)
			}
			if level == 42 {
				if len(before) != 21 || len(after) != 13 {
					t.Fatalf("42 candidates=%d/%d", len(before), len(after))
				}
				n := 0
				for _, id := range after {
					if s.riftLegendaryBaseEligible(s.items[id]) {
						n++
					}
				}
				if n != 11 {
					t.Fatal("legendary base pool changed", n)
				}
			}
		}
	}
	for _, id := range []domain.ItemID{1001, 1301, 1501, 1601, 1801, 1903, 2002} {
		found := false
		for _, p := range fresh.EquipmentPools {
			for _, item := range p.Items {
				found = found || item == id
			}
		}
		if !found || s.items[id].Equip.LevelReq != 1 {
			t.Fatal("legitimate level-one equipment lost", id)
		}
	}
	for _, r := range []*domain.RiftRules{old, fresh} {
		raw, _ := json.Marshal(&domain.RiftRun{RulesVersion: r.Version, Rules: r})
		var saved domain.RiftRun
		_ = json.Unmarshal(raw, &saved)
		if e := saved.BindRules(s.riftRules); e != nil || !reflect.DeepEqual(saved.Rules, r) {
			t.Fatal("frozen run changed", r.Version, e)
		}
	}
	active := s.riftRules.Active()
	active.EquipmentPools[0].Items[0] = 999999
	if s.riftRules.Active().EquipmentPools[0].Items[0] == 999999 {
		t.Fatal("mutable published pool")
	}
}
func TestRiftCosmeticHatsRealRewardGenerator(t *testing.T) {
	s, old, fresh, excluded := cosmeticHatFixture(t)
	beforeItems, _ := json.Marshal(s.items)
	legacyHats := map[domain.ItemID]bool{}
	legendarySlots := map[int32]bool{}
	generated := 0
	for _, rules := range []*domain.RiftRules{old, fresh} {
		for _, level := range []int32{1, 12, 24, 35, 42, 60, 65} {
			for _, gender := range []uint8{1, 2} {
				for _, tier := range []int32{1, 10} {
					legal := map[domain.ItemID]bool{}
					for _, id := range missingSlotLegalPool(s, rules, level, level, 2, true, gender) {
						legal[id] = true
					}
					plan := domain.PlanRiftRewards(tier, true, rules)
					for seed := 0; seed < 64; seed++ {
						run := &domain.RiftRun{RunID: fmt.Sprintf("cosmetic-l%d-g%d-t%d-s%d", level, gender, tier, seed), Tier: tier, Timely: true, RulesVersion: rules.Version, Rules: rules, Members: map[domain.CharID]*domain.RiftMember{100: {CharID: 100, Level: level, Eligible: true}}}
						s.rift = &endgameRiftState{data: endgameRiftPersisted{Version: 2, Run: run, Profiles: map[domain.CharID]endgameRiftIdentity{100: {Race: 2, Gender: gender, Employed: true, RewardLevel: level}}, Grants: map[domain.CharID]endgameRiftGrant{}}}
						s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: level}})
						count := 0
						for _, stack := range s.rift.data.Grants[100].Items {
							d := s.items[stack.Item]
							if d.Equip == nil {
								continue
							}
							count++
							if !legal[stack.Item] {
								t.Fatal("reward escaped candidate pool", rules.Version, level, stack.Item)
							}
							if excluded[stack.Item] {
								if rules.Version == 11 {
									t.Fatal("new cosmetic reward", stack.Item)
								}
								legacyHats[stack.Item] = true
							}
							if rules.Version == 11 && level == 42 && stack.Legendary.PowerID != "" {
								legendarySlots[d.Equip.Slot] = true
							}
						}
						if count != int(plan.Equipment+plan.NormalBossEquipment) {
							t.Fatalf("quantity changed v%d l%d: got %d want %+v", rules.Version, level, count, plan)
						}
						ready := s.rift.data.clone().Grants
						s.generateEndgameRiftGrants()
						if !reflect.DeepEqual(ready, s.rift.data.Grants) {
							t.Fatal("ready rewards rerolled")
						}
						generated++
					}
				}
			}
		}
	}
	if len(legacyHats) != 12 {
		t.Fatal("legacy pool no longer produces every hat", legacyHats)
	}
	for _, slot := range []int32{2, 3, 6, 7, 8, 13} {
		if !legendarySlots[slot] {
			t.Fatal("42 legendary slot not observed", slot, legendarySlots)
		}
	}
	afterItems, _ := json.Marshal(s.items)
	if string(beforeItems) != string(afterItems) {
		t.Fatal("equipment mutated")
	}
	t.Logf("%d real settlements; all12 hats retained in frozen v10 and absent in v11; legendary slots=%v", generated, legendarySlots)
}

package scene

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func missingSlotLegalPool(s *Scene, r *domain.RiftRules, poolLevel, wearLevel int32, race domain.Race, employed bool, gender uint8) []domain.ItemID {
	ch := &domain.Character{Race: race, Employed: employed, EmploymentKnown: true}
	sex := uint8(2)
	if gender == 1 {
		sex = 1
	}
	eligible := func(id domain.ItemID) bool {
		d := s.items[id]
		return d.Equip != nil && !d.HardDungeonOnly && !d.Equip.AscensionOnly && d.Equip.LevelReq <= wearLevel && d.Equip.LevelReq <= 60 && d.Equip.Need.AllowsCharacter(ch) && (d.Equip.Need.Sex == 0 || d.Equip.Need.Sex == sex)
	}
	return r.FillMissingEquipmentSlots(r.EquipmentPoolForLevel(poolLevel, eligible), wearLevel, s.items, eligible)
}

func TestRiftSlotFallbackNativeLevelsClassesAndBoundaries(t *testing.T) {
	s, old, fresh, _ := cosmeticHatFixture(t)
	slots := func(ids []domain.ItemID, slot int32) []domain.ItemID {
		var out []domain.ItemID
		for _, id := range ids {
			if s.items[id].Equip.Slot == slot {
				out = append(out, id)
			}
		}
		return out
	}
	expected := func(level int32, slot int32) []domain.ItemID {
		if level < 6 {
			return nil
		}
		if slot == 1 {
			if level < 12 {
				return []domain.ItemID{2701}
			}
			if level < 18 {
				return []domain.ItemID{2702}
			}
			return []domain.ItemID{2703, 2704, 2705, 2706, 2707}
		}
		switch {
		case level < 12:
			return []domain.ItemID{2605}
		case level < 18:
			return []domain.ItemID{2609}
		case level < 24:
			return []domain.ItemID{2606, 2608}
		case level < 30:
			return []domain.ItemID{2604, 2610}
		case level < 36:
			return []domain.ItemID{2601, 2607}
		default:
			return []domain.ItemID{2602, 2603}
		}
	}
	checked := 0
	for level := int32(1); level <= 65; level++ {
		for _, poolLevel := range []int32{level, 65} { // Mixed parties use encounter tier65, each recipient keeps their wear gate.
			for race := domain.Race(0); race < 5; race++ {
				for _, employed := range []bool{false, true} {
					for _, gender := range []uint8{0, 1} {
						rawRules := *fresh
						rawRules.EquipmentSlotFallbacks = nil
						before := missingSlotLegalPool(s, &rawRules, poolLevel, level, race, employed, gender)
						got := missingSlotLegalPool(s, fresh, poolLevel, level, race, employed, gender)
						want := append([]domain.ItemID(nil), before...)
						for _, slot := range []int32{1, 10} {
							if len(slots(before, slot)) == 0 {
								want = append(want, expected(level, slot)...)
							}
						}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("level%d pool%d race%d employed%v gender%d got%v want%v", level, poolLevel, race, employed, gender, got, want)
						}
						legacy := missingSlotLegalPool(s, old, poolLevel, level, race, employed, gender)
						if len(legacy) == 0 {
							t.Fatal("native fixture lost baseline")
						}
						checked++
					}
				}
			}
		}
	}
	for _, level := range []int32{42, 43} {
		got := missingSlotLegalPool(s, fresh, level, level, 2, true, 0)
		if len(got) != 20 || !reflect.DeepEqual(slots(got, 1), expected(level, 1)) || !reflect.DeepEqual(slots(got, 10), expected(level, 10)) {
			t.Fatal("42/43 fallback mismatch", level, got)
		}
		eligible := 0
		for _, id := range got {
			if s.riftLegendaryBaseEligible(s.items[id]) {
				eligible++
			}
		}
		if eligible != 20 {
			t.Fatal("42/43 legendary bases", level, eligible)
		}
		powerIDs := func(pool []domain.ItemID) map[string]bool {
			out := map[string]bool{}
			for _, id := range pool {
				for _, p := range domain.LegendaryCandidates(s.items[id]) {
					out[p.ID] = true
				}
			}
			return out
		}
		beforePowers := powerIDs(missingSlotLegalPool(s, old, level, level, 2, true, 0))
		afterPowers := powerIDs(got)
		if !reflect.DeepEqual(beforePowers, afterPowers) || len(afterPowers) != 117 {
			t.Fatal("42/43 power gates unexpectedly expanded", len(beforePowers), len(afterPowers))
		}
	}
	t.Logf("%d native candidate scenarios across levels1..65, all5 classes, beginner/employed, both genders and mixed parties", checked)
}

func TestRiftSlotFallbackRealRewardsAndLegendaryGates(t *testing.T) {
	s, old, fresh, _ := cosmeticHatFixture(t)
	observed := map[domain.ItemID]bool{}
	powers := map[int32]map[string]bool{1: {}, 10: {}}
	aboveWear := map[int32]bool{}
	for _, level := range []int32{42, 43} {
		for _, rules := range []*domain.RiftRules{old, fresh} {
			for seed := 0; seed < 256; seed++ {
				run := &domain.RiftRun{RunID: fmt.Sprintf("slot-l%d-v%d-s%d", level, rules.Version, seed), Tier: 10, Timely: true, RulesVersion: rules.Version, Rules: rules, Members: map[domain.CharID]*domain.RiftMember{100: {CharID: 100, Level: level, Eligible: true}}}
				s.rift = &endgameRiftState{data: endgameRiftPersisted{Version: 2, RewardSourceLevel: level, Run: run, Profiles: map[domain.CharID]endgameRiftIdentity{100: {Race: 2, Gender: 0, Employed: true, RewardLevel: level}}, Grants: map[domain.CharID]endgameRiftGrant{}}}
				s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: level}})
				for _, st := range s.rift.data.Grants[100].Items {
					d := s.items[st.Item]
					if d.Equip == nil || (d.Equip.Slot != 1 && d.Equip.Slot != 10) {
						continue
					}
					if rules.Version == 10 {
						t.Fatal("old42/43 run acquired a fallback", st.Item)
					}
					observed[st.Item] = true
					if d.Equip.Slot == 1 && d.Equip.LevelReq != 18 || d.Equip.Slot == 10 && d.Equip.LevelReq != 36 {
						t.Fatal("wear level changed", st.Item)
					}
					if st.Legendary.PowerID != "" {
						p, ok := domain.LegendaryPower(st.Legendary.PowerID)
						if !ok || st.Endgame.SourceLevel != level || !domain.LegendaryPowerSourceLevelEligible(p, level) || !domain.LegendaryPowerEligibleForItem(p, d, st) {
							t.Fatal("ineligible legendary", st.Item, st.Legendary.PowerID)
						}
						if err := st.Legendary.Validate(); err != nil {
							t.Fatal(err)
						}
						powers[d.Equip.Slot][p.ID] = true
						aboveWear[d.Equip.Slot] = aboveWear[d.Equip.Slot] || p.MinLevel > d.Equip.LevelReq
					}
				}
			}
		}
	}
	for _, id := range []domain.ItemID{2602, 2603, 2703, 2704, 2705, 2706, 2707} {
		if !observed[id] || !s.riftLegendaryBaseEligible(s.items[id]) {
			t.Fatal("new base not generated/legendary eligible", id)
		}
		counts := len(domain.LegendaryCandidates(s.items[id]))
		if counts == 0 {
			t.Fatal("no powers", id)
		}
		t.Logf("base%d wear%d resource%d legendary candidates%d", id, s.items[id].Equip.LevelReq, s.items[id].Equip.ResourceLevel, counts)
	}
	if !aboveWear[1] || !aboveWear[10] {
		t.Fatal("real low-wear face/backpack missing higher monster powers", aboveWear)
	}
	if len(observed) != 7 || len(powers[1]) == 0 || len(powers[10]) == 0 {
		t.Fatal("fallback slot quality not observed", observed, powers)
	}
	t.Logf("1,024 real settlements; legendary powers observed face=%d backpack=%d", len(powers[1]), len(powers[10]))
}

func TestRiftSlotFallbackMixedPartyRecipientGates(t *testing.T) {
	s, _, fresh, _ := cosmeticHatFixture(t)
	// Native catalog currently allows both genders/all classes. Counterfactual
	// restrictions verify filtering happens BEFORE selecting the fallback tier.
	for _, id := range []domain.ItemID{2602, 2603} {
		d := s.items[id]
		e := *d.Equip
		d.Equip = &e
		d.Equip.Need.Professions = 1 // Warrior, not assassin.
		d.Equip.Need.BeginnerAllowed = false
		d.Equip.Need.Sex = 1
		s.items[id] = d
	}
	for _, tc := range []struct {
		race     domain.Race
		employed bool
		gender   uint8
		want     []domain.ItemID
	}{
		{2, true, 0, []domain.ItemID{2601, 2607}},
		{0, true, 0, []domain.ItemID{2601, 2607}},
		{0, true, 1, []domain.ItemID{2602, 2603}},
		{0, false, 1, []domain.ItemID{2601, 2607}},
	} {
		got := missingSlotLegalPool(s, fresh, 43, 43, tc.race, tc.employed, tc.gender)
		var bags []domain.ItemID
		for _, id := range got {
			if s.items[id].Equip.Slot == 10 {
				bags = append(bags, id)
			}
		}
		if !reflect.DeepEqual(bags, tc.want) {
			t.Fatal("recipient gates ignored", tc, bags)
		}
	}
	for seed := 0; seed < 64; seed++ {
		members := map[domain.CharID]*domain.RiftMember{100: {CharID: 100, Level: 17, Eligible: true}, 200: {CharID: 200, Level: 65, Eligible: true}}
		profiles := map[domain.CharID]endgameRiftIdentity{100: {Race: 2, Gender: 0, Employed: true}, 200: {Race: 0, Gender: 1, Employed: true}}
		run := &domain.RiftRun{RunID: fmt.Sprintf("mixed-slots-%d", seed), Tier: 10, Timely: true, RulesVersion: 11, Rules: fresh, Members: members}
		s.rift = &endgameRiftState{data: endgameRiftPersisted{Version: 2, RewardSourceLevel: 68, Run: run, Profiles: profiles, Grants: map[domain.CharID]endgameRiftGrant{}}}
		s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: 43}, 200: {Level: 65}})
		for char, grant := range s.rift.data.Grants {
			profile := profiles[char]
			legal := map[domain.ItemID]bool{}
			for _, id := range missingSlotLegalPool(s, fresh, 65, members[char].Level, profile.Race, profile.Employed, profile.Gender) {
				legal[id] = true
			}
			for _, st := range grant.Items {
				if s.items[st.Item].Equip != nil && !legal[st.Item] {
					t.Fatal("party/completion level leaked into wearer gate", char, st.Item)
				}
			}
			if len(grant.Items) == 0 {
				t.Fatal("lost mixed party rewards", char)
			}
		}
	}
}

package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
)

// This is an offline catalog test. It never loads a character or connects to a
// database. All new stacks are temporary values owned by this test process.
// Original-source base fixtures and production constructors/validators are
// intentionally reused rather than inventing equipment definitions.
func TestV6ShowcaseThreeCatalogLoadouts(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/daoist-endgame/showcase/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan showcasePlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if len(UniqueCatalog(testEndgameCatalog())) != 259 || len(AstralRecipes(testEndgameCatalog())) != 40 || len(plan.Loadouts) != 3 || plan.GrantCount != 27 {
		t.Fatal("catalog or loadout coverage changed")
	}
	bases, _, _ := cardBudgetLoad(t)
	wantRecipes := map[int64][]int32{42: {36, 38}, 183: {28, 36}, 206: {35, 36, 40}}
	wantClass := map[int64]Race{42: Assassin, 183: Warlock, 206: Healer}
	type reportRow struct {
		CharacterID                 int64
		Class                       string
		Gender                      uint8
		ItemCount                   int
		FormationIDs                []int32
		FixtureNakedBase            Base
		FixtureComputedBase         Base
		FixturePanel                Stats
		FixtureRemainingAllocatedTo string
		Skills                      map[SkillID]string
		Items                       []map[string]any
	}
	var report []reportRow
	for _, set := range plan.Loadouts {
		if race, ok := wantClass[set.CharacterID]; !ok || race != set.Race {
			t.Fatal("unexpected target/class", set.CharacterID, set.Race)
		}
		for gender := uint8(0); gender < 2; gender++ {
			ch := &Character{Race: set.Race, Level: 60, EmploymentKnown: true, Employed: true}
			defs := map[ItemID]ItemDef{}
			// Fixture icon is irrelevant to the authoritative rune effects. Actual
			// grants must use the locally loaded rune ItemDef, as the plan states.
			for _, r := range AstralRuneItems(7) {
				defs[r.ID] = r
			}
			lookup := func(id ItemID) (ItemDef, bool) { d, ok := defs[id]; return d, ok }
			worn, need := NewEquipSet(), (Base{})
			var bonuses []SkillRankBonus
			seenUID := map[int64]bool{}
			row := reportRow{CharacterID: set.CharacterID, Class: set.Class, Gender: gender, ItemCount: len(set.Items), Skills: map[SkillID]string{}}
			if len(set.Items) != 9 {
				t.Fatal("loadout must contain nine items")
			}
			for _, item := range set.Items {
				var id ItemID
				for _, candidate := range item.Candidates {
					if candidate.Sex == 0 || candidate.Sex == 1 && gender == 1 || candidate.Sex == 2 && gender == 0 {
						if id != 0 {
							t.Fatal("ambiguous gender candidate")
						}
						id = candidate.BaseID
					}
				}
				if id == 0 || item.Version != UniqueContentVersion {
					t.Fatal("missing gender candidate or wrong version")
				}
				source, ok := bases[id]
				if !ok {
					t.Fatal("missing original-source base", id)
				}
				def := cardBudgetDefinition(source)
				defs[id] = def
				if !source.Native || source.Level > 60 || !def.Equip.Need.AllowsCharacter(ch) || EquipSlot(def.Equip.Slot) != item.Slot {
					t.Fatal("base level/class/slot illegal", id)
				}
				n := def.Equip.Need.Base
				need = Base{STR: max(need.STR, n.STR), VIT: max(need.VIT, n.VIT), INT: max(need.INT, n.INT),
					SPI: max(need.SPI, n.SPI), AGI: max(need.AGI, n.AGI), DEX: max(need.DEX, n.DEX)}
				if def.Equip.Need.Meet(60, Base{STR: 100000, VIT: 100000, INT: 100000, SPI: 100000, AGI: 100000, DEX: 100000}, gender) != RejectNone {
					t.Fatal("invalid sex variant", id)
				}
				st := NewStack(def, 1)
				if st.UID <= 0 || seenUID[st.UID] {
					t.Fatal("invalid new equipment UID")
				}
				seenUID[st.UID] = true
				var affixes []Affix
				var itemBonuses []SkillRankBonus
				switch item.Kind {
				case "unique":
					if item.UniqueID != int32(id) || item.FormationID != 0 || item.UniqueRoll != 12000 || item.SocketCount != 0 || len(item.SocketRows) != 0 {
						t.Fatal("invalid unique plan metadata", id)
					}
					if !ApplyUnique(&st, def, func(n int) int { return n - 1 }) {
						t.Fatal("ApplyUnique failed", id)
					}
					if st.Endgame.UniqueRollBP != 12000 || st.Endgame.UniqueContentVersion != 6 {
						t.Fatal("unique roll/version")
					}
					d, ok := UniqueOf(st, testEndgameCatalog())
					if !ok || d.Name != item.Name {
						t.Fatal("unique resolution/name mismatch", id)
					}
					affixes, itemBonuses = UniqueItemBonuses(def, st), d.SkillBonuses
					if _, ok := MatchAstralRecipe(def, st); ok {
						t.Fatal("unique illegally formed an array")
					}
				case "formation":
					r, ok := AstralRecipeByID(item.FormationID, testEndgameCatalog())
					if !ok || item.UniqueID != 0 || item.FormationRoll != 12000 || item.SocketCount != len(r.Runes) || len(item.SocketRows) != len(r.Runes) {
						t.Fatal("invalid formation plan metadata", id)
					}
					if !AstralBaseEligible(def, st) || EquipmentSocketCapacity(def, st) < len(r.Runes) {
						t.Fatal("ineligible white base", id)
					}
					if r.Professions != 0 && r.Professions&(1<<uint8(set.Race)) == 0 {
						t.Fatal("wrong recipe class")
					}
					st.Endgame.FormationRollBP, st.Endgame.FormationContentVersion = 12000, 6
					st.SocketCount = uint8(len(r.Runes))
					for i, runeID := range r.Runes {
						if item.SocketRows[i].Index != i || item.SocketRows[i].Rune != runeID {
							t.Fatal("manifest rune order differs from production recipe")
						}
						rd, ok := lookup(runeID)
						if !ok {
							t.Fatal("missing rune")
						}
						card := NewStack(rd, 1)
						if card.UID <= 0 || seenUID[card.UID] || !card.Card.Initialized {
							t.Fatal("invalid rune identity/snapshot")
						}
						seenUID[card.UID] = true
						st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = runeID, card.UID, card.Card
					}
					if got, ok := MatchAstralRecipe(def, st); !ok || got.ID != r.ID {
						t.Fatal("production recipe match failed", id, r.ID)
					}
					bad := st
					bad.Sockets[0], bad.Sockets[1] = bad.Sockets[1], bad.Sockets[0]
					if _, ok := MatchAstralRecipe(def, bad); ok {
						t.Fatal("wrong rune order accepted")
					}
					affixes, itemBonuses = AstralItemBonuses(def, st, r), r.SkillBonuses
				default:
					t.Fatal("unsupported item kind", item.Kind)
				}
				if EquipmentRequiredLevel(def, st) > 60 {
					t.Fatal("rune/formation requirement exceeds level 60")
				}
				for i, bonus := range itemBonuses {
					bonus.SourceID = fmt.Sprintf("showcase:%d:%d:%d", set.CharacterID, item.Slot, i)
					bonuses = append(bonuses, bonus)
				}
				if !worn.At(item.Slot).Empty() {
					t.Fatal("duplicate equipment slot")
				}
				worn.Set(item.Slot, st)
				row.Items = append(row.Items, map[string]any{"slot": item.Slot, "base_id": id, "name": item.Name, "kind": item.Kind,
					"required_level": EquipmentRequiredLevel(def, st), "requirements": def.Equip.Need,
					"resolved_catalog_affixes_including_item_local": affixes, "socket_count": st.SocketCount, "version": 6, "roll_bp": 12000})
			}
			if !worn.At(SlotWeapon).Empty() || !worn.At(SlotShield).Empty() || worn.At(SlotTwoHand).Empty() {
				t.Fatal("weapon/offhand conflict")
			}
			active := ActiveAstralRecipes(worn, lookup, ch)
			if len(active) != len(wantRecipes[set.CharacterID]) {
				t.Fatalf("want one formation per channel, got %v", active)
			}
			for _, a := range active {
				row.FormationIDs = append(row.FormationIDs, a.Recipe.ID)
			}
			sort.Slice(row.FormationIDs, func(i, j int) bool { return row.FormationIDs[i] < row.FormationIDs[j] })
			if !reflect.DeepEqual(row.FormationIDs, wantRecipes[set.CharacterID]) {
				t.Fatal("wrong active recipe set", row.FormationIDs)
			}
			base := StartingBase(set.Race)
			for level := int32(2); level <= 60; level++ {
				base = base.Add(GrowthAt(set.Race, level))
			}
			points := int32(FreePointsPerLevel * 59)
			for _, field := range []struct {
				actual *int32
				need   int32
			}{{&base.STR, need.STR}, {&base.VIT, need.VIT}, {&base.INT, need.INT}, {&base.SPI, need.SPI}, {&base.AGI, need.AGI}, {&base.DEX, need.DEX}} {
				delta := max(int32(0), field.need-*field.actual)
				*field.actual += delta
				points -= delta
			}
			if points < 0 {
				t.Fatal("cannot meet full-set requirements with legal level60 free points")
			}
			switch set.Race {
			case Assassin:
				base.STR += points
				row.FixtureRemainingAllocatedTo = "STR"
			case Healer:
				base.SPI += points
				row.FixtureRemainingAllocatedTo = "SPI"
			case Warlock:
				base.INT += points
				row.FixtureRemainingAllocatedTo = "INT"
			}
			worn.Each(func(_ EquipSlot, st Stack) {
				d, _ := lookup(st.Item)
				if d.Equip.Need.Meet(60, base, gender) != RejectNone {
					t.Fatal("synthetic full-set requirement check failed", st.Item)
				}
			})
			row.FixtureNakedBase = base
			row.FixtureComputedBase, row.FixturePanel = Compute(StatSource{Base: base, Level: 60, Innate: Stats{MoveSpeed: 100},
				Worn: worn, Defs: lookup, Passive: AstralBonuses(worn, lookup, ch)})
			hard := Learned{}
			for _, id := range V5SkillIDs(testSkillProgressionCatalog()) {
				d, _ := V5SkillIdentity(id, testSkillProgressionCatalog())
				if d.Profession == int32(set.Race)+1 {
					hard[id] = 1 // Synthetic rank-resolution probe, not a real character.
				}
			}
			resolved, err := ResolveSkillLoadout(int32(set.Race)+1, hard, bonuses, nil, testSkillProgressionCatalog())
			if err != nil {
				t.Fatal(err)
			}
			for id, r := range resolved {
				if r.SoftRank != "0" {
					row.Skills[id] = r.SoftRank
				}
			}
			if set.Race == Healer && (row.Skills[10301] != "7" || row.Skills[10304] != "5" || row.Skills[10319] != "2") {
				t.Fatal("healer rank package changed", row.Skills)
			}
			if set.Race == Warlock && (row.Skills[10409] != "2" || row.Skills[10412] != "2" || row.Skills[10401] != "7") {
				t.Fatal("warlock rank package changed", row.Skills)
			}
			if set.Race == Assassin && (row.Skills[10219] != "5" || len(row.Skills) != 1) {
				t.Fatal("unexpected assassin skill ranks")
			}
			report = append(report, row)
		}
	}
	if path := os.Getenv("SHOWCASE_V6_VALIDATION_REPORT"); path != "" {
		payload := map[string]any{"status": "passed", "content_version": 6, "catalog_unique_count": 259, "catalog_formation_count": 40,
			"loadout_variants_tested": len(report), "items_validated": 54, "database_access": false, "item_grant_executed": false,
			"method": "Production domain test and original-source base fixtures",
			"limits": []string{"Both gender branches are tested; actual character gender, attributes, skills and inventory were not read.",
				"Fixture panels use legal level60 natural growth and free-point allocation; these are NOT current test001 character panels.",
				"No random yellow affixes, non-rune cards, refinement or fusion. Full-combat DPS/HPS optimality is not claimed.",
				"Actual local ItemDef, role identity and backpack state must be revalidated before any later authorized grant."}, "variants": report}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("Validated 6 class/gender variants, 54 equipment instances; no database access or grants")
}

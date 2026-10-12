package scene

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func v5PacketJSON(t *testing.T, path string, target any) {
	t.Helper()
	blob, err := os.ReadFile("../../../../docs/daoist-endgame/" + path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(blob, target); err != nil {
		t.Fatal(err)
	}
}
func v5PacketActualSkills(t *testing.T) (domain.SkillTable, domain.StatusTable) {
	t.Helper()
	var original []domain.SkillDef
	v5PacketJSON(t, "v5/skill-original-ranks.json", &original)
	if len(original) != 149*5 {
		t.Fatalf("original fixture rows = %d, want 745", len(original))
	}
	skills := domain.SkillTable{}
	for _, d := range original {
		skills[domain.SkillKey{ID: d.ID, Level: d.Level}] = d
	}
	if err := domain.InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	var defs []domain.StatusDef
	v5PacketJSON(t, "v5/skill-status-anchors.json", &defs)
	statuses := domain.StatusTable{}
	for _, d := range defs {
		statuses[domain.StatusKey{ID: d.ID, Level: d.Level}] = d
	}
	return skills, statuses
}
func v5PacketActualEquipment(t *testing.T) map[domain.ItemID]domain.ItemDef {
	t.Helper()
	var raw struct {
		Bases []struct {
			ID                      domain.ItemID `json:"base_id"`
			Name                    string        `json:"base_name"`
			Level                   int32         `json:"wearer_level"`
			Tier                    int32         `json:"resource_tier"`
			Type, Slot              int32
			Professions             []domain.Race
			Sex                     uint8
			Beginner                bool           `json:"beginner_allowed"`
			Restricted              bool           `json:"profession_restricted"`
			Unknown                 bool           `json:"unknown_innate_properties"`
			SuitID                  int32          `json:"suit_id"`
			Fixed                   []domain.Affix `json:"fixed_affixes"`
			Intrinsic, Requirements map[string]int32
		}
	}
	v5PacketJSON(t, "balance/native_equipment_budget_references.json", &raw)
	out := map[domain.ItemID]domain.ItemDef{}
	for _, r := range raw.Bases {
		i, req := r.Intrinsic, r.Requirements
		var mask uint8
		for _, race := range r.Professions {
			mask |= 1 << race
		}
		d := domain.ItemDef{ID: r.ID, Name: r.Name, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: r.Slot, LevelReq: r.Level, ResourceLevel: r.Tier, Type: r.Type, Durable: 100, Affixes: append([]domain.Affix(nil), r.Fixed...), UnknownInnateProperties: r.Unknown,
			Base: domain.Base{STR: i["str"], VIT: i["vit"], INT: i["int_"], SPI: i["spi"], AGI: i["agi"], DEX: i["dex"]}, Bonus: domain.Stats{MinAtk: i["min_atk"], MaxAtk: i["max_atk"], MAtk: i["matk"], Def: i["def"], MDef: i["mdef"], Hit: i["hit_rate"]},
			Need: domain.Requirement{Level: r.Level, Sex: r.Sex, Professions: mask, BeginnerAllowed: r.Beginner, ProfessionRestricted: r.Restricted, Base: domain.Base{STR: req["str_need"], VIT: req["vit_need"], INT: req["int_need"], SPI: req["spi_need"], AGI: req["agi_need"], DEX: req["dex_need"]}},
		}}
		if r.SuitID != 0 {
			d.Equip.Suit = &domain.EquipmentSuit{ID: r.SuitID}
		}
		out[d.ID] = d
	}
	for _, d := range domain.AstralRuneItems(7) {
		out[d.ID] = d
	}
	return out
}

type v5PacketGear struct {
	slot  domain.EquipSlot
	stack domain.Stack
	label string
}

func v5PacketGearCandidates(t *testing.T, s *Scene, p *entity.Entity) [][]v5PacketGear {
	t.Helper()
	ch := p.Player.Char
	useful := func(bonuses []domain.SkillRankBonus) bool {
		for _, b := range bonuses {
			for _, id := range domain.V5SkillIDs() {
				identity, _ := domain.V5SkillIdentity(id)
				if b.Matches(identity, int32(ch.Race)+1) {
					return true
				}
			}
		}
		return false
	}
	legal := func(d domain.ItemDef) bool {
		return d.Equip != nil && d.Equip.Need.AllowsCharacter(ch) && d.Equip.Need.Meet(ch.Level, ch.Base, ch.Appear.Gender) == domain.RejectNone
	}
	groups := map[domain.EquipSlot][]v5PacketGear{}
	add := func(d domain.ItemDef, st domain.Stack, label string) {
		slot := domain.EquipSlot(d.Equip.Slot)
		group := slot
		if slot == domain.SlotTwoHand {
			group = domain.SlotWeapon
		}
		groups[group] = append(groups[group], v5PacketGear{slot, st, label})
	}
	for _, u := range domain.UniqueCatalog() {
		if !useful(u.SkillBonuses) {
			continue
		}
		d, ok := s.items[u.BaseID]
		if !ok || !legal(d) {
			continue
		}
		st := domain.NewStack(d, 1)
		st.UID = 9223372036854700000 + int64(d.ID)
		st.Endgame = domain.EndgameItemState{UniqueID: u.ID, UniqueRollBP: domain.UniqueRollMaxBP, UniqueContentVersion: domain.UniqueContentVersion}
		st.Durability = 100
		st.MaxDurability = 100
		add(d, st, fmt.Sprintf("unique:%d", u.ID))
	}
	ids := make([]domain.ItemID, 0, len(s.items))
	for id := range s.items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, r := range domain.AstralRecipes() {
		if !useful(r.SkillBonuses) || r.Professions != 0 && r.Professions&(1<<ch.Race) == 0 {
			continue
		}
		// All battle recipes use the same weapon-conflict group. Support recipes use
		// face first, preserving legitimate unique gloves/rings instead of stacking
		// a formation on a unique, which the real eligibility rules forbid.
		found := false
		for _, slot := range r.Slots {
			for _, id := range ids {
				d := s.items[id]
				if !legal(d) || d.Equip.Slot != int32(slot) {
					continue
				}
				st := domain.NewStack(d, 1)
				st.UID = 9223372036854750000 + int64(r.ID)*100 + int64(slot)
				st.Durability = 100
				st.MaxDurability = 100
				st.Endgame.FormationContentVersion = domain.UniqueContentVersion
				st.SocketCount = uint8(len(r.Runes))
				for i, rune := range r.Runes {
					st.Sockets[i] = rune
					st.SocketUIDs[i] = 9223372036854760000 + int64(r.ID)*100 + int64(i)
					st.SocketCards[i] = s.items[rune].CardDefaults
				}
				matched, ok := domain.MatchAstralRecipe(d, st)
				if !ok || matched.ID != r.ID || domain.EquipmentRequiredLevel(d, st) > 100 {
					continue
				}
				add(d, st, fmt.Sprintf("formation:%d/base:%d", r.ID, d.ID))
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatalf("no legitimate plain base for recipe%d class%d", r.ID, ch.Race)
		}
	}
	slots := make([]int, 0, len(groups))
	for slot := range groups {
		slots = append(slots, int(slot))
	}
	sort.Ints(slots)
	out := make([][]v5PacketGear, 0, len(slots))
	for _, slot := range slots {
		out = append(out, groups[domain.EquipSlot(slot)])
	}
	return out
}

func TestV5SkillRanksPacketAllFiveClassesActualLegalGear(t *testing.T) {
	skills, statuses := v5PacketActualSkills(t)
	items := v5PacketActualEquipment(t)
	totalRows := 0
	for race := domain.Warrior; race <= domain.Warlock; race++ {
		t.Run(fmt.Sprintf("class%d", race+1), func(t *testing.T) {
			classIDs := []domain.SkillID{}
			for _, id := range domain.V5SkillIDs() {
				d, _ := domain.V5SkillIdentity(id)
				if d.Profession == int32(race)+1 {
					classIDs = append(classIDs, id)
				}
			}
			expected := 30
			if race == domain.Healer {
				expected = 29
			}
			if len(classIDs) != expected {
				t.Fatalf("class%d battle rows%d want%d (11005 remains life skill)", race+1, len(classIDs), expected)
			}
			totalRows += len(classIDs)
			maximum, scenarios, maxSources, unlearnedMaximum := 0, 0, 0, 0
			var maxLabels []string
			seenFormation, seenSingle, seenBroad := false, false, false
			for _, gender := range []uint8{0, 1} {
				s := newTestScene(t, nil)
				join(t, s, 1, 100, "技能面板", 100, 100)
				p := s.players[1]
				ch := p.Player.Char
				s.skills = skills
				s.statuses = statuses
				s.items = items
				ch.Race = race
				ch.Level = 100
				ch.Appear.Gender = gender
				ch.EmploymentKnown = true
				ch.Employed = true
				ch.SkillAllocationVersion = 5
				ch.SkillPointsKnown = true
				ch.Base = domain.StartingBase(race)
				for lv := int32(2); lv <= 100; lv++ {
					ch.Base = ch.Base.Add(domain.GrowthAt(race, lv))
				}
				// These are only the legitimate 198 freely assigned stat points.
				ch.Base.STR += 40
				ch.Base.INT += 40
				ch.Base.SPI += 40
				ch.Base.VIT += 40
				ch.Base.AGI += 20
				ch.Base.DEX += 18
				p.HP, p.MaxHP, p.MP, p.MaxMP = 10000, 10000, 10000, 10000
				groups := v5PacketGearCandidates(t, s, p)
				for _, allocation := range []string{"breadth99", "focused99", "unlearned99"} {
					ch.Skills = domain.Learned{}
					for _, id := range classIDs {
						ch.Skills[id] = 1
					}
					left := 99 - int32(len(classIDs))
					for left > 0 {
						for _, id := range classIDs {
							if left == 0 {
								break
							}
							if ch.Skills[id] >= 20 {
								continue
							}
							take := int32(1)
							if allocation == "focused99" {
								take = min(20-ch.Skills[id], left)
							}
							ch.Skills[id] += take
							left -= take
						}
					}
					ch.SkillPoints = 0
					if allocation == "unlearned99" {
						ch.Skills = domain.Learned{}
						ch.SkillPoints = 99
					}
					if err := domain.ValidateSkillAllocation(100, int32(race)+1, ch.Skills, ch.SkillPoints); err != nil {
						t.Fatalf("illegal test hard allocation: %v", err)
					}
					var enumerate func(int, []v5PacketGear)
					enumerate = func(index int, gear []v5PacketGear) {
						if index < len(groups) {
							for _, g := range groups[index] {
								enumerate(index+1, append(gear, g))
							}
							return
						}
						p.Player.Worn = domain.NewEquipSet()
						labels := []string{}
						for _, g := range gear {
							p.Player.Worn.Set(g.slot, g.stack)
							labels = append(labels, g.label)
						}
						if s.functionalWorn(p).Count() != len(gear) {
							t.Fatalf("constructed gear has inactive/conflicting item: %v", labels)
						}
						ranks, err := s.skillRanksForDisplay(p)
						if err != nil {
							t.Fatal(err)
						}
						if len(ranks) != expected {
							t.Fatalf("display omitted class skills: %d/%d", len(ranks), expected)
						}
						rows := make([]domain.SkillRankView, 0, len(ranks))
						sources := 0
						for _, id := range classIDs {
							row, ok := ranks[id]
							if !ok || len(row.NextEffectLines) == 0 {
								t.Fatalf("missing enriched next-rank/first-investment preview for%d", id)
							}
							if allocation == "unlearned99" {
								if row.CanCast || row.EffectiveRank != "0" || len(row.CurrentEffectLines) != 0 {
									t.Fatalf("equipment unlocked unlearned skill%d", id)
								}
							} else if len(row.CurrentEffectLines) == 0 {
								t.Fatalf("missing enriched current effect for%d", id)
							}
							if strings.Contains(strings.Join(append(append([]string(nil), row.CurrentEffectLines...), row.NextEffectLines...), ""), "无法解析") {
								t.Fatalf("actual skill%d failed display evaluation", id)
							}
							rows = append(rows, row)
							sources += len(row.Contributions)
							for _, b := range row.Contributions {
								seenFormation = seenFormation || strings.Contains(b.SourceID, "formation")
								seenSingle = seenSingle || b.Scope == "single"
								seenBroad = seenBroad || b.Scope == "all" || b.Scope == "class"
							}
						}
						response := domain.EndgameResponse{Kind: "skill.ranks", RequestID: strings.Repeat("r", 64), Revision: "9223372036854775807", OK: true, SkillRanks: rows, Result: map[string]string{"hardCap": "20", "playerCap": "100", "petCap": "80", "unspentPoints": fmt.Sprint(ch.SkillPoints), "formulaVersion": "5"}}
						raw, err := json.Marshal(response)
						if err != nil {
							t.Fatal(err)
						}
						encoded := protocol.EncodeEndgame(response)
						if len(raw) >= 60000 || encoded == nil {
							t.Fatalf("class%d skills payload requires chunking: JSON%d protocolLimit%d rows%d gear%v", race+1, len(raw), domain.EndgameMaxJSONBytes, len(rows), labels)
						}
						if !bytes.Contains(encoded, raw) {
							t.Fatal("protocol envelope truncated enriched response")
						}
						var decoded domain.EndgameResponse
						if err = json.Unmarshal(raw, &decoded); err != nil || len(decoded.SkillRanks) != expected {
							t.Fatalf("round trip lost skill rows: %v", err)
						}
						scenarios++
						if allocation == "unlearned99" && len(raw) > unlearnedMaximum {
							unlearnedMaximum = len(raw)
						}
						if sources > maxSources {
							maxSources = sources
						}
						if len(raw) > maximum {
							maximum = len(raw)
							maxLabels = append([]string{fmt.Sprintf("gender%d/%s", gender, allocation)}, labels...)
						}
					}
					enumerate(0, nil)
				}
			}
			if scenarios == 0 || !seenFormation || !seenBroad || !seenSingle {
				t.Fatalf("incomplete gear source coverage scenarios%d broad%v single%v formation%v", scenarios, seenBroad, seenSingle, seenFormation)
			}
			t.Logf("class%d: rows=%d largestJSON=%d bytes <60000; legitimate gear/allocation scenarios=%d; maxRepeatedSources=%d; unlearnedFirstPointJSON=%d; largest=%v", race+1, expected, maximum, scenarios, maxSources, unlearnedMaximum, maxLabels)
		})
	}
	if totalRows != 149 {
		t.Fatalf("five-class coverage%d want149", totalRows)
	}
}

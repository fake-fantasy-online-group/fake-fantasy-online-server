package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// These are real scene-produced wire pages, also consumed by the native client
// contracts. The oracle is intentionally not the production filtering helper.
func TestPowerBookProfessionPagesAndSharedWire(t *testing.T) {
	type wireCase struct {
		Name     string                   `json:"name"`
		Race     int                      `json:"race"`
		Employed bool                     `json:"employed"`
		Class    string                   `json:"class"`
		Done     []int32                  `json:"done"`
		Legacy   bool                     `json:"legacy"`
		Pages    []domain.EndgameResponse `json:"pages"`
	}
	var fixture []wireCase
	totalRows, totalPages := 0, 0
	for _, tc := range []wireCase{
		{Name: "warrior", Race: 0, Employed: true, Class: "战士"}, {Name: "swordsman", Race: 1, Employed: true, Class: "剑客"},
		{Name: "assassin", Race: 2, Employed: true, Class: "刺客"}, {Name: "healer", Race: 3, Employed: true, Class: "药师"},
		{Name: "warlock", Race: 4, Employed: true, Class: "术士"}, {Name: "novice", Race: 4},
		{Name: "legacy-warrior", Race: 0, Employed: true, Class: "战士", Legacy: true},
		{Name: "legacy-swordsman", Race: 1, Employed: true, Class: "剑客", Legacy: true},
		{Name: "legacy-assassin", Race: 2, Employed: true, Class: "刺客", Legacy: true},
		{Name: "legacy-healer", Race: 3, Employed: true, Class: "药师", Legacy: true},
		{Name: "legacy-warlock", Race: 4, Employed: true, Class: "术士", Legacy: true}, {Name: "unknown", Race: 99, Employed: true},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			s, sink, b := powerBookSceneFixture(t)
			p := s.players[1]
			p.Player.Char.Race = domain.Race(tc.Race)
			p.Player.Char.Employed = tc.Employed
			p.Player.Char.EmploymentKnown = true
			// Produce the native task receipt independently of employment. The
			// folded 0036 migration marks legacy characters employed but does not
			// invent a 400..404 completion; exercise that real persisted shape.
			s.quests = domain.QuestTable{}
			jobs := []domain.QuestID{403, 404, 402, 400, 401}
			if tc.Employed && !tc.Legacy && tc.Race < len(jobs) {
				p.Player.Char.Quests.Finish(jobs[tc.Race])
			}
			questBefore := p.Player.Char.Quests.FinishedIDs()
			s.pushQuestLog(p)
			tasks, ok := firstOf[event.ActiveQuests](cubeTake(s, sink))
			if !ok || !reflect.DeepEqual(tasks.Done, questBefore) {
				t.Fatal("real task receipt changed quest history", tasks)
			}
			tc.Done = tasks.Done
			hasJob := false
			for _, id := range tc.Done {
				if id >= 400 && id <= 404 {
					hasJob = true
				}
			}
			if hasJob != (tc.Employed && !tc.Legacy && tc.Race < len(jobs)) {
				t.Fatal("fabricated/missing career task", tc.Done)
			}

			book := domain.LegendaryPowerBook{}
			expected := map[string]bool{}
			for i, power := range domain.LegendaryPowers() {
				if i%2 == 0 {
					roll := int32(8123)
					if !domain.LegendaryPowerHasRoll(power.ID) {
						roll = 0
					}
					book[power.ID] = roll
				}
				if power.Class == "全职业" || tc.Class != "" && power.Class == tc.Class {
					expected[power.ID] = true
				}
			}
			payload, err := domain.WriteLegendaryPowerBook(`{"keep":"other-profession-records"}`, book)
			if err != nil {
				t.Fatal(err)
			}
			s.endgamePlayer(p).state.Payload = payload
			bag := p.Player.Bag.Clone()
			seen := map[string]bool{}
			cursor := ""
			owned, unowned := 0, 0
			for {
				s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", RequestID: fmt.Sprintf("profession-%s-%s", tc.Name, cursor), Cursor: cursor})
				reply, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
				if !ok || !reply.Message.OK {
					t.Fatal(reply)
				}
				expectedProfession := 0
				if tc.Class != "" {
					expectedProfession = tc.Race + 1
				}
				if reply.Message.Result["powerBookProfession"] != strconv.Itoa(expectedProfession) {
					t.Fatal("authoritative profession metadata", reply.Message.Result)
				}
				raw, err := json.Marshal(reply.Message)
				if err != nil || len(raw) > domain.EndgameMaxJSONBytes {
					t.Fatal("wire budget", len(raw), err)
				}
				for _, row := range reply.Message.Powers {
					if !expected[row.PowerID] || seen[row.PowerID] {
						t.Fatal("foreign/duplicate power", row.PowerID)
					}
					seen[row.PowerID] = true
					roll, recorded := book[row.PowerID]
					if row.Unlocked != recorded {
						t.Fatal("record state", row)
					}
					if recorded {
						owned++
						if row.CurrentValueText != domain.LegendaryPowerCurrentValueText(row.PowerID, roll) {
							t.Fatal("current value", row)
						}
					} else {
						unowned++
						if row.RollBP != 0 || row.CurrentValueText != "" {
							t.Fatal("unowned current", row)
						}
					}
				}
				tc.Pages = append(tc.Pages, reply.Message)
				totalPages++
				if reply.Message.NextCursor == "" {
					break
				}
				if reply.Message.NextCursor == cursor {
					t.Fatal("stuck cursor")
				}
				cursor = reply.Message.NextCursor
			}
			if len(seen) != len(expected) || owned == 0 || unowned == 0 {
				t.Fatal("missing records", len(seen), len(expected), owned, unowned)
			}
			totalRows += len(seen)
			for _, cursor := range []string{strconv.Itoa(len(expected)), strconv.Itoa(len(expected) + 1), "-1", "01", "+1", " 1", "bad", "999999999999999999999999"} {
				s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", Cursor: cursor})
				reply, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
				if cursor == strconv.Itoa(len(expected)) {
					if !reply.Message.OK || len(reply.Message.Powers) != 0 || reply.Message.NextCursor != "" {
						t.Fatal("end cursor", reply)
					}
				} else if reply.Message.OK || reply.Message.Error != "cursor" {
					t.Fatal("invalid cursor", cursor, reply)
				}
			}
			if b.commits != 0 || payload != s.endgamePlayer(p).state.Payload || !reflect.DeepEqual(bag, p.Player.Bag) || !reflect.DeepEqual(questBefore, p.Player.Char.Quests.FinishedIDs()) {
				t.Fatal("read changed inventory or records")
			}
			fixture = append(fixture, tc)
			t.Logf("%s: %d own/common rows, %d pages, %d owned + %d unobtained; 8 cursor boundaries", tc.Name, len(seen), len(tc.Pages), owned, unowned)
		})
	}
	path := filepath.Join("testdata", "powerbook", "profession-pages.json")
	raw, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if os.Getenv("UPDATE_POWERBOOK_CONTRACT") == "1" {
		if err = os.WriteFile(path, raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := os.ReadFile(path)
	if err != nil || string(stored) != string(raw) {
		t.Fatal("profession wire fixture differs", err)
	}
	t.Logf("%d authoritative profession/task-receipt contexts; %d total visible rows across %d production wire pages", len(fixture), totalRows, totalPages)
}

func TestPowerBookProfessionChangeRejectsOldCursorWithoutDeletingRecords(t *testing.T) {
	s, sink, _ := powerBookSceneFixture(t)
	p := s.players[1]
	p.Player.Char.Race = domain.Warlock
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	payload := `{"powerBook":{"SS-A01":8123,"ZS-A01":8123}}`
	s.endgamePlayer(p).state.Payload = payload
	oldEnd := len(powerBookIDsForCharacter(p.Player.Char))
	p.Player.Char.Employed = false
	s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", Cursor: strconv.Itoa(oldEnd)})
	reply, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if reply.Message.OK || reply.Message.Error != "cursor" {
		t.Fatal("stale larger cursor", reply)
	}
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.Employed = true
	s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list"})
	reply, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !reply.Message.OK {
		t.Fatal(reply)
	}
	for _, row := range reply.Message.Powers {
		if row.Class != "战士" && row.Class != "全职业" {
			t.Fatal("old class leaked", row)
		}
	}
	if s.endgamePlayer(p).state.Payload != payload {
		t.Fatal("class change deleted recorded powers")
	}
}

package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

func TestPowerCodexCurrentValueAll330OwnedBoundariesAndUnowned(t *testing.T) {
	type valueRow struct {
		PowerID          string `json:"powerId"`
		Unlocked         bool   `json:"unlocked"`
		CurrentValueText string `json:"currentValueText,omitempty"`
		RangeText        string `json:"rangeText,omitempty"`
		PrimaryRangeText string `json:"primaryRangeText,omitempty"`
	}
	type scenario struct {
		Name string     `json:"name"`
		Rows []valueRow `json:"rows"`
	}
	var fixture []scenario
	for _, tc := range []struct {
		name  string
		owned bool
		roll  int32
	}{{"unowned", false, 0}, {"legacy-zero", true, 0}, {"minimum", true, 7500}, {"8123", true, 8123}, {"maximum", true, 10000}} {
		s, sink, _ := powerBookSceneFixture(t)
		p := s.players[1]
		book := domain.LegendaryPowerBook{}
		if tc.owned {
			for _, power := range domain.LegendaryPowers() {
				roll := tc.roll
				if !domain.LegendaryPowerHasRoll(power.ID) {
					roll = 0
				}
				book[power.ID] = roll
			}
		}
		payload, err := domain.WriteLegendaryPowerBook("{}", book)
		if err != nil {
			t.Fatal(err)
		}
		s.endgamePlayer(p).state.Payload = payload
		before := p.Player.Bag.Clone()
		seen := map[string]bool{}
		out := scenario{Name: tc.name}
		for _, race := range []domain.Race{domain.Warrior, domain.Swordsman, domain.Assassin, domain.Healer, domain.Warlock} {
			p.Player.Char.Race = race
			p.Player.Char.Employed, p.Player.Char.EmploymentKnown = true, true
			cursor := ""
			professionSeen := map[string]bool{}
			for {
				s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", Cursor: cursor})
				reply, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
				if !ok || !reply.Message.OK {
					t.Fatal("list", reply)
				}
				for _, row := range reply.Message.Powers {
					if professionSeen[row.PowerID] {
						t.Fatal("duplicate profession page", row.PowerID)
					}
					professionSeen[row.PowerID] = true
					if seen[row.PowerID] {
						continue
					}
					seen[row.PowerID] = true
					want := ""
					if tc.owned {
						want = domain.LegendaryPowerCurrentValueText(row.PowerID, book[row.PowerID])
					}
					if row.Unlocked != tc.owned || row.CurrentValueText != want || row.PrimaryRangeText != domain.LegendaryPowerPrimaryRangeText(row.PowerID) {
						t.Fatalf("%s %+v want %s", tc.name, row, want)
					}
					if tc.name == "8123" && row.PowerID == "TY-A01" && row.CurrentValueText != "28.43%" {
						t.Fatal("not authored primary stat", row)
					}
					out.Rows = append(out.Rows, valueRow{row.PowerID, row.Unlocked, row.CurrentValueText, row.RangeText, row.PrimaryRangeText})
				}
				if reply.Message.NextCursor == "" {
					break
				}
				if reply.Message.NextCursor == cursor {
					t.Fatal("stuck cursor")
				}
				cursor = reply.Message.NextCursor
			}
		}
		sort.Slice(out.Rows, func(i, j int) bool { return out.Rows[i].PowerID < out.Rows[j].PowerID })
		if len(seen) != 330 {
			t.Fatal("catalog changed", len(seen))
		}
		if s.endgamePlayer(p).state.Payload != payload || !reflect.DeepEqual(before, p.Player.Bag) {
			t.Fatal("listing mutated state")
		}
		fixture = append(fixture, out)
	}
	path := filepath.Join("testdata", "powerbook", "current-values.json")
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
		t.Fatal("current-value producer fixture differs", err)
	}
}

// Two exact, profession-filtered production pages give the client owned numeric,
// owned functional and unowned rows, including misleading earlier prose digits.
func TestPowerCodexCurrentValueSharedResponsePages(t *testing.T) {
	for _, tc := range []struct{ id, file string }{{"SS-A01", "current-value-response"}, {"TY-A01", "current-value-universal-response"}} {
		s, sink, _ := powerBookSceneFixture(t)
		p := s.players[1]
		p.Player.Char.Race = domain.Warlock
		p.Player.Char.Employed, p.Player.Char.EmploymentKnown = true, true
		book := domain.LegendaryPowerBook{"TY-A01": 8123, "SS-A01": 8123, "SS-A13": 8123, "SS-U06": 0}
		payload, err := domain.WriteLegendaryPowerBook("{}", book)
		if err != nil {
			t.Fatal(err)
		}
		s.endgamePlayer(p).state.Payload = payload
		cursor := ""
		for i, id := range powerBookIDsForCharacter(p.Player.Char) {
			if id == tc.id {
				cursor = strconv.Itoa(i)
				break
			}
		}
		if cursor == "" {
			t.Fatal("missing start", tc.id)
		}
		s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", RequestID: tc.file, Cursor: cursor})
		response, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if !ok || !response.Message.OK {
			t.Fatal("page", response)
		}
		path := filepath.Join("testdata", "powerbook", tc.file+".json")
		raw, err := json.MarshalIndent(response.Message, "", "  ")
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
			t.Fatal("shared response differs", tc.file, err)
		}
	}
}

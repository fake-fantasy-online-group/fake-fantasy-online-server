package scene

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// The same exact JSON artifacts are consumed by the client regression suite.
// Explicit regeneration is for intentional protocol/description changes only.
func TestPowerBookSharedClientContract(t *testing.T) {
	s, sink, b := powerBookSceneFixture(t)
	p := s.players[1]
	book := domain.LegendaryPowerBook{"TY-A01": 8123, "SS-A01": 10000, "SS-U06": 0}
	payload, err := domain.WriteLegendaryPowerBook("{}", book)
	if err != nil {
		t.Fatal(err)
	}
	s.endgamePlayer(p).state.Payload = payload
	s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", RequestID: "powerbook-list-fixture"})
	list, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || !list.Message.OK {
		t.Fatal("list fixture", list)
	}
	st := p.Player.Bag.At(200)
	st.Legendary.PowerID = "TY-A02"
	p.Player.Bag.Set(200, st)
	req := domain.EndgameRequest{Op: "powerbook.imprint", RequestID: "powerbook-fixture-0001", ExpectedRevision: "0", TargetUID: "42", PowerID: "TY-A01"}
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("imprint fixture", cubeTake(s, sink))
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	success, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || success.Message.Kind != "powerbook.result" || !success.Message.OK {
		t.Fatal("success fixture", success)
	}
	req.RequestID = "powerbook-fixture-0002"
	req.ExpectedRevision = "1"
	req.PowerID = "TY-A02"
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	failure, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || failure.Message.OK || failure.Message.Error != "power_locked" {
		t.Fatal("failure fixture", failure)
	}
	fixtures := map[string]any{"list": list.Message, "imprint-success": success.Message, "imprint-error": failure.Message, "imprint-request": domain.EndgameRequest{Op: "powerbook.imprint", RequestID: "powerbook-fixture-0001", ExpectedRevision: "0", TargetUID: "42", PowerID: "TY-A01"}}
	for name, value := range fixtures {
		path := filepath.Join("testdata", "powerbook", name+".json")
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, '\n')
		if os.Getenv("UPDATE_POWERBOOK_CONTRACT") == "1" {
			if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, encoded, 0644); err != nil {
				t.Fatal(err)
			}
		}
		stored, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var actual, want any
		if json.Unmarshal(encoded, &actual) != nil || json.Unmarshal(stored, &want) != nil || !reflect.DeepEqual(actual, want) {
			t.Fatalf("%s differs from shared client fixture; review before regeneration", name)
		}
	}
}

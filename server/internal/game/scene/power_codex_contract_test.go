package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"os"
	"path/filepath"
	"testing"
)

func TestPowerCodexSharedContract(t *testing.T) {
	s, sink, b := powerBookSceneFixture(t)
	p := s.players[1]
	p.Player.EndgamePowerCodex = true
	s.endgamePlayer(p).state.Payload = `{"powerBook":{"TY-A02":8123}}`
	req := domain.EndgameRequest{Op: "powerbook.extract", RequestID: "extract-contract-0001", ExpectedRevision: "0", PowerID: "TY-A02"}
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("no extraction")
	}
	var st domain.Stack
	b.plan.Snapshot.Bag.Each(func(_ int, v domain.Stack) {
		if v.Item == domain.PowerScrollItemID {
			st = v
		}
	})
	// Freeze only the generated instance UID in this portable fixture.
	st.UID = 9007199254740993
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	response, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	fixtures := map[string]any{"extract-request": req, "extract-success": response.Message, "scroll-snapshot": domain.EndgameResponse{Kind: "snapshot", OK: true, Revision: "1", Items: []domain.EndgameItemView{s.endgameItem(st, "bag", 6)}}}
	for name, value := range fixtures {
		path := filepath.Join("testdata", "powerbook", name+".json")
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, '\n')
		if os.Getenv("UPDATE_POWERBOOK_CONTRACT") == "1" {
			if err := os.WriteFile(path, raw, 0644); err != nil {
				t.Fatal(err)
			}
		}
		stored, err := os.ReadFile(path)
		if err != nil || string(stored) != string(raw) {
			t.Fatal("fixture mismatch", name, err)
		}
	}
}

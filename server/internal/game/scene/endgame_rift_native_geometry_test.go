package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// These coordinates are the authored native portal polygons, not enlarged
// circles around return points. The manifest is shared with migration 103's
// independent route/geometry audit, so each restored gate reaches the handler.
func TestRiftNativeGeometryEveryCandidateGateReachesAdvanceHandler(t *testing.T) {
	raw, err := os.ReadFile("../../testfixture/riftportals/all-native-gates.json")
	if err != nil {
		t.Fatal(err)
	}
	var gates []struct {
		MapID  int32                  `json:"map_id"`
		ProcID int                    `json:"proc_id"`
		Poly   []float64              `json:"poly"`
		Point  struct{ X, Y float64 } `json:"point"`
	}
	if err := json.Unmarshal(raw, &gates); err != nil {
		t.Fatal(err)
	}
	if len(gates) == 0 {
		t.Fatal("empty verified gate manifest")
	}
	full := fullCoverageSceneFixture(t, 22, 22)
	for _, gate := range gates {
		for _, restored := range []bool{false, true} {
			t.Run(fmt.Sprintf("map%d/proc%d/restored_%v", gate.MapID, gate.ProcID, restored), func(t *testing.T) {
				s, b, _ := seededRiftFloorFixture(t)
				s.riftRules = full.riftRules
				s.riftLayouts = full.riftLayouts
				s.defs = full.defs
				s.rift.data.Run.Rules = s.riftRules.Active()
				s.rift.data.Run.RulesVersion = 10
				s.rift.data.Run.Progress = 5760
				oldID := s.id
				s.id.MapID = gate.MapID
				delete(s.router.scenes, oldID)
				s.router.scenes[s.id] = s
				s.rift.data.MapID = gate.MapID
				s.rift.data.Floor.MapID = gate.MapID
				s.rift.data.Floor.GeneratorVersion = domain.RiftFloorFullCoverageVersion
				// Keep the generated exit away: a native gate must work independently.
				s.rift.data.Floor.Exit = domain.Pos{MapID: gate.MapID, X: 100000, Y: 100000}
				if gate.MapID == 36 {
					s.rift.data.Floor.Exit = domain.Pos{MapID: 36, X: 14112, Y: 3680}
				}
				s.rift.portals = nil
				p := s.players[1]
				p.Player.Char.Race = domain.Assassin
				p.Player.EndgameRiftPages = true
				area := domain.Polygon{}
				at := domain.Pos{MapID: gate.MapID}
				for i := 0; i < len(gate.Poly); i += 2 {
					area = append(area, domain.Pos{MapID: gate.MapID, X: gate.Poly[i], Y: gate.Poly[i+1]})
					at.X += gate.Poly[i] / float64(len(gate.Poly)/2)
					at.Y += gate.Poly[i+1] / float64(len(gate.Poly)/2)
				}
				at.X, at.Y = gate.Point.X, gate.Point.Y
				if !area.Contains(at) {
					t.Fatal("authored centroid not inside native polygon")
				}
				p.Pos = at
				p.Player.Char.Pos = at
				s.router.riftAdmissions[s.id] = &endgameRiftAdmission{Data: s.rift.data.clone(), Paid: true, Allowed: map[domain.CharID]bool{100: true}}
				if oldID != s.id {
					delete(s.router.riftAdmissions, oldID)
				}
				b.runs[s.rift.data.Run.RunID] = s.rift.data.record(5)
				if restored {
					s.rift.portals = []domain.Portal{domain.NewPortal(domain.Teleport{Area: area})}
				}
				req := advanceRiftFloorRequest(s)
				req.ExpectedRevision = "0"
				var target *Scene
				if restored {
					target = prepareRiftFloorTarget(t, s)
				}
				s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
				s.finishEndgame(false)
				if !restored {
					requireRiftFloorError(t, s, p, "exit")
					if len(b.writes) != 0 {
						t.Fatal("missing native geometry wrote a floor")
					}
					return
				}
				if len(b.writes) != 1 || s.rift.transition == nil {
					t.Fatalf("restored native gate failed handler: writes=%d transition=%v", len(b.writes), s.rift.transition)
				}
				if b.writes[0].Revision != 6 || b.cas[0] != 5 {
					t.Fatal("native gate bypassed durable floor CAS")
				}
				if s.rift.transition.next.Floor.FloorIndex != 2 || s.rift.transition.next.Run.Progress != 5760 {
					t.Fatal("native gate did not preserve progression")
				}
				if s.rift.transition.next.Run.RulesVersion != 10 || s.rift.transition.next.Floor.GeneratorVersion != 6 {
					t.Fatal("native gate lost active rules/layout")
				}
				b.finishFloorWrite(t, 0, true, false)
				s.stepEndgameRift()
				target.drain()
				target.stepEndgameRift()
				arrived := target.players[p.ID]
				if !s.rift.retired || arrived == nil || arrived.Pos != target.rift.data.Floor.Entry || target.rift.data.Floor.FloorIndex != 2 {
					t.Fatalf("restored gate did not complete actual player handoff: retired=%v arrived=%+v targetRift=%+v", s.rift.retired, arrived, target.rift)
				}
				if arrived.Player.Bag.CountOf(domain.RiftKeyItemID) != 3 || b.commits != 0 {
					t.Fatal("native portal handoff charged a key or character transaction")
				}
			})
		}
	}
}

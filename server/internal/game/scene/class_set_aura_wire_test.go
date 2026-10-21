package scene

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Export actual production snapshot chunks for the client parser/portal contract.
// Exercise both the wearer and a teammate with no set of their own.
func TestClassSetAuraSnapshotWireIdentities(t *testing.T) {
	s, ps := partySetFive(t)
	join(t, s, 6, 600, "recipient", 160, 100)
	peer := s.players[6]
	peer.Player.Party = 1
	ps = append(ps, peer)
	s.stepEquipmentAuras()
	for _, p := range ps {
		p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
			st.UID = 9007199254740993 + int64(p.ID)*100 + int64(slot)
			p.Player.Worn.Set(slot, st)
		})
		p.Player.Bag = domain.NewBag(80)
		for i := 0; i < 40; i++ {
			p.Player.Bag.Set(i, domain.Stack{UID: 9007199254750000 + int64(p.ID)*100 + int64(i), Item: 870010, Count: 1, Durability: 100, MaxDurability: 100})
		}
		p.Player.EndgameGrowthV5 = true
		s.endgamePlayer(p).loaded = true
		sink := p.Player.Sink.(*fakeSink)
		s.flush()
		sink.take()
		s.sendEndgameSnapshot(p)
		messages := endgameMessages(s, sink)
		var chunks []json.RawMessage
		for _, m := range messages {
			if m.Kind != "snapshot" {
				continue
			}
			body, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			chunks = append(chunks, body)
			if len(protocol.Encode(p.ID, event.EndgameMessage{Who: p.ID, Message: m})) == 0 {
				t.Fatal("snapshot did not encode")
			}
		}
		if path := os.Getenv("CLASS_SET_SNAPSHOT_FIXTURE_DIR"); path != "" {
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
			body, _ := json.MarshalIndent(chunks, "", "  ")
			if err := os.WriteFile(filepath.Join(path, fmt.Sprintf("snapshot-%d.json", p.ID)), body, 0644); err != nil {
				t.Fatal(err)
			}
		}
		found := 0
		for _, m := range messages {
			if m.Kind != "snapshot" || m.CombatStats == nil {
				continue
			}
			for _, a := range m.CombatStats.EquipmentAuras {
				if !strings.HasPrefix(a.AuraID, "class_set_") {
					continue
				}
				found++
				if a.SourceUID != 0 {
					t.Errorf("player %d class aura %s leaked synthetic item identity %d", p.ID, a.AuraID, a.SourceUID)
				}
				if a.SourceEntityID == 0 {
					t.Fatal("missing actual aura owner")
				}
			}
		}
		if found != 5 {
			t.Fatalf("player %d has %d aura views, want five distinct sources", p.ID, found)
		}
		for _, g := range s.equipmentState().auras[p.ID] {
			if classSetAuraID(g.effect) != 0 && (g.uid >= 0 || !s.equipmentAuraGrantValid(p, g)) {
				t.Fatal("wire projection changed internal source binding")
			}
		}
	}
}

// The optional audited native plans exercise the production request dispatcher,
// receipt lookup, member admission, durable CAS and private actor handoff, not
// just portal geometry. No public target or scenery script is consulted.
func TestRiftAllAuditedMapsSnapshotAndMemberHandoff(t *testing.T) {
	path := os.Getenv("RIFT_PORTAL_AUDITED_PLANS")
	if path == "" {
		t.Skip("set RIFT_PORTAL_AUDITED_PLANS to native-map audit plans")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []struct {
		MapID int32
		Plan  domain.RiftFloorPlan
	}
	if err = json.Unmarshal(raw, &records); err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("empty map audit")
	}
	for _, rec := range records {
		t.Run(fmt.Sprint(rec.MapID), func(t *testing.T) {
			s, b, _ := seededRiftFloorFixture(t, domain.TrialKeyMember{CharID: 100, Level: 60}, domain.TrialKeyMember{CharID: 200, Level: 60})
			oldID := s.id
			s.id.MapID = rec.MapID
			s.rift.data.MapID = rec.MapID
			s.rift.data.Floor = &rec.Plan
			delete(s.router.scenes, oldID)
			s.router.scenes[s.id] = s
			admission := s.router.riftAdmissions[oldID]
			delete(s.router.riftAdmissions, oldID)
			s.router.riftAdmissions[s.id] = admission
			admission.Data = s.rift.data.clone()
			s.rift.portals = nil
			s.portals = nil
			guest := partySetMember(t, s, 2, 1)
			guest.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
				st.UID = 10000 + int64(slot)
				guest.Player.Worn.Set(slot, st)
				d := s.items[st.Item]
				d.InventoryTabKnown = true
				s.items[st.Item] = d
			})
			guest.Pos = rec.Plan.Exit
			s.rift.ready[200] = true
			guest.Player.EndgameGrowthV5 = true
			s.endgamePlayer(guest).loaded = true
			s.sendEndgameSnapshot(guest)
			saw := false
			for _, m := range endgameMessages(s, guest.Player.Sink.(*fakeSink)) {
				if m.Kind == "snapshot" && m.CombatStats != nil {
					for _, a := range m.CombatStats.EquipmentAuras {
						if a.AuraID == "class_set_1" {
							saw = true
							if a.SourceUID != 0 {
								t.Fatal("invalid source UID")
							}
						}
					}
				}
			}
			if !saw {
				t.Fatal("set wearer lacked production snapshot aura")
			}
			target := prepareRiftFloorTarget(t, s)
			req := advanceRiftFloorRequest(s)
			req.ExpectedRevision = "0"
			body, _ := json.Marshal(req)
			decoded, ok := protocol.DecodeEndgame(protocol.NewW(0).U16(domain.EndgameSchema).Str(string(body)).Bytes()[2:])
			if !ok {
				t.Fatal("request decode failed")
			}
			s.onEndgame(EndgameRequest{ID: guest.ID, Request: decoded})
			s.finishEndgame(false)
			// A second member's simultaneous request must not create another floor.
			s.endgamePlayer(s.players[1]).loaded = true
			s.players[1].Pos = rec.Plan.Exit
			req.RequestID = "second-member-portal-01"
			s.onEndgame(EndgameRequest{ID: 1, Request: req})
			s.finishEndgame(false)
			if len(b.writes) != 1 {
				t.Fatalf("floor writes=%d", len(b.writes))
			}
			b.finishFloorWrite(t, 0, true, false)
			s.stepEndgameRift()
			target.drain()
			target.stepEndgameRift()
			if target.players[2] == nil || target.rift.data.Run.Payer != 100 || target.rift.data.Floor.FloorIndex != rec.Plan.FloorIndex+1 {
				t.Fatal("non-opener handoff failed")
			}
		})
	}
	t.Logf("production request/snapshot/CAS/private handoff covered %d native plans", len(records))
}

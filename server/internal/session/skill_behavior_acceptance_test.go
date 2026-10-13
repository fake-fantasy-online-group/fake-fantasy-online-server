package session

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"testing"
)

func TestSkillBehaviorAcceptanceSessionWireRequiresBothCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name               string
		caps               []string
		accepted, behavior bool
	}{
		{"old", []string{domain.EndgameV1}, true, false},
		{"growth_only", []string{domain.EndgameV1, domain.EndgameGrowthV5}, true, false},
		{"behavior_without_growth", []string{domain.EndgameV1, domain.EndgameSkillBehaviorV1}, true, false},
		{"behavior_without_base", []string{domain.EndgameGrowthV5, domain.EndgameSkillBehaviorV1}, false, false},
		{"new", []string{domain.EndgameV1, domain.EndgameGrowthV5, domain.EndgameSkillBehaviorV1}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newNativeStatsRig(t, domain.EndgameLoadResult{})
			r.s.OnPacket(protocol.OpEndgame, growthPacket(domain.EndgameRequest{Op: "hello", Capabilities: tc.caps}))
			r.sync()
			responses := nativeStatsResponses(t, r.take())
			if tc.accepted {
				responses = assertHelloAdmissionPending(t, responses)
			}
			found := false
			for _, response := range responses {
				if response.Kind != "hello" {
					continue
				}
				found = true
				present := false
				for _, cap := range response.Capabilities {
					present = present || cap == domain.EndgameSkillBehaviorV1
				}
				if response.OK != tc.accepted || present != tc.behavior {
					t.Fatal("wrong real wire negotiation", response)
				}
				if !tc.accepted && response.Error != "unsupported_capability" {
					t.Fatal("missing base not rejected", response)
				}
			}
			if !found {
				t.Fatal("no real hello response", responses)
			}
			r.inspect(func(s *scene.Scene) {
				if s.EntityAt(1).Player.EndgameSkillBehavior != tc.behavior {
					t.Fatal("scene and session negotiation diverged")
				}
			})
		})
	}
}

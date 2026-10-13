package scene

import (
	"encoding/json"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestRiftHudTimeUsesRunRulesAndFrozenCompletion(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.stepEndgameRift()
	r := s.rift.data.Run
	rules := *r.Rules
	rules.TimedMS = 1234000
	r.Rules = &rules
	r.StartedMS = *now - 234000
	p := s.players[1]
	v := s.riftView(p, s.rift.data, true, true)
	if v.TotalSeconds != 1234 || v.RemainingSeconds != 1000 {
		t.Fatalf("active run uses its own duration: %+v", v)
	}
	r.PenaltyMS = 20000
	v = s.riftView(p, s.rift.data, true, true)
	if v.TotalSeconds != 1234 || v.RemainingSeconds != 980 {
		t.Fatalf("penalty advances elapsed time without changing total: %+v", v)
	}
	r.Phase = domain.RiftCompleted
	r.CompletedMS = *now
	*now += 3600000
	for _, live := range []bool{true, false} {
		v = s.riftView(p, s.rift.data, true, live)
		if v.TotalSeconds != 1234 || v.RemainingSeconds != 980 {
			t.Fatalf("completed timer must freeze (live=%v): %+v", live, v)
		}
		raw, err := json.Marshal(v)
		var decoded domain.EndgameRiftView
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.TotalSeconds != 1234 || decoded.RemainingSeconds != 980 {
			t.Fatalf("time metadata must survive wire serialization: %s", raw)
		}
	}
}

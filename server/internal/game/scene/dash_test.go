package scene

import (
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestDashDurationBounds(t *testing.T) {
	o := domain.Pos{MapID: 7, X: 100, Y: 100}
	for _, tc := range []struct {
		dx   float64
		want int32
	}{{0, 150}, {200, 200}, {400, 250}, {2000, 250}} {
		if got := dashDurationMS(o, domain.Pos{MapID: 7, X: 100 + tc.dx, Y: 100}); got != tc.want {
			t.Fatalf("distance %v duration %d want %d", tc.dx, got, tc.want)
		}
	}
}

func TestSkillTeleportIsSameMapDashWithControlImmunity(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Warlock, domain.Learned{10432: 1})
	from := p.Pos
	to := domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 200, Y: p.Pos.Y}
	s.outbox = nil
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
	if len(s.casts) == 0 {
		t.Fatal("native teleport rejected", s.outbox)
	}
	s.stepCastsAt(s.now().Add(time.Minute), false)
	s.stepCastsAt(s.now().Add(2*time.Minute), false)
	if p.Pos == from || p.Pos.MapID != from.MapID {
		t.Fatal("dash did not move on the same map", p.Pos)
	}
	if s.mapLoads[p.ID] != nil {
		t.Fatal("same-map dash started a map reload")
	}
	var dash *event.EntityMoved
	for _, o := range s.outbox {
		if e, ok := o.ev.(event.EntityMoved); ok && e.ID == p.ID && e.Dash {
			e := e
			dash = &e
		}
	}
	if dash == nil || !dash.Snap || !dash.Stop || dash.DurationMS < dashMinMS || dash.DurationMS > dashMaxMS || dash.To != p.Pos {
		t.Fatalf("missing authoritative dash event (Snap keeps old clients compatible): %+v", dash)
	}
	stun := domain.StatusDef{ID: 1002, Level: 1, DurationSec: 3, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun}
	if s.applyStatusDef(p, stun, p.ID+1000) || p.Status.HasEffective(1002) {
		t.Fatal("control applied during dash window")
	}
	if !s.legendaryMeleeDisplacementImmune(p) {
		t.Fatal("displacement not blocked during dash window")
	}
	weak := domain.StatusDef{ID: domain.StatusSlow, DurationSec: 3, TypeKnown: true, Type: domain.StatusBad}
	if !s.applyStatusDef(p, weak, p.ID+1000) {
		t.Fatal("non-control debuff must still apply while dashing")
	}
	s.tick += domain.Ticks(int(dash.DurationMS))
	if s.dashControlImmune(p) || s.legendaryMeleeDisplacementImmune(p) {
		t.Fatal("dash immunity outlived the dash")
	}
	if !s.applyStatusDef(p, stun, p.ID+1000) {
		t.Fatal("control rejected after the dash ended")
	}
}

func TestSkillDashRejectsUnwalkableLanding(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Warlock, domain.Learned{10432: 1})
	from := p.Pos
	s.walkable = func(domain.Pos) bool { return false }
	if s.skillDash(p, domain.Pos{MapID: from.MapID, X: from.X + 100, Y: from.Y}, 0) {
		t.Fatal("dash onto unwalkable point accepted")
	}
	if p.Pos != from || s.dashControlImmune(p) {
		t.Fatal("rejected dash moved or granted immunity")
	}
}

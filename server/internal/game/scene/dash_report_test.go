package scene

import (
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func dashReportMoves(s *Scene, id domain.EntityID) int {
	n := 0
	for _, o := range s.outbox {
		if e, ok := o.ev.(event.EntityMoved); ok && e.ID == id {
			n++
		}
	}
	return n
}

// After interpolating a dash the client reports the landing point once. That
// echo is not a move: no speed credit, no broadcast (observers would cut their
// dash interpolation short), no facing reset, no portal/rest side effects.
func TestDashLandingEchoIsNotAMove(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Warlock, domain.Learned{10432: 1})
	p.Stats.MoveSpeed = 100 // the fixture walks at speed 0; give the speed window real credit
	from := p.Pos
	if !s.skillDash(p, domain.Pos{MapID: from.MapID, X: from.X + 200.6, Y: from.Y + 0.4}, 3) {
		t.Fatal("dash failed")
	}
	landing := p.Pos
	if landing.X != float64(int32(landing.X)) || landing.Y != float64(int32(landing.Y)) {
		t.Fatal("dash landing is not on whole pixels, the client echo cannot match", landing)
	}
	s.outbox = nil
	s.onMove(MoveTo{ID: p.ID, To: landing, ReceivedAt: time.Now()})
	if dashReportMoves(s, p.ID) != 0 || p.Pos != landing || p.Dir != 3 {
		t.Fatalf("landing echo treated as a move: events=%d pos=%v dir=%d", dashReportMoves(s, p.ID), p.Pos, p.Dir)
	}
	if _, spent := s.moveWindows[p.ID]; spent {
		t.Fatal("landing echo touched the speed window")
	}
	if _, guarded := s.dashLanding[p.ID]; guarded {
		t.Fatal("echo did not release the dash guard")
	}
	// A plain echo without any dash is ignored the same way.
	s.onMove(MoveTo{ID: p.ID, To: landing, ReceivedAt: time.Now()})
	if dashReportMoves(s, p.ID) != 0 {
		t.Fatal("plain echo broadcast a move")
	}
	// Normal walking afterwards still works and is still speed-checked.
	step := domain.Pos{MapID: landing.MapID, X: landing.X + 20, Y: landing.Y}
	s.onMove(MoveTo{ID: p.ID, To: step, ReceivedAt: time.Now()})
	if p.Pos != step || dashReportMoves(s, p.ID) != 1 {
		t.Fatal("walk after dash rejected", p.Pos)
	}
	far := domain.Pos{MapID: landing.MapID, X: step.X + 5000, Y: step.Y}
	s.onMove(MoveTo{ID: p.ID, To: far, ReceivedAt: time.Now()})
	if p.Pos != step {
		t.Fatal("speed check bypassed after dash")
	}
}

// A report the client sent before it received the dash must not drag the
// player back toward the origin while their own client sits at the landing.
func TestDashDropsStalePreDashReport(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Warlock, domain.Learned{10432: 1})
	p.Stats.MoveSpeed = 100 // the fixture walks at speed 0; give the speed window real credit
	from := p.Pos
	if !s.skillDash(p, domain.Pos{MapID: from.MapID, X: from.X + 120, Y: from.Y}, 0) {
		t.Fatal("dash failed")
	}
	landing := p.Pos
	s.outbox = nil
	s.onMove(MoveTo{ID: p.ID, To: domain.Pos{MapID: from.MapID, X: from.X + 10, Y: from.Y}, ReceivedAt: time.Now()})
	if p.Pos != landing || dashReportMoves(s, p.ID) != 0 {
		t.Fatal("stale pre-dash report moved the player back", p.Pos)
	}
	// A report near the landing is a real post-dash step and releases the guard.
	step := domain.Pos{MapID: landing.MapID, X: landing.X + 15, Y: landing.Y}
	s.onMove(MoveTo{ID: p.ID, To: step, ReceivedAt: time.Now()})
	if p.Pos != step {
		t.Fatal("post-dash step rejected", p.Pos)
	}
	if _, guarded := s.dashLanding[p.ID]; guarded {
		t.Fatal("guard kept after an accepted step")
	}
	// After the grace window a report toward the origin is ordinary walking again.
	if !s.skillDash(p, domain.Pos{MapID: from.MapID, X: step.X + 120, Y: step.Y}, 0) {
		t.Fatal("second dash failed")
	}
	s.tick += domain.Ticks(dashMaxMS + dashReportGraceMS)
	back := domain.Pos{MapID: from.MapID, X: p.Pos.X - 100, Y: p.Pos.Y}
	s.onMove(MoveTo{ID: p.ID, To: back, ReceivedAt: time.Now()})
	if p.Pos != back {
		t.Fatal("guard outlived its grace window", p.Pos)
	}
}

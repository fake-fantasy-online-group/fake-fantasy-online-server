package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

// Coordinates and polygons are the original door pair reconciled by migration75.
// Exercise actor ownership transfers in both directions, not only SQL rows.
func TestZhoutianOriginalDoorRoundTrip(t *testing.T) {
	r, rig := twoScenes(t)
	town, _ := r.Ensure(domain.SceneID{MapID: 7})
	room, _ := r.Ensure(domain.SceneID{MapID: 506})
	entrance := domain.NewPortal(domain.Teleport{ProcID: 5, To: domain.Pos{MapID: 506, X: 498, Y: 541}, ToDir: 67, Area: domain.Polygon{{X: 9250, Y: 5980}, {X: 9385, Y: 6060}, {X: 9250, Y: 6140}, {X: 9115, Y: 6060}}})
	exit := domain.NewPortal(domain.Teleport{ProcID: 1, To: domain.Pos{MapID: 7, X: 9330, Y: 6099}, ToDir: 67, Area: domain.Polygon{{X: 583, Y: 580}, {X: 630, Y: 620}, {X: 675, Y: 591}, {X: 642, Y: 550}}})
	inspect(t, town, rig, func(s *Scene) { s.portals = []domain.Portal{entrance} })
	inspect(t, room, rig, func(s *Scene) { s.portals = []domain.Portal{exit} })
	town.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "甲", Pos: domain.Pos{MapID: 7, X: 9441, Y: 6195}}, Sink: &fakeSink{}})
	inspect(t, town, rig, func(*Scene) {})
	town.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 9250, Y: 6060}})
	inspect(t, town, rig, func(*Scene) {})
	inspect(t, town, rig, func(s *Scene) {
		if s.PlayerCount() != 0 {
			t.Fatal("entry did not transfer town ownership")
		}
	})
	inspect(t, room, rig, func(s *Scene) {
		p := s.EntityAt(1)
		if p == nil || p.Pos.X != 498 || p.Pos.Y != 541 {
			t.Fatal("entry landing differs from original room")
		}
	})
	// Arrival must not immediately bounce into the return gate.
	inspect(t, room, rig, func(s *Scene) {
		if s.PlayerCount() != 1 {
			t.Fatal("room arrival bounced back")
		}
	})
	room.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 506, X: 630, Y: 585}})
	inspect(t, room, rig, func(*Scene) {})
	inspect(t, room, rig, func(s *Scene) {
		if s.PlayerCount() != 0 {
			t.Fatal("exit did not transfer room ownership")
		}
	})
	inspect(t, town, rig, func(s *Scene) {
		p := s.EntityAt(1)
		if p == nil || p.Pos.X != 9330 || p.Pos.Y != 6099 {
			t.Fatal("return landing differs from corrected native exit")
		}
	})
	inspect(t, town, rig, func(s *Scene) {
		if s.PlayerCount() != 1 {
			t.Fatal("town landing bounced into entrance")
		}
	})
}

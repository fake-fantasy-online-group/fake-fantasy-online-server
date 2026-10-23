package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"

// Snapshot at the scene boundary, before effective appearance reaches the session.
// Every login, map snapshot and appearance refresh shares this authoritative path.
func (s *Scene) withNativeAttackRange(ev event.Event) event.Event {
	e := s.entities[ev.Subject()]
	if e == nil || e.Player == nil {
		return ev
	}
	native := e.Look.AtkDist
	if e.Player.Char != nil && e.Player.Char.Appear.AtkDist > 0 {
		native = e.Player.Char.Appear.AtkDist
	}
	switch x := ev.(type) {
	case event.SelfEntered:
		x.NativeAtkDist = native
		return x
	case event.EntitySpawned:
		x.NativeAtkDist = native
		return x
	case event.AppearanceChanged:
		x.NativeAtkDist = native
		return x
	}
	return ev
}

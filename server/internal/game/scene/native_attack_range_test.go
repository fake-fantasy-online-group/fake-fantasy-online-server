package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestNativeAttackRangeAllScenePresentationEdges(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "JK-A01")
	p.Player.Char.Appear.AtkDist = 75
	p.Look.AtkDist = 350
	visible := p.Player.Char.Clone()
	visible.Appear = p.Look.Appearance
	events := []event.Event{event.SelfEntered{ID: p.ID, Char: visible}, s.spawnEvent(p), event.AppearanceChanged{Who: p.ID, Appearance: p.Look.Appearance}}
	for _, ev := range events {
		s.outbox = nil
		s.emitTo(p.ID, ev)
		var native uint16
		switch x := s.outbox[0].ev.(type) {
		case event.SelfEntered:
			native = x.NativeAtkDist
			if x.Char.Appear.AtkDist != 350 {
				t.Fatal("lost effective self range")
			}
		case event.EntitySpawned:
			native = x.NativeAtkDist
		case event.AppearanceChanged:
			native = x.NativeAtkDist
		}
		if native != 75 {
			t.Fatalf("%T native %d", ev, native)
		}
	}
	p.Player.Char.Appear.AtkDist = 200
	s.outbox = nil
	s.emit(event.AppearanceChanged{Who: p.ID, Appearance: p.Look.Appearance})
	if s.outbox[0].ev.(event.AppearanceChanged).NativeAtkDist != 200 {
		t.Fatal("equipment refresh stale native")
	}
	p.Player.Char.Appear.AtkDist = 0
	s.outbox = nil
	s.emitTo(p.ID, event.AppearanceChanged{Who: p.ID, Appearance: domain.Appearance{AtkDist: 350}})
	if s.outbox[0].ev.(event.AppearanceChanged).NativeAtkDist != 350 {
		t.Fatal("legacy native-zero fallback")
	}
}

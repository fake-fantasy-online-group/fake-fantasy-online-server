package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestGroundEffectRequiresNegotiationAndMap(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "JK-A01")
	g := event.GroundEffect{Owner: p.ID, MapID: s.id.MapID, Key: "ground", Effect: "FX"}
	if s.canDeliverWorldEvent(p.ID, g) {
		t.Fatal("legacy received extension")
	}
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "capabilities", Capabilities: []string{domain.EndgameV1, domain.EndgameGroundFXV1}})
	if !p.Player.EndgameGroundFX || !s.canDeliverWorldEvent(p.ID, g) {
		t.Fatal("negotiated ground not delivered")
	}
	if p.Player.EndgameRiftPages {
		t.Fatal("ground-only opted into unrelated capability")
	}
	if !p.CapturePlayerRuntime(s.tick).EndgameGroundFX {
		t.Fatal("handoff lost capability")
	}
	s.mapLoads = map[domain.EntityID]*mapLoad{p.ID: {}}
	if s.canDeliverWorldEvent(p.ID, g) {
		t.Fatal("loading player received visual")
	}
	delete(s.mapLoads, p.ID)
	p.Player.Sink.(*fakeSink).take()
	s.outbox = nil
	s.emit(g)
	s.flush()
	events := p.Player.Sink.(*fakeSink).take()
	if len(events) != 1 {
		t.Fatalf("negotiated broadcast not delivered: %d", len(events))
	}
	p.Player.EndgameGroundFX = false
	s.emit(g)
	s.flush()
	if len(p.Player.Sink.(*fakeSink).take()) != 0 {
		t.Fatal("unnegotiated broadcast delivered")
	}
	p.Player.EndgameGroundFX = true
	g.MapID++
	if s.canDeliverWorldEvent(p.ID, g) {
		t.Fatal("wrong map delivered")
	}
}

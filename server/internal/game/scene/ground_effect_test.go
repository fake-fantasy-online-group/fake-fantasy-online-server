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

func TestGroundEffectReplayRemainingLifetimeAndReplacement(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "JK-A01")
	s.tick = 10
	e := event.GroundEffect{Owner: p.ID, MapID: s.id.MapID, Pos: domain.Pos{X: 123, Y: 456}, Key: "zone", Effect: "FX", Mode: 1, DurationMS: 1000}
	s.emit(e)
	once := e
	once.Key = "burst"
	once.Mode = 0
	s.emit(once)
	s.outbox = nil
	s.tick = 13
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "capabilities", Capabilities: []string{domain.EndgameV1, domain.EndgameGroundFXV1}})
	s.flush()
	sink := p.Player.Sink.(*fakeSink)
	take := func() []event.GroundEffect {
		var out []event.GroundEffect
		for _, v := range sink.take() {
			if g, ok := v.(event.GroundEffect); ok {
				out = append(out, g)
			}
		}
		return out
	}
	replay := take()
	if len(replay) != 1 || replay[0].DurationMS != 700 || replay[0].Pos != e.Pos {
		t.Fatalf("late join replay %+v", replay)
	}
	s.tick = 16
	s.sendAllEntities(p.ID)
	s.flush()
	replay = take()
	if len(replay) != 1 || replay[0].DurationMS != 400 {
		t.Fatalf("reentry reset clock %+v", replay)
	}
	s.tick = 20
	s.replayGroundEffects(p.ID)
	s.flush()
	if len(take()) != 0 || len(s.groundEffects) != 0 {
		t.Fatal("expired zone replayed")
	}
	s.tick = 21
	s.emit(e)
	s.tick = 25
	s.emit(e)
	s.outbox = nil
	s.timer.advance(31)
	if len(s.groundEffects) != 1 {
		t.Fatal("old callback killed replacement")
	}
	s.tick = 35
	s.timer.advance(35)
	if len(s.groundEffects) != 0 {
		t.Fatal("deadline did not clear state")
	}
	s.flush()
	replay = take()
	if len(replay) != 1 || replay[0].Mode != 2 {
		t.Fatalf("expiry stop %+v", replay)
	}
}
func TestGroundEffectExplicitStopAndOwnerExit(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "JK-A01")
	p.Player.EndgameGroundFX = true
	e := event.GroundEffect{Owner: p.ID, MapID: s.id.MapID, Key: "zone", Effect: "FX", Mode: 1}
	s.emit(e)
	s.outbox = nil
	s.tick += 100
	s.replayGroundEffects(p.ID)
	if len(s.outbox) != 1 || s.outbox[0].ev.(event.GroundEffect).DurationMS != 0 {
		t.Fatal("indefinite duration altered")
	}
	e.Mode = 2
	e.Effect = ""
	s.emit(e)
	s.outbox = nil
	s.replayGroundEffects(p.ID)
	if len(s.outbox) != 0 {
		t.Fatal("stopped region replayed")
	}
	e.Mode = 1
	e.Effect = "FX"
	s.emit(e)
	s.outbox = nil
	s.removeOwnerGroundEffects(p.ID)
	if len(s.groundEffects) != 0 || len(s.outbox) != 1 || s.outbox[0].ev.(event.GroundEffect).Mode != 2 {
		t.Fatal("source exit failed to clear region")
	}
	s.outbox = nil
	s.replayGroundEffects(p.ID)
	if len(s.outbox) != 0 {
		t.Fatal("departed source replayed")
	}
}

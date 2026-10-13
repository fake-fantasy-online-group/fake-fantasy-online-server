package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"testing"
)

type replenishingHandoffSink struct {
	scene   *Scene
	entered int
	stopped bool
}

func (s *replenishingHandoffSink) Emit(event.Event) {}
func (s *replenishingHandoffSink) Close()           {}
func (s *replenishingHandoffSink) BeginMapLoad(_ domain.EntityID, _ domain.SceneID) (uint64, bool) {
	s.entered++
	if !s.stopped {
		s.scene.PostHandoff(s.nextEnter(domain.EntityID(s.entered + 1)))
	}
	return 0, true
}
func (s *replenishingHandoffSink) CompleteMapLoad(domain.EntityID, domain.SceneID, uint64) bool {
	return true
}
func (s *replenishingHandoffSink) nextEnter(id domain.EntityID) Enter {
	return Enter{ID: id, Char: &domain.Character{ID: int64(id), Name: "handoff", Level: 10, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Runtime: &entity.PlayerRuntime{}, Sink: s}
}

func TestHandoffAcceptedPrefixDoesNotStarveMailbox(t *testing.T) {
	s := New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: 1}, Mailbox: 2})
	sink := &replenishingHandoffSink{scene: s}
	if !s.PostHandoff(sink.nextEnter(1)) {
		t.Fatal("initial handoff rejected")
	}
	ran := false
	if !s.Post(Inspect{Fn: func(*Scene) { ran = true }, Done: make(chan struct{})}) {
		t.Fatal("mailbox command rejected")
	}
	// Every Enter adds a subsequent handoff. A drain-until-empty loop would never
	// reach this ordinary mailbox command; the accepted-prefix boundary must.
	s.drain()
	if !ran || sink.entered != 2 || len(s.handoffs) != 1 {
		t.Fatalf("mailbox=%v entered=%d pending=%d", ran, sink.entered, len(s.handoffs))
	}
	s.Stop()
	sink.stopped = true
	if s.PostHandoff(sink.nextEnter(4)) {
		t.Fatal("stopped scene accepted a new owner")
	}
	s.drainForShutdown()
	if sink.entered != 3 || len(s.handoffs) != 0 || len(s.players) != 3 {
		t.Fatal("accepted prefix was lost or repeated during stop")
	}
}

type handoffLifecycleSink struct {
	panicBegin bool
	completed  int
}

func (*handoffLifecycleSink) Emit(event.Event) {}
func (*handoffLifecycleSink) Close()           {}
func (s *handoffLifecycleSink) BeginMapLoad(domain.EntityID, domain.SceneID) (uint64, bool) {
	if s.panicBegin {
		panic("injected unrelated target Enter failure")
	}
	return 1, true
}
func (s *handoffLifecycleSink) CompleteMapLoad(domain.EntityID, domain.SceneID, uint64) bool {
	s.completed++
	return true
}

func TestHandoffPanicPreservesDequeuedLifecycle(t *testing.T) {
	for _, leave := range []bool{false, true} {
		for _, stop := range []bool{false, true} {
			name := "ready"
			if leave {
				name = "leave"
			}
			if stop {
				name += "_stop"
			}
			t.Run(name, func(t *testing.T) {
				s := New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: 1}, Mailbox: 4})
				owner := &handoffLifecycleSink{}
				enter := func(id domain.EntityID, sink event.Sink) Enter {
					return Enter{ID: id, Char: &domain.Character{ID: int64(id), Name: "owner", Level: 10, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Runtime: &entity.PlayerRuntime{}, Sink: sink}
				}
				s.execTracked(enter(1, owner))
				s.flush()
				s.Post(Inspect{Fn: func(*Scene) { s.PostHandoff(enter(2, &handoffLifecycleSink{panicBegin: true})) }, Done: make(chan struct{})})
				var cmd Command = MapReady{ID: 1, Scene: s.id, Epoch: 1}
				if leave {
					cmd = Leave{ID: 1, Reason: "lifecycle after prior handoff"}
				}
				if !s.Post(cmd) {
					t.Fatal("lifecycle enqueue failed")
				}
				if !s.frame() {
					t.Fatal("single unrelated handoff panic abandoned scene")
				}
				if s.pendingMailbox == nil {
					t.Fatal("dequeued lifecycle lost behind unrelated handoff panic")
				}
				if stop {
					s.Stop()
					s.drainForShutdown()
				} else {
					if !s.frame() {
						t.Fatal("could not retry pending lifecycle")
					}
				}
				if s.pendingMailbox != nil {
					t.Fatal("completed lifecycle remained replayable")
				}
				if leave {
					if s.players[1] != nil {
						t.Fatal("preserved Leave did not retire player")
					}
				} else {
					if s.mapLoads[1] != nil || owner.completed != 1 {
						t.Fatal("preserved MapReady did not release exactly one first snapshot")
					}
				}
				s.drain()
				if owner.completed > 1 {
					t.Fatal("retained command executed more than once")
				}
			})
		}
	}
}

func TestHandoffPreservedMailboxOwnPanicDoesNotReplay(t *testing.T) {
	s := New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: 1}, Mailbox: 4})
	bad := Enter{ID: 2, Char: &domain.Character{ID: 2, Name: "bad", Level: 10, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Runtime: &entity.PlayerRuntime{}, Sink: &handoffLifecycleSink{panicBegin: true}}
	s.Post(Inspect{Fn: func(*Scene) { s.PostHandoff(bad) }, Done: make(chan struct{})})
	calls := 0
	s.Post(Inspect{Fn: func(*Scene) { calls++; panic("own command failed after side effect") }, Done: make(chan struct{})})
	s.frame()
	if s.pendingMailbox == nil || calls != 0 {
		t.Fatal("command not preserved before own execution")
	}
	s.frame()
	if s.pendingMailbox != nil || calls != 1 {
		t.Fatal("own execution was not removed before panic")
	}
	s.frame()
	if calls != 1 {
		t.Fatal("command side effect replayed after its own panic")
	}
}

func TestHandoffAbandonFinalizesPreservedUnenteredCharacter(t *testing.T) {
	s := New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: 1}, Mailbox: 4})
	bad := Enter{ID: 2, Char: &domain.Character{ID: 2, Name: "bad", Level: 10, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Runtime: &entity.PlayerRuntime{}, Sink: &handoffLifecycleSink{panicBegin: true}}
	saver := newRecordSaver()
	s.Post(Inspect{Fn: func(*Scene) { s.PostHandoff(bad) }, Done: make(chan struct{})})
	s.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "not yet entered", Level: 12, Pos: domain.Pos{MapID: 1, X: 100, Y: 100}}, Sink: nullSink{}, FinalSaver: saver})
	s.frame()
	if s.pendingMailbox == nil || s.players[1] != nil {
		t.Fatal("fixture did not preserve unentered ownership")
	}
	s.Stop()
	s.finalizePendingEnters()
	if s.pendingMailbox != nil || saver.count() != 1 {
		t.Fatal("abandon lost or duplicated preserved character save")
	}
	if lv, ok := saver.levelOf(100); !ok || lv != 12 {
		t.Fatal("wrong preserved snapshot")
	}
	s.finalizePendingEnters()
	if saver.count() != 1 {
		t.Fatal("finalization duplicated preserved owner")
	}
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

// Exercise the production skill -> monster death -> guardian arrival -> monster
// cleanup -> cancelPendingCombat call chain, rather than simulating a short slice.
func TestCastQueueRiftGuardianCleanupDuringImpact(t *testing.T) {
	s, _, _ := arrivalRiftFixture(t)
	r := s.rift.data.Run
	r.Progress = r.Rules.ProgressMax - r.Rules.NormalProgress
	p := s.players[1]
	p.Stats.MAtk = 1000000
	var target domain.EntityID
	for id, m := range s.monsters {
		if r.Monsters[id].Class == 0 {
			target = id
			m.HP = 1
			break
		}
	}
	if target == 0 {
		t.Fatal("missing normal monster")
	}
	s.casts = []pendingCast{{src: p.ID, targets: []domain.EntityID{target}, def: 火球术, released: true}}
	s.stepCastsAt(time.Now(), false)
	if r.Phase != domain.RiftBossArriving || len(s.monsters) != 0 {
		t.Fatalf("arrival not triggered: phase=%v monsters=%d", r.Phase, len(s.monsters))
	}
	if len(s.casts) != 0 {
		t.Fatalf("settled casts retained: %d", len(s.casts))
	}
}

// A one-shot RNG callback puts mutations exactly inside a production special
// effect, including enqueue/reallocation not currently done by production effects.
type castQueueCallbackSource struct{ callback func() }

func (r *castQueueCallbackSource) Seed(int64) {}
func (r *castQueueCallbackSource) Int63() int64 {
	if f := r.callback; f != nil {
		r.callback = nil
		f()
	}
	return 1 << 32
}

func TestCastQueueReentrantCancellationAndAppend(t *testing.T) {
	for _, mode := range []string{"cancel", "teleport", "clear"} {
		t.Run(mode, func(t *testing.T) {
			s, ids := skillScene(t)
			join(t, s, 1, 100, "first", 100, 100)
			join(t, s, 2, 200, "second", 100, 100)
			now := time.Now()
			future := now.Add(time.Hour)
			wait := func(src domain.EntityID, id domain.SkillID, targets ...domain.EntityID) pendingCast {
				d := 火弹术
				d.ID = id
				return pendingCast{src: src, def: d, targets: targets, releaseAtWall: future}
			}
			at := domain.Pos{MapID: 14, X: 200, Y: 200}
			trigger := pendingCast{src: 1, released: true, movementAt: &at, def: domain.SkillDef{ID: 999, Effect: domain.SkillEffect{Kind: domain.EffectTeleportPoint}}}
			s.casts = []pendingCast{wait(1, 101, ids[0]), wait(2, 102, ids[0]), trigger, wait(1, 103, ids[0]), wait(2, 104, 1, ids[0])}
			originalCapacity := cap(s.casts)
			s.rng = rand.New(&castQueueCallbackSource{callback: func() {
				switch mode {
				case "cancel":
					s.cancelPendingCombat(1)
				case "teleport":
					s.onTeleport(Teleport{ID: 1, To: s.id, At: at, ClientMapReady: true})
				case "clear":
					s.cancelPendingCombat(1, 2)
				}
				// Force a new backing array, and make new casts immediately due. The
				// bounded current pass must neither execute nor drop any of them.
				for i := 0; i < originalCapacity+1; i++ {
					c := wait(2, domain.SkillID(200+i), ids[0])
					c.releaseAtWall = time.Time{}
					s.casts = append(s.casts, c)
				}
				s.stepCastsAt(now, false) // Reentrant stepping must not take queue ownership.
			}})
			hp := s.entities[ids[0]].HP
			s.stepCastsAt(now, false)
			if s.entities[ids[0]].HP != hp {
				t.Fatal("callback-enqueued cast settled in the same pass")
			}
			var want []domain.SkillID
			if mode != "clear" {
				want = []domain.SkillID{102, 104}
			}
			for i := 0; i < originalCapacity+1; i++ {
				want = append(want, domain.SkillID(200+i))
			}
			var got []domain.SkillID
			for _, c := range s.casts {
				got = append(got, c.def.ID)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("queue order/liveness: got %v want %v", got, want)
			}
			if mode != "clear" && !reflect.DeepEqual(s.casts[1].targets, []domain.EntityID{ids[0]}) {
				t.Fatal("target-only cancellation lost surviving AOE target", s.casts[1].targets)
			}
			if s.steppingCasts {
				t.Fatal("queue ownership not released")
			}
			for _, c := range s.casts[len(s.casts):cap(s.casts)] {
				if c.src != 0 {
					t.Fatal("stale tail retained")
				}
			}
		})
	}
}

func TestCastQueueRiftCleanupCancelsEarlierAndLaterWaiting(t *testing.T) {
	s, _, _ := arrivalRiftFixture(t)
	r := s.rift.data.Run
	r.Progress = r.Rules.ProgressMax - r.Rules.NormalProgress
	p := s.players[1]
	p.Stats.MAtk = 1000000
	var ids []domain.EntityID
	for id, m := range s.monsters {
		if r.Monsters[id].Class == 0 {
			ids = append(ids, id)
			m.HP = 1
		}
	}
	if len(ids) < 2 {
		t.Fatal("need two monsters")
	}
	now := time.Now()
	future := now.Add(time.Hour)
	waiting := func(id domain.SkillID, targets ...domain.EntityID) pendingCast {
		d := 火球术
		d.ID = id
		return pendingCast{src: p.ID, def: d, targets: targets, releaseAtWall: future}
	}
	s.casts = []pendingCast{
		waiting(101, ids[0]), waiting(102, p.ID),
		{src: p.ID, def: 火球术, targets: ids, released: true},
		waiting(103, ids[1]), waiting(104, ids[1], p.ID),
	}
	s.stepCastsAt(now, false)
	if r.Phase != domain.RiftBossArriving {
		t.Fatal("guardian arrival missing")
	}
	if len(s.casts) != 2 || s.casts[0].def.ID != 102 || s.casts[1].def.ID != 104 {
		t.Fatalf("waiting casts resurrected or lost: %+v", s.casts)
	}
	for _, c := range s.casts {
		if !reflect.DeepEqual(c.targets, []domain.EntityID{p.ID}) {
			t.Fatalf("removed monster target retained: %v", c.targets)
		}
	}
}

func TestCastQueueOwnershipReleasedAfterEffectPanic(t *testing.T) {
	s, ids := skillScene(t)
	join(t, s, 1, 100, "caster", 100, 100)
	p := s.players[1]
	p.Stats.MAtk = 10
	now := time.Now()
	at := p.Pos
	wait := pendingCast{src: p.ID, def: 火球术, targets: []domain.EntityID{ids[0]}, releaseAtWall: now.Add(time.Hour)}
	trigger := pendingCast{src: p.ID, released: true, movementAt: &at, def: domain.SkillDef{ID: 999, Effect: domain.SkillEffect{Kind: domain.EffectTeleportPoint}}}
	ready := pendingCast{src: p.ID, def: 火弹术, targets: []domain.EntityID{ids[0]}, released: true}
	s.casts = []pendingCast{wait, trigger, ready}
	s.rng = rand.New(&castQueueCallbackSource{callback: func() { panic("injected effect panic") }})
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		s.stepCastsAt(now, false)
	}()
	if s.steppingCasts {
		t.Fatal("recovered panic permanently retained cast ownership")
	}
	hp := s.entities[ids[0]].HP
	s.stepCastsAt(now, false)
	if s.entities[ids[0]].HP >= hp {
		t.Fatal("later surviving cast did not resume")
	}
	if len(s.casts) != 1 || s.casts[0].def.ID != wait.def.ID || s.casts[0].releaseAtWall != wait.releaseAtWall {
		t.Fatalf("cast resumed twice or lost waiting work: %+v", s.casts)
	}
	hp = s.entities[ids[0]].HP
	s.stepCastsAt(now, false)
	if s.entities[ids[0]].HP != hp {
		t.Fatal("settled cast repeated after panic")
	}
}

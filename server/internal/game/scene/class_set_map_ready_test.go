package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestClassSetDragonRealTransferReconcilesSelfAppearance(t *testing.T) {
	for _, broken := range []bool{false, true} {
		name := "intact"
		if broken {
			name = "sixth piece breaks while loading"
		}
		t.Run(name, func(t *testing.T) {
			fixture, p := meleeSetFixture(t, 5, 6)
			r, rig := twoScenes(t)
			src, _ := r.Ensure(domain.SceneID{MapID: 7})
			dst, _ := r.Ensure(domain.SceneID{MapID: 14})
			inspect(t, src, rig, func(s *Scene) { s.items = fixture.items })
			inspect(t, dst, rig, func(s *Scene) { s.items = fixture.items })
			sink := &riftReconnectLoadingSink{}
			src.Post(Enter{ID: 1, Char: p.Player.Char.Clone(), Worn: p.Player.Worn.Clone(), Sink: sink})
			inspect(t, src, rig, func(s *Scene) {
				if s.players[1] == nil || s.players[1].Look.EquipView[0] != 1371 {
					t.Error("initial dragon missing")
				}
			})
			sink.take()
			src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 14}, At: domain.Pos{MapID: 14, X: 9768, Y: 1982}, Dir: 67})
			inspect(t, src, rig, func(s *Scene) {
				if s.players[1] != nil {
					t.Error("source retained player")
				}
			})
			inspect(t, dst, rig, func(s *Scene) {
				q := s.players[1]
				if q == nil || s.mapLoads[1] == nil {
					t.Error("transfer did not enter loading barrier")
					return
				}
				if broken {
					st := q.Player.Worn.At(domain.SlotBody)
					st.Durability = 0
					q.Player.Worn.Set(domain.SlotBody, st)
					s.refreshStats(q)
				}
				if q.Player.Char.Appear.EquipView[0] == 1371 {
					t.Error("transfer persisted dragon")
				}
			})
			for _, ev := range sink.take() {
				if _, ok := ev.(event.AppearanceChanged); ok {
					t.Error("appearance leaked through loading barrier")
				}
				if _, ok := ev.(event.SelfEntered); ok {
					t.Error("transfer replayed login")
				}
			}
			dst.Post(MapReady{ID: 1, Scene: dst.ID(), Epoch: 17})
			var want domain.Appearance
			inspect(t, dst, rig, func(s *Scene) {
				q := s.players[1]
				if q == nil {
					t.Error("missing transferred player")
					return
				}
				want = q.Look.Appearance
				if broken && want.EquipView[0] == 1371 || !broken && want.EquipView[0] != 1371 {
					t.Error("wrong server appearance after ready")
				}
				if q.ID != 1 || q.Kind != domain.KindPlayer || s.snapshotOf(q).Char.Appear.EquipView[0] == 1371 {
					t.Error("identity or saved appearance corrupted")
				}
			})
			found := false
			for _, ev := range sink.take() {
				if _, ok := ev.(event.SelfEntered); ok {
					t.Error("map ready replayed login")
				}
				if a, ok := ev.(event.AppearanceChanged); ok && a.Who == 1 {
					found = true
					if a.Appearance != want {
						t.Error("self received stale appearance")
					}
				}
			}
			if !found {
				t.Fatal("map ready did not reconcile local appearance")
			}
		})
	}
}

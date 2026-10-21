package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
)

func TestClassSetRuntimeRealRouterTransfer(t *testing.T) {
	for _, suit := range []int32{4, 3, 2} {
		t.Run(map[int32]string{4: "unruly", 3: "elements", 2: "mana"}[suit], func(t *testing.T) {
			fixture, p := meleeSetFixture(t, suit, 2)
			if suit == 3 {
				p.Player.Char.Race = domain.Warlock
			}
			if suit == 2 {
				p.Player.Char.Race = domain.Healer
			}
			r, rig := twoScenes(t)
			src, _ := r.Ensure(domain.SceneID{MapID: 7})
			dst, _ := r.Ensure(domain.SceneID{MapID: 14})
			inspect(t, src, rig, func(s *Scene) { s.items = fixture.items; s.tick = 10000 })
			inspect(t, dst, rig, func(s *Scene) { s.items = fixture.items; s.tick = 1000 })
			src.Post(Enter{ID: 1, Char: p.Player.Char.Clone(), Worn: p.Player.Worn.Clone(), Sink: &fakeSink{}})
			var initial entity.ClassSetRuntime
			inspect(t, src, rig, func(s *Scene) {
				q := s.players[1]
				q.MP = 0
				switch suit {
				case 4:
					for i := 0; i < 3; i++ {
						s.classSetMeleeCastReleased(q, &domain.SkillDef{ID: classSetTaunt})
					}
					st := s.equipment.unruly[1]
					st.expires = s.tick + domain.Ticks(17000)
					s.equipment.unruly[1] = st
				case 3:
					st := s.casterSetState(q)
					st.fireUntil = s.tick + domain.Ticks(2000)
					st.iceUntil = s.tick + domain.Ticks(1500)
				case 2:
					s.casterSetState(q).nextMana = s.tick + domain.Ticks(2000)
				}
				initial = s.captureClassSetRuntime(q)
			})
			for trip := 0; trip < 2; trip++ {
				src.Post(Teleport{ID: 1, To: dst.ID(), At: domain.Pos{MapID: dst.ID().MapID, X: 9768, Y: 1982}})
				inspect(t, src, rig, func(s *Scene) {
					if s.players[1] != nil {
						t.Error("source retained player")
					}
					if s.equipment.unruly[1].stacks != 0 || s.equipment.classSetCasters[1] != nil {
						t.Error("source retained set state")
					}
				})
				inspect(t, dst, rig, func(s *Scene) {
					q := s.players[1]
					if q == nil {
						t.Fatal("destination missing player")
					}
					got := s.captureClassSetRuntime(q)
					check := func(name string, a, b domain.Tick) {
						if b == 0 || b > a || a-b > 20 {
							t.Errorf("%s remaining %d -> %d", name, a, b)
						}
					}
					switch suit {
					case 4:
						if got.UnrulyStacks != 3 {
							t.Errorf("stacks=%d", got.UnrulyStacks)
						}
						check("unruly", initial.UnrulyFor, got.UnrulyFor)
					case 3:
						check("fire", initial.FireFor, got.FireFor)
						check("ice", initial.IceFor, got.IceFor)
						views := s.classSetCasterPresentation(q)
						if len(views) != 2 || views[0].icon.RemainMS != int32(got.FireFor.Millis()) || views[1].icon.RemainMS != int32(got.IceFor.Millis()) {
							t.Errorf("transferred display timer %+v", views)
						}
					case 2:
						if !got.ManaScheduled || q.MP != 0 {
							t.Errorf("transfer regenerated mana: scheduled=%v MP=%d", got.ManaScheduled, q.MP)
						}
						check("mana", initial.ManaReadyIn, got.ManaReadyIn)
					}
					initial = got
				})
				src, dst = dst, src
			}
			inspect(t, src, rig, func(s *Scene) {
				q := s.players[1]
				switch suit {
				case 4:
					s.tick = s.equipment.unruly[1].expires
					if s.classSetUnrulyStacks(q) != 0 {
						t.Error("expired stacks survived")
					}
				case 3:
					st := s.casterSetState(q)
					s.tick = st.fireUntil
					if s.casterSetOutgoingDamage(q, domain.HurtFire, 100) != 100 {
						t.Error("expired elemental buff survived")
					}
				case 2:
					st := s.casterSetState(q)
					s.tick = st.nextMana
					s.stepCasterSets()
					want := q.MaxMP * 3 / 100
					if q.MP != want {
						t.Errorf("mana at original deadline %d want %d", q.MP, want)
					}
					s.stepCasterSets()
					if q.MP != want {
						t.Error("double mana tick")
					}
				}
			})
		})
	}
}

func TestClassSetRuntimeFiltersInvalidGrants(t *testing.T) {
	for _, suit := range []int32{4, 3, 2} {
		for _, invalid := range []string{"dead", "broken", "expired"} {
			t.Run(map[int32]string{4: "sword", 3: "mage", 2: "healer"}[suit]+"/"+invalid, func(t *testing.T) {
				s, p := meleeSetFixture(t, suit, 2)
				if suit == 3 {
					p.Player.Char.Race = domain.Warlock
				}
				if suit == 2 {
					p.Player.Char.Race = domain.Healer
				}
				s.tick = 100
				input := entity.ClassSetRuntime{UnrulyStacks: 3, UnrulyFor: 30, FireFor: 20, IceFor: 10, ManaScheduled: true, ManaReadyIn: 15}
				s.restoreClassSetRuntime(p, input)
				if invalid == "dead" {
					p.HP = 0
				}
				if invalid == "broken" {
					st := p.Player.Worn.At(domain.SlotHead)
					st.Durability = 0
					p.Player.Worn.Set(domain.SlotHead, st)
					s.refreshStats(p)
				}
				if invalid == "expired" {
					s.tick += 40
				}
				got := s.captureClassSetRuntime(p)
				if got.UnrulyStacks != 0 || got.FireFor != 0 || got.IceFor != 0 {
					t.Errorf("invalid buff captured: %+v", got)
				}
				if invalid != "expired" && got.ManaScheduled {
					t.Error("invalid mana clock captured")
				}
				// A normal handoff is not a persisted login: an empty runtime restores no grants.
				fresh, q := meleeSetFixture(t, suit, 2)
				fresh.restoreClassSetRuntime(q, entity.ClassSetRuntime{})
				if fresh.classSetUnrulyStacks(q) != 0 {
					t.Error("login invented stacks")
				}
			})
		}
	}
}

func TestClassSetRuntimeRebasesExactDurationsAndDueMana(t *testing.T) {
	for _, suit := range []int32{4, 3, 2} {
		src, p := meleeSetFixture(t, suit, 2)
		dst, q := meleeSetFixture(t, suit, 2)
		if suit == 3 {
			p.Player.Char.Race = domain.Warlock
			q.Player.Char.Race = domain.Warlock
		}
		if suit == 2 {
			p.Player.Char.Race = domain.Healer
			q.Player.Char.Race = domain.Healer
		}
		src.tick = 90000
		dst.tick = 2
		input := entity.ClassSetRuntime{UnrulyStacks: 7, UnrulyFor: 123, FireFor: 43, IceFor: 17, ManaScheduled: true, ManaReadyIn: 9}
		src.restoreClassSetRuntime(p, input)
		src.tick += 5
		r := src.captureClassSetRuntime(p)
		dst.restoreClassSetRuntime(q, r)
		got := dst.captureClassSetRuntime(q)
		if got != r {
			t.Fatalf("suit %d changed remaining durations: %+v -> %+v", suit, r, got)
		}
		switch suit {
		case 4:
			if got.UnrulyFor != 118 || got.UnrulyStacks != 7 {
				t.Fatal(got)
			}
		case 3:
			if got.FireFor != 38 || got.IceFor != 12 {
				t.Fatal(got)
			}
		case 2:
			if got.ManaReadyIn != 4 {
				t.Fatal(got)
			}
			src.tick += 20
			r = src.captureClassSetRuntime(p)
			if !r.ManaScheduled || r.ManaReadyIn != 0 {
				t.Fatalf("due clock underflowed: %+v", r)
			}
			dst.restoreClassSetRuntime(q, r)
			q.MP = 0
			dst.stepCasterSets()
			if q.MP != q.MaxMP*3/100 {
				t.Fatal("due mana lost")
			}
		}
	}
}

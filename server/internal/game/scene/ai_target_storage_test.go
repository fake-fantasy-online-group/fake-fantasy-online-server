package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"reflect"
	"testing"
)

func TestTargetCallerStorage(t *testing.T) {
	s, id := aiScene(t, 主动树妖, 100, 100)
	join(t, s, 1, 100, "target", 120, 100)
	m := s.monsters[id]
	m.Monster.Target = 1
	var view ai.Target
	got := s.targetInfoInto(m, &view)
	if got != &view || !got.Alive || got.Pos != s.players[1].Pos {
		t.Fatalf("wrong target: %+v", got)
	}
	old := view
	s.players[1].Pos.X++
	var next ai.Target
	s.targetInfoInto(m, &next)
	if view != old || next.Pos == old.Pos {
		t.Fatal("caller views alias or stale")
	}
	s.players[1].HP = 0
	if s.targetInfoInto(m, &next) == nil || next.Alive {
		t.Fatal("dead target must retain position and false Alive")
	}
	delete(s.entities, 1)
	if s.targetInfoInto(m, &next) != nil || m.Monster.Target != 0 {
		t.Fatal("missing target must drop aggro")
	}
	if s.targetInfoInto(m, &next) != nil {
		t.Fatal("zero target must return nil")
	}
}

func BenchmarkTargetInfo(b *testing.B) {
	for _, mode := range []string{"legacy", "caller-owned"} {
		b.Run(mode, func(b *testing.B) {
			s := &Scene{entities: map[domain.EntityID]*entity.Entity{1: {ID: 1, HP: 100}}}
			m := &entity.Entity{Monster: &entity.Monster{Target: 1}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var view ai.Target
				var got *ai.Target
				if mode == "legacy" {
					got = s.targetInfoLegacyReference(m)
				} else {
					got = s.targetInfoInto(m, &view)
				}
				if got == nil {
					b.Fatal("missing target")
				}
			}
		})
	}
}

func (s *Scene) targetInfoLegacyReference(m *entity.Entity) *ai.Target {
	id := m.Monster.Target
	if id == 0 {
		return nil
	}
	t, ok := s.entities[id]
	if !ok || entityInvisible(t) || s.entityMapLoading(t) || !s.equipmentCombatReady(t) {
		s.dropTarget(m) // 目标下线/传送离图：原地脱战，不回出生点
		return nil
	}
	return &ai.Target{Pos: t.Pos, Alive: t.Alive()}
}

func TestTargetRejectedViews(t *testing.T) {
	for _, mode := range []string{"invisible", "loading", "unready"} {
		t.Run(mode, func(t *testing.T) {
			s, id := aiScene(t, 主动树妖, 100, 100)
			join(t, s, 1, 100, "target", 120, 100)
			m := s.monsters[id]
			p := s.players[1]
			m.Monster.Target = 1
			switch mode {
			case "invisible":
				p.Status.Apply(domain.StatusDef{ID: 99999, Invisible: true, DurationSec: 100}, s.tick, p.ID)
			case "loading":
				s.mapLoads = map[domain.EntityID]*mapLoad{p.ID: {}}
			case "unready":
				s.requireGrowthV5 = true
				p.Player.EndgameGrowthV5 = false
			}
			var out ai.Target
			if s.targetInfoInto(m, &out) != nil || m.Monster.Target != 0 {
				t.Fatal("rejected target did not drop aggro")
			}
		})
	}
}

// Compare the fill helper against the frozen allocating implementation, including
// dropTarget's authoritative observer stop and all monster state side effects.
func TestTargetInfoStorageParity(t *testing.T) {
	for _, mode := range []string{"alive", "dead", "zero", "missing", "invisible", "loading", "unready", "grace", "range"} {
		t.Run(mode, func(t *testing.T) {
			old, oldID := aiScene(t, 主动树妖, 100, 100)
			got, gotID := aiScene(t, 主动树妖, 100, 100)
			oldSink := join(t, old, 1, 100, "observer", 120, 100)
			gotSink := join(t, got, 1, 100, "observer", 120, 100)
			for _, s := range []*Scene{old, got} {
				var m *entity.Entity
				for _, candidate := range s.monsters {
					m = candidate
				}
				m.Monster.Target = 1
				m.Monster.CombatPathActive = true
				m.Monster.TauntPower = 99
				p := s.players[1]
				switch mode {
				case "dead":
					p.HP = 0
				case "zero":
					m.Monster.Target = 0
				case "missing":
					m.Monster.Target = 999999
				case "invisible":
					p.Status.Apply(domain.StatusDef{ID: 99999, Invisible: true, DurationSec: 100}, s.tick, p.ID)
				case "loading":
					s.mapLoads = map[domain.EntityID]*mapLoad{p.ID: {}}
				case "unready":
					s.requireGrowthV5 = true
					p.Player.EndgameGrowthV5 = false
				case "grace":
					p.Player.Protect(s.tick + 100)
				case "range":
					p.Pos.X = 10000
				}
			}
			oldSink.take()
			gotSink.take()
			var view ai.Target
			want := old.targetInfoLegacyReference(old.monsters[oldID])
			actual := got.targetInfoInto(got.monsters[gotID], &view)
			if !reflect.DeepEqual(want, actual) {
				t.Fatalf("view: want %+v, got %+v", want, actual)
			}
			if !reflect.DeepEqual(old.monsters[oldID], got.monsters[gotID]) {
				t.Fatal("monster state diverged")
			}
			if want, actual := oldSink.take(), gotSink.take(); !reflect.DeepEqual(want, actual) {
				t.Fatalf("observer events: want %+v, got %+v", want, actual)
			}
		})
	}
}

func TestTargetInfoStorageOverwritesReusedView(t *testing.T) {
	s, id := aiScene(t, 主动树妖, 100, 100)
	join(t, s, 1, 100, "target", 120, 100)
	m := s.monsters[id]
	m.Monster.Target = 1
	var view ai.Target
	if s.targetInfoInto(m, &view) != &view || !view.Alive {
		t.Fatal("initial view")
	}
	s.players[1].Pos = domain.Pos{}
	s.players[1].HP = 0
	if s.targetInfoInto(m, &view) != &view || view != (ai.Target{}) {
		t.Fatalf("stale reused fields: %+v", view)
	}
	// Compatibility callers receive independent snapshots.
	s.players[1].HP = 100
	first := s.targetInfo(m)
	s.players[1].Pos.X = 42
	second := s.targetInfo(m)
	if first == second || first.Pos.X != 0 || second.Pos.X != 42 {
		t.Fatal("wrapper snapshots alias")
	}
}

func TestTargetInfoStorageTimerRefresh(t *testing.T) {
	for _, mode := range []string{"retarget", "dead", "missing", "invisible", "loading", "unready", "grace", "range"} {
		t.Run(mode, func(t *testing.T) {
			s, id := aiScene(t, 主动树妖, 100, 100)
			join(t, s, 1, 100, "first", 120, 100)
			join(t, s, 2, 101, "second", 130, 100)
			m := s.monsters[id]
			m.Monster.Target = 1
			rule := domain.MonsterSkillRule{Skill: domain.SkillDef{ID: 90001, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 200, PrepareMS: 100, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}}
			s.startMonsterSkill(m, rule, []domain.EntityID{1})
			p := s.players[1]
			switch mode {
			case "retarget":
				m.Monster.Target = 2
			case "dead":
				p.HP = 0
			case "missing":
				delete(s.entities, p.ID)
			case "invisible":
				p.Status.Apply(domain.StatusDef{ID: 99999, Invisible: true, DurationSec: 100}, s.tick, p.ID)
			case "loading":
				s.mapLoads = map[domain.EntityID]*mapLoad{p.ID: {}}
			case "unready":
				s.requireGrowthV5 = true
				p.Player.EndgameGrowthV5 = false
			case "grace":
				p.Player.Protect(s.tick + 100)
			case "range":
				p.Pos.X = 10000
			}
			s.tick = m.Monster.CastingUntil
			s.timer.advance(s.tick)
			if m.Monster.CastingSkill != 0 || m.Monster.CastingUntil != 0 {
				t.Fatal("chant state not cleared")
			}
			if mode != "retarget" {
				if len(s.casts) != 0 {
					t.Fatalf("invalid target queued %d casts", len(s.casts))
				}
				return
			}
			if len(s.casts) != 1 || !reflect.DeepEqual(s.casts[0].targets, []domain.EntityID{2}) || s.casts[0].visualTarget != 2 {
				t.Fatalf("timer did not refresh target: %+v", s.casts)
			}
			var reused ai.Target
			m.Monster.Target = 1
			s.targetInfoInto(m, &reused)
			reused = ai.Target{}
			if s.casts[0].targets[0] != 2 || m.Monster.SkillCasts[rule.Skill.ID] != 1 {
				t.Fatal("queued cast retained mutable target view")
			}
		})
	}
}

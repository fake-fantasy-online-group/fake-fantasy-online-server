package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Original source snapshots remain immutable. This explicit correction layer
// has separate provenance and is checked against actual read-only PG loaders.
func nativeRestoredSceneFixtures(t *testing.T) (domain.SkillTable, domain.StatusTable) {
	t.Helper()
	skills, statuses := realV5SkillFixtures(t)
	b, err := os.ReadFile("../../../../docs/daoist-endgame/original-skill-audit/restored-runtime-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Source   string
		Skills   []domain.SkillDef
		Statuses []domain.StatusDef
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source == "" || len(fixture.Skills) != 55 {
		t.Fatal("incomplete restored fixture", fixture.Source, len(fixture.Skills))
	}
	for _, d := range fixture.Skills {
		skills[domain.SkillKey{ID: d.ID, Level: d.Level}] = d
	}
	for _, d := range fixture.Statuses {
		statuses[domain.StatusKey{ID: d.ID, Level: d.Level}] = d
	}
	if err := domain.InstallV5SkillProgression(skills, catalogfixture.SkillProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
	return skills, statuses
}

func nativeMovementPlayer(t *testing.T, s *Scene, id domain.EntityID, skills domain.Learned) (*entity.Entity, *fakeSink) {
	t.Helper()
	sink := join(t, s, id, int64(id)*100, "root test", 100, 100)
	p := s.players[id]
	p.Player.Char.Race, p.Player.Char.Level = domain.Swordsman, 100
	p.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	p.Player.Char.Skills = skills
	p.Player.Char.Base = domain.Base{STR: 200, VIT: 1000, SPI: 1000, AGI: 20, DEX: 1000000}
	s.refreshStats(p)
	p.HP, p.MP = p.MaxHP, p.MaxMP
	return p, sink
}

func nativeApplyMovementSkill(t *testing.T, s *Scene, source, target *entity.Entity, id domain.SkillID, rank int32) {
	t.Helper()
	def, ok := s.skills.Get(id, rank)
	if !ok {
		t.Fatal("missing restored skill", id, rank)
	}
	if !s.applySkill(source, target, def, 0, false) || !target.Status.BlocksMovement() {
		t.Fatal("real skill failed to root", id, rank, target.Status)
	}
	if target.Status.Controlled() != domain.ControlNone || !canAct(target) || !canCast(target) || !canDealDamage(target) {
		t.Fatal("root escalated to action/cast/attack lock", id, rank)
	}
}

func TestNativeMovementOnlyPlayerStillAttacksAndCasts(t *testing.T) {
	for _, id := range []domain.SkillID{10111, 10221} {
		for _, rank := range []int32{1, 5, 20} {
			t.Run(fmt.Sprintf("%d/%d", id, rank), func(t *testing.T) {
				s := newTestScene(t, nil)
				s.skills, s.statuses = nativeRestoredSceneFixtures(t)
				p, sink := nativeMovementPlayer(t, s, 1, domain.Learned{10102: 1})
				source, _ := nativeMovementPlayer(t, s, 2, domain.Learned{id: min(rank, 20)})
				if id == 10221 {
					source.Player.Char.Race = domain.Assassin
				}
				target := putMonster(s, 900, 100000, domain.Stats{}, 140, 100)
				nativeApplyMovementSkill(t, s, source, p, id, rank)
				s.flush()
				sink.take()
				from := p.Pos
				to := from
				to.X++
				s.onMove(MoveTo{ID: p.ID, To: to})
				if p.Pos != from {
					t.Fatal("rooted player moved", from, p.Pos)
				}
				visible := s.visibleStatuses(p)
				if len(visible) == 0 || visible[0].buff.Immobile != 1 {
					t.Fatal("root did not expose immobile buff", visible)
				}
				before := target.HP
				s.onAttack(Attack{ID: p.ID, Target: target.ID, Blow: combat.Blow{GuaranteedHit: true}})
				if len(s.attacks) != 1 {
					t.Fatal("root rejected basic attack")
				}
				s.stepCombatAt(s.now().Add(time.Second), false)
				if target.HP >= before {
					t.Fatal("rooted basic attack failed to deal damage")
				}
				s.onUseSkill(UseSkill{ID: p.ID, Skill: 10102, Target: target.ID})
				if len(s.casts) != 1 {
					s.flush()
					t.Fatal("root rejected learned skill", sink.take())
				}
				before = target.HP
				s.stepCastsAt(s.now().Add(2*time.Second), false)
				s.stepCastsAt(s.now().Add(3*time.Second), false)
				if target.HP >= before || p.Pos != from {
					t.Fatal("rooted skill failed or moved player", target.HP, before, p.Pos)
				}
			})
		}
	}
}

func TestNativeMovementMonsterChaseAndWanderStayStill(t *testing.T) {
	for _, id := range []domain.SkillID{10111, 10221} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			s := newTestScene(t, nil)
			s.skills, s.statuses = nativeRestoredSceneFixtures(t)
			source, sink := nativeMovementPlayer(t, s, 1, domain.Learned{id: 5})
			if id == 10221 {
				source.Player.Char.Race = domain.Assassin
			}
			m := putMonster(s, 900, 100000, domain.Stats{MoveSpeed: 100}, 140, 100)
			m.Monster.Kind = domain.MonsterNormal
			nativeApplyMovementSkill(t, s, source, m, id, 5)
			s.flush()
			sink.take()
			from := m.Pos
			to := from
			to.X += 10
			s.moveEntity(m, to)
			m.Monster.NextRoamAt = s.tick
			if m.Monster.NextRoamAt == 0 {
				s.tick = 1
				m.Monster.NextRoamAt = 1
			}
			if !s.stepWander(m, ai.Params{MoveSpeed: 100}) || m.Pos != from || m.Monster.Roaming {
				t.Fatal("rooted monster began wandering", m.Pos, m.Monster)
			}
			s.flush()
			if moves := countOf[event.EntityMoved](sink.take()); moves != 0 {
				t.Fatal("rooted monster emitted false movement path", moves)
			}
			// Full target acquisition/AI is unchanged: a rooted monster may strike
			// a nearby target, while the shared locomotion entry remains blocked.
			m.Monster.Target = source.ID
			m.Stats.MinAtk, m.Stats.MaxAtk, m.Stats.Hit = 100, 100, 1000000
			before := source.HP
			s.onAttack(Attack{ID: m.ID, Target: source.ID, Blow: combat.Blow{GuaranteedHit: true}})
			s.stepCombatAt(s.now().Add(time.Second), false)
			if source.HP >= before {
				t.Fatal("rooted monster basic attack failed")
			}
		})
	}
}

func TestNativeMovementSilenceRemovalAndExpiry(t *testing.T) {
	for _, mode := range []string{"remove", "expire", "cleanse"} {
		t.Run(mode, func(t *testing.T) {
			s := newTestScene(t, nil)
			s.skills, s.statuses = nativeRestoredSceneFixtures(t)
			p, _ := nativeMovementPlayer(t, s, 1, domain.Learned{10102: 1})
			source, _ := nativeMovementPlayer(t, s, 2, domain.Learned{10111: 1})
			nativeApplyMovementSkill(t, s, source, p, 10111, 1)
			silence, _ := s.statuses.Get(domain.StatusSilence, 1)
			if !s.applyStatusDef(p, silence, source.ID) {
				t.Fatal("silence blocked")
			}
			if canMove(p) || canCast(p) || !canDealDamage(p) {
				t.Fatal("root+silence composition wrong")
			}
			p.Status.Remove(domain.StatusSilence)
			if canMove(p) || !canCast(p) {
				t.Fatal("removing silence also removed root or kept cast lock")
			}
			switch mode {
			case "remove":
				p.Status.Remove(10111)
			case "expire":
				s.tick += domain.Ticks(6000)
				s.stepStatus()
			case "cleanse":
				if !s.applyStatusDef(p, domain.StatusDef{ID: 1055, Level: 1, DurationSec: 5, TypeKnown: true, Type: domain.StatusGood, ClearHarmful: true}, p.ID) {
					t.Fatal("cleanse failed")
				}
			}
			if !canMove(p) || !canCast(p) || !canDealDamage(p) {
				t.Fatal("movement lock remained after release", mode)
			}
			from := p.Pos
			to := from
			to.X++
			s.onMove(MoveTo{ID: p.ID, To: to})
			if p.Pos != to {
				t.Fatal("movement not restored", mode, p.Pos, to)
			}
		})
	}
}

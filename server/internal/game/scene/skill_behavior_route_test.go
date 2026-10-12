package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
	"time"
)

func TestSkillBehaviorDragonkillUsesOriginalSingleTargetRoute(t *testing.T) {
	for _, rank := range []int32{1, 5, 20} {
		s := newTestScene(t, nil)
		join(t, s, 1, 100, "dragonkill", 100, 100)
		p := s.players[1]
		p.Player.Char.Race = domain.Race(1)
		p.Player.Char.Level = 100
		p.Player.Char.SkillAllocationVersion = 5
		p.Player.Char.Skills = domain.Learned{10105: rank}
		p.Player.EndgameSkillBehavior = true
		s.skills, s.statuses = realV5SkillFixtures(t)
		p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, Hit: 1 << 20}
		p.MP, p.MaxMP = 10000, 10000
		target := putMonster(s, 900, 100000, domain.Stats{}, 150, 100)
		neighbor := putMonster(s, 901, 100000, domain.Stats{}, 151, 100)
		rows, err := s.skillRanksForDisplay(p)
		if err != nil {
			t.Fatal(err)
		}
		c := rows[10105].Contract
		if c == nil || c.Kind != "special" || c.Current == nil || c.Current.InputMode != "entity" || c.Current.Area.Shape != "none" || !c.Current.Usable {
			t.Fatalf("typed special identity drift: %+v", c)
		}
		now := time.Unix(1700000000, 0)
		s.now = func() time.Time { return now }
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10105, Target: target.ID})
		if len(s.casts) != 1 || p.MP != 10000-c.Current.MPCost || len(s.casts[0].targets) != 1 || s.casts[0].targets[0] != target.ID {
			t.Fatalf("rank%d special single-target cast rejected/expanded: mp%d casts%+v", rank, p.MP, s.casts)
		}
		now = now.Add(time.Duration(c.Current.PrepareMS)*time.Millisecond + time.Second)
		s.stepCastsAt(now, false)
		s.stepCastsAt(now.Add(time.Second), false)
		if target.HP >= 100000 || neighbor.HP != 100000 {
			t.Fatalf("rank%d special failed or became area: %d/%d", rank, target.HP, neighbor.HP)
		}
		def, _ := s.skills.Get(10105, rank)
		if got := s.resolveTargets(p, def, neighbor.ID, &domain.Pos{X: 100, Y: 100}); len(got) != 1 || got[0] != neighbor.ID {
			t.Fatal("special entity route must ignore forged point center", got)
		}
		neighbor.Pos.X = 1000
		if got := s.resolveTargets(p, def, neighbor.ID, nil); len(got) != 0 {
			t.Fatal("special single target bypassed range", got)
		}
	}
}

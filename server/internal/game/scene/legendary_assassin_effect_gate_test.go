package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

// These tests assert paid MP, not merely the modifier attached to a SkillDef.
func TestAssassinEffectGateUtilityWindowActualPayment(t *testing.T) {
	for _, tc := range []struct {
		id             string
		trigger, query domain.SkillID
		pct            int32
	}{{"CK-U01", 10208, 10205, 55}, {"CK-U07", 10213, 10214, 45}} {
		t.Run(tc.id, func(t *testing.T) {
			var paid [2]int32
			for i, enabled := range []bool{false, true} {
				powers := []string{}
				if enabled {
					powers = append(powers, tc.id)
				}
				s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{tc.trigger: 1, tc.query: 1}, powers...)
				trigger := UseSkill{Skill: tc.trigger}
				query := UseSkill{Skill: tc.query, Target: m.ID}
				if tc.id == "CK-U07" {
					trigger.At = &m.Pos
					query.At = &m.Pos
					query.Target = 0
				}
				assassinRegressionCast(t, s, p, trigger)
				before := p.MP
				assassinRegressionCast(t, s, p, query)
				paid[i] = before - p.MP
			}
			if paid[0] <= 0 || paid[1] != (paid[0]*tc.pct+99)/100 {
				t.Fatalf("actual MP paid baseline=%d enabled=%d expected%d", paid[0], paid[1], (paid[0]*tc.pct+99)/100)
			}
		})
	}
}

func TestAssassinEffectGateBothPullPowersWithoutExclusion(t *testing.T) {
	for _, learned := range []bool{false, true} {
		for _, powers := range [][]string{{"CK-A28", "CK-A30"}, {"CK-A30", "CK-A28"}} {
			skills := domain.Learned{10225: 1}
			if learned {
				skills[10226] = 1
			}
			s, p, m := nativeRepairFixture(t, domain.Assassin, skills, powers...)
			m.Pos.X = 300
			s.aoi.Move(m, m.Pos)
			s.outbox = nil
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10225, Target: m.ID})
			want := float64(180)
			if learned {
				want = 140
			}
			if m.Pos.X != want {
				t.Fatalf("order%v learned%v endX%v want%v", powers, learned, m.Pos.X, want)
			}
			// A28 must execute even when A30 is equipped; reaching its intermediate
			// destination proves the previous mutually exclusive early return is gone.
			sawLimitedPull := false
			for _, out := range s.outbox {
				if move, ok := out.ev.(event.EntityMoved); ok && move.ID == m.ID && move.To.X == 180 {
					sawLimitedPull = true
				}
			}
			if !sawLimitedPull {
				t.Fatalf("A28 was suppressed by A30 order%v learned%v", powers, learned)
			}
		}
	}
}

package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Cross the actual player-cast, monster-AI and normal combat queues together.
// The native skills, shield and burning source are never injected as statuses;
// controlled actor statistics only make the final damage comparison exact.
func TestLegendaryRepairAreaNativeShieldsAndMonsterAI(t *testing.T) {
	for _, setPieces := range []int{0, 2, 4} {
		t.Run(fmt.Sprintf("set=%d", setPieces), func(t *testing.T) {
			losses := make([]int32, 3)
			for count := 1; count <= 3; count++ {
				powers := []string{"SS-A06", "SS-D01", "SS-D09"}
				s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10402: 1, 10421: 1, 10425: 1}, powers[:count]...)
				if setPieces > 0 {
					nativeRepairSet(t, s, p, 3, setPieces)
				}
				nativeRepairCast(t, s, p, 10425, p.ID)
				nativeRepairCast(t, s, p, 10421, m.ID)
				if !legendaryCasterHasSkill(p, 10425) || !m.Status.HasEffective(1036) {
					t.Fatal("native shield/burning prerequisite absent")
				}
				side := putMonster(s, 901, 100000, domain.Stats{}, 200, 100)
				beforeMain := m.HP
				nativeRepairCast(t, s, p, 10402, m.ID)
				if m.HP >= beforeMain || side.HP == 100000 {
					t.Fatal("final fireball shape lost primary or nearby victim")
				}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, Hit: 1 << 20}
				m.Monster.Target = p.ID
				p.HP = p.MaxHP
				s.outbox = nil
				before := p.HP
				s.stepAI()
				if len(s.attacks) != 1 || s.attacks[0].src != m.ID {
					t.Fatalf("monster AI failed to admit its normal retaliation: %+v", s.attacks)
				}
				s.stepCombat()
				losses[count-1] = before - p.HP
				hits, reflections := 0, 0
				for _, out := range s.outbox {
					if e, ok := out.ev.(event.DamageDealt); ok {
						if e.Src == m.ID && e.Dst == p.ID && e.Origin == domain.DamageNormalOrigin {
							hits++
						}
						if e.Origin == domain.DamageReflectionOrigin {
							reflections++
						}
					}
				}
				if hits != 1 || reflections > 1 {
					t.Fatalf("normal/reflect chain repeated: hits=%d reflections=%d", hits, reflections)
				}
			}
			if losses[0] <= 0 || losses[1] != losses[0]*72/100 || losses[2] != losses[1]*60/100 {
				t.Fatalf("native shield plus one/two/three powers did not compose once: %v", losses)
			}
		})
	}
}

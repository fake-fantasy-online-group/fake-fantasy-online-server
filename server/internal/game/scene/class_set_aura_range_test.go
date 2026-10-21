package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestClassSetAuraApprovedRadiusBoundary(t *testing.T) {
	for suit := int32(1); suit <= 5; suit++ {
		for _, distance := range []float64{299, 300, 301} {
			t.Run(fmt.Sprintf("suit%d/distance%.0f", suit, distance), func(t *testing.T) {
				s := newCombatScene(t)
				source := partySetMember(t, s, 1, suit)
				join(t, s, 2, 200, "队友", source.Pos.X+distance, source.Pos.Y)
				target := s.players[2]
				target.Player.Party = source.Player.Party
				s.stepEquipmentAuras()
				a, ok := classSetAura(suit)
				if !ok || a.RadiusPX != 300 || a.RefreshMS != 500 || a.LeaseMS != 1000 {
					t.Fatalf("unexpected contract %+v", a)
				}
				got := s.classSetAuraValue(target, a.Modifier)
				want := int32(0)
				if distance <= 300 {
					want = a.Value
				}
				if got != want {
					t.Fatalf("distance %.0f benefit=%d want=%d", distance, got, want)
				}
				// Leaving the inclusive boundary invalidates the source without waiting
				// for a spatial refresh or visual lease timeout.
				if distance <= 300 {
					s.aoi.Move(target, domain.Pos{MapID: source.Pos.MapID, X: source.Pos.X + 301, Y: source.Pos.Y})
					if got := s.classSetAuraValue(target, a.Modifier); got != 0 {
						t.Fatalf("left range retained %d", got)
					}
				}
			})
		}
		text := classSetSkillText(suit, 6)
		if !strings.Contains(text, "自身及300范围内队友") || strings.Contains(text, "240范围") {
			t.Fatalf("stale suit%d tooltip: %s", suit, text)
		}
	}
}

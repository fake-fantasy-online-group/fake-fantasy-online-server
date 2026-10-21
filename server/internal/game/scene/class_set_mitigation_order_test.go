package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func mitigationOrderFixture(t *testing.T, suit int32, expanded bool, shield int32, immune bool) (*Scene, *entity.Entity) {
	t.Helper()
	s, p := meleeSetFixture(t, suit, 6)
	p.Stats.MDef = 0
	if suit == 4 {
		taunt := domain.SkillDef{ID: classSetTaunt}
		for i := 0; i < 10; i++ {
			s.classSetMeleeCastReleased(p, &taunt)
		}
	}
	status := domain.StatusDef{ID: 99001, Level: 1, DurationSec: 30, TypeKnown: true, Type: domain.StatusGood, MagicDamageReductionFlat: 20, DamageShield: shield, Invulnerable: immune}
	if expanded {
		status.DamageReductionCapPct = 95
	}
	p.Status.Apply(status, s.tick, p.ID)
	return s, p
}

func TestClassSetSecondaryMatchesDirectFlatThenSetThenShield(t *testing.T) {
	for _, tc := range []struct {
		name         string
		suit         int32
		shield, want int32
	}{
		{"dragon", 5, 0, 120}, {"dragon shield", 5, 10, 110},
		{"unruly", 4, 0, 40}, {"unruly shield", 4, 10, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p := mitigationOrderFixture(t, tc.suit, true, tc.shield, false)
			foe := putMonster(s, 900, 10000, domain.Stats{MAtk: 100}, 110, 100)
			direct, _, _ := s.resolveStrikeStatuses(foe, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
			s, p = mitigationOrderFixture(t, tc.suit, true, tc.shield, false)
			secondary := s.mitigateSecondaryStatusDamage(p, true, 100)
			if direct.Amount != tc.want || secondary != tc.want {
				t.Fatalf("direct=%d secondary=%d want=%d", direct.Amount, secondary, tc.want)
			}
		})
	}
}

func TestClassSetSecondaryPreservesLegacyGateAndImmunity(t *testing.T) {
	for _, tc := range []struct{ suit, want int32 }{{5, 150}, {4, 50}} {
		s, p := mitigationOrderFixture(t, tc.suit, false, 10, false)
		if got := s.mitigateSecondaryStatusDamage(p, true, 100); got != tc.want {
			t.Fatalf("legacy gate suit%d got%d want%d", tc.suit, got, tc.want)
		}
		s, p = mitigationOrderFixture(t, tc.suit, true, 10, true)
		if got := s.mitigateSecondaryStatusDamage(p, true, 100); got != 0 {
			t.Fatalf("immune took%d", got)
		}
		p.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == 99001 && a.ShieldLeft != 10 {
				t.Fatalf("immune consumed shield%d", a.ShieldLeft)
			}
		})
	}
}

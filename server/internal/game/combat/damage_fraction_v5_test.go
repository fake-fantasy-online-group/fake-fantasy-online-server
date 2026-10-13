package combat

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
	"testing"
)

func TestCalculateStrikeV5DoesNotTemporarilyKill(t *testing.T) {
	a := &entity.Entity{ID: 1, HP: 100, Stats: domain.Stats{MAtk: 1000}}
	b := &entity.Entity{ID: 2, HP: 3, MaxHP: 100}
	ev := CalculateStrike(a, b, &HitTracker{}, Blow{Magic: true}, nil)
	if b.HP != 3 || ev.DstHP != 0 || ev.Amount != 1000 {
		t.Fatal(ev, b.HP)
	}
}
func TestDamageFractionV5NativeDefenseAndOverflow(t *testing.T) {
	if x := RollPhysicalSkillFraction(1000, 1000, 10, 2482, 19, nil); x != 1296 {
		t.Fatal(x)
	}
	if x := RollMagicFixedDefenseFraction(1000, 2482, 19, 100, 10); x != 1233 {
		t.Fatal(x)
	}
	if x := RollMagicFixedDefenseFraction(math.MaxInt32, math.MaxInt64, 1, 0, 0); x != math.MaxInt32 {
		t.Fatal(x)
	}
	if x := RollPhysicalSkillFraction(1, 1, 0, math.MaxInt64, math.MaxInt64, nil); x != 1 {
		t.Fatal(x)
	}
	for d := int32(0); d < 100; d++ {
		if a, b := RollMagicFixedDefenseFraction(315, 208, 1, d, 3), RollMagicFixedDefense(315, 208, d, 3); a != b {
			t.Fatal(a, b)
		}
	}
}

func TestExplicitZeroFractionV5IsNotLegacyDefault(t *testing.T) {
	if got := RollMagicFixedDefenseFraction(100000, 0, 1, 10000, 10000); got != 0 {
		t.Fatal(got)
	}
	if got := RollPhysicalSkillFraction(100000, 100000, 1000, 0, 1, nil); got != 0 {
		t.Fatal(got)
	}
	if got := RollMagicFixedDefenseFraction(100, 0, 0, 0, 0); got != 100 {
		t.Fatal("legacy absent fraction changed", got)
	}
}

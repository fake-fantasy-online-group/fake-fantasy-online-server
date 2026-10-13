package combat

import (
	"math"
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type upperEndpointRand struct{}

func (upperEndpointRand) Intn(n int) int { return n - 1 }

func TestRiftHighAttackDamageBounds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		got, want int32
	}{
		{"physical skill saturation", RollPhysicalSkill(math.MaxInt32, math.MaxInt32, 0, 180, nil), math.MaxInt32},
		{"physical resistance after multiplier", RollPhysicalSkill(math.MaxInt32, math.MaxInt32, math.MaxInt32, 200, nil), math.MaxInt32},
		{"physical negative resistance", RollPhysical(math.MaxInt32, math.MaxInt32, math.MinInt32, nil), math.MaxInt32},
		{"physical minimum", RollPhysical(math.MinInt32, math.MinInt32, math.MaxInt32, nil), 1},
		{"physical wide range", RollPhysical(math.MinInt32, math.MaxInt32, 0, upperEndpointRand{}), math.MaxInt32},
		{"magic skill saturation", RollMagicFixedDefense(math.MaxInt32, 180, 0, 0), math.MaxInt32},
		{"magic resistance after multiplier", RollMagicFixedDefense(math.MaxInt32, 200, 0, math.MaxInt32), math.MaxInt32},
		{"magic negative resistance", RollMagicFixedDefense(math.MaxInt32, 100, 0, math.MinInt32), math.MaxInt32},
		{"magic minimum", RollMagicFixedDefense(math.MinInt32, math.MaxInt32, math.MaxInt32, math.MaxInt32), 1},
		{"legacy magic saturation", RollMagic(math.MaxInt32, 180, 60, 0, 0), math.MaxInt32},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got=%d want=%d", tc.name, tc.got, tc.want)
		}
	}
}

func TestRiftHighCriticalDamageNeverHealsTarget(t *testing.T) {
	for _, magic := range []bool{false, true} {
		src := &entity.Entity{ID: 1, HP: 1, Stats: domain.Stats{
			MinAtk: math.MaxInt32, MaxAtk: math.MaxInt32, MAtk: math.MaxInt32,
			CritRate: 10000, MCritRate: 10000,
		}}
		dst := &entity.Entity{ID: 2, HP: math.MaxInt32, MaxHP: math.MaxInt32}
		var tracker HitTracker
		ev := Strike(src, dst, &tracker, Blow{Magic: magic, SkillPct: 180}, rand.New(rand.NewSource(1)))
		if ev.Amount != math.MaxInt32 || dst.HP != 0 || ev.DstHP != 0 || !ev.Flag.Has(event.DamageCrit) || !ev.Flag.Has(event.DamageFatal) {
			t.Fatalf("magic=%v: %+v target HP=%d", magic, ev, dst.HP)
		}
	}
}

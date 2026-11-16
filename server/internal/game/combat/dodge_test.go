package combat

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type constRand struct {
	v     int
	calls int
}

func (r *constRand) Intn(n int) int {
	r.calls++
	return min(r.v, n-1)
}

func TestLegendaryDodgeAfterCritAndGuaranteedHit(t *testing.T) {
	a := mkAttacker(1, domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000})
	for _, b := range []Blow{
		{GuaranteedHit: true, DodgeChanceBP: 5000},
		{GuaranteedHit: true, GuaranteedCrit: true, DodgeChanceBP: 5000},
		{Magic: true, SkillPct: 100, DodgeChanceBP: 5000},
	} {
		d := mkTarget(2, 1000, domain.Stats{})
		ev := CalculateStrike(a, d, &HitTracker{}, b, &constRand{v: 0})
		if !ev.Flag.Has(event.DamageMiss) || !ev.Flag.Has(event.DamageDodged) || ev.Flag.Has(event.DamageCrit) || ev.Amount != 0 || d.HP != 1000 {
			t.Fatalf("dodge did not void the hit: %+v", ev)
		}
		if b.Magic && !ev.Flag.Has(event.DamageMagic) {
			t.Fatal("magic dodge lost its magic flag")
		}
		d = mkTarget(2, 1000, domain.Stats{})
		ev = CalculateStrike(a, d, &HitTracker{}, b, &constRand{v: 9999})
		if ev.Flag.Has(event.DamageMiss) || ev.Flag.Has(event.DamageDodged) || ev.Amount <= 0 {
			t.Fatalf("failed dodge roll still missed: %+v", ev)
		}
	}
}

func TestLegendaryDodgeSkipsPeriodicAndZeroChance(t *testing.T) {
	a := mkAttacker(1, domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000})
	d := mkTarget(2, 1000, domain.Stats{})
	ev := CalculateStrike(a, d, &HitTracker{}, Blow{GuaranteedHit: true, Origin: domain.DamageDOTOrigin, DodgeChanceBP: 10000}, &constRand{v: 0})
	if ev.Flag.Has(event.DamageDodged) || ev.Flag.Has(event.DamageMiss) {
		t.Fatal("periodic damage was dodged")
	}
	zero, some := &constRand{v: 9999}, &constRand{v: 9999}
	CalculateStrike(a, mkTarget(2, 1000, domain.Stats{}), &HitTracker{}, Blow{GuaranteedHit: true}, zero)
	CalculateStrike(a, mkTarget(2, 1000, domain.Stats{}), &HitTracker{}, Blow{GuaranteedHit: true, DodgeChanceBP: 1}, some)
	if some.calls != zero.calls+1 {
		t.Fatalf("zero dodge chance must not consume a roll: %d vs %d", zero.calls, some.calls)
	}
}

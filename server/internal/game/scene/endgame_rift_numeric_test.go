package scene

import (
	"math"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestRiftMonsterHighHPPercentage(t *testing.T) {
	for _, tc := range []struct{ hp, max, want int32 }{
		{math.MaxInt32, math.MaxInt32, 100}, {math.MaxInt32 / 2, math.MaxInt32, 49},
		{0, math.MaxInt32, 0}, {-1, math.MaxInt32, 0}, {math.MaxInt32, 1, 100}, {1, 0, 0},
	} {
		if got := monsterHPPct(&entity.Entity{HP: tc.hp, MaxHP: tc.max}); got != tc.want {
			t.Fatalf("%+v: got=%d", tc, got)
		}
	}
}

func TestRiftHighDamageAndFlatBonusThroughStatuses(t *testing.T) {
	s := newCombatScene(t)
	src := putMonster(s, 900, math.MaxInt32, domain.Stats{MinAtk: math.MaxInt32, MaxAtk: math.MaxInt32}, 100, 100)
	dst := putMonster(s, 901, math.MaxInt32, domain.Stats{}, 110, 100)
	dst.Status = domain.NewStatusSet()
	dst.Status.Apply(domain.StatusDef{ID: 9001, DurationSec: 60, PhysicalDamageReductionPct: 25}, 0, dst.ID)
	var tracker combat.HitTracker
	ev, raw, _ := s.resolveStrikeStatuses(src, dst, &tracker, combat.Blow{GuaranteedHit: true, SkillPct: 180, FlatDamage: 100})
	want := int32(int64(math.MaxInt32) - int64(math.MaxInt32)*25/100)
	if raw != math.MaxInt32 || ev.Amount != want || dst.HP != math.MaxInt32-want || ev.Flag.Has(event.DamageFatal) {
		t.Fatalf("raw=%d resolved=%+v hp=%d", raw, ev, dst.HP)
	}
}

func TestRiftHighHPPeriodicStatus(t *testing.T) {
	s := newCombatScene(t)
	m := putMonster(s, 900, math.MaxInt32, domain.Stats{}, 100, 100)
	s.tickStatus(m, domain.Active{Source: m.ID, Def: domain.StatusDef{
		ID: 9001, HPChanges: []domain.ResourceChange{{Value: -6, Mode: domain.ModePercent}},
	}})
	wantLoss := int32(int64(math.MaxInt32) * 6 / 100)
	if m.HP != math.MaxInt32-wantLoss {
		t.Fatalf("periodic percentage overflow: HP=%d want=%d", m.HP, math.MaxInt32-wantLoss)
	}
	m.HP = math.MaxInt32 - 1
	s.tickStatus(m, domain.Active{Source: m.ID, Def: domain.StatusDef{
		ID: 9002, HPChanges: []domain.ResourceChange{{Value: 100, Mode: domain.ModePercent}, {Value: 100, Mode: domain.ModePercent}},
	}})
	if m.HP != math.MaxInt32 {
		t.Fatalf("stacked healing delta overflow: HP=%d", m.HP)
	}
}

func TestRiftHighDamageStealAndReflection(t *testing.T) {
	s := newCombatScene(t)
	src := putMonster(s, 900, math.MaxInt32, domain.Stats{}, 100, 100)
	dst := putMonster(s, 901, math.MaxInt32, domain.Stats{}, 110, 100)
	src.HP, src.MP, src.MaxMP = 1000000000, 1000000000, math.MaxInt32
	src.Status = domain.NewStatusSet()
	src.Status.Apply(domain.StatusDef{ID: 9001, DurationSec: 60, LifeStealPct: 90, LifeStealChance: 100, ManaStealPct: 90, ManaStealChance: 100}, 0, src.ID)
	s.afterStrikeStatuses(src, dst, event.DamageDealt{Src: src.ID, Dst: dst.ID, Amount: 1000000000}, 1000000000, 0)
	if src.HP != 1900000000 || src.MP != 1900000000 {
		t.Fatalf("steal overflow: hp=%d mp=%d", src.HP, src.MP)
	}
	src.Status = nil
	dst.Status = domain.NewStatusSet()
	dst.Status.Apply(domain.StatusDef{ID: 9002, DurationSec: 60, PhysicalReflectPct: 90}, 0, dst.ID)
	s.afterStrikeStatuses(src, dst, event.DamageDealt{Src: src.ID, Dst: dst.ID, Amount: 1000000000}, 1000000000, 0)
	if src.HP != 1000000000 {
		t.Fatalf("reflection overflow: hp=%d", src.HP)
	}
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math/rand"
	"testing"
)

func TestEffectContestSingleDrawAcrossSkillAndRandomStatus(t *testing.T) {
	for _, randomExtra := range []bool{false, true} {
		s := newTestScene(t, nil)
		src := &entity.Entity{ID: 800, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}}
		dst := &entity.Entity{ID: 801, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}}
		s.entities[src.ID], s.entities[dst.ID] = src, dst
		src.Stats.EffectHitBP = 10000
		dst.Stats.StatusResist = 100
		s.statuses = statusTable(domain.StatusDef{ID: domain.StatusWeaken, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusBad})
		def := domain.SkillDef{ID: 10000, Statuses: []domain.StatusApplication{{ID: domain.StatusWeaken, Level: 1, ChanceBP: 3000, ChanceExplicit: true}}}
		expected := rand.New(rand.NewSource(611))
		s.rng = rand.New(rand.NewSource(611))
		for i := 0; i < 100; i++ {
			draw := expected.Float64()
			if randomExtra {
				s.applyMonsterSkillExtras(src, def, []domain.EntityID{dst.ID}, []domain.MonsterSkillExtra{{Kind: domain.MonsterExtraRandomStatus, Statuses: def.Statuses}})
			} else {
				s.applySkill(src, dst, def, 0, false)
			}
			got := dst.Status != nil && dst.Status.Has(domain.StatusWeaken)
			if got != (draw < .3) {
				t.Fatalf("random=%v iteration=%d draw=%v applied=%v; base chance sampled twice?", randomExtra, i, draw, got)
			}
			if dst.Status != nil {
				dst.Status.Remove(domain.StatusWeaken)
			}
			dst.Stats.StatusResist = 100
		}
		if s.rng.Float64() != expected.Float64() {
			t.Fatal("extra random draws")
		}
	}
}

func TestEffectContestExplicitZeroBuffsAndImmunity(t *testing.T) {
	s := newTestScene(t, nil)
	src := &entity.Entity{ID: 800, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}}
	dst := &entity.Entity{ID: 801, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}, Status: domain.NewStatusSet()}
	s.entities[src.ID], s.entities[dst.ID] = src, dst
	src.Stats.EffectHitBP = 100000
	dst.Stats.StatusResist = 100000
	bad := domain.StatusDef{ID: domain.StatusBlind, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusBad, ChanceExplicit: true}
	if s.applyStatusDef(dst, bad, src.ID) {
		t.Fatal("explicit 0 applied")
	}
	good := bad
	good.Type = domain.StatusGood
	good.ChanceExplicit = false
	if !s.applyStatusDef(dst, good, src.ID) {
		t.Fatal("buff incorrectly resisted")
	}
	dst.Status.Remove(good.ID)
	dst.Status.Apply(domain.StatusDef{ID: 99, DurationSec: 20, BlockHarmful: true}, 0, 0)
	bad.ChanceBP = 10000
	bad.EffectHitBP = 100000000
	expected := rand.New(rand.NewSource(19))
	s.rng = rand.New(rand.NewSource(19))
	if s.applyStatusDef(dst, bad, src.ID) {
		t.Fatal("accuracy pierced immunity")
	}
	if s.rng.Float64() != expected.Float64() {
		t.Fatal("immunity consumed probability draw")
	}
}

func TestEffectContestUsesAcceptedSnapshotAndPersistentLesser(t *testing.T) {
	s := newTestScene(t, nil)
	src := &entity.Entity{ID: 800, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}}
	s.entities[src.ID] = src
	src.Stats.EffectHitBP = 5000
	cast := s.captureCombatSourceV5(src, false)
	finish := s.beginEquipmentRoot(src.ID, 0, cast.Equipment)
	defer finish()
	s.equipment.root.source = cast
	src.Stats.EffectHitBP = 9000
	if got := s.sourceEffectHitBP(src); got != 5000 {
		t.Fatal("late equipment increased cast", got)
	}
	s.equipment.root.currentSource = s.captureCombatSourceV5(src, false)
	if got := s.sourceEffectHitBP(src); got != 5000 {
		t.Fatal("persistent source exceeded cast", got)
	}
	src.Stats.EffectHitBP = 2000
	s.equipment.root.currentSource = s.captureCombatSourceV5(src, false)
	if got := s.sourceEffectHitBP(src); got != 2000 {
		t.Fatal("persistent source retained removed gear", got)
	}
}

func TestIceShieldEffectContestSourceIsHolder(t *testing.T) {
	s := newTestScene(t, nil)
	attacker := &entity.Entity{ID: 800, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}}
	holder := &entity.Entity{ID: 801, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}, Status: domain.NewStatusSet()}
	s.entities[attacker.ID], s.entities[holder.ID] = attacker, holder
	attacker.Stats.EffectHitBP = 100000
	attacker.Stats.StatusResist = 100
	s.statuses = statusTable(domain.StatusDef{ID: domain.StatusFreeze, Level: 1, DurationSec: 5, TypeKnown: true, Type: domain.StatusBad})
	holder.Status.Apply(domain.StatusDef{ID: domain.StatusIceShield, DurationSec: 20, FreezeAttackerChance: 100, FreezeAttackerEffectHitBP: 10000, SourceEffectHitKnown: true}, 0, holder.ID)
	if !s.applyCombatTriggeredStatus(holder, attacker, domain.StatusFreeze) {
		t.Fatal("holder local accuracy should cancel resistance")
	}
	attacker.Status.Remove(domain.StatusFreeze)
	attacker.Stats.StatusResist = 100
	holder.Status.Remove(domain.StatusIceShield)
	holder.Status.Apply(domain.StatusDef{ID: domain.StatusIceShield, DurationSec: 20, FreezeAttackerChance: 100, SourceEffectHitKnown: true}, 0, holder.ID)
	expected := rand.New(rand.NewSource(611))
	s.rng = rand.New(rand.NewSource(611))
	for i := 0; i < 30; i++ {
		want := expected.Float64() < .5
		if got := s.applyCombatTriggeredStatus(holder, attacker, domain.StatusFreeze); got != want {
			t.Fatal("attacker accuracy leaked to ice shield", i)
		}
		attacker.Status.Remove(domain.StatusFreeze)
		attacker.Stats.StatusResist = 100
	}
}

func TestEffectContestAssassinationOnlyDirectDeathException(t *testing.T) {
	s := newTestScene(t, nil)
	src := putMonster(s, 800, 1000, domain.Stats{EffectHitBP: 10000}, 100, 100)
	dst := putMonster(s, 801, 1000, domain.Stats{StatusResist: 100}, 110, 100)
	fx := domain.SkillEffect{Kind: domain.EffectInstantDeath, ChanceBP: 1300, EffectHitBP: 10000}
	expected := rand.New(rand.NewSource(611))
	s.rng = rand.New(rand.NewSource(611))
	for i := 0; i < 100; i++ {
		if got := s.instantDeathLands(src, dst, 10210, fx); got != (expected.Float64() < .195) {
			t.Fatal("assassination ratio wrong", i)
		}
	}
	fx.ChanceBP = 10000
	dst.Stats.StatusResist = 10000000
	if !s.instantDeathLands(src, dst, 99999, fx) {
		t.Fatal("unrelated instant death was changed")
	}
}

func TestTotalMitigationUsesPostDefenseAmplifiedBaseline(t *testing.T) {
	s := newCombatScene(t)
	src := putMonster(s, 900, 100000, domain.Stats{MAtk: 1000}, 100, 100)
	dst := putMonster(s, 901, 100000, domain.Stats{MDef: 200, MagicResist: 50}, 110, 100)
	dst.Status = domain.NewStatusSet()
	dst.Status.Apply(domain.StatusDef{ID: 9001, DurationSec: 20, MagicDamageTakenPct: 200, MagicDamageReductionPct: 90, MagicDamageReductionFlat: 100000}, 0, dst.ID)
	var tracker combat.HitTracker
	ev, raw, _ := s.resolveStrikeStatuses(src, dst, &tracker, combat.Blow{Magic: true, SkillPct: 100})
	want := int32((int64(raw)*3 + 3) / 4)
	if ev.Amount != want {
		t.Fatalf("raw post-defense %d final %d want %d", raw, ev.Amount, want)
	}
	if raw >= 1000 {
		t.Fatal("ordinary native defense was bypassed", raw)
	}
	dst.Status.Apply(domain.StatusDef{ID: 9002, DurationSec: 20, DamageShield: 100000}, 0, dst.ID)
	ev, _, _ = s.resolveStrikeStatuses(src, dst, &tracker, combat.Blow{Magic: true, SkillPct: 100})
	if ev.Amount != 0 {
		t.Fatal("shield pool lost full absorption", ev.Amount)
	}
}

func TestTrapEffectAccuracyCannotBenefitFromEitherEquipmentSwap(t *testing.T) {
	for _, ratings := range [][2]int32{{10000, 0}, {0, 10000}} {
		s := newCombatScene(t)
		s.alloc = domain.NewEntityAlloc()
		join(t, s, 1, 100, "caster", 100, 100)
		source := s.players[1]
		target := putMonster(s, 801, 1000, domain.Stats{StatusResist: 100}, 110, 100)
		s.monsters[target.ID] = target
		s.statuses = statusTable(domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 5, TypeKnown: true, Type: domain.StatusBad})
		def := domain.SkillDef{ID: 77777, Statuses: []domain.StatusApplication{{ID: domain.StatusStun, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}}
		source.Stats.EffectHitBP = ratings[0]
		s.placeTrap(source, def, target.Pos, 30, 0)
		if len(s.traps) != 1 {
			t.Fatal("trap was not placed")
		}
		source.Stats.EffectHitBP = ratings[1]
		seed := int64(0)
		for rand.New(rand.NewSource(seed)).Float64() < .5 {
			seed++
		}
		s.rng = rand.New(rand.NewSource(seed))
		s.stepTraps()
		if len(s.traps) != 0 {
			t.Fatal("trap did not trigger")
		}
		if target.Status != nil && target.Status.Has(domain.StatusStun) {
			t.Fatalf("cast/current ratings %v used higher rating", ratings)
		}
	}
}

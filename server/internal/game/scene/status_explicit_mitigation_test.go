package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func explicitSceneMitigationBuff() domain.StatusDef {
	return domain.StatusDef{ID: 810001, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 5,
		PhysicalDamageReductionPct: 95, MagicDamageReductionPct: 95, DamageReductionCapPct: 95,
		BlockHarmful: true, SuppressHarmful: true}
}

func TestExplicitMitigationSceneUsesOneSharedBucketForEveryEntity(t *testing.T) {
	for _, player := range []bool{false, true} {
		for _, magic := range []bool{false, true} {
			s := newCombatScene(t)
			src := putMonster(s, 890, 100000, domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}, 100, 100)
			var dst *entity.Entity
			if player {
				join(t, s, 1, 100, "target", 110, 100)
				dst = s.players[1]
			} else {
				dst = putMonster(s, 891, 100000, domain.Stats{}, 110, 100)
			}
			if !s.applyStatusDef(dst, explicitSceneMitigationBuff(), dst.ID) {
				t.Fatal("cannot apply buff")
			}
			dst.HP, dst.MaxHP = 100000, 100000
			dst.Status.Apply(domain.StatusDef{ID: 810002, DurationSec: 30, PhysicalDamageReductionPct: 50, MagicDamageReductionPct: 50}, s.tick, dst.ID)
			ev, raw, _ := s.resolveStrikeStatuses(src, dst, &combat.HitTracker{}, combat.Blow{Magic: magic, GuaranteedHit: true})
			want := int32((int64(raw)*5 + 99) / 100)
			if ev.Amount != want || raw <= 0 {
				t.Fatalf("player=%v magic=%v raw=%d got=%d want=%d", player, magic, raw, ev.Amount, want)
			}
			dst.Status.Remove(explicitSceneMitigationBuff().ID)
			dst.Status.Apply(domain.StatusDef{ID: 810003, DurationSec: 30, PhysicalDamageReductionPct: 95, MagicDamageReductionPct: 95}, s.tick, dst.ID)
			ev, raw, _ = s.resolveStrikeStatuses(src, dst, &combat.HitTracker{}, combat.Blow{Magic: magic, GuaranteedHit: true})
			if ev.Amount != int32((int64(raw)+3)/4) {
				t.Fatal("ordinary cap changed", ev.Amount, raw)
			}
		}
	}
}

func TestExplicitSuppressionSceneStatsAdmissionAndExclusiveExpiry(t *testing.T) {
	s, mid, _ := statusScene(t)
	p := beefUp(s)
	baseAtk := p.Stats.MinAtk
	bad := domain.StatusDef{ID: 810010, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10,
		Control: domain.ControlStun, BlockMove: true, CannotAttack: true, AttackCapKnown: true, AttackCap: 1,
		PhysicalDamageTakenPct: 100, Affixes: []domain.Affix{{Attr: domain.AttrAtk, Mode: domain.ModePercent, Value: -50}}}
	if !s.applyStatusDef(p, bad, mid) || p.Stats.MinAtk != 1 || canMove(p) || canCast(p) {
		t.Fatal("initial debuff absent")
	}
	start := s.tick
	if !s.applyStatusDef(p, explicitSceneMitigationBuff(), p.ID) {
		t.Fatal("buff rejected")
	}
	if p.Stats.MinAtk != baseAtk || !canMove(p) || !canCast(p) || !canDealDamage(p) {
		t.Fatal("cached stats/control not refreshed on application")
	}
	for _, d := range []domain.StatusDef{
		{Control: domain.ControlStun}, {Control: domain.ControlSilence}, {BlockMove: true},
		{PhysicalDamageTakenPct: 100}, {PreventInvisible: true}, {CannotAttack: true},
		{HPChanges: []domain.ResourceChange{{Value: -20}}},
	} {
		d.ID, d.Level, d.DurationSec, d.TypeKnown, d.Type = 810020, 1, 10, true, domain.StatusBad
		if s.applyStatusDef(p, d, mid) {
			t.Fatal("harmful state admitted", d)
		}
	}
	positive := domain.StatusDef{ID: 810021, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 10,
		Affixes: []domain.Affix{{Attr: domain.AttrAtk, Mode: domain.ModePercent, Value: 20}}}
	if !s.applyStatusDef(p, positive, p.ID) || p.Stats.MinAtk <= baseAtk {
		t.Fatal("positive buff stopped working")
	}
	for _, view := range s.visibleStatuses(p) {
		if view.icon.Status == int32(bad.ID) && view.buff.Immobile != 0 {
			t.Fatal("client immobile flag leaked from suppressed control")
		}
	}
	s.tick = start + domain.Ticks(5000) - 1
	s.stepStatus()
	if !canMove(p) || p.Stats.MinAtk <= 1 {
		t.Fatal("immunity ended early")
	}
	s.tick++
	s.stepStatus()
	if canMove(p) || canCast(p) || p.Stats.MinAtk != 1 || !p.Status.Has(bad.ID) {
		t.Fatal("remaining harmful effects/cached stats failed to resume exactly at expiry")
	}
}

func TestExplicitSuppressionSceneDOTClockAndQueuedReceipt(t *testing.T) {
	for _, late := range []bool{false, true} {
		s, mid, _ := statusScene(t)
		p := beefUp(s)
		bad := domain.StatusDef{ID: 810030, Level: 1, TypeKnown: true, Type: domain.StatusBad,
			DurationSec: 20, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -20}}, MPChanges: []domain.ResourceChange{{Value: -2}}}
		if !s.applyStatusDef(p, bad, mid) || !s.applyStatusDef(p, explicitSceneMitigationBuff(), p.ID) {
			t.Fatal("application failed")
		}
		start, hp, mp := s.tick, p.HP, p.MP
		// A receipt captured before immunity must also be rejected on settlement.
		s.tickStatus(p, domain.Active{Def: bad, Source: mid})
		if p.HP != hp || p.MP != mp {
			t.Fatal("queued harmful receipt bypassed immunity")
		}
		if late {
			s.tick = start + domain.Ticks(6000)
			s.stepStatus()
			if p.HP != hp || p.MP != mp {
				t.Fatal("long-frame suppression backlog replayed")
			}
			s.tick += domain.Ticks(1000)
		} else {
			for ms := 1000; ms < 5000; ms += 1000 {
				s.tick = start + domain.Ticks(ms)
				s.stepStatus()
				if p.HP != hp || p.MP != mp {
					t.Fatal("harmful resource tick during immunity")
				}
			}
			s.tick = start + domain.Ticks(5000)
		}
		s.stepStatus()
		if p.HP != hp-20 || p.MP != mp-2 {
			t.Fatalf("late=%v expected one resumed tick, HP %d->%d MP %d->%d", late, hp, p.HP, mp, p.MP)
		}
	}
}

func TestExplicitMitigationSecondaryPathsAndLegacyReflection(t *testing.T) {
	s := newCombatScene(t)
	reflector := putMonster(s, 890, 100000, domain.Stats{}, 100, 100)
	target := putMonster(s, 891, 100000, domain.Stats{}, 110, 100)
	target.Status = domain.NewStatusSet()
	ordinary := explicitSceneMitigationBuff()
	ordinary.DamageReductionCapPct, ordinary.SuppressHarmful, ordinary.BlockHarmful = 0, false, false
	target.Status.Apply(ordinary, s.tick, target.ID)
	before := target.HP
	s.emitReflectedDamage(reflector, target, 1000, true)
	if target.HP != before-1000 {
		t.Fatal("ordinary legacy reflection changed")
	}
	if !s.applyStatusDef(target, explicitSceneMitigationBuff(), target.ID) {
		t.Fatal("buff rejected")
	}
	before = target.HP
	s.emitReflectedDamage(reflector, target, 1000, false)
	if target.HP != before-50 {
		t.Fatal("physical reflection bypassed explicit shared cap", before-target.HP)
	}
	before = target.HP
	s.emitReflectedDamage(reflector, target, 1000, true)
	if target.HP != before-50 {
		t.Fatal("magic reflection bypassed explicit shared cap", before-target.HP)
	}
	// A custom explicit mitigation source can omit harmful suppression and
	// still participate in the same secondary damage formula.
	mitigationOnly := explicitSceneMitigationBuff()
	mitigationOnly.SuppressHarmful, mitigationOnly.BlockHarmful = false, false
	target.Status.Apply(mitigationOnly, s.tick, target.ID)
	before = target.HP
	s.tickStatus(target, domain.Active{Def: domain.StatusDef{ID: 810041, TypeKnown: true, Type: domain.StatusBad,
		HPChanges: []domain.ResourceChange{{Value: -1000}}}, Source: reflector.ID})
	if target.HP != before-50 {
		t.Fatal("periodic damage bypassed explicit shared cap", before-target.HP)
	}
}

func TestExplicitSuppressionDoesNotPausePositiveTicks(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	p.HP -= 100
	before := p.HP
	if !s.applyStatusDef(p, explicitSceneMitigationBuff(), p.ID) {
		t.Fatal("buff rejected")
	}
	s.tickStatus(p, domain.Active{Def: domain.StatusDef{ID: 810050, TypeKnown: true, Type: domain.StatusGood,
		HPChanges: []domain.ResourceChange{{Value: 20}}}, Source: p.ID})
	if p.HP != before+20 {
		t.Fatal("positive periodic healing suppressed")
	}
}

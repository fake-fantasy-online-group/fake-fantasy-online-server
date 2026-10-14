package domain

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func explicitMitigationBuff() StatusDef {
	return StatusDef{ID: 800001, Level: 1, TypeKnown: true, Type: StatusGood, DurationSec: 5,
		PhysicalDamageReductionPct: 95, MagicDamageReductionPct: 95, DamageReductionCapPct: 95,
		BlockHarmful: true, SuppressHarmful: true}
}

func TestExplicitMitigationSharedCapAndRounding(t *testing.T) {
	for _, capPct := range []int32{0, -100, 75, 95, 100, 200} {
		for _, amount := range []int32{0, 1, 2, 3, 4, 19, 20, 21, 1000, math.MaxInt32} {
			b := CombatDamageBuckets{DamageReductionPct: 100, ResistanceBP: 7500, MitigationBP: 4500,
				HalfDamageLayers: 2, FlatReduction: 9999999999, DamageReductionCapPct: capPct}
			keep := int64(100 - min(int32(100), max(int32(75), capPct)))
			want := int32((int64(amount)*keep + 99) / 100)
			if got := b.IncomingDamage(amount); got != want {
				t.Fatalf("cap=%d amount=%d got=%d want=%d", capPct, amount, got, want)
			}
		}
	}
	s := NewStatusSet()
	s.Apply(explicitMitigationBuff(), 0, 0)
	s.Apply(StatusDef{ID: 800002, PhysicalDamageReductionPct: 50, MagicDamageReductionPct: 50, DamageReductionCapPct: 80}, 0, 0)
	for _, magic := range []bool{false, true} {
		if got, _ := s.MitigateDamage(magic, 1000); got != 50 {
			t.Fatalf("magic=%v same aggregate cap: got %d, want 50", magic, got)
		}
	}
	s.Apply(StatusDef{ID: 800003, DamageShield: 40}, 0, 0)
	if got, _ := s.MitigateDamage(false, 1000); got != 10 {
		t.Fatalf("shield must follow shared cap: %d", got)
	}
	s.Remove(explicitMitigationBuff().ID)
	s.Remove(800002)
	s.Apply(StatusDef{ID: 800004, PhysicalDamageReductionPct: 95}, 0, 0)
	if got, _ := s.MitigateDamage(false, 1000); got != 250 {
		t.Fatalf("ordinary reduction must retain 75%% cap: %d", got)
	}
	b := CombatDamageBuckets{ResistanceBP: -5000, DamageReductionPct: 95, DamageReductionCapPct: 95}
	if got := b.IncomingDamage(1000); got != 75 {
		t.Fatalf("negative resistance amplification must precede cap: %d", got)
	}
}

func TestExplicitSuppressionEffectiveQueriesPreserveRawInstances(t *testing.T) {
	s := NewStatusSet()
	bad := StatusDef{ID: 800010, TypeKnown: true, Type: StatusBad, DurationSec: 20,
		Control: ControlStun, BlockMove: true, CannotAttack: true, PreventInvisible: true,
		BlockBeneficial: true, AttackCapKnown: true, AttackCap: 0, PhysicalDamageTakenPct: 200,
		MagicDamageTakenPct: 150, DamageReductionCapPct: 100, DamageShield: 100,
		Affixes: []Affix{{Attr: AttrAtk, Value: -50, Mode: ModePercent}}, BreakOnDamage: true,
		NextBasicAttackPct: 30, NextSkillDamagePct: 50, PoisonOnHitChance: 100, PoisonOnHitLevel: 1}
	good := StatusDef{ID: 800011, TypeKnown: true, Type: StatusGood, DurationSec: 30,
		Affixes: []Affix{{Attr: AttrAtk, Value: 20, Mode: ModePercent}}, PhysicalReflectPct: 20}
	s.Apply(bad, 0, 0)
	s.Apply(good, 0, 0)
	if !s.BlocksStatus(good) || s.Controlled() != ControlStun {
		t.Fatal("preexisting harmful behavior missing")
	}
	if s.BlocksStatus(explicitMitigationBuff()) {
		t.Fatal("a harmful beneficial-status gate cannot prevent its own explicit suppression")
	}
	s.Apply(explicitMitigationBuff(), 0, 0)
	if s.Controlled() != ControlNone || s.BlocksMovement() || s.CannotDealDamage() || s.PreventsInvisible() {
		t.Fatal("suppressed harmful admission effect still active")
	}
	if _, known := s.MaximumAttack(); known || s.DamageTakenIncreasePct(false) != 0 || s.DamageTakenIncreasePct(true) != 0 {
		t.Fatal("suppressed attack cap or vulnerability still active")
	}
	if s.DamageReductionCapPct() != 95 || s.BlocksStatus(good) || !s.BlocksStatus(bad) || s.ReflectPct(false) != 20 {
		t.Fatal("positive effects or status admission changed")
	}
	if got := s.Affixes(); len(got) != 1 || got[0] != good.Affixes[0] {
		t.Fatal("negative affix leaked", got)
	}
	if poison, _, _, _, _, _, _ := s.CombatTriggers(); poison != 0 {
		t.Fatal("harmful carrier still triggers")
	}
	if got, _ := s.AbsorbDamage(10); got != 10 {
		t.Fatal("suppressed harmful shield consumed")
	}
	if got, _, _, _ := s.ConsumeAttackModifiers(false, true, false, false); got != 0 {
		t.Fatal("suppressed one-shot consumed")
	}
	if got := s.RemoveEffectiveWhere(func(d StatusDef) bool { return d.BreakOnDamage }); len(got) != 0 {
		t.Fatal("suppressed status broken by damage")
	}
	raw, effective := 0, 0
	s.Each(func(Active) { raw++ })
	s.EachEffective(func(Active) { effective++ })
	if raw != 3 || effective != 2 || !s.Has(bad.ID) || s.HasEffective(bad.ID) {
		t.Fatalf("raw=%d effective=%d", raw, effective)
	}
	saved := s.SaveForLogout(Ticks(1000), time.Unix(1000, 0), 0)
	blob, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip []SavedStatus
	if err := json.Unmarshal(blob, &roundtrip); err != nil {
		t.Fatal(err)
	}
	restored := RestoreSavedStatuses(roundtrip, 10, time.Unix(1000, 0), 0)
	if restored.Count() != 3 || !restored.SuppressesHarmful() || restored.DamageReductionCapPct() != 95 {
		t.Fatal("save/restore lost suppressed instances or new source properties")
	}
	s.Expire(Ticks(5000))
	if s.Controlled() != ControlStun || !s.BlocksMovement() || !s.HasEffective(bad.ID) || s.DamageTakenIncreasePct(false) != 200 {
		t.Fatal("remaining negative lifetime did not resume")
	}
}

func TestExplicitSuppressionPeriodicExpiryAndNoBacklog(t *testing.T) {
	for _, end := range []Tick{Ticks(5000), Ticks(5300), Ticks(10000)} {
		s := NewStatusSet()
		bad := StatusDef{ID: 800020, TypeKnown: true, Type: StatusBad, DurationSec: 20, IntervalSec: 1, PeriodicDamageFraction100: 75,
			HPChanges: []ResourceChange{{Value: -10}}}
		s.Apply(bad, 0, 0)
		s.Apply(explicitMitigationBuff(), 0, 0)
		if due := s.DueTicks(Ticks(1000)); len(due) != 0 {
			t.Fatal("suppressed tick emitted")
		}
		if gone := s.ExpireHarmfulSuppression(end); len(gone) != 1 {
			t.Fatal("immunity did not expire", end)
		}
		if due := s.DueTicks(end); len(due) != 0 {
			t.Fatal("suppressed backlog emitted after long frame", end)
		}
		due := s.DueTicks(end + Ticks(1000))
		if len(due) != 1 || due[0].PeriodicDamageRemainder100 != 75 || due[0].PeriodicDamageTickExtra != 0 {
			t.Fatal("suppressed fractional ticks accumulated damage", due)
		}
	}
	// With regular frames, the first tick exactly at immunity's exclusive end lands.
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 800021, TypeKnown: true, Type: StatusBad, DurationSec: 20, IntervalSec: 1}, 0, 0)
	s.Apply(explicitMitigationBuff(), 0, 0)
	for ms := 1000; ms < 5000; ms += 1000 {
		if len(s.DueTicks(Ticks(ms))) != 0 {
			t.Fatal("tick during suppression")
		}
	}
	s.ExpireHarmfulSuppression(Ticks(5000))
	if len(s.DueTicks(Ticks(5000))) != 1 {
		t.Fatal("exact-end tick should resume")
	}
	// Harmful states expiring during immunity still disappear naturally.
	s = NewStatusSet()
	s.Apply(StatusDef{ID: 800022, TypeKnown: true, Type: StatusBad, DurationSec: 2, IntervalSec: 1}, 0, 0)
	s.Apply(explicitMitigationBuff(), 0, 0)
	s.DueTicks(Ticks(2000))
	s.Expire(Ticks(2000))
	if s.Has(800022) || !s.SuppressesHarmful() {
		t.Fatal("harmful expiry was paused")
	}
}

func TestExplicitSuppressionLegacyBlockHarmfulAndPositiveControl(t *testing.T) {
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 800030, TypeKnown: true, Type: StatusBad, DurationSec: 10, Control: ControlStun, IntervalSec: 1}, 0, 0)
	s.Apply(StatusDef{ID: StatusSpiritLock, DurationSec: 10, BlockHarmful: true}, 0, 0)
	if s.Controlled() != ControlStun || len(s.DueTicks(Ticks(1000))) != 1 {
		t.Fatal("legacy BlockHarmful must not suppress preexisting control or DOT")
	}
	s.Apply(explicitMitigationBuff(), 0, 0)
	s.Apply(StatusDef{ID: StatusBerserk, TypeKnown: true, Type: StatusGood, DurationSec: 10, Control: ControlSilence, BlockHarmful: true}, 0, 0)
	if s.Controlled() != ControlSilence {
		t.Fatal("beneficial control semantics must remain effective")
	}
}

func TestExplicitSuppressionMonsterFrequencyPenalty(t *testing.T) {
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 800040, TypeKnown: true, Type: StatusBad, DurationSec: 10, AttackSpeedIsFrequency: true,
		Affixes: []Affix{{Attr: AttrAtkSpeed, Mode: ModePercent, Value: 50}}}, 0, 0)
	base := Stats{AtkSpeedMS: 1000}
	if got := ApplyMonsterStatusStats(base, s).AtkSpeedMS; got != 2000 {
		t.Fatal(got)
	}
	s.Apply(explicitMitigationBuff(), 0, 0)
	if got := ApplyMonsterStatusStats(base, s).AtkSpeedMS; got != 1000 {
		t.Fatal("negative frequency effect leaked", got)
	}
}

func TestExplicitSuppressionOverlayAndMixedInstanceConsumption(t *testing.T) {
	s := NewStatusSet()
	bad := StatusDef{ID: 800060, TypeKnown: true, Type: StatusBad, DurationSec: 20, NextBasicAttackPct: 100}
	good := StatusDef{ID: 800061, TypeKnown: true, Type: StatusGood, DurationSec: 20}
	s.Apply(bad, 0, 0)
	overlay := StatusOverlay{{Existing: bad.ID, Incoming: good.ID}: OverlayBlock,
		{Existing: bad.ID, Incoming: explicitMitigationBuff().ID}: OverlayCounteract}
	if !s.BlockedByOverlay(good, overlay) {
		t.Fatal("baseline block missing")
	}
	if !s.ApplyWithOverlay(explicitMitigationBuff(), 0, 0, overlay).Applied || !s.Has(bad.ID) {
		t.Fatal("suppression must ignore negative counteraction without deleting original")
	}
	if s.BlockedByOverlay(good, overlay) || !s.ApplyWithOverlay(good, 0, 0, overlay).Applied {
		t.Fatal("suppressed negative matrix rule still blocks beneficial statuses")
	}
	positiveSameID := bad
	positiveSameID.Type, positiveSameID.NextBasicAttackPct = StatusGood, 20
	if !s.ApplyWithOverlay(positiveSameID, 0, 0, StatusOverlay{{Existing: bad.ID, Incoming: bad.ID}: OverlayCoexist}).Applied {
		t.Fatal("mixed status-level classification setup failed")
	}
	bonus, _, _, removed := s.ConsumeAttackModifiers(false, true, false, false)
	if bonus != 20 || len(removed) != 0 || !s.Has(bad.ID) || s.HasEffective(bad.ID) {
		t.Fatal("consuming a beneficial layer deleted its suppressed harmful sibling")
	}
}

func TestExplicitSuppressionLongFrameRemovesExpiredDebuffBeforeStatsResume(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		s := NewStatusSet()
		s.Apply(StatusDef{ID: 800070, TypeKnown: true, Type: StatusBad, DurationSec: 2, Control: ControlStun}, 0, 0)
		s.Apply(explicitMitigationBuff(), 0, 0)
		gone := s.ExpireHarmfulSuppression(Ticks(6000))
		if len(gone) != 2 || s.Count() != 0 || s.Controlled() != ControlNone {
			t.Fatal("same-frame map order briefly restored expired harmful state", gone)
		}
	}
}

func TestExplicitSuppressionOverlayPreservesMixedSiblingsByInstance(t *testing.T) {
	for _, sameID := range []bool{false, true} {
		for _, rule := range []OverlayRule{OverlayReplace, OverlayMustReplace, OverlayCounteract} {
			s := NewStatusSet()
			bad := StatusDef{ID: 800080, TypeKnown: true, Type: StatusBad, DurationSec: 20, Control: ControlStun}
			good := StatusDef{ID: bad.ID, TypeKnown: true, Type: StatusGood, DurationSec: 20}
			s.Apply(bad, 0, 0)
			s.ApplyWithOverlay(good, 0, 0, StatusOverlay{{Existing: bad.ID, Incoming: good.ID}: OverlayCoexist})
			s.Apply(explicitMitigationBuff(), 0, 0)
			incoming := StatusDef{ID: 800081, TypeKnown: true, Type: StatusGood, DurationSec: 10}
			if sameID {
				incoming.ID = bad.ID
			}
			result := s.ApplyWithOverlay(incoming, Ticks(1000), 0, StatusOverlay{{Existing: bad.ID, Incoming: incoming.ID}: rule})
			if !s.Has(bad.ID) || len(result.Removed) != 0 {
				t.Fatalf("sameID=%v rule=%d erased/reported removal of suppressed sibling: %+v", sameID, rule, result)
			}
			if rule == OverlayCounteract {
				if !result.Counteracted || result.Applied || s.HasEffective(bad.ID) {
					t.Fatal("counteraction did not consume only positive layer", result)
				}
			} else if !result.Applied || result.Counteracted {
				t.Fatal("replacement did not apply", result)
			}
			var badLayers int
			s.Each(func(a Active) {
				if a.Def.Harmful() {
					badLayers++
					if a.ExpireAt != Ticks(20000) {
						t.Fatal("replacement changed suppressed expiry")
					}
				}
			})
			if badLayers != 1 {
				t.Fatal("suppressed layer count changed", badLayers)
			}
			s.ExpireHarmfulSuppression(Ticks(5000))
			if s.Controlled() != ControlStun {
				t.Fatal("remaining suppressed control never resumed")
			}
		}
	}
}

func TestExplicitSuppressionExpiredDOTDoesNotReplayItsFinalTick(t *testing.T) {
	for _, lifetime := range []int32{5, 6} {
		for _, now := range []Tick{Ticks(5000), Ticks(7000)} {
			if now < Ticks(int(lifetime)*1000) {
				continue
			}
			s := NewStatusSet()
			bad := StatusDef{ID: 800090, TypeKnown: true, Type: StatusBad, DurationSec: lifetime, IntervalSec: 1}
			s.Apply(bad, 0, 0)
			s.Apply(explicitMitigationBuff(), 0, 0)
			for ms := 1000; ms < 5000; ms += 1000 {
				s.DueTicks(Ticks(ms))
			}
			s.ExpireHarmfulSuppression(now)
			if s.Has(bad.ID) || len(s.DueTicks(now)) != 0 {
				t.Fatalf("expired DOT revived or replayed: lifetime=%d now=%d", lifetime, now)
			}
		}
	}
	// Unrelated ordinary states retain their established last-tick order.
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 800091, TypeKnown: true, Type: StatusBad, DurationSec: 5, IntervalSec: 1}, 0, 0)
	s.ExpireHarmfulSuppression(Ticks(5000))
	if !s.Has(800091) || len(s.DueTicks(Ticks(5000))) != 1 {
		t.Fatal("ordinary final-DOT behavior changed")
	}
	s.Expire(Ticks(5000))
	if s.Has(800091) {
		t.Fatal("ordinary state failed to expire")
	}
}

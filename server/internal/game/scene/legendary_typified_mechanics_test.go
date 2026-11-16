package scene

import (
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// typifiedIncoming records post-mitigation damage using the same strike fixture as the existing D05 floor test.
func typifiedIncoming(t *testing.T, id string, race domain.Race, blow combat.Blow, hp, atk int32) int32 {
	t.Helper()
	s, p, m := universalSoloFixture(t, id, race, domain.Learned{})
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MinAtk: atk, MaxAtk: atk, MAtk: atk}
	p.HP, p.MaxHP = hp, hp
	before := p.HP
	s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, blow)
	return before - p.HP
}

func TestTypifiedPassiveWholeCharacterStats(t *testing.T) {
	for _, tc := range []struct {
		id   string
		race domain.Race
		set  func(*domain.Stats)
		got  func(domain.Stats) int32
		want int32
	}{
		{"TY-D01", domain.Warrior, func(s *domain.Stats) { s.Def = 100 }, func(s domain.Stats) int32 { return s.Def }, 180},
		{"TY-D02", domain.Healer, func(s *domain.Stats) { s.MDef = 100 }, func(s domain.Stats) int32 { return s.MDef }, 180},
		{"TY-D03", domain.Warrior, func(s *domain.Stats) { s.MaxHP = 100 }, func(s domain.Stats) int32 { return s.MaxHP }, 135},
		{"TY-U06", domain.Warrior, func(s *domain.Stats) { s.MaxMP = 100 }, func(s domain.Stats) int32 { return s.MaxMP }, 135},
		{"TY-U09", domain.Warrior, func(s *domain.Stats) { s.MoveSpeed = 100 }, func(s domain.Stats) int32 { return s.MoveSpeed }, 160},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, _ := universalSoloFixture(t, tc.id, tc.race, domain.Learned{})
			st := domain.Stats{}
			tc.set(&st)
			cs := s.legendaryContextsFor(p, p, combat.Blow{}, event.DamageDealt{}, true)
			if len(cs) != 1 {
				t.Fatalf("missing typified context: %d", len(cs))
			}
			h := legendaryHandlers[cs[0].Power.ID].PassiveStats
			if h == nil {
				t.Fatal("missing PassiveStats")
			}
			h(&cs[0], &st)
			if tc.got(st) != tc.want {
				t.Fatalf("%s got %d want %d (whole-character, not gear delta)", tc.id, tc.got(st), tc.want)
			}
		})
	}
}

func TestTypifiedIndependentDamageReduction(t *testing.T) {
	hit := combat.Blow{SkillID: 10002, GuaranteedHit: true, SkillPct: 100}
	area := hit
	area.Area = true
	magic := hit
	magic.Magic = true
	dot := hit
	dot.Origin = domain.DamageDOTOrigin
	if got := typifiedIncoming(t, "TY-D05", domain.Warrior, area, 10000, 1000); got != 400 {
		t.Fatalf("D05 area 60%% got %d", got)
	}
	if got := typifiedIncoming(t, "TY-D06", domain.Warrior, hit, 10000, 1000); got != 600 {
		t.Fatalf("D06 single 40%% including non-basic got %d", got)
	}
	if got := typifiedIncoming(t, "TY-D07", domain.Warrior, hit, 10000, 1000); got != 600 {
		t.Fatalf("D07 physical 40%% got %d", got)
	}
	if got := typifiedIncoming(t, "TY-D08", domain.Healer, magic, 10000, 1000); got != 600 {
		t.Fatalf("D08 magic 40%% got %d", got)
	}
	if got := typifiedIncoming(t, "TY-D19", domain.Warrior, dot, 10000, 1000); got != 350 {
		t.Fatalf("D19 dot 65%% got %d", got)
	}
	// Crit + oversized-hit clauses together must cap at 70% and still escape the 25% floor.
	s, p, m := universalSoloFixture(t, "TY-D10", domain.Warrior, domain.Learned{})
	p.HP, p.MaxHP = 10000, 10000
	dealt := event.DamageDealt{Amount: 2000, Src: m.ID, Dst: p.ID}
	dealt.Flag = event.DamageCrit
	bp := s.typifiedPoolReduction(m, p, combat.Blow{SkillID: 10002, GuaranteedHit: true, GuaranteedCrit: true, SkillPct: 100}, dealt)
	if bp != 7000 {
		t.Fatalf("D10 crit+heavy typified pool %d want 7000 (70%% cap, no 25%% floor)", bp)
	}
}

func TestTypifiedResourceTriggersSkipBasicAndDoNotRecurse(t *testing.T) {
	t.Run("TY-U02/skip-basic", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U02", domain.Warrior, domain.Learned{10002: 1})
		s.refreshStats(p)
		p.MaxMP, p.MP = 1000, 400
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true, SkillPct: 100})
		if p.MP != 400 {
			t.Fatalf("U02 must not restore on a basic attack: %d", p.MP)
		}
		p.MP = 500
		before := p.MP
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		native, _ := s.skills.Get(10002, 1)
		want := before - native.MPCost + typifiedPctOf(p.MaxMP, 6)
		if p.MP != want {
			t.Fatalf("U02 skill restore MP %d want %d", p.MP, want)
		}
	})
	t.Run("TY-U03/cost-and-refund", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U03", domain.Warrior, domain.Learned{10002: 1})
		s.refreshStats(p)
		p.MP = p.MaxMP
		before := p.MP
		native, _ := s.skills.Get(10002, 1)
		paidWant := (native.MPCost*70 + 99) / 100
		d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		if d.MPCost != paidWant {
			t.Fatalf("U03 prepared cost %d want %d", d.MPCost, paidWant)
		}
		refund := typifiedPctOf(paidWant, 40)
		if before-p.MP != paidWant-refund {
			t.Fatalf("U03 net spend %d want %d", before-p.MP, paidWant-refund)
		}
	})
	t.Run("TY-U06/self-restore", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U06", domain.Healer, domain.Learned{10301: 1})
		a := nativeRepairAlly(t, s, p, 2, 140)
		a.HP, a.MaxHP = 1000, 2000
		s.refreshStats(p)
		p.MP = p.MaxMP / 2
		beforeSelf, beforeAlly := p.MP, a.MP
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10301, Target: a.ID})
		native, _ := s.skills.Get(10301, 1)
		gain := domain.SaturateInt32(int64(p.MaxMP) * 250 / 10000)
		want := beforeSelf - native.MPCost + gain
		if p.MP != want {
			t.Fatalf("U06 should restore the caster, MP %d want %d", p.MP, want)
		}
		if a.MP != beforeAlly {
			t.Fatalf("U06 restored the target instead of the caster: %d", a.MP)
		}
	})
	t.Run("TY-D11/no-resource-recursion", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D11", domain.Healer, domain.Learned{})
		p.Stats = domain.Stats{}
		m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
		p.HP, p.MaxHP, p.MP, p.MaxMP = 10000, 10000, 10000, 10000
		beforeHP, beforeMP := p.HP, p.MP
		s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true, SkillPct: 100})
		taken, absorbed := beforeHP-p.HP, beforeMP-p.MP
		if absorbed != typifiedPctOf(1000, 45) || taken != 1000-absorbed {
			t.Fatalf("D11 absorb HP %d MP %d", taken, absorbed)
		}
	})
}

func TestTypifiedHealWindowAndShieldSources(t *testing.T) {
	t.Run("TY-D12/incoming-heal-and-window", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D12", domain.Warrior, domain.Learned{})
		p.MaxHP, p.HP = 1000, 1
		cs := s.legendaryContextsFor(p, p, combat.Blow{}, event.DamageDealt{Dst: p.ID}, true)
		if len(cs) != 1 {
			t.Fatal("missing D12 context")
		}
		if got := typifiedIncomingHeal(&cs[0], 100); got != 220 {
			t.Fatalf("D12 incoming heal hook %d want 220", got)
		}
		// typifiedIncomingHeal already marks the window on the first call.
		if !typifiedHasWindow(p, "TY-D12") {
			t.Fatal("D12 missing post-heal DR window")
		}
		p.Stats = domain.Stats{}
		m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
		p.HP, p.MaxHP = 10000, 10000
		before := p.HP
		s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true, SkillPct: 100})
		if before-p.HP != 650 {
			t.Fatalf("D12 window 35%% DR got %d", before-p.HP)
		}
	})
	t.Run("TY-D13/ally-not-self", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-D13", domain.Warrior, domain.Learned{})
		s.refreshStats(p)
		if p.MaxHP < 200 {
			t.Fatalf("fixture MaxHP %d", p.MaxHP)
		}
		p.HP = p.MaxHP / 2
		before := p.HP
		ally := nativeRepairAlly(t, s, p, 2, 140)
		shield := domain.StatusDef{ID: 199921, Level: 1, Name: "ally shield", TypeKnown: true, Type: domain.StatusGood, DurationSec: 6, DamageShield: 100}
		if !s.applyStatusDef(p, shield, ally.ID) {
			t.Fatal("ally shield rejected")
		}
		if universalShieldValue(p) != 300 {
			t.Fatalf("ally shield should be +200%%, got %d", universalShieldValue(p))
		}
		wantHP := before + typifiedPctOf(300, 20)
		if p.HP != wantHP {
			t.Fatalf("D13 convert-to-heal once got HP %d want %d", p.HP, wantHP)
		}
		hp := p.HP
		second := shield
		second.ID = 199922
		if !s.applyStatusDef(p, second, ally.ID) {
			t.Fatal("second ally shield rejected")
		}
		if p.HP != hp {
			t.Fatalf("same source must not convert again: %d", p.HP)
		}
		selfHP := p.HP
		selfShield := domain.StatusDef{ID: 199923, Level: 1, Name: "self shield", TypeKnown: true, Type: domain.StatusGood, DurationSec: 6, DamageShield: 100}
		if !s.applyStatusDef(p, selfShield, p.ID) {
			t.Fatal("self shield rejected")
		}
		if p.HP != selfHP {
			t.Fatalf("self shield triggered D13 heal: %d", p.HP)
		}
	})
	t.Run("TY-U13/clear-on-leave", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U13", domain.Warrior, domain.Learned{10002: 1})
		p.MaxHP, p.HP = 1000, 1000
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		if universalShieldValue(p) != typifiedPctOf(1000, 25) {
			t.Fatalf("U13 cast shield %d", universalShieldValue(p))
		}
		typifiedClearHeldGeneratedShields(s, p)
		if universalShieldValue(p) != 0 {
			t.Fatalf("U13 typified shield survived leave/swap: %d", universalShieldValue(p))
		}
	})
	t.Run("TY-D16/emergency-and-clear", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D16", domain.Warrior, domain.Learned{})
		p.Stats = domain.Stats{}
		m.Stats = domain.Stats{MinAtk: 5000, MaxAtk: 5000}
		p.HP, p.MaxHP = 10000, 10000
		s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true, SkillPct: 100})
		if p.HP > 6000 {
			p.HP = 6000
			s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true, SkillPct: 100})
		}
		if universalShieldValue(p) != typifiedPctOf(p.MaxHP, 35) {
			t.Fatalf("D16 emergency shield %d", universalShieldValue(p))
		}
		typifiedClearHeldGeneratedShields(s, p)
		if universalShieldValue(p) != 0 {
			t.Fatalf("D16 typified shield survived leave/swap: %d", universalShieldValue(p))
		}
	})
}

func TestTypifiedStatusRulesAndCooldowns(t *testing.T) {
	t.Run("TY-U08/enemy-slow", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U08", domain.Warrior, domain.Learned{})
		d := domain.StatusDef{ID: 1009, Level: 1, Name: "slow", TypeKnown: true, Type: domain.StatusBad, DurationSec: 8, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50, Mode: domain.ModePercent}}}
		if s.applyStatusDef(p, d, m.ID) {
			t.Fatal("U08 should cleanse and reject the first move limit")
		}
		if p.Status.Has(1009) {
			t.Fatal("U08 left the slow in place")
		}
		if !typifiedHasWindow(p, "TY-U08") {
			t.Fatal("U08 immunity window missing")
		}
		if s.applyStatusDef(p, d, m.ID) {
			t.Fatal("U08 window should reject a second slow")
		}
	})
	t.Run("TY-D09/control-duration-and-shield", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D09", domain.Warrior, domain.Learned{})
		s.refreshStats(p)
		d := domain.StatusDef{ID: 1034, Level: 1, Name: "stun", TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, Control: domain.ControlStun, Immobile: 1}
		if !s.applyStatusDef(p, d, m.ID) {
			t.Fatal("D09 should still apply shortened control")
		}
		got := verifiedActive(t, p, 1034)
		if got.Def.LegendaryModifiers.ControlDurationReductionBP < 5000 {
			t.Fatalf("D09 missing 50%% control cut: %+v", got.Def.LegendaryModifiers)
		}
		want := typifiedPctOf(p.MaxHP, 35)
		if got := universalShieldValue(p); got != want {
			t.Fatalf("D09 on-control shield %d want %d (35%% of MaxHP %d)", got, want, p.MaxHP)
		}
	})
	t.Run("TY-D14/defense-loss", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D14", domain.Warrior, domain.Learned{})
		d := domain.StatusDef{ID: 1990, Level: 1, Name: "shred", TypeKnown: true, Type: domain.StatusBad, DurationSec: 8, Affixes: []domain.Affix{{Attr: domain.AttrDef, Value: -20, Mode: domain.ModePercent}, {Attr: domain.AttrMDef, Value: -20, Mode: domain.ModePercent}}}
		if s.applyStatusDef(p, d, m.ID) {
			t.Fatal("D14 should cleanse and reject defense shred")
		}
	})
	t.Run("TY-D15/attack-slow", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D15", domain.Warrior, domain.Learned{})
		d := domain.StatusDef{ID: 1991, Level: 1, Name: "atk slow", TypeKnown: true, Type: domain.StatusBad, DurationSec: 8, Affixes: []domain.Affix{{Attr: domain.AttrAtkSpeed, Value: 50, Mode: domain.ModePercent}}}
		if s.applyStatusDef(p, d, m.ID) {
			t.Fatal("D15 should immune attack-speed shred")
		}
	})
	t.Run("TY-D20/poison", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D20", domain.Warrior, domain.Learned{})
		d := domain.StatusDef{ID: domain.StatusPoison, Level: 1, Name: "poison", TypeKnown: true, Type: domain.StatusBad, DurationSec: 10}
		if s.applyStatusDef(p, d, m.ID) {
			t.Fatal("D20 should cleanse the first poison and grant immunity")
		}
	})
	t.Run("TY-U01/chant", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U01", domain.Warlock, domain.Learned{10401: 1})
		started := time.Unix(100, 0)
		s.now = func() time.Time { return started }
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10401, Target: m.ID})
		if len(s.casts) != 1 {
			t.Fatal("U01 cast rejected")
		}
		native, _ := s.skills.Get(10401, 1)
		want := native.PrepareMS * 40 / 100
		got := s.casts[0].releaseAtWall.Sub(started)
		if got != time.Duration(want)*time.Millisecond {
			t.Fatalf("U01 chant %v want %d ms", got, want)
		}
	})
	t.Run("TY-U15/cooldown-floor", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U15", domain.Warrior, domain.Learned{10002: 1})
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		ready := p.CapturePlayerRuntime(s.tick).SkillReadyIn[10002]
		if ready != domain.Ticks(600) {
			t.Fatalf("U15 remaining cooldown %d want %d", ready, domain.Ticks(600))
		}
	})
}

func TestTypifiedUtilityGeometryAndDetect(t *testing.T) {
	t.Run("TY-U10/full-targets-no-split", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U10", domain.Healer, domain.Learned{10304: 1})
		near := nativeRepairAlly(t, s, p, 2, 500)
		far := nativeRepairAlly(t, s, p, 3, 2000)
		near.HP, near.MaxHP = 1, 10000
		far.HP, far.MaxHP = 1, 10000
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10304})
		if near.HP <= 1 {
			t.Fatalf("U10 should heal in the enlarged radius, near=%d", near.HP)
		}
		if far.HP != 1 {
			t.Fatalf("U10 must not invent a global heal, far=%d", far.HP)
		}
	})
	t.Run("TY-U19/inherent-detect-radius", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U19", domain.Warrior, domain.Learned{})
		cs := s.legendaryContextsFor(p, p, combat.Blow{}, event.DamageDealt{}, true)
		if len(cs) != 1 || cs[0].RollValue("aura", 400) != 400 {
			t.Fatalf("U19 aura default %v", len(cs))
		}
	})
}

func TestTypifiedApplyShieldHelperLands(t *testing.T) {
	s, p, _ := universalSoloFixture(t, "TY-U13", domain.Warrior, domain.Learned{})
	p.MaxHP, p.HP = 1000, 1000
	cs := s.legendaryContextsFor(p, p, combat.Blow{SkillID: 10002}, event.DamageDealt{}, true)
	if len(cs) != 1 {
		t.Fatal("missing U13 context")
	}
	cs[0].RootID = 1
	typifiedApplyShield(&cs[0], p, 25, 5)
	if got := universalShieldValue(p); got != typifiedPctOf(p.MaxHP, 25) {
		t.Fatalf("typifiedApplyShield helper %d maxHP %d window %v", got, p.MaxHP, typifiedHasWindow(p, "TY-U13"))
	}
}

func typifiedHookContext(t *testing.T, id string, race domain.Race) (*Scene, *entity.Entity, *entity.Entity, legendaryContext) {
	t.Helper()
	s, p, m := universalSoloFixture(t, id, race, domain.Learned{})
	cs := s.legendaryContextsFor(p, m, combat.Blow{SkillID: 10002}, event.DamageDealt{}, true)
	if len(cs) != 1 {
		t.Fatalf("missing %s context: %d", id, len(cs))
	}
	c := cs[0]
	c.RootID = 1
	c.Skill = 10002
	c.IsBasic = false
	c.Def = domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, CooldownMS: 8000}
	return s, p, m, c
}

func TestTypifiedRemainingMainBehaviors(t *testing.T) {
	t.Run("TY-U04/extra-heal-and-mana-once", func(t *testing.T) {
		_, p, _, c := typifiedHookContext(t, "TY-U04", domain.Healer)
		p.MaxHP, p.HP = 2000, 1000
		p.MaxMP, p.MP = 1000, 500
		c.Target = p
		typifiedHealResolved(&c, 80, 80, 0)
		if p.HP != 1000+typifiedPctOf(2000, 5) {
			t.Fatalf("U04 extra heal HP %d", p.HP)
		}
		if p.MP != 500+typifiedPctOf(1000, 3) {
			t.Fatalf("U04 self mana %d", p.MP)
		}
		hp, mp := p.HP, p.MP
		typifiedHealResolved(&c, 80, 80, 0)
		if p.HP != hp || p.MP != mp {
			t.Fatalf("U04 retriggered on same cast: HP %d MP %d", p.HP, p.MP)
		}
	})
	t.Run("TY-U05/hot-tick-and-refresh", func(t *testing.T) {
		_, p, _, c := typifiedHookContext(t, "TY-U05", domain.Healer)
		d := domain.StatusDef{ID: 199501, Level: 1, Name: "hot", TypeKnown: true, Type: domain.StatusGood, DurationSec: 10, IntervalSec: 2, SkillApplied: true, HPChanges: []domain.ResourceChange{{Value: 20}}}
		typifiedStatusApplied(&c, &d)
		if d.LegendaryModifiers.HealingTickBP < 10000 {
			t.Fatalf("U05 hot tick BP %d", d.LegendaryModifiers.HealingTickBP)
		}
		if p.Status == nil {
			p.Status = domain.NewStatusSet()
		}
		p.Status.Apply(d, c.Scene.tick, p.ID)
		before := verifiedActive(t, p, 199501).ExpireAt
		c.Scene.tick += domain.Ticks(3000)
		c.Target = p
		typifiedHealResolved(&c, 10, 10, 0)
		got := verifiedActive(t, p, 199501).ExpireAt
		if got <= before {
			t.Fatalf("U05 should refresh owned HoT remaining: before=%d after=%d", before, got)
		}
	})
	t.Run("TY-U07/teleport-window", func(t *testing.T) {
		_, p, _, c := typifiedHookContext(t, "TY-U07", domain.Assassin)
		typifiedTeleported(&c, domain.Pos{})
		if !typifiedHasWindow(p, "TY-U07") {
			t.Fatal("U07 missing post-move window")
		}
		d := domain.SkillDef{ID: 18002, Kind: domain.SkillSingle, Dist: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectTeleportPoint}}}
		typifiedPrepare(&c, &d)
		if d.Dist <= 100 {
			t.Fatalf("U07 should enlarge teleport/follow-up range, Dist %d", d.Dist)
		}
	})
	t.Run("TY-U11/skill-buff-duration-not-self", func(t *testing.T) {
		_, _, _, c := typifiedHookContext(t, "TY-U11", domain.Warrior)
		d := domain.StatusDef{ID: 199511, Level: 1, Name: "bless", TypeKnown: true, Type: domain.StatusGood, DurationSec: 8, SkillApplied: true}
		typifiedStatusApplied(&c, &d)
		if d.LegendaryModifiers.DurationBP < 10000 {
			t.Fatalf("U11 duration BP %d", d.LegendaryModifiers.DurationBP)
		}
		self := d
		self.LegendaryPowerID = "TY-U11"
		self.LegendaryModifiers = domain.LegendaryStatusModifiers{}
		typifiedStatusApplied(&c, &self)
		if self.LegendaryModifiers.DurationBP != 0 {
			t.Fatalf("U11 must not extend typified-generated duration: %d", self.LegendaryModifiers.DurationBP)
		}
	})
	t.Run("TY-U12/window-power-and-prepare", func(t *testing.T) {
		_, p, _, c := typifiedHookContext(t, "TY-U12", domain.Warrior)
		typifiedApplyTimedAffix(&c, p, 5)
		if !typifiedHasWindow(p, "TY-U12") {
			t.Fatal("U12 window missing")
		}
		c.Def.PrepareMS = 0
		if got := typifiedBeforeDamage(&c, 1000); got != universalScaleBP(1000, 2500) {
			t.Fatalf("U12 instant power %d", got)
		}
		d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, PrepareMS: 2000}
		typifiedPrepare(&c, &d)
		if d.LegendaryModifiers.PrepareReductionBP < 4000 {
			t.Fatalf("U12 chant cut %d", d.LegendaryModifiers.PrepareReductionBP)
		}
		instant := domain.SkillDef{ID: 10301, Kind: domain.SkillSingle, PrepareMS: 0}
		typifiedPrepare(&c, &instant)
		if instant.LegendaryModifiers.HealingBP < 2500 {
			t.Fatalf("U12 instant heal amp %d", instant.LegendaryModifiers.HealingBP)
		}
	})
	t.Run("TY-U14/high-mana-power", func(t *testing.T) {
		_, p, _, c := typifiedHookContext(t, "TY-U14", domain.Healer)
		p.MaxMP, p.MP = 1000, 1000
		c.PaidMP = 80
		if got := typifiedBeforeDamage(&c, 1000); got != universalScaleBP(1000, 6000) {
			t.Fatalf("U14 paid 8%% power %d", got)
		}
		c.PaidMP = 70
		if got := typifiedBeforeDamage(&c, 1000); got != 1000 {
			t.Fatalf("U14 below 8%% should be unchanged %d", got)
		}
		c.PaidMP = 80
		c.RootID = 2
		p.HP, p.MaxHP = 1000, 2000
		c.Target = p
		typifiedHealResolved(&c, 100, 100, 0)
		if p.HP != 1000+typifiedPctOf(100, 60) {
			t.Fatalf("U14 resolved heal amp HP %d", p.HP)
		}
		c.PaidMP = 70
		c.RootID = 3
		before := p.HP
		typifiedHealResolved(&c, 100, 100, 0)
		if p.HP != before {
			t.Fatalf("U14 below 8%% should not extra-heal %d", p.HP)
		}
	})
	t.Run("TY-U16/heal-amp-and-overflow-shield", func(t *testing.T) {
		_, _, m, c := typifiedHookContext(t, "TY-U16", domain.Healer)
		d := domain.SkillDef{ID: 10301, Kind: domain.SkillSingle}
		typifiedPrepare(&c, &d)
		if d.LegendaryModifiers.HealingBP < 10000 {
			t.Fatalf("U16 healing BP %d", d.LegendaryModifiers.HealingBP)
		}
		m.MaxHP, m.HP = 1000, 1000
		c.Target = m
		typifiedHealResolved(&c, 100, 200, 100)
		if got := universalShieldValue(m); got != typifiedPctOf(100, 50) {
			t.Fatalf("U16 overflow shield %d", got)
		}
	})
	t.Run("TY-U17/resurrect-floor-and-window", func(t *testing.T) {
		_, _, m, c := typifiedHookContext(t, "TY-U17", domain.Healer)
		m.MaxHP, m.HP = 1000, 1
		m.MaxMP, m.MP = 200, 0
		c.Target = m
		typifiedResurrected(&c)
		if m.HP != 1000 {
			t.Fatalf("U17 life floor %d", m.HP)
		}
		if m.MP != typifiedPctOf(200, 50) {
			t.Fatalf("U17 mana restore %d", m.MP)
		}
		if !typifiedHasWindow(m, "TY-U17") {
			t.Fatal("U17 reduction window missing")
		}
	})
	t.Run("TY-U18/dispel-chance-and-shield", func(t *testing.T) {
		_, _, m, c := typifiedHookContext(t, "TY-U18", domain.Healer)
		d := domain.SkillDef{ID: 10320, Kind: domain.SkillSingle, Effects: []domain.SkillEffect{{ClearHarmful: true, DispelChanceBP: 2000}}}
		typifiedPrepare(&c, &d)
		if d.Effects[0].DispelChanceBP != 9000 {
			t.Fatalf("U18 dispel chance %d", d.Effects[0].DispelChanceBP)
		}
		m.MaxHP, m.HP = 1000, 1000
		c.Target = m
		typifiedDispelled(&c, []domain.Active{{Def: domain.StatusDef{ID: 1001}}})
		if got := universalShieldValue(m); got != typifiedPctOf(1000, 15) {
			t.Fatalf("U18 dispel shield %d", got)
		}
	})
	t.Run("TY-U20/move-speed-window", func(t *testing.T) {
		_, p, _, c := typifiedHookContext(t, "TY-U20", domain.Warrior)
		typifiedCastReleased(&c)
		if !typifiedHasWindow(p, "TY-U20") {
			t.Fatal("U20 post-cast window missing")
		}
	})
	t.Run("TY-D17/controlled-reduction", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D17", domain.Warrior, domain.Learned{})
		p.Stats = domain.Stats{}
		m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
		p.HP, p.MaxHP = 10000, 10000
		stun := domain.StatusDef{ID: 1034, Level: 1, Name: "stun", TypeKnown: true, Type: domain.StatusBad, DurationSec: 8, Control: domain.ControlStun, Immobile: 1}
		if !s.applyStatusDef(p, stun, m.ID) {
			t.Fatal("D17 fixture control rejected")
		}
		before := p.HP
		s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true, SkillPct: 100})
		if got := before - p.HP; got != 550 {
			t.Fatalf("D17 controlled 45%% got %d", got)
		}
	})
	t.Run("TY-D17/control-end-natural-dispel-handoff-unequip", func(t *testing.T) {
		slow := domain.StatusDef{ID: 199917, Level: 1, Name: "slow", TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, EffectHitBP: 100000000, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50, Mode: domain.ModePercent}}}
		stun := func(id domain.StatusID, sec int32, control domain.Control) domain.StatusDef {
			return domain.StatusDef{ID: id, Level: 1, Name: "control", TypeKnown: true, Type: domain.StatusBad, DurationSec: sec, EffectHitBP: 100000000, Control: control, Immobile: 1}
		}
		hasteOK := func(p *entity.Entity) bool {
			if !typifiedHasWindow(p, "TY-D17") {
				return false
			}
			ok := false
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.ID == typifiedStatusID("TY-D17") {
					for _, aff := range a.Def.Affixes {
						if aff.Attr == domain.AttrMoveSpeed && aff.Value == 40 && aff.Mode == domain.ModePercent {
							ok = true
						}
					}
				}
			})
			return ok
		}
		t.Run("natural", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-D17", domain.Warrior, domain.Learned{})
			if !s.applyStatusDef(p, stun(1034, 1, domain.ControlStun), m.ID) || !s.applyStatusDef(p, slow, m.ID) {
				t.Fatal("natural fixtures rejected")
			}
			s.tick += domain.Ticks(1000)
			s.stepStatus()
			if p.Status.Has(1034) || p.Status.Has(199917) || !hasteOK(p) {
				t.Fatal("natural expiry did not cleanse slow or grant haste", p.Status.Has(1034), p.Status.Has(199917), typifiedHasWindow(p, "TY-D17"))
			}
			s.tick += domain.Ticks(2000)
			s.stepStatus()
			if typifiedHasWindow(p, "TY-D17") {
				t.Fatal("haste did not expire")
			}
			s.tick += domain.Ticks(2000)
			s.stepStatus()
			if typifiedHasWindow(p, "TY-D17") {
				t.Fatal("haste retriggered without a new control")
			}
		})
		t.Run("dispel", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-D17", domain.Warrior, domain.Learned{})
			if !s.applyStatusDef(p, stun(1034, 8, domain.ControlStun), m.ID) || !s.applyStatusDef(p, slow, m.ID) {
				t.Fatal("dispel fixtures rejected")
			}
			s.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool { return typifiedControlLike(&d) }))
			if p.Status.Has(1034) || p.Status.Has(199917) || !hasteOK(p) {
				t.Fatal("dispel did not cleanse slow or grant haste")
			}
		})
		t.Run("handoff", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-D17", domain.Warrior, domain.Learned{})
			if !s.applyStatusDef(p, stun(1034, 1, domain.ControlStun), m.ID) || !s.applyStatusDef(p, stun(1035, 8, domain.ControlSilence), m.ID) || !s.applyStatusDef(p, slow, m.ID) {
				t.Fatal("handoff fixtures rejected")
			}
			s.tick += domain.Ticks(1000)
			s.stepStatus()
			if !p.Status.Has(1035) || typifiedHasWindow(p, "TY-D17") || !p.Status.Has(199917) {
				t.Fatal("first control ending fired while another control remained")
			}
			s.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == 1035 }))
			if p.Status.Has(1035) || p.Status.Has(199917) || !hasteOK(p) {
				t.Fatal("last control ending missed cleanse or haste")
			}
		})
		t.Run("unequip", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-D17", domain.Warrior, domain.Learned{})
			if !s.applyStatusDef(p, stun(1034, 8, domain.ControlStun), m.ID) || !s.applyStatusDef(p, slow, m.ID) {
				t.Fatal("unequip fixtures rejected")
			}
			p.Player.Worn.Each(func(slot domain.EquipSlot, _ domain.Stack) {
				p.Player.Worn.Set(slot, domain.Stack{})
			})
			s.refreshStats(p)
			s.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool { return typifiedControlLike(&d) }))
			if typifiedHasWindow(p, "TY-D17") {
				t.Fatal("unequipped D17 still granted control-end haste")
			}
			if !p.Status.Has(199917) {
				t.Fatal("unequipped D17 should leave remaining slow")
			}
		})
	})
	t.Run("TY-D18/shield-duration-and-break-heal", func(t *testing.T) {
		s, p, m, c := typifiedHookContext(t, "TY-D18", domain.Warrior)
		s.refreshStats(p)
		shield := domain.StatusDef{ID: 199918, Level: 1, Name: "guard", TypeKnown: true, Type: domain.StatusGood, DurationSec: 6, DamageShield: 200}
		typifiedStatusReceiving(&c, &shield)
		if shield.LegendaryModifiers.DurationBP < 10000 {
			t.Fatalf("D18 duration BP %d", shield.LegendaryModifiers.DurationBP)
		}
		if p.Status == nil {
			p.Status = domain.NewStatusSet()
		}
		p.HP = p.MaxHP / 2
		before := p.HP
		p.Status.Apply(shield, s.tick, m.ID)
		var snapshot []domain.Active
		p.Status.EachEffective(func(a domain.Active) { snapshot = append(snapshot, a) })
		p.Status.Remove(199918)
		typifiedOnShieldsResolved(s, p, snapshot)
		if p.HP != before+typifiedPctOf(200, 50) {
			t.Fatalf("D18 break heal HP %d want %d", p.HP, before+typifiedPctOf(200, 50))
		}
	})
}

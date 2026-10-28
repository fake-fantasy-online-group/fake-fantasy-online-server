package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
	"testing"
)

// These are deliberately separate from the solo coverage numerator.
func TestLegendaryUniversalContractCombinations(t *testing.T) {
	for _, status := range []domain.StatusID{domain.StatusPoison, 1036} {
		t.Run(fmt.Sprintf("critical_snapshot/status=%d", status), func(t *testing.T) {
			critPower := "TY-A28"
			if status == 1036 {
				critPower = "TY-A29"
			}
			race := domain.Warlock
			sourceSkill := int32(10421)
			if status == domain.StatusPoison {
				race = domain.Assassin
				sourceSkill = 10214
			}
			s, p, m := nativeRepairFixture(t, race, domain.Learned{}, critPower, "TY-A11", "TY-A33")
			p.Stats.CritRate = 8000
			p.Stats.MCritRate = 8000
			d := domain.StatusDef{ID: status, SourceSkillID: sourceSkill, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -100}}, TypeKnown: true, Type: domain.StatusBad}
			if !s.applyStatusDef(m, d, p.ID) {
				t.Fatal("periodic apply")
			}
			a := universalSoloActive(t, m, status)
			p.Stats.CritRate = 0
			p.Stats.MCritRate = 0
			p.Player.Worn = domain.NewEquipSet()
			s.invalidateCombatAttributes(p)
			hp, mp := m.HP, p.MP
			s.tickStatus(m, a)
			if hp-m.HP != 250 || p.MP != mp {
				t.Fatal("accepted 80+20 percent crit and 2+0.5 multiplier", hp-m.HP, p.MP-mp)
			}
		})
	}
	t.Run("poison_critical_defense_mana_shield", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, "TY-D10", "TY-D19", "TY-D11")
		p.Status.Apply(domain.StatusDef{ID: 10320, SourceSkillID: 10320, DurationSec: 60, DamageShield: 100}, s.tick, p.ID)
		s.refreshStats(p)
		p.Stats = domain.Stats{}
		p.HP = p.MaxHP
		p.MP = 1000
		d := domain.StatusDef{ID: domain.StatusPoison, SourceSkillID: 10214, HPChanges: []domain.ResourceChange{{Value: -1000}}, TypeKnown: true, Type: domain.StatusBad, LegendaryModifiers: domain.LegendaryStatusModifiers{PeriodicCanCrit: true, PeriodicCritChanceBP: 10000}}
		hp := p.HP
		s.tickStatus(p, domain.Active{Source: m.ID, Def: d})
		if hp-p.HP != 530 || p.MP != 730 || universalShieldValue(p) != 0 {
			t.Fatalf("2000 minus additive30+25%% then mana30%% then shield100: hp%d mp%d shield%d", hp-p.HP, p.MP, universalShieldValue(p))
		}
	})
	t.Run("reflection_no_recursive_reflection", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, "TY-D11", "TY-D07")
		p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
		p.MP = 1000
		p.Status.Apply(domain.StatusDef{ID: 190991, DurationSec: 60, PhysicalReflectPct: 100}, s.tick, p.ID)
		m.Status.Apply(domain.StatusDef{ID: 190992, DurationSec: 60, PhysicalReflectPct: 100}, s.tick, m.ID)
		hp := p.HP
		ev, raw, ref := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
		s.afterStrikeStatuses(p, m, ev, raw, ref)
		n := 0
		for _, o := range s.outbox {
			if e, ok := o.ev.(event.DamageDealt); ok && e.Origin == domain.DamageReflectionOrigin {
				n++
			}
		}
		if hp-p.HP != 700 || p.MP != 700 || n != 1 || m.MaxHP-m.HP != 1000 {
			t.Fatal("reflection consumed/echoed twice", hp-p.HP, p.MP, n, m.MaxHP-m.HP)
		}
	})
	t.Run("class_rectangle_length_width", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{10003: 1}, "ZS-A16", "TY-A02", "TY-A06")
		st := p.Player.Worn.At(domain.SlotWeapon)
		it := s.items[st.Item]
		d, _ := s.skills.Get(10003, 1)
		it.Equip.Type = d.RequiredWeaponType
		s.items[st.Item] = it
		side := putMonster(s, 901, 10000, domain.Stats{}, 350, 190)
		outside := putMonster(s, 902, 10000, domain.Stats{}, 389, 100)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10003, Target: m.ID})
		if side.HP == 10000 || outside.HP != 10000 {
			t.Fatal("final class rectangle288x192 not consumed", side.HP, outside.HP)
		}
		a, n := assassinRegressionDamage(s, m.ID, 10003)
		b, bn := assassinRegressionDamage(s, side.ID, 10003)
		if a <= 0 || a != b || n != 1 || bn != 1 {
			t.Fatal("geometry added attack or changed damage", a, b, n, bn)
		}
	})
	t.Run("single_conversion_preserves_single_bonus", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{10002: 1}, "TY-A05", "TY-A08", "TY-A04")
		side := putMonster(s, 901, 10000, domain.Stats{}, 280, 100)
		outside := putMonster(s, 902, 10000, domain.Stats{}, 299, 100)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		universalSoloDamage(t, s, m.ID, 10002, 130*160/100, 1)
		universalSoloDamage(t, s, side.ID, 10002, 130*160/100, 1)
		if outside.HP != 10000 {
			t.Fatal("single conversion final radius168 escaped")
		}
	})
	t.Run("equipment_defense_basic_base", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, "TY-A39", "TY-D01")
		st := p.Player.Worn.At(domain.SlotWeapon)
		it := s.items[st.Item]
		it.Equip.Bonus.Def = 100
		s.items[st.Item] = it
		p.Look.AtkDist = 100
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
		universalSoloDamage(t, s, m.ID, 0, 128, 1)
	})
	t.Run("native_lifesteal_healing_additive", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{10104: 1}, "TY-A17", "TY-U16")
		p.HP = 1000
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10104, Target: m.ID})
		if p.HP != 1044 {
			t.Fatal("20*(1+0.6+0.6) restoration", p.HP)
		}
	})
}

func TestLegendaryUniversalDirectReductionCombination(t *testing.T) {
	for _, magic := range []bool{false, true} {
		t.Run(fmt.Sprint(magic), func(t *testing.T) {
			typed := "TY-D07"
			if magic {
				typed = "TY-D08"
			}
			s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, "TY-D05", typed, "TY-D17")
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
			p.Status.Apply(domain.StatusDef{ID: 1034, DurationSec: 10, BlockMove: true}, s.tick, m.ID)
			hp := p.HP
			s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, Area: true, Magic: magic, GuaranteedHit: true})
			if hp-p.HP != 300 {
				t.Fatal("direct25+20+25 percent reduction must add", hp-p.HP)
			}
		})
	}
}

func TestLegendaryUniversalBasicFanTargetCoefficientCombination(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, "TY-A01", "TY-A34", "TY-A36")
	p.Look.AtkDist = 100
	side := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
	universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
	universalSoloDamage(t, s, m.ID, 0, 180, 1)
	universalSoloDamage(t, s, side.ID, 0, 98, 1)
}

func TestLegendaryUniversalClassConeComposition(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{10006: 1}, "ZS-A13", "TY-A03", "TY-A04")
	// Native radius150*(1+.2+.5)=255; angle120*1.4=168, not chained radius270.
	a := 75.0 * math.Pi / 180
	inside := putMonster(s, 901, 10000, domain.Stats{}, 100+250*math.Cos(a), 100+250*math.Sin(a))
	far := putMonster(s, 902, 10000, domain.Stats{}, 356, 100)
	a = 85 * math.Pi / 180
	angleOut := putMonster(s, 903, 10000, domain.Stats{}, 100+100*math.Cos(a), 100+100*math.Sin(a))
	assassinRegressionCast(t, s, p, UseSkill{Skill: 10006, Target: m.ID})
	universalSoloDamage(t, s, m.ID, 10006, 188, 1)
	universalSoloDamage(t, s, inside.ID, 10006, 188, 1)
	if far.HP != 10000 || angleOut.HP != 10000 {
		t.Fatal("final class cone boundary", far.HP, angleOut.HP)
	}
}

func TestLegendaryUniversalMovementStatCombination(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, "TY-U09", "TY-U20")
	st := p.Player.Worn.At(domain.SlotWeapon)
	it := s.items[st.Item]
	it.Equip.Bonus.MoveSpeed = 100
	s.items[it.ID] = it
	p.Status.Apply(domain.StatusDef{ID: 190998, DurationSec: 60, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: 100}}}, s.tick, p.ID)
	s.refreshStats(p)
	enhanced := s.playerMoveSpeedPX(p)
	for _, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotBody, domain.SlotHead, domain.SlotRing, domain.SlotNeck, domain.SlotGlove, domain.SlotFace, domain.SlotBag} {
		v := p.Player.Worn.At(slot)
		if v.Item != 0 {
			v.Legendary = domain.LegendaryInstance{}
			p.Player.Worn.Set(slot, v)
		}
	}
	s.refreshStats(p)
	want := (universalPct(characterInnateStats(p.Player.Char).MoveSpeed, 20) + 40) * clientPixelsPerMoveSpeed
	if enhanced-s.playerMoveSpeedPX(p) != want {
		t.Fatal("base and equipment movement contributions multiplied together")
	}
}

// Settled safeguard has no equipped powers: native monster mitigation and shield
// are reachable. PvP is disabled, so this does not certify a D11 player receiver.
func TestLegendaryUniversalSettledPayloadNativeShieldBoundary(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{})
	m.Status.Apply(domain.StatusDef{ID: 190994, DurationSec: 60, DamageShield: 100, PhysicalDamageReductionPct: 50}, s.tick, m.ID)
	m.MP = 1000
	hp := m.HP
	c := legendaryContext{Scene: s, Owner: p, Target: m, Skill: 10002}
	c.Damage(m, 700, false, true)
	if hp-m.HP != 600 || m.MP != 1000 || universalShieldValue(m) != 0 {
		t.Fatal("settled payload repeated mitigation or shield", hp-m.HP, m.MP, universalShieldValue(m))
	}
}

func TestLegendaryUniversalTrapAdditiveRadiusCombination(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "TY-A21", "TY-A04")
	at := domain.Pos{MapID: p.Pos.MapID, X: 150, Y: 100}
	s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 339, Y: 100})
	outside := putMonster(s, 901, 10000, domain.Stats{}, 341, 100)
	s.monsters[outside.ID] = outside
	assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &at})
	if len(s.traps) != 1 {
		t.Fatal("trap count")
	}
	s.stepTraps()
	universalSoloDamage(t, s, m.ID, 10213, 200, 1)
	if outside.HP != 10000 || len(s.traps) != 0 {
		t.Fatal("additive190radius/one consumption")
	}
}

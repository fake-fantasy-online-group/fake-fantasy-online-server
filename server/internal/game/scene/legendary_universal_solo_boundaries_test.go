package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/rand"
	"testing"
	"time"
)

func TestLegendaryUniversalSoloDamageBoundaries(t *testing.T) {
	for _, id := range []string{"TY-A05", "TY-A08", "TY-A25", "TY-A26", "TY-A38", "TY-A40"} {
		t.Run(id+"/basic_excluded", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{})
			p.Look.AtkDist = 100
			st := p.Player.Worn.At(domain.SlotWeapon)
			it := s.items[st.Item]
			it.Equip.Bonus = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100}
			s.items[st.Item] = it
			p.Status.Apply(domain.StatusDef{ID: 10320, SourceSkillID: 10320, DurationSec: 30, DamageShield: 100}, s.tick, p.ID)
			side := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
			universalSoloDamage(t, s, m.ID, 0, 100, 1)
			if side.HP != 10000 {
				t.Fatal("active-only power added basic target")
			}
		})
	}
	for _, id := range []string{"TY-A05", "TY-A08"} {
		t.Run(id+"/existing_area", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{10022: 1})
			for i := 0; i < 7; i++ {
				putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 150+float64(i*10), 100)
			}
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10022, Target: m.ID})
			universalSoloDamage(t, s, m.ID, 10022, 155, 1)
			for i := 0; i < 7; i++ {
				universalSoloDamage(t, s, domain.EntityID(901+i), 10022, 155, 1)
			}
		})
	}
	for _, tc := range []struct {
		id    string
		race  domain.Race
		skill domain.SkillID
		want  int32
	}{{"TY-A25", domain.Warlock, 10409, 100}, {"TY-A26", domain.Warrior, 10002, 130}} {
		t.Run(tc.id+"/wrong_damage_type", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, tc.race, domain.Learned{tc.skill: 1})
			st := p.Player.Worn.At(domain.SlotWeapon)
			it := s.items[st.Item]
			it.Equip.Bonus = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100}
			s.items[st.Item] = it
			assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: m.ID})
			universalSoloDamage(t, s, m.ID, tc.skill, tc.want, 1)
		})
	}
	for _, kind := range []string{"none", "power_shield", "depleted"} {
		t.Run("TY-A40/"+kind, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-A40", domain.Warrior, domain.Learned{10002: 1})
			if kind != "none" {
				d := domain.StatusDef{ID: 10320, SourceSkillID: 10320, DurationSec: 30, DamageShield: 100}
				if kind == "power_shield" {
					d.LegendaryPowerID = "test-generated"
				}
				p.Status.Apply(d, s.tick, p.ID)
				if kind == "depleted" {
					p.Status.AbsorbDamage(100)
				}
			}
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
			universalSoloDamage(t, s, m.ID, 10002, 130, 1)
		})
	}
	t.Run("TY-A38/current_mana_independent", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A38", domain.Warrior, domain.Learned{10002: 1})
		p.MP = 8
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		universalSoloDamage(t, s, m.ID, 10002, 530, 1)
		if p.MP != 0 {
			t.Fatal("extra/refunded mana")
		}
	})
	for _, id := range []string{"TY-A34", "TY-A36"} {
		for _, single := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/single=%v", id, single), func(t *testing.T) {
				skill := domain.SkillID(10022)
				target := domain.EntityID(0)
				want := int32(155)
				if single {
					skill = 10002
					target = 900
					want = 130
				}
				s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{skill: 1})
				assassinRegressionCast(t, s, p, UseSkill{Skill: skill, Target: target})
				universalSoloDamage(t, s, m.ID, skill, want, 1)
			})
		}
	}
	t.Run("TY-A33/noncritical", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A33", domain.Warrior, domain.Learned{})
		ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true})
		if ev.Amount != 100 {
			t.Fatal("noncritical amplified")
		}
	})
}

func TestLegendaryUniversalSoloRangeAndTimingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		skill domain.SkillID
		x     float64
		want  bool
	}{{"extended", 10401, 700, true}, {"outside", 10401, 731, false}, {"no_projectile", 10002, 220, false}} {
		t.Run("TY-A07/"+tc.name, func(t *testing.T) {
			race := domain.Warlock
			if tc.skill == 10002 {
				race = domain.Warrior
			}
			s, p, m := universalSoloFixture(t, "TY-A07", race, domain.Learned{tc.skill: 1})
			s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: tc.x, Y: 100})
			before := p.MP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: tc.skill, Target: m.ID})
			if (len(s.casts) == 1) != tc.want {
				t.Fatal("actual range admission", len(s.casts), tc.want)
			}
			if tc.want {
				s.stepCastsAt(s.now().Add(time.Minute), false)
				s.stepCastsAt(s.now().Add(2*time.Minute), false)
				if m.HP == m.MaxHP {
					t.Fatal("admitted projectile no damage")
				}
			} else if before != p.MP || m.HP != m.MaxHP {
				t.Fatal("rejected cast consumed/damaged")
			}
		})
	}
	t.Run("TY-U01/instant_stays_instant", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U01", domain.Warrior, domain.Learned{10002: 1})
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: m.ID})
		s.stepCastsAt(s.now(), false)
		released := false
		for _, o := range s.outbox {
			if e, ok := o.ev.(event.SkillCastChanged); ok && e.Phase == event.SkillCastRelease {
				released = true
			}
		}
		if !released {
			t.Fatal("instant cast gained windup")
		}
		s.stepCastsAt(s.now().Add(time.Second), false)
		universalSoloDamage(t, s, m.ID, 10002, 130, 1)
	})
	t.Run("TY-U15/other_cooldown_unchanged", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U15", domain.Warrior, domain.Learned{10002: 1, 10005: 1})
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10005, Target: m.ID})
		before := p.CapturePlayerRuntime(s.tick).SkillReadyIn[10005]
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		if p.CapturePlayerRuntime(s.tick).SkillReadyIn[10005] != before {
			t.Fatal("other cooldown refunded")
		}
	})
}

func TestLegendaryUniversalSoloRecoveryBoundaries(t *testing.T) {
	t.Run("TY-U06/cap", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U06", domain.Healer, domain.Learned{10302: 1})
		a := nativeRepairAlly(t, s, p, 2, 140)
		a.MP = 950
		a.MaxMP = 1000
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10302, Target: a.ID})
		if a.MP != 1000 {
			t.Fatal("restore cap", a.MP)
		}
	})
	t.Run("TY-U16/no_hot_amplification", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U16", domain.Healer, domain.Learned{})
		d := domain.StatusDef{ID: 10303, SourceSkillID: 10303, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: 100}}}
		s.applyStatusDef(p, d, p.ID)
		a := universalSoloActive(t, p, 10303)
		p.HP = 1
		s.tickStatus(p, a)
		if p.HP != 101 {
			t.Fatal("direct-heal power amplified HoT")
		}
	})
	t.Run("TY-D12/no_generated_heal_amplification", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D12", domain.Healer, domain.Learned{})
		p.HP = 1
		s.legendaryState().depth = 1
		s.applySkill(m, p, domain.SkillDef{ID: 10301, Effects: []domain.SkillEffect{{Kind: domain.EffectHeal, HealFlat: 100}}}, 0, false)
		s.legendaryState().depth = 0
		if p.HP != 101 {
			t.Fatal("generated heal amplified", p.HP)
		}
	})
	t.Run("TY-U17/full_cap_no_auto_revive", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U17", domain.Healer, domain.Learned{10309: 5})
		a := nativeRepairAlly(t, s, p, 2, 140)
		a.HP = 0
		s.stepStatus()
		if a.HP != 0 {
			t.Fatal("automatic revive")
		}
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10309, Target: a.ID})
		if a.HP != a.MaxHP {
			t.Fatal("full revive cap", a.HP, a.MaxHP)
		}
	})
	t.Run("TY-A17/native_lifesteal", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A17", domain.Swordsman, domain.Learned{})
		d := domain.StatusDef{ID: 1018, SourceSkillID: 10104, DurationSec: 60, LifeStealPct: 10, LifeStealChance: 100}
		if !s.applyStatusDef(p, d, p.ID) {
			t.Fatal("leech status")
		}
		p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
		p.HP = 1000
		p.Look.AtkDist = 100
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
		if p.HP != 1016 {
			t.Fatal("native leech consumer", p.HP)
		}
	})
	t.Run("TY-U18/guaranteed_cleanse", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U18", domain.Healer, domain.Learned{10309: 1})
		a := nativeRepairAlly(t, s, p, 2, 140)
		a.Status.Apply(domain.StatusDef{ID: domain.StatusPoison, DurationSec: 60, TypeKnown: true, Type: domain.StatusBad}, s.tick, 900)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10309, Target: a.ID})
		if a.Status.Has(domain.StatusPoison) {
			t.Fatal("native guaranteed cleanse failed")
		}
	})
}

func TestLegendaryUniversalSoloStateBoundary(t *testing.T) {
	t.Run("TY-D17/control_ended", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D17", domain.Warrior, domain.Learned{})
		p.Stats = domain.Stats{}
		m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
		p.Status.Apply(domain.StatusDef{ID: 1034, DurationSec: 1, BlockMove: true}, s.tick, m.ID)
		p.Status.Remove(1034)
		before := p.HP
		s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true})
		if before-p.HP != 1000 {
			t.Fatal("ended control reduction persisted")
		}
	})
	for _, id := range []string{"TY-D11", "TY-D19"} {
		t.Run(id+"/source_gone_and_guarded", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{})
			p.MP = 1000
			delete(s.entities, m.ID)
			delete(s.monsters, m.ID)
			d := domain.StatusDef{ID: domain.StatusPoison, LegendaryPowerID: "source-payload", HPChanges: []domain.ResourceChange{{Value: -1000}}, TypeKnown: true, Type: domain.StatusBad}
			before := p.HP
			s.legendaryState().depth = 1
			s.tickStatus(p, domain.Active{Source: m.ID, Def: d})
			s.legendaryState().depth = 0
			want := int32(750)
			if id == "TY-D11" {
				want = 700
			}
			if before-p.HP != want {
				t.Fatal("source/guard bypass", before-p.HP, want)
			}
		})
	}
	t.Run("TY-A14/slow_floor", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-A14", domain.Healer, domain.Learned{})
		join(t, s, 2, 102, "slow receiver", 130, 100)
		a := s.players[2]
		base := a.Stats.MoveSpeed
		d := domain.StatusDef{ID: 190989, SourceSkillID: 10310, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: -80}}}
		if !s.applyStatusDef(a, d, p.ID) {
			t.Fatal("slow rejected")
		}
		if a.Stats.MoveSpeed != max(int32(1), base/100) {
			t.Fatal("slow floor", a.Stats.MoveSpeed)
		}
	})
}

// Deliberately controlled pre-existing discount input exercises rounding/cap at
// real onUseSkill payment. It does not claim an authored one-MP skill or a new
// equipment build exists; the solo main tests above use unmodified native data.
func TestLegendaryUniversalSoloPaymentBoundary(t *testing.T) {
	for _, tc := range []struct {
		id                    string
		cost, reduction, want int32
	}{{"TY-U03", 1, 0, 1}, {"TY-U03", 1, 7000, 0}, {"TY-U13", 1, 0, 1}, {"TY-U13", 1, 8000, 0}, {"TY-A24", 8, 15000, 0}} {
		t.Run(fmt.Sprintf("%s/cost=%d/preexisting=%d", tc.id, tc.cost, tc.reduction), func(t *testing.T) {
			skill := domain.SkillID(10002)
			race := domain.Warrior
			if tc.id == "TY-U13" {
				skill = 10320
				race = domain.Healer
			}
			s, p, m := universalSoloFixture(t, tc.id, race, domain.Learned{skill: 1})
			s.refreshStats(p)
			p.MP = p.MaxMP
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
			d, _ := s.skills.Get(skill, 1)
			d.FormulaVersion = 0
			d.Progression = nil
			d.MPCost = tc.cost
			d.LegendaryModifiers.ManaReductionBP = tc.reduction
			s.skills[domain.SkillKey{ID: skill, Level: 1}] = d
			target := m.ID
			if tc.id == "TY-U13" {
				target = p.ID
			}
			before := p.MP
			assassinRegressionCast(t, s, p, UseSkill{Skill: skill, Target: target})
			if before-p.MP != tc.want {
				t.Fatal("payment boundary", before-p.MP, tc.want)
			}
			if tc.id == "TY-A24" {
				damage, n := assassinRegressionDamage(s, m.ID, int32(skill))
				if n != 1 || damage != 390 {
					t.Fatal("capped free cast damage", damage, n)
				}
			}
		})
	}
}

func TestLegendaryUniversalSoloAdditionalBoundaries(t *testing.T) {
	t.Run("TY-A12/hit_unchanged", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A12", domain.Warrior, domain.Learned{})
		p.Stats.Hit = 0
		m.Stats.Def = 10000
		hp := m.HP
		ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 10002})
		if !ev.Flag.Has(event.DamageMiss) || m.HP != hp || m.Stats.Def != 10000 {
			t.Fatal("fixed-resistance penetration changed hit/defense")
		}
	})
	t.Run("TY-A15/immunity", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A15", domain.Warrior, domain.Learned{})
		m.Status.Apply(domain.StatusDef{ID: 190995, DurationSec: 60, BlockHarmful: true}, s.tick, m.ID)
		d := domain.StatusDef{ID: 1034, SourceSkillID: 10326, DurationSec: 10, BlockMove: true, ChanceBP: 10000, ChanceExplicit: true, TypeKnown: true, Type: domain.StatusBad}
		if s.applyStatusDef(m, d, p.ID) || !canMove(m) {
			t.Fatal("effect accuracy bypassed immunity")
		}
	})
	t.Run("TY-U08/control_not_shortened", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U08", domain.Warrior, domain.Learned{})
		d := domain.StatusDef{ID: 1034, SourceSkillID: 10326, DurationSec: 10, BlockMove: true, TypeKnown: true, Type: domain.StatusBad}
		if !s.applyStatusDef(p, d, m.ID) {
			t.Fatal("root missing")
		}
		a := universalSoloActive(t, p, 1034)
		if a.ExpireAt-s.tick != domain.Ticks(10000) || canMove(p) {
			t.Fatal("slow resistance shortened root")
		}
	})
	t.Run("TY-A20/obstacle_stops_pull", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A20", domain.Warlock, domain.Learned{10432: 1})
		s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 410, Y: 100})
		s.walkable = func(pos domain.Pos) bool { return pos.X <= 320 || pos.X >= 350 }
		s.rng = rand.New(rand.NewSource(universalSoloSuccessSeed(6000)))
		to := domain.Pos{MapID: p.Pos.MapID, X: 300, Y: 100}
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10432, At: &to})
		if p.Pos != to || m.Pos.X != 350 {
			t.Fatal("pull crossed obstacle", p.Pos, m.Pos)
		}
	})
	t.Run("TY-U07/outside_and_invalid_target", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U07", domain.Assassin, domain.Learned{10227: 1})
		s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 521, Y: 100})
		before := p.MP
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10227, Target: m.ID})
		if len(s.casts) != 0 || p.MP != before {
			t.Fatal("jump admitted beyond420")
		}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10227, Target: p.ID})
		if len(s.casts) != 0 {
			t.Fatal("jump self target accepted")
		}
	})
	t.Run("TY-D16/nonabsorbing_shield", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D16", domain.Warlock, domain.Learned{})
		d := domain.StatusDef{ID: 10425, SourceSkillID: 10425, DurationSec: 10, PhysicalDamageReductionPct: 20, PhysicalReflectPct: 30}
		if !s.applyStatusDef(p, d, p.ID) {
			t.Fatal("native defense")
		}
		p.HP = p.MaxHP
		m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
		hp := p.HP
		ev, raw, ref := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
		s.afterStrikeStatuses(m, p, ev, raw, ref)
		if hp-p.HP != 80 || m.MaxHP-m.HP != 24 || universalShieldValue(p) != 0 {
			t.Fatal("nonabsorbing shield was amplified")
		}
	})
}

// A controlled, already present same-kind bucket is the input to the real
// native target-selection/settlement consumer. It is not another legendary.
func TestLegendaryUniversalSoloAdditiveGeometry(t *testing.T) {
	for _, tc := range []struct {
		id           string
		skill        domain.SkillID
		x, y, ox, oy float64
	}{
		{"TY-A02", 10005, 410, 100, 416, 100}, {"TY-A03", 10006, 390, 100, 401, 100},
		{"TY-A04", 10022, 380, 100, 386, 100}, {"TY-A06", 10005, 200, 150, 200, 153},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{tc.skill: 1})
			d, _ := s.skills.Get(tc.skill, 1)
			switch tc.id {
			case "TY-A02":
				d.LegendaryModifiers.LengthBP = 5000
			case "TY-A03", "TY-A04":
				d.LegendaryModifiers.RadiusBP = 5000
			case "TY-A06":
				d.LegendaryModifiers.WidthBP = 5000
			}
			s.skills[domain.SkillKey{ID: tc.skill, Level: 1}] = d
			yes := putMonster(s, 901, 10000, domain.Stats{}, tc.x, tc.y)
			no := putMonster(s, 902, 10000, domain.Stats{}, tc.ox, tc.oy)
			assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: m.ID})
			if yes.HP == 10000 || no.HP != 10000 {
				t.Fatal("geometry was multiplied rather than added", yes.HP, no.HP)
			}
		})
	}
	t.Run("TY-A04/cone_cap", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A04", domain.Warrior, domain.Learned{10006: 1})
		d, _ := s.skills.Get(10006, 1)
		d.Area.Angle = 300
		d.FormulaVersion = 0
		d.Progression = nil
		s.skills[domain.SkillKey{ID: 10006, Level: 1}] = d
		behind := putMonster(s, 901, 10000, domain.Stats{}, 20, 100)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10006, Target: m.ID})
		universalSoloDamage(t, s, behind.ID, 10006, 130, 1)
	})
	for _, id := range []string{"TY-A34", "TY-A36"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{10022: 1})
			d, _ := s.skills.Get(10022, 1)
			d.LegendaryModifiers.DamageBP = 2000
			s.skills[domain.SkillKey{ID: 10022, Level: 1}] = d
			side := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10022, Target: m.ID})
			main, other := int32(310), int32(186)
			if id == "TY-A36" {
				main, other = 186, 248
			}
			universalSoloDamage(t, s, m.ID, 10022, main, 1)
			universalSoloDamage(t, s, side.ID, 10022, other, 1)
		})
	}
	t.Run("TY-U10", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U10", domain.Healer, domain.Learned{10304: 1})
		d, _ := s.skills.Get(10304, 1)
		d.LegendaryModifiers.RadiusBP = 2000
		s.skills[domain.SkillKey{ID: 10304, Level: 1}] = d
		a := nativeRepairAlly(t, s, p, 2, 570)
		b := nativeRepairAlly(t, s, p, 3, 581)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10304})
		if a.HP != 231 || b.HP != 1 {
			t.Fatal("additive healing radius", a.HP, b.HP)
		}
	})
	t.Run("TY-U15", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U15", domain.Warrior, domain.Learned{10002: 1})
		d, _ := s.skills.Get(10002, 1)
		d.LegendaryModifiers.CooldownReductionBP = 3000
		s.skills[domain.SkillKey{ID: 10002, Level: 1}] = d
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		if p.CapturePlayerRuntime(s.tick).SkillReadyIn[10002] != domain.Ticks(500) {
			t.Fatal("additive cooldown deadline")
		}
	})
}

func TestLegendaryUniversalSoloFinalBoundaries(t *testing.T) {
	t.Run("TY-U02/extra_payload", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U02", domain.Warrior, domain.Learned{})
		p.MP = 0
		c := legendaryContext{Scene: s, Owner: p, Target: m}
		hp := m.HP
		c.Damage(m, 100, false, false)
		if hp-m.HP != 100 || p.MP != 0 {
			t.Fatal("extra attack restored basic mana", hp-m.HP, p.MP)
		}
	})
	t.Run("TY-A10/minimum_interval", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A10", domain.Warrior, domain.Learned{})
		p.Status.Apply(domain.StatusDef{ID: 190996, SourceSkillID: 10311, DurationSec: 60, Affixes: []domain.Affix{{Attr: domain.AttrAtkSpeed, Mode: domain.ModePercent, Value: -100000}}}, s.tick, p.ID)
		s.refreshStats(p)
		p.Look.AtkDist = 100
		universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
		if p.CapturePlayerRuntime(s.tick).AttackReadyIn != 1 {
			t.Fatal("native minimum interval lost", p.CapturePlayerRuntime(s.tick).AttackReadyIn, p.Stats)
		}
		s.onAttack(Attack{ID: p.ID, Target: m.ID})
		if len(s.attacks) != 0 {
			t.Fatal("extra attack within minimum interval")
		}
	})
	t.Run("TY-U05/direct_heal_unchanged", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U05", domain.Healer, domain.Learned{10301: 1})
		a := nativeRepairAlly(t, s, p, 2, 140)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10301, Target: a.ID})
		if a.HP != 201 {
			t.Fatal("HoT power altered direct healing", a.HP)
		}
	})
	t.Run("TY-A04/rectangle_unchanged", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A04", domain.Warrior, domain.Learned{10005: 1})
		side := putMonster(s, 901, 10000, domain.Stats{}, 200, 130)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10005, Target: m.ID})
		universalSoloDamage(t, s, m.ID, 10005, 95, 1)
		if side.HP != 10000 {
			t.Fatal("circle/cone power altered rectangle")
		}
	})
	t.Run("TY-A20/failed_movement_no_pull", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A20", domain.Warlock, domain.Learned{10432: 1})
		s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 410, Y: 100})
		from, pos := p.Pos, m.Pos
		s.rng = rand.New(rand.NewSource(1))
		to := domain.Pos{MapID: p.Pos.MapID, X: 300, Y: 100}
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10432, At: &to})
		if p.Pos != from || m.Pos != pos || m.HP != m.MaxHP {
			t.Fatal("failed native teleport pulled/damaged")
		}
	})
	t.Run("TY-U07/target_jump", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U07", domain.Assassin, domain.Learned{10227: 1})
		s.refreshStats(p)
		p.MP = p.MaxMP
		s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 500, Y: 100})
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10227, Target: m.ID})
		if p.Pos != m.Pos {
			t.Fatal("extended target distance not used", p.Pos, m.Pos)
		}
	})
	t.Run("TY-U19/no_new_detection", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U19", domain.Healer, domain.Learned{10301: 1})
		a := nativeRepairAlly(t, s, p, 2, 140)
		join(t, s, 3, 103, "hidden enemy", 160, 100)
		e := s.players[3]
		e.Status.Apply(domain.StatusDef{ID: 1017, DurationSec: 60, Invisible: true}, s.tick, e.ID)
		s.placeTrap(m, domain.SkillDef{ID: 10213}, domain.Pos{MapID: p.Pos.MapID, X: 170, Y: 100}, 50, 100)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10301, Target: a.ID})
		if !entityInvisible(e) {
			t.Fatal("heal acquired reveal")
		}
		for _, tr := range s.traps {
			if tr.Trap.VisibleToPlayer(p.ID) {
				t.Fatal("heal acquired detection")
			}
		}
	})
}

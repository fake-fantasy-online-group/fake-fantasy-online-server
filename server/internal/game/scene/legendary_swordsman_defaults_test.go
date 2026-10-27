package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
)

func TestLegendarySwordsmanAllFiftyCurrent(t *testing.T) {
	n := 0
	for _, p := range domain.LegendaryPowers() {
		if p.Class != "剑客" {
			continue
		}
		n++
		if !legendaryHandlerCurrent(p.ID) {
			t.Error("missing current handler", p.ID)
		}
	}
	if n != 50 {
		t.Fatal(n)
	}
}
func TestLegendarySwordsmanImmediateAttackDefaults(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		want  int32
	}{
		{"JK-A01", 10102, 200}, {"JK-A03", 10102, 260}, {"JK-A19", 10122, 125}, {"JK-A21", 10116, 420}, {"JK-A26", 10104, 200}, {"JK-A27", 10102, 420}, {"JK-A28", 10119, 360}, {"JK-A30", 10123, 210},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			swordCast(t, s, p, m, domain.SkillDef{ID: tc.skill, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
			if got := 100000 - m.HP; got != tc.want {
				t.Fatal("damage", got, "want", tc.want)
			}
			// Repeated native casts do not wait for a timer or retain marks.
			before := m.HP
			swordCast(t, s, p, m, domain.SkillDef{ID: tc.skill, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
			if before-m.HP != tc.want {
				t.Fatal("second cast blocked by old timer")
			}
		})
	}
}
func TestLegendarySwordsmanActualNativeResourceConversion(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A12")
	p.MaxMP = 1000
	p.MP = 990
	swordCast(t, s, p, m, swordDualDef(10119))
	if p.MP != 1000 || m.HP != 100000-150-28 {
		t.Fatal("overflow/extra damage counted as native mana", p.MP, m.HP)
	}
	s, p, m = legendaryMeleeFixture(t, "JK-U07")
	p.MaxMP = 1000
	p.MP = 0
	swordEquip(t, s, p, "JK-A19", domain.SlotBody)
	putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
	swordCast(t, s, p, m, domain.SkillDef{ID: 10102, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
	if p.MP != 120 {
		t.Fatal("native cast mana generation counted targets or capped at zero paid", p.MP)
	}
}
func TestLegendarySwordsmanNativeRootAndTauntExtraDamage(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A06")
	st := domain.StatusDef{ID: 800111, Level: 1, DurationSec: 6, BlockMove: true, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
	s.statuses = statusTable(st)
	d := domain.SkillDef{ID: 10111, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}, Statuses: []domain.StatusApplication{{ID: st.ID, Level: 1}}}
	swordCast(t, s, p, m, d)
	if m.HP != 99700 {
		t.Fatal("root-native proc", m.HP)
	}
	s, p, m = legendaryMeleeFixture(t, "JK-A25")
	swordCast(t, s, p, m, domain.SkillDef{ID: 10127, Effects: []domain.SkillEffect{{Kind: domain.EffectTaunt, TauntPower: 100}}})
	if m.HP != 99840 || m.Monster.Target != p.ID {
		t.Fatal("taunt not native success", m.HP, m.Monster.Target)
	}
}
func TestLegendarySwordsmanRootArrayOnlyOnce(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A27")
	q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
	finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	s.equipmentState().root.def = domain.SkillDef{ID: 10121}
	for _, dst := range []*entity.Entity{m, q} {
		s.resolveStrikeStatuses(p, dst, &combat.HitTracker{}, combat.Blow{SkillID: 10121, SkillPct: 100, GuaranteedHit: true})
	}
	finish()
	if m.HP != 99580 || q.HP != 9580 {
		t.Fatal("AoE multiplied per native hit", m.HP, q.HP)
	}
}
func TestLegendarySwordsmanPermanentShieldUnequip(t *testing.T) {
	for _, id := range []string{"JK-D01", "JK-D03", "JK-D08"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, id)
			if id == "JK-D03" {
				p.HP = p.MaxHP
				swordCast(t, s, p, m, swordDualDef(10104))
			} else {
				skill := domain.SkillID(10102)
				if id == "JK-D08" {
					skill = 10116
				}
				swordCast(t, s, p, m, domain.SkillDef{ID: skill, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
			}
			found := false
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.LegendaryPowerID == id && a.ShieldLeft > 0 && a.ExpireAt == 0 {
					found = true
				}
			})
			if !found {
				t.Fatal("permanent native shield absent")
			}
			p.Player.Worn = &domain.EquipSet{}
			s.pruneLegendaryOwner(p)
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.LegendaryPowerID == id {
					t.Fatal("shield survived source unequip")
				}
			})
		})
	}
}
func TestLegendarySwordsmanImmediateDefenseDefaults(t *testing.T) {
	for _, tc := range []struct {
		id       string
		distance float64
		damage   int32
		want     int32
	}{
		{"JK-D04", 30, 100, 50}, {"JK-D07", 30, 100, 100}, {"JK-D09", 30, 800, 440}, {"JK-D09", 30, 799, 799}, {"JK-D10", 76, 100, 65}, {"JK-D10", 75, 100, 100},
	} {
		s, p, m := legendaryMeleeFixture(t, tc.id)
		m.Pos.X = p.Pos.X + tc.distance
		m.Monster.Target = p.ID
		m.Monster.TauntPower = 1
		m.Status = domain.NewStatusSet()
		m.Status.Apply(domain.StatusDef{ID: domain.StatusSilence, Level: 1, DurationSec: 6, Control: domain.ControlSilence}, s.tick, p.ID)
		c := legendaryContext{Scene: s, Owner: p, Target: m}
		if got := legendaryHandlers[tc.id].BeforeIncoming(&c, tc.damage); got != tc.want {
			t.Error(tc.id, got, tc.want)
		}
	}
	s, p, _ := legendaryMeleeFixture(t, "JK-D02")
	if got := s.legendaryMeleeDotDamage(p, 100); got != 45 {
		t.Fatal("permanent periodic mitigation", got)
	}
}
func TestLegendarySwordsmanImmediateUtilityDefaults(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-U02")
	p.Status = domain.NewStatusSet()
	for _, st := range []domain.StatusDef{{ID: domain.StatusPoison, DurationSec: 10}, {ID: domain.StatusSlow, DurationSec: 10}, {ID: 800111, BlockMove: true, DurationSec: 10}} {
		p.Status.Apply(st, s.tick, m.ID)
	}
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10104}, m.ID, []domain.EntityID{m.ID})
	if p.Status.Has(domain.StatusPoison) || p.Status.Has(domain.StatusSlow) || !p.Status.Has(800111) {
		t.Fatal("cleanse missed poison/slow or removed root")
	}
	s, p, m = legendaryMeleeFixture(t, "JK-U08")
	slow := domain.StatusDef{ID: domain.StatusSlow, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -10}}}
	if s.legendaryStatusReceiving(p, m, &slow) {
		t.Fatal("equipment movement immunity missing")
	}
	p.Player.Worn = &domain.EquipSet{}
	slow.Affixes = []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -10}}
	if !s.legendaryStatusReceiving(p, m, &slow) {
		t.Fatal("immunity survived unequip")
	}
	for _, tc := range []struct {
		id           string
		skill        domain.SkillID
		dist, radius int32
	}{{"JK-U04", 10127, 100, 180}, {"JK-U10", 10113, 145, 100}, {"JK-A17", 10121, 100, 150}} {
		s, p, m = legendaryMeleeFixture(t, tc.id)
		d := domain.SkillDef{ID: tc.skill, Dist: 100, Radius: 100}
		s.legendaryPrepareSkill(p, &d, m.ID)
		if d.Dist != tc.dist || d.Radius != tc.radius {
			t.Error(tc.id, d.Dist, d.Radius)
		}
	}
}
func TestLegendarySwordsmanExplicitKeeps(t *testing.T) {
	for _, tc := range []struct {
		id            string
		skill         domain.SkillID
		primary, side int32
		far           bool
	}{
		{"JK-A02", 10117, 115, 115, true}, {"JK-A04", 10117, 175, 75, true}, {"JK-A07", 10103, 200, 0, false}, {"JK-A08", 10103, 100, 130, false}, {"JK-A22", 10123, 190, 170, false}, {"JK-A23", 10123, 100, 110, false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			if tc.far {
				m.Pos.X = 190
				s.aoi.Move(m, m.Pos)
			}
			q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			if tc.id == "JK-A23" {
				m.HP = 50000
			}
			before := m.HP
			swordCast(t, s, p, m, domain.SkillDef{ID: tc.skill, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
			if before-m.HP != tc.primary || 10000-q.HP != tc.side {
				t.Fatal("kept contract", before-m.HP, 10000-q.HP, tc)
			}
		})
	}
}
func TestLegendarySwordsmanRefundCapsAndDefensiveCounter(t *testing.T) {
	for _, id := range []string{"JK-U03", "JK-U09"} {
		s, p, m := legendaryMeleeFixture(t, id)
		p.MaxMP = 1000
		p.MP = 0
		skill := domain.SkillID(10119)
		if id == "JK-U06" {
			skill = 10111
		}
		finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
		r := s.equipmentState().root
		r.def = domain.SkillDef{ID: skill}
		r.paidMP = 100
		s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: int32(skill), SkillPct: 100, GuaranteedHit: true})
		finish()
		want := int32(60)
		if id == "JK-U09" {
			want = 80
		}
		if p.MP != want {
			t.Error(id, p.MP, want)
		}
	}
	s, p, m := legendaryMeleeFixture(t, "JK-A29")
	m.Stats.MinAtk = 100
	m.Stats.MaxAtk = 100
	s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true})
	if m.HP != 99820 {
		t.Fatal("instant counter missing", m.HP)
	}
	before := m.HP
	s.legendaryState().depth = 1
	s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true, Origin: domain.DamageProcOrigin, ProcDepth: 1})
	s.legendaryState().depth = 0
	if m.HP != before {
		t.Fatal("proc counter recursively fired")
	}
}
func TestLegendarySwordsmanAllyShieldPrunedWithSource(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-D08")
	join(t, s, 2, 200, "ally", 120, 100)
	q := s.players[2]
	p.Player.Party = 1
	q.Player.Party = 1
	q.HP = 1
	swordCast(t, s, p, m, domain.SkillDef{ID: 10116, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
	found := false
	q.Status.EachEffective(func(a domain.Active) {
		if a.Def.LegendaryPowerID == "JK-D08" && a.Source == p.ID && a.ShieldLeft > 0 {
			found = true
		}
	})
	if !found {
		t.Fatal("ally shield absent")
	}
	p.Player.Worn = &domain.EquipSet{}
	s.pruneLegendaryOwner(p)
	q.Status.EachEffective(func(a domain.Active) {
		if a.Def.LegendaryPowerID == "JK-D08" {
			t.Fatal("ally shield survived source unequip")
		}
	})
}
func TestLegendarySwordsmanLionSplashLimitAndNativeCounterShield(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A20")
	for i := 0; i < 7; i++ {
		putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
	}
	swordCast(t, s, p, m, domain.SkillDef{ID: 10116, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
	for i := 0; i < 7; i++ {
		want := int32(9840)
		if got := s.entities[domain.EntityID(901+i)].HP; got != want {
			t.Fatal("splash every in-range enemy", i, got, want)
		}
	}
	s, p, m = legendaryMeleeFixture(t, "JK-A29")
	m.Stats.MinAtk = 100
	m.Stats.MaxAtk = 100
	p.Status = domain.NewStatusSet()
	p.Status.Apply(domain.StatusDef{ID: 900001, DurationSec: 10, DamageShield: 1000}, s.tick, p.ID)
	before := p.HP
	s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true})
	if p.HP != before || m.HP != 99820 {
		t.Fatal("shield absorbed hit did not counter", p.HP, m.HP)
	}
}
func TestLegendarySwordsmanMovePassiveAndCleanseScope(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-U01")
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10117}, 0, nil)
	stats := domain.Stats{MoveSpeed: 100}
	s.legendaryPassiveStats(p, &stats)
	if stats.MoveSpeed != 180 {
		t.Fatal("passive move", stats.MoveSpeed)
	}
	s, p, m = legendaryMeleeFixture(t, "JK-U05")
	p.Status = domain.NewStatusSet()
	p.Status.Apply(domain.StatusDef{ID: domain.StatusSlow, DurationSec: 10}, s.tick, m.ID)
	p.Status.Apply(domain.StatusDef{ID: domain.StatusStun, DurationSec: 10, Control: domain.ControlStun}, s.tick, m.ID)
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10103}, m.ID, []domain.EntityID{m.ID})
	if p.Status.Has(domain.StatusSlow) || !p.Status.Has(domain.StatusStun) {
		t.Fatal("phantom cleanse scope")
	}
	s, p, m = legendaryMeleeFixture(t, "JK-U08")
	if !s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 6, Control: domain.ControlStun, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}, m.ID) {
		t.Fatal("movement immunity became stun immunity")
	}
}

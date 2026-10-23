package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func TestLegendaryWarriorDirectDamageContracts(t *testing.T) {
	for _, tc := range []struct {
		id     string
		skill  domain.SkillID
		damage int32
	}{{"ZS-A01", 10002, 180}, {"ZS-A04", 10005, 115}, {"ZS-A05", 10005, 160}, {"ZS-A06", 10005, 210}, {"ZS-A07", 10005, 150}, {"ZS-A09", 10013, 125}, {"ZS-A11", 10017, 160}, {"ZS-A12", 10017, 110}, {"ZS-A13", 10006, 145}, {"ZS-A15", 10006, 160}, {"ZS-A16", 10003, 115}, {"ZS-A17", 10003, 130}, {"ZS-A18", 10021, 190}, {"ZS-A19", 10021, 170}, {"ZS-A20", 10015, 340}, {"ZS-A21", 10015, 200}, {"ZS-A22", 10022, 120}, {"ZS-A23", 10022, 340}, {"ZS-A24", 10023, 185}, {"ZS-A25", 10023, 135}, {"ZS-A27", 10027, 180}, {"ZS-A28", 10005, 140}, {"ZS-A29", 10005, 280}, {"ZS-A30", 10002, 165}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			p.Player.Char.Skills = domain.Learned{tc.skill: 1}
			d := domain.SkillDef{ID: tc.skill, Level: 1, TargetEnemy: true, Kind: domain.SkillSingle, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
			before := m.HP
			warriorTestCast(t, s, p, m, d)
			if got := before - m.HP; got != tc.damage {
				t.Fatalf("damage%d want%d", got, tc.damage)
			}
		})
	}
}
func TestLegendaryWarriorIncomingContracts(t *testing.T) {
	for _, tc := range []struct {
		id     string
		damage int32
		magic  bool
	}{{"ZS-D02", 84, false}, {"ZS-D04", 40, true}, {"ZS-D05", 60, false}, {"ZS-D06", 65, false}, {"ZS-D07", 55, false}, {"ZS-D08", 70, false}, {"ZS-D10", 75, false}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100}
			ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true, Magic: tc.magic})
			if ev.Amount != tc.damage {
				t.Fatalf("incoming%d want%d", ev.Amount, tc.damage)
			}
		})
	}
}
func TestLegendaryWarriorResourceAndGeometryContracts(t *testing.T) {
	for _, tc := range []struct {
		id                                  string
		skill                               domain.SkillID
		mana, radius, width, dist, prep, cd int32
	}{{"ZS-U01", 10002, 60, 100, 100, 100, 1000, 1000}, {"ZS-U02", 10013, 40, 100, 100, 100, 1000, 1000}, {"ZS-U03", 10017, 100, 100, 100, 150, 1000, 1000}, {"ZS-U04", 10028, 70, 170, 100, 100, 1000, 1000}, {"ZS-U05", 10006, 55, 145, 100, 100, 1000, 1000}, {"ZS-U06", 10005, 40, 100, 100, 100, 1000, 1000}, {"ZS-U08", 10005, 70, 100, 170, 100, 1000, 1000}, {"ZS-U09", 10017, 100, 100, 100, 100, 500, 500}, {"ZS-U10", 10028, 0, 125, 100, 100, 1000, 1000}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			area := domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
			if tc.id == "ZS-U08" {
				area = domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Width: 100, Length: 100}
			}
			d := domain.SkillDef{ID: tc.skill, Level: 1, Kind: domain.SkillArea, RequiredWeaponType: 1, MPCost: 100, Dist: 100, PrepareMS: 1000, CooldownMS: 1000, Area: area, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			s.legendaryPrepareSkill(p, &d, m.ID)
			if d.MPCost != tc.mana || d.Dist != tc.dist || d.PrepareMS != tc.prep || d.CooldownMS != tc.cd {
				t.Fatalf("native costs/range %+v", d)
			}
			if area.Shape == domain.SkillAreaCircle && d.Area.Radius != tc.radius {
				t.Fatalf("radius%d", d.Area.Radius)
			}
			if area.Shape == domain.SkillAreaRectangle && (d.Area.Width != tc.width || d.Area.Length != 170) {
				t.Fatalf("rectangle %+v", d.Area)
			}
		})
	}
}
func TestLegendaryWarriorShieldRefillsAndSlowImmunity(t *testing.T) {
	for _, id := range []string{"ZS-D01", "ZS-D09"} {
		t.Run(id, func(t *testing.T) {
			s, p, _ := legendaryMeleeFixture(t, id)
			c := legendaryContext{Scene: s, Owner: p, SourceUID: 991, Power: domain.LegendaryPowerDefinition{ID: id}}
			warriorShield(&c)
			p.Status.AbsorbDamage(1000)
			warriorShield(&c)
			var shield int32
			p.Status.EachEffective(func(a domain.Active) {
				shield += a.ShieldLeft
				if a.ExpireAt != 0 {
					t.Fatal("permanent shield expired")
				}
			})
			if shield != legendaryMeleeScale(p.MaxHP, 25) {
				t.Fatalf("shield%d", shield)
			}
			p.Player.Worn = &domain.EquipSet{}
			s.refreshStats(p)
			if p.Status != nil {
				p.Status.EachEffective(func(a domain.Active) {
					if a.Def.LegendaryPowerID == id {
						t.Fatal("shield survived removing source gear")
					}
				})
			}
		})
	}
	s, p, m := legendaryMeleeFixture(t, "ZS-D10")
	if s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusSlow, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50, Mode: domain.ModePercent}}}, m.ID) {
		t.Fatal("slow accepted")
	}
}
func TestLegendaryWarriorChainAndBasicCoefficient(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	s.skills = skillTable(domain.SkillDef{ID: 10002, Level: 1, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 150}})
	before := m.HP
	s.onAttack(Attack{ID: p.ID, Target: m.ID})
	s.stepCombat()
	if before-m.HP != 330 {
		t.Fatalf("basic higher coefficient%d", before-m.HP)
	}
	d := domain.SkillDef{ID: 10002, Level: 1, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 50, GuaranteedHit: true}}
	before = m.HP
	warriorTestCast(t, s, p, m, d)
	if before-m.HP != 150 {
		t.Fatalf("chain higher coefficient%d", before-m.HP)
	}
}
func TestLegendaryWarriorQuakeOverlapAndLifetime(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A14")
	c := legendaryContext{Scene: s, Owner: p, SourceUID: 991, Power: domain.LegendaryPowerDefinition{ID: "ZS-A14"}, Skill: 10006, Center: p.Pos, Def: domain.SkillDef{ID: 10006, Area: domain.SkillAreaTargeting{Radius: 100}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}}
	warriorQuakeZone(&c)
	warriorQuakeZone(&c)
	before := m.HP
	for _, ms := range []int64{1000, 2000, 3000, 4000} {
		c.NowMS = ms
		warriorQuakeTick(&c)
	}
	if before-m.HP != 105 {
		t.Fatalf("overlapping quake damage%d want105", before-m.HP)
	}
	c.NowMS = 5000
	c.Owner.Pos.MapID++
	before = m.HP
	warriorQuakeTick(&c)
	if m.HP != before {
		t.Fatal("old-map zone carried")
	}
}
func TestLegendaryWarriorPetrifyImmediateControlAndImmunity(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A26")
	s.statuses = statusTable(domain.StatusDef{ID: domain.StatusPetrify, Level: 1, DurationSec: 5, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun, BreakOnDamage: true})
	d := domain.SkillDef{ID: 10016, Level: 1, HurtType: domain.HurtPhysical, CooldownMS: 1000, Statuses: []domain.StatusApplication{{ID: domain.StatusPetrify, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}}
	before := m.HP
	warriorTestCast(t, s, p, m, d)
	if before-m.HP != 240 {
		t.Fatalf("immediate%d", before-m.HP)
	}
	if m.Status == nil || !m.Status.Has(domain.StatusPetrify) {
		t.Fatal("new petrify broken")
	}
	immune := putMonster(s, 902, 10000, domain.Stats{}, 140, 100)
	s.defs = spawn.MapDefs{338: {ID: 338, ImmuneSkills: map[domain.SkillID]struct{}{10016: {}}}}
	before = immune.HP
	warriorTestCast(t, s, p, immune, d)
	if before-immune.HP != 240 {
		t.Fatal("control immunity suppressed A26 damage")
	}
	if immune.Status != nil && immune.Status.Has(domain.StatusPetrify) {
		t.Fatal("control immunity bypassed")
	}
	warriorEquip(t, s, p, "ZS-D09", domain.SlotBody)
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.CooldownMS != 500 {
		t.Fatalf("duplicate cooldown reduction%d", d.CooldownMS)
	}
}
func TestLegendaryWarriorManaRecoveryOnceBasicRoot(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-U07")
	warriorEquip(t, s, p, "ZS-A21", domain.SlotBody)
	warriorEquip(t, s, p, "ZS-A20", domain.SlotHead)
	s.skills = skillTable(domain.SkillDef{ID: 10015, Level: 1, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}})
	putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
	p.MaxMP = 1000
	p.MP = 0
	s.onAttack(Attack{ID: p.ID, Target: m.ID})
	s.stepCombat()
	if p.MP != 50 {
		t.Fatalf("multi-target basic restored%d", p.MP)
	}
}
func TestLegendaryWarriorSpeedAndOwnArmorDefense(t *testing.T) {
	for _, id := range []string{"ZS-U03", "ZS-U04"} {
		s, p, _ := legendaryMeleeFixture(t, id)
		st := domain.Stats{MoveSpeed: 100}
		s.legendaryPassiveStats(p, &st)
		if st.MoveSpeed != 130 {
			t.Fatalf("%s speed%d", id, st.MoveSpeed)
		}
		p.Player.Worn = &domain.EquipSet{}
		st.MoveSpeed = 100
		s.legendaryPassiveStats(p, &st)
		if st.MoveSpeed != 100 {
			t.Fatal("speed remained unequipped")
		}
	}
	s, p, m := legendaryMeleeFixture(t, "ZS-D03")
	m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{ID: 19001, SourceSkillID: 10009, PhysicalDamageTakenPct: 20, DurationSec: 10}, s.tick, p.ID)
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100})
	if ev.Amount != 50 {
		t.Fatalf("armor defense%d", ev.Amount)
	}
}
func TestLegendaryWarriorMissDoesNotApplyArmorOrEcho(t *testing.T) {
	for _, id := range []string{"ZS-A07", "ZS-A18"} {
		s, p, m := legendaryMeleeFixture(t, id)
		p.Stats.Hit = 0
		m.Stats.Def = 1000000
		skill := domain.SkillID(10005)
		if id == "ZS-A18" {
			skill = 10021
		}
		d := domain.SkillDef{ID: skill, Level: 1, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		before := m.HP
		warriorTestCast(t, s, p, m, d)
		if before != m.HP {
			t.Fatalf("%s miss dealt%d", id, before-m.HP)
		}
		found := false
		if m.Status != nil {
			m.Status.EachEffective(func(domain.Active) { found = true })
		}
		if found {
			t.Fatal("miss applied status")
		}
	}
}
func TestLegendaryWarriorEquippedSlowCleansing(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-D05")
	slow := domain.StatusDef{ID: domain.StatusSlow, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50, Mode: domain.ModePercent}}}
	if !s.applyStatusDef(p, slow, m.ID) {
		t.Fatal("fixture slow rejected")
	}
	warriorEquip(t, s, p, "ZS-D10", domain.SlotBody)
	s.refreshStats(p)
	if p.Status.Has(domain.StatusSlow) {
		t.Fatal("existing slow survived equipping immunity")
	}
}
func TestLegendaryWarriorChainCircleTargetCapAndNativeControls(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A02")
	p.Look.AtkDist = 100
	s.statuses = statusTable(domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 5, Control: domain.ControlStun})
	for i := 0; i < 6; i++ {
		putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
	}
	d := domain.SkillDef{ID: 10002, Level: 1, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, Statuses: []domain.StatusApplication{{ID: domain.StatusStun, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}}
	ids := warriorTestCast(t, s, p, m, d)
	if len(ids) != 5 || ids[0] != m.ID {
		t.Fatalf("targets %v", ids)
	}
	if m.Status == nil || !m.Status.Has(domain.StatusStun) {
		t.Fatal("primary lost native control")
	}
	for _, id := range ids[1:] {
		side := s.entities[id]
		if side.Status != nil && side.Status.Has(domain.StatusStun) {
			t.Fatal("side copied control")
		}
		if side.MaxHP-side.HP != 115 {
			t.Fatalf("side damage%d", side.MaxHP-side.HP)
		}
	}
}

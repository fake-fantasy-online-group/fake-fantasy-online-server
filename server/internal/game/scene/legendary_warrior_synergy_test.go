package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func warriorEquip(t *testing.T, s *Scene, p *entity.Entity, id string, slot domain.EquipSlot) {
	t.Helper()
	item := domain.ItemID(920000 + int(slot))
	s.items[item] = domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1}}}
	p.Player.Worn.Set(slot, domain.Stack{UID: int64(item), Item: item, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
}
func warriorTestCast(t *testing.T, s *Scene, p, m *entity.Entity, d domain.SkillDef) []domain.EntityID {
	t.Helper()
	d.TargetEnemy = true
	s.legendaryPrepareSkill(p, &d, m.ID)

	ids := s.resolveAreaTargets(p, d, m.ID, nil)
	if len(ids) == 0 {
		ids = []domain.EntityID{m.ID}
	}
	ids = s.legendarySelectTargets(p, d, m.ID, ids)
	finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	root := s.equipmentState().root
	root.def = d
	root.center = p.Pos
	root.targets = ids
	for _, id := range ids {
		s.applySkill(p, s.entities[id], s.legendaryTargetDefinition(p, d, s.entities[id], m.ID, ids), 0, false)
	}
	finish()
	return ids
}
func TestLegendaryWarriorManaBuildActualZeroMP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ids    []string
		damage int32
	}{{"drain", []string{"ZS-U02"}, 100}, {"conversion", []string{"ZS-A10"}, 220}, {"damage", []string{"ZS-A09"}, 125}, {"three", []string{"ZS-U02", "ZS-A10", "ZS-A09"}, 365}} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.ids[0])
			for i, id := range tc.ids[1:] {
				warriorEquip(t, s, p, id, []domain.EquipSlot{domain.SlotBody, domain.SlotHead}[i])
			}
			m.MP = 0
			d := domain.SkillDef{ID: 10013, Level: 1, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, {Kind: domain.EffectDamageMP, SourceAttackPct: 60}}}
			before := m.HP
			warriorTestCast(t, s, p, m, d)
			if got := before - m.HP; got != tc.damage {
				t.Fatalf("damage=%d want%d", got, tc.damage)
			}
			if m.MP != 0 {
				t.Fatalf("negative mana %d", m.MP)
			}
			p.Player.Worn = &domain.EquipSet{}
			before = m.HP
			warriorTestCast(t, s, p, m, d)
			if before-m.HP != 100 {
				t.Fatalf("unequipped power survived: %d", before-m.HP)
			}
		})
	}
}
func TestLegendaryWarriorHeartEyeBuildActualAoE(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A21")
	warriorEquip(t, s, p, "ZS-A20", domain.SlotBody)
	warriorEquip(t, s, p, "ZS-A18", domain.SlotHead)
	eye := domain.SkillDef{ID: 10015, Level: 1, TargetEnemy: true, Kind: domain.SkillSingle, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 165, GuaranteedHit: true}}
	s.skills = skillTable(eye)
	far := putMonster(s, 901, 10000, domain.Stats{}, 200, 100)
	before, farBefore := m.HP, far.HP
	d := domain.SkillDef{LegendaryBasic: true, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	ids := warriorTestCast(t, s, p, m, d)
	if len(ids) != 2 {
		t.Fatalf("targets %v", ids)
	}
	if before-m.HP != 405 || farBefore-far.HP != 769 {
		t.Fatalf("damage primary=%d far=%d", before-m.HP, farBefore-far.HP)
	}
}
func TestLegendaryWarriorArmorBuildSameHitAndGeometry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ids    []string
		damage int32
		width  int32
	}{{"width", []string{"ZS-A04"}, 115, 190}, {"apply", []string{"ZS-A07"}, 195, 100}, {"strength", []string{"ZS-A08"}, 100, 100}, {"three", []string{"ZS-A04", "ZS-A07", "ZS-A08"}, 239, 190}} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.ids[0])
			for i, id := range tc.ids[1:] {
				warriorEquip(t, s, p, id, []domain.EquipSlot{domain.SlotBody, domain.SlotHead}[i])
			}
			st := domain.StatusDef{ID: 19001, SourceSkillID: 10009, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, PhysicalDamageTakenPct: 30}
			s.statuses = statusTable(st)
			pierce := domain.SkillDef{ID: 10005, Level: 1, TargetEnemy: true, Kind: domain.SkillArea, HurtType: domain.HurtPhysical, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Length: 100, Width: 100}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
			br := domain.SkillDef{ID: 10009, Level: 1, TargetEnemy: true, Kind: domain.SkillSingle, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, Statuses: []domain.StatusApplication{{ID: 19001, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}}
			s.skills = skillTable(pierce, br)
			check := pierce
			s.legendaryPrepareSkill(p, &check, m.ID)
			if check.Area.Width != tc.width {
				t.Fatalf("width%d", check.Area.Width)
			}
			before := m.HP
			warriorTestCast(t, s, p, m, pierce)
			if got := before - m.HP; got != tc.damage {
				t.Fatalf("same hit damage%d expected%d", got, tc.damage)
			}
			if tc.name == "three" {
				s.legendaryPrepareSkill(p, &br, m.ID)
				if br.Area.Width != 190 || br.Area.Length != 120 {
					t.Fatalf("borrowed geometry %+v", br.Area)
				}
			}
		})
	}
}
func TestLegendaryWarriorBasicNativeAdmissionAndNoCost(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A21")
	warriorEquip(t, s, p, "ZS-A20", domain.SlotBody)
	warriorEquip(t, s, p, "ZS-A18", domain.SlotHead)
	s.skills = skillTable(domain.SkillDef{ID: 10015, Level: 1, TargetEnemy: true, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 165, GuaranteedHit: true}})
	far := putMonster(s, 901, 10000, domain.Stats{}, 200, 100)
	before, farBefore := m.HP, far.HP
	p.MP = 0
	s.onAttack(Attack{ID: p.ID, Target: m.ID})
	s.stepCombat()
	if before-m.HP != 405 || farBefore-far.HP != 769 {
		t.Fatalf("native attack primary%d far%d", before-m.HP, farBefore-far.HP)
	}
	if p.MP != 0 {
		t.Fatal("basic spent mana")
	}
	before = m.HP
	s.onAttack(Attack{ID: p.ID, Target: m.ID})
	s.stepCombat()
	if m.HP != before {
		t.Fatal("basic cadence bypassed")
	}
	m.HP = 0
	s.onAttack(Attack{ID: p.ID, Target: m.ID})
	if len(s.attacks) != 0 {
		t.Fatal("dead target accepted")
	}
}
func TestLegendaryWarriorWarcryMaxRadiusAndPull(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-U04")
	warriorEquip(t, s, p, "ZS-A28", domain.SlotBody)
	d := domain.SkillDef{ID: 10028, Level: 1, Kind: domain.SkillArea, TargetEnemy: true, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}, MPCost: 100, Effect: domain.SkillEffect{Kind: domain.EffectTaunt, TauntPower: 100}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Area.Radius != 170 || d.DirectEffects()[0].TauntPower != 200 {
		t.Fatalf("bad composed warcry %+v", d)
	}
	s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 230, Y: 100})
	c := legendaryContext{Scene: s, Owner: p, Target: m, Power: domain.LegendaryPowerDefinition{ID: "ZS-A28"}}
	warriorPull(&c, m)
	if m.Pos.X >= 230 || sqDist(m.Pos, p.Pos) < 30*30 {
		t.Fatalf("bad pull %+v", m.Pos)
	}
	m.Monster.Kind = domain.MonsterBoss
	s.defs = spawn.MapDefs{338: {ID: 338, ImmuneSkills: map[domain.SkillID]struct{}{10028: {}}}}
	old := m.Pos
	warriorPull(&c, m)
	if m.Pos != old {
		t.Fatal("boss immunity bypassed")
	}
}
func TestLegendaryWarriorArmorRestoredNativeRanks(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A04")
	warriorEquip(t, s, p, "ZS-A07", domain.SlotBody)
	warriorEquip(t, s, p, "ZS-A08", domain.SlotHead)
	s.skills, s.statuses = nativeRestoredSceneFixtures(t)
	p.Player.Char.Skills = domain.Learned{10005: 5, 10009: 5}
	def, ok := warriorLearned(&legendaryContext{Scene: s, Owner: p}, 10005, false)
	if !ok {
		t.Fatal("missing native pierce")
	}
	br, ok := warriorLearned(&legendaryContext{Scene: s, Owner: p}, 10009, false)
	if !ok || len(br.Statuses) == 0 {
		t.Fatal("missing native armor break")
	}
	base, ok := s.resolveSkillStatusDef(br, br.Statuses[0], 0)
	if !ok {
		t.Fatal("missing native break status")
	}
	warriorTestCast(t, s, p, m, def)
	found := false
	if m.Status != nil {
		m.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == base.ID {
				found = true
				if a.Def.PhysicalDamageTakenPct != specialScale(base.PhysicalDamageTakenPct, 150) {
					t.Fatalf("native status%d base%d", a.Def.PhysicalDamageTakenPct, base.PhysicalDamageTakenPct)
				}
			}
		})
	}
	if !found {
		t.Fatalf("native armor not applied from %+v", br.Statuses)
	}
}
func TestLegendaryWarriorPullStopsAtWallsAndBodies(t *testing.T) {
	for _, wall := range []bool{true, false} {
		s, p, m := legendaryMeleeFixture(t, "ZS-A28")
		s.aoi.Move(m, domain.Pos{MapID: 7, X: 230, Y: 100})
		if wall {
			s.walkable = func(pos domain.Pos) bool { return pos.X < 170 || pos.X > 180 }
		} else {
			putMonster(s, 901, 10000, domain.Stats{}, 180, 100)
		}
		c := legendaryContext{Scene: s, Owner: p, Power: domain.LegendaryPowerDefinition{ID: "ZS-A28"}}
		warriorPull(&c, m)
		if m.Pos.X < 180 {
			t.Fatalf("pull crossed obstacle %+v", m.Pos)
		}
		if m.Pos.X == 230 {
			t.Fatal("pull did not approach obstacle")
		}
	}
}
func TestLegendaryWarriorHeartEyeStandaloneAndUnlearned(t *testing.T) {
	for _, learned := range []bool{false, true} {
		s, p, m := legendaryMeleeFixture(t, "ZS-A21")
		p.Player.Char.Skills = domain.Learned{}
		want := int32(200)
		if learned {
			p.Player.Char.Skills[10015] = 1
			want = 165
		}
		s.skills = skillTable(domain.SkillDef{ID: 10015, Level: 1, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 165, GuaranteedHit: true}})
		p.Stats.Hit = 0
		m.Stats.Def = 1000000
		before := m.HP
		s.onAttack(Attack{ID: p.ID, Target: m.ID})
		s.stepCombat()
		if before-m.HP != want {
			t.Fatalf("learned%v damage%d", learned, before-m.HP)
		}
	}
}
func TestLegendaryWarriorManaShieldTierExclusive(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-U02")
	warriorEquip(t, s, p, "ZS-A10", domain.SlotBody)
	warriorEquip(t, s, p, "ZS-A09", domain.SlotHead)
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{ID: 19009, DamageShield: 10, DurationSec: 10}, s.tick, m.ID)
	d := domain.SkillDef{ID: 10013, Level: 1, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, {Kind: domain.EffectDamageMP, SourceAttackPct: 60}}}
	before := m.HP
	warriorTestCast(t, s, p, m, d)
	if before-m.HP != 632 {
		t.Fatalf("exclusive shield tier damage%d", before-m.HP)
	}
}
func TestLegendaryWarriorFlyMissDoesNotHitSides(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A12")
	p.Stats.Hit = 0
	m.Stats.Def = 1000000
	side := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
	before := side.HP
	d := domain.SkillDef{ID: 10017, Level: 1, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	warriorTestCast(t, s, p, m, d)
	if side.HP != before {
		t.Fatal("missed primary still triggered landing AoE")
	}
}
func TestLegendaryWarriorDrainFractionBeforeBaseConversion(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-U02")
	warriorEquip(t, s, p, "ZS-A10", domain.SlotBody)
	warriorEquip(t, s, p, "ZS-D04", domain.SlotHead)
	p.Stats.MinAtk = 1000
	p.Stats.MaxAtk = 1000
	m.MP = 10000
	m.MaxMP = 10000
	d := domain.SkillDef{ID: 10013, Level: 1, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, {Kind: domain.EffectDamageMP, SourceAttackPct: 61}}}
	before := m.HP
	warriorTestCast(t, s, p, m, d)
	if m.MP != 8872 || before-m.HP != 3256 {
		t.Fatalf("drain%d damage%d want1128/3256", 10000-m.MP, before-m.HP)
	}
}
func TestLegendaryWarriorPieceOrderDoesNotChangeManaBase(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		s, p, m := legendaryMeleeFixture(t, "ZS-U02")
		body, head := "ZS-A10", "ZS-A09"
		if reverse {
			body, head = head, body
		}
		warriorEquip(t, s, p, body, domain.SlotBody)
		warriorEquip(t, s, p, head, domain.SlotHead)
		d := domain.SkillDef{ID: 10013, Level: 1, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, {Kind: domain.EffectDamageMP, SourceAttackPct: 60}}}
		before := m.HP
		warriorTestCast(t, s, p, m, d)
		if before-m.HP != 365 {
			t.Fatalf("reverse%v damage%d", reverse, before-m.HP)
		}
	}
}
func TestLegendaryWarriorIncomingMitigationAddsAcrossGear(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-D05")
	warriorEquip(t, s, p, "ZS-D07", domain.SlotHead)
	m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100})
	if ev.Amount != 15 {
		t.Fatalf("expected85%% additive mitigation, got%d", ev.Amount)
	}
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func universalTestContext(t *testing.T, id string) (*Scene, *entity.Entity, *entity.Entity, *legendaryContext) {
	t.Helper()
	s, p, m := legendaryMeleeFixture(t, id)
	if m.Status == nil {
		m.Status = domain.NewStatusSet()
	}
	cs := s.legendaryContexts(p, m, combat.Blow{SkillID: 1}, event.DamageDealt{})
	if len(cs) != 1 {
		t.Fatalf("%s current handler contexts=%d", id, len(cs))
	}
	return s, p, m, &cs[0]
}
func TestLegendaryUniversalSkillSynergy(t *testing.T) {
	for _, tc := range []struct {
		id    string
		shape domain.SkillAreaShape
		kind  domain.SkillKind
		want  int32
	}{
		{"TY-A04", domain.SkillAreaCircle, domain.SkillArea, 140}, {"TY-A06", domain.SkillAreaRectangle, domain.SkillArea, 160}, {"TY-A08", 0, domain.SkillSingle, 160}, {"TY-U03", 0, domain.SkillSingle, 70}, {"TY-U15", 0, domain.SkillSingle, 800}, {"TY-A37", 0, domain.SkillSingle, 175},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m, _ := universalTestContext(t, tc.id)
			d := domain.SkillDef{ID: 123, Kind: tc.kind, MPCost: 100, CooldownMS: 1000, Dist: 100, LegendaryBasic: tc.id == "TY-A37", Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}, Area: domain.SkillAreaTargeting{Shape: tc.shape, Center: domain.SkillAreaCenterCaster, Radius: 100, Width: 100, Length: 100}}
			if tc.shape == domain.SkillAreaCircle {
				d.Area.Width = 0
				d.Area.Length = 0
			}
			if tc.shape == domain.SkillAreaRectangle {
				d.Area.Radius = 0
			}
			s.legendaryPrepareSkill(p, &d, m.ID)
			got := d.Effects[0].DamagePct
			switch tc.id {
			case "TY-A04":
				got = d.Area.Radius
			case "TY-A06":
				got = d.Area.Width
			case "TY-U03":
				got = d.MPCost
			case "TY-U15":
				got = d.CooldownMS
			}
			if got != tc.want {
				t.Fatalf("got %d want %d def=%+v", got, tc.want, d)
			}
			s.legendaryFinalizeSkill(&d)
			if d.LegendaryModifiers.AppliedDamageBP > 7500 {
				t.Fatal("double applied")
			}
		})
	}
}
func TestLegendaryUniversalConservationAdditiveCrossClass(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A24")
	warriorEquip(t, s, p, "TY-U03", domain.SlotRing)
	warriorEquip(t, s, p, "ZS-A02", domain.SlotBody)
	d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, MPCost: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.MPCost != 70 || d.LegendaryModifiers.AppliedDamageBP < 6000 {
		t.Fatalf("cost %d damage bucket %d", d.MPCost, d.LegendaryModifiers.AppliedDamageBP)
	}
	d2 := domain.SkillDef{Kind: domain.SkillSingle, MPCost: 100, LegendaryModifiers: domain.LegendarySkillModifiers{ManaReductionBP: 10000}}
	s.legendaryFinalizeSkill(&d2)
	if d2.MPCost != 0 {
		t.Fatal("explicit free skill charged")
	}
}
func TestLegendaryUniversalGeometryComposesOnce(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A04")
	d := domain.SkillDef{Kind: domain.SkillArea, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 300}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Area.Angle != 360 || !d.Area.Valid() {
		t.Fatalf("cone=%+v", d.Area)
	}
	d = domain.SkillDef{Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Width: 160, Length: 100}, LegendaryModifiers: domain.LegendarySkillModifiers{GeometryInherited: true, WidthBP: 6000}}
	s.legendaryFinalizeSkill(&d)
	if d.Area.Width != 160 {
		t.Fatal("borrowed width applied twice")
	}
}
func TestLegendaryUniversalPrimaryAndHealGeometry(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-A34")
	c.Primary = m.ID
	d := domain.SkillDef{Kind: domain.SkillArea, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}, LegendaryModifiers: domain.LegendarySkillModifiers{DamageBP: 6000}}
	s.legendaryFinalizeSkill(&d)
	universalPrimaryModifier(c, &d)
	if d.Effects[0].DamagePct != 240 {
		t.Fatalf("primary additive=%d", d.Effects[0].DamagePct)
	}
	_ = p
	s, p, m, _ = universalTestContext(t, "TY-U10")
	d = domain.SkillDef{Kind: domain.SkillArea, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}, Effects: []domain.SkillEffect{{Kind: domain.EffectHeal}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Area.Radius != 140 {
		t.Fatalf("heal radius=%d", d.Area.Radius)
	}
}
func TestLegendaryUniversalNativePeriodicModifiers(t *testing.T) {
	for _, tc := range []struct {
		id          string
		status      domain.StatusID
		delta, want int64
	}{{"TY-A18", domain.StatusPoison, -100, -200}, {"TY-A27", 1036, -100, -200}, {"TY-U05", 998, 100, 160}} {
		t.Run(tc.id, func(t *testing.T) {
			s, _, _, c := universalTestContext(t, tc.id)
			d := domain.StatusDef{ID: tc.status, SourceSkillID: 123, IntervalSec: 1, DurationSec: 9}
			if tc.delta > 0 {
				d.Affixes = []domain.Affix{{Attr: domain.AttrHPRegen, Value: 100}}
			}
			universalNativeStatus(c, &d)
			s.legendaryFinalizeStatus(&d)
			if got := s.legendaryPeriodicAmount(domain.Active{Def: d}, tc.delta); got != tc.want {
				t.Fatalf("tick=%d", got)
			}
			if d.DurationSec != 9 {
				t.Fatal("duration changed")
			}
			d.LegendaryPowerID = "TY-A18"
			if s.legendaryPeriodicAmount(domain.Active{Def: d}, tc.delta) != tc.delta {
				t.Fatal("recursive power tick amplified")
			}
		})
	}
}
func TestLegendaryUniversalShieldStrengthAndCost(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-D16")
	d := domain.StatusDef{ID: 998, SkillApplied: true, DamageShield: 100, DurationSec: 5}
	universalNativeStatus(c, &d)
	s.legendaryFinalizeStatus(&d)
	s.legendaryFinalizeStatus(&d)
	if d.DamageShield != 160 {
		t.Fatalf("shield %d", d.DamageShield)
	}
	_ = p
	_ = m
	s, p, m, _ = universalTestContext(t, "TY-U13")
	warriorEquip(t, s, p, "TY-U03", domain.SlotRing)
	skill := domain.SkillDef{Kind: domain.SkillSingle, MPCost: 100, Statuses: []domain.StatusApplication{{ProgressionDamageShield: 100}}}
	s.legendaryPrepareSkill(p, &skill, m.ID)
	if skill.MPCost != 50 {
		t.Fatalf("stack cost %d", skill.MPCost)
	}
}
func TestLegendaryUniversalDefenseAndSelfDamage(t *testing.T) {
	for _, tc := range []struct {
		id               string
		area             bool
		origin           domain.DamageOrigin
		mp, want, mpwant int32
	}{{"TY-D05", true, domain.DamageNormalOrigin, 100, 75, 100}, {"TY-D19", false, domain.DamageDOTOrigin, 100, 75, 100}, {"TY-D11", false, domain.DamageNormalOrigin, 20, 80, 0}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m, c := universalTestContext(t, tc.id)
			_ = s
			p.MP = tc.mp
			c.Blow.Area = tc.area
			c.Blow.Origin = tc.origin
			if got := universalSynergyIncoming(c, 100); got != tc.want || p.MP != tc.mpwant {
				t.Fatalf("remaining=%d mp=%d", got, p.MP)
			}
			c.Target = p
			if universalSynergyIncoming(c, 100) != 100 {
				t.Fatal("self damage used defenses")
			}
			_ = m
		})
	}
}
func TestLegendaryUniversalControlAndSlow(t *testing.T) {
	for _, id := range []string{"TY-D09", "TY-U08"} {
		t.Run(id, func(t *testing.T) {
			s, p, m, _ := universalTestContext(t, id)
			d := domain.StatusDef{ID: 998, DurationSec: 5, BlockMove: true, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -100}}}
			s.legendaryStatusReceiving(p, m, &d)
			s.legendaryFinalizeStatus(&d)
			if id == "TY-D09" && d.LegendaryDurationMS != 3500 {
				t.Fatalf("duration %d", d.LegendaryDurationMS)
			}
			if id == "TY-U08" && d.Affixes[0].Value != -60 {
				t.Fatalf("slow %d", d.Affixes[0].Value)
			}
		})
	}
}
func TestLegendaryUniversalEquipmentOnlyStats(t *testing.T) {
	for _, id := range []string{"TY-D01", "TY-U20"} {
		t.Run(id, func(t *testing.T) {
			s, p, _, c := universalTestContext(t, id)
			item := s.items[900001]
			item.Equip.Bonus = domain.Stats{Def: 100, MoveSpeed: 100}
			s.items[900001] = item
			stats := domain.Stats{Def: 1000, MoveSpeed: 1000}
			universalEquipmentStats(c, &stats)
			got := stats.Def
			if id == "TY-U20" {
				got = stats.MoveSpeed
			}
			if got != 1040 {
				t.Fatalf("equipment boost=%d", got)
			}
			p.Player.Worn = &domain.EquipSet{}
			stats = domain.Stats{Def: 1000, MoveSpeed: 1000}
			universalEquipmentStats(c, &stats)
			if stats.Def != 1000 || stats.MoveSpeed != 1000 {
				t.Fatal("base stats amplified")
			}
		})
	}
}
func TestLegendaryUniversalShieldBreakNoHPAmplification(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A16")
	for _, tc := range []struct{ shield, want int32 }{{100, 50}, {300, 0}, {1, 99}, {0, 100}} {
		m.Status = domain.NewStatusSet()
		if tc.shield > 0 {
			m.Status.Apply(domain.StatusDef{ID: 999, DamageShield: tc.shield, DurationSec: 10}, s.tick, p.ID)
		}
		got, _ := s.legendaryAbsorbDamage(p, m, combat.Blow{}, 100)
		if got != tc.want {
			t.Fatalf("shield%d remaining%d want%d", tc.shield, got, tc.want)
		}
	}
}
func TestLegendaryUniversalBasicRootManaOnce(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-U02")
	p.MP = 0
	p.MaxMP = 100
	c.IsBasic = true
	c.Skill = 10021
	c.RootID = 1
	c.Blow.Origin = domain.DamageNormalOrigin
	universalBasicMana(c)
	universalBasicMana(c)
	if p.MP != 10 {
		t.Fatalf("root mana=%d", p.MP)
	}
	c.IsChild = true
	c.RootID = 2
	universalBasicMana(c)
	if p.MP != 10 {
		t.Fatal("child regenerated resources")
	}
	_ = s
	_ = m
}
func TestLegendaryUniversalBasicFanSharedNativeRoot(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A01")
	warriorEquip(t, s, p, "TY-A37", domain.SlotGlove)
	warriorEquip(t, s, p, "TY-U02", domain.SlotRing)
	p.MP = 0
	p.MaxMP = 100
	p.Look.AtkDist = 100
	p.HP = 1000
	p.Status.Apply(domain.StatusDef{ID: 1018, DurationSec: 120, PoisonOnHitChance: 100, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true, LifeStealPct: 10, LifeStealChance: 100}, s.tick, p.ID)
	s.statuses = map[domain.StatusKey]domain.StatusDef{}
	s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}] = domain.StatusDef{ID: domain.StatusPoison, Level: 21, TypeKnown: true, Type: domain.StatusBad, DurationSec: 60, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	side := putMonster(s, 901, 10000, domain.Stats{}, 140, 105)
	outside := putMonster(s, 902, 10000, domain.Stats{}, 50, 100)
	finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	before := m.HP
	sideBefore := side.HP
	outsideBefore := outside.HP
	if !s.legendaryBasicAttack(p, m, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100}) {
		t.Fatal("not transformed")
	}
	finish()
	if before-m.HP != 175 || sideBefore-side.HP != 122 || outsideBefore-outside.HP != 0 {
		t.Fatalf("main %d side%d outside%d", before-m.HP, sideBefore-side.HP, outsideBefore-outside.HP)
	}
	if p.MP != 10 {
		t.Fatalf("fan resource once=%d", p.MP)
	}
	if !m.Status.Has(domain.StatusPoison) || !side.Status.Has(domain.StatusPoison) {
		t.Fatal("native poison not inherited by fan")
	}
	if p.HP != 1029 {
		t.Fatalf("native lifesteal main+side=%d", p.HP)
	}
}
func TestLegendaryUniversalNativeManaEquipmentConservation(t *testing.T) {
	for _, power := range []string{"TY-A24", "TY-U03"} {
		t.Run(power, func(t *testing.T) {
			s, p, tr := uniqueTestGear(t, "skill_economy")
			warriorEquip(t, s, p, power, domain.SlotRing)
			m := putMonster(s, 900, 10000, domain.Stats{}, 130, 100)
			d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, MPCost: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
			s.applyUniqueSkillModifiers(p, &d)
			nativeCost := domain.ApplyUniqueBP(int32(100), -tr.PowerBP)
			s.legendaryPrepareSkill(p, &d, m.ID)
			if power == "TY-A24" {
				if d.MPCost != nativeCost || d.LegendaryModifiers.AppliedDamageBP != tr.PowerBP*2 {
					t.Fatalf("native reduction=%d cost%d/%d damage%d", tr.PowerBP, d.MPCost, nativeCost, d.LegendaryModifiers.AppliedDamageBP)
				}
			} else if d.LegendaryModifiers.AppliedDamageBP != 0 {
				t.Fatal("cost reduction granted damage without conservation")
			}
		})
	}
	s, p, m, _ := universalTestContext(t, "TY-A24")
	d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, MPCost: 50, ManaCostReductionRanks: 10, ManaCostReductionApplied: true, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Effects[0].DamagePct != 100 {
		t.Fatal("learned skill discount created conservation bonus")
	}
}
func TestLegendaryUniversalClassTransformThenGenericGeometry(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A02")
	warriorEquip(t, s, p, "TY-A04", domain.SlotGlove)
	p.Look.AtkDist = 100
	d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, Dist: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Area.Shape != domain.SkillAreaCircle || d.Area.Radius != 182 || d.Effects[0].DamagePct != 115 {
		t.Fatalf("class shape + generic geometry=%+v damage%d", d.Area, d.Effects[0].DamagePct)
	}
}
func TestLegendaryUniversalNativeAndPowerCostSingleBucket(t *testing.T) {
	s, p, tr := uniqueTestGear(t, "skill_economy")
	warriorEquip(t, s, p, "TY-A24", domain.SlotRing)
	warriorEquip(t, s, p, "TY-U03", domain.SlotNeck)
	d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, MPCost: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.applyUniqueSkillModifiers(p, &d)
	s.legendaryPrepareSkill(p, &d)
	reduction := tr.PowerBP + 3000
	if d.MPCost != 100-reduction/100 || d.LegendaryModifiers.AppliedDamageBP != reduction*2 {
		t.Fatalf("native%d cost%d damage%d", tr.PowerBP, d.MPCost, d.LegendaryModifiers.AppliedDamageBP)
	}
}
func TestLegendaryUniversalNativeStatusBoundaryAndTick(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A18")
	d := domain.StatusDef{ID: domain.StatusPoison, TypeKnown: true, Type: domain.StatusBad, SourceSkillID: 10211, DurationSec: 10, IntervalSec: 1, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -100}}}
	if !s.applyStatusDef(m, d, p.ID) {
		t.Fatal("native poison rejected")
	}
	before := m.HP
	s.tick += domain.Ticks(1000)
	s.stepStatus()
	if before-m.HP != 200 {
		t.Fatalf("native tick=%d", before-m.HP)
	}
	s, p, _, _ = universalTestContext(t, "TY-D16")
	if !s.applyStatusDef(p, domain.StatusDef{ID: 998, SourceSkillID: 10320, DurationSec: 5, DamageShield: 100}, p.ID) {
		t.Fatal("native shield rejected")
	}
	if universalShieldValue(p) != 160 {
		t.Fatalf("native shield=%d", universalShieldValue(p))
	}
}
func TestLegendaryUniversalManaDefenseNativeRestoration(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-D11")
	warriorEquip(t, s, p, "TY-U03", domain.SlotRing)
	p.MP = 20
	p.MaxMP = 100
	m.Stats.MinAtk = 100
	m.Stats.MaxAtk = 100
	before := p.HP
	_, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100})
	if before-p.HP != 80 || p.MP != 0 {
		t.Fatalf("spillover HP%d MP%d", before-p.HP, p.MP)
	}
	s.applySkill(p, p, domain.SkillDef{ID: 10307, Effects: []domain.SkillEffect{{Kind: domain.EffectRestoreMP, RestoreMPFlat: 50}}}, 0, false)
	if p.MP != 50 {
		t.Fatalf("native mana recovery=%d", p.MP)
	}
	before = p.HP
	_, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100})
	if before-p.HP != 70 || p.MP != 20 {
		t.Fatalf("recovered mana shield HP%d MP%d", before-p.HP, p.MP)
	}
	d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, MPCost: 20, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.MPCost != 14 {
		t.Fatalf("cheap skill=%d", d.MPCost)
	}
}
func TestLegendaryUniversalCopiedPoisonKeepsSnapshotOnce(t *testing.T) {
	s, _, _, c := universalTestContext(t, "TY-A18")
	d := domain.StatusDef{ID: domain.StatusPoison, SourceSkillID: 10211, IntervalSec: 1, DurationSec: 5}
	universalNativeStatus(c, &d)
	s.legendaryFinalizeStatus(&d)
	copy := d
	copy.LegendaryPoisonCopy = true
	universalNativeStatus(c, &copy)
	s.legendaryFinalizeStatus(&copy)
	if s.legendaryPeriodicAmount(domain.Active{Def: copy}, -100) != -200 {
		t.Fatal("copied poison reapplied source bucket")
	}
}
func TestLegendaryUniversalActiveCostDamageNeverBecomesBasicBonus(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A24")
	warriorEquip(t, s, p, "TY-U03", domain.SlotRing)
	warriorEquip(t, s, p, "TY-A08", domain.SlotGlove)
	d := domain.SkillDef{LegendaryBasic: true, Kind: domain.SkillSingle, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Effects[0].DamagePct != 100 || d.LegendaryModifiers.ManaReductionBP != 0 {
		t.Fatalf("basic acquired active benefits: %+v", d.LegendaryModifiers)
	}
}
func TestLegendaryUniversalBasicPowerRetainsNativeCritical(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A37")
	p.Stats.CritRate = 10000
	finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	before := m.HP
	if !s.legendaryBasicAttack(p, m, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100}) {
		t.Fatal("basic modifier not executed")
	}
	finish()
	if before-m.HP != 350 {
		t.Fatalf("native critical lost: damage%d", before-m.HP)
	}
}
func TestLegendaryUniversalPeriodicDefenseOutlivesEnemySource(t *testing.T) {
	s, p, _, _ := universalTestContext(t, "TY-D19")
	if n := s.universalPeriodicIncoming(nil, p, 100, domain.EntityID(99999)); n != 75 {
		t.Fatalf("despawned source damage%d", n)
	}
	if n := s.universalPeriodicIncoming(nil, p, 100, p.ID); n != 100 {
		t.Fatal("self periodic abuse")
	}
}

// Metadata must agree with the live additive equipment-cost bucket even while
// the owner is dead/loading; preview does not authorize combat execution.
func TestLegendaryUniversalDeadPreviewEquipmentCost(t *testing.T) {
	s, p, tr := uniqueTestGear(t, "skill_economy")
	warriorEquip(t, s, p, "TY-A24", domain.SlotRing)
	warriorEquip(t, s, p, "TY-U03", domain.SlotNeck)
	p.HP = 0
	d := domain.SkillDef{ID: 10002, Kind: domain.SkillSingle, MPCost: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.applyUniqueSkillModifiers(p, &d)
	s.legendaryPreviewSkill(p, &d)
	reduction := tr.PowerBP + 3000
	if d.MPCost != 100-reduction/100 || d.LegendaryModifiers.AppliedDamageBP != reduction*2 {
		t.Fatalf("dead preview native%d cost%d damage%d", tr.PowerBP, d.MPCost, d.LegendaryModifiers.AppliedDamageBP)
	}
}

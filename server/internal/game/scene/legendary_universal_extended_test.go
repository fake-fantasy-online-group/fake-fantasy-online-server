package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
	"time"
)

func TestLegendaryUniversalExtendedGeometryAndUtilities(t *testing.T) {
	for _, tc := range []struct {
		id     string
		effect domain.EffectKind
		want   int32
	}{
		{"TY-A02", domain.EffectDamage, 160}, {"TY-A03", domain.EffectDamage, 150}, {"TY-A07", domain.EffectDamage, 200}, {"TY-A09", domain.EffectDamage, 1}, {"TY-A19", domain.EffectDamageMP, 8000}, {"TY-A21", domain.EffectPlaceTrap, 150}, {"TY-U01", domain.EffectDamage, 700}, {"TY-U04", domain.EffectHeal, 70}, {"TY-U06", domain.EffectRestoreMP, 150}, {"TY-U07", domain.EffectTeleportPoint, 140}, {"TY-U16", domain.EffectHeal, 160}, {"TY-U17", domain.EffectResurrect, 75}, {"TY-U18", domain.EffectHeal, 9000}, {"TY-U19", domain.EffectRevealInvisible, 200},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m, _ := universalTestContext(t, tc.id)
			d := domain.SkillDef{ID: 123, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 100, MPCost: 100, PrepareMS: 1000, ProjectileSpeedPXPerSec: 100, Effects: []domain.SkillEffect{{Kind: tc.effect, DamagePct: 100, TrapRadius: 100, RestoreMPFlat: 100, HealFlat: 100, HealPctOfMax: 50, RevealRadius: 100, ClearHarmful: true, DispelChanceBP: 5000}}}
			if tc.id == "TY-A02" {
				d.Kind = domain.SkillArea
				d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Width: 100, Length: 100}
			}
			if tc.id == "TY-A03" {
				d.Kind = domain.SkillArea
				d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Angle: 120, Radius: 100}
			}
			if tc.id == "TY-A09" || tc.id == "TY-A35" {
				d.LegendaryBasic = true
			}
			s.legendaryPrepareSkill(p, &d, m.ID)
			var got int32
			switch tc.id {
			case "TY-A02":
				got = d.Area.Length
			case "TY-A03":
				got = d.Area.Radius
			case "TY-A07":
				got = d.ProjectileSpeedPXPerSec
			case "TY-A09":
				if d.Effects[0].GuaranteedHit {
					got = 1
				}
			case "TY-A19":
				got = d.LegendaryModifiers.ManaDrainBP
			case "TY-A21":
				got = d.Effects[0].TrapRadius
			case "TY-A35", "TY-U07":
				got = d.Dist
			case "TY-U01":
				got = d.PrepareMS
			case "TY-U04":
				got = d.MPCost
			case "TY-U06":
				got = d.Effects[0].RestoreMPFlat
			case "TY-U16":
				got = d.Effects[0].HealFlat
			case "TY-U17":
				got = d.Effects[0].HealPctOfMax
			case "TY-U18":
				got = d.Effects[0].DispelChanceBP
			case "TY-U19":
				got = d.Effects[0].RevealRadius
			}
			if got != tc.want {
				t.Fatalf("got%d want%d", got, tc.want)
			}
		})
	}
}
func TestLegendaryUniversalProjectileUsesExistingTravelOnly(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A07")
	m.Pos.X = 200
	d := domain.SkillDef{ID: 123, Dist: 100, ProjectileSpeedPXPerSec: 100}
	now := time.Unix(10, 0)
	before := s.skillImpactAt(p, d, m.ID, nil, now)
	s.legendaryPrepareSkill(p, &d, m.ID)
	after := s.skillImpactAt(p, d, m.ID, nil, now)
	if before.Sub(now) != time.Second || after.Sub(now) != 500*time.Millisecond || d.Dist != 140 {
		t.Fatalf("travel%v/%v range%d", before.Sub(now), after.Sub(now), d.Dist)
	}
	d = domain.SkillDef{ID: 123, Dist: 100}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Dist != 100 || d.ProjectileSpeedPXPerSec != 0 {
		t.Fatal("invented projectile")
	}
}
func TestLegendaryUniversalSingleCircleThenGenericShape(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A05")
	warriorEquip(t, s, p, "TY-A04", domain.SlotGlove)
	warriorEquip(t, s, p, "TY-A08", domain.SlotRing)
	d := domain.SkillDef{ID: 123, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 300, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if !d.Area.Valid() || d.Area.Radius != 168 || d.Effects[0].DamagePct != 100 {
		t.Fatalf("geometry%+v damage%d", d.Area, d.Effects[0].DamagePct)
	}
}
func TestLegendaryUniversalNativeStatusCostClassification(t *testing.T) {
	for _, tc := range []struct {
		id     string
		status domain.StatusDef
	}{{"TY-U12", domain.StatusDef{ID: 998, BlockMove: true}}, {"TY-U14", domain.StatusDef{ID: domain.StatusPoison}}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m, _ := universalTestContext(t, tc.id)
			tc.status.Level = 1
			s.statuses = map[domain.StatusKey]domain.StatusDef{{ID: tc.status.ID, Level: 1}: tc.status}
			d := domain.SkillDef{ID: 123, MPCost: 100, Statuses: []domain.StatusApplication{{ID: tc.status.ID, Level: 1}}}
			s.legendaryPrepareSkill(p, &d, m.ID)
			if d.MPCost != 70 {
				t.Fatalf("cost%d", d.MPCost)
			}
		})
	}
}
func TestLegendaryUniversalNativeShieldSupportsCasting(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A40")
	p.Status.Apply(domain.StatusDef{ID: 998, DamageShield: 100, DurationSec: 5, SourceSkillID: 10320}, s.tick, p.ID)
	d := domain.SkillDef{ID: 123, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Effects[0].DamagePct != 140 || universalShieldValue(p) != 100 {
		t.Fatal("shield casting did not compose without consuming shield")
	}
}
func TestLegendaryUniversalExtendedEquipmentStats(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{{"TY-A10", 1200}, {"TY-D02", 1040}, {"TY-D03", 1030}, {"TY-D04", 1040}, {"TY-U09", 0}} {
		t.Run(tc.id, func(t *testing.T) {
			s, _, _, c := universalTestContext(t, tc.id)
			item := s.items[900001]
			item.Equip.Bonus = domain.Stats{MDef: 100, MaxHP: 100, MaxMP: 100}
			s.items[900001] = item
			st := domain.Stats{MDef: 1000, MaxHP: 1000, MaxMP: 1000, AtkSpeedMS: 1500}
			universalExtendedStats(c, &st)
			got := st.MDef
			switch tc.id {
			case "TY-A10":
				got = st.AtkSpeedMS
			case "TY-D03":
				got = st.MaxHP
			case "TY-D04":
				got = st.MaxMP
			case "TY-U09":
				got = st.MoveSpeed
				tc.want = universalPct(characterInnateStats(c.Owner.Player.Char).MoveSpeed, 20)
			}
			if got != tc.want {
				t.Fatalf("got%d want%d", got, tc.want)
			}
		})
	}
}
func TestLegendaryUniversalNativeBaseContributions(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{{"TY-A25", 50}, {"TY-A26", 50}, {"TY-A38", 40}, {"TY-A39", 20}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m, _ := universalTestContext(t, tc.id)
			item := s.items[900001]
			item.Equip.Bonus = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Def: 100}
			s.items[900001] = item
			p.MaxMP = 1000
			d := domain.SkillDef{ID: 123, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
			if tc.id == "TY-A26" {
				d.HurtType = domain.HurtFire
			}
			if tc.id == "TY-A39" {
				d.LegendaryBasic = true
			}
			s.legendaryPrepareSkill(p, &d, m.ID)
			if d.Effects[0].BaseDamageFlat != tc.want {
				t.Fatalf("base%d want%d", d.Effects[0].BaseDamageFlat, tc.want)
			}
		})
	}
}
func TestLegendaryUniversalLandingPullUsesNativeMovement(t *testing.T) {
	s, p, m, c := universalTestContext(t, "TY-A20")
	c.Skill = 123
	c.Def = domain.SkillDef{ID: 123, Effects: []domain.SkillEffect{{Kind: domain.EffectTeleportPoint}}}
	m.Pos.X = 250
	m.Pos.Y = 100
	s.aoi.Move(m, m.Pos)
	from := p.Pos
	from.X -= 50
	before := m.Pos
	universalLandingPull(c, from)
	if m.Pos.X >= before.X || m.Pos.X < 150 {
		t.Fatalf("pull position%+v", m.Pos)
	}
	stopped := m.Pos
	c.Skill = 0
	universalLandingPull(c, from)
	if m.Pos != stopped {
		t.Fatal("ordinary teleport generated pull")
	}
}
func TestLegendaryUniversalClassShapePrecedesGenericSingleConversion(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A02")
	warriorEquip(t, s, p, "TY-A05", domain.SlotGlove)
	p.Look.AtkDist = 100
	d := domain.SkillDef{ID: 10002, TargetEnemy: true, Kind: domain.SkillSingle, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.LegendaryModifiers.UniversalSingleCircle || d.Area.Center != domain.SkillAreaCenterCaster {
		t.Fatalf("generic replaced class shape: %+v", d.Area)
	}
}
func TestLegendaryUniversalNativeLifestealAndHealCombine(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A17")
	warriorEquip(t, s, p, "TY-U16", domain.SlotRing)
	d := domain.SkillDef{ID: 10103, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}, {Kind: domain.EffectRestoreHP, Target: domain.EffectTargetCaster, SourceAttackPct: 100, RequiresPreviousLanded: true}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Effects[1].SourceAttackPct != 220 {
		t.Fatalf("life+heal should add60+60, got%d", d.Effects[1].SourceAttackPct)
	}
}
func TestLegendaryUniversalBasicReachNativeAppearance(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A35")
	item := s.items[900001]
	item.Equip.Appearance = domain.EquipAppearance{Part: domain.AppearanceWeaponR, Known: true, AttackKnown: true, AtkDist: 100}
	s.items[900001] = item
	s.refreshAppearance(p)
	if p.Look.AtkDist != 150 || p.Player.Char.Appear.AtkDist != 100 {
		t.Fatalf("effective%d persisted%d", p.Look.AtkDist, p.Player.Char.Appear.AtkDist)
	}
	s.refreshAppearance(p)
	if p.Look.AtkDist != 150 {
		t.Fatal("refresh stacked reach")
	}
	p.HP = 0
	if got := s.classSetAppearance(p, p.Player.Char.Appear).AtkDist; got != 150 {
		t.Fatalf("dead/login projection%d", got)
	}
	p.HP = 10000
	warriorEquip(t, s, p, "TY-A01", domain.SlotGlove)
	d := domain.SkillDef{LegendaryBasic: true, Dist: int32(p.Look.AtkDist), Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Area.Radius != 150 {
		t.Fatalf("fan double range%d", d.Area.Radius)
	}
	st := p.Player.Worn.At(domain.SlotWeapon)
	st.Legendary = domain.LegendaryInstance{}
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.refreshAppearance(p)
	if p.Look.AtkDist != 100 {
		t.Fatalf("unequip retained reach%d", p.Look.AtkDist)
	}
}
func TestLegendaryUniversalDetectionUsesNativeRadius(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-U19")
	d := domain.SkillDef{ID: 123, Radius: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDetectTrap}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Radius != 200 {
		t.Fatalf("native detection radius%d", d.Radius)
	}
}
func TestLegendaryUniversalBasicRangeKeepsNativeWeaponIdentity(t *testing.T) {
	for _, tc := range []struct {
		native uint16
		ranged bool
	}{{120, false}, {200, true}} {
		s, p, _, _ := universalTestContext(t, "TY-A35")
		item := s.items[900001]
		item.Equip.Appearance = domain.EquipAppearance{Part: domain.AppearanceWeaponR, Known: true, AttackKnown: true, AtkDist: tc.native}
		s.items[900001] = item
		s.refreshAppearance(p)
		if p.Look.AtkDist != tc.native*3/2 || p.UsesRangedWeapon() != tc.ranged {
			t.Fatalf("native%d effective%d ranged%v", tc.native, p.Look.AtkDist, p.UsesRangedWeapon())
		}
	}
}
func TestLegendaryUniversalAttackSpeedUsesNativeCadence(t *testing.T) {
	s, p, _, c := universalTestContext(t, "TY-A10")
	p.Stats.AtkSpeedPct = 0
	p.Look.AtkDist = 100
	p.Player.Char.Appear.AtkDist = 100
	universalExtendedStats(c, &p.Stats)
	p.DidBasicAttack(s.tick)
	if p.ReadyToAttack(s.tick+domain.Ticks(700)) || !p.ReadyToAttack(s.tick+domain.Ticks(800)) {
		t.Fatal("20percent native cadence reduction not applied")
	}
}

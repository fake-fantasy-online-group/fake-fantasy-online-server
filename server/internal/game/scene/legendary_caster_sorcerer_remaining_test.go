package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func sorcererRemainingDamage(id domain.SkillID) domain.SkillDef {
	return domain.SkillDef{ID: id, Level: 1, Prof: 5, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 300, HurtType: domain.HurtFire, MPCost: 100, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
}
func sorcererRemainingReady(c *legendaryContext, d domain.StatusDef) {
	d.ID = 190001
	d.DurationSec = 30
	d.TypeKnown = true
	d.Type = domain.StatusGood
	c.Owner.Status.Apply(d, c.Scene.tick, c.Owner.ID)
}
func TestLegendaryCasterSorcererRemainingGeometryAndCost(t *testing.T) {
	for _, id := range []string{"SS-A03", "SS-A06", "SS-U03", "SS-U07", "SS-U10", "SS-D08"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			d := sorcererRemainingDamage(legendaryFireball)
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: 100}
			switch id {
			case "SS-A03":
				d.ID = legendaryFireBolt
				d.Area = domain.SkillAreaTargeting{}
				d.Kind = domain.SkillSingle
			case "SS-U07":
				c.Skill = legendaryFireball
				c.NowMS = s.tick.Millis()
				legendaryHandlers[id].RootFinished(c)
			case "SS-U10":
				c.Skill = 10432
				c.NowMS = s.tick.Millis()
				legendaryHandlers[id].Teleported(c, domain.Pos{})
			case "SS-U03":
				d.Statuses = []domain.StatusApplication{{ID: 1036}}
				sorcererRemainingReady(c, domain.StatusDef{NextStatusDurationPct: 100})
			}
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			switch id {
			case "SS-A03":
				if d.Area.Radius != 150 || d.SpecialWeaponTargetLimit != 0 {
					t.Fatalf("splash geometry %+v", d.Area)
				}
			case "SS-A06":
				if d.Area.Radius != 165 {
					t.Fatal(d.Area)
				}
			case "SS-U03":
				if d.MPCost != 45 {
					t.Fatal(d.MPCost)
				}
			case "SS-U07":
				if d.MPCost != 55 {
					t.Fatal(d.MPCost)
				}
			case "SS-D08":
				if d.MPCost != 80 {
					t.Fatal(d.MPCost)
				}
			case "SS-U10":
				if d.Dist != 600 {
					t.Fatal(d.Dist)
				}
			}
			// Removing the item must remove its definition modifier immediately.
			c.Owner.Player.Worn = &domain.EquipSet{}
			base := sorcererRemainingDamage(legendaryFireBolt)
			s.legendaryPrepareSkill(c.Owner, &base, c.Target.ID)
			if base.MPCost != 100 || base.Dist != 300 || base.Area.Valid() {
				t.Fatalf("unequipped modifier persists %+v", base)
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingTargetBonuses(t *testing.T) {
	for _, tc := range []struct {
		id        string
		skill     domain.SkillID
		near, far int32
	}{
		{"SS-A06", legendaryFireball, 120, 145}, {"SS-A08", legendaryIceArrow, 120, 155}, {"SS-A21", legendaryBlizzard, 180, 120}, {"SS-A14", legendaryIceBurst, 120, 120},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			native := sorcererRemainingDamage(tc.skill)
			native.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 300}
			native.Radius = 300
			s.skills[domain.SkillKey{ID: tc.skill, Level: 1}] = native
			finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, domain.EquipmentCombatStats{})
			defer finish()
			s.equipmentState().root.center = c.Owner.Pos
			for _, v := range []struct {
				x    float64
				want int32
			}{{130, tc.near}, {350, tc.far}} {
				s.aoi.Move(c.Target, domain.Pos{MapID: c.Owner.Pos.MapID, X: v.x, Y: 100})
				d := s.legendaryTargetDefinition(c.Owner, native, c.Target, c.Target.ID, []domain.EntityID{c.Target.ID})
				if d.DirectEffects()[0].DamagePct != v.want {
					t.Fatalf("at%v got%d want%d", v.x, d.DirectEffects()[0].DamagePct, v.want)
				}
			}
			wrong := sorcererRemainingDamage(10499)
			wrong = s.legendaryTargetDefinition(c.Owner, wrong, c.Target, c.Target.ID, []domain.EntityID{c.Target.ID})
			if wrong.DirectEffects()[0].DamagePct != 100 {
				t.Fatal("wrong skill boosted")
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingFocusedFireball(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A05")
	a := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
	b := putMonster(s, 902, 10000, domain.Stats{}, 170, 100)
	d := sorcererRemainingDamage(legendaryFireball)
	for _, tc := range []struct {
		ids     []domain.EntityID
		primary domain.EntityID
		want    int32
	}{{[]domain.EntityID{c.Target.ID}, c.Target.ID, 175}, {[]domain.EntityID{c.Target.ID, a.ID}, c.Target.ID, 175}, {[]domain.EntityID{c.Target.ID, a.ID, b.ID}, c.Target.ID, 135}, {[]domain.EntityID{c.Target.ID, a.ID, b.ID}, a.ID, 100}} {
		got := s.legendaryTargetDefinition(c.Owner, d, c.Target, tc.primary, tc.ids)
		if got.DirectEffects()[0].DamagePct != tc.want {
			t.Fatalf("targets%v primary%d damage%d", tc.ids, tc.primary, got.DirectEffects()[0].DamagePct)
		}
	}
	b.HP = 0
	got := s.legendaryTargetDefinition(c.Owner, d, c.Target, c.Target.ID, []domain.EntityID{c.Target.ID, a.ID, b.ID})
	if got.DirectEffects()[0].DamagePct != 175 {
		t.Fatal("dead target counted")
	}
}
func TestLegendaryCasterSorcererRemainingFrozenBonuses(t *testing.T) {
	for _, id := range []string{"SS-A09", "SS-A22"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			skill := legendaryIceArrow
			want := int32(200)
			if id == "SS-A22" {
				skill = legendaryBlizzard
				want = 190
			}
			d := sorcererRemainingDamage(skill)
			d.Statuses = []domain.StatusApplication{{ID: 10427, ChanceBP: 2500, ChanceExplicit: true}}
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if id == "SS-A09" && d.Statuses[0].ChanceBP != 10000 {
				t.Fatal("learned native slow not guaranteed")
			}
			got := s.legendaryTargetDefinition(c.Owner, d, c.Target, c.Target.ID, []domain.EntityID{c.Target.ID})
			if got.DirectEffects()[0].DamagePct != 100 {
				t.Fatal("no slow still boosted")
			}
			c.Target.Status.Apply(domain.StatusDef{ID: domain.StatusSlow, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad}, s.tick, c.Owner.ID)
			got = s.legendaryTargetDefinition(c.Owner, d, c.Target, c.Target.ID, []domain.EntityID{c.Target.ID})
			if got.DirectEffects()[0].DamagePct != want {
				t.Fatalf("slow damage%d", got.DirectEffects()[0].DamagePct)
			}
			if id == "SS-A09" {
				delete(c.Owner.Player.Char.Skills, 10427)
				got = s.legendaryTargetDefinition(c.Owner, d, c.Target, c.Target.ID, nil)
				if got.DirectEffects()[0].DamagePct != 100 {
					t.Fatal("unlearned mastery boosted")
				}
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingIcePenetration(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A10")
	for _, skill := range []domain.SkillID{legendaryIceArrow, legendaryFireBolt} {
		b := combat.Blow{SkillID: int32(skill), ElementReductionPenetrationBP: 1000}
		s.legendaryModifyBlow(c.Owner, c.Target, &b)
		want := int32(1000)
		if skill == legendaryIceArrow {
			want = 6000
		}
		if b.ElementReductionPenetrationBP != want {
			t.Fatal(b.ElementReductionPenetrationBP)
		}
	}
}
func TestLegendaryCasterSorcererRemainingSecretStateBonuses(t *testing.T) {
	for _, id := range []string{"SS-A29", "SS-D10", "SS-U03"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			build := func() domain.SkillDef {
				d := sorcererRemainingDamage(legendaryFireBolt)
				if id == "SS-U03" {
					d.Statuses = []domain.StatusApplication{{ID: 1036}}
				}
				return d
			}
			d := build()
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if d.DirectEffects()[0].DamagePct != 100 || d.MPCost != 100 {
				t.Fatal("without native ready state")
			}
			ready := domain.StatusDef{NextSkillDamagePct: 100}
			if id == "SS-U03" {
				ready = domain.StatusDef{NextStatusDurationPct: 100}
			}
			sorcererRemainingReady(c, ready)
			d = build()
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			switch id {
			case "SS-A29":
				if d.DirectEffects()[0].DamagePct != 166 {
					t.Fatal(d.DirectEffects())
				}
			case "SS-D10":
				if d.DirectEffects()[0].DamagePct != 135 {
					t.Fatal(d.DirectEffects())
				}
			case "SS-U03":
				if d.MPCost != 45 {
					t.Fatal(d.MPCost)
				}
			}
			if !c.Owner.Status.HasEffective(190001) {
				t.Fatal("preparation consumed native readiness")
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingPassives(t *testing.T) {
	for _, tc := range []struct {
		id        string
		hp, speed int32
	}{{"SS-D04", 1300, 100}, {"SS-D08", 1300, 100}, {"SS-U04", 1000, 100}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			st := domain.Stats{MaxHP: 1000, MoveSpeed: 100}
			s.legendaryPassiveStats(c.Owner, &st)
			if st.MaxHP != tc.hp || st.MoveSpeed != tc.speed {
				t.Fatal(st)
			}
			if tc.id == "SS-D04" {
				d := sorcererRemainingDamage(10432)
				d = s.legendaryTargetDefinition(c.Owner, d, c.Target, c.Target.ID, nil)
				if d.DirectEffects()[0].DamagePct != 130 {
					t.Fatal("phantom child modifier")
				}
			}
			c.Owner.Player.Worn = &domain.EquipSet{}
			st = domain.Stats{MaxHP: 1000, MoveSpeed: 100}
			s.legendaryPassiveStats(c.Owner, &st)
			if st.MaxHP != 1000 || st.MoveSpeed != 100 {
				t.Fatal("passive survived removal")
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingIncoming(t *testing.T) {
	for _, id := range []string{"SS-D01", "SS-D02", "SS-D05", "SS-D10"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			c.Owner.MaxHP = 10000
			hit := func(n int32) int32 {
				return s.legendaryBeforeDamage(c.Target, c.Owner, combat.Blow{}, event.DamageDealt{}, n, true)
			}
			if id != "SS-D10" && hit(1000) != 1000 {
				t.Fatal("missing native state still mitigates")
			}
			state := domain.StatusDef{SkillSnapshotID: 10425}
			if id == "SS-D02" {
				state.SkillSnapshotID = 10435
			}
			if id == "SS-D05" {
				state.NextStatusDurationPct = 100
			}
			sorcererRemainingReady(c, state)
			want := int32(720)
			switch id {
			case "SS-D02":
				want = 450
			case "SS-D05":
				want = 500
			case "SS-D10":
				want = 650
			}
			if hit(1000) != want || hit(1000) != want {
				t.Fatalf("incoming%d want%d or unexpected ICD", hit(1000), want)
			}
			if id == "SS-D01" {
				s.aoi.Move(c.Target, domain.Pos{MapID: c.Owner.Pos.MapID, X: 500, Y: 100})
				if hit(1000) != 1000 {
					t.Fatal("far attacker mitigated")
				}
			}
			if id == "SS-D02" && hit(800) != 800 {
				t.Fatal("8percent threshold must be strict")
			}
			if id == "SS-D05" && hit(10000) != 7800 {
				t.Fatal("22percent maxHP cap missing")
			}
		})
	}
}

func TestLegendaryCasterSorcererRemainingElementPassive(t *testing.T) {
	for _, tc := range []struct {
		hurt    int32
		passive domain.SkillID
	}{{domain.HurtFire, 10431}, {domain.HurtIce, 10424}} {
		t.Run(map[domain.SkillID]string{10431: "fire_receives_ice", 10424: "ice_receives_fire"}[tc.passive], func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-A25")
			s.skills[domain.SkillKey{ID: tc.passive, Level: 1}] = domain.SkillDef{ID: tc.passive, Level: 1, Prof: 5, PassiveModifiers: []domain.PassiveModifier{{Kind: domain.PassiveSkillDamagePct, Value: 37}}}
			c.Owner.Player.Char.Skills[tc.passive] = 1
			d := sorcererRemainingDamage(legendaryFireBolt)
			d.HurtType = tc.hurt
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if d.DirectEffects()[0].DamagePct != 137 {
				t.Fatalf("cross-element passive%d", d.DirectEffects()[0].DamagePct)
			}
			delete(c.Owner.Player.Char.Skills, tc.passive)
			d = sorcererRemainingDamage(legendaryFireBolt)
			d.HurtType = tc.hurt
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if d.DirectEffects()[0].DamagePct != 100 {
				t.Fatal("unlearned passive unlocked")
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingBurnSpread(t *testing.T) {
	for _, own := range []bool{true, false} {
		t.Run(map[bool]string{true: "own", false: "foreign"}[own], func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-A07")
			source := c.Owner.ID
			if !own {
				source = 99
			}
			burn := domain.StatusDef{ID: 1036, DurationSec: 10, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModeAbsolute}}}
			c.Target.Status.Apply(burn, s.tick, source)
			var ids []domain.EntityID
			for i := 0; i < 5; i++ {
				m := putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, float64(150+i*20), 100)
				ids = append(ids, m.ID)
			}
			finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, domain.EquipmentCombatStats{})
			root := s.equipmentState().root
			root.def = sorcererRemainingDamage(legendaryFireball)
			root.def.Radius = 300
			root.center = c.Target.Pos
			root.hits = []equipmentHit{{src: c.Owner.ID, dst: c.Target.ID, ev: event.DamageDealt{Amount: 100}}}
			finish()
			count := 0
			for _, id := range ids {
				if s.entities[id].HP != 10000 {
					count++
					if s.entities[id].HP != 9910 {
						t.Fatalf("spread must be90 each, got%d", 10000-s.entities[id].HP)
					}
				}
			}
			want := 0
			if own {
				want = 5
			}
			if count != want {
				t.Fatalf("spread count%d want%d", count, want)
			}
			if !c.Target.Status.HasEffective(1036) {
				t.Fatal("original burn consumed")
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingBurnOpening(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A19")
	burn := domain.StatusDef{ID: 1036, SkillSnapshotID: 10421, DurationSec: 10, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModeAbsolute}}}
	finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, domain.EquipmentCombatStats{})
	c.Target.Status.Apply(burn, s.tick, c.Owner.ID)
	before := c.Target.HP
	s.legendaryStatusCommitted(c.Owner, c.Target, &burn)
	s.legendaryStatusCommitted(c.Owner, c.Target, &burn)
	finish()
	if before-c.Target.HP != 65 {
		t.Fatalf("burn opening%d", before-c.Target.HP)
	}
	if legendaryCasterBurn(c, c.Target) != 100 {
		t.Fatal("opening shortened burn")
	}
}
func TestLegendaryCasterSorcererRemainingTeleportExplosion(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A27")
	base := sorcererRemainingDamage(legendaryFireBolt)
	s.skills[domain.SkillKey{ID: legendaryFireBolt, Level: 1}] = base
	before := c.Target.HP
	from := c.Owner.Pos
	s.legendaryCasterTeleported(c.Owner, from, 10432)
	if c.Target.HP != before {
		t.Fatal("failed move exploded")
	}
	s.aoi.Move(c.Owner, domain.Pos{MapID: from.MapID, X: 500, Y: 100})
	s.legendaryCasterTeleported(c.Owner, from, 10432)
	if before-c.Target.HP != 180 {
		t.Fatalf("departure explosion%d", before-c.Target.HP)
	}
	before = c.Target.HP
	s.legendaryCasterTeleported(c.Owner, from, 10499)
	if c.Target.HP != before {
		t.Fatal("unrelated teleport exploded")
	}
}
func TestLegendaryCasterSorcererRemainingControlBreakDoesNotRefund(t *testing.T) {
	for _, landed := range []bool{false, true} {
		t.Run(map[bool]string{true: "hit", false: "empty"}[landed], func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-U06")
			c.Owner.MP = 800
			c.Owner.Player.StartSkillCooldown(legendaryFireball, s.tick, 100)
			finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, domain.EquipmentCombatStats{})
			root := s.equipmentState().root
			root.def = sorcererRemainingDamage(legendaryFireball)
			root.paidMP = 100
			if landed {
				root.hits = []equipmentHit{{src: c.Owner.ID, dst: c.Target.ID, ev: event.DamageDealt{Amount: 100}}}
			}
			finish()
			want := int32(800)
			if c.Owner.MP != want {
				t.Fatalf("refund%d want%d", c.Owner.MP, want)
			}
			if c.Owner.Player.SkillReadyAt(legendaryFireball, s.tick+20) {
				t.Fatal("control-break power must not alter fireball cooldown")
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingMirrorSlow(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U08")
	slow := domain.StatusDef{ID: domain.StatusSlow, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad}
	c.Owner.Status.Apply(slow, s.tick, c.Target.ID)
	mirror := domain.StatusDef{ID: 10417, SkillSnapshotID: 10417, DurationSec: 10, TypeKnown: true, Type: domain.StatusGood}
	c.Owner.Status.Apply(mirror, s.tick, c.Owner.ID)
	s.legendarySkillResolved(c.Owner, c.Owner, domain.SkillDef{ID: 10417}, true)
	if c.Owner.Status.HasEffective(domain.StatusSlow) {
		t.Fatal("mirror did not cleanse")
	}
	if s.legendaryStatusReceiving(c.Owner, c.Target, &slow) {
		t.Fatal("mirror did not prevent new slow")
	}
	delete(c.Owner.Player.Char.Skills, 10417)
	s.syncLegendaryPermanentBuffs(c.Owner)
	c.Owner.Status.Remove(10417)
	if !s.legendaryStatusReceiving(c.Owner, c.Target, &slow) {
		t.Fatal("slow immunity outlived mirror")
	}
}

func TestLegendaryCasterSorcererRemainingSplashNativeCast(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A03")
	c.Owner.Player.Char.Skills = domain.Learned{legendaryFireBolt: 1}
	d := sorcererRemainingDamage(legendaryFireBolt)
	d.MPCost = 0
	s.skills[domain.SkillKey{ID: legendaryFireBolt, Level: 1}] = d
	for i := 0; i < 5; i++ {
		putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, float64(150+i*20), 100)
	}
	before := c.Target.HP
	legendaryCasterCastNow(t, s, c.Owner, legendaryFireBolt, c.Target.ID)
	if before-c.Target.HP != 100 {
		t.Fatalf("primary changed %d", before-c.Target.HP)
	}
	count := 0
	for i := 0; i < 5; i++ {
		m := s.entities[domain.EntityID(901+i)]
		if m.HP != 10000 {
			count++
			if m.HP != 9900 {
				t.Fatal("side not full base damage")
			}
		}
	}
	if count != 5 {
		t.Fatalf("side count%d want5, no recursive expansion", count)
	}
}
func TestLegendaryCasterSorcererRemainingIcePull(t *testing.T) {
	for _, landed := range []bool{true, false} {
		t.Run(map[bool]string{true: "landed", false: "no_hit"}[landed], func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-A14")
			s.aoi.Move(c.Target, domain.Pos{MapID: c.Owner.Pos.MapID, X: 280, Y: 100})
			before := c.Target.Pos
			finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, domain.EquipmentCombatStats{})
			root := s.equipmentState().root
			root.def = sorcererRemainingDamage(legendaryIceBurst)
			root.center = c.Owner.Pos
			if landed {
				root.hits = []equipmentHit{{src: c.Owner.ID, dst: c.Target.ID, ev: event.DamageDealt{Amount: 100}}}
			}
			finish()
			want := before.X
			if landed {
				want -= 90
			}
			if c.Target.Pos.X != want {
				t.Fatalf("pull got%v want%v", c.Target.Pos.X, want)
			}
		})
	}
}
func TestLegendaryCasterSorcererRemainingNativeRelationNegatives(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U10")
	d := sorcererRemainingDamage(legendaryIceBurst)
	d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
	s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
	if d.Dist != 300 || d.Area.Radius != 100 {
		t.Fatal("self centered range or radius changed")
	}
	s, c = legendaryCasterFixture(t, "SS-U03")
	sorcererRemainingReady(c, domain.StatusDef{NextStatusDurationPct: 100})
	d = sorcererRemainingDamage(legendaryFireBolt)
	s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
	if d.MPCost != 100 {
		t.Fatal("statusless cast consumes concentration cost bonus")
	}
	s, c = legendaryCasterFixture(t, "SS-A29")
	sorcererRemainingReady(c, domain.StatusDef{NextCastTimeReductionPct: 100})
	d = sorcererRemainingDamage(legendaryFireBolt)
	s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
	if d.DirectEffects()[0].DamagePct != 100 {
		t.Fatal("already instant skill borrowed cast readiness")
	}
	d = sorcererRemainingDamage(legendaryFireball)
	d.PrepareMS = 1000
	s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
	if d.DirectEffects()[0].DamagePct != 166 {
		t.Fatal("cast readiness bonus missing")
	}
}

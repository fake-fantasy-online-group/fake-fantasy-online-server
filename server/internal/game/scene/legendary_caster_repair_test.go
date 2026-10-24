package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
	"time"
)

func TestCasterRepairNativeControlSnapshots(t *testing.T) {
	for _, power := range []string{"SS-A12", "SS-A15", "SS-A22"} {
		t.Run(power+" preserves existing freeze", func(t *testing.T) {
			skill := legendaryIceBurst
			if power == "SS-A22" {
				skill = legendaryBlizzard
			}
			s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{skill: 1}, power)
			old := domain.StatusDef{ID: domain.StatusFreeze, Level: 5, DurationSec: 40, TypeKnown: true, Type: domain.StatusBad, BreakOnDamage: true, SourceSkillID: 10428, SkillSnapshotID: 10428}
			m.Status.Apply(old, s.tick, p.ID)
			before := m.HP
			nativeRepairCast(t, s, p, skill, m.ID)
			if m.HP >= before {
				t.Fatal("preserved freeze incorrectly prevented damage")
			}
			found := false
			m.Status.EachEffective(func(a domain.Active) {
				if a.Def.ID == domain.StatusFreeze && a.Def.SkillSnapshotID == 10428 && a.ExpireAt == s.tick+domain.Ticks(40000) {
					found = true
				}
			})
			if !found {
				t.Fatal("original freeze removed or shortened")
			}
		})
	}
	run := func(preexisting bool) int32 {
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10410: 1}, "SS-A15")
		slow := domain.StatusDef{ID: 10427, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -20, Mode: domain.ModePercent}}}
		if preexisting {
			m.Status.Apply(slow, s.tick, p.ID)
		}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10410, Target: m.ID})
		if len(s.casts) == 0 {
			t.Fatal("ice burst rejected")
		}
		if preexisting {
			m.Status.Remove(10427)
		} else {
			m.Status.Apply(slow, s.tick, p.ID)
		}
		before := m.HP
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		return before - m.HP
	}
	without, with := run(false), run(true)
	if with*100 < without*159 || with*100 > without*161 {
		t.Fatal("A15 did not use accepted-cast native slow snapshot", without, with)
	}
}

func TestCasterRepairGuaranteedStatusRetainsImmunity(t *testing.T) {
	for _, tc := range []struct {
		power                  string
		skill, status, passive int32
	}{{"SS-A09", 10409, 10427, 10427}, {"SS-A17", 10405, 1036, 10421}} {
		t.Run(tc.power, func(t *testing.T) {
			for _, immune := range []bool{false, true} {
				s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{domain.SkillID(tc.skill): 1, domain.SkillID(tc.passive): 1}, tc.power)
				m.Stats.StatusResist = 1000000
				if immune {
					m.Status.Apply(domain.StatusDef{ID: 199998, DurationSec: 60, BlockHarmful: true}, s.tick, m.ID)
				}
				nativeRepairCast(t, s, p, domain.SkillID(tc.skill), m.ID)
				if got := m.Status.HasEffective(domain.StatusID(tc.status)); got == immune {
					t.Fatalf("immune=%v status applied=%v", immune, got)
				}
			}
		})
	}
}

func TestCasterRepairNewBurnOnlyAndElementalSharing(t *testing.T) {
	run := func(powers ...string) (int32, int32, int64) {
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10421: 1, 10431: 1}, powers...)
		s.statusOverlay = domain.StatusOverlay{{Existing: 1036, Incoming: 1036}: domain.OverlayCoexist}
		before := m.HP
		nativeRepairCast(t, s, p, 10421, m.ID)
		first := before - m.HP
		// Advance the ordinary cooldown, without progressing status ticks.
		s.tick += domain.Ticks(6000)
		p.MP = p.MaxMP
		before = m.HP
		nativeRepairCast(t, s, p, 10421, m.ID)
		second := before - m.HP
		total := int64(0)
		m.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == 1036 {
				total += s.legendaryPeriodicAmount(a, -1000)
			}
		})
		return first, second, total
	}
	first, second, _ := run("SS-A19")
	if first <= 0 || first != second {
		t.Fatal("new burn re-settled older instances", first, second)
	}
	baseline, _, baseTick := run("SS-A19")
	shared, _, sharedTick := run("SS-A19", "SS-A25")
	if shared <= baseline || sharedTick >= baseTick {
		t.Fatal("cross-element passive missing from burn and settlement", baseline, shared, baseTick, sharedTick)
	}
}

func TestCasterRepairHealerInterruptsMonsterAI(t *testing.T) {
	for _, immune := range []bool{false, true} {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10312: 1}, "YS-A12")
		md := domain.MonsterDef{ID: m.Monster.TypeID, Kind: domain.MonsterNormal, Family: domain.MonsterFamilyUndead}
		if immune {
			md.ImmuneSkills = map[domain.SkillID]struct{}{10312: {}}
		}
		s.defs = spawn.MapDefs{m.Monster.TypeID: md}
		m.Monster.SkillReady = map[domain.SkillID]domain.Tick{}
		m.Monster.SkillCasts = map[domain.SkillID]int32{}
		rule := domain.MonsterSkillRule{Skill: domain.SkillDef{ID: 900999, Level: 1, Kind: domain.SkillSingle, PrepareMS: 2000, Dist: 500, TargetEnemy: true, HurtType: domain.HurtHoly, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}}
		s.startMonsterSkill(m, rule, []domain.EntityID{p.ID})
		serial, deadline := m.Monster.CastSerial, m.Monster.CastingUntil
		before := m.HP
		nativeRepairCast(t, s, p, 10312, m.ID)
		if m.HP >= before {
			t.Fatal("control immunity suppressed native exorcism damage", immune)
		}
		if immune {
			if m.Monster.CastSerial != serial || m.Monster.CastingSkill == 0 {
				t.Fatal("control immune cast interrupted")
			}
		} else {
			if m.Monster.CastSerial == serial || m.Monster.CastingSkill != 0 {
				t.Fatal("actual monster AI windup survived")
			}
			s.tick = deadline
			s.timer.advance(s.tick)
			if len(s.casts) != 0 || m.Monster.SkillCasts[900999] != 0 {
				t.Fatal("cancelled AI timer resurrected cast")
			}
		}
	}
}

func TestCasterRepairHealerShackleAndHolyGeometry(t *testing.T) {
	for _, powers := range [][]string{{"YS-A18"}, {"YS-A19"}, {"YS-A18", "YS-A19"}} {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10325: 1}, powers...)
		before := m.HP
		nativeRepairCast(t, s, p, 10325, m.ID)
		damage := before - m.HP
		slow := int32(0)
		m.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == domain.StatusSlow {
				for _, aff := range a.Def.Affixes {
					if aff.Attr == domain.AttrAtkSpeed {
						slow = aff.Value
					}
				}
			}
		})
		if len(powers) == 1 && powers[0] == "YS-A18" {
			if damage != 0 || slow != 45 {
				t.Fatal("native shackle or explicit slow boost changed", damage, slow)
			}
		}
		if len(powers) == 1 && powers[0] == "YS-A19" {
			if damage != 200 || slow != 30 {
				t.Fatal("base A19 shackle", damage, slow)
			}
		}
		if len(powers) == 2 && (damage != 320 || slow != 45) {
			t.Fatal("A18 must amplify A19 direct damage and native attack slow", damage, slow)
		}
	}
	for _, combo := range []bool{false, true} {
		powers := []string{"YS-A29"}
		if combo {
			powers = append(powers, "YS-A06")
		}
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10317: 1}, powers...)
		p.Status.Apply(domain.StatusDef{ID: 1027, DurationSec: 20, NextSkillDamagePct: 40}, s.tick, p.ID)
		d, _ := s.skills.Get(10317, 1)
		s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
		s.legendaryPrepareSkill(p, &d, m.ID)
		want := int32(100)
		if combo {
			want = 300
		}
		if d.Area.Shape != domain.SkillAreaCircle || d.Area.Radius != want {
			t.Fatal("holy shape/flat range lost", combo, d.Area)
		}
		side := putMonster(s, 901, 100000, domain.Stats{}, m.Pos.X+float64(want)-1, 100)
		outside := putMonster(s, 902, 100000, domain.Stats{}, m.Pos.X+float64(want)+1, 100)
		nativeRepairCast(t, s, p, 10317, m.ID)
		if side.HP == 100000 || outside.HP != 100000 {
			t.Fatal("final radius not consumed", combo, side.HP, outside.HP)
		}
	}
}

func TestCasterRepairFireballCountsOnlyLegalHits(t *testing.T) {
	run := func(extra bool) int32 {
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10402: 1}, "SS-A05")
		if extra {
			for i := 0; i < 2; i++ {
				x := putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, 140+float64(i)*10, 100)
				x.Monster.TypeID = 999
			}
			s.defs = spawn.MapDefs{999: {ID: 999, Kind: domain.MonsterNormal, ImmuneSkills: map[domain.SkillID]struct{}{10402: {}}}}
		}
		before := m.HP
		nativeRepairCast(t, s, p, 10402, m.ID)
		return before - m.HP
	}
	if alone, withImmune := run(false), run(true); alone != withImmune {
		t.Fatal("skill-immune bystanders diluted the actual one-hit bonus", alone, withImmune)
	}
}

func TestCasterRepairFlameWidthAndNativeBurnCombination(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10405: 1, 10421: 1}, "SS-A18", "SS-A17", "SS-A16")
	original, _ := s.skills.Get(10405, 1)
	d := original
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
	s.legendaryPrepareSkill(p, &d, m.ID)
	if original.Area.Shape != domain.SkillAreaCircle || d.Area.Shape != original.Area.Shape || d.Area.Radius != original.Area.Radius*180/100 {
		t.Fatal("native circular width did not expand by 80%", original.Area, d.Area)
	}
	inside := putMonster(s, 901, 100000, domain.Stats{}, m.Pos.X+float64(d.Area.Radius)-1, m.Pos.Y)
	outside := putMonster(s, 902, 100000, domain.Stats{}, m.Pos.X+float64(d.Area.Radius)+1, m.Pos.Y)
	nativeRepairCast(t, s, p, 10405, m.ID)
	if inside.HP == 100000 || !inside.Status.HasEffective(1036) || outside.HP != 100000 {
		t.Fatal("wide flame/burn/settlement combination lost real geometry", inside.HP, outside.HP)
	}
}

func TestCasterRepairRangeThresholdAndWideAreaComposition(t *testing.T) {
	run := func(extended bool) int32 {
		powers := []string{"SS-A08"}
		if extended {
			powers = append(powers, "SS-U10")
		}
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10409: 1}, powers...)
		pos := m.Pos
		pos.X = p.Pos.X + 300
		s.aoi.Move(m, pos)
		before := m.HP
		nativeRepairCast(t, s, p, 10409, m.ID)
		return before - m.HP
	}
	normal, extended := run(false), run(true)
	if normal <= extended {
		t.Fatal("50% threshold ignored effective range", normal, extended)
	}
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10405: 1}, "SS-A18", "TY-A04")
	d, _ := s.skills.Get(10405, 1)
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Area.Radius != 330 {
		t.Fatal("native width and general radius must share additive geometry bucket", d.Area)
	}
	inside := putMonster(s, 901, 100000, domain.Stats{}, m.Pos.X+329, m.Pos.Y)
	outside := putMonster(s, 902, 100000, domain.Stats{}, m.Pos.X+331, m.Pos.Y)
	nativeRepairCast(t, s, p, 10405, m.ID)
	if inside.HP == 100000 || outside.HP != 100000 {
		t.Fatal("combined native range boundary", inside.HP, outside.HP)
	}
}

func TestCasterRepairPullSkipsImmuneBeforeLimit(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10410: 1}, "SS-A14")
	pos := m.Pos
	pos.X = 170
	s.aoi.Move(m, pos)
	ordinaryBefore := m.Pos
	// The farther five boss targets all land damage, but must not occupy the
	// five displacement-eligible slots. Separate angles avoid collision artifacts.
	for i, pos := range []domain.Pos{{X: 195, Y: 100}, {X: 130, Y: 190}, {X: 40, Y: 170}, {X: 20, Y: 70}, {X: 110, Y: 5}} {
		target := putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, pos.X, pos.Y)
		target.Monster.Kind = domain.MonsterBoss
	}
	nativeRepairCast(t, s, p, 10410, m.ID)
	if m.Pos == ordinaryBefore || sqDist(m.Pos, p.Pos) >= sqDist(ordinaryBefore, p.Pos) {
		t.Fatal("immune bosses consumed displacement cap", m.Pos, ordinaryBefore)
	}
	for id, target := range s.entities {
		if id >= 901 && target.Monster != nil && target.HP == 100000 {
			t.Fatal("immune displacement incorrectly prevented direct damage", id)
		}
	}
}

func TestCasterRepairIceSettlementInheritsOnlyPurePenetration(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10409: 1}, "SS-A10", "SS-A11")
	b := combat.Blow{SkillID: 10409, HurtType: domain.HurtIce, Magic: true, LegendaryPayload: true, Origin: domain.DamageProcOrigin, ProcDepth: 1}
	s.legendaryModifyBlow(p, m, &b)
	if b.ElementReductionPenetrationBP != 5000 {
		t.Fatal("ice-derived settlement lost penetration", b)
	}
	b = combat.Blow{SkillID: 10401, HurtType: domain.HurtFire, Magic: true, LegendaryPayload: true, Origin: domain.DamageProcOrigin, ProcDepth: 1}
	s.legendaryModifyBlow(p, m, &b)
	if b.ElementReductionPenetrationBP != 0 {
		t.Fatal("unrelated payload acquired ice penetration", b)
	}
	m.Status.Apply(domain.StatusDef{ID: 1036, DurationSec: 20, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -10}}}, s.tick, p.ID)
	before := m.HP
	nativeRepairCast(t, s, p, 10409, m.ID)
	if before-m.HP <= 200 || !m.Status.HasEffective(1036) {
		t.Fatal("native ice arrow lost independent burn settlement", before, m.HP)
	}
}

// Final-shape predicates must run after a charged Holy Light gains geometry.
// In both cases use the ordinary accepted cast and native impact settlement.
func TestCasterRepairChargedHolyFinalShapeComposition(t *testing.T) {
	for _, tc := range []struct {
		name   string
		powers []string
		radius int32
	}{
		{"new circle plus universal radius", []string{"YS-A29", "TY-A04"}, 140},
		{"class circle flat range plus universal radius", []string{"YS-A29", "YS-A06", "TY-A04"}, 420},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10317: 1}, tc.powers...)
			p.Status.Apply(domain.StatusDef{ID: 1027, DurationSec: 20, NextSkillDamagePct: 40}, s.tick, p.ID)
			inside := putMonster(s, 901, 100000, domain.Stats{}, m.Pos.X+float64(tc.radius)-1, m.Pos.Y)
			outside := putMonster(s, 902, 100000, domain.Stats{}, m.Pos.X+float64(tc.radius)+1, m.Pos.Y)
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10317, Target: m.ID})
			if len(s.casts) == 0 {
				t.Fatal("charged Holy Light rejected")
			}
			d := s.casts[0].def
			if d.Area.Shape != domain.SkillAreaCircle || d.Area.Radius != tc.radius {
				t.Fatalf("final shape not composed before radius: %+v", d.Area)
			}
			s.stepCastsAt(s.now().Add(time.Minute), false)
			s.stepCastsAt(s.now().Add(2*time.Minute), false)
			if inside.HP == 100000 || outside.HP != 100000 {
				t.Fatal("final area boundary not consumed", inside.HP, outside.HP)
			}
		})
	}
}

func TestCasterRepairChargedHolyRectangleComposition(t *testing.T) {
	for _, widen := range []bool{false, true} {
		powers := []string{"YS-A29", "YS-A08"}
		width := int32(260)
		if widen {
			powers = append(powers, "TY-A06")
			width = 416
		}
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10317: 1}, powers...)
		p.Status.Apply(domain.StatusDef{ID: 1027, DurationSec: 20, NextSkillDamagePct: 40}, s.tick, p.ID)
		longitudinalInside := putMonster(s, 901, 100000, domain.Stats{}, p.Pos.X+699, p.Pos.Y)
		longitudinalOutside := putMonster(s, 902, 100000, domain.Stats{}, p.Pos.X+701, p.Pos.Y)
		lateralInside := putMonster(s, 903, 100000, domain.Stats{}, p.Pos.X+200, p.Pos.Y+float64(width)/2-1)
		lateralOutside := putMonster(s, 904, 100000, domain.Stats{}, p.Pos.X+200, p.Pos.Y+float64(width)/2+1)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10317, Target: m.ID})
		if len(s.casts) == 0 {
			t.Fatal("charged rectangular Holy Light rejected")
		}
		d := s.casts[0].def
		if d.Area.Shape != domain.SkillAreaRectangle || d.Area.Center != domain.SkillAreaCenterCaster || d.Area.Length != 700 || d.Area.Width != width {
			t.Fatal("flat geometry or late percentage lost", widen, d.Area)
		}
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		if longitudinalInside.HP == 100000 || lateralInside.HP == 100000 || longitudinalOutside.HP != 100000 || lateralOutside.HP != 100000 {
			t.Fatal("rectangle boundaries not consumed", widen, longitudinalInside.HP, lateralInside.HP, longitudinalOutside.HP, lateralOutside.HP)
		}
	}
}

func TestCasterRepairGrantedLightWithoutLearningBolt(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, "YS-A23", "YS-A01", "YS-A02")
	p.HP = p.MaxHP - 1000
	for i := 0; i < 6; i++ {
		putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, m.Pos.X+float64(i+1)*10, m.Pos.Y)
	}
	nativeRepairCast(t, s, p, 10301, 0)
	hits := 0
	for _, target := range s.entities {
		if target.Monster != nil && target.HP < 100000 {
			hits++
		}
	}
	if m.HP == 100000 || hits != 4 {
		t.Fatal("granted rank-one light bolt missing or split cap bypassed", m.HP, hits)
	}
}

// U02 and U08 are both weapon-only powers; enhanced weapon1324 has native
// level requirement38, below U08's51. Do not fabricate an unattainable transfer
// combination or treat direct status injection as a player-reachable repair.
func TestCasterRepairNativeMirrorAcquisitionCleansesExistingSlow(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Warlock, domain.Learned{10417: 1}, "SS-U08")
	slow := domain.StatusDef{ID: 10427, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -20, Mode: domain.ModePercent}}}
	p.Status.Apply(slow, s.tick, p.ID)
	nativeRepairCast(t, s, p, 10417, 0)
	if p.Status.HasEffective(10427) || !legendaryCasterHasSkill(p, 10417) {
		t.Fatal("mirror failed to cleanse original native slow")
	}
}

func TestCasterRepairFireBasisNoLearningPrerequisite(t *testing.T) {
	for _, power := range []string{"SS-A27", "SS-A28"} {
		for _, pieces := range []int{0, 2, 4} {
			for _, triple := range []bool{false, true} {
				amounts := map[int32]int32{}
				for _, rank := range []int32{0, 1, 5} {
					learned := domain.Learned{10418: 1, 10432: 1, 10433: 5, 10409: 1}
					if rank > 0 {
						learned[10401] = rank
					}
					powers := []string{power}
					if power == "SS-A27" {
						powers = append(powers, "SS-U01")
						if triple {
							powers = append(powers, "SS-D04")
						}
					} else {
						powers = append(powers, "SS-U02")
						if triple {
							powers = append(powers, "SS-D03")
						}
					}
					s, p, m := nativeRepairFixture(t, domain.Warlock, learned, powers...)
					if pieces > 0 {
						nativeRepairSet(t, s, p, 3, pieces)
						// Activate the ordinary two-piece fire-damage benefit.
						nativeRepairCast(t, s, p, 10409, m.ID)
					}
					before := m.HP
					if power == "SS-A27" {
						inside := putMonster(s, 901, 100000, domain.Stats{}, p.Pos.X+179, p.Pos.Y)
						outside := putMonster(s, 902, 100000, domain.Stats{}, p.Pos.X+181, p.Pos.Y)
						from := p.Pos
						to := domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 200, Y: p.Pos.Y}
						s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
						if len(s.casts) == 0 {
							t.Fatal("legal aux teleport rejected", rank, s.outbox)
						}
						s.stepCastsAt(s.now().Add(time.Minute), false)
						s.stepCastsAt(s.now().Add(2*time.Minute), false)
						if p.Pos == from || inside.HP == 100000 || outside.HP != 100000 {
							t.Fatal("native departure explosion failed or moved center", rank, p.Pos, inside.HP, outside.HP)
						}
					} else {
						// Real native shield transfers before A28 counts the owner's
						// defensive kinds; it is not a custom invented buff.
						shield, ok := s.statuses.Get(1037, 1)
						if !ok {
							t.Fatal("native fire shield unavailable")
						}
						m.Status.Apply(shield, s.tick, m.ID)
						nativeRepairCast(t, s, p, 10433, m.ID)
						if !p.Status.HasEffective(1037) || m.Status.HasEffective(1037) {
							t.Fatal("native defensive status not stolen before dispel damage", rank)
						}
					}
					amounts[rank] = before - m.HP
					// Native Fire Bolt has coefficient 208% at rank one and
					// 240% at rank five; this fixture has exactly 100 MAtk.
					base := map[int32]int32{0: 208, 1: 208, 5: 240}[rank]
					want := base * 270 / 100 // 220% plus one newly stolen defense kind.
					if power == "SS-A27" {
						want = base * 180 / 100
						if triple {
							want = want * 130 / 100
						}
						if pieces >= 2 {
							want = want * 150 / 100
						}
					}
					if amounts[rank] != want {
						t.Fatal("borrowed basis gained wrong tags or repeated set payload", power, rank, pieces, triple, amounts[rank], want)
					}
					if p.Player.Char.Skills[10401] != rank {
						t.Fatal("borrowed basis unlocked or changed active Fire Bolt", rank)
					}
					if rank == 0 {
						s.onUseSkill(UseSkill{ID: p.ID, Skill: 10401, Target: m.ID})
						if len(s.casts) != 0 {
							t.Fatal("unlearned Fire Bolt became castable")
						}
					}
				}
				if amounts[0] <= 0 || amounts[0] != amounts[1] || amounts[5] <= amounts[1] {
					t.Fatal("granted basis did not preserve rank-one fallback/learned rank", power, pieces, triple, amounts)
				}
			}
		}
	}
}

func TestCasterRepairDispelGrantedBasisWithoutSuccessfulDispel(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		powers := []string{}
		if enabled {
			powers = append(powers, "SS-A28")
		}
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10418: 1, 10432: 1, 10433: 5}, powers...)
		nativeRepairCast(t, s, p, 10433, m.ID)
		want := int32(0)
		if enabled {
			want = 208 * 220 / 100
		}
		if got := 100000 - m.HP; got != want {
			t.Fatal("dispel without removable status damage", enabled, got, want)
		}
	}
}

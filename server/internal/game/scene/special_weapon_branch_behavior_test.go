package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"testing"
)

func TestSpecialWeaponResourceBranchActualSettlement(t *testing.T) {
	for _, tc := range []struct {
		item  domain.ItemID
		skill domain.SkillID
		prof  int32
		kind  domain.EffectKind
		want  int32
	}{{1023, 10104, 2, domain.EffectRestoreHP, 160}, {1523, 10119, 2, domain.EffectRestoreMP, 50}, {1320, 10301, 4, domain.EffectHeal, 130}} {
		t.Run(fmt.Sprintf("%d/prof%d", tc.item, tc.prof), func(t *testing.T) {
			for _, active := range []bool{false, true} {
				s, p, m := specialWeaponActors(t)
				p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
				p.MaxHP, p.HP = 10000, 100
				p.MaxMP, p.MP = 10000, 100
				effect := domain.SkillEffect{Kind: tc.kind, SourceAttackPct: 100}
				if tc.kind == domain.EffectHeal {
					effect = domain.SkillEffect{Kind: tc.kind, HealFlat: 100}
				}
				d := domain.SkillDef{ID: tc.skill, Prof: tc.prof, TargetSelf: true, Effect: effect}
				if active {
					applySpecialWeaponDefinition(tc.item, &d)
				}
				s.applySkill(p, p, d, 0, false)
				got := p.HP - 100
				if tc.kind == domain.EffectRestoreMP {
					got = p.MP - 100
				}
				want := int32(100)
				if active {
					want = tc.want
				}
				if got != want {
					t.Fatalf("active=%v actual restored=%d want%d target=%d", active, got, want, m.ID)
				}
			}
		})
	}
}

func TestSpecialWeaponSwordOverflowNativeRoot(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, p, m := swordsmanCoverageFixture(t, "JK-A01", false)
		s.refreshEntityStats(p)
		p.MP = p.MaxMP - 10
		p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
		d := swordDualDef(10119)
		d.Prof = 2
		if active {
			applySpecialWeaponDefinition(1522, &d)
		}
		swordCast(t, s, p, m, d)
		shield := int32(0)
		p.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == 1800101522 {
				shield += a.ShieldLeft
				if a.Def.DurationSec != 5 {
					t.Fatal("duration", a.Def.DurationSec)
				}
			}
		})
		want := int32(0)
		if active {
			want = 5
		}
		if shield != want || p.MP != p.MaxMP {
			t.Fatalf("active=%v shield=%d want%d MP=%d", active, shield, want, p.MP)
		}
	}
}

func TestSpecialWeaponBurnBranchesRouteOwnBurn(t *testing.T) {
	for _, tc := range []struct {
		item    domain.ItemID
		skill   domain.SkillID
		pct     int32
		consume bool
	}{{1322, 10401, 60, true}, {1325, 10405, 100, false}} {
		t.Run(fmt.Sprintf("%d/prof5", tc.item), func(t *testing.T) {
			for _, active := range []bool{false, true} {
				s, p, m := specialWeaponActors(t)
				burn := domain.StatusDef{ID: 1036, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, IntervalSec: 2, HPChanges: []domain.ResourceChange{{Value: -100}}}
				m.Status.Apply(burn, s.tick, p.ID)
				d := domain.SkillDef{ID: tc.skill, Prof: 5, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, HurtType: domain.HurtFire}
				if active {
					applySpecialWeaponDefinition(tc.item, &d)
				}
				before := m.HP
				s.specialWeaponAfterHit(p, m, d, true, false)
				want := int32(0)
				if active {
					want = 500 * tc.pct / 100
				}
				if before-m.HP != want {
					t.Fatalf("active=%v remaining burn=%d want%d", active, before-m.HP, want)
				}
				exists := m.Status.Has(1036)
				if exists == (active && tc.consume) {
					t.Fatalf("wrong burn retention active=%v exists=%v", active, exists)
				}
			}
		})
	}
}

func TestSpecialWeaponRemoteGroupHealActualRecipients(t *testing.T) {
	t.Run("1324/prof4", func(t *testing.T) {
		for _, active := range []bool{false, true} {
			s, p, _ := specialWeaponActors(t)
			join(t, s, 2, 101, "far anchor", 350, 100)
			join(t, s, 3, 102, "far neighbour", 360, 100)
			join(t, s, 4, 103, "near caster", 110, 100)
			for _, id := range []domain.EntityID{1, 2, 3, 4} {
				a := s.players[id]
				a.Player.Party = 7
				a.MaxHP = 10000
				a.HP = 1000
			}
			d := domain.SkillDef{ID: 10304, Prof: 4, Kind: domain.SkillArea, Dist: 100, TargetSelf: true, TargetTeam: true, Radius: 50, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 50}, Effect: domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 100}}
			if active {
				applySpecialWeaponDefinition(1324, &d)
			}
			ids := s.resolveTargets(p, d, 2, nil)
			for _, id := range ids {
				s.applySkill(p, s.entities[id], d, 0, false)
			}
			for _, id := range []domain.EntityID{1, 2, 3, 4} {
				want := int32(1000)
				remote := id == 2 || id == 3
				if remote == active {
					want = 1100
				}
				if s.players[id].HP != want {
					t.Fatalf("active=%v recipient=%d HP=%d want%d targets=%v", active, id, s.players[id].HP, want, ids)
				}
			}
			if active && d.Dist != 300 {
				t.Fatal("remote cast range", d.Dist)
			}
		}
	})
}

func TestSpecialWeaponRemovedNativeEffects(t *testing.T) {
	for _, tc := range []struct {
		item    domain.ItemID
		skill   domain.SkillID
		prof    int32
		removed domain.EffectKind
	}{{1021, 10102, 2, domain.EffectKnockback}, {1925, 10223, 3, domain.EffectKnockback}, {1527, 10123, 2, domain.EffectInstantDeath}} {
		t.Run(fmt.Sprintf("%d/prof%d", tc.item, tc.prof), func(t *testing.T) {
			for _, active := range []bool{false, true} {
				d := domain.SkillDef{ID: tc.skill, Prof: tc.prof, TargetEnemy: true, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}, {Kind: tc.removed, ChanceBP: 10000, ChanceExplicit: true, KnockbackDistance: 40}}}
				if active {
					applySpecialWeaponDefinition(tc.item, &d)
				}
				removedCount := 0
				damageCount := 0
				for _, effect := range d.DirectEffects() {
					if effect.Kind == tc.removed {
						removedCount++
					}
					if effect.Kind == domain.EffectDamage {
						damageCount++
					}
				}
				want := 1
				if active {
					want = 0
				}
				if removedCount != want || damageCount != 1 {
					t.Fatalf("active=%v removed-kind=%d damage=%d", active, removedCount, damageCount)
				}
			}
		})
	}
}

// Each definition multiplier must reach real damage settlement; identity or
// DamagePct inspection alone would not detect a lost dispatch/consumer.
func TestSpecialWeaponDamageBranchActualSettlement(t *testing.T) {
	for _, tc := range []struct {
		item  domain.ItemID
		skill domain.SkillID
		prof  int32
		want  int32
	}{
		{1021, 10102, 2, 130}, {1022, 10103, 2, 80}, {1023, 10104, 2, 90},
		{1520, 10117, 2, 115}, {1521, 10111, 2, 65}, {1523, 10119, 2, 135}, {1524, 10121, 2, 90},
		{1620, 10005, 1, 110}, {1621, 10017, 1, 115}, {1627, 10022, 1, 135},
		{1820, 10219, 3, 70}, {1826, 10209, 3, 65}, {1827, 10209, 3, 135},
		{1922, 10221, 3, 70}, {1923, 10205, 3, 130}, {1925, 10223, 3, 130}, {1926, 10206, 3, 120}, {1928, 10216, 3, 140},
		{1320, 10409, 5, 130}, {1321, 10410, 5, 90}, {1326, 10412, 5, 125},
	} {
		t.Run(fmt.Sprintf("%d/prof%d", tc.item, tc.prof), func(t *testing.T) {
			for _, active := range []bool{false, true} {
				s, p, m := specialWeaponActors(t)
				p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
				d := domain.SkillDef{ID: tc.skill, Prof: tc.prof, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
				if active {
					applySpecialWeaponDefinition(tc.item, &d)
				}
				before := m.HP
				if !s.applySkill(p, m, d, 0, false) {
					t.Fatal("damage not applied")
				}
				want := int32(100)
				if active {
					want = tc.want
				}
				if got := before - m.HP; got != want {
					t.Fatalf("active=%v actual damage=%d want%d", active, got, want)
				}
			}
		})
	}
}

func TestSpecialWeaponConditionalDamageActualSettlement(t *testing.T) {
	for _, tc := range []struct {
		item            domain.ItemID
		skill           domain.SkillID
		threshold, want int32
	}{{1028, 10116, 50, 145}, {1527, 10123, 30, 180}} {
		t.Run(fmt.Sprintf("%d/prof2", tc.item), func(t *testing.T) {
			for _, below := range []bool{false, true} {
				s, p, m := specialWeaponActors(t)
				p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
				m.MaxHP = 10000
				m.HP = tc.threshold * 100
				if below {
					m.HP--
				}
				d := domain.SkillDef{ID: tc.skill, Prof: 2, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
				applySpecialWeaponDefinition(tc.item, &d)
				before := m.HP
				s.applySkill(p, m, d, 0, false)
				want := int32(100)
				if below {
					want = tc.want
				}
				if got := before - m.HP; got != want {
					t.Fatalf("below=%v damage=%d want%d", below, got, want)
				}
			}
		})
	}
}

func TestSpecialWeaponControlStatusActualApplication(t *testing.T) {
	for _, tc := range []struct {
		item      domain.ItemID
		skill     domain.SkillID
		prof, pct int32
	}{{1024, 10111, 2, 50}, {1025, 10113, 2, 30}, {1927, 10224, 3, 50}, {1823, 10208, 3, 50}, {1026, 10107, 2, 20}} {
		t.Run(fmt.Sprintf("%d/prof%d", tc.item, tc.prof), func(t *testing.T) {
			for _, active := range []bool{false, true} {
				s, p, m := specialWeaponActors(t)
				d := domain.SkillDef{ID: tc.skill, Prof: tc.prof}
				if active {
					applySpecialWeaponDefinition(tc.item, &d)
				}
				state := domain.StatusDef{ID: 180099991, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood}
				if tc.item == 1026 {
					state.PhysicalDamageReductionPct = 20
					state.MagicDamageReductionPct = 20
				}
				specialWeaponStatus(d, &state)
				if !s.applyStatusDef(m, state, p.ID) {
					t.Fatal("status not installed")
				}
				found := false
				m.Status.Each(func(a domain.Active) {
					if a.Def.ID != state.ID {
						return
					}
					found = true
					want := int32(20)
					if active {
						want = 20 * tc.pct / 100
					}
					if a.Def.DurationSec != want {
						t.Fatalf("active=%v duration=%d want%d", active, a.Def.DurationSec, want)
					}
					if tc.item == 1026 {
						res := int32(20)
						if active {
							res = 30
						}
						if a.Def.PhysicalDamageReductionPct != res || a.Def.MagicDamageReductionPct != res {
							t.Fatal("resistance consumer lost", a.Def)
						}
					}
					if tc.item == 1823 {
						bonus := int32(0)
						if active {
							bonus = 60
						}
						if a.Def.SpecialWeaponNextBasicPct != bonus {
							t.Fatal("stealth next attack bonus lost")
						}
					}
				})
				if !found {
					t.Fatal("status missing")
				}
			}
		})
	}
}

func TestSpecialWeaponNativeResourceAcceptance(t *testing.T) {
	for _, tc := range []struct {
		item         domain.ItemID
		skill        domain.SkillID
		prof, cd, mp int32
	}{{1929, 10225, 3, 75, 80}, {1327, 10303, 4, 0, 100}, {1327, 10432, 5, 0, 130}} {
		t.Run(fmt.Sprintf("%d/prof%d", tc.item, tc.prof), func(t *testing.T) {
			for _, active := range []bool{false, true} {
				s, p := behaviorAcceptancePlayer(t, tc.prof)
				p.Player.Char.Skills = domain.Learned{tc.skill: 5}
				if active {
					specialWeaponEquip(t, s, p, tc.item)
				}
				p.MaxMP, p.MP = 100000, 100000
				native, _ := s.skills.Get(tc.skill, 5)
				at := domain.Pos{X: 120, Y: 100}
				s.exec(UseSkill{ID: p.ID, Skill: tc.skill, Target: p.ID, At: &at})
				if len(s.casts) == 0 {
					t.Fatalf("active=%v native cast rejected", active)
				}
				expected := native.MPCost
				if active {
					expected = expected * tc.mp / 100
				}
				if got := 100000 - p.MP; got != expected {
					t.Fatalf("active=%v paidMP=%d want%d", active, got, expected)
				}
				expectedCD := native.CooldownMS
				if active {
					expectedCD = expectedCD * tc.cd / 100
				}
				if s.casts[0].def.CooldownMS != expectedCD {
					t.Fatal("accepted cast cooldown mismatch")
				}
			}
		})
	}
}

func TestSpecialWeaponGeometryActualRecipients(t *testing.T) {
	for _, tc := range []struct {
		item         domain.ItemID
		skill        domain.SkillID
		shape        domain.SkillAreaShape
		probe        domain.Pos
		activeInside bool
	}{{1623, 10006, domain.SkillAreaCone, domain.Pos{X: 160, Y: 150}, true}, {1625, 10021, domain.SkillAreaRectangle, domain.Pos{X: 150, Y: 175}, true}, {1625, 10021, domain.SkillAreaRectangle, domain.Pos{X: 190, Y: 100}, false}} {
		t.Run(fmt.Sprintf("%d/prof1/%v", tc.item, tc.probe), func(t *testing.T) {
			for _, active := range []bool{false, true} {
				s, p, m := specialWeaponActors(t)
				m.Pos = domain.Pos{MapID: p.Pos.MapID, X: 150, Y: 100}
				s.aoi.Enter(m)
				other := *m
				other.ID = 998
				other.Pos = tc.probe
				other.Pos.MapID = p.Pos.MapID
				s.entities[other.ID] = &other
				s.monsters[other.ID] = &other
				s.aoi.Enter(&other)
				d := domain.SkillDef{ID: tc.skill, Prof: 1, Kind: domain.SkillArea, TargetEnemy: true, Dist: 200, Radius: 100, Area: domain.SkillAreaTargeting{Shape: tc.shape, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 60, Width: 100, Length: 100}}
				if tc.shape == domain.SkillAreaCone {
					d.Area.Width = 0
					d.Area.Length = 0
				} else {
					d.Area.Radius = 0
					d.Area.Angle = 0
				}
				if active {
					applySpecialWeaponDefinition(tc.item, &d)
				}
				ids := s.resolveTargets(p, d, m.ID, nil)
				got := false
				for _, id := range ids {
					if id == other.ID {
						got = true
					}
				}
				want := tc.activeInside
				if !active {
					want = !want
				}
				if got != want {
					t.Fatalf("active=%v probe included=%v want%v targets%v", active, got, want, ids)
				}
			}
		})
	}
}

func TestSpecialWeaponTrapRadiusActualPlacement(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, p, _ := specialWeaponActors(t)
		d := domain.SkillDef{ID: 10213, Prof: 3, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectPlaceTrap, TrapRadius: 100}}
		if active {
			applySpecialWeaponDefinition(1924, &d)
		}
		s.alloc = domain.NewEntityAlloc()
		s.placeTrap(p, d, p.Pos, d.DirectEffects()[0].TrapRadius, 100)
		if len(s.traps) != 1 {
			t.Fatalf("active=%v traps=%d", active, len(s.traps))
		}
		for _, trap := range s.traps {
			want := int32(100)
			if active {
				want = 120
			}
			if trap.Trap.Radius != want {
				t.Fatalf("active=%v radius=%d want%d", active, trap.Trap.Radius, want)
			}
		}
	}
}

func TestSpecialWeaponCounterActualSettlement(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, "CK-A24", false)
		if active {
			specialWeaponEquip(t, s, p, 1821)
		}
		p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
		p.Player.Char.Skills = domain.Learned{10203: 1}
		s.skills = domain.SkillTable{{ID: 10203, Level: 1}: {ID: 10203, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveCounter: &domain.PassiveCounterAttack{ChanceBP: 10000, DamagePct: 100}}}
		at, ok := s.passiveCounterPending(p, m)
		if !ok {
			t.Fatal("counter not scheduled")
		}
		s.attacks = append(s.attacks, at)
		before := m.HP
		s.tick++
		s.stepCombatAt(s.now(), false)
		want := int32(100)
		if active {
			want = 150
		}
		if got := before - m.HP; got != want {
			t.Fatalf("active=%v counter damage=%d want%d", active, got, want)
		}
	}
}

func TestSpecialWeaponConcentrationNextBuffActualShield(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, p, _ := specialWeaponActors(t)
		preparation := domain.SkillDef{ID: 10322, Prof: 4}
		if active {
			applySpecialWeaponDefinition(1326, &preparation)
		}
		concentration := domain.StatusDef{ID: 180099992, Level: 1, DurationSec: 20, NextStatusDurationPct: 100}
		specialWeaponStatus(preparation, &concentration)
		s.applyStatusDef(p, concentration, p.ID)
		next := domain.SkillDef{ID: 10320, Prof: 4, Statuses: []domain.StatusApplication{{ID: domain.StatusDamageShield}}}
		s.specialWeaponPrepareCast(p, &next)
		shield := domain.StatusDef{ID: domain.StatusDamageShield, Level: 1, DurationSec: 20, DamageShield: 100}
		specialWeaponStatus(next, &shield)
		s.applyStatusDef(p, shield, p.ID)
		got := int32(0)
		p.Status.Each(func(a domain.Active) {
			if a.Def.ID == domain.StatusDamageShield {
				got += a.ShieldLeft
			}
		})
		want := int32(100)
		if active {
			want = 125
		}
		if got != want {
			t.Fatalf("active=%v actual shield=%d want%d", active, got, want)
		}
	}
}

func TestSpecialWeaponStealthBranchActualFirstHit(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, p, m := specialWeaponActors(t)
		p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
		d := domain.SkillDef{ID: 10208, Prof: 3}
		if active {
			applySpecialWeaponDefinition(1823, &d)
		}
		st := domain.StatusDef{ID: domain.StatusStealth, DurationSec: 30, Invisible: true, StealthDamagePct: 35}
		specialWeaponStatus(d, &st)
		p.Status.Apply(st, s.tick, p.ID)
		for i := 0; i < 2; i++ {
			blow := combat.Blow{SkillPct: 100, GuaranteedHit: true}
			s.prepareBasicAttackStatuses(p, &blow)
			before := m.HP
			s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, blow)
			want := int32(135)
			if active && i == 0 {
				want = 216
			}
			if got := before - m.HP; got != want {
				t.Fatalf("active=%v hit=%d damage=%d want%d", active, i, got, want)
			}
		}
	}
}

func TestSpecialWeaponDefenseBranchActualIncomingDamage(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, magic := range []bool{false, true} {
			s, p, m := specialWeaponActors(t)
			p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.MAtk = 100, 100, 100
			d := domain.SkillDef{ID: 10107, Prof: 2}
			if active {
				applySpecialWeaponDefinition(1026, &d)
			}
			st := domain.StatusDef{ID: 180099993, DurationSec: 20, Level: 1, PhysicalDamageReductionPct: 20, MagicDamageReductionPct: 20}
			specialWeaponStatus(d, &st)
			s.applyStatusDef(m, st, p.ID)
			before := m.HP
			s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true, Magic: magic})
			want := int32(80)
			if active {
				want = 70
			}
			if got := before - m.HP; got != want {
				t.Fatalf("active=%v magic=%v incoming=%d want%d", active, magic, got, want)
			}
		}
	}
}

func TestSpecialWeaponShieldConversionActualSplitSelection(t *testing.T) {
	for _, near := range []bool{false, true} {
		s, p, _ := specialWeaponActors(t)
		p.Player.Party = 7
		p.HP = p.MaxHP
		join(t, s, 2, 102, "shield owner", 100, 100)
		join(t, s, 3, 103, "less injured", 200, 100)
		join(t, s, 4, 104, "most injured", 250, 100)
		for _, id := range []domain.EntityID{2, 3, 4} {
			a := s.players[id]
			a.Player.Party = 7
			a.Player.Char.Level = 100
			s.refreshEntityStats(a)
			a.HP = a.MaxHP / 2
		}
		dst, second := s.players[2], s.players[4]
		second.HP = 1
		baselineDst, baselineOther := dst.HP, s.players[3].HP
		if !near {
			s.players[3].Pos.X = 251
			second.Pos.X = 251
		}
		dst.Status.Apply(domain.StatusDef{ID: domain.StatusDamageShield, Level: 1, DurationSec: 20, DamageShield: 105}, s.tick, p.ID)
		d := domain.SkillDef{ID: 10320, Prof: 4, TargetTeam: true, SpecialWeaponBaseID: 1323}
		if !s.specialWeaponConvertShield(p, dst, d) {
			t.Fatal("conversion did not consume own shield")
		}
		wantDst, wantSecond := baselineDst+63, int32(1)
		if near {
			wantDst, wantSecond = baselineDst+32, 32
		}
		if dst.HP != wantDst || second.HP != wantSecond || s.players[3].HP != baselineOther {
			t.Fatalf("near=%v dst=%d second=%d other=%d", near, dst.HP, second.HP, s.players[3].HP)
		}
		if dst.Status.HasEffective(domain.StatusDamageShield) {
			t.Fatal("consumed shield remains")
		}
	}
}

func TestSpecialWeaponVisibleBackstabNativeAdmission(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, p := behaviorAcceptancePlayer(t, 3)
		p.Player.Char.Skills = domain.Learned{10209: 5}
		if active {
			specialWeaponEquip(t, s, p, 1826)
		}
		putMonster(s, 999, 10000, domain.Stats{}, 110, 100)
		if entityInvisible(p) {
			t.Fatal("fixture unexpectedly invisible")
		}
		s.exec(UseSkill{ID: p.ID, Skill: 10209, Target: 999})
		if got := len(s.casts) > 0; got != active {
			t.Fatalf("active=%v visible backstab accepted=%v", active, got)
		}
	}
}

func TestSpecialWeaponBreakFollowupActualDamage(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, p, m := specialWeaponActors(t)
		p.Player.Char.Race = domain.Warrior
		p.Player.Char.Skills = domain.Learned{10027: 1, 10009: 1}
		specialWeaponEquip(t, s, p, 1626)
		p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
		if active {
			s.specialWeaponAfterHit(p, m, domain.SkillDef{ID: 10027, Prof: 1, SpecialWeaponBaseID: 1626}, true, false)
		}
		for i := 0; i < 2; i++ {
			d := domain.SkillDef{ID: 10009, Prof: 1, SpecialWeaponBaseID: 1626, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
			s.specialWeaponPrepareCast(p, &d)
			before := m.HP
			s.applySkill(p, m, d, 0, false)
			want := int32(100)
			if active && i == 0 {
				want = 140
			}
			if got := before - m.HP; got != want {
				t.Fatalf("active=%v hit=%d actual=%d want%d", active, i, got, want)
			}
		}
	}
}

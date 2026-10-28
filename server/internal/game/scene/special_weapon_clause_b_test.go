package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
	"time"
)

func TestSpecialWeaponClauseBBackstabKillNegatives(t *testing.T) {
	for _, tc := range []struct {
		skill          domain.SkillID
		landed, killed bool
	}{{10209, true, false}, {10208, true, true}, {10209, false, true}} {
		s, p, m := specialWeaponActors(t)
		p.Player.StartSkillCooldown(10208, s.tick, 100)
		s.specialWeaponAfterHit(p, m, domain.SkillDef{SpecialWeaponBaseID: 1824, ID: tc.skill}, tc.landed, tc.killed)
		if p.Player.SkillReadyAt(10208, s.tick+99) || !p.Player.SkillReadyAt(10208, s.tick+100) {
			t.Fatalf("nonqualifying hit refreshed cooldown: %+v", tc)
		}
	}
}

func TestSpecialWeaponClauseBForeignBurnIsUntouched(t *testing.T) {
	for _, tc := range []struct {
		item  domain.ItemID
		skill domain.SkillID
	}{{1322, 10401}, {1325, 10405}} {
		t.Run(fmt.Sprint(tc.item), func(t *testing.T) {
			s, p, m := specialWeaponActors(t)
			m.Status.Apply(domain.StatusDef{ID: 1036, Level: 1, DurationSec: 10, IntervalSec: 2, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -100}}}, s.tick, 777)
			d := domain.SkillDef{ID: tc.skill, Prof: 5}
			applySpecialWeaponDefinition(tc.item, &d)
			before := m.HP
			s.specialWeaponAfterHit(p, m, d, true, false)
			if m.HP != before || !m.Status.HasEffective(1036) {
				t.Fatal("foreign burn cashed out or removed")
			}
		})
	}
}

func TestSpecialWeaponClauseBSlowLiveStatsAndPreservation(t *testing.T) {
	for _, tc := range []struct {
		item         domain.ItemID
		pct, seconds int32
	}{{1320, 20, 2}, {1925, 30, 3}, {1928, 70, 6}} {
		t.Run(fmt.Sprint(tc.item), func(t *testing.T) {
			s, p, _ := specialWeaponActors(t)
			join(t, s, 2, 102, "slow target", 110, 100)
			dst := s.players[2]
			s.refreshStats(dst)
			base := dst.Stats.MoveSpeed
			prof := int32(3)
			if tc.item == 1320 {
				prof = 5
			}
			d := domain.SkillDef{SpecialWeaponBaseID: tc.item, Prof: prof}
			s.specialWeaponAfterHit(p, dst, d, true, false)
			if got, want := dst.Stats.MoveSpeed, base*(100-tc.pct)/100; got != want {
				t.Fatalf("speed=%d want%d", got, want)
			}
			s.tick += domain.Ticks(int(tc.seconds)*1000) - 1
			s.stepStatus()
			if !dst.Status.HasEffective(domain.StatusSlow) {
				t.Fatal("slow expired early")
			}
			s.tick++
			s.stepStatus()
			if dst.Stats.MoveSpeed != base || dst.Status.HasEffective(domain.StatusSlow) {
				t.Fatal("slow did not release live speed")
			}
			for _, prior := range []struct{ pct, seconds int32 }{{tc.pct + 10, 1}, {1, tc.seconds + 1}} {
				s.applyStatusDef(dst, domain.StatusDef{ID: domain.StatusSlow, Level: 1, DurationSec: prior.seconds, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: -prior.pct}}}, 777)
				before := universalSoloActive(t, dst, domain.StatusSlow)
				speed := dst.Stats.MoveSpeed
				s.specialWeaponAfterHit(p, dst, d, true, false)
				after := universalSoloActive(t, dst, domain.StatusSlow)
				if before.Source != after.Source || before.ExpireAt != after.ExpireAt || speed != dst.Stats.MoveSpeed {
					t.Fatal("stronger or longer existing slow replaced")
				}
				s.statusesRemoved(dst, dst.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == domain.StatusSlow }))
			}
		})
	}
}

func TestSpecialWeaponClauseBShieldForeignSourceAndTie(t *testing.T) {
	s, p, _ := specialWeaponActors(t)
	p.Player.Party = 7
	p.HP = p.MaxHP
	for _, id := range []domain.EntityID{2, 3, 4} {
		join(t, s, id, int64(id)*100, "ally", 110, 100)
		a := s.players[id]
		a.Player.Party = 7
		a.Player.Char.Level = 100
		s.refreshStats(a)
		a.HP = 1
	}
	dst := s.players[2]
	d := domain.SkillDef{ID: 10320, Prof: 4, TargetTeam: true, SpecialWeaponBaseID: 1323}
	shield := domain.StatusDef{ID: domain.StatusDamageShield, Level: 1, DurationSec: 20, DamageShield: 100}
	dst.Status.Apply(shield, s.tick, 777)
	if s.specialWeaponConvertShield(p, dst, d) || dst.HP != 1 || !dst.Status.HasEffective(domain.StatusDamageShield) {
		t.Fatal("foreign shield converted")
	}
	dst.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == domain.StatusDamageShield })
	dst.Status.Apply(shield, s.tick, p.ID)
	if !s.specialWeaponConvertShield(p, dst, d) {
		t.Fatal("own shield not converted")
	}
	if dst.HP != 31 || s.players[3].HP != 31 || s.players[4].HP != 1 {
		t.Fatalf("tie did not choose lower ID: %d %d %d", dst.HP, s.players[3].HP, s.players[4].HP)
	}
}

func TestSpecialWeaponClauseBCleanseHealCountCap(t *testing.T) {
	for count := 0; count <= 4; count++ {
		s, p, m := specialWeaponActors(t)
		m.HP = 1
		removed := make([]domain.StatusID, count)
		for i := range removed {
			removed[i] = domain.StatusID(100 + i)
		}
		s.specialWeaponAfterDispel(p, m, domain.SkillDef{ID: 10316, Prof: 4, SpecialWeaponBaseID: 1325}, removed, nil)
		if want := int32(1) + m.MaxHP*int32(min(count, 3))*3/100; m.HP != want {
			t.Fatalf("count%d hp%d want%d", count, m.HP, want)
		}
	}
}

func TestSpecialWeaponTeleportActualMovementAndRecast(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 5)
	p.Player.Char.Skills = domain.Learned{10432: 5}
	specialWeaponEquip(t, s, p, 1327)
	p.MaxMP, p.MP = 100000, 100000
	for _, x := range []float64{120, 140} {
		at := domain.Pos{MapID: p.Pos.MapID, X: x, Y: 100}
		s.exec(UseSkill{ID: p.ID, Skill: 10432, Target: p.ID, At: &at})
		if len(s.casts) != 1 {
			t.Fatalf("teleport %v not admitted", x)
		}
		s.stepCastsAt(s.now().Add(time.Second), false)
		s.stepCastsAt(s.now().Add(2*time.Second), false)
		if p.Pos.X != x || p.Pos.Y != 100 {
			t.Fatalf("teleport destination=%v wantX%v", p.Pos, x)
		}
		if len(s.casts) != 0 {
			t.Fatal("cast not finished")
		}
	}
}

func TestSpecialWeaponClauseBSmokeLearnedAdmissionAndStealthExpiry(t *testing.T) {
	for _, learned := range []bool{false, true} {
		s, p := behaviorAcceptancePlayer(t, 3)
		specialWeaponEquip(t, s, p, 1825)
		if learned {
			p.Player.Char.Skills = domain.Learned{10224: 5}
		}
		putMonster(s, 999, 10000, domain.Stats{}, 110, 100)
		s.exec(UseSkill{ID: p.ID, Skill: 10224, Target: 999})
		if (len(s.casts) == 1) != learned {
			t.Fatalf("learned=%v casts=%d", learned, len(s.casts))
		}
		if !learned {
			continue
		}
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		if !entityInvisible(p) {
			t.Fatal("accepted dualblade smoke did not grant stealth")
		}
		s.tick += domain.Ticks(2000) - 1
		s.stepStatus()
		if !entityInvisible(p) {
			t.Fatal("stealth expired early")
		}
		s.tick++
		s.stepStatus()
		if entityInvisible(p) {
			t.Fatal("stealth persisted past two seconds")
		}
	}
}

func TestSpecialWeaponClauseBOverflowShieldReplacesAndExpires(t *testing.T) {
	s, p, _ := specialWeaponActors(t)
	d := domain.SkillDef{ID: 10302, Prof: 4, SpecialWeaponBaseID: 1322, Effects: []domain.SkillEffect{{Kind: domain.EffectRestoreMP, RestoreMPFlat: 100}}}
	p.MP = p.MaxMP - 20
	s.applySkill(p, p, d, 0, false)
	if a := universalSoloActive(t, p, 1800101322); a.ShieldLeft != 40 {
		t.Fatal("first overflow", a.ShieldLeft)
	}
	s.tick += domain.Ticks(1000)
	p.MP = p.MaxMP - 80
	s.applySkill(p, p, d, 0, false)
	if a := universalSoloActive(t, p, 1800101322); a.ShieldLeft != 10 {
		t.Fatal("replacement stacked or retained old balance", a.ShieldLeft)
	}
	s.tick += domain.Ticks(8000) - 1
	s.stepStatus()
	if !p.Status.HasEffective(1800101322) {
		t.Fatal("replacement shield expired early")
	}
	s.tick++
	s.stepStatus()
	if p.Status.HasEffective(1800101322) {
		t.Fatal("replacement shield did not expire")
	}
}

func TestSpecialWeaponClauseBConcentrationNativeStatAndConsumption(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	p.Player.Char.Skills = domain.Learned{10322: 5, 10307: 5}
	specialWeaponEquip(t, s, p, 1326)
	item := s.items[p.Player.Worn.At(domain.SlotWeapon).Item]
	equip := *item.Equip
	item.Equip = &equip
	item.Equip.Bonus.MAtk += 100
	s.items[item.ID] = item
	s.invalidateCombatAttributes(p)
	s.refreshStats(p)
	base := p.Stats.MAtk
	focusDef := assassinRegressionCast(t, s, p, UseSkill{Skill: 10322, Target: p.ID})
	if len(focusDef.Statuses) == 0 {
		t.Fatal("missing focus payload")
	}
	focus := universalSoloActive(t, p, focusDef.Statuses[0].ID)
	native, _ := s.skills.Get(10307, 5)
	if len(native.Statuses) == 0 {
		t.Fatal("missing buff payload")
	}
	expected, ok := s.resolveSkillStatusDef(native, native.Statuses[0], 0)
	if !ok {
		t.Fatal("missing status")
	}
	p.MP = p.MaxMP
	assassinRegressionCast(t, s, p, UseSkill{Skill: 10307, Target: p.ID})
	active := universalSoloActive(t, p, native.Statuses[0].ID)
	if p.Status.HasEffective(focus.Def.ID) {
		t.Fatal("concentration not consumed")
	}
	if want := domain.Ticks(int(expected.DurationSec)*1000) * (domain.Tick(100 + focus.Def.NextStatusDurationPct)) / 100; active.ExpireAt-s.tick != want {
		t.Fatalf("duration=%d want%d", active.ExpireAt-s.tick, want)
	}
	var add, pct int32
	for _, a := range expected.Affixes {
		if a.Attr == domain.AttrMAtk {
			v := a.Value * 125 / 100
			if a.Mode == domain.ModePercent {
				pct += v
			} else {
				add += v
			}
		}
	}
	if add == 0 && pct == 0 {
		t.Fatalf("fixture lacks magic attack %+v", expected)
	}
	if want := (base + add) * (100 + pct) / 100; p.Stats.MAtk != want {
		t.Fatalf("live magic attack=%d want%d (base%d)", p.Stats.MAtk, want, base)
	}
}

func TestSpecialWeaponClauseBRemoteIceBlastActualRecipients(t *testing.T) {
	s, p, m := specialWeaponActors(t)
	s.skills, s.statuses = realV5SkillFixtures(t)
	p.Player.Char.Skills = domain.Learned{10428: 5, 10410: 5}
	p.Stats.MAtk = 100
	m.Pos.X = 300
	s.aoi.Enter(m)
	putMonster(s, 998, 10000, domain.Stats{}, 310, 100)
	putMonster(s, 997, 10000, domain.Stats{}, 100, 100)
	m.Status.Apply(domain.StatusDef{ID: domain.StatusFreeze, Level: 5, DurationSec: 20, SourceSkillID: 10428}, s.tick, p.ID)
	d := domain.SkillDef{ID: 10410, Prof: 5, Kind: domain.SkillArea, Dist: 100, TargetEnemy: true, HurtType: domain.HurtIce, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 50}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
	applySpecialWeaponDefinition(1323, &d)
	s.specialWeaponAim(p, &d, m.ID)
	ids := s.resolveTargets(p, d, m.ID, nil)
	for _, id := range ids {
		s.applySkill(p, s.entities[id], d, 0, false)
	}
	if m.HP != 9920 || s.entities[998].HP != 9920 || s.entities[997].HP != 10000 {
		t.Fatalf("remote blast HP center=%d neighbor=%d caster-neighbor=%d", m.HP, s.entities[998].HP, s.entities[997].HP)
	}
}

func TestSpecialWeaponClauseBNativeManaPayments(t *testing.T) {
	for _, tc := range []struct {
		item      domain.ItemID
		skill     domain.SkillID
		prof, pct int32
	}{{1923, 10205, 3, 120}, {1320, 10301, 4, 110}, {1321, 10306, 4, 200}} {
		s, p := behaviorAcceptancePlayer(t, tc.prof)
		p.Player.Char.Skills = domain.Learned{tc.skill: 5}
		specialWeaponEquip(t, s, p, tc.item)
		native, _ := s.skills.Get(tc.skill, 5)
		target := p.ID
		if tc.prof == 3 {
			putMonster(s, 999, 10000, domain.Stats{}, 110, 100)
			target = 999
		}
		before := p.MP
		s.exec(UseSkill{ID: p.ID, Skill: tc.skill, Target: target})
		if len(s.casts) != 1 || before-p.MP != native.MPCost*tc.pct/100 {
			t.Fatalf("item%d admitted%d payment%d want%d", tc.item, len(s.casts), before-p.MP, native.MPCost*tc.pct/100)
		}
	}
}

func TestSpecialWeaponClauseBRejuvenationNativeTicksAndRecast(t *testing.T) {
	var nativeHeal int32
	var nativeDuration domain.Tick
	for _, enhanced := range []bool{false, true} {
		s, p := behaviorAcceptancePlayer(t, 4)
		p.Player.Char.Skills = domain.Learned{10303: 5}
		if enhanced {
			specialWeaponEquip(t, s, p, 1327)
		}
		p.MaxHP = 100000
		p.HP = 1
		d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10303, Target: p.ID})
		a := universalSoloActive(t, p, d.Statuses[0].ID)
		duration := a.ExpireAt - s.tick
		before := p.HP
		for tick := s.tick + 1; tick <= a.ExpireAt; tick++ {
			s.tick = tick
			s.stepStatus()
		}
		healed := p.HP - before
		if !enhanced {
			nativeHeal = healed
			nativeDuration = duration
		} else if healed != nativeHeal || duration != nativeDuration || healed <= 0 {
			t.Fatalf("enhanced periodic heal%d duration%d native%d/%d", healed, duration, nativeHeal, nativeDuration)
		}
		if enhanced {
			// Clear unrelated wall-clock cast admission state by using the ordinary
			// finished-cast helper; scene tick remains at the first cast's instant.
			s2, p2 := behaviorAcceptancePlayer(t, 4)
			p2.Player.Char.Skills = domain.Learned{10303: 5}
			specialWeaponEquip(t, s2, p2, 1327)
			for i := 0; i < 2; i++ {
				p2.MP = p2.MaxMP
				assassinRegressionCast(t, s2, p2, UseSkill{Skill: 10303, Target: p2.ID})
			}
		}
	}
}

func TestSpecialWeaponClauseBStealRejectsPermanentAndNonWhitelist(t *testing.T) {
	for _, tc := range []struct {
		id        domain.StatusID
		permanent bool
	}{{domain.StatusDamageShield, true}, {180099999, false}} {
		s, p, m := specialWeaponActors(t)
		s.applyStatusDef(m, domain.StatusDef{ID: tc.id, Level: 1, DurationSec: 20, Permanent: tc.permanent, DamageShield: 100, TypeKnown: true, Type: domain.StatusGood}, m.ID)
		d := domain.SkillDef{ID: 10433, Prof: 5, SpecialWeaponBaseID: 1324, TargetEnemy: true, Effect: domain.SkillEffect{RemoveStatusIDs: []domain.StatusID{domain.StatusDamageShield}, DispelChanceBP: 10000}}
		s.applySkill(p, m, d, 0, false)
		if !m.Status.HasEffective(tc.id) || p.Status.HasEffective(tc.id) {
			t.Fatalf("ineligible defense removed/stolen %+v", tc)
		}
	}
}

func TestSpecialWeaponClauseBTrapBoundaryAndUnchangedDamage(t *testing.T) {
	for _, enhanced := range []bool{false, true} {
		s, p, m := specialWeaponActors(t)
		p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
		s.alloc = domain.NewEntityAlloc()
		d := domain.SkillDef{ID: 10213, Prof: 3, TargetEnemy: true, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
		if enhanced {
			applySpecialWeaponDefinition(1924, &d)
		}
		radius := d.DirectEffects()[0].TrapRadius
		m.Pos.X = p.Pos.X + float64(radius) + 1
		s.placeTrap(p, d, p.Pos, radius, 100)
		s.stepTraps()
		if len(s.traps) != 1 || m.HP != 10000 {
			t.Fatal("outside enemy triggered trap")
		}
		m.Pos.X--
		s.stepTraps()
		if len(s.traps) != 0 || m.HP != 9900 {
			t.Fatalf("enhanced=%v boundary trap%d damage%d", enhanced, len(s.traps), 10000-m.HP)
		}
	}
}

func TestSpecialWeaponClauseBSmokeExactZoneBoundary(t *testing.T) {
	s, p, m := specialWeaponActors(t)
	s.skills, s.statuses = realV5SkillFixtures(t)
	p.Player.Char.Skills = domain.Learned{10224: 5}
	specialWeaponEquip(t, s, p, 1825)
	d, _ := s.skills.Get(10224, 5)
	applySpecialWeaponDefinition(1825, &d)
	m.Pos.X = p.Pos.X + 120
	s.specialWeaponCastFinished(p, d, nil)
	s.tick += domain.Ticks(1000)
	s.stepSpecialWeaponSmoke()
	count := func() int {
		n := 0
		m.Status.EachEffective(func(a domain.Active) {
			if a.Def.SpecialWeaponSourceBase == 1825 {
				n++
			}
		})
		return n
	}
	if count() != 2 {
		t.Fatal("exact smoke edge not admitted")
	}
	m.Pos.X++
	s.tick += domain.Ticks(1000)
	s.stepSpecialWeaponSmoke()
	if count() != 0 {
		t.Fatal("outside smoke edge retained debuffs")
	}
}

func TestSpecialWeaponClauseBSmokeLiveDebuffMagnitude(t *testing.T) {
	for _, item := range []domain.ItemID{1825, 1927} {
		s, p, _ := specialWeaponActors(t)
		s.skills, s.statuses = realV5SkillFixtures(t)
		specialWeaponEquip(t, s, p, 1326)
		gear := s.items[p.Player.Worn.At(domain.SlotWeapon).Item]
		equip := *gear.Equip
		gear.Equip = &equip
		gear.Equip.Bonus.MAtk += 100
		gear.Equip.Bonus.Hit += 100
		s.items[gear.ID] = gear
		s.invalidateCombatAttributes(p)
		s.refreshStats(p)
		hit, magic := p.Stats.Hit, p.Stats.MAtk
		native, _ := s.skills.Get(10224, 5)
		d := native
		applySpecialWeaponDefinition(item, &d)
		hitPct, magicPct := int32(0), int32(0)
		for _, app := range native.Statuses {
			original, ok := s.resolveSkillStatusDef(native, app, 0)
			if !ok {
				t.Fatal("missing native smoke status")
			}
			state, ok := s.resolveSkillStatusDef(d, app, 0)
			if !ok {
				t.Fatal("missing weapon smoke status")
			}
			for _, af := range original.Affixes {
				v := af.Value
				if item == 1825 {
					v = v * 50 / 100
				}
				if af.Mode != domain.ModePercent {
					t.Fatal("unexpected native mode")
				}
				switch af.Attr {
				case domain.AttrHit:
					hitPct += v
				case domain.AttrMAtk:
					magicPct += v
				}
			}
			// Bypass only the hit contest: magnitudes are tested after successful
			// status admission, independently of the shared probability consumer.
			state.ChanceExplicit = true
			state.ChanceBP = 10000
			if !s.applyStatusDef(p, state, 999) {
				t.Fatal("debuff not installed")
			}
		}
		if hitPct == 0 || magicPct == 0 {
			t.Fatal("missing native smoke penalties")
		}
		if p.Stats.Hit != (hit+hit*hitPct/100) || p.Stats.MAtk != (magic+magic*magicPct/100) {
			t.Fatalf("item%d live hit%d magic%d expected%d/%d", item, p.Stats.Hit, p.Stats.MAtk, (hit + hit*hitPct/100), (magic + magic*magicPct/100))
		}
	}
}

func TestSpecialWeaponClauseBFrostCannotFreeze(t *testing.T) {
	for _, enhanced := range []bool{false, true} {
		s, p, m := specialWeaponActors(t)
		s.skills, s.statuses = realV5SkillFixtures(t)
		d, _ := s.skills.Get(10216, 5)
		// Guarantee native status application to isolate removal, not RNG.
		for i := range d.Statuses {
			d.Statuses[i].ChanceExplicit = true
			d.Statuses[i].ChanceBP = 10000
		}
		if enhanced {
			applySpecialWeaponDefinition(1928, &d)
		}
		s.applySkill(p, m, d, 0, false)
		if m.Status.HasEffective(domain.StatusFreeze) == enhanced {
			t.Fatalf("enhanced=%v freeze=%v", enhanced, m.Status.HasEffective(domain.StatusFreeze))
		}
	}
}

func TestSpecialWeaponClauseBBackstabRetainsInvisibleAdmission(t *testing.T) {
	for _, invisible := range []bool{false, true} {
		s, p := behaviorAcceptancePlayer(t, 3)
		p.Player.Char.Skills = domain.Learned{10209: 5}
		specialWeaponEquip(t, s, p, 1827)
		putMonster(s, 999, 10000, domain.Stats{}, 110, 100)
		if invisible {
			s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusStealth, Level: 1, DurationSec: 20, Invisible: true}, p.ID)
		}
		s.exec(UseSkill{ID: p.ID, Skill: 10209, Target: 999})
		if (len(s.casts) == 1) != invisible {
			t.Fatalf("invisible=%v admission%d", invisible, len(s.casts))
		}
	}
}

func TestSpecialWeaponClauseBCircleReplacesConeRearAndSide(t *testing.T) {
	for _, offset := range []domain.Pos{{X: -80}, {Y: 80}, {X: -81}} {
		s, p, m := specialWeaponActors(t)
		m.Pos.X += 30
		s.aoi.Enter(m)
		probe := putMonster(s, 998, 10000, domain.Stats{}, p.Pos.X+offset.X, p.Pos.Y+offset.Y)
		d := domain.SkillDef{ID: 10206, Prof: 3, Kind: domain.SkillArea, TargetEnemy: true, Dist: 500, Radius: 100, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 60}}
		applySpecialWeaponDefinition(1926, &d)
		ids := s.resolveTargets(p, d, m.ID, nil)
		found := false
		for _, id := range ids {
			if id == probe.ID {
				found = true
			}
		}
		if found != (offset.X != -81) {
			t.Fatalf("offset%+v targets%v", offset, ids)
		}
	}
}

func TestSpecialWeaponClauseBTrapNativePlacementRange(t *testing.T) {
	for _, outside := range []bool{false, true} {
		s, p := behaviorAcceptancePlayer(t, 3)
		p.Player.Char.Skills = domain.Learned{10213: 5}
		specialWeaponEquip(t, s, p, 1924)
		d, _ := s.skills.Get(10213, 5)
		distance := d.Dist * 150 / 100
		if outside {
			distance++
		}
		at := domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + float64(distance), Y: p.Pos.Y}
		before := p.MP
		s.exec(UseSkill{ID: p.ID, Skill: 10213, At: &at})
		if (len(s.casts) == 1) == outside {
			t.Fatalf("outside=%v native placement admission%d", outside, len(s.casts))
		}
		if outside && p.MP != before {
			t.Fatal("rejected placement charged mana")
		}
	}
}

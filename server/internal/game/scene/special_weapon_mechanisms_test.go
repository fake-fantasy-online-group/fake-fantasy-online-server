package scene

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"os"
	"reflect"
	"testing"
)

func TestSpecialWeaponFinalRequestedChanges(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, rank := range []int32{1, 5, 20, 100} {
		for _, tc := range []struct {
			item           domain.ItemID
			skill          domain.SkillID
			damage, cd, mp int32
		}{{1021, 10102, 130, 100, 100}, {1022, 10103, 80, 100, 100}, {1023, 10104, 90, 100, 100}, {1024, 10111, 100, 40, 100}, {1025, 10113, 100, 30, 100}, {1026, 10107, 100, 60, 100}, {1520, 10117, 115, 100, 100}, {1521, 10111, 65, 50, 100}, {1523, 10119, 135, 100, 100}, {1524, 10121, 90, 100, 100}, {1620, 10005, 110, 100, 100}, {1621, 10017, 115, 100, 100}, {1626, 10027, 100, 35, 100}, {1627, 10022, 135, 100, 100}, {1826, 10209, 65, 100, 100}, {1827, 10209, 135, 120, 100}, {1922, 10221, 70, 25, 100}, {1923, 10205, 130, 100, 120}, {1925, 10223, 130, 100, 100}, {1927, 10224, 100, 150, 100}, {1928, 10216, 140, 70, 100}, {1320, 10301, 100, 100, 110}, {1320, 10409, 130, 100, 100}, {1321, 10306, 100, 150, 200}, {1321, 10410, 90, 100, 100}, {1926, 10206, 120, 100, 100}, {1326, 10412, 125, 100, 100}, {1327, 10303, 100, 0, 100}, {1327, 10432, 100, 0, 130}, {1820, 10219, 70, 25, 100}, {1929, 10225, 100, 75, 80}} {
			t.Run(fmt.Sprintf("%d/%d/%d", tc.item, tc.skill, rank), func(t *testing.T) {
				original, ok := skills.Get(tc.skill, rank)
				if !ok {
					t.Fatal("missing native skill")
				}
				d := original
				applySpecialWeaponDefinition(tc.item, &d)
				if d.CooldownMS != specialScale(original.CooldownMS, tc.cd) || d.MPCost != specialScale(original.MPCost, tc.mp) {
					t.Fatal("resource change diverged", d.CooldownMS, d.MPCost, original.CooldownMS, original.MPCost)
				}
				for _, e := range d.DirectEffects() {
					old := domain.SkillEffect{}
					for _, candidate := range original.DirectEffects() {
						if candidate.Kind == e.Kind {
							old = candidate
							break
						}
					}
					if e.Kind == domain.EffectDamage && e.DamagePct != specialScale(old.DamagePct, tc.damage) && old.DamagePctDenominator == 0 {
						t.Fatal("damage multiplier", e, old)
					}
				}
				if tc.skill == 10432 {
					for _, e := range d.DirectEffects() {
						if e.Kind == domain.EffectTeleportPoint && e.ChanceBP != 10000 {
							t.Fatal("teleport not guaranteed")
						}
					}
				}
				again, _ := skills.Get(tc.skill, rank)
				if !reflect.DeepEqual(original, again) {
					t.Fatal("mutated native skill")
				}
			})
		}
	}
	s := newTestScene(t, nil)
	s.statuses = statuses
	original, _ := skills.Get(10303, 5)
	d := original
	applySpecialWeaponDefinition(1327, &d)
	if !reflect.DeepEqual(d.Statuses, original.Statuses) {
		t.Fatal("rejuvenation payload/duration changed")
	}
	original, _ = skills.Get(10433, 5)
	d = original
	applySpecialWeaponDefinition(1324, &d)
	if d.MPCost != original.MPCost || !reflect.DeepEqual(d.DirectEffects(), original.DirectEffects()) {
		t.Fatal("steal changed original dispel rules")
	}
	original, _ = skills.Get(10322, 5)
	d = original
	applySpecialWeaponDefinition(1326, &d)
	state, ok := s.resolveSkillStatusDef(d, d.Statuses[0], 0)
	if !ok || state.NextStatusDurationPct <= 0 || state.SpecialWeaponBuffBonusPct != 25 {
		t.Fatal("concentration lost duration or power", state)
	}
}
func TestSpecialWeaponNativeAndLowLevelAreInert(t *testing.T) {
	for _, id := range []domain.ItemID{1019, 1020, 1518, 1519, 1618, 1619, 1818, 1819, 1920, 1921, 1318, 1319} {
		d := domain.SkillDef{ID: 10432, Prof: 5, CooldownMS: 20000}
		applySpecialWeaponDefinition(id, &d)
		if d.SpecialWeaponBaseID != 0 || d.CooldownMS != 20000 {
			t.Fatal("low-level weapon got an effect", id)
		}
		if domain.EnhancedWeaponBaseID(id) != 0 {
			t.Fatal("low-level task item has mechanism identity", id)
		}
	}
	d := domain.SkillDef{ID: 10432, Prof: 4, CooldownMS: 20000}
	applySpecialWeaponDefinition(1327, &d)
	if d.CooldownMS != 20000 {
		t.Fatal("wrong profession received teleport")
	}
	for _, id := range []domain.ItemID{1022, 1521, 1621, 1821, 1923, 1321} {
		if domain.EnhancedWeaponBaseID(id) != 0 {
			t.Fatal("ordinary dungeon weapon enabled", id)
		}
	}
}
func TestSpecialWeaponControlAndGeometry(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.statuses = statuses
	for _, tc := range []struct {
		item  domain.ItemID
		skill domain.SkillID
		pct   int32
	}{{1024, 10111, 50}, {1025, 10113, 30}, {1820, 10219, 20}, {1922, 10221, 30}, {1626, 10027, 50}} {
		original, _ := skills.Get(tc.skill, 5)
		d := original
		applySpecialWeaponDefinition(tc.item, &d)
		for i, a := range d.Statuses {
			before, ok := s.resolveSkillStatusDef(original, original.Statuses[i], 0)
			after, ok2 := s.resolveSkillStatusDef(d, a, 0)
			if ok && ok2 && after.DurationSec != max(int32(1), specialScale(before.DurationSec, tc.pct)) {
				t.Fatal("control duration", tc.item, before.DurationSec, after.DurationSec)
			}
		}
	}
	for _, tc := range []struct {
		item          domain.ItemID
		skill         domain.SkillID
		radius, limit int32
		center        domain.SkillAreaCenter
	}{{1022, 10103, 100, 3, domain.SkillAreaCenterCaster}, {1521, 10111, 100, 3, domain.SkillAreaCenterTarget}, {1825, 10224, 120, 5, domain.SkillAreaCenterCaster}, {1927, 10224, 150, 5, domain.SkillAreaCenterTarget}, {1321, 10306, 150, 3, domain.SkillAreaCenterTarget}} {
		d, _ := skills.Get(tc.skill, 1)
		applySpecialWeaponDefinition(tc.item, &d)
		if !d.Area.Valid() || d.Area.Center != tc.center || d.Area.Radius != tc.radius || d.SpecialWeaponTargetLimit != 0 {
			t.Fatal("invalid area", tc, d.Area)
		}
	}
	d, _ := skills.Get(10224, 1)
	applySpecialWeaponDefinition(1825, &d)
	if d.RequiredWeaponType != 8 {
		t.Fatal("dualblade smoke override missing")
	}
	d, _ = skills.Get(10209, 1)
	applySpecialWeaponDefinition(1826, &d)
	if d.RequiresInvisible {
		t.Fatal("open backstab still requires invisible")
	}
}
func specialWeaponActors(t *testing.T) (*Scene, *entity.Entity, *entity.Entity) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "武器测试", 100, 100)
	p := s.players[1]
	p.Player.Char.Level = 100
	s.refreshEntityStats(p)
	p.HP = p.MaxHP
	p.MP = p.MaxMP
	dst := &entity.Entity{ID: 999, Kind: domain.KindMonster, Pos: p.Pos, HP: 10000, MaxHP: 10000, MP: 100, MaxMP: 100, Status: domain.NewStatusSet(), Monster: &entity.Monster{Kind: domain.MonsterNormal}}
	s.entities[dst.ID] = dst
	s.monsters[dst.ID] = dst
	return s, p, dst
}
func TestSpecialWeaponOverflowAndActualManaShield(t *testing.T) {
	s, p, dst := specialWeaponActors(t)
	p.Stats.MaxAtk = 1000
	d := domain.SkillDef{ID: 10013, Prof: 1, SpecialWeaponBaseID: 1622, Effects: []domain.SkillEffect{{Kind: domain.EffectDamageMP, SourceAttackPct: 20}}}
	if !s.applySkill(p, dst, d, 0, false) || dst.MP != 0 {
		t.Fatal("mana destruction not applied")
	}
	shield := int32(0)
	p.Status.Each(func(a domain.Active) { shield += a.ShieldLeft })
	if shield != 50 {
		t.Fatal("shield must use actual MP, not attempted destruction", shield)
	}
	s.applySkill(p, dst, d, 0, false)
	after := int32(0)
	p.Status.Each(func(a domain.Active) { after += a.ShieldLeft })
	if after != 50 {
		t.Fatal("empty mana granted extra shield", after)
	}
	p.MP = p.MaxMP - 10
	d = domain.SkillDef{ID: 10302, Prof: 4, SpecialWeaponBaseID: 1322, Effects: []domain.SkillEffect{{Kind: domain.EffectRestoreMP, RestoreMPFlat: 50}}}
	s.applySkill(p, p, d, 0, false)
	got := int32(0)
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1800101322 {
			got = a.ShieldLeft
			if a.Def.DurationSec != 8 {
				t.Fatal("wrong shield duration")
			}
		}
	})
	if got != 20 || p.MP != p.MaxMP {
		t.Fatal("wrong overflow shield", got, p.MP)
	}
}
func TestSpecialWeaponBurnCashoutPreservesOrConsumesOwnBurn(t *testing.T) {
	for _, consume := range []bool{false, true} {
		s, p, dst := specialWeaponActors(t)
		burn := domain.StatusDef{ID: 1036, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, IntervalSec: 2, HPChanges: []domain.ResourceChange{{Value: -100}}}
		dst.Status.Apply(burn, s.tick, p.ID)
		before := dst.HP
		s.specialWeaponBurn(p, dst, 100, consume)
		if dst.HP != before-500 {
			t.Fatal("remaining damage not settled", consume, dst.HP)
		}
		count := 0
		dst.Status.Each(func(a domain.Active) {
			if a.Def.ID == 1036 {
				count++
			}
		})
		if (count == 0) != consume {
			t.Fatal("incorrect burn removal", consume, count)
		}
	}
}
func TestSpecialWeaponStealAndShieldHealing(t *testing.T) {
	s, p, dst := specialWeaponActors(t)
	shield := domain.StatusDef{ID: domain.StatusDamageShield, Level: 1, Name: "光盾", TypeKnown: true, Type: domain.StatusGood, DurationSec: 20, DamageShield: 1000}
	dst.Status.Apply(shield, s.tick, dst.ID)
	d := domain.SkillDef{ID: 10433, Prof: 5, SpecialWeaponBaseID: 1324, Effects: []domain.SkillEffect{{RemoveStatusIDs: []domain.StatusID{domain.StatusDamageShield}, DispelChanceBP: 10000}}}
	s.applySkill(p, dst, d, 0, false)
	got := int32(0)
	p.Status.Each(func(a domain.Active) { got += a.ShieldLeft })
	if got != 1000 {
		t.Fatal("defense spell not stolen", got)
	}
	p.HP = 100
	// Use real derived HP: applying/removing a status refreshes panel stats.
	p.Status.Remove(domain.StatusDamageShield)
	p.Status.Apply(shield, s.tick, p.ID)
	d = domain.SkillDef{ID: 10320, Prof: 4, SpecialWeaponBaseID: 1323, TargetSelf: true, TargetTeam: true}
	if !s.specialWeaponConvertShield(p, p, d) || p.HP != 700 {
		t.Fatal("no injured second ally must keep full60%", p.HP)
	}
	got = 0
	p.Status.Each(func(a domain.Active) { got += a.ShieldLeft })
	if got != 0 {
		t.Fatal("converted shield still present", got)
	}
}
func TestSpecialWeaponBuffPowerPreservesDurationAndBoolean(t *testing.T) {
	d := domain.StatusDef{DurationSec: 30, Invulnerable: true, DamageShield: 100, Affixes: []domain.Affix{{Attr: domain.AttrMAtk, Value: 40, Mode: domain.ModePercent}}}
	specialWeaponBuffPower(&d, 25)
	if d.DurationSec != 30 || !d.Invulnerable || d.DamageShield != 125 || d.Affixes[0].Value != 50 {
		t.Fatal("wrong concentrated buff", d)
	}
}
func TestSpecialWeaponInstantTrapDoesNotRemainPlaced(t *testing.T) {
	s, p, dst := specialWeaponActors(t)
	p.Stats.MAtk = 100
	d := domain.SkillDef{ID: 10213, Prof: 3, SpecialWeaponBaseID: 1822, HurtType: domain.HurtFire, TargetEnemy: true, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
	if !s.specialWeaponImmediateTrap(p, d, p.Pos, 100, 0, nil) {
		t.Fatal("instant trap not intercepted")
	}
	if len(s.traps) != 0 || dst.HP >= 10000 {
		t.Fatal("trap stayed placed or did not damage", len(s.traps), dst.HP)
	}
}

func specialWeaponEquip(t *testing.T, s *Scene, p *entity.Entity, base domain.ItemID) {
	t.Helper()
	id := base
	if domain.EnhancedWeaponBaseID(id) != base {
		id = domain.HardDungeonItemOffset + base
	}
	if domain.EnhancedWeaponBaseID(id) != base {
		t.Fatal("test needs an enabled, authorized weapon identity", base)
	}
	if s.items == nil {
		s.items = make(map[domain.ItemID]domain.ItemDef)
	}
	d := domain.ItemDef{ID: id, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), Durable: 100, Type: 8}}
	s.items[id] = d
	st := domain.NewStack(d, 1)
	st.UID = int64(id)
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.invalidateCombatAttributes(p)
	s.refreshStats(p)
	p.HP = p.MaxHP
	p.MP = p.MaxMP
}
func TestSpecialWeaponAcceptedGroupCleanseMatchesPreviewAndBrokenGate(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	p.Player.Char.Skills = domain.Learned{10306: 5}
	specialWeaponEquip(t, s, p, 1321)
	p.Player.Party = 7
	for i := 2; i <= 5; i++ {
		join(t, s, domain.EntityID(i), int64(i)*100, "群净化目标", 100+float64(i), 100)
		s.players[domain.EntityID(i)].Player.Party = 7
	}
	d, _ := s.skills.Get(10306, 5)
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
	view := s.skillBehaviorPayload(p, d)
	if d.SpecialWeaponBaseID != 1321 || view.Area.Radius != 150 || view.Area.Center != "target" {
		t.Fatal("equipment not applied to preview", d.SpecialWeaponBaseID, view)
	}
	before := p.MP
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: p.ID})
	if len(s.casts) != 1 || len(s.casts[0].targets) != 5 || before-p.MP != view.MPCost || s.casts[0].def.CooldownMS != view.CooldownMS {
		t.Fatal("accepted cast differs from preview", len(s.casts), view)
	}
	st := p.Player.Worn.At(domain.SlotWeapon)
	st.MaxDurability = 100
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotWeapon, st)
	d, _ = s.skills.Get(10306, 5)
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
	if d.SpecialWeaponBaseID != 0 || d.Area.Valid() {
		t.Fatal("broken item still modifies skill")
	}
}
func TestSpecialWeaponWarcryAndFollowupAreConsumedOnce(t *testing.T) {
	s, p, _ := specialWeaponActors(t)
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.Level = 100
	p.Player.Char.Skills = domain.Learned{10028: 1, 10005: 1, 10027: 1, 10009: 1}
	specialWeaponEquip(t, s, p, 1624)
	s.specialWeaponCastReleased(p, domain.SkillDef{ID: 10028, SpecialWeaponBaseID: 1624})
	s.specialWeaponLostHP(p, 500)
	s.specialWeaponConsumeCry(p)
	shield := int32(0)
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1800101624 {
			shield = a.ShieldLeft
		}
	})
	if shield != min(int32(100), p.MaxHP/10) {
		t.Fatal("warcry shield does not use actual loss/cap", shield)
	}
	s.specialWeaponConsumeCry(p)
	again := int32(0)
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1800101624 {
			again = a.ShieldLeft
		}
	})
	if again != shield {
		t.Fatal("warcry consumed twice")
	}
	specialWeaponEquip(t, s, p, 1626)
	s.specialWeaponAfterHit(p, p, domain.SkillDef{ID: 10027, SpecialWeaponBaseID: 1626}, true, false)
	d := domain.SkillDef{ID: 10009, SpecialWeaponBaseID: 1626, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.specialWeaponPrepareCast(p, &d)
	if d.Effects[0].DamagePct != 140 {
		t.Fatal("heavy hit did not empower break")
	}
	d.Effects[0].DamagePct = 100
	d.Effects[0].DamagePctNumerator = 0
	d.Effects[0].DamagePctDenominator = 0
	s.specialWeaponPrepareCast(p, &d)
	if d.Effects[0].DamagePct != 100 {
		t.Fatal("followup bonus repeated")
	}
}
func TestSpecialWeaponRemoteIceBlastConsumesOnlyActiveIcebind(t *testing.T) {
	s, p, dst := specialWeaponActors(t)
	s.skills, s.statuses = realV5SkillFixtures(t)
	p.Player.Char.Skills = domain.Learned{10428: 1, 10410: 1}
	freeze := domain.StatusDef{ID: domain.StatusFreeze, DurationSec: 20, SkillSnapshotID: 10428, Level: 5}
	dst.Status.Apply(freeze, s.tick, p.ID)
	d, _ := s.skills.Get(10410, 1)
	applySpecialWeaponDefinition(1323, &d)
	s.specialWeaponAim(p, &d, dst.ID)
	if d.Area.Center != domain.SkillAreaCenterTarget || d.SpecialWeaponConsumeFreeze != dst.ID || d.Dist != 450 {
		t.Fatal("remote iceblast targeting not changed", d.Area, d.Dist)
	}
	for _, a := range d.Statuses {
		if a.ID == domain.StatusFreeze {
			t.Fatal("iceblast still freezes")
		}
	}
	s.specialWeaponConsumeFreeze(p, d)
	count := 0
	dst.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusFreeze {
			count++
		}
	})
	if count != 0 {
		t.Fatal("active icebind not consumed")
	}
	freeze.SkillSnapshotID = 10427
	dst.Status.Apply(freeze, s.tick, p.ID)
	d, _ = s.skills.Get(10410, 1)
	applySpecialWeaponDefinition(1323, &d)
	s.specialWeaponAim(p, &d, dst.ID)
	if d.SpecialWeaponConsumeFreeze != 0 {
		t.Fatal("passive freeze wrongly qualifies")
	}
}
func TestSpecialWeaponSmokePersistsAndCleansUp(t *testing.T) {
	s, p, dst := specialWeaponActors(t)
	s.skills, s.statuses = realV5SkillFixtures(t)
	p.Player.Char.Skills = domain.Learned{10224: 1}
	specialWeaponEquip(t, s, p, 1825)
	d, _ := s.skills.Get(10224, 1)
	applySpecialWeaponDefinition(1825, &d)
	s.specialWeaponCastReleased(p, d)
	if !entityInvisible(p) {
		t.Fatal("smoke did not grant stealth")
	}
	s.specialWeaponCastFinished(p, d, nil)
	s.tick += domain.Ticks(1000)
	s.stepSpecialWeaponSmoke()
	count := 0
	dst.Status.Each(func(a domain.Active) {
		if a.Def.SpecialWeaponSourceBase == 1825 {
			count++
		}
	})
	if count != 2 {
		t.Fatal("persistent smoke did not apply native debuffs", count)
	}
	dst.Pos.X += 500
	s.tick += domain.Ticks(1000)
	s.stepSpecialWeaponSmoke()
	count = 0
	dst.Status.Each(func(a domain.Active) {
		if a.Def.SpecialWeaponSourceBase == 1825 {
			count++
		}
	})
	if count != 0 {
		t.Fatal("outside smoke retained zone debuffs")
	}
	s.tick += domain.Ticks(6000)
	s.stepSpecialWeaponSmoke()
	if len(s.specialWeapons.smoke) != 0 {
		t.Fatal("expired zone retained")
	}
}

func TestSpecialWeaponStealPreservesExactInstanceBalance(t *testing.T) {
	s, p, dst := specialWeaponActors(t)
	a := domain.Active{Source: dst.ID, ExpireAt: s.tick + 15, NextTickAt: s.tick + 7, ShieldLeft: 300, PeriodicDamageRemainder100: 37, Def: domain.StatusDef{ID: domain.StatusDamageShield, Level: 5, DurationSec: 20, DamageShield: 1000, SkillSnapshotID: 10320, TypeKnown: true, Type: domain.StatusGood}}
	s.specialWeaponAfterDispel(p, dst, domain.SkillDef{ID: 10433, SpecialWeaponBaseID: 1324}, []domain.StatusID{domain.StatusDamageShield}, []domain.Active{a})
	found := false
	p.Status.Each(func(got domain.Active) {
		if got.Def.ID == a.Def.ID {
			found = true
			if got.ExpireAt != a.ExpireAt || got.NextTickAt != a.NextTickAt || got.ShieldLeft != 300 || got.PeriodicDamageRemainder100 != 37 || got.Def.SkillSnapshotID != 0 {
				t.Fatal("stolen buff was recast/refilled", got)
			}
		}
	})
	if !found {
		t.Fatal("stolen buff missing")
	}
}
func TestSpecialWeaponStealthBonusMultipliesNativeBonusOnlyOnce(t *testing.T) {
	s, p, _ := specialWeaponActors(t)
	d := domain.StatusDef{ID: domain.StatusStealth, DurationSec: 30, Invisible: true, StealthDamagePct: 35, SpecialWeaponNextBasicPct: 60}
	p.Status = domain.NewStatusSet()
	p.Status.Apply(d, s.tick, p.ID)
	blow := combat.Blow{SkillPct: 100}
	s.prepareBasicAttackStatuses(p, &blow)
	if blow.SkillPct != 216 {
		t.Fatal("weapon60% must multiply native135%, not add60points", blow.SkillPct)
	}
	blow = combat.Blow{SkillPct: 100}
	s.prepareBasicAttackStatuses(p, &blow)
	if blow.SkillPct != 135 {
		t.Fatal("one-shot weapon bonus repeated", blow.SkillPct)
	}
}

func TestSpecialWeaponWarcryAfterWindowAndWeaponSwap(t *testing.T) {
	s, p, _ := specialWeaponActors(t)
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.Skills = domain.Learned{10028: 1}
	specialWeaponEquip(t, s, p, 1624)
	s.specialWeaponCastReleased(p, domain.SkillDef{ID: 10028, SpecialWeaponBaseID: 1624})
	s.specialWeaponLostHP(p, 500)
	s.tick += domain.Ticks(4000)
	s.specialWeaponLostHP(p, 500)
	if s.specialWeapons.players[p.ID].cryDamage != 500 {
		t.Fatal("damage collected after three-second window")
	}
	s.specialWeaponConsumeCry(p)
	got := int32(0)
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1800101624 {
			got = a.ShieldLeft
		}
	})
	if got != min(int32(100), p.MaxHP/10) {
		t.Fatal("stored charge did not survive until next attack", got)
	}
	s.specialWeaponCastReleased(p, domain.SkillDef{ID: 10028, SpecialWeaponBaseID: 1624})
	s.specialWeaponLostHP(p, 500)
	p.Status.Remove(1800101624)
	st := p.Player.Worn.At(domain.SlotWeapon)
	p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.specialWeaponConsumeCry(p)
	got = 0
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1800101624 {
			got = a.ShieldLeft
		}
	})
	if got != 0 {
		t.Fatal("swapping away and back retained warcry charge")
	}
}

// Every approved profession branch is checked against real skills, rather than
// considering a few representative examples evidence of catalogue completeness.
func TestSpecialWeaponEveryApprovedBranch(t *testing.T) {
	var rows []struct {
		Item       domain.ItemID    `json:"item"`
		Name       string           `json:"name"`
		Level      int32            `json:"level"`
		Profession int32            `json:"profession"`
		Skills     []domain.SkillID `json:"skills"`
	}
	raw, err := os.ReadFile("testdata/special_weapon_approved_branches.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 56 {
		t.Fatal("approved branch count", len(rows))
	}
	skills, _ := realV5SkillFixtures(t)
	for _, row := range rows {
		t.Run(fmt.Sprintf("%d/prof%d", row.Item, row.Profession), func(t *testing.T) {
			for _, id := range row.Skills {
				first, ok := skills.Get(id, 1)
				if !ok {
					t.Fatal("missing real skill", id)
				}
				if first.LevelNeed > row.Level {
					t.Fatalf("skill %d starts at %d, weapon is %d", id, first.LevelNeed, row.Level)
				}
				for _, rank := range []int32{1, 5, 20, 100} {
					original, ok := skills.Get(id, rank)
					if !ok {
						t.Fatal("missing grown skill", id, rank)
					}
					before, _ := json.Marshal(original)
					d := original
					applySpecialWeaponDefinition(row.Item, &d)
					if d.SpecialWeaponBaseID != row.Item {
						t.Fatal("unwired approved branch", id, rank, original.Prof)
					}
					after, _ := json.Marshal(original)
					if string(before) != string(after) {
						t.Fatal("mutated shared native definition", id, rank)
					}
					d = original
					d.Prof = 99
					applySpecialWeaponDefinition(row.Item, &d)
					if d.SpecialWeaponBaseID != 0 {
						t.Fatal("cross-profession activation")
					}
				}
			}
		})
	}
}

func TestSpecialWeaponPersistentStatusDowngradeKeepsWeaponSnapshot(t *testing.T) {
	for _, tc := range []struct {
		base  domain.ItemID
		skill domain.SkillID
		prof  int32
		power int32
	}{{1026, 10107, 2, 0}, {1823, 10208, 3, 0}, {1825, 10224, 3, 0}, {1326, 10322, 4, 0}, {1326, 10320, 4, 25}} {
		t.Run(fmt.Sprintf("%d/%d", tc.base, tc.skill), func(t *testing.T) {
			s, p := behaviorAcceptancePlayer(t, tc.prof)
			p.Player.Char.Skills = domain.Learned{tc.skill: 5}
			specialWeaponEquip(t, s, p, tc.base)
			d, _ := s.skills.Get(tc.skill, 5)
			applySpecialWeaponDefinition(tc.base, &d)
			d.SpecialWeaponStatusPowerPct = tc.power
			d.SnapshotRawRanks = domain.Learned{tc.skill: 5}
			d.SnapshotHardRanks = d.SnapshotRawRanks.Clone()
			if len(d.Statuses) == 0 {
				t.Fatal("native status fixture absent")
			}
			app := d.Statuses[0]
			old, ok := s.resolveSkillStatusDef(d, app, 0)
			if !ok {
				t.Fatal("status unresolved")
			}
			stampSkillStatusSnapshot(&old, d)
			p.Player.Char.Skills[tc.skill] = 1
			s.invalidateCombatAttributes(p)
			holder := p
			if !app.TargetType.Matches(p.Kind) {
				holder = &entity.Entity{Kind: domain.KindMonster}
			}
			got, keep, changed := s.resolvePersistentStatus(p, holder, old)
			if !keep || !changed {
				t.Fatal("downgrade did not rebuild")
			}
			expected, _ := s.skills.Get(tc.skill, 1)
			applySpecialWeaponDefinition(tc.base, &expected)
			expected.SpecialWeaponStatusPowerPct = tc.power
			want, ok := s.resolveSkillStatusDef(expected, expected.Statuses[0], 0)
			if !ok {
				t.Fatal("expected status unresolved")
			}
			if got.SpecialWeaponSourceBase != want.SpecialWeaponSourceBase || got.SpecialWeaponBuffBonusPct != want.SpecialWeaponBuffBonusPct || got.SpecialWeaponStatusPowerPct != tc.power || got.SpecialWeaponNextBasicPct != want.SpecialWeaponNextBasicPct || !reflect.DeepEqual(got.Affixes, want.Affixes) || got.DurationSec != want.DurationSec {
				t.Fatalf("weapon snapshot lost on downgrade: got=%+v want=%+v", got, want)
			}
		})
	}
}

func TestSpecialWeaponConditionalAndSecondaryHooks(t *testing.T) {
	s, p, dst := specialWeaponActors(t)
	for _, tc := range []struct {
		base    domain.ItemID
		hp, pct int32
	}{{1028, 49, 145}, {1028, 50, 100}, {1527, 29, 180}, {1527, 30, 100}} {
		dst.MaxHP = 100
		dst.HP = tc.hp
		d := domain.SkillDef{SpecialWeaponBaseID: tc.base, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
		s.specialWeaponBeforeHit(p, dst, &d)
		if d.Effects[0].DamagePct != tc.pct {
			t.Fatal("conditional threshold", tc, d.Effects)
		}
	}
	dst.HP = 100
	dst.Status = domain.NewStatusSet()
	for _, id := range []domain.StatusID{1010, 1011} {
		dst.Status.Apply(domain.StatusDef{ID: id, Level: 1, DurationSec: 30}, s.tick, dst.ID)
	}
	s.specialWeaponAfterHit(p, dst, domain.SkillDef{SpecialWeaponBaseID: 1027}, true, false)
	count := 0
	dst.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1010 || a.Def.ID == 1011 {
			count++
		}
	})
	if count != 1 {
		t.Fatal("must dispel exactly one defense", count)
	}
	for _, tc := range []struct {
		base   domain.ItemID
		status domain.StatusID
		value  int32
	}{{1525, 1800101525, -15}, {1925, domain.StatusSlow, -30}, {1928, domain.StatusSlow, -70}} {
		dst.Status = domain.NewStatusSet()
		s.specialWeaponAfterHit(p, dst, domain.SkillDef{SpecialWeaponBaseID: tc.base}, true, false)
		found := false
		dst.Status.Each(func(a domain.Active) {
			if a.Def.ID == tc.status && len(a.Def.Affixes) > 0 && a.Def.Affixes[0].Value == tc.value {
				found = true
			}
		})
		if !found {
			t.Fatal("secondary debuff absent", tc)
		}
	}
	dst.Status = domain.NewStatusSet()
	s.specialWeaponAfterHit(p, dst, domain.SkillDef{SpecialWeaponBaseID: 1320, Prof: 5}, true, false)
	slow := false
	dst.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusSlow && a.Def.DurationSec == 2 {
			slow = true
		}
	})
	if !slow {
		t.Fatal("ice arrow native slow absent")
	}
	s.specialWeaponCastFinished(p, domain.SkillDef{SpecialWeaponBaseID: 1526, ID: 10127}, []domain.EntityID{2, 3, 4})
	move := false
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1800101526 {
			move = true
		}
	})
	if !move {
		t.Fatal("three-target taunt speed absent")
	}
	p.Player.StartSkillCooldown(10208, s.tick, 100)
	s.specialWeaponAfterHit(p, dst, domain.SkillDef{SpecialWeaponBaseID: 1824, ID: 10209}, true, true)
	if !p.Player.SkillReadyAt(10208, s.tick+50) || p.Player.SkillReadyAt(10208, s.tick+49) {
		t.Fatal("backstab kill cooldown not halved")
	}
	p.Player.Char.Race = domain.Assassin
	specialWeaponEquip(t, s, p, 1821)
	p.Player.Char.Skills = domain.Learned{10203: 1}
	if s.specialWeaponCounterDamage(p, 100) != 150 {
		t.Fatal("counter bonus")
	}
	blow := combat.Blow{SkillPct: 100}
	s.specialWeaponBasicAttack(p, &blow)
	if blow.SkillPct != 90 {
		t.Fatal("counter weapon basic tradeoff", blow.SkillPct)
	}
}

func TestSpecialWeaponAllApprovedGeometryRatios(t *testing.T) {
	skills, _ := realV5SkillFixtures(t)
	for _, tc := range []struct {
		item  domain.ItemID
		skill domain.SkillID
		field string
		pct   int32
	}{{1520, 10117, "dist", 140}, {1524, 10121, "radius", 135}, {1526, 10127, "radius", 160}, {1620, 10005, "width", 160}, {1621, 10017, "dist", 140}, {1623, 10006, "angle", 150}, {1625, 10021, "width", 200}, {1625, 10021, "length", 80}, {1627, 10022, "radius", 80}, {1822, 10213, "dist", 120}, {1924, 10213, "dist", 150}, {1924, 10213, "trap", 120}, {1926, 10206, "radius", 80}, {1321, 10410, "radius", 140}, {1326, 10412, "radius", 120}} {
		for _, rank := range []int32{1, 5, 20, 100} {
			old, _ := skills.Get(tc.skill, rank)
			d := old
			applySpecialWeaponDefinition(tc.item, &d)
			value := func(x domain.SkillDef) int32 {
				switch tc.field {
				case "dist":
					return x.Dist
				case "width":
					return x.Area.Width
				case "length":
					return x.Area.Length
				case "angle":
					if x.Area.Shape == domain.SkillAreaCircle {
						return 360
					}
					return x.Area.Angle
				case "trap":
					for _, e := range x.DirectEffects() {
						if e.Kind == domain.EffectPlaceTrap {
							return e.TrapRadius
						}
					}
				}
				return x.Area.Radius
			}
			want := int32(int64(value(old)) * int64(tc.pct) / 100)
			if tc.field == "angle" {
				want = min(int32(360), want)
			}
			if got := value(d); got != want {
				t.Fatalf("%d rank%d %s=%d want%d", tc.item, rank, tc.field, got, want)
			}
		}
	}
}

func TestSpecialWeaponAllApprovedBranchesActivateWithCorrectItemIdentity(t *testing.T) {
	var rows []struct {
		Item       domain.ItemID    `json:"item"`
		Level      int32            `json:"level"`
		Profession int32            `json:"profession"`
		Skills     []domain.SkillID `json:"skills"`
	}
	data, err := os.ReadFile("testdata/special_weapon_approved_branches.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	unique := map[domain.ItemID]bool{}
	task := map[domain.ItemID]bool{}
	hard := map[domain.ItemID]bool{}
	for _, row := range rows {
		t.Run(fmt.Sprintf("%d/prof%d", row.Item, row.Profession), func(t *testing.T) {
			unique[row.Item] = true
			wanted := domain.HardDungeonItemOffset + row.Item
			if row.Level == 14 {
				wanted = row.Item
				task[row.Item] = true
				if domain.EnhancedWeaponBaseID(domain.HardDungeonItemOffset+row.Item) != 0 {
					t.Fatal("task weapon fabricated a hard variant")
				}
			} else {
				hard[row.Item] = true
				if domain.EnhancedWeaponBaseID(row.Item) != 0 {
					t.Fatal("ordinary dungeon original gained mechanism")
				}
			}
			if domain.EnhancedWeaponBaseID(wanted) != row.Item {
				t.Fatal("approved branch not activated", wanted, row.Item)
			}
			s, p := behaviorAcceptancePlayer(t, row.Profession)
			p.Player.Char.Skills = domain.Learned{}
			for _, id := range row.Skills {
				p.Player.Char.Skills[id] = 1
			}
			specialWeaponEquip(t, s, p, row.Item)
			if got := s.specialWeapon(p); got != row.Item {
				t.Fatal("functional equipment identity", got, row.Item)
			}
			for _, id := range row.Skills {
				d, _ := s.skills.Get(id, 1)
				s.applySpecialWeaponModifiers(p, &d, p.Player.Char.Skills)
				if d.SpecialWeaponBaseID != row.Item {
					t.Fatal("equipped branch did not modify learned skill", id)
				}
			}
			st := p.Player.Worn.At(domain.SlotWeapon)
			st.MaxDurability = 100
			st.Durability = 0
			p.Player.Worn.Set(domain.SlotWeapon, st)
			if s.specialWeapon(p) != 0 {
				t.Fatal("broken authorized weapon kept mechanism")
			}
			st.Durability = 100
			p.Player.Worn.Set(domain.SlotWeapon, st)
			if row.Level != 14 {
				def, _ := s.itemDef(st.Item)
				def.ID = row.Item
				s.items[row.Item] = def
				st.Item = row.Item
				p.Player.Worn.Set(domain.SlotWeapon, st)
				s.invalidateCombatAttributes(p)
				if s.specialWeapon(p) != 0 {
					t.Fatal("ordinary dungeon weapon activated through worn path")
				}
			}
		})
	}
	if len(rows) != 56 || len(unique) != 48 || len(task) != 6 || len(hard) != 42 {
		t.Fatal("activation coverage counts", len(rows), len(unique), len(task), len(hard))
	}
}

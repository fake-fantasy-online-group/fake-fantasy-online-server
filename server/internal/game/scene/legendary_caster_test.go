package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func legendaryCasterFixture(t *testing.T, id string) (*Scene, *legendaryContext) {
	t.Helper()
	s := newCombatScene(t)
	join(t, s, 1, 100, "caster", 100, 100)
	p := s.players[1]
	p.Player.Char.Level = 100
	p.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.Race = domain.Race(4)
	if id[:2] == "YS" {
		p.Player.Char.Race = domain.Race(3)
	}
	p.Player.Char.Skills = domain.Learned{}
	p.HP = 10000
	p.MaxHP = 10000
	p.MP = 1000
	p.MaxMP = 1000
	p.Stats = domain.Stats{MAtk: 100, MinAtk: 100, MaxAtk: 100, Hit: 100000}
	s.skills, s.statuses = realV5SkillFixtures(t)
	for key, d := range s.skills {
		if key.Level == 1 && d.Prof == int32(p.Player.Char.Race)+1 {
			p.Player.Char.Skills[key.ID] = 1
		}
	}
	if s.items == nil {
		s.items = map[domain.ItemID]domain.ItemDef{}
	}
	slot := domain.SlotWeapon
	if id[3] == 'D' {
		slot = domain.SlotBody
	}
	s.items[990001] = domain.ItemDef{ID: 990001, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1 << uint8(p.Player.Char.Race)}}}
	p.Player.Worn.Set(slot, domain.Stack{UID: 991, Item: 990001, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})

	m := putMonster(s, 900, 100000, domain.Stats{}, 130, 100)
	m.Status = domain.NewStatusSet()
	s.invalidateCombatAttributes(p)
	power, _ := domain.LegendaryPower(id)
	return s, &legendaryContext{Scene: s, Owner: p, Target: m, Primary: m.ID, Power: power, SourceUID: 991, RootID: 1, NowMS: 1000, Blow: combat.Blow{Origin: domain.DamageNormalOrigin}, Hit: event.DamageDealt{Amount: 100}}
}
func TestLegendaryCasterDefinitionModifiers(t *testing.T) {
	for _, tc := range []struct {
		id     string
		skill  domain.SkillID
		mp, cd int32
	}{{"SS-A20", 10412, 100, 80}, {"SS-U01", 10432, 85, 0}, {"YS-U01", 10303, 80, 0}, {"YS-U02", 10322, 70, 100}} {
		t.Run(tc.id, func(t *testing.T) {
			_, c := legendaryCasterFixture(t, tc.id)
			original, _ := c.Scene.skills.Get(tc.skill, 1)
			d := original
			legendaryHandlers[tc.id].PrepareSkill(c, &d)
			c.Scene.legendaryFinalizeSkill(&d)
			if d.MPCost != (original.MPCost*tc.mp+99)/100 || d.CooldownMS != specialScale(original.CooldownMS, tc.cd) {
				t.Fatal("resource modifier", d.MPCost, d.CooldownMS)
			}
			again, _ := c.Scene.skills.Get(tc.skill, 1)
			if !reflect.DeepEqual(original, again) {
				t.Fatal("mutated learned catalog")
			}
			if tc.id == "SS-U01" {
				for _, e := range d.DirectEffects() {
					if e.Kind == domain.EffectTeleportPoint && e.ChanceBP != 10000 {
						t.Fatal("teleport chance")
					}
				}
			}
		})
	}
}
func TestLegendaryCasterBurnBurstsPreserveSourceAndDuration(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		pct   int32
	}{{"SS-A02", 10401, 80}, {"SS-A11", 10409, 100}, {"SS-A16", 10405, 160}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			c.Skill = tc.skill
			c.Target.Status = domain.NewStatusSet()
			burn := domain.StatusDef{ID: 1036, DurationSec: 10, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModeAbsolute}}}
			c.Target.Status.Apply(burn, s.tick, c.Owner.ID)
			before := legendaryCasterBurn(c, c.Target)
			hp := c.Target.HP
			legendaryHandlers[tc.id].SkillResolved(c)
			if c.Target.HP != hp-int32(before)*tc.pct/100 {
				t.Fatal("remaining burst", hp-c.Target.HP, before)
			}
			if legendaryCasterBurn(c, c.Target) != before {
				t.Fatal("burn consumed or shortened")
			}
			hp = c.Target.HP
			legendaryHandlers[tc.id].SkillResolved(c)
			if c.Target.HP != hp {
				t.Fatal("duplicate within root")
			}
			c.RootID++
			c.Blow.ProcDepth = 1
			legendaryHandlers[tc.id].SkillResolved(c)
			if c.Target.HP != hp {
				t.Fatal("recursive burst")
			}
		})
	}
}
func TestLegendaryCasterSecondLightImmediateNoTransfer(t *testing.T) {
	_, c := legendaryCasterFixture(t, "YS-A02")
	c.Skill = legendaryLightBolt
	hp := c.Target.HP
	legendaryHandlers[c.Power.ID].AfterHit(c)
	if c.Target.HP != hp-85 {
		t.Fatal("not immediate85", hp-c.Target.HP)
	}
	legendaryHandlers[c.Power.ID].AfterHit(c)
	if c.Target.HP != hp-85 {
		t.Fatal("same cast repeated")
	}
	c.RootID++
	c.Target.HP = 0
	other := putMonster(c.Scene, 901, 1000, domain.Stats{}, 140, 100)
	legendaryHandlers[c.Power.ID].AfterHit(c)
	if other.HP != 1000 {
		t.Fatal("transferred dead echo")
	}
}

func TestLegendaryCasterRegenAndDispelStealFullDuration(t *testing.T) {
	_, c := legendaryCasterFixture(t, "YS-U01")
	c.Skill = 10303
	d := domain.StatusDef{DurationSec: 12, HPChanges: []domain.ResourceChange{{Value: 20, Mode: domain.ModeAbsolute}}}
	legendaryHandlers[c.Power.ID].StatusApplied(c, &d)
	if d.DurationSec != 12 || c.Scene.legendaryPeriodicAmount(domain.Active{Def: d}, int64(d.HPChanges[0].Value)) != 30 {
		t.Fatal(d)
	}
	_, c = legendaryCasterFixture(t, "SS-U02")
	c.Skill = 10433
	shield := domain.Active{Def: domain.StatusDef{ID: 1021, Level: 1, DurationSec: 10, DamageShield: 100}, ExpireAt: c.Scene.tick + domain.Ticks(10000), ShieldLeft: 30}
	legendaryHandlers[c.Power.ID].Dispelled(c, []domain.Active{shield})
	found := false
	c.Owner.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == 1021 {
			found = true
			if a.ShieldLeft != 100 || a.ExpireAt != c.Scene.tick+domain.Ticks(10000) {
				t.Fatal("stolen shield must refill once with full base duration", a)
			}
		}
	})
	if !found {
		t.Fatal("not transferred")
	}
}

var _ *entity.Entity

func TestLegendaryCasterNativeCastEchoAndBurn(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		prof  uint8
		want  int32
	}{{"YS-A02", 10319, 4, 185}, {"SS-A02", 10401, 5, 180}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			d := domain.SkillDef{ID: tc.skill, Level: 1, Prof: int32(tc.prof), Kind: domain.SkillSingle, HurtType: domain.HurtHoly, Dist: 500, MPCost: 20, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			s.skills = skillTable(d)
			c.Owner.Player.Char.Skills = domain.Learned{tc.skill: 1}
			s.invalidateCombatAttributes(c.Owner)
			if tc.id == "SS-A02" {
				c.Target.Status.Apply(domain.StatusDef{ID: 1036, DurationSec: 10, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModeAbsolute}}}, s.tick, c.Owner.ID)
			}
			hp := c.Target.HP
			if tc.id == "SS-A02" && legendaryCasterBurn(c, c.Target) == 0 {
				t.Fatal("fixture burn absent before cast", c.Target.Status)
			}
			s.onUseSkill(UseSkill{ID: c.Owner.ID, Skill: tc.skill, Target: c.Target.ID})
			if len(s.casts) != 1 {
				t.Fatal("native cast rejected")
			}
			s.stepCasts()
			if got := hp - c.Target.HP; got != tc.want {
				t.Fatal("native cast total", got, tc.want, "burn", legendaryCasterBurn(c, c.Target), "states", s.legendaryState().states, "active", domain.ActiveLegendaryPowers(s.functionalWorn(c.Owner), s.itemDef, c.Owner.Player.Char))
			}
			if c.Owner.MP != 980 {
				t.Fatal("paid mana", c.Owner.MP)
			}
		})
	}
}

func legendaryCasterEquipBuild(t *testing.T, s *Scene, p *entity.Entity, ids ...string) {
	t.Helper()
	slots := []domain.EquipSlot{domain.SlotWeapon, domain.SlotBody, domain.SlotHead}
	for i, id := range ids {
		item := domain.ItemID(990010 + i)
		slot := slots[i]
		s.items[item] = domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1 << uint8(p.Player.Char.Race)}}}
		p.Player.Worn.Set(slot, domain.Stack{UID: int64(1000 + i), Item: item, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
	}
}
func legendaryCasterCastNow(t *testing.T, s *Scene, p *entity.Entity, skill domain.SkillID, target domain.EntityID) {
	t.Helper()
	s.invalidateCombatAttributes(p)
	rank, _ := s.skillLevelForUse(p, skill)
	d, ok := s.skills.Get(skill, rank)
	if !ok {
		t.Fatal("missing native skill", skill)
	}
	d.PrepareMS = 0
	d.CooldownMS = 0
	d.MPCost = 0
	d.ImpactDelayMS = 0
	d.ProjectileSpeedPXPerSec = 0
	s.skills[domain.SkillKey{ID: skill, Level: rank}] = d
	s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, Target: target})
	if len(s.casts) == 0 {
		t.Fatal("native build cast rejected", skill)
	}
	s.stepCasts()
}
func TestLegendaryCasterSynergyFireNativeBurnBeforeSettlement(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A17")
	legendaryCasterEquipBuild(t, s, c.Owner, "SS-A17", "SS-A16", "SS-D09")
	before := c.Target.HP
	legendaryCasterCastNow(t, s, c.Owner, legendaryFlame, c.Target.ID)
	remaining := legendaryCasterBurn(c, c.Target)
	if remaining <= 0 {
		t.Fatal("windflame did not apply native burn")
	}
	if int64(before-c.Target.HP) < remaining*160/100 {
		t.Fatal("remaining settlement missing", before-c.Target.HP, remaining)
	}
	a, ok := legendaryCasterSourceStatus(c.Target, 1036, c.Owner.ID)
	if !ok || a.ExpireAt-s.tick != domain.Ticks(40000) {
		t.Fatal("stronger native high-heat burn duration changed", a.ExpireAt-s.tick)
	}
	incoming := *c
	incoming.Power, _ = domain.LegendaryPower("SS-D09")
	if n := legendaryHandlers["SS-D09"].BeforeIncoming(&incoming, 100); n != 60 {
		t.Fatal("burn ownership mitigation", n)
	}
}
func TestLegendaryCasterSynergyHealFullHPStillSplits(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-U04")
	legendaryCasterEquipBuild(t, s, c.Owner, "YS-U04", "YS-A23", "YS-A01")
	// Only the primary is within the heal-origin 250 radius; the side is
	// reached solely by the child bolt's 180-radius split.
	s.aoi.Move(c.Target, domain.Pos{MapID: c.Owner.Pos.MapID, X: 330, Y: 100})
	side := putMonster(s, 901, 100000, domain.Stats{}, 400, 100)
	far := putMonster(s, 902, 100000, domain.Stats{}, 900, 100)
	before, sideBefore, farBefore := c.Target.HP, side.HP, far.HP
	legendaryCasterCastNow(t, s, c.Owner, 10301, c.Owner.ID)
	primaryDamage, sideDamage := before-c.Target.HP, sideBefore-side.HP
	if primaryDamage <= 0 || sideDamage <= 0 {
		t.Fatal("full overheal child or split missing", primaryDamage, sideDamage)
	}
	if sideDamage != primaryDamage*60/100 {
		t.Fatal("split does not inherit healing base at60 percent", primaryDamage, sideDamage)
	}
	if far.HP != farBefore {
		t.Fatal("split exceeded native transformed area")
	}
	if c.Owner.HP != c.Owner.MaxHP {
		t.Fatal("healing was sacrificed")
	}
}
func TestLegendaryCasterSynergyFreezeImmediateAndRemoteBurst(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A24")
	legendaryCasterEquipBuild(t, s, c.Owner, "SS-A24", "SS-A12", "SS-A15")
	at := c.Target.Pos
	at.X = 400
	s.aoi.Move(c.Target, at)
	before := c.Target.HP
	legendaryCasterCastNow(t, s, c.Owner, legendaryFreeze, c.Target.ID)
	if c.Target.HP >= before {
		t.Fatal("freeze has no immediate child damage")
	}
	if !legendaryCasterHas(c.Target, domain.StatusFreeze) {
		t.Fatal("freeze control lost")
	}
	before = c.Target.HP
	legendaryCasterCastNow(t, s, c.Owner, legendaryIceBurst, c.Target.ID)
	if c.Target.HP >= before {
		t.Fatal("remote burst missed target")
	}
}

func TestLegendaryCasterSynergyDispelAttackStealDefense(t *testing.T) {
	run := func(withBuff bool) int32 {
		s, c := legendaryCasterFixture(t, "SS-U02")
		legendaryCasterEquipBuild(t, s, c.Owner, "SS-U02", "SS-A28", "SS-D03")
		s.rng = rand.New(rand.NewSource(3))
		if withBuff {
			c.Target.Status.Apply(domain.StatusDef{ID: 1037, Level: 1, DurationSec: 20, PhysicalReflectPct: 20, TypeKnown: true, Type: domain.StatusGood}, s.tick, c.Target.ID)
		}
		before := c.Target.HP
		legendaryCasterCastNow(t, s, c.Owner, 10433, c.Target.ID)
		if withBuff {
			a, ok := legendaryCasterSourceStatus(c.Owner, 1037, c.Owner.ID)
			if !ok {
				t.Fatal("defensive magic not stolen")
			}
			if a.Def.PhysicalReflectPct != 30 {
				t.Fatal("stolen defensive magic not strengthened", a.Def.PhysicalReflectPct)
			}
		}
		return before - c.Target.HP
	}
	without, with := run(false), run(true)
	if without <= 0 || with <= without {
		t.Fatal("dispel damage absent or failed same-cast stolen state", without, with)
	}
}
func TestLegendaryCasterSynergyFocusedGroupShieldBurst(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-U02")
	legendaryCasterEquipBuild(t, s, c.Owner, "YS-U02", "YS-D03", "YS-A21")
	for i := 2; i <= 3; i++ {
		join(t, s, domain.EntityID(i), int64(100+i), "ally", 100+float64(i)*10, 100)
		s.players[domain.EntityID(i)].Player.Party = 1
	}
	c.Owner.Player.Party = 1
	legendaryCasterCastNow(t, s, c.Owner, 10322, c.Owner.ID)
	before := c.Target.HP
	legendaryCasterCastNow(t, s, c.Owner, 10320, c.Owner.ID)
	var shield int32
	for i := 1; i <= 3; i++ {
		p := s.players[domain.EntityID(i)]
		found := false
		p.Status.EachEffective(func(a domain.Active) {
			if a.Source == c.Owner.ID && a.Def.SkillSnapshotID == 10320 {
				found = true
				if shield == 0 {
					shield = a.Def.DamageShield
				}
				if a.ShieldLeft != shield {
					t.Fatal("inconsistent shield", i, a.ShieldLeft, shield)
				}
			}
		})
		if !found {
			t.Fatal("group shield missing", i)
		}
	}
	if got := before - c.Target.HP; got != shield*80/100 {
		t.Fatal("shield burst did not deduplicate overlapping allies", got, shield)
	}
	if !legendaryCasterNativeReady(c.Owner, true, false, false) {
		t.Fatal("learned permanent focus was consumed")
	}
}
func TestLegendaryCasterSynergyExorcismWeaknessPull(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A09")
	legendaryCasterEquipBuild(t, s, c.Owner, "YS-A09", "YS-A17", "YS-A19")
	side := putMonster(s, 901, 100000, domain.Stats{}, 330, 100)
	side.Status = domain.NewStatusSet()
	before := side.HP
	legendaryCasterCastNow(t, s, c.Owner, 10325, c.Target.ID)
	if side.HP >= before {
		t.Fatal("shackle area damage missing")
	}
	if sqDist(side.Pos, c.Target.Pos) > 80*80+1 {
		t.Fatal("shackle did not pull", side.Pos, c.Target.Pos)
	}
	legendaryCasterCastNow(t, s, c.Owner, 10324, c.Target.ID)
	if !legendaryCasterHasSkill(side, 10324) {
		t.Fatal("native weakness lost")
	}
	before = side.HP
	legendaryCasterCastNow(t, s, c.Owner, legendaryExorcism, c.Target.ID)
	if side.HP >= before {
		t.Fatal("all-race exorcism missed gathered enemy")
	}
}

func TestLegendaryCasterNativeBurnKillAndMirrorReflection(t *testing.T) {
	t.Run("burn death", func(t *testing.T) {
		s, c := legendaryCasterFixture(t, "SS-U05")
		c.Owner.MP = 500
		c.Target.HP = 5
		a := domain.Active{Source: c.Owner.ID, Def: domain.StatusDef{ID: 1036, SkillSnapshotID: 10421, SourceSkillID: 10421, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModeAbsolute}}}}
		s.tickStatus(c.Target, a)
		if c.Owner.MP != 600 {
			t.Fatal("native fatal burn refund", c.Owner.MP)
		}
		s.legendaryPeriodicResolved(c.Owner, c.Target, a, event.DamageDealt{Flag: event.DamageFatal})
		if c.Owner.MP != 600 {
			t.Fatal("duplicate death refund", c.Owner.MP)
		}
	})
	t.Run("mirror replacement", func(t *testing.T) {
		s, c := legendaryCasterFixture(t, "SS-A26")
		c.Target.Stats.MAtk = 100
		c.Owner.Status.Apply(domain.StatusDef{ID: 1011, SkillSnapshotID: 10417, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, c.Owner.ID)
		before := c.Target.HP
		hit, raw, reflected := s.resolveStrikeStatuses(c.Target, c.Owner, &combat.HitTracker{}, combat.Blow{SkillID: 10401, Magic: true, HurtType: domain.HurtFire, SkillPct: 100, GuaranteedHit: true})
		s.afterStrikeStatuses(c.Target, c.Owner, hit, raw, reflected)
		if reflected <= 0 || before-c.Target.HP != reflected {
			t.Fatal("mirror reflection doubled or missing", reflected, before-c.Target.HP)
		}
		if reflected != min(raw*120/100, c.Owner.MaxHP*18/100) {
			t.Fatal("mirror percentage/cap", raw, reflected)
		}
	})
}
func TestLegendaryCasterDualNativeShieldsAndNoTimer(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-D07")
	s.statusOverlay = domain.StatusOverlay{domain.OverlayKey{Existing: 1037, Incoming: 1038}: domain.OverlayMustReplace, domain.OverlayKey{Existing: 1038, Incoming: 1037}: domain.OverlayMustReplace}
	if !s.applyStatusDef(c.Owner, domain.StatusDef{ID: 1037, SkillSnapshotID: 10425, DurationSec: 20}, c.Owner.ID) || !s.applyStatusDef(c.Owner, domain.StatusDef{ID: 1038, SkillSnapshotID: 10435, DurationSec: 20}, c.Owner.ID) {
		t.Fatal("native shield application rejected")
	}
	if !c.Owner.Status.HasEffective(1037) || !c.Owner.Status.HasEffective(1038) {
		t.Fatal("dual native shields still replace")
	}
	if s.statusOverlay[domain.OverlayKey{Existing: 1037, Incoming: 1038}] != domain.OverlayMustReplace {
		t.Fatal("global overlay mutated")
	}
	for k, st := range s.legendaryState().states {
		if k.Owner == c.Owner.ID && st.ExpiresMS > 0 {
			t.Fatal("equipment timer created", k)
		}
	}
}
func TestLegendaryCasterDamageBonusesAddBeforeChildSettlement(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A12")
	legendaryCasterEquipBuild(t, s, c.Owner, "SS-A13", "SS-A12", "SS-A15")
	c.Target.Status.Apply(domain.StatusDef{ID: domain.StatusFreeze, DurationSec: 20}, s.tick, c.Owner.ID)
	d := domain.SkillDef{ID: legendaryIceBurst, TargetEnemy: true, HurtType: domain.HurtIce, Radius: 200, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
	finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, s.equipmentOffense(c.Owner))
	defer finish()
	s.equipment.root.center = c.Target.Pos
	s.equipment.root.def = d
	d = s.legendaryTargetDefinition(c.Owner, d, c.Target, c.Target.ID, []domain.EntityID{c.Target.ID})
	// 20% remote +85% center +60% frozen are one additive bucket.
	if d.DirectEffects()[0].DamagePct != 265 {
		t.Fatal("multipliers multiplied instead of added", d.DirectEffects()[0])
	}
}

func TestLegendaryCasterControlImmuneStillTakesTransformedDamage(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"SS-A24", 10428}, {"YS-A17", 10324}, {"YS-A19", 10325}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			s.defs = spawn.MapDefs{338: {ID: 338, ImmuneSkills: map[domain.SkillID]struct{}{tc.skill: {}}}}
			before, at := c.Target.HP, c.Target.Pos
			legendaryCasterCastNow(t, s, c.Owner, tc.skill, c.Target.ID)
			if c.Target.HP >= before {
				t.Fatal("control immunity incorrectly blocked transformed damage")
			}
			if c.Target.Pos != at {
				t.Fatal("immune target was displaced")
			}
			if legendaryCasterHasSkill(c.Target, tc.skill) {
				t.Fatal("immune target received native control")
			}
		})
	}
}

// A manually supplied state without invested instant-cast remains one-shot;
// equipment must not turn an unlearned skill into a permanent preparation.
func TestLegendaryCasterUnlearnedManualInstantConsumesAndEchoSingleTarget(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A30")
	side := putMonster(s, 901, 100000, domain.Stats{}, 140, 100)
	side.Status = domain.NewStatusSet()
	c.Owner.Status.Apply(domain.StatusDef{ID: 1028, SkillSnapshotID: 10420, DurationSec: 40, NextCastTimeReductionPct: 100}, s.tick, c.Owner.ID)
	d := domain.SkillDef{ID: 10402, Level: 1, Prof: 5, Kind: domain.SkillArea, HurtType: domain.HurtFire, PrepareMS: 1000, Dist: 500, TargetEnemy: true, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: 180}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.skills = skillTable(d)
	c.Owner.Player.Char.Skills = domain.Learned{10402: 1}
	s.invalidateCombatAttributes(c.Owner)
	before, sideBefore := c.Target.HP, side.HP
	s.onUseSkill(UseSkill{ID: c.Owner.ID, Skill: 10402, Target: c.Target.ID})
	s.stepCasts()
	if c.Owner.Status.HasEffective(1028) {
		t.Fatal("native instant not consumed")
	}
	if sideBefore-side.HP <= 0 || before-c.Target.HP != (sideBefore-side.HP)+(sideBefore-side.HP)*80/100 {
		t.Fatal("echo repeated area or missing", before-c.Target.HP, sideBefore-side.HP)
	}
}

func TestLegendaryCasterNewSupportWavesUseNativeRoots(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"YS-A22", 10306}, {"YS-A24", 10304}, {"YS-A28", 10302}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			side := putMonster(s, 901, 100000, domain.Stats{}, 170, 100)
			side.Status = domain.NewStatusSet()
			before, other := c.Target.HP, side.HP
			legendaryCasterCastNow(t, s, c.Owner, tc.skill, c.Owner.ID)
			if c.Target.HP >= before || side.HP >= other {
				t.Fatal("native support root failed to damage nearby enemies", before-c.Target.HP, other-side.HP)
			}
			if before-c.Target.HP != other-side.HP {
				t.Fatal("wave duplicated primary or varied per friendly target")
			}
		})
	}
}
func TestLegendaryCasterGroupCleanseNativeTargets(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"YS-D02", 10306}, {"YS-D07", 10316}, {"YS-U10", 10318}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			join(t, s, 2, 102, "ally", 220, 100)
			ally := s.players[2]
			c.Owner.Player.Party = 1
			ally.Player.Party = 1
			d, _ := s.skills.Get(tc.skill, 1)
			var status domain.StatusID
			for _, e := range d.DirectEffects() {
				if len(e.RemoveStatusIDs) > 0 {
					status = e.RemoveStatusIDs[0]
					break
				}
			}
			if status == 0 {
				t.Fatal("fixture missing native cleanse list")
			}
			for _, p := range []*entity.Entity{c.Owner, ally} {
				p.Status.Apply(domain.StatusDef{ID: status, Level: 1, DurationSec: 60, TypeKnown: true, Type: domain.StatusBad}, s.tick, c.Target.ID)
			}
			s.rng = rand.New(rand.NewSource(1))
			legendaryCasterCastNow(t, s, c.Owner, tc.skill, c.Owner.ID)
			if tc.id != "YS-D07" && (c.Owner.Status.HasEffective(status) || ally.Status.HasEffective(status)) {
				t.Fatal("guaranteed native group cleanse left status", status)
			}
			// Selection itself must include the legal friend regardless native chance.
			var found bool
			d, _ = s.skills.Get(tc.skill, 1)
			s.legendaryPrepareSkill(c.Owner, &d, c.Owner.ID)
			for _, id := range s.resolveTargets(c.Owner, d, c.Owner.ID, nil) {
				if id == ally.ID {
					found = true
				}
			}
			if !found {
				t.Fatal("group target not selected")
			}
		})
	}
}
func TestLegendaryCasterRemoteRevealAdmission(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-U08")
	d, _ := s.skills.Get(10327, 1)
	s.legendaryPrepareSkill(c.Owner, &d, c.Owner.ID)
	at := domain.Pos{MapID: s.id.MapID, X: 600, Y: 100}
	ids, to := s.resolveSkillTargets(c.Owner, d, 0, &at)
	if len(ids) != 1 || to == nil || *to != at {
		t.Fatal("existing ground aim not admitted", ids, to)
	}
	at.X = 900
	if ids, _ := s.resolveSkillTargets(c.Owner, d, 0, &at); len(ids) != 0 {
		t.Fatal("out-of-range reveal admitted")
	}
}
func TestLegendaryCasterNativeWindupDefenseEndsWithCast(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		pct   int32
	}{{"YS-D06", 10304, 60}, {"YS-D08", 10305, 40}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			h := legendaryHandlers[tc.id]
			if h.BeforeIncoming(c, 100) != 100 {
				t.Fatal("defense outside cast")
			}
			s.casts = append(s.casts, pendingCast{src: c.Owner.ID, def: domain.SkillDef{ID: tc.skill}, releaseAtWall: s.now().Add(time.Second), windup: &castWindup{}})
			if h.BeforeIncoming(c, 100) != tc.pct {
				t.Fatal("defense missing during native windup")
			}
			s.casts[0].windup.finished = true
			if h.BeforeIncoming(c, 100) != 100 {
				t.Fatal("defense outlived native windup")
			}
		})
	}
}

func TestLegendaryCasterReplacementModifiersThroughPipeline(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"SS-A01", 10402}, {"SS-A18", 10405}, {"SS-A23", 10428}, {"SS-U09", 10402}, {"YS-A03", 10319}, {"YS-A08", 10317}, {"YS-A10", 10324}, {"YS-A11", 10312}, {"YS-A12", 10312}, {"YS-A14", 10326}, {"YS-A16", 10326}, {"YS-A18", 10325}, {"YS-A25", 10307}, {"YS-A26", 10321}, {"YS-A28", 10302}, {"YS-D01", 10301}, {"YS-U09", 10305}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			d, ok := s.skills.Get(tc.skill, 1)
			if !ok {
				t.Fatal("missing native")
			}
			before := d
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if reflect.DeepEqual(before, d) || len(d.LegendarySources) != 1 {
				t.Fatal("real equipped pipeline did not apply", tc.id)
			}
			switch tc.id {
			case "SS-A23":
				if d.Area.Radius < 150 || d.SpecialWeaponTargetLimit != 0 {
					t.Fatal("freeze replacement not actual area", d.Area)
				}
			case "YS-A08":
				if d.Area.Shape != domain.SkillAreaRectangle || d.Area.Width != 160 || d.Area.Length != 600 {
					t.Fatal("holy rectangle missing", d.Area)
				}
			case "YS-U09":
				for _, e := range d.DirectEffects() {
					if e.Kind == domain.EffectResurrect && e.HealPctOfMax < 70 {
						t.Fatal("revive threshold")
					}
				}
			default:
				if d.LegendaryModifiers.DamageBP == 0 && d.LegendaryModifiers.HealingBP == 0 && d.LegendaryModifiers.RestoreMPBP == 0 && d.LegendaryModifiers.CooldownReductionBP == 0 && d.LegendaryModifiers.StatusPowerBP == 0 && d.Area == before.Area {
					t.Fatal("no material native modifier")
				}
			}
		})
	}
}
func TestLegendaryCasterExpandedFreezeAppliesRealNativeControls(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A23")
	legendaryCasterEquipBuild(t, s, c.Owner, "SS-A23", "SS-D06", "SS-A24")
	side := putMonster(s, 901, 100000, domain.Stats{}, 350, 100)
	side.Status = domain.NewStatusSet()
	before := side.HP
	legendaryCasterCastNow(t, s, c.Owner, 10428, c.Target.ID)
	if !side.Status.HasEffective(domain.StatusFreeze) || side.HP >= before {
		t.Fatal("additional freeze target lacks native control and child damage")
	}
}
func TestLegendaryCasterDefenseGrantWorksWithoutRecipientGear(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A26")
	legendaryCasterEquipBuild(t, s, c.Owner, "YS-U06", "YS-A26", "YS-D04")
	join(t, s, 2, 102, "ally", 120, 100)
	ally := s.players[2]
	c.Owner.Player.Party = 1
	ally.Player.Party = 1
	legendaryCasterCastNow(t, s, c.Owner, 10321, c.Owner.ID)
	for _, p := range []*entity.Entity{c.Owner, ally} {
		d := domain.SkillDef{ID: 10319, Kind: domain.SkillSingle, HurtType: domain.HurtHoly, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		s.legendaryPrepareSkill(p, &d, c.Target.ID)
		if d.LegendaryModifiers.DamageBP != 4000 {
			t.Fatal("native defense holy grant absent or duplicated", p.ID, d.LegendaryModifiers.DamageBP)
		}
		if p.Status.DamageReductionPct(true) < 35 {
			t.Fatal("defense damage reduction absent")
		}
	}
}

func TestLegendaryCasterSplitRetainsSixtyPercentWithAmplifier(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-U04")
	legendaryCasterEquipBuild(t, s, c.Owner, "YS-A23", "YS-A01", "YS-A03")
	s.aoi.Move(c.Target, domain.Pos{MapID: c.Owner.Pos.MapID, X: 330, Y: 100})
	side := putMonster(s, 901, 100000, domain.Stats{}, 400, 100)
	side.Status = domain.NewStatusSet()
	before, other := c.Target.HP, side.HP
	legendaryCasterCastNow(t, s, c.Owner, 10301, c.Owner.ID)
	mainDamage, sideDamage := before-c.Target.HP, other-side.HP
	if mainDamage <= 0 || sideDamage != mainDamage*60/100 {
		t.Fatal("shape coefficient multiplied or added incorrectly", mainDamage, sideDamage)
	}
}
func TestLegendaryCasterDispelHOTIsImmediateAndBounded(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A03")
	for _, id := range []domain.StatusID{190001, 190002} {
		c.Target.Status.Apply(domain.StatusDef{ID: id, DurationSec: 30, IntervalSec: 1, TypeKnown: true, Type: domain.StatusGood, HPChanges: []domain.ResourceChange{{Value: 20, Mode: domain.ModeAbsolute}}}, s.tick, c.Target.ID)
	}
	before := c.Target.HP
	legendaryCasterCastNow(t, s, c.Owner, 10319, c.Target.ID)
	if c.Target.HP >= before {
		t.Fatal("light bolt lost native attack")
	}
	count := 0
	c.Target.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == 190001 || a.Def.ID == 190002 {
			count++
		}
	})
	if count != 1 {
		t.Fatal("HoT removal not exactly one", count)
	}
}
func TestLegendaryCasterSecondFireballInheritsAmplifierOnce(t *testing.T) {
	run := func(echo bool) int32 {
		s, c := legendaryCasterFixture(t, "SS-A01")
		if echo {
			legendaryCasterEquipBuild(t, s, c.Owner, "SS-A01", "SS-A04")
		}
		before := c.Target.HP
		legendaryCasterCastNow(t, s, c.Owner, 10402, c.Target.ID)
		return before - c.Target.HP
	}
	normal, echo := run(false), run(true)
	if echo != normal+normal*70/100 {
		t.Fatal("fireball echo missed pure modifiers or recursively generated", normal, echo)
	}
}

func TestLegendaryCasterFocusedFireballCountStableWithinRoot(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A05")
	a := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
	b := putMonster(s, 902, 10000, domain.Stats{}, 170, 100)
	finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, domain.EquipmentCombatStats{})
	defer finish()
	ids := []domain.EntityID{c.Target.ID, a.ID, b.ID}
	s.equipment.root.targets = ids
	s.equipment.root.def = domain.SkillDef{ID: 10402}
	d := domain.SkillDef{ID: 10402, Kind: domain.SkillArea, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	main := s.legendaryTargetDefinition(c.Owner, d, c.Target, c.Target.ID, ids)
	if main.DirectEffects()[0].DamagePct != 135 {
		t.Fatal("first target branch")
	}
	c.Target.HP = 0
	side := s.legendaryTargetDefinition(c.Owner, d, a, c.Target.ID, ids)
	if side.DirectEffects()[0].DamagePct != 100 {
		t.Fatal("earlier fatal changed same-root target-count branch", side.DirectEffects()[0].DamagePct)
	}
}
func TestLegendaryCasterMirrorSlowImmunityAlsoGuardsChildStatus(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U08")
	c.Owner.Status.Apply(domain.StatusDef{ID: 190100, SkillSnapshotID: 10417, DurationSec: 30}, s.tick, c.Owner.ID)
	rt := s.legendaryState()
	rt.depth = 1
	rt.child = &legendaryChildFrame{Def: domain.SkillDef{ID: 10409}, Depth: 1}
	slow := domain.StatusDef{ID: domain.StatusSlow, SkillSnapshotID: 10409, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad}
	if s.applyStatusDef(c.Owner, slow, c.Target.ID) {
		t.Fatal("child slow bypassed native mirror immunity")
	}
	c.Owner.Status.Remove(190100)
	if !s.legendaryStatusReceiving(c.Owner, c.Target, &slow) {
		t.Fatal("immunity remained without native mirror")
	}
	rt.depth = 0
	rt.child = nil
}
func TestLegendaryCasterTypedMetadataEquippedAndRemoved(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		mode  string
	}{{"YS-U08", 10327, "point"}, {"YS-D02", 10306, "point"}, {"SS-A03", 10401, "point"}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			c.Owner.Player.EndgameGrowthV5 = true
			c.Owner.Player.EndgameSkillBehavior = true
			native, _ := s.skills.Get(tc.skill, 1)
			beforeMP := c.Owner.MP
			c.Owner.Status.Apply(domain.StatusDef{ID: 190999, DurationSec: 60, NextCastTimeReductionPct: 50}, s.tick, c.Owner.ID)
			states := len(s.legendaryState().states)
			statusBefore := c.Owner.Status.Clone()
			s.rng = rand.New(rand.NewSource(7723))
			expectedRNG := rand.New(rand.NewSource(7723)).Int63()
			r, err := s.skillRanksForDisplay(c.Owner)
			if err != nil {
				t.Fatal(err)
			}
			again, againErr := s.skillRanksForDisplay(c.Owner)
			if againErr != nil || !reflect.DeepEqual(r, again) {
				t.Fatal("repeated forced projection changed output", againErr)
			}
			if !reflect.DeepEqual(statusBefore, c.Owner.Status) || s.rng.Int63() != expectedRNG {
				t.Fatal("projection consumed status or RNG")
			}
			v := r[tc.skill].Contract
			if v == nil || v.Current == nil {
				t.Fatal("typed current metadata absent")
			}
			if v.Current.InputMode != tc.mode {
				t.Fatal("wrong equipped input", v.Current.InputMode, tc.mode)
			}
			if tc.id == "YS-U08" && (v.Current.CastDistance != 600 || v.Current.Area.Center != "point" || !v.Current.PointRequired || v.Current.SelfFallback) {
				t.Fatal("eagle point contract incomplete", v.Current)
			}
			if tc.id == "YS-D02" && v.Current.Area.Radius != 250 {
				t.Fatal("group cleanse metadata missing")
			}
			if tc.id == "SS-A03" && v.Current.Area.Radius != 150 {
				t.Fatal("firebolt circle metadata missing")
			}
			if c.Owner.MP != beforeMP || !c.Owner.Status.HasEffective(190999) || len(s.legendaryState().states) != states {
				t.Fatal("UI preview consumed resources/state")
			}
			for _, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotBody, domain.SlotHead} {
				c.Owner.Player.Worn.Set(slot, domain.Stack{})
			}
			s.invalidateCombatAttributes(c.Owner)
			r, err = s.skillRanksForDisplay(c.Owner)
			if err != nil {
				t.Fatal(err)
			}
			v = r[tc.skill].Contract
			if tc.id == "YS-U08" && (v.Current.InputMode != "self" || v.Current.CastDistance != native.Dist) {
				t.Fatal("unequip did not restore native eagle input", v.Current)
			}
			if tc.id == "YS-D02" && v.Current.Area.Radius == 250 {
				t.Fatal("unequipped group cleanse shape stuck")
			}
			if tc.id == "SS-A03" && v.Current.Area.Radius == 150 {
				t.Fatal("unequipped firebolt shape stuck")
			}
		})
	}
}
func TestLegendaryCasterOptionalEntityCenterMetadata(t *testing.T) {
	for _, tc := range []struct {
		id       string
		skill    domain.SkillID
		friendly bool
	}{{"SS-A12", 10410, false}, {"YS-A05", 10317, true}, {"YS-U05", 10311, true}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			c.Owner.Player.EndgameGrowthV5 = true
			c.Owner.Player.EndgameSkillBehavior = true
			ranks, err := s.skillRanksForDisplay(c.Owner)
			if err != nil {
				t.Fatal(err)
			}
			v := ranks[tc.skill].Contract.Current
			if v.InputMode != "entity" || !v.SelfFallback || !v.TargetSelf || v.PointRequired || v.Area.Center != "target" || v.CastDistance <= 0 {
				t.Fatal("optional entity center not selectable", v)
			}
			if tc.friendly && !v.TargetTeam {
				t.Fatal("friendly center missing target permission")
			}
			if !tc.friendly && !v.TargetEnemy {
				t.Fatal("enemy center missing target permission")
			}
			if tc.id == "SS-A12" {
				d, _ := s.skills.Get(tc.skill, 1)
				s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
				if d.Area.Center != domain.SkillAreaCenterTarget || len(s.resolveTargets(c.Owner, d, c.Target.ID, nil)) == 0 {
					t.Fatal("metadata enemy selection not legal in native resolution")
				}
			}
		})
	}
}

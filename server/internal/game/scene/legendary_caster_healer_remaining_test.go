package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
	"time"
)

func TestLegendaryCasterRemainingHealerTargetBranches(t *testing.T) {
	for _, tc := range []struct {
		id     string
		skill  domain.SkillID
		status domain.SkillID
		bonus  int32
	}{{"YS-A04", 10319, 10324, 9000}, {"YS-A13", 10326, 10325, 8000}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			c.Skill = tc.skill
			c.Def = domain.SkillDef{ID: tc.skill, Kind: domain.SkillSingle, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			h := legendaryHandlers[tc.id]
			d := c.Def
			h.TargetDefinition(c, &d)
			if d.LegendaryModifiers.AppliedDamageBP != 0 {
				t.Fatal("conditional bonus without native debuff")
			}
			c.Target.Status.Apply(domain.StatusDef{ID: 190100, SkillSnapshotID: tc.status, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}, s.tick, c.Owner.ID)
			d = c.Def
			h.TargetDefinition(c, &d)
			if d.LegendaryModifiers.AppliedDamageBP != tc.bonus {
				t.Fatal("owner native debuff bonus missing", d.LegendaryModifiers)
			}
			c.Target.Status.Remove(190100)
			c.Target.Status.Apply(domain.StatusDef{ID: 190100, SkillSnapshotID: tc.status, DurationSec: 30}, s.tick, 999)
			d = c.Def
			h.TargetDefinition(c, &d)
			if d.LegendaryModifiers.AppliedDamageBP != 0 {
				t.Fatal("another owner debuff qualified")
			}
		})
	}
}
func TestLegendaryCasterRemainingHealerNativeStatusBranches(t *testing.T) {
	for _, tc := range []struct {
		id     string
		status domain.SkillID
		skill  domain.SkillID
		bonus  int32
	}{{"YS-A20", 10330, 10319, 4000}, {"YS-A27", 10311, 10312, 5000}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			d, _ := s.skills.Get(tc.skill, 1)
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if d.LegendaryModifiers.DamageBP != 0 {
				t.Fatal("bonus without native status")
			}
			c.Owner.Status.Apply(domain.StatusDef{ID: 190100, SkillSnapshotID: tc.status, DurationSec: 30, TypeKnown: true, Type: domain.StatusGood}, s.tick, c.Owner.ID)
			d, _ = s.skills.Get(tc.skill, 1)
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if d.LegendaryModifiers.DamageBP != tc.bonus {
				t.Fatal("native state not used", d.LegendaryModifiers)
			}
			d, _ = s.skills.Get(10302, 1)
			s.legendaryPrepareSkill(c.Owner, &d, c.Owner.ID)
			if d.LegendaryModifiers.DamageBP != 0 {
				t.Fatal("unrelated skill boosted")
			}
		})
	}
}
func TestLegendaryCasterRemainingHealerDefenses(t *testing.T) {
	for _, id := range []string{"YS-D05", "YS-D09", "YS-D10"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			h := legendaryHandlers[id]
			if h.BeforeIncoming(c, 100) != 100 {
				t.Fatal("defense without native condition")
			}
			switch id {
			case "YS-D05":
				c.Owner.Status.Apply(domain.StatusDef{ID: 190100, SkillSnapshotID: 10303, DurationSec: 30}, s.tick, c.Owner.ID)
				s.casts = append(s.casts, pendingCast{src: c.Owner.ID, def: domain.SkillDef{ID: 10301}, releaseAtWall: s.now().Add(time.Second), windup: &castWindup{}})
				if h.BeforeIncoming(c, 100) != 75 || !s.legendaryCasterSteadyCast(c.Owner, 10301) {
					t.Fatal("HoT native windup defense missing")
				}
				s.casts = nil
			case "YS-D09":
				c.Owner.Status.Apply(domain.StatusDef{ID: 190100, SkillSnapshotID: 10330, DurationSec: 30}, s.tick, c.Owner.ID)
				c.Skill = 0
				if h.BeforeIncoming(c, 100) != 65 {
					t.Fatal("needle basic defense missing")
				}
				c.Skill = 10401
			case "YS-D10":
				c.Target.Status.Apply(domain.StatusDef{ID: 190100, SkillSnapshotID: 10325, DurationSec: 30}, s.tick, c.Owner.ID)
				if h.BeforeIncoming(c, 100) != 60 {
					t.Fatal("owned shackle defense missing")
				}
				c.Target.Status.Remove(190100)
			}
			if h.BeforeIncoming(c, 100) != 100 {
				t.Fatal("defense remains beyond native condition")
			}
		})
	}
}
func TestLegendaryCasterRemainingHealerFriendlyCenters(t *testing.T) {
	for _, id := range []string{"YS-A05", "YS-U05"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			join(t, s, 2, 102, "ally", 450, 100)
			ally := s.players[2]
			ally.Player.Party = 1
			c.Owner.Player.Party = 1
			skill := legendaryHolyLight
			if id == "YS-U05" {
				skill = 10311
			}
			d, _ := s.skills.Get(skill, 1)
			s.legendaryPrepareSkill(c.Owner, &d, ally.ID)
			if d.Dist != 600 || d.Area.Center != domain.SkillAreaCenterTarget {
				t.Fatal("friendly remote center missing", d.Dist, d.Area)
			}
			if id == "YS-U05" {
				ids := s.legendarySelectTargets(c.Owner, d, ally.ID, []domain.EntityID{ally.ID})
				found := false
				for _, x := range ids {
					if x == c.Owner.ID {
						found = true
					}
				}
				if !found {
					t.Fatal("remote haste omitted caster")
				}
			}
			d, _ = s.skills.Get(skill, 1)
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if id == "YS-A05" && d.LegendaryModifiers.DamageBP != 0 {
				t.Fatal("enemy center got friendly bonus")
			}
		})
	}
}
func TestLegendaryCasterRemainingHealerReleaseAndInstant(t *testing.T) {
	for _, id := range []string{"YS-A29", "YS-A30"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			d, _ := s.skills.Get(10319, 1)
			d.PrepareMS = 1000
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if d.LegendaryInstantCast || d.LegendaryNextDamageBonusPct != 0 {
				t.Fatal("secret bonus without native state")
			}
			st := domain.StatusDef{ID: 190100, DurationSec: 30}
			if id == "YS-A29" {
				st.NextSkillDamagePct = 50
			} else {
				st.NextCastTimeReductionPct = 50
			}
			c.Owner.Status.Apply(st, s.tick, c.Owner.ID)
			d, _ = s.skills.Get(10319, 1)
			d.PrepareMS = 1000
			s.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			if id == "YS-A29" {
				if d.Area.Radius != 220 || d.LegendaryNextDamageBonusPct != 50 {
					t.Fatal("release geometry/coefficient missing", d)
				}
			} else if !d.LegendaryInstantCast || d.LegendaryModifiers.DamageBP != 4000 || d.LegendaryModifiers.HealingBP != 4000 {
				t.Fatal("instant full cast/strength missing")
			}
		})
	}
}
func TestLegendaryCasterRemainingHealerManaTransfers(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-U03")
	c.Skill = 10302
	c.Target = c.Owner
	c.Owner.Player.Party = 1
	for i := 2; i <= 4; i++ {
		join(t, s, domain.EntityID(i), int64(100+i), "ally", float64(100+i*20), 100)
		p := s.players[domain.EntityID(i)]
		p.Player.Party = 1
		p.MaxMP = 1000
		p.MP = 0
	}
	legendaryHandlers["YS-U03"].ManaRestored(c, 0, 500)
	if s.players[2].MP != 200 || s.players[3].MP != 200 || s.players[4].MP != 0 {
		t.Fatal("overflow transfer bounds")
	}
	c.Skill = 10301
	legendaryHandlers["YS-U03"].ManaRestored(c, 0, 500)
	if s.players[2].MP != 200 {
		t.Fatal("unrelated heal generated mana")
	}
}
func TestLegendaryCasterBurnSettlementKillReward(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U05")
	legendaryCasterEquipBuild(t, s, c.Owner, "SS-U05", "SS-A02")
	c.Owner.MP = 0
	c.Target.HP = 500
	c.Target.Status.Apply(domain.StatusDef{ID: 1036, DurationSec: 20, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -100, Mode: domain.ModeAbsolute}}}, s.tick, c.Owner.ID)
	legendaryCasterCastNow(t, s, c.Owner, 10401, c.Target.ID)
	if c.Target.Alive() || c.Owner.MP != c.Owner.MaxMP*6/100 {
		t.Fatal("burn settlement fatal did not restore exactly6 percent", c.Target.HP, c.Owner.MP, c.Owner.MaxMP)
	}
	c.Hit = event.DamageDealt{Flag: event.DamageFatal}
	c.Target.HP = 0
	c.Power, _ = domain.LegendaryPower("SS-A02")
	before := c.Owner.MP
	s.legendaryCasterDamageResolved(c, c.Target, c.Hit)
	if c.Owner.MP != before {
		t.Fatal("duplicate death rewarded")
	}
	c.Target.LegendaryLifeSequence++
	s.legendaryCasterDamageResolved(c, c.Target, c.Hit)
	if c.Owner.MP != before+c.Owner.MaxMP*6/100 {
		t.Fatal("new life death incorrectly deduplicated")
	}
	c.Power, _ = domain.LegendaryPower("SS-A04")
	c.Target.LegendaryLifeSequence++
	before = c.Owner.MP
	s.legendaryCasterDamageResolved(c, c.Target, c.Hit)
	if c.Owner.MP != before {
		t.Fatal("ordinary fire echo counted as burn settlement")
	}
}

func TestLegendaryCasterRemainingHealerCenterAndDamageLedger(t *testing.T) {
	t.Run("YS-A06", func(t *testing.T) {
		s, c := legendaryCasterFixture(t, "YS-A06")
		c.Skill = 10317
		c.Center = c.Target.Pos
		c.Def = domain.SkillDef{ID: 10317, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		d := c.Def
		legendaryHandlers["YS-A06"].TargetDefinition(c, &d)
		if d.LegendaryModifiers.AppliedDamageBP != 6000 {
			t.Fatal("center bonus")
		}
		c.Center.X -= 150
		d = c.Def
		legendaryHandlers["YS-A06"].TargetDefinition(c, &d)
		if d.LegendaryModifiers.AppliedDamageBP != 2000 {
			t.Fatal("outer bonus")
		}
		_ = s
	})
	t.Run("YS-A15", func(t *testing.T) {
		_, c := legendaryCasterFixture(t, "YS-A15")
		c.Skill = 10326
		c.Primary = c.Target.ID
		c.Hit.Amount = 100
		h := legendaryHandlers["YS-A15"]
		h.AfterHit(c)
		c.Primary = 999
		c.LostHP = 1000
		h.AfterHit(c)
		c.Primary = c.Target.ID
		before := c.Target.HP
		h.RootFinished(c)
		if before-c.Target.HP != 100 {
			t.Fatal("actual secondary loss not capped to primary", before-c.Target.HP)
		}
	})
}
func TestLegendaryCasterRemainingHealerLightFieldBoundaries(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A07")
	c.Skill = 10317
	c.Center = c.Target.Pos
	c.Def, _ = s.skills.Get(10317, 1)
	h := legendaryHandlers["YS-A07"]
	h.RootFinished(c)
	c.RootID++
	h.RootFinished(c)
	before := c.Target.HP
	h.Tick(c)
	if c.Target.HP != before {
		t.Fatal("ground field ticked before authored interval")
	}
	c.NowMS += 1000
	h.Tick(c)
	damage := before - c.Target.HP
	if damage <= 0 {
		t.Fatal("field has no tick")
	}
	h.Tick(c)
	if before-c.Target.HP != damage {
		t.Fatal("overlapping/repeated field ticks stacked")
	}
}
func TestLegendaryCasterRemainingHealerCleanseRefund(t *testing.T) {
	for _, success := range []bool{false, true} {
		s, c := legendaryCasterFixture(t, "YS-U07")
		c.Owner.MP = 0
		c.Skill = 10306
		c.PaidMP = 100
		c.RootID = 123
		root := s.equipmentState()
		root.root = &equipmentRoot{id: 123, paidMP: 100, src: c.Owner.ID, def: domain.SkillDef{ID: 10306}}
		if success {
			legendaryHandlers["YS-U07"].Dispelled(c, []domain.Active{{}})
		}
		legendaryHandlers["YS-U07"].RootFinished(c)
		want := int32(90)
		if success {
			want = 30
		}
		if c.Owner.MP != want {
			t.Fatal("cleanse refund", success, c.Owner.MP, want)
		}
	}
}
func TestLegendaryCasterExorcismDispelNativeBenefitsNotPermanent(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A09")
	c.Target.Status.Apply(domain.StatusDef{ID: 190001, DurationSec: 60, TypeKnown: true, Type: domain.StatusGood, Affixes: []domain.Affix{{Attr: domain.AttrMAtk, Value: 50, Mode: domain.ModePercent}}}, s.tick, c.Target.ID)
	c.Target.Status.Apply(domain.StatusDef{ID: 190002, DurationSec: 60, TypeKnown: true, Type: domain.StatusGood}, s.tick, c.Target.ID)
	c.Target.Status.Apply(domain.StatusDef{ID: 190003, Permanent: true, TypeKnown: true, Type: domain.StatusGood}, s.tick, c.Target.ID)
	c.Target.Status.Apply(domain.StatusDef{ID: 190004, DurationSec: 60, TypeKnown: true, Type: domain.StatusBad}, s.tick, c.Owner.ID)
	legendaryCasterCastNow(t, s, c.Owner, 10312, c.Target.ID)
	if c.Target.Status.HasEffective(190001) || c.Target.Status.HasEffective(190002) || !c.Target.Status.HasEffective(190003) || !c.Target.Status.HasEffective(190004) {
		t.Fatal("native benefit classification/permanent guard violated")
	}
}

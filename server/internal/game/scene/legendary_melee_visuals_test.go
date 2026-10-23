package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strings"
	"testing"
)

func meleeVisualEvents(s *Scene) (hits []event.SkillHitEffect, ground []event.GroundEffect) {
	for _, p := range s.outbox {
		switch e := p.ev.(type) {
		case event.SkillHitEffect:
			if strings.HasPrefix(e.Effect, "LP_") {
				hits = append(hits, e)
			}
		case event.GroundEffect:
			ground = append(ground, e)
		}
	}
	return
}
func TestLegendaryMeleeQuakeActualCastGroundLifetime(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A14")
	d := domain.SkillDef{ID: 10006, Level: 1, Prof: 1, Kind: domain.SkillArea, TargetEnemy: true, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
	s.skills = skillTable(d)
	s.outbox = nil
	center := p.Pos
	s.onUseSkill(UseSkill{ID: p.ID, Skill: d.ID, Target: m.ID})
	s.stepCasts()
	_, g := meleeVisualEvents(s)
	if len(g) != 1 || g[0].Effect != "LP_ZS_A14_01" || g[0].Mode != 1 || g[0].DurationMS != 3000 || g[0].Pos != center {
		t.Fatalf("release %+v", g)
	}
	key := g[0].Key
	s.aoi.Move(p, domain.Pos{MapID: p.Pos.MapID, X: 300, Y: 300})
	s.outbox = nil
	for _, ms := range []int{1000, 2000, 3000, 4000} {
		s.tick = domain.Tick(ms / domain.TickMS)
		s.stepLegendaryPowers()
	}
	_, g = meleeVisualEvents(s)
	pulses, stops := 0, 0
	for _, e := range g {
		if e.Mode == 0 {
			pulses++
			if e.Effect != "LP_ZS_A14_02" || e.Pos != center || e.DurationMS != 0 {
				t.Fatalf("pulse %+v", e)
			}
		}
		if e.Mode == 2 && e.Key == key {
			stops++
		}
	}
	if pulses != 3 || stops != 1 || len(s.groundEffects) != 0 {
		t.Fatalf("pulses %d stops %d tracked %d events %+v", pulses, stops, len(s.groundEffects), g)
	}
}
func TestLegendaryMeleeQuakeCleanupAndReplacement(t *testing.T) {
	for _, action := range []string{"unequip", "prune", "map", "owner", "replace"} {
		t.Run(action, func(t *testing.T) {
			s, p, _ := legendaryMeleeFixture(t, "ZS-A14")
			d := domain.SkillDef{ID: 10006, Area: domain.SkillAreaTargeting{Radius: 100}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			s.legendaryCastReleasedAt(p, d, 0, nil, 0, p.Pos)
			_, start := meleeVisualEvents(s)
			key := start[0].Key
			s.outbox = nil
			switch action {
			case "unequip":
				p.Player.Worn = &domain.EquipSet{}
				s.stepLegendaryPowers()
			case "prune":
				p.Player.Worn = &domain.EquipSet{}
				s.pruneLegendaryOwner(p)
			case "map":
				p.Pos.MapID++
				s.stepLegendaryPowers()
			case "owner":
				s.clearLegendaryOwner(p.ID)
			case "replace":
				s.legendaryCastReleasedAt(p, d, 0, nil, 0, p.Pos)
				s.legendaryCastReleasedAt(p, d, 0, nil, 0, p.Pos)
			}
			_, g := meleeVisualEvents(s)
			stopped := 0
			for _, e := range g {
				if e.Mode == 2 && e.Key == key {
					stopped++
				}
			}
			if stopped != 1 {
				t.Fatalf("old field not stopped once: %+v", g)
			}
			if action != "replace" && len(s.groundEffects) != 0 {
				t.Fatal("ground lingered")
			}
			if action == "replace" && len(s.groundEffects) != 2 {
				t.Fatal("overlap cap differs from gameplay")
			}
		})
	}
}
func TestLegendaryMeleeActualImpactVisuals(t *testing.T) {
	for _, tc := range []struct {
		id     string
		skill  domain.SkillID
		effect string
		count  int
	}{
		{"ZS-A02", 10002, "LP_ZS_A02_01", 1}, {"ZS-A05", 10005, "LP_ZS_A05_01", 1}, {"ZS-A06", 10005, "LP_ZS_A06_01", 1}, {"ZS-A13", 10006, "LP_ZS_A13_01", 1}, {"ZS-A16", 10003, "LP_ZS_A16_02", 1}, {"ZS-A18", 10021, "LP_ZS_A18_01", 1}, {"ZS-A22", 10022, "LP_ZS_A22_01", 1}, {"ZS-A23", 10022, "LP_ZS_A23_01", 1},
		{"JK-A07", 10103, "LP_JK_A07_01", 2}, {"JK-A28", 10104, "LP_JK_A28_01", 1},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			s.outbox = nil
			warriorTestCast(t, s, p, m, domain.SkillDef{ID: tc.skill, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}})
			hits, _ := meleeVisualEvents(s)
			n := 0
			for _, e := range hits {
				if e.Effect == tc.effect && e.Target == m.ID {
					n++
				}
			}
			if n != tc.count {
				t.Fatalf("actual impacts %+v want %s x%d", hits, tc.effect, tc.count)
			}
		})
	}
}
func TestLegendaryMeleeProcVisualOnlyAfterResolution(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A06")
	end := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	c.RootID = s.equipmentState().root.id
	s.outbox = nil
	c.damageVisual("stage-01", m, 100, false, false)
	hits, _ := meleeVisualEvents(s)
	if len(hits) != 0 {
		t.Fatal("queued damage played early")
	}
	m.HP = 0
	end()
	hits, _ = meleeVisualEvents(s)
	if len(hits) != 0 {
		t.Fatal("discarded payload played")
	}
	m.HP = 1000
	s.outbox = nil
	c.damageVisual("stage-01", m, 100, false, false)
	hits, _ = meleeVisualEvents(s)
	if len(hits) != 1 || hits[0].Effect != "LP_ZS_A06_01" {
		t.Fatalf("resolved %+v", hits)
	}
}
func TestLegendaryMeleeNoObsoleteMarksOrGround(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{
		{"ZS-A10", 10013}, {"ZS-A25", 10023}, {"ZS-A27", 10027}, {"ZS-A28", 10028}, {"JK-A13", 10113}, {"JK-A16", 10121}, {"JK-A17", 10121}, {"JK-A21", 10116}, {"JK-U10", 10113},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			s.outbox = nil
			warriorTestCast(t, s, p, m, domain.SkillDef{ID: tc.skill, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}})
			hits, ground := meleeVisualEvents(s)
			if len(hits)+len(ground) != 0 {
				t.Fatalf("obsolete effect %+v %+v", hits, ground)
			}
		})
	}
}
func TestLegendaryMeleeSpearBurstUsesImpactNotOldMark(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A12")
	s.outbox = nil
	warriorTestCast(t, s, p, m, domain.SkillDef{ID: 10017, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}})
	hits, g := meleeVisualEvents(s)
	if len(hits) != 0 || len(g) != 1 || g[0].Effect != "LP_ZS_A12_02" || g[0].Mode != 0 || g[0].DurationMS != 0 || g[0].Pos != m.Pos {
		t.Fatalf("burst %+v %+v", hits, g)
	}
}

func TestLegendaryMeleeSwordAreaActualVisuals(t *testing.T) {
	for _, tc := range []struct {
		id             string
		skill          domain.SkillID
		stage1, stage2 int
	}{
		{"JK-A04", 10117, 2, 0}, {"JK-A08", 10103, 1, 0}, {"JK-A19", 10102, 1, 0}, {"JK-A20", 10116, 1, 0}, {"JK-A22", 10123, 1, 2}, {"JK-A23", 10123, 1, 0}, {"JK-A27", 10102, 2, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			if tc.id == "JK-A04" {
				s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 190, Y: 100})
			}
			if tc.id == "JK-A23" {
				m.HP = m.MaxHP / 2
			}
			s.outbox = nil
			swordCast(t, s, p, m, domain.SkillDef{ID: tc.skill, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}})
			hits, _ := meleeVisualEvents(s)
			a, b := 0, 0
			for _, e := range hits {
				if strings.HasSuffix(e.Effect, "_01") {
					a++
				}
				if strings.HasSuffix(e.Effect, "_02") {
					b++
				}
			}
			if a != tc.stage1 || b != tc.stage2 {
				t.Fatalf("stages %d/%d want %d/%d: %+v", a, b, tc.stage1, tc.stage2, hits)
			}
		})
	}
}
func TestLegendaryMeleeCastCuesAndShields(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{
		{"ZS-A04", 10005}, {"ZS-A16", 10003}, {"ZS-D01", 10028}, {"ZS-D09", 10016}, {"ZS-U04", 10028}, {"ZS-U08", 10028}, {"ZS-U10", 10028},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, _ := legendaryMeleeFixture(t, tc.id)
			s.outbox = nil
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, 0, nil)
			hits, _ := meleeVisualEvents(s)
			if len(hits) != 1 || hits[0].Target != p.ID {
				t.Fatalf("cast cue %+v", hits)
			}
		})
	}
	s, p, _ := legendaryMeleeFixture(t, "ZS-U04")
	warriorEquip(t, s, p, "ZS-U08", domain.SlotBody)
	warriorEquip(t, s, p, "ZS-U10", domain.SlotHead)
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10028}, 0, nil)
	hits, _ := meleeVisualEvents(s)
	if len(hits) != 1 {
		t.Fatalf("stacked cry duplicate %+v", hits)
	}
	for _, id := range []string{"JK-D01", "JK-D08"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, id)
			skill := domain.SkillID(10102)
			if id == "JK-D08" {
				skill = 10116
			}
			s.outbox = nil
			swordCast(t, s, p, m, domain.SkillDef{ID: skill, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}})
			hits, _ := meleeVisualEvents(s)
			if len(hits) != 1 || hits[0].Target != p.ID {
				t.Fatalf("shield %+v", hits)
			}
		})
	}
}
func TestLegendaryMeleeCleanseVisualOnlyWhenRemoved(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "JK-U02")
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10104}, 0, nil)
	hits, _ := meleeVisualEvents(s)
	if len(hits) != 0 {
		t.Fatal("empty cleanse flashed")
	}
	s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusPoison, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad}, p.ID)
	s.outbox = nil
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10104}, 0, nil)
	hits, _ = meleeVisualEvents(s)
	if len(hits) != 1 || hits[0].Effect != "LP_JK_U02_01" {
		t.Fatalf("cleanse %+v", hits)
	}
}
func TestLegendaryMeleeImpactNeverPlaysForMissOrPreview(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A05")
	c := s.legendaryContexts(p, m, combat.Blow{SkillID: 10005}, event.DamageDealt{Flag: event.DamageMiss})[0]
	s.outbox = nil
	legendaryHandlers[c.Power.ID].AfterHit(&c)
	c.Hit.Flag = 0
	c.Preview = true
	legendaryHandlers[c.Power.ID].AfterHit(&c)
	c.damageVisual("stage-01", m, 100, false, false)
	hits, g := meleeVisualEvents(s)
	if len(hits)+len(g) != 0 {
		t.Fatalf("miss or preview %+v %+v", hits, g)
	}
}

func TestLegendaryMeleeRevisedImmediateStages(t *testing.T) {
	for _, id := range []string{"JK-A06", "JK-A12", "JK-A25"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, id)
			s.outbox = nil
			switch id {
			case "JK-A06":
				st := domain.StatusDef{ID: 800111, Level: 1, DurationSec: 6, BlockMove: true, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
				s.statuses = statusTable(st)
				swordCast(t, s, p, m, domain.SkillDef{ID: 10111, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}, Statuses: []domain.StatusApplication{{ID: st.ID, Level: 1}}})
			case "JK-A12":
				p.MaxMP = 1000
				p.MP = 990
				swordCast(t, s, p, m, swordDualDef(10119))
			case "JK-A25":
				swordCast(t, s, p, m, domain.SkillDef{ID: 10127, Effects: []domain.SkillEffect{{Kind: domain.EffectTaunt, TauntPower: 100}}})
			}
			hits, g := meleeVisualEvents(s)
			stage := "_02"
			if id == "JK-A25" {
				stage = "_01"
			}
			if len(g) != 0 || len(hits) != 1 || !strings.HasSuffix(hits[0].Effect, stage) || hits[0].Target != m.ID {
				t.Fatalf("immediate %+v %+v", hits, g)
			}
		})
	}
}
func TestLegendaryMeleeShieldBreakStages(t *testing.T) {
	for _, shield := range []int32{0, 200, 1000} {
		s, p, m := legendaryMeleeFixture(t, "JK-A14")
		if shield > 0 {
			s.applyStatusDef(m, domain.StatusDef{ID: 800333, Level: 1, DurationSec: 10, DamageShield: shield}, m.ID)
		}
		s.outbox = nil
		swordCast(t, s, p, m, domain.SkillDef{ID: 10113, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
		hits, _ := meleeVisualEvents(s)
		expected := 0
		if shield == 200 {
			expected = 2
		}
		if shield == 1000 {
			expected = 1
		}
		if len(hits) != expected {
			t.Fatalf("shield%d %+v", shield, hits)
		}
	}
}

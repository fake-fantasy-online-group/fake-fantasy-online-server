package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestLegendaryMeleeFunctionalMovementWindows(t *testing.T) {
	for _, tc := range []struct {
		id       string
		skill    domain.SkillID
		duration int
		speed    int32
	}{
		{"ZS-U03", 10017, 8000, 180}, {"ZS-U04", 10028, 12000, 160}, {"JK-U01", 10117, 8000, 180},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, _ := legendaryMeleeFixture(t, tc.id)
			speed := func() int32 { st := domain.Stats{MoveSpeed: 100}; s.legendaryPassiveStats(p, &st); return st.MoveSpeed }
			if speed() != 100 {
				t.Fatal("unearned window")
			}
			delete(p.Player.Char.Skills, tc.skill)
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, 0, nil)
			if speed() != 100 {
				t.Fatal("unlearned window")
			}
			p.Player.Char.Skills[tc.skill] = 1
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, 0, nil)
			if speed() != tc.speed {
				t.Fatal("cast did not activate", speed())
			}
			s.tick += domain.Ticks(2000)
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, 0, nil)
			s.tick += domain.Ticks(tc.duration - 100)
			if speed() != tc.speed {
				t.Fatal("refresh did not retain window")
			}
			s.tick += domain.Ticks(100)
			if speed() != 100 {
				t.Fatal("refresh stacked duration")
			}
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, 0, nil)
			delete(p.Player.Char.Skills, tc.skill)
			if speed() != 100 {
				t.Fatal("respec retained window")
			}
			s.stepLegendaryPowers()
			p.Player.Char.Skills[tc.skill] = 1
			if speed() != 100 {
				t.Fatal("relearning resurrected window")
			}
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, 0, nil)
			p.Player.Worn = &domain.EquipSet{}
			s.pruneLegendaryOwner(p)
			if speed() != 100 {
				t.Fatal("unequip retained speed")
			}
		})
	}
}

func TestLegendaryMeleeFunctionalDefenseWindows(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		magic bool
		want  int32
	}{{"JK-D06", 10111, false, 65}, {"JK-D07", 10113, true, 50}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, tc.id)
			get := func(origin domain.DamageOrigin, magic bool) int32 {
				c := s.legendaryContexts(p, m, combat.Blow{Magic: magic, Origin: origin}, event.DamageDealt{})[0]
				return legendaryHandlers[tc.id].BeforeIncoming(&c, 100)
			}
			if get(domain.DamageNormalOrigin, tc.magic) != 100 {
				t.Fatal("unearned defense")
			}
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, m.ID, nil)
			if get(domain.DamageNormalOrigin, tc.magic) != tc.want {
				t.Fatal("window missing")
			}
			if get(domain.DamageDOTOrigin, tc.magic) != tc.want {
				t.Fatal("matching periodic damage bypassed reduction")
			}
			if tc.magic && get(domain.DamageNormalOrigin, false) != 100 {
				t.Fatal("magic defense reduced physical")
			}
			delete(p.Player.Char.Skills, tc.skill)
			if get(domain.DamageNormalOrigin, tc.magic) != 100 {
				t.Fatal("respec defense")
			}
		})
	}
}
func TestLegendaryMeleeFunctionalManaAndSlow(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"ZS-U01", 10002}, {"JK-U06", 10111}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, mode := range []string{"equipped", "unequipped", "wrong-skill"} {
				t.Run(mode, func(t *testing.T) {
					s, p, m := legendaryMeleeFixture(t, tc.id)
					if mode == "unequipped" {
						p.Player.Worn = &domain.EquipSet{}
					}
					skill := tc.skill
					if mode == "wrong-skill" {
						skill = 10999
					}
					d := domain.SkillDef{ID: skill, MPCost: 100, ManaCostFractionNumerator: 100, ManaCostFractionDenominator: 1, Dist: 100}
					s.legendaryPrepareSkill(p, &d, m.ID)
					wantCost, wantDist := int32(100), int32(100)
					if mode == "equipped" {
						wantCost = 0
						if tc.id == "JK-U06" {
							wantDist = 180
						}
					}
					if d.MPCost != wantCost || d.ManaCostFractionNumerator != int64(wantCost) || d.Dist != wantDist {
						t.Fatalf("cost/fraction/range=%d/%d/%d want %d/%d/%d", d.MPCost, d.ManaCostFractionNumerator, d.Dist, wantCost, wantCost, wantDist)
					}
				})
			}
		})
	}
	s, p, m := legendaryMeleeFixture(t, "JK-U05")
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10103}, m.ID, nil)
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	slow := domain.StatusDef{ID: domain.StatusSlow, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50}}}
	if legendaryHandlers["JK-U05"].StatusReceiving(&c, &slow) {
		t.Fatal("pure slow survived")
	}
	mixed := domain.StatusDef{ID: domain.StatusStun, Control: domain.ControlStun, BlockMove: true, HPChanges: []domain.ResourceChange{{Value: -10}}, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50}}}
	if !legendaryHandlers["JK-U05"].StatusReceiving(&c, &mixed) || !mixed.BlockMove || mixed.Control != domain.ControlStun || len(mixed.HPChanges) != 1 || len(mixed.Affixes) != 0 {
		t.Fatal("mixed control payload altered", mixed)
	}
}

func TestLegendaryMeleeFunctionalUnlimitedAreaCandidates(t *testing.T) {
	for _, id := range []string{"ZS-A02", "JK-A09"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, id)
			for i := 0; i < 300; i++ {
				putMonster(s, domain.EntityID(2000+i), 1000, domain.Stats{}, 140, 100)
			}
			outside := putMonster(s, 4000, 1000, domain.Stats{}, 1000, 1000)
			dead := putMonster(s, 4001, 0, domain.Stats{}, 140, 100)
			c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
			c.Primary = m.ID
			var ids []domain.EntityID
			if id == "ZS-A02" {
				ids = warriorCircle(&c, nil, p.Pos, 100)
			} else {
				ids = swordExpand(&c, nil, 100, false)
			}
			if len(ids) != 301 {
				t.Fatal("in-range targets truncated", len(ids))
			}
			seen := map[domain.EntityID]bool{}
			for _, target := range ids {
				if target == outside.ID || target == dead.ID || seen[target] {
					t.Fatal("eligibility/dedup changed", target)
				}
				seen[target] = true
			}
		})
	}
}

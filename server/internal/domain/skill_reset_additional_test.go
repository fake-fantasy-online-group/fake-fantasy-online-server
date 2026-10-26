package domain_test

import (
	d "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	catalog "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"testing"
)

func TestResetAllocatedSkillsStarterRanksArePaidPoints(t *testing.T) {
	ch := &d.Character{Level: 10, SkillPoints: 4, SkillPointsKnown: true, SkillAllocationVersion: 5, Skills: d.Learned{d.SkillBeginnerBolt: 3, d.SkillBeginnerGuard: 2, d.SkillAlchemy: 1}}
	_, refund, err := d.ResetAllocatedSkills(ch, catalog.SkillProgressionCatalog())
	if err != nil || refund != 5 || ch.SkillPoints != 9 || ch.Skills[d.SkillAlchemy] != 1 {
		t.Fatal(ch, refund, err)
	}
}

func TestResetAllocatedSkillsAllProfessionsAndOverCapKeepGrants(t *testing.T) {
	c := catalog.SkillProgressionCatalog()
	for profession := int32(1); profession <= 5; profession++ {
		for _, level := range []int32{100, 120} {
			hard := d.Learned{}
			var first d.SkillID
			for _, id := range c.IDs() {
				identity, _ := c.Identity(id)
				if identity.Profession == profession {
					hard[id] = 1
					if first == 0 {
						first = id
					}
				}
			}
			if first == 0 {
				t.Fatal("missing class", profession)
			}
			ch := &d.Character{Level: level, Race: d.Race(profession - 1), SkillPoints: d.SkillPointBudget(level) - int32(len(hard)), SkillPointsKnown: true, SkillAllocationVersion: 5, Skills: hard}
			grants := []d.SkillGrant{{SourceID: "equipment:test", SkillID: first, Rank: 5, Origin: "equipment"}}
			oldPoints := ch.SkillPoints
			oldSpent := len(hard)
			_, refund, err := d.ResetAllocatedSkills(ch, c)
			if err != nil || int(refund) != oldSpent || ch.SkillPoints != oldPoints+refund || ch.SkillPoints != 99 {
				t.Fatal(profession, level, refund, err)
			}
			resolved, err := d.ResolveSkillLoadout(profession, ch.Skills, nil, grants, c)
			if err != nil || resolved[first].HardRank != 0 || resolved[first].GrantRank != "5" || !resolved[first].CanCast {
				t.Fatal("grant lost or refunded", profession, resolved[first], err)
			}
		}
	}
}

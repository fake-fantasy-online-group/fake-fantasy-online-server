package domain_test

import (
	d "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	catalog "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"reflect"
	"testing"
)

func TestResetAllocatedSkillsConservesBudgetAndPreservesNonBudget(t *testing.T) {
	ch := &d.Character{Race: d.Warlock, Level: 35, SkillPoints: 27, SkillPointsKnown: true, SkillAllocationVersion: 5, Skills: d.Learned{10401: 7, d.SkillAlchemy: 900}}
	ch.Hotbar[0] = d.HotbarSlot{Kind: d.HotbarKindSkill, ID: 10401}
	ch.Hotbar[1] = d.HotbarSlot{Kind: d.HotbarKindPetSkill, ID: 10401}
	removed, n, err := d.ResetAllocatedSkills(ch, catalog.SkillProgressionCatalog())
	if err != nil || n != 7 || !removed[10401] || ch.SkillPoints != 34 || ch.Skills[d.SkillAlchemy] != 900 || ch.Skills[10401] != 0 {
		t.Fatal(ch, n, err)
	}
	if ch.Hotbar[0].ID != 0 || ch.Hotbar[1].ID != 10401 {
		t.Fatal(ch.Hotbar)
	}
	before := ch.Clone()
	if _, _, err = d.ResetAllocatedSkills(ch, catalog.SkillProgressionCatalog()); err == nil || !reflect.DeepEqual(before, ch) {
		t.Fatal("empty reset mutated", err)
	}
}
func TestResetAllocatedSkillsRejectsInvalidHistoryWithoutMutation(t *testing.T) {
	for _, version := range []uint32{0, 5} {
		ch := &d.Character{Race: d.Warlock, Level: 35, SkillPoints: 34, SkillPointsKnown: true, SkillAllocationVersion: version, Skills: d.Learned{10401: 7}}
		before := ch.Clone()
		if _, _, err := d.ResetAllocatedSkills(ch, catalog.SkillProgressionCatalog()); err == nil || !reflect.DeepEqual(before, ch) {
			t.Fatal("invalid history changed", version, err)
		}
	}
}

package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestPetCleanseRunsOnlyAfterMatchingHarmfulStatusLands(t *testing.T) {
	for _, tc := range []struct {
		name               string
		status             domain.StatusDef
		chance, mp, wantMP int32
		wantStatus         bool
	}{
		{"success", 中毒, 10000, 100, 76, false},
		{"miss still costs MP", 中毒, 0, 100, 76, true},
		{"insufficient MP", 中毒, 10000, 23, 23, true},
		{"unmatched harmful status", 昏迷, 10000, 100, 100, true},
		{"beneficial status", 狂暴, 10000, 100, 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := petScene(t)
			s.petSkills = domain.PetSkillTable{13018: {
				ID: 13018, Name: "净化", MaxLevel: 50,
				Effect: &domain.PetSkillEffect{Kind: "cleanse_status", ChanceBP: tc.chance,
					Value: 24, SourceStep: 1, CleanseStatusIDs: []int32{int32(domain.StatusPoison)}},
			}}
			inst := 战力宠()
			inst.Skills = []domain.PetSkill{{ID: 13018, Level: 1}}
			owner := s.players[1]
			owner.Player.Char.Pets = append(owner.Player.Char.Pets, inst)
			do(s, SummonPet{ID: owner.ID, Inst: inst.ID})
			pet := s.entities[owner.Player.Pet]
			if pet == nil {
				t.Fatal("test pet was not summoned")
			}
			pet.MP = tc.mp
			owner.Stats.StatusResist = 0
			if !s.applyStatusDef(owner, tc.status, owner.ID) {
				t.Fatal("status should land before the cleanse attempt")
			}
			if owner.Status.Has(tc.status.ID) != tc.wantStatus || pet.MP != tc.wantMP {
				t.Fatalf("status=%v MP=%d, want status=%v MP=%d",
					owner.Status.Has(tc.status.ID), pet.MP, tc.wantStatus, tc.wantMP)
			}
		})
	}
}

func TestPetCleanseSupportsLegacyAndMergedTargets(t *testing.T) {
	legacy := &domain.PetSkillEffect{SourceAttr: int32(domain.StatusPoison)}
	if !petSkillCleanses(legacy, domain.StatusPoison) || petSkillCleanses(legacy, domain.StatusStun) {
		t.Fatal("legacy effect must preserve its single source-attribute target")
	}
	legacy.CleanseStatusIDs = []int32{int32(domain.StatusStun), int32(domain.StatusSilence)}
	if petSkillCleanses(legacy, domain.StatusPoison) ||
		!petSkillCleanses(legacy, domain.StatusStun) || !petSkillCleanses(legacy, domain.StatusSilence) {
		t.Fatal("configured status list must override the legacy target")
	}
}

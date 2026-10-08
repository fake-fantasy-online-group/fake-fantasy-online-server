package data

import (
	"context"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestLoadPetCleanseTargetListAndLegacyFallback(t *testing.T) {
	ctx := context.Background()
	tx, err := testPool(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE game_pet_skill_effects (
		skill_id INT, kind TEXT, chance_bp INT, chance_per_level_bp INT,
		value INT, value_per_level INT, source_attr INT, source_step INT,
		require_selected BOOL, cleanse_status_ids INT[]
	) ON COMMIT DROP;
	INSERT INTO game_pet_skill_effects VALUES
	(13018,'cleanse_status',50,50,24,4,1001,1,FALSE,ARRAY[1001,1002,1003]),
	(13019,'cleanse_status',500,100,24,4,1002,1,FALSE,NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	defs := domain.PetSkillTable{13018: {ID: 13018}, 13019: {ID: 13019}}
	if err := loadPetSkillEffects(ctx, tx, defs); err != nil {
		t.Fatal(err)
	}
	if got := defs[13018].Effect; got == nil || len(got.CleanseStatusIDs) != 3 || got.ChanceBP != 50 {
		t.Fatalf("merged cleanse definition lost its target list: %+v", got)
	}
	if got := defs[13019].Effect; got == nil || len(got.CleanseStatusIDs) != 0 || got.SourceAttr != 1002 {
		t.Fatalf("legacy cleanse definition changed: %+v", got)
	}
}

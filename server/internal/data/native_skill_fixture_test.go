package data

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

const (
	nativeArmorBreak domain.StatusID = 10009
	nativeMoveLock   domain.StatusID = 10111
)

type nativeRestorationSeed struct {
	Applications []struct {
		SkillID domain.SkillID
		Rank    int32
		App     domain.StatusApplication
	}
	States []domain.StatusDef
}

func nativeRestorationSQLFixture() nativeRestorationSeed {
	raw, err := testfixture.ReadMigrationJSON("000000_init-62.sql", "$skill_config$")
	if err != nil {
		panic(err)
	}
	var out nativeRestorationSeed
	if err = json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}
func restoredNativeSkillApplications(id domain.SkillID, rank int32) []domain.StatusApplication {
	for _, r := range nativeRestorationSQLFixture().Applications {
		if r.SkillID == id && r.Rank == rank {
			r.App.SourceSkill = id
			r.App.ChanceExplicit = true
			return []domain.StatusApplication{r.App}
		}
	}
	return nil
}
func completeRestoredNativeStatus(d *domain.StatusDef) {
	for _, r := range nativeRestorationSQLFixture().States {
		if d.ID == r.ID && d.Level == r.Level {
			*d = r
			return
		}
	}
}
func installRestoredNativeStatuses(out domain.StatusTable) error {
	for k := range out {
		if k.ID == nativeArmorBreak || k.ID == nativeMoveLock {
			return fmt.Errorf("fixture collision %d", k.ID)
		}
	}
	for _, r := range nativeRestorationSQLFixture().States {
		if r.ID == nativeArmorBreak || r.ID == nativeMoveLock {
			out[domain.StatusKey{ID: r.ID, Level: r.Level}] = r
		}
	}
	return nil
}

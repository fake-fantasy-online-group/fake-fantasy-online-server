package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadProgressionRules(ctx context.Context, q Querier, player *domain.LevelTable, pet *domain.PetLevelTable) (*domain.ProgressionRules, error) {
	rows, err := q.Query(ctx, `SELECT player_cap,pet_cap,player_checksum,pet_checksum,reward_version FROM game_progression_versions WHERE version=$1`, domain.ProgressionVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("missing progression-v5 migration")
	}
	var pc, qc int32
	var ph, qh, reward string
	if err = rows.Scan(&pc, &qc, &ph, &qh, &reward); err != nil {
		return nil, err
	}
	if pc != domain.LevelCap || qc != domain.PetLevelCap || reward != domain.TrialRewardVersion {
		return nil, fmt.Errorf("progression cap mismatch")
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return domain.NewProgressionRules(player, pet, ph, qh)
}

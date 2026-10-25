package data

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadWingProgression(ctx context.Context, q Querier, items map[domain.ItemID]domain.ItemDef) (*domain.WingProgressionRules, error) {
	rows, err := q.Query(ctx, `SELECT configuration::text FROM game_wing_progression_rules WHERE active ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result *domain.WingProgressionRules
	for rows.Next() {
		if result != nil {
			return nil, fmt.Errorf("multiple active wing configurations")
		}
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c domain.WingProgressionConfig
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		result, err = domain.NewWingProgressionRules(c, items)
		if err != nil {
			return nil, err
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	// No published economy means this optional feature is off, not a whole-server outage.
	return result, nil
}

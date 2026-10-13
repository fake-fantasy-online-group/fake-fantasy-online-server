package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadRiftEntrances(ctx context.Context, q Querier) ([]domain.EndgameRiftEntranceView, error) {
	// LEFT JOIN makes an invalid or hidden binding fail the scan rather than
	// silently disappearing and leaving a successfully started but unusable server.
	rows, err := q.Query(ctx, `SELECT e.map_id, e.npc_id, n.name, n.script
        FROM game_rift_entrances e
        LEFT JOIN map_defs m ON m.id=e.map_id AND m.file=e.map_file
        LEFT JOIN map_npcs n ON n.map_file=m.file AND n.npc_id=e.npc_id AND NOT n.hide
        ORDER BY e.map_id,e.npc_id`)
	if err != nil {
		return nil, fmt.Errorf("load rift entrances: %w", err)
	}
	defer rows.Close()
	var result []domain.EndgameRiftEntranceView
	for rows.Next() {
		var e domain.EndgameRiftEntranceView
		if err := rows.Scan(&e.MapID, &e.NPCID, &e.NPC, &e.Script); err != nil {
			return nil, fmt.Errorf("invalid native rift entrance: %w", err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no configured rift entrances")
	}
	return result, nil
}

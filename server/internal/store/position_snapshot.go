package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// A position snapshot must never write currency, inventories, or child tables.
// One UPDATE is atomic without an explicit multi-statement transaction.
func savePosition(ctx context.Context, q querier, c *domain.Character) error {
	if c == nil {
		return nil
	}
	tag, err := q.Exec(ctx, `UPDATE characters SET map_id=$2, pos_x=$3, pos_y=$4, scene_instance=$5 WHERE id=$1`, c.ID, c.Pos.MapID, c.Pos.X, c.Pos.Y, int64(c.SceneInstance))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrCharNotFound
	}
	return nil
}

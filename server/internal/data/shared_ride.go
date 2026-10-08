package data

import (
	"context"
	"fmt"
)

func LoadRideInviteSeconds(ctx context.Context, q Querier) (int32, error) {
	rows, err := q.Query(ctx, `SELECT invite_seconds FROM game_shared_ride_rule WHERE id=1`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var seconds int32
	if !rows.Next() {
		return 0, fmt.Errorf("data: 缺少同乘邀请规则")
	}
	if err := rows.Scan(&seconds); err != nil {
		return 0, err
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("data: 同乘邀请有效期无效")
	}
	return seconds, rows.Err()
}

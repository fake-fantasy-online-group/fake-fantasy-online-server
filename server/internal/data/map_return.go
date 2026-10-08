package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// MapReturnTable 按静态地图号配置死亡回城和回城物品的共同落点。
// map.def 的 ReviveMap/RevivePos 只用于离线溯源，运行时只读 PostgreSQL。
type MapReturnTable map[int32]domain.Pos

func LoadMapReturns(ctx context.Context, q Querier) (MapReturnTable, error) {
	rows, err := q.Query(ctx, `
		SELECT m.id, COALESCE(r.to_map_id, 0), COALESCE(r.x, 0), COALESCE(r.y, 0)
		  FROM map_defs m
		  LEFT JOIN game_map_returns r ON r.map_id = m.id
		 ORDER BY m.id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查地图回城点: %w", err)
	}
	defer rows.Close()

	out := make(MapReturnTable)
	for rows.Next() {
		var mapID, toMapID, x, y int32
		if err := rows.Scan(&mapID, &toMapID, &x, &y); err != nil {
			return nil, fmt.Errorf("data: 读地图回城点: %w", err)
		}
		if mapID <= 0 || toMapID <= 0 || x < 0 || y < 0 {
			return nil, fmt.Errorf("data: 地图 %d 回城点缺失或无效", mapID)
		}
		out[mapID] = domain.Pos{MapID: toMapID, X: float64(x), Y: float64(y)}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历地图回城点: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("data: 地图回城点为空")
	}
	return out, nil
}

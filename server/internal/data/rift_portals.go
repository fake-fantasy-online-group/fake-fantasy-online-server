package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// RiftPortalAreas contains source geometry only. A rift gate never uses its
// scenery's public destination, script, price, or dungeon admission behavior.
type RiftPortalAreas map[int32][]domain.Polygon

func LoadRiftPortalAreas(ctx context.Context, q Querier) (RiftPortalAreas, error) {
	rows, err := q.Query(ctx, `SELECT src.id, t.poly
		FROM map_teleports t JOIN map_defs src ON src.file = t.map_file
		ORDER BY src.id, t.proc_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查询洞天传送门区域: %w", err)
	}
	defer rows.Close()
	out := RiftPortalAreas{}
	for rows.Next() {
		var source int32
		var points []int32
		if err := rows.Scan(&source, &points); err != nil {
			return nil, fmt.Errorf("data: 读取洞天传送门区域: %w", err)
		}
		if len(points) < 6 || len(points)%2 != 0 {
			continue
		}
		area := make(domain.Polygon, 0, len(points)/2)
		for i := 0; i < len(points); i += 2 {
			area = append(area, domain.Pos{MapID: source, X: float64(points[i]), Y: float64(points[i+1])})
		}
		out[source] = append(out[source], area)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历洞天传送门区域: %w", err)
	}
	return out, nil
}

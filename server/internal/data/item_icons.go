package data

import (
	"context"
	"fmt"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadItemIconMappings(ctx context.Context, q Querier) ([]domain.ItemIconMapping, error) {
	rows, err := q.Query(ctx, `
		SELECT arm_id, icon_small
		  FROM gamedata.ov_item_icon_special
		 ORDER BY row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查特殊物品图标: %w", err)
	}
	defer rows.Close()
	var out []domain.ItemIconMapping
	for rows.Next() {
		var item int64
		var rawIcon string
		if err := rows.Scan(&item, &rawIcon); err != nil {
			return nil, fmt.Errorf("data: 读特殊物品图标: %w", err)
		}
		icon, err := strconv.ParseInt(rawIcon, 10, 32)
		if err != nil || item <= 0 || item > 1<<31-1 || icon <= 0 {
			return nil, fmt.Errorf("data: 非法特殊物品图标 item=%d icon=%q", item, rawIcon)
		}
		out = append(out, domain.ItemIconMapping{Item: domain.ItemID(item), Icon: int32(icon)})
		if len(out) > 1<<16-1 {
			return nil, fmt.Errorf("data: 特殊物品图标超过 U16 上限: %d", len(out))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历特殊物品图标: %w", err)
	}

	more, err := q.Query(ctx, `SELECT item_id,icon FROM game_pet_carrier_items ORDER BY item_id`)
	if err != nil {
		return nil, err
	}
	defer more.Close()
	for more.Next() {
		var v domain.ItemIconMapping
		if err = more.Scan(&v.Item, &v.Icon); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, more.Err()
}

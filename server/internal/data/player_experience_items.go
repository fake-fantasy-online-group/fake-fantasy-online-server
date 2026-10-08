package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadPlayerExperienceItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT item_id,player_level,experience
		FROM game_item_player_experience ORDER BY item_id,player_level`)
	if err != nil {
		return fmt.Errorf("data: 查角色经验道具: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var level int32
		var experience int64
		if err := rows.Scan(&id, &level, &experience); err != nil {
			return fmt.Errorf("data: 读角色经验道具: %w", err)
		}
		item, ok := items[id]
		if !ok || item.Equip != nil || !item.InventoryTabKnown || level < 1 || experience <= 0 {
			return fmt.Errorf("data: 角色经验道具配置无效 item=%d level=%d exp=%d", id, level, experience)
		}
		if item.PlayerExperience == nil {
			item.PlayerExperience = make(map[int32]int64)
		}
		item.PlayerExperience[level] = experience
		item.ConsumeOnUse = true
		items[id] = item
	}
	return rows.Err()
}

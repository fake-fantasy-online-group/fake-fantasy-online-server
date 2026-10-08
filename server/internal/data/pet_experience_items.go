package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadPetExperienceItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT item_id,pet_level,experience,max_above_owner
		FROM game_item_pet_experience ORDER BY item_id,pet_level`)
	if err != nil {
		return fmt.Errorf("data: 查宠物经验道具: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var level, maxAbove int32
		var experience int64
		if err := rows.Scan(&id, &level, &experience, &maxAbove); err != nil {
			return fmt.Errorf("data: 读宠物经验道具: %w", err)
		}
		item, ok := items[id]
		if !ok || item.Equip != nil || level < 1 || experience <= 0 || maxAbove < 0 {
			return fmt.Errorf("data: 宠物经验道具配置无效 item=%d level=%d exp=%d", id, level, experience)
		}
		if item.PetExperience == nil {
			item.PetExperience = &domain.PetExperienceItem{
				MaxAboveOwner: maxAbove, ByLevel: make(map[int32]int64),
			}
		}
		if item.PetExperience.MaxAboveOwner != maxAbove {
			return fmt.Errorf("data: 宠物经验道具%d等级限制不一致", id)
		}
		item.PetExperience.ByLevel[level] = experience
		// Item_ppoul 的 use_waste=0 与原作实际消耗行为冲突，以独立配置为准。
		item.ConsumeOnUse = true
		items[id] = item
	}
	return rows.Err()
}

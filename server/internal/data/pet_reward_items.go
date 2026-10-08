package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadPetRewardItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT item_id,pet_id,prefix_id,weight FROM game_item_pet_rewards ORDER BY item_id,pet_id`)
	if err != nil {
		return fmt.Errorf("data: 查宠物礼盒奖池: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var r domain.PetItemReward
		if err := rows.Scan(&id, &r.Pet, &r.Prefix, &r.Weight); err != nil {
			return err
		}
		d, ok := items[id]
		if !ok || d.Equip != nil || !d.InventoryTabKnown || r.Pet <= 0 || r.Prefix < 0 || r.Weight <= 0 || r.Weight > 10000 {
			return fmt.Errorf("data: 宠物礼盒%d奖池无效", id)
		}
		d.PetRewards = append(d.PetRewards, r)
		d.ConsumeOnUse = true
		items[id] = d
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for id, item := range items {
		if len(item.PetRewards) == 0 {
			continue
		}
		var total int64
		for _, reward := range item.PetRewards {
			total += int64(reward.Weight)
		}
		if total != 10000 {
			return fmt.Errorf("data: 宠物礼盒%d权重合计%d，应为10000", id, total)
		}
	}
	return nil
}

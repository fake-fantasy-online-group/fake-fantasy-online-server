package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadSaddles(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT s.item_id,s.speed_bonus_bp,s.passengers,
		s.combination_model,s.distinct_pets,s.use_pet_max_speed,p.pet_id
		FROM game_item_saddles s JOIN game_item_saddle_pets p USING(item_id)
		ORDER BY s.item_id,p.pet_id`)
	if err != nil {
		return fmt.Errorf("data: 查鞍具: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var pet domain.PetID
		var d domain.SaddleDef
		if err := rows.Scan(&id, &d.SpeedBonusBP, &d.Passengers, &d.CombinationModel, &d.DistinctPets, &d.UsePetMaxSpeed, &pet); err != nil {
			return err
		}
		item, ok := items[id]
		if !ok || item.Equip != nil || d.SpeedBonusBP < 0 || d.Passengers < 1 || pet <= 0 {
			return fmt.Errorf("data: 鞍具配置无效 item=%d pet=%d", id, pet)
		}
		if item.Saddle == nil {
			item.Saddle = &d
		}
		// 鞍具是特殊宠物的可重复骑乘入口；原始 use_waste 不是它的消耗规则。
		item.ConsumeOnUse = false
		item.Saddle.Pets = append(item.Saddle.Pets, pet)
		items[id] = item
	}
	return rows.Err()
}

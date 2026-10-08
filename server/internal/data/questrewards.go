package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadQuestProfessionRewards(ctx context.Context, q Querier, quests domain.QuestTable) error {
	rows, err := q.Query(ctx, `SELECT r.task_id,r.race,r.item_id,r.refine_level,
		e.id IS NOT NULL FROM game_task_profession_rewards r
		LEFT JOIN game_equipment e ON e.id=r.item_id ORDER BY r.task_id,r.race`)
	if err != nil {
		return fmt.Errorf("data: 查职业任务奖励: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var task, race, item, refine int32
		var equipment bool
		if err := rows.Scan(&task, &race, &item, &refine, &equipment); err != nil {
			return err
		}
		id := domain.QuestID(task)
		def, exists := quests[id]
		if !exists || race < 0 || race > int32(domain.Warlock) || !equipment || refine < 0 || refine > 10 || def.Exchange != nil {
			return fmt.Errorf("data: 职业任务奖励无效 task=%d race=%d item=%d refine=%d", task, race, item, refine)
		}
		if def.ProfessionRewards == nil {
			def.ProfessionRewards = make(map[domain.Race]domain.QuestEquipmentReward)
		}
		key := domain.Race(race)
		if _, duplicate := def.ProfessionRewards[key]; duplicate {
			return fmt.Errorf("data: 职业任务奖励重复 task=%d race=%d", task, race)
		}
		def.ProfessionRewards[key] = domain.QuestEquipmentReward{Item: domain.ItemID(item), RefineLevel: refine}
		quests[id] = def
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, def := range quests {
		if len(def.ProfessionRewards) > 0 && len(def.ProfessionRewards) != 5 {
			return fmt.Errorf("data: 任务%d职业奖励未覆盖全部五条路线", def.ID)
		}
	}
	return nil
}

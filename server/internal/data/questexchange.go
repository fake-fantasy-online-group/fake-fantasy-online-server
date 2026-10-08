package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// applyQuestExchanges 从 PostgreSQL 接入“交多少算多少”的任务规则。自然语言奖励
// 解析会把多种物品的单价、多个档位错误相加；配置了兑换规则的任务必须清空那份
// 固定奖励，只使用此处经过结构化校验的逐件奖励与最高档奖励。
func applyQuestExchanges(ctx context.Context, q Querier, all domain.QuestTable) error {
	rows, err := q.Query(ctx, `
		SELECT task_id, min_total_qty
		  FROM game_task_exchange_rules
		 ORDER BY task_id`)
	if err != nil {
		return fmt.Errorf("data: 查任务按量兑换规则: %w", err)
	}
	rules := make(map[domain.QuestID]*domain.QuestExchangeRule)
	for rows.Next() {
		var taskID, minTotalQty int32
		if err := rows.Scan(&taskID, &minTotalQty); err != nil {
			rows.Close()
			return fmt.Errorf("data: 读任务按量兑换规则: %w", err)
		}
		id := domain.QuestID(taskID)
		def, ok := all[id]
		if !ok {
			rows.Close()
			return fmt.Errorf("data: 按量兑换任务 %d 不在任务表", taskID)
		}
		if !def.Repeatable || minTotalQty <= 0 || !questRewardEmpty(def.Reward) {
			rows.Close()
			return fmt.Errorf("data: 按量兑换任务 %d 规则或固定奖励无效", taskID)
		}
		if _, dup := rules[id]; dup {
			rows.Close()
			return fmt.Errorf("data: 按量兑换任务 %d 重复", taskID)
		}
		rules[id] = &domain.QuestExchangeRule{MinTotalQty: minTotalQty}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("data: 遍历任务按量兑换规则: %w", err)
	}
	rows.Close()

	exchangeItemRows, err := q.Query(ctx, `
		SELECT task_id, ordinal, item_id, item_name,
		       unit_exp, unit_honor, unit_gold, unit_silver, unit_copper
		  FROM game_task_exchange_items
		 ORDER BY task_id, ordinal`)
	if err != nil {
		return fmt.Errorf("data: 查任务按量兑换物品: %w", err)
	}
	for exchangeItemRows.Next() {
		var taskID, ordinal, itemID int32
		var item domain.QuestExchangeItem
		if err := exchangeItemRows.Scan(&taskID, &ordinal, &itemID, &item.ItemName,
			&item.UnitReward.Exp, &item.UnitReward.Honor,
			&item.UnitReward.Money.Gold, &item.UnitReward.Money.Silver,
			&item.UnitReward.Money.Copper); err != nil {
			exchangeItemRows.Close()
			return fmt.Errorf("data: 读任务按量兑换物品: %w", err)
		}
		rule := rules[domain.QuestID(taskID)]
		if rule == nil || ordinal != int32(len(rule.Items)+1) || itemID <= 0 || item.ItemName == "" {
			exchangeItemRows.Close()
			return fmt.Errorf("data: 按量兑换任务 %d 的第 %d 件物品无效", taskID, ordinal)
		}
		item.Item = domain.ItemID(itemID)
		rule.Items = append(rule.Items, item)
	}
	if err := exchangeItemRows.Err(); err != nil {
		exchangeItemRows.Close()
		return fmt.Errorf("data: 遍历任务按量兑换物品: %w", err)
	}
	exchangeItemRows.Close()

	for id, rule := range rules {
		def := all[id]
		if len(rule.Items) == 0 || (def.GoalItem != 0 && def.GoalItem != rule.Items[0].Item) {
			return fmt.Errorf("data: 按量兑换任务 %d 没有物品或首件物品不匹配", id)
		}
	}

	tierRows, err := q.Query(ctx, `
		SELECT x.task_id, x.min_qty, x.exp, x.honor, x.gold, x.silver, x.copper,
		       x.title, x.title = '' OR EXISTS (
		           SELECT 1 FROM game_titles t WHERE t.title = x.title)
		  FROM game_task_exchange_tiers x
		 ORDER BY x.task_id, x.min_qty`)
	if err != nil {
		return fmt.Errorf("data: 查任务按量兑换档位: %w", err)
	}
	for tierRows.Next() {
		var taskID int32
		var tier domain.QuestExchangeTier
		var titleExists bool
		if err := tierRows.Scan(&taskID, &tier.MinQty, &tier.Reward.Exp, &tier.Reward.Honor,
			&tier.Reward.Money.Gold, &tier.Reward.Money.Silver, &tier.Reward.Money.Copper,
			&tier.Title, &titleExists); err != nil {
			tierRows.Close()
			return fmt.Errorf("data: 读任务按量兑换档位: %w", err)
		}
		rule := rules[domain.QuestID(taskID)]
		if rule == nil || tier.MinQty < rule.MinTotalQty || !titleExists ||
			(len(rule.Tiers) > 0 && tier.MinQty <= rule.Tiers[len(rule.Tiers)-1].MinQty) {
			tierRows.Close()
			return fmt.Errorf("data: 按量兑换任务 %d 档位 %d 无效", taskID, tier.MinQty)
		}
		rule.Tiers = append(rule.Tiers, tier)
	}
	if err := tierRows.Err(); err != nil {
		tierRows.Close()
		return fmt.Errorf("data: 遍历任务按量兑换档位: %w", err)
	}
	tierRows.Close()

	tierItemRows, err := q.Query(ctx, `
		SELECT task_id, min_qty, item_id, qty
		  FROM game_task_exchange_tier_items
		 ORDER BY task_id, min_qty, seq`)
	if err != nil {
		return fmt.Errorf("data: 查任务按量兑换档位物品: %w", err)
	}
	for tierItemRows.Next() {
		var taskID, minQty, itemID, qty int32
		if err := tierItemRows.Scan(&taskID, &minQty, &itemID, &qty); err != nil {
			tierItemRows.Close()
			return fmt.Errorf("data: 读任务按量兑换档位物品: %w", err)
		}
		rule := rules[domain.QuestID(taskID)]
		if rule == nil || itemID <= 0 || qty <= 0 {
			tierItemRows.Close()
			return fmt.Errorf("data: 按量兑换任务 %d 档位物品无效", taskID)
		}
		found := false
		for i := range rule.Tiers {
			if rule.Tiers[i].MinQty == minQty {
				rule.Tiers[i].Reward.Items = append(rule.Tiers[i].Reward.Items,
					domain.RewardItem{Item: domain.ItemID(itemID), Qty: qty})
				found = true
				break
			}
		}
		if !found {
			tierItemRows.Close()
			return fmt.Errorf("data: 按量兑换任务 %d 不存在档位 %d", taskID, minQty)
		}
	}
	if err := tierItemRows.Err(); err != nil {
		tierItemRows.Close()
		return fmt.Errorf("data: 遍历任务按量兑换档位物品: %w", err)
	}
	tierItemRows.Close()

	for id, rule := range rules {
		def := all[id]
		def.Exchange = rule
		all[id] = def
	}
	return nil
}

func questRewardEmpty(reward domain.QuestReward) bool {
	return reward.Exp == 0 && reward.Honor == 0 && reward.Money.Empty() && len(reward.Items) == 0
}

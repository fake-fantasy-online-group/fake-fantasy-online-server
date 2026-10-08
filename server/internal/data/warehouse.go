package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadWarehouseRule(ctx context.Context, q Querier) (domain.WarehouseRule, error) {
	var r domain.WarehouseRule
	var pages, slots, moveDir, moneyMode, maxPages int16
	rows, err := q.Query(ctx, `
		SELECT initial_pages, slots_per_page, max_stack, move_deposit_dir, money_deposit_mode,
		       max_pages, expand_base_cost, expand_cost_step
		  FROM game_warehouse_rule
		 WHERE singleton = TRUE`)
	if err != nil {
		return r, fmt.Errorf("data: 查个人仓库规则: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if rowErr := rows.Err(); rowErr != nil {
			return r, fmt.Errorf("data: 遍历个人仓库规则: %w", rowErr)
		}
		return r, fmt.Errorf("data: 缺少个人仓库规则")
	}
	if scanErr := rows.Scan(&pages, &slots, &r.MaxStack, &moveDir, &moneyMode, &maxPages,
		&r.ExpandBaseCost, &r.ExpandCostStep); scanErr != nil {
		return r, fmt.Errorf("data: 读个人仓库规则: %w", scanErr)
	}
	r.InitialPages, r.SlotsPerPage = uint8(pages), int(slots)
	r.MoveDepositDir, r.MoneyDepositMode = uint8(moveDir), uint8(moneyMode)
	r.MaxPages = uint8(maxPages)
	if !r.Valid() {
		return domain.WarehouseRule{}, fmt.Errorf("data: 非法个人仓库规则: %+v", r)
	}
	return r, nil
}

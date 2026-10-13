package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// The optional table keeps old database audit fixtures readable. Once present,
// malformed rules or any missing/ambiguous selected affix abort loading.
func loadStatusOverlayStrengths(ctx context.Context, q Querier, statuses domain.StatusTable) error {
	rows, err := q.Query(ctx, `SELECT to_regclass('game_status_overlay_strengths') IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("data: 查状态覆盖强度表: %w", err)
	}
	present := false
	if rows.Next() {
		err = rows.Scan(&present)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return fmt.Errorf("data: 读状态覆盖强度表: %w", err)
	}
	if !present {
		return nil
	}
	rows, err = q.Query(ctx, `SELECT status_id, attr_id, mode, value_order FROM game_status_overlay_strengths ORDER BY status_id`)
	if err != nil {
		return fmt.Errorf("data: 查状态覆盖强度规则: %w", err)
	}
	defer rows.Close()
	seen := map[domain.StatusID]bool{}
	for rows.Next() {
		var id domain.StatusID
		var rule domain.StatusOverlayStrength
		if err := rows.Scan(&id, &rule.Attr, &rule.Mode, &rule.ValueOrder); err != nil {
			return fmt.Errorf("data: 读状态覆盖强度规则: %w", err)
		}
		if id <= 0 || seen[id] || !rule.Valid() || !knownAttr(rule.Attr) {
			return fmt.Errorf("data: 状态 %d 覆盖强度规则非法或重复", id)
		}
		seen[id] = true
		count := 0
		for key, def := range statuses {
			if key.ID != id {
				continue
			}
			def.OverlayStrength = &rule
			if _, valid := def.OverlayPriority(); !valid {
				return fmt.Errorf("data: 状态 %d/%d 覆盖强度属性缺失、模式错误或重复", id, key.Level)
			}
			statuses[key] = def
			count++
		}
		if count == 0 {
			return fmt.Errorf("data: 状态 %d 覆盖强度规则引用不存在的定义", id)
		}
	}
	return rows.Err()
}

package data

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadStatusVisuals reads optional presentation-only mappings. Old databases
// without the table retain their existing behavior; malformed configured rows
// fail loading rather than guessing a replacement status or effect.
func LoadStatusVisuals(ctx context.Context, q Querier) (domain.StatusVisualTable, error) {
	out := domain.StatusVisualTable{}
	rows, err := q.Query(ctx, `SELECT to_regclass('game_status_visuals') IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("data: 查状态显示映射表: %w", err)
	}
	if !rows.Next() {
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("data: 读状态显示映射表: %w", err)
		}
		return nil, fmt.Errorf("data: 状态显示映射表存在性查询没有结果")
	}
	var present bool
	err = rows.Scan(&present)
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("data: 读状态显示映射表: %w", err)
	}
	if !present {
		return out, nil
	}

	rows, err = q.Query(ctx, `
		SELECT status_id, expected_name, effect_name, use_effect_name_for_self, position_mode
		  FROM game_status_visuals
		 ORDER BY status_id, expected_name`)
	if err != nil {
		return nil, fmt.Errorf("data: 查状态显示映射: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var expectedName string
		var rule domain.StatusVisualRule
		if err := rows.Scan(&id, &expectedName, &rule.EffectName, &rule.UseEffectNameForSelf, &rule.PositionMode); err != nil {
			return nil, fmt.Errorf("data: 读状态显示映射: %w", err)
		}
		if id <= 0 || id > 1<<31-1 {
			return nil, fmt.Errorf("data: 状态显示映射状态号 %d 非法", id)
		}
		if !validStatusVisualName(expectedName) {
			return nil, fmt.Errorf("data: 状态 %d 显示映射原名称 %q 非法", id, expectedName)
		}
		if rule.EffectName != "" && (!validStatusVisualName(rule.EffectName) ||
			strings.ContainsAny(rule.EffectName, `/\:`) || strings.Contains(rule.EffectName, "..") || rule.EffectName == ".") {
			return nil, fmt.Errorf("data: 状态 %d/%q 显示映射效果名称 %q 非法", id, expectedName, rule.EffectName)
		}
		if rule.PositionMode < domain.StatusVisualPositionInherit || rule.PositionMode > 3 {
			return nil, fmt.Errorf("data: 状态 %d/%q 显示映射位置 %d 越界", id, expectedName, rule.PositionMode)
		}
		key := domain.StatusVisualKey{ID: domain.StatusID(id), ExpectedName: expectedName}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("data: 状态 %d/%q 显示映射重复", id, expectedName)
		}
		out[key] = rule
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历状态显示映射: %w", err)
	}
	return out, nil
}

func validStatusVisualName(name string) bool {
	// Protocol strings use a U16 byte length; reject unusable keys at startup.
	return name != "" && len(name) <= 1<<16-1 && utf8.ValidString(name) && name == strings.TrimSpace(name) &&
		strings.IndexFunc(name, unicode.IsControl) < 0
}

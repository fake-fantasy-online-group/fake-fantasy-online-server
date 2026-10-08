package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 原始ResRefine为Cat/Level/ItemLevel/Res+4个ResResult。
// 旧列scheme_id实际是ItemLevel，rand_cat是Oper及保留字节；这里只从
// PostgreSQL读取已确认的字段，不在正式运行时读取本地表文件。
func loadRefineEffects(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT slot_mask,forge_level,scheme_id,
		reserved, ARRAY[rand_cat_1,rand_cat_2,rand_cat_3,rand_cat_4],
		ARRAY[attr_type_1,attr_type_2,attr_type_3,attr_type_4],
		ARRAY[unit_1,unit_2,unit_3,unit_4],
		ARRAY[success_rate_1,success_rate_2,success_rate_3,success_rate_4],
		ARRAY[value_1,value_2,value_3,value_4]
		FROM gamedata.ov_refinerand ORDER BY row_no`)
	if err != nil {
		return fmt.Errorf("data: 查精炼属性: %w", err)
	}
	defer rows.Close()
	type rawEffect struct {
		affixes []domain.Affix
		reason  string
	}
	rules := make(map[[2]int32]map[int32]rawEffect)
	for rows.Next() {
		var position, level, itemLevel, format int32
		var ops, attrs, units, probabilities, values []int32
		if err := rows.Scan(&position, &level, &itemLevel, &format, &ops, &attrs, &units, &probabilities, &values); err != nil {
			return err
		}
		if level <= 0 || len(attrs) != 4 || len(ops) != 4 || len(units) != 4 || len(probabilities) != 4 || len(values) != 4 {
			return fmt.Errorf("data: 精炼属性%d/%d/%d格式错误", position, itemLevel, level)
		}
		var r rawEffect
		if format != 0 {
			r.reason = "精炼特殊操作尚未支持"
		}
		for i, attr := range attrs {
			if attr == 0 {
				continue
			}
			if ops[i] != 1 || probabilities[i] != 100 || (units[i] != 0 && units[i] != 1) || values[i] < -32768 || values[i] > 32767 {
				r.reason = "精炼特殊操作尚未支持"
			}
			r.affixes = append(r.affixes, domain.Affix{Attr: attr, Mode: units[i], Value: values[i]})
		}
		key := [2]int32{position, itemLevel}
		if rules[key] == nil {
			rules[key] = make(map[int32]rawEffect)
		}
		if _, exists := rules[key][level]; exists {
			return fmt.Errorf("data: 精炼属性键重复 %v/+%d", key, level)
		}
		rules[key][level] = r
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, def := range items {
		if def.Equip == nil {
			continue
		}
		e := def.Equip
		e.RefineEffects = make(map[int32]domain.RefineEffect)
		for level, raw := range rules[[2]int32{e.Position, e.Tier}] {
			result := domain.RefineEffect{DisabledReason: raw.reason}
			if result.DisabledReason == "" {
				var ok bool
				result.Bonuses, ok = domain.ResolveRefineBonuses(e, raw.affixes)
				if !ok {
					result.DisabledReason = "精炼属性尚未支持"
				}
			}
			e.RefineEffects[level] = result
		}
	}
	return nil
}

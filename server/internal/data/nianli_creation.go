package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadNianliCreationRules 读取念力造物的服务端结果。客户端技能表只描述
// “制造小药瓶”，不能由服务端按名字猜产物或材料。
func LoadNianliCreationRules(ctx context.Context, q Querier, items ItemDefs,
	skills domain.SkillTable) (domain.NianliCreationTable, error) {
	rows, err := q.Query(ctx, `
		SELECT skill_id, product_item_id, product_qty, nianli_cost
		  FROM game_nianli_creation_rules ORDER BY skill_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查念力造物规则: %w", err)
	}
	defer rows.Close()
	out := domain.NianliCreationTable{}
	for rows.Next() {
		var rule domain.NianliCreationRule
		if err := rows.Scan(&rule.Skill, &rule.Product, &rule.ProductQty, &rule.NianliCost); err != nil {
			return nil, fmt.Errorf("data: 读念力造物规则: %w", err)
		}
		def, skillOK := skills.Get(rule.Skill, 1)
		_, itemOK := items[rule.Product]
		if !domain.IsNianliCreationSkill(rule.Skill) || !skillOK || def.Prof != 0 ||
			!itemOK || rule.ProductQty <= 0 || rule.NianliCost <= 0 {
			return nil, fmt.Errorf("data: 念力造物技能 %d 配置无效", rule.Skill)
		}
		out[rule.Skill] = rule
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历念力造物规则: %w", err)
	}
	if len(out) != 3 {
		return nil, fmt.Errorf("data: 念力造物规则应有 3 条，实得 %d", len(out))
	}

	materialRows, err := q.Query(ctx, `
		SELECT skill_id, material_order, item_id, qty
		  FROM game_nianli_creation_materials ORDER BY skill_id, material_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查念力造物材料: %w", err)
	}
	defer materialRows.Close()
	for materialRows.Next() {
		var skill domain.SkillID
		var order, qty int32
		var item domain.ItemID
		if err := materialRows.Scan(&skill, &order, &item, &qty); err != nil {
			return nil, fmt.Errorf("data: 读念力造物材料: %w", err)
		}
		rule, ok := out[skill]
		_, itemOK := items[item]
		if !ok || !itemOK || order != int32(len(rule.Materials))+1 || qty <= 0 {
			return nil, fmt.Errorf("data: 念力造物技能 %d 第 %d 条材料配置无效", skill, order)
		}
		rule.Materials = append(rule.Materials, domain.NianliCreationMaterial{Item: item, Qty: qty})
		out[skill] = rule
	}
	if err := materialRows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历念力造物材料: %w", err)
	}
	return out, nil
}

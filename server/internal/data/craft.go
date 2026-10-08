package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadCraftRecipes 加载 1~5 材料配方：客户端 ov_combine 投影 + 业务补充配方。
//
// 补充配方（game_craft_extra_recipes）放不下客户端原始表的行，例如锋刃卡升阶
// 那种"失败材料消失"的配方；row_no 段从 4900 起，不与原始行冲突。
func LoadCraftRecipes(ctx context.Context, q Querier, items ItemDefs) (domain.CraftTable, error) {
	out := domain.CraftTable{
		ByType:    map[uint8][]domain.CraftRecipe{},
		ByProduct: map[uint8]map[domain.ItemID][]domain.CraftRecipe{},
	}
	rows, err := q.Query(ctx, `
		SELECT row_no, product_item_id, cost, need_lifeskill_id,
		       prof_need, prof_max, prof_gain, disappear_rate, success_rate,
		       skill_level_need, mat_count, mat_1, mat_2, mat_3, mat_4, mat_5,
		       out_qty, show_type
		  FROM gamedata.ov_combine
		UNION ALL
		SELECT row_no, product_item_id, cost, need_lifeskill_id,
		       prof_need, prof_max, prof_gain, disappear_rate, success_rate,
		       skill_level_need, mat_count, mat_1, mat_2, mat_3, mat_4, mat_5,
		       out_qty, show_type
		  FROM game_craft_extra_recipes
	 ORDER BY show_type, row_no`)
	if err != nil {
		return out, fmt.Errorf("data: 查合成配方: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d domain.CraftRecipe
		var product, requiredSkill int32
		var matCount int16
		var mats [5]int64
		var showType int64
		if err := rows.Scan(&d.RowNo, &product, &d.Cost, &requiredSkill,
			&d.ProficiencyMin, &d.ProficiencyMax, &d.ProficiencyGain,
			&d.DisappearRate, &d.SuccessRate, &d.SkillLevelNeed, &matCount,
			&mats[0], &mats[1], &mats[2], &mats[3], &mats[4],
			&d.OutputQty, &showType); err != nil {
			return out, fmt.Errorf("data: 读合成配方: %w", err)
		}
		if showType < 0 || showType > 255 || matCount < 0 || matCount > 5 ||
			d.OutputQty <= 0 || d.Cost < 0 || d.Cost > 1<<31-1 ||
			d.SuccessRate < 0 || d.SuccessRate > 100 || d.DisappearRate < 0 ||
			d.SuccessRate+d.DisappearRate > 100 {
			return out, fmt.Errorf("data: 合成配方 row=%d 参数越界", d.RowNo)
		}
		d.ShowType, d.Product, d.RequiredSkill = uint8(showType), domain.ItemID(product), domain.SkillID(requiredSkill)
		if _, ok := items[d.Product]; !ok {
			return out, fmt.Errorf("data: 合成配方 row=%d 产物 %d 不存在", d.RowNo, d.Product)
		}
		for i := 0; i < int(matCount); i++ {
			id, qty := domain.ItemID(uint16(mats[i])), int32(uint64(mats[i])>>16)
			if id <= 0 || qty <= 0 {
				return out, fmt.Errorf("data: 合成配方 row=%d 第%d材料非法 %#x", d.RowNo, i, mats[i])
			}
			if _, ok := items[id]; !ok {
				return out, fmt.Errorf("data: 合成配方 row=%d 材料 %d 不存在", d.RowNo, id)
			}
			d.Materials = append(d.Materials, domain.CraftMaterial{Item: id, Qty: qty})
		}
		out.ByType[d.ShowType] = append(out.ByType[d.ShowType], d)
		if out.ByProduct[d.ShowType] == nil {
			out.ByProduct[d.ShowType] = map[domain.ItemID][]domain.CraftRecipe{}
		}
		out.ByProduct[d.ShowType][d.Product] = append(out.ByProduct[d.ShowType][d.Product], d)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("data: 遍历合成配方: %w", err)
	}
	return out, nil
}

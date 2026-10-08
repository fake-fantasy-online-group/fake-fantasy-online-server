package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadWashRecipes(ctx context.Context, q Querier, items ItemDefs) (domain.WashTable, error) {
	out := domain.WashTable{}
	rows, err := q.Query(ctx, `
		SELECT p.row_no, p.arm_type1, p.arm_type2, p.arm_type3, p.arm_level,
		       p.hole_id, p.material_id, p.material_count, p.cost_gold,
		       e.weight, ce.attr_id, ce.value, ce.mode
		  FROM gamedata.ov_rebarmattrpurify p
		  JOIN gamedata.ov_rebarmattrpurify_entry e USING (row_no)
		  JOIN gamedata.ov_card c ON c.index = e.card_id
		  JOIN gamedata.ov_card_entry ce ON ce.row_no = c.row_no
		 WHERE ce.op_type = 1 AND ce.prob = 100 AND ce.attr_id <> 0
	 ORDER BY p.row_no, e.idx, ce.idx`)
	if err != nil {
		return out, fmt.Errorf("data: 查洗练配方: %w", err)
	}
	defer rows.Close()
	type raw struct {
		recipe  domain.WashRecipe
		classes [3]int32
	}
	byRow := map[int32]*raw{}
	for rows.Next() {
		var rowNo, tier, quality int32
		var classes [3]int32
		var material int64
		var choice domain.WashChoice
		var qty int32
		var cost int64
		if err := rows.Scan(&rowNo, &classes[0], &classes[1], &classes[2], &tier,
			&quality, &material, &qty, &cost, &choice.Weight,
			&choice.Affix.Attr, &choice.Affix.Value, &choice.Affix.Mode); err != nil {
			return out, fmt.Errorf("data: 读洗练配方: %w", err)
		}
		if quality < 1 || quality > 4 || material <= 0 || qty <= 0 || cost < 0 ||
			cost > 1<<31-1 || choice.Weight <= 0 {
			return out, fmt.Errorf("data: 洗练配方 row=%d 参数非法", rowNo)
		}
		if _, ok := items[domain.ItemID(material)]; !ok {
			return out, fmt.Errorf("data: 洗练配方 row=%d 材料%d不存在", rowNo, material)
		}
		r := byRow[rowNo]
		if r == nil {
			r = &raw{classes: classes, recipe: domain.WashRecipe{RowNo: rowNo,
				EquipTier: tier, Quality: uint8(quality), Material: domain.ItemID(material),
				MaterialQty: qty, Cost: cost}}
			byRow[rowNo] = r
		}
		r.recipe.Choices = append(r.recipe.Choices, choice)
	}
	for _, r := range byRow {
		for _, class := range r.classes {
			if class == 0 {
				continue
			}
			recipe := r.recipe
			recipe.EquipClass = class
			key := [3]int32{class, recipe.EquipTier, int32(recipe.Quality)}
			if _, dup := out[key]; dup {
				return out, fmt.Errorf("data: 洗练配方键重复 %v", key)
			}
			out[key] = recipe
		}
	}
	return out, rows.Err()
}

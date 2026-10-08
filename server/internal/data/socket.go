package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadSocketRecipes(ctx context.Context, q Querier, items ItemDefs) (domain.SocketTable, error) {
	out := domain.SocketTable{}
	rows, err := q.Query(ctx, `
		SELECT row_no, hole_pos, arm_slot, taiji_level, hole_cost, inlay_cost,
		       need_lifeskill_id, destroy_rate, success_rate, mat_count,
		       mat_1id, mat_1_count, mat_2id, mat_2_count, protect_item
		  FROM gamedata.ov_enchase ORDER BY row_no`)
	if err != nil {
		return out, fmt.Errorf("data: 查打孔镶嵌配方: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.SocketRecipe
		var hole int16
		var skill int32
		var matCount int16
		var ids, qty [2]int32
		if err := rows.Scan(&r.RowNo, &hole, &r.EquipClass, &r.EquipTier,
			&r.DrillCost, &r.InlayCost, &skill, &r.DestroyRate, &r.SuccessRate,
			&matCount, &ids[0], &qty[0], &ids[1], &qty[1], &r.ProtectItem); err != nil {
			return out, fmt.Errorf("data: 读打孔镶嵌配方: %w", err)
		}
		r.RequiredSkill = domain.SkillID(skill)
		if hole < 0 || hole > 4 || matCount < 0 || matCount > 2 ||
			r.DrillCost < 0 || r.DrillCost > 1<<31-1 || r.InlayCost < 0 || r.InlayCost > 1<<31-1 ||
			r.SuccessRate < 0 || r.SuccessRate > 100 || r.DestroyRate < 0 ||
			r.DestroyRate > 100 {
			return out, fmt.Errorf("data: 打孔镶嵌配方 row=%d 参数越界", r.RowNo)
		}
		r.Hole = uint8(hole)
		for i := 0; i < int(matCount); i++ {
			id := domain.ItemID(ids[i])
			if id <= 0 || qty[i] <= 0 {
				return out, fmt.Errorf("data: 打孔镶嵌配方 row=%d 材料%d非法", r.RowNo, i)
			}
			if _, ok := items[id]; !ok {
				return out, fmt.Errorf("data: 打孔镶嵌配方 row=%d 材料%d不存在", r.RowNo, id)
			}
			r.Materials = append(r.Materials, domain.CraftMaterial{Item: id, Qty: qty[i]})
		}
		if r.ProtectItem != 0 {
			if _, ok := items[r.ProtectItem]; !ok {
				return out, fmt.Errorf("data: 打孔镶嵌配方 row=%d 保护物%d不存在", r.RowNo, r.ProtectItem)
			}
		}
		key := [3]int32{int32(r.Hole), r.EquipClass, r.EquipTier}
		if _, dup := out[key]; dup {
			return out, fmt.Errorf("data: 打孔镶嵌配方键重复 %v", key)
		}
		out[key] = r
	}
	return out, rows.Err()
}

package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadRefineRecipes(ctx context.Context, q Querier, items ItemDefs) (domain.RefineTable, error) {
	out := domain.RefineTable{}
	rows, err := q.Query(ctx, `
		SELECT r.row_no, r.refine_level, r.arm_slot, r.arm_level, r.cost, r.need_lifeskill_id,
		       r.prof_need, r.prof_max, r.prof_gain, r.success_rate, r.fail_downgrade,
		       r.mat_count, r.mat_1id, r.mat_1_count, r.mat_2id, r.mat_2_count,
		       r.mat_3id, r.mat_3_count,
		       COALESCE(protect.protect_item, r.protect_item),
		       r.refine_level >= policy.destroy_from_level,
               CASE WHEN r.refine_level >= policy.destroy_from_level THEN policy.confirm_cooldown_ms ELSE 0 END
		  FROM gamedata.ov_refine r
		  LEFT JOIN game_refine_protect_items protect ON protect.refine_level = r.refine_level
		  CROSS JOIN game_refine_policy policy
		 WHERE policy.id=1 ORDER BY r.row_no`)
	if err != nil {
		return out, fmt.Errorf("data: 查精炼配方: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.RefineRecipe
		var skill int32
		var matCount int16
		var ids, qty [3]int32
		if err := rows.Scan(&r.RowNo, &r.CurrentLevel, &r.EquipClass, &r.EquipTier,
			&r.Cost, &skill, &r.ProficiencyMin, &r.ProficiencyMax, &r.ProficiencyGain,
			&r.SuccessRate, &r.FailureLevel, &matCount,
			&ids[0], &qty[0], &ids[1], &qty[1], &ids[2], &qty[2], &r.ProtectItem, &r.FailureDestroys, &r.ConfirmCooldownMS); err != nil {
			return out, fmt.Errorf("data: 读精炼配方: %w", err)
		}
		r.RequiredSkill = domain.SkillID(skill)
		if matCount < 0 || matCount > 3 || r.Cost < 0 || r.Cost > 1<<31-1 ||
			r.SuccessRate < 0 || r.SuccessRate > 100 || r.CurrentLevel < 0 {
			return out, fmt.Errorf("data: 精炼配方 row=%d 参数越界", r.RowNo)
		}
		for i := 0; i < int(matCount); i++ {
			id := domain.ItemID(ids[i])
			if id <= 0 || qty[i] <= 0 {
				return out, fmt.Errorf("data: 精炼配方 row=%d 材料%d非法", r.RowNo, i)
			}
			if _, ok := items[id]; !ok {
				return out, fmt.Errorf("data: 精炼配方 row=%d 材料%d不存在", r.RowNo, id)
			}
			r.Materials = append(r.Materials, domain.CraftMaterial{Item: id, Qty: qty[i]})
		}
		if r.ProtectItem != 0 {
			if _, ok := items[r.ProtectItem]; !ok {
				return out, fmt.Errorf("data: 精炼配方 row=%d 保护物%d不存在", r.RowNo, r.ProtectItem)
			}
		}
		key := [3]int32{r.EquipClass, r.EquipTier, r.CurrentLevel}
		if _, dup := out[key]; dup {
			return out, fmt.Errorf("data: 精炼配方键重复 class/tier/level=%v", key)
		}
		out[key] = r
	}
	return out, rows.Err()
}

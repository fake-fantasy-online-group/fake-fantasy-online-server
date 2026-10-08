package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadPetSkillEffects(ctx context.Context, q Querier, defs domain.PetSkillTable) error {
	rows, err := q.Query(ctx, `SELECT skill_id,kind,chance_bp,chance_per_level_bp,value,value_per_level,source_attr,source_step,require_selected,cleanse_status_ids FROM game_pet_skill_effects ORDER BY skill_id`)
	if err != nil {
		return fmt.Errorf("data: 宠物天赋效果: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.SkillID
		var e domain.PetSkillEffect
		if err := rows.Scan(&id, &e.Kind, &e.ChanceBP, &e.ChancePerLevelBP, &e.Value, &e.ValuePerLevel, &e.SourceAttr, &e.SourceStep, &e.RequireSelected, &e.CleanseStatusIDs); err != nil {
			return err
		}
		d, ok := defs[id]
		if !ok {
			return fmt.Errorf("data: 宠物天赋技能不存在 %d", id)
		}
		if d.Active != e.RequireSelected {
			return fmt.Errorf("data: 宠物技能 %d(%s) 主动/被动配置与效果启用条件不一致", id, d.Name)
		}
		switch e.Kind {
		case "auto_pickup", "owner_heal", "guard_damage", "max_weight_from_vit",
			"escape_low_hp", "loyalty", "stupid", "hungry_scholar", "work_efficiency",
			"cleanse_status", "physical_reduction_flat", "magic_reduction_flat",
			"physical_extra", "magic_extra", "physical_reduction", "magic_reduction",
			"physical_attack_pct", "magic_attack_pct", "experience_bonus":
		default:
			return fmt.Errorf("data: 未支持宠物天赋 %s", e.Kind)
		}
		if e.ChanceBP < 0 || e.ChanceBP > 10000 || e.SourceStep <= 0 || e.Value < 0 {
			return fmt.Errorf("data: 宠物天赋参数非法 %d", id)
		}
		d.Effect = &e
		defs[id] = d
	}
	return rows.Err()
}

func loadPetEggItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT item_id,pet_id,name,description FROM game_item_pet_eggs ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 宠物蛋物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var pet domain.PetID
		var name, desc string
		if err := rows.Scan(&id, &pet, &name, &desc); err != nil {
			return err
		}
		d, ok := items[id]
		if !ok || pet <= 0 {
			return fmt.Errorf("data: 宠物蛋配置非法 %d", id)
		}
		d.GrantPetID = pet
		d.Name = name
		d.Description = desc
		items[id] = d
	}
	return rows.Err()
}

func LoadPickupRange(ctx context.Context, q Querier) (int32, error) {
	rows, err := q.Query(ctx, `SELECT pickup_range FROM game_pickup_rule WHERE id=1`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var n int32
	if !rows.Next() {
		return 0, fmt.Errorf("data: 缺少拾取规则")
	}
	if err = rows.Scan(&n); err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("data: 非法拾取范围 %d", n)
	}
	return n, rows.Err()
}

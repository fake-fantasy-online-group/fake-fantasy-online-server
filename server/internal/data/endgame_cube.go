package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadCubeRules reads normalized configuration exactly once at world startup.
// Missing configuration, unknown operations or dangling references fail closed.
func LoadCubeRules(ctx context.Context, q Querier, items map[domain.ItemID]domain.ItemDef) (*domain.CubeRuleTable, error) {
	rows, err := q.Query(ctx, `SELECT id,name,operation,description,target_required,money,source_count,quality_step_bp,material_count,output_count,enabled FROM game_cube_recipes ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load cube recipes: %w", err)
	}
	var defs []domain.CubeRecipeDef
	positions := map[int32]int{}
	enabled := map[int32]bool{}
	expectedChildren := map[int32][2]int32{}
	for rows.Next() {
		var d domain.CubeRecipeDef
		var materials, outputs int32
		var active bool
		if err = rows.Scan(&d.ID, &d.Name, &d.Operation, &d.Description, &d.TargetRequired, &d.Money, &d.SourceCount, &d.QualityStepBP, &materials, &outputs, &active); err != nil {
			rows.Close()
			return nil, err
		}
		if _, exists := positions[d.ID]; exists {
			rows.Close()
			return nil, fmt.Errorf("duplicate cube recipe %d", d.ID)
		}
		enabled[d.ID] = active
		expectedChildren[d.ID] = [2]int32{materials, outputs}
		positions[d.ID] = len(defs)
		defs = append(defs, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT recipe_id,seq,item_id,count FROM game_cube_materials ORDER BY recipe_id,seq`)
	if err != nil {
		return nil, fmt.Errorf("load cube materials: %w", err)
	}
	for rows.Next() {
		var recipe, seq int32
		var m domain.CubeMaterialDef
		if err = rows.Scan(&recipe, &seq, &m.ItemID, &m.Count); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := positions[recipe]
		if !ok || seq != int32(len(defs[i].Materials)+1) {
			rows.Close()
			return nil, fmt.Errorf("invalid cube material sequence for %d", recipe)
		}
		defs[i].Materials = append(defs[i].Materials, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT recipe_id,seq,item_id,count,level_divisor,level_cap,multiplier FROM game_cube_outputs ORDER BY recipe_id,seq`)
	if err != nil {
		return nil, fmt.Errorf("load cube outputs: %w", err)
	}
	for rows.Next() {
		var recipe, seq int32
		var o domain.CubeOutputDef
		if err = rows.Scan(&recipe, &seq, &o.ItemID, &o.Count, &o.LevelDivisor, &o.LevelCap, &o.Multiplier); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := positions[recipe]
		if !ok || seq != int32(len(defs[i].Outputs)+1) {
			rows.Close()
			return nil, fmt.Errorf("invalid cube output sequence for %d", recipe)
		}
		defs[i].Outputs = append(defs[i].Outputs, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, d := range defs {
		if expectedChildren[d.ID] != [2]int32{int32(len(d.Materials)), int32(len(d.Outputs))} {
			return nil, fmt.Errorf("incomplete cube recipe %d ingredients/outputs", d.ID)
		}
	}
	table, err := domain.NewCubeRuleTable(defs, items)
	if err != nil {
		return nil, err
	}
	// Do not silently disable existing recipe identities on an incomplete migration.
	for _, id := range []int32{1001, 1002, 1003, 1006, 1007, 1008, 1009, 1010, 1011, 1012} {
		if _, ok := table.Recipe(id); !ok {
			return nil, fmt.Errorf("missing core cube recipe %d", id)
		}
	}
	for n := int32(1); n <= 28; n++ {
		for _, base := range []int32{11000, 12000} {
			if _, ok := table.Recipe(base + n); !ok {
				return nil, fmt.Errorf("missing core cube recipe %d", base+n)
			}
		}
	}
	// Validate every historical row above, then expose only enabled recipes.
	// Both input matching and legacy recipeId execution use this same snapshot.
	active := make([]domain.CubeRecipeDef, 0, len(defs))
	for _, d := range defs {
		if enabled[d.ID] {
			active = append(active, d)
		}
	}
	return domain.NewCubeRuleTable(active, items)
}

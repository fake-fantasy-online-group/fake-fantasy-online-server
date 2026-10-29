package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Rules are explicit authored facts. Never infer binding from trade permissions,
// item name, acquisition route or an instance-kind storage implementation detail.
func loadFixedItemBindings(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT id,fixed_binding,binding_rule_source FROM game_items WHERE fixed_binding IS NOT NULL UNION ALL SELECT id,fixed_binding,binding_rule_source FROM game_equipment WHERE fixed_binding IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("data: fixed item binding rules: %w", err)
	}
	defer rows.Close()
	seen := map[domain.ItemID]bool{}
	for rows.Next() {
		var id domain.ItemID
		var bound bool
		var source string
		if err := rows.Scan(&id, &bound, &source); err != nil {
			return err
		}
		def, ok := items[id]
		if !ok || def.Equip != nil || source == "" || seen[id] {
			return fmt.Errorf("data: invalid fixed binding rule for item %d", id)
		}
		seen[id] = true
		def.FixedBinding = domain.ItemBindingFixedUnbound
		if bound {
			def.FixedBinding = domain.ItemBindingFixedBound
		}
		if def.InstanceKind == domain.ItemInstanceSocketCard {
			def.CardDefaults.Bound = bound
		}
		items[id] = def
	}
	return rows.Err()
}

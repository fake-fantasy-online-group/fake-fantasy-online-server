package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadPetPPItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT i.item_id,p.pp_low,p.pp_high,p.success,p.total_limit
		FROM game_item_pet_pp i JOIN gamedata.ov_petppoint p ON p.pp_item=i.item_id
		WHERE i.kind='affection' ORDER BY i.item_id,p.pp_low`)
	if err != nil {
		return fmt.Errorf("data: 查宠爱PP果: %w", err)
	}
	defer rows.Close()
	var cap int32
	for rows.Next() {
		var id domain.ItemID
		var band domain.PetPPBand
		var limit int32
		if err := rows.Scan(&id, &band.Low, &band.High, &band.SuccessPct, &limit); err != nil {
			return err
		}
		item, ok := items[id]
		if !ok || item.Equip != nil || !item.InventoryTabKnown || limit <= 0 ||
			band.Low < 0 || band.High < band.Low || band.High > limit ||
			band.SuccessPct < 0 || band.SuccessPct > 100 || (cap != 0 && cap != limit) {
			return fmt.Errorf("data: 宠爱PP果%d分档无效", id)
		}
		cap = limit
		if item.PetAffectionPP == nil {
			item.PetAffectionPP = &domain.PetPPItem{Limit: limit}
		}
		bands := item.PetAffectionPP.Bands
		next := int32(0)
		if len(bands) > 0 {
			next = bands[len(bands)-1].High + 1
		}
		if band.Low != next {
			return fmt.Errorf("data: 宠爱PP果%d分档不连续", id)
		}
		item.PetAffectionPP.Bands = append(bands, band)
		item.ConsumeOnUse = true
		items[id] = item
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for id, item := range items {
		if rule := item.PetAffectionPP; rule != nil && rule.Bands[len(rule.Bands)-1].High < rule.Limit-1 {
			return fmt.Errorf("data: 宠爱PP果%d分档未覆盖上限", id)
		}
	}
	return nil
}

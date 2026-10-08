package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadPetCarrierItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT c.pet_id,c.item_id,c.bag_tab,c.icon,p.can_deal FROM game_pet_carrier_items c JOIN LATERAL(SELECT can_deal FROM gamedata.ov_petgrow p WHERE p.pet_id=c.pet_id ORDER BY row_no LIMIT 1) p ON TRUE ORDER BY c.pet_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.PetID
		var id domain.ItemID
		var tab uint8
		var icon, canDeal int32
		if err = rows.Scan(&p, &id, &tab, &icon, &canDeal); err != nil {
			return err
		}
		d, ok := items[id]
		if !ok || p <= 0 || tab >= domain.BagTabCount {
			return fmt.Errorf("invalid pet carrier %d", id)
		}
		d.PetCarrierSpecies = p
		d.InventoryTab, d.InventoryTabKnown = tab, true
		d.Icon = icon
		d.Stackable = false
		d.InstanceKind = domain.ItemInstancePet
		d.CanTrade = canDeal != 0
		d.CanMail = false
		d.SellPrice = -1
		d.ConsumeOnUse = false
		items[id] = d
	}
	return rows.Err()
}

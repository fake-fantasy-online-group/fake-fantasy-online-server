package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadDragonFusions(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT item_id,target_slot,base_bonus_pct FROM game_item_dragon_fusions ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 查龙系列融合: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var slot domain.EquipSlot
		var pct int32
		if err := rows.Scan(&id, &slot, &pct); err != nil {
			return err
		}
		item, ok := items[id]
		if !ok || item.Equip != nil || !item.InventoryTabKnown ||
			(slot != domain.SlotNeck && slot != domain.SlotFace) || pct <= 0 || pct > 100 {
			return fmt.Errorf("data: 龙系列%d目标部位或属性无效", id)
		}
		item.DragonFusion = &domain.EquipmentSoulDef{TargetSlot: slot}
		// 原始描述明确为五项基础属性，灵巧不在加成中。
		for _, attr := range []int32{domain.AttrSTR, domain.AttrAGI, domain.AttrVIT, domain.AttrINT, domain.AttrSPI} {
			item.DragonFusion.Affixes = append(item.DragonFusion.Affixes,
				domain.Affix{Attr: attr, Value: pct, Mode: domain.ModePercent})
		}
		items[id] = item
	}
	return rows.Err()
}

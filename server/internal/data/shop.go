package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadShops 读取全部 NPC 商店并解析它们的库存来源。stock_source_shop_id 允许
// 同职能 NPC 复用已确认的原服库存，但每个客户端 ShopID 仍有独立、可审计的配置行。
func LoadShops(ctx context.Context, q Querier, items ItemDefs) (domain.ShopTable, int, error) {
	rows, err := q.Query(ctx, `
		SELECT s.shop_id, s.allow_sell, s.allow_repair,
		       s.repair_price_bp, s.special_repair_price_bp, s.special_repair_nianli,
		       s.source_kind,
		       si.slot, si.item_id
		  FROM game_shops s
		  LEFT JOIN game_shop_items si
		    ON si.shop_id = COALESCE(s.stock_source_shop_id, s.shop_id)
		 ORDER BY s.shop_id, si.slot`)
	if err != nil {
		return nil, 0, fmt.Errorf("data: 查 NPC 商店: %w", err)
	}
	defer rows.Close()

	out := domain.ShopTable{}
	stockRows := 0
	for rows.Next() {
		var (
			shopID                                                   int32
			allowSell                                                bool
			allowRepair                                              bool
			repairPriceBP, specialRepairPriceBP, specialRepairNianli int32
			source                                                   string
			slot                                                     *int32
			itemRaw                                                  *int32
		)
		if err := rows.Scan(&shopID, &allowSell, &allowRepair,
			&repairPriceBP, &specialRepairPriceBP, &specialRepairNianli,
			&source, &slot, &itemRaw); err != nil {
			return nil, stockRows, fmt.Errorf("data: 读 NPC 商店行: %w", err)
		}
		shop, exists := out[shopID]
		if !exists {
			if shopID <= 0 || source == "" {
				return nil, stockRows, fmt.Errorf("data: 商店 %d 元数据无效", shopID)
			}
			if repairPriceBP <= 0 || specialRepairPriceBP <= 0 || specialRepairNianli < 0 {
				return nil, stockRows, fmt.Errorf("data: 商店 %d 修理配置无效", shopID)
			}
			shop = domain.Shop{
				ID: shopID, AllowSell: allowSell, AllowRepair: allowRepair,
				RepairPriceBP: repairPriceBP, SpecialRepairPriceBP: specialRepairPriceBP,
				SpecialRepairNianli: specialRepairNianli, Source: source,
			}
		}
		if slot != nil || itemRaw != nil {
			if slot == nil || itemRaw == nil || *slot != int32(len(shop.Items)) {
				return nil, stockRows, fmt.Errorf("data: 商店 %d 库存槽不连续", shopID)
			}
			if len(shop.Items) >= 120 {
				return nil, stockRows, fmt.Errorf("data: 商店 %d 超过客户端 120 项上限", shopID)
			}
			id := domain.ItemID(*itemRaw)
			def, ok := items[id]
			if !ok || def.Price <= 0 {
				return nil, stockRows, fmt.Errorf("data: 商店 %d 引用无效物品 %d", shopID, id)
			}
			for _, existing := range shop.Items {
				if existing == id {
					return nil, stockRows, fmt.Errorf("data: 商店 %d 重复物品 %d", shopID, id)
				}
			}
			shop.Items = append(shop.Items, id)
			stockRows++
		}
		out[shopID] = shop
	}
	if err := rows.Err(); err != nil {
		return nil, stockRows, fmt.Errorf("data: 遍历 NPC 商店: %w", err)
	}
	if len(out) == 0 {
		return nil, stockRows, fmt.Errorf("data: game_shops 是空的")
	}
	return out, stockRows, nil
}

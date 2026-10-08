package domain

// Shop 是一个 NPC 商店的服务端权威配置。库存只保存物品号与展示顺序；
// 买入价来自同一份 ItemDef，避免库存表再复制一份会漂移的价格。
type Shop struct {
	ID        int32
	Items     []ItemID
	AllowSell bool
	// RepairPriceBP 是普通/特殊修理按“物品买价 × 缺失耐久比例”计算后的
	// 万分比倍率；10000=1 倍。SpecialRepairNianli 是特殊修理每件念力消耗。
	AllowRepair          bool
	RepairPriceBP        int32
	SpecialRepairPriceBP int32
	SpecialRepairNianli  int32
	Source               string
}

// ShopTable 按客户端 SellList/ShopID 索引全部 NPC 商店。
type ShopTable map[int32]Shop

// Contains 报告商店是否出售该物品。购买时必须重新查这张权威表，不能信客户端。
func (s Shop) Contains(id ItemID) bool {
	for _, item := range s.Items {
		if item == id {
			return true
		}
	}
	return false
}

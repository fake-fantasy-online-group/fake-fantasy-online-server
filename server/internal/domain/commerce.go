package domain

import "time"

type RackCategory struct {
	ID   uint8
	Name string
}

type RackGood struct {
	Item     ItemID
	Pet      PetID // 非零时购买后直接发入宠物栏，不进入普通物品背包。
	Category uint8
	Price    int64
	Quantity int32
	Flags    uint8
	Remain   int32
	Part     string
}

type RackCatalog struct {
	Categories []RackCategory
	Goods      []RackGood
	byItem     map[ItemID]RackGood
}

func NewRackCatalog(categories []RackCategory, goods []RackGood) RackCatalog {
	c := RackCatalog{Categories: append([]RackCategory(nil), categories...), Goods: append([]RackGood(nil), goods...), byItem: map[ItemID]RackGood{}}
	for _, good := range goods {
		c.byItem[good.Item] = good
	}
	return c
}

func (c RackCatalog) Good(item ItemID) (RackGood, bool) {
	good, ok := c.byItem[item]
	return good, ok
}

func (c RackCatalog) Valid() bool {
	return len(c.Categories) > 0 && len(c.Goods) > 0 && len(c.byItem) == len(c.Goods)
}

type DepositRule struct {
	AutoCredit   bool
	CaiyuPerUnit int64
	MaxAmount    int32
}

func (r DepositRule) Valid() bool { return r.CaiyuPerUnit > 0 && r.MaxAmount > 0 }

// RackRefundRule 是神奇货架回收页的服务端规则。客户端只能展示这三个值；
// 是否真的买过、是否仍在窗口内、物品是否还在背包，都由服务端重新裁决。
type RackRefundRule struct {
	Percent   int32
	WindowDay int32
	UsedOK    bool
}

func (r RackRefundRule) Valid() bool {
	return r.Percent > 0 && r.Percent <= 100 && r.WindowDay > 0 && r.WindowDay <= 365
}

// RackRefundOffer 是购买流水聚合后的可退份额，同物品、单价和每份数量合并一行。
// RemainingShares 尚未结合在线背包；场景会再按实际可消耗数量收窄。
type RackRefundOffer struct {
	Item            ItemID
	UnitPrice       int32
	Bundle          int32
	RemainingShares int32
	LeftDays        int32
	OldestPurchase  time.Time
}

package domain

import "time"

type StallType uint8

const (
	StallSell StallType = iota
	StallBuy
)

func (t StallType) Valid() bool { return t == StallSell || t == StallBuy }

// StallOpenItems 是两类店铺成功开张时各自消耗的古币。
// 映射由 PostgreSQL 的黄金古币/白银古币记录加载，不在场景逻辑里写死物品号。
type StallOpenItems map[StallType]ItemID

func (r StallOpenItems) ItemFor(t StallType) (ItemID, bool) {
	id, ok := r[t]
	return id, ok && id != 0
}

func (r StallOpenItems) TypeFor(item ItemID) (StallType, bool) {
	for _, typ := range []StallType{StallSell, StallBuy} {
		if id, ok := r.ItemFor(typ); ok && id == item {
			return typ, true
		}
	}
	return 0, false
}

func (r StallOpenItems) Valid() bool {
	_, sell := r.ItemFor(StallSell)
	_, buy := r.ItemFor(StallBuy)
	return sell && buy
}

type StallItem struct {
	Stack     Stack
	UnitPrice int64
}

type StallSale struct {
	Time  time.Time
	Item  string
	Count int32
	Money int64
}

// Stall 是摊主的摆摊状态。出售摊的 Items 是已经从背包移出的真实实例；
// 收购摊的 Items 只是欲收购物品样本，成交时才检查并扣除摊主余额。
type Stall struct {
	Type  StallType
	Name  string
	Items []StallItem
	Sales []StallSale
}

func NewStall(typ StallType, name string) *Stall {
	if !typ.Valid() || name == "" {
		return nil
	}
	return &Stall{Type: typ, Name: name}
}

func (s *Stall) Clone() *Stall {
	if s == nil {
		return nil
	}
	return &Stall{Type: s.Type, Name: s.Name,
		Items: append([]StallItem(nil), s.Items...), Sales: append([]StallSale(nil), s.Sales...)}
}

func (s *Stall) Add(item StallItem) bool {
	if s == nil || !s.Type.Valid() || item.Stack.Empty() || item.UnitPrice <= 0 || len(s.Items) >= 1<<16-1 {
		return false
	}
	for _, old := range s.Items {
		if old.Stack.Item == item.Stack.Item {
			return false // 客户端 stallIdxByItem 以 item id 为唯一键。
		}
	}
	s.Items = append(s.Items, item)
	return true
}

func (s *Stall) Remove(index int) (StallItem, bool) {
	if s == nil || index < 0 || index >= len(s.Items) {
		return StallItem{}, false
	}
	item := s.Items[index]
	copy(s.Items[index:], s.Items[index+1:])
	s.Items[len(s.Items)-1] = StallItem{}
	s.Items = s.Items[:len(s.Items)-1]
	return item, true
}

func (s *Stall) At(index int) (StallItem, bool) {
	if s == nil || index < 0 || index >= len(s.Items) {
		return StallItem{}, false
	}
	return s.Items[index], true
}

func (s *Stall) SetCount(index int, count int32) bool {
	if s == nil || index < 0 || index >= len(s.Items) || count < 0 {
		return false
	}
	if count == 0 {
		_, _ = s.Remove(index)
		return true
	}
	s.Items[index].Stack.Count = count
	return true
}

func (s *Stall) AddSale(sale StallSale) {
	if s == nil || sale.Time.IsZero() || sale.Item == "" || sale.Count <= 0 || sale.Money <= 0 {
		return
	}
	if len(s.Sales) == 1<<16-1 {
		copy(s.Sales, s.Sales[1:])
		s.Sales = s.Sales[:len(s.Sales)-1]
	}
	s.Sales = append(s.Sales, sale)
}

// CheckedMulPositive 返回两个正整数的乘积，供价格×数量的所有边界共用。
func CheckedMulPositive(a, b int64) (int64, bool) {
	if a <= 0 || b <= 0 || a > int64(^uint64(0)>>1)/b {
		return 0, false
	}
	return a * b, true
}

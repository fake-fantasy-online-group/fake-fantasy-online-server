package domain

// WarehouseRule 是仓库行为配置。方向枚举由 PostgreSQL 配置，默认值可按部署需要调整。
type WarehouseRule struct {
	InitialPages     uint8
	SlotsPerPage     int
	MaxStack         int32
	MoveDepositDir   uint8
	MoneyDepositMode uint8
	MaxPages         uint8
	ExpandBaseCost   int64
	ExpandCostStep   int64
}

func (r WarehouseRule) Valid() bool {
	maxPages := r.MaxPages
	if maxPages == 0 { // 兼容只构造旧五字段规则的测试/工具；这种规则不允许扩页。
		maxPages = r.InitialPages
	}
	return r.InitialPages > 0 && r.SlotsPerPage > 0 && r.MaxStack > 0 &&
		r.MoveDepositDir <= 1 && r.MoneyDepositMode <= 1 && maxPages >= r.InitialPages &&
		r.ExpandBaseCost >= 0 && r.ExpandCostStep >= 0
}

func (r WarehouseRule) ExpansionCost(currentPages int) (int64, bool) {
	if !r.Valid() || currentPages < int(r.InitialPages) || currentPages >= int(r.MaxPages) {
		return 0, false
	}
	step := int64(currentPages - int(r.InitialPages))
	return r.ExpandBaseCost + step*r.ExpandCostStep, true
}

// Warehouse 是角色独立的个人仓库。页与格都是客户端可感知位置，必须原样持久化。
type Warehouse struct {
	pages    [][]Stack
	money    int64
	maxStack int32
}

func NewWarehouse(pageCount uint8, slotsPerPage int, maxStack int32) *Warehouse {
	if pageCount == 0 || slotsPerPage <= 0 || maxStack <= 0 {
		return nil
	}
	w := &Warehouse{pages: make([][]Stack, int(pageCount)), maxStack: maxStack}
	for i := range w.pages {
		w.pages[i] = make([]Stack, slotsPerPage)
	}
	return w
}

func (w *Warehouse) Clone() *Warehouse {
	if w == nil {
		return nil
	}
	out := &Warehouse{pages: make([][]Stack, len(w.pages)), money: w.money, maxStack: w.maxStack}
	for i := range w.pages {
		out.pages[i] = append([]Stack(nil), w.pages[i]...)
	}
	return out
}

func (w *Warehouse) PageCount() int {
	if w == nil {
		return 0
	}
	return len(w.pages)
}

func (w *Warehouse) SlotsPerPage() int {
	if w == nil || len(w.pages) == 0 {
		return 0
	}
	return len(w.pages[0])
}

func (w *Warehouse) MaxStack() int32 {
	if w == nil {
		return 0
	}
	return w.maxStack
}

func (w *Warehouse) Money() int64 {
	if w == nil {
		return 0
	}
	return w.money
}

func (w *Warehouse) SetMoney(v int64) bool {
	if w == nil || v < 0 {
		return false
	}
	w.money = v
	return true
}

func (w *Warehouse) At(page, slot int) Stack {
	if w == nil || page < 0 || page >= len(w.pages) || slot < 0 || slot >= len(w.pages[page]) {
		return Stack{}
	}
	return w.pages[page][slot]
}

func (w *Warehouse) Set(page, slot int, st Stack) bool {
	if w == nil || page < 0 || page >= len(w.pages) || slot < 0 || slot >= len(w.pages[page]) {
		return false
	}
	w.pages[page][slot] = st
	return true
}

func (w *Warehouse) Each(fn func(page, slot int, st Stack)) {
	if w == nil {
		return
	}
	for page := range w.pages {
		for slot, st := range w.pages[page] {
			if !st.Empty() {
				fn(page, slot, st)
			}
		}
	}
}

func (w *Warehouse) FirstEmpty(page int) int {
	if w == nil || page < 0 || page >= len(w.pages) {
		return -1
	}
	for slot, st := range w.pages[page] {
		if st.Empty() {
			return slot
		}
	}
	return -1
}

func (w *Warehouse) Expand(maxPages uint8) bool {
	if w == nil || len(w.pages) == 0 || len(w.pages) >= int(maxPages) {
		return false
	}
	w.pages = append(w.pages, make([]Stack, len(w.pages[0])))
	return true
}

// SameStackInstance 报告两个堆是否能安全合并。任何实例养成或保护状态不同都
// 必须保持为两堆，不能因模板 id 相同而吞掉 UID/耐久/词条。
func SameStackInstance(a, b Stack) bool {
	if a.Empty() || b.Empty() {
		return false
	}
	ac, bc := a, b
	ac.Count, bc.Count = 0, 0
	return ac == bc
}

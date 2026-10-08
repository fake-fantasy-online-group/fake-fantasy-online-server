package domain

// 衣柜保存已经从背包移入的魔法斗篷。客户端只给五个分类：帽子、
// 面饰、衣服、背包和武器；武器分类同时覆盖左右手外观槽。
type WardrobeCategory uint8

const (
	WardrobeCap WardrobeCategory = iota
	WardrobeFace
	WardrobeBody
	WardrobeBackpack
	WardrobeWeapon
	wardrobeCategoryCount
)

func (c WardrobeCategory) Valid() bool { return c < wardrobeCategoryCount }

// WardrobeRule 是魔法衣橱的开启与逐格解锁规则。新角色容量为 0；消耗
// OpenKeyCost 把钥匙后一次开启 OpenCapacity 格，之后按 UnlockCosts 逐格解锁。
type WardrobeRule struct {
	KeyItem      ItemID
	OpenKeyCost  int32
	OpenCapacity int32
	MaxCapacity  int32
	UnlockCosts  map[int32]int32 // key=解锁后的容量（11..200），value=钥匙数
}

func (r WardrobeRule) Valid() bool {
	if r.KeyItem == 0 || r.OpenKeyCost <= 0 || r.OpenCapacity <= 0 ||
		r.MaxCapacity < r.OpenCapacity || r.MaxCapacity > 1<<16-1 {
		return false
	}
	for capacity := r.OpenCapacity + 1; capacity <= r.MaxCapacity; capacity++ {
		if r.UnlockCosts[capacity] <= 0 {
			return false
		}
	}
	return true
}

func (r WardrobeRule) NextUnlock(capacity int32) (next, keyCost int32, ok bool) {
	if !r.Valid() {
		return 0, 0, false
	}
	if capacity == 0 {
		return r.OpenCapacity, r.OpenKeyCost, true
	}
	if capacity < r.OpenCapacity || capacity >= r.MaxCapacity {
		return 0, 0, false
	}
	next = capacity + 1
	return next, r.UnlockCosts[next], r.UnlockCosts[next] > 0
}

// WardrobeDef 是一件魔法斗篷已经闭合的展示定义。ID 是背包里的魔法斗篷，
// Appearance 来自 ov_avatararm.avatar_index 指向的装备外观资源。
type WardrobeDef struct {
	ID           ItemID
	Category     WardrobeCategory
	Appearance   EquipAppearance
	Name         string
	Description  string
	CannotAttach bool
	// Sex 沿用 ov_arm.sex：0 不限、1 男、2 女。只有静态表明确声明时才限制。
	Sex uint8
}

type WardrobeTable map[ItemID]WardrobeDef

// WardrobeEntry 是衣柜中的一个有序收藏。Worn 表示玩家选择了该分类中的这项。
type WardrobeEntry struct {
	Item     ItemID
	Category WardrobeCategory
	Worn     bool
}

type Wardrobe struct {
	capacity int32
	items    []WardrobeEntry
}

func NewWardrobe(capacity int32) *Wardrobe {
	if capacity < 0 {
		return nil
	}
	return &Wardrobe{capacity: capacity}
}

func (w *Wardrobe) SetCapacity(capacity int32) bool {
	if w == nil || capacity < 0 || capacity < int32(len(w.items)) {
		return false
	}
	w.capacity = capacity
	return true
}

func (w *Wardrobe) Clone() *Wardrobe {
	if w == nil {
		return nil
	}
	return &Wardrobe{capacity: w.capacity, items: append([]WardrobeEntry(nil), w.items...)}
}

func (w *Wardrobe) Capacity() int32 {
	if w == nil {
		return 0
	}
	return w.capacity
}

func (w *Wardrobe) Count() int {
	if w == nil {
		return 0
	}
	return len(w.items)
}

func (w *Wardrobe) Each(fn func(index int, entry WardrobeEntry)) {
	if w == nil || fn == nil {
		return
	}
	for i, entry := range w.items {
		fn(i, entry)
	}
}

func (w *Wardrobe) Contains(item ItemID) bool {
	return w.indexOf(item) >= 0
}

func (w *Wardrobe) Add(item ItemID, category WardrobeCategory) bool {
	if w == nil || item == 0 || !category.Valid() || len(w.items) >= int(w.capacity) || w.Contains(item) {
		return false
	}
	w.items = append(w.items, WardrobeEntry{Item: item, Category: category})
	return true
}

// Restore 仅供持久化加载使用；position 的合法性由 store 在调用前验证。
func (w *Wardrobe) Restore(entry WardrobeEntry) bool {
	if w == nil || entry.Item == 0 || !entry.Category.Valid() || len(w.items) >= int(w.capacity) || w.Contains(entry.Item) {
		return false
	}
	if entry.Worn {
		for _, old := range w.items {
			if old.Category == entry.Category && old.Worn {
				return false
			}
		}
	}
	w.items = append(w.items, entry)
	return true
}

// ToggleWear 复原客户端“左键穿/脱”的同一条 WardrobeWear 命令。点中当前
// 分类已经穿着的外观时脱下；点中别的外观时替换该分类的穿着项。
func (w *Wardrobe) ToggleWear(item ItemID) bool {
	i := w.indexOf(item)
	if i < 0 {
		return false
	}
	if w.items[i].Worn {
		w.items[i].Worn = false
		return true
	}
	category := w.items[i].Category
	for j := range w.items {
		if w.items[j].Category == category {
			w.items[j].Worn = j == i
		}
	}
	return true
}

func (w *Wardrobe) IsWorn(item ItemID) bool {
	i := w.indexOf(item)
	return i >= 0 && w.items[i].Worn
}

// Extract 从永久收藏中取出一件。调用方必须先在同一事务预演提取凭证和
// 背包空间；这里不接触背包，保持领域对象职责单一。
func (w *Wardrobe) Extract(item ItemID) (WardrobeEntry, bool) {
	i := w.indexOf(item)
	if i < 0 {
		return WardrobeEntry{}, false
	}
	entry := w.items[i]
	copy(w.items[i:], w.items[i+1:])
	w.items = w.items[:len(w.items)-1]
	return entry, true
}

func (w *Wardrobe) CategoryCount(item ItemID) int {
	i := w.indexOf(item)
	if i < 0 {
		return 0
	}
	category := w.items[i].Category
	count := 0
	for _, entry := range w.items {
		if entry.Category == category {
			count++
		}
	}
	return count
}

// Move 的 to 是 Win05 过滤到当前分类后得到的格号，不是全衣橱切片下标。
// 其它分类在完整快照中的位置保持不变，只重排同类条目。
func (w *Wardrobe) Move(item ItemID, to int) bool {
	from := w.indexOf(item)
	if from < 0 || to < 0 {
		return false
	}
	category := w.items[from].Category
	positions := make([]int, 0)
	fromCategory := -1
	for index, entry := range w.items {
		if entry.Category != category {
			continue
		}
		if index == from {
			fromCategory = len(positions)
		}
		positions = append(positions, index)
	}
	if fromCategory < 0 || to >= len(positions) || fromCategory == to {
		return false
	}

	entry := w.items[from]
	if fromCategory < to {
		for index := fromCategory; index < to; index++ {
			w.items[positions[index]] = w.items[positions[index+1]]
		}
	} else {
		for index := fromCategory; index > to; index-- {
			w.items[positions[index]] = w.items[positions[index-1]]
		}
	}
	w.items[positions[to]] = entry
	return true
}

func (w *Wardrobe) Active(category WardrobeCategory) (ItemID, bool) {
	if w == nil || !category.Valid() {
		return 0, false
	}
	for _, entry := range w.items {
		if entry.Category == category && entry.Worn {
			return entry.Item, true
		}
	}
	return 0, false
}

func (w *Wardrobe) indexOf(item ItemID) int {
	if w == nil || item == 0 {
		return -1
	}
	for i := range w.items {
		if w.items[i].Item == item {
			return i
		}
	}
	return -1
}

// WardrobeCategoryOfAppearance 把 0x800a 外观槽映射到客户端衣柜的五类。
func WardrobeCategoryOfAppearance(part AppearancePart) (WardrobeCategory, bool) {
	switch part {
	case AppearanceCap:
		return WardrobeCap, true
	case AppearanceFace:
		return WardrobeFace, true
	case AppearanceBody:
		return WardrobeBody, true
	case AppearanceBackpack:
		return WardrobeBackpack, true
	case AppearanceWeaponR, AppearanceWeaponL:
		return WardrobeWeapon, true
	default:
		return 0, false
	}
}

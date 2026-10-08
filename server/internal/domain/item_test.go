package domain

import "testing"

// 真实物品做样本: 树枝(4103, 树妖 31.4% 掉)、牛肉面(3002)、铁剑(装备)。
var (
	树枝  = ItemDef{ID: 4103, Name: "树枝", Stackable: true, Price: 2}
	牛肉面 = ItemDef{ID: 3002, Name: "牛肉面", Stackable: true, Price: 10}
	铁剑  = ItemDef{ID: 4001, Name: "铁剑", Stackable: false, Price: 300,
		Equip: &EquipDef{Slot: 4, LevelReq: 5, Durable: 100}}
)

func TestBagAddStacks(t *testing.T) {
	b := NewBag(3)
	if left := b.Add(树枝, 10); left != 0 {
		t.Fatalf("空背包放 10 个应全放下, 剩 %d", left)
	}
	if b.CountOf(树枝.ID) != 10 {
		t.Fatalf("应有 10 个, 实际 %d", b.CountOf(树枝.ID))
	}
	if b.FreeSlots() != 2 {
		t.Fatalf("10 个可堆叠物应只占 1 格, 实际占了 %d 格", 3-b.FreeSlots())
	}

	// 再放要先填满已有的堆, 不能一上来就开新格
	b.Add(树枝, 5)
	if b.FreeSlots() != 2 {
		t.Fatal("同种物品应先填已有的堆")
	}
	if b.At(0).Count != 15 {
		t.Fatalf("第一格应堆到 15, 实际 %d", b.At(0).Count)
	}
}

// 堆满一格才开下一格。
func TestBagOverflowsToNextSlot(t *testing.T) {
	b := NewBag(3)
	b.Add(树枝, MaxStack+5)
	if b.At(0).Count != MaxStack {
		t.Fatalf("第一格应堆满 %d, 实际 %d", MaxStack, b.At(0).Count)
	}
	if b.At(1).Count != 5 {
		t.Fatalf("溢出的 5 个应进第二格, 实际 %d", b.At(1).Count)
	}
}

// 放不下时**部分成功**并如实报告剩余 —— 捡 50 个只放得下 30 个时,
// 放下 30 比一个都不放更符合直觉。
func TestBagAddPartial(t *testing.T) {
	b := NewBag(1)
	left := b.Add(树枝, MaxStack+20)
	if left != 20 {
		t.Fatalf("1 格只放得下 %d 个, 应剩 20, 实际剩 %d", MaxStack, left)
	}
	if b.CountOf(树枝.ID) != MaxStack {
		t.Fatalf("应放下 %d 个, 实际 %d", MaxStack, b.CountOf(树枝.ID))
	}
}

// 不可堆叠的东西一件一格。
func TestBagNonStackableOnePerSlot(t *testing.T) {
	b := NewBag(3)
	if left := b.Add(铁剑, 2); left != 0 {
		t.Fatalf("3 格应放得下 2 把剑, 剩 %d", left)
	}
	if b.FreeSlots() != 1 {
		t.Fatalf("2 把剑应占 2 格, 实际占 %d 格", 3-b.FreeSlots())
	}
	if b.At(0).Count != 1 || b.At(1).Count != 1 {
		t.Error("不可堆叠物每格只能是 1 件")
	}
	// 装备要带上满耐久
	if b.At(0).Durability != 100 {
		t.Errorf("新装备应是满耐久 100, 实际 %d", b.At(0).Durability)
	}
	// 放不下的要报出来
	if left := b.Add(铁剑, 5); left != 4 {
		t.Fatalf("只剩 1 格, 应剩 4 把放不下, 实际 %d", left)
	}
}

func TestBagRemove(t *testing.T) {
	b := NewBag(5)
	b.Add(树枝, 150) // 99 + 51, 两格

	if !b.Remove(树枝.ID, 120) {
		t.Fatal("有 150 个, 扣 120 应成功")
	}
	if got := b.CountOf(树枝.ID); got != 30 {
		t.Fatalf("应剩 30, 实际 %d", got)
	}
	// 不够就一个都不扣 —— 半成功会让"扣物品→给奖励"出现只扣一半的中间态
	if b.Remove(树枝.ID, 999) {
		t.Fatal("不够时不该扣成功")
	}
	if got := b.CountOf(树枝.ID); got != 30 {
		t.Fatalf("失败的扣除不该动到东西, 实际剩 %d", got)
	}
}

// 扣空的格子要真的变空, 不能留个 count=0 的幽灵。
func TestBagRemoveClearsSlot(t *testing.T) {
	b := NewBag(3)
	b.Add(树枝, 5)
	b.RemoveAt(0, 5)
	if !b.At(0).Empty() {
		t.Fatalf("扣光的格子应变空, 实际 %+v", b.At(0))
	}
	if b.FreeSlots() != 3 {
		t.Fatal("扣光之后格子应能再用")
	}
}

func TestBagRemoveAtBounds(t *testing.T) {
	b := NewBag(2)
	b.Add(树枝, 5)
	for _, c := range []struct {
		slot int
		n    int32
		why  string
	}{
		{-1, 1, "负数格号"},
		{99, 1, "越界格号"},
		{1, 1, "空格"},
		{0, 0, "数量为 0"},
		{0, 6, "超过该格数量"},
	} {
		if b.RemoveAt(c.slot, c.n) {
			t.Errorf("%s 不该扣成功", c.why)
		}
	}
	if b.CountOf(树枝.ID) != 5 {
		t.Error("失败的扣除动到了东西")
	}
}

// 挪格子: 同种合并, 异种交换。位置是玩家能感知的状态。
func TestBagMove(t *testing.T) {
	b := NewBag(4)
	b.Add(树枝, 10)
	b.Set(2, Stack{Item: 树枝.ID, Count: 5})

	if !b.Move(2, 0) {
		t.Fatal("同种应能合并")
	}
	if b.At(0).Count != 15 || !b.At(2).Empty() {
		t.Fatalf("合并后应是 15 + 空, 实际 %d + %+v", b.At(0).Count, b.At(2))
	}

	// 异种交换
	b.Set(1, Stack{Item: 牛肉面.ID, Count: 3})
	b.Move(0, 1)
	if b.At(0).Item != 牛肉面.ID || b.At(1).Item != 树枝.ID {
		t.Fatalf("异种应交换, 实际 0=%d 1=%d", b.At(0).Item, b.At(1).Item)
	}
}

// 带实例 id 的东西(装备)不能被合并掉 —— 每件的耐久不同, 合了就分不出来了。
func TestBagMoveDoesNotMergeInstances(t *testing.T) {
	b := NewBag(3)
	b.Set(0, Stack{UID: 1, Item: 铁剑.ID, Count: 1, Durability: 100})
	b.Set(1, Stack{UID: 2, Item: 铁剑.ID, Count: 1, Durability: 30})

	b.Move(1, 0)
	if b.At(0).UID == b.At(1).UID {
		t.Fatal("两件独立实例被合并了 —— 耐久会丢")
	}
	if b.At(0).Durability == b.At(1).Durability {
		t.Fatal("耐久应各自保留")
	}
}

func TestBagMoveInvalid(t *testing.T) {
	b := NewBag(3)
	b.Add(树枝, 5)
	for _, c := range [][2]int{{0, 0}, {-1, 0}, {0, 99}, {1, 0}} {
		if b.Move(c[0], c[1]) {
			t.Errorf("Move(%d,%d) 不该成功", c[0], c[1])
		}
	}
}

func TestBagNilSafe(t *testing.T) {
	var b *Bag
	if b.Cap() != 0 || b.FreeSlots() != 0 || b.CountOf(1) != 0 {
		t.Error("nil 背包的查询应返回零值")
	}
	if b.Add(树枝, 5) != 5 {
		t.Error("nil 背包应报告一个都没放下")
	}
	if b.RemoveAt(0, 1) || b.Remove(1, 1) || b.Move(0, 1) || b.Set(0, Stack{}) {
		t.Error("nil 背包的写操作应返回 false")
	}
	b.Each(func(int, Stack) { t.Error("nil 背包不该有东西") })
}

func TestBagDefaultCapacity(t *testing.T) {
	if NewBag(0).Cap() != DefaultBagSlots || NewBag(-5).Cap() != DefaultBagSlots {
		t.Fatalf("非法格数应退到默认 %d", DefaultBagSlots)
	}
}

// 存档快照必须是**深拷贝**: 交出去之后场景还会继续改背包。
func TestSnapshotCloneIsDeep(t *testing.T) {
	ch := &Character{ID: 1, Name: "甲", Attrs: []int32{1, 2}}
	bag := NewBag(3)
	bag.Add(树枝, 5)

	cp := Snapshot{Char: ch, Bag: bag}.Clone()
	if cp.Char == ch || cp.Bag == bag {
		t.Fatal("Clone 返回了同一个对象")
	}

	bag.Add(牛肉面, 3)
	ch.Level = 99
	if cp.Bag.CountOf(牛肉面.ID) != 0 {
		t.Error("改原背包影响了快照 —— 写库的 goroutine 会读到半截状态")
	}
	if cp.Char.Level == 99 {
		t.Error("改原角色影响了快照")
	}
}

func TestSnapshotCloneNilBag(t *testing.T) {
	cp := Snapshot{Char: &Character{ID: 1}}.Clone()
	if cp.Bag != nil {
		t.Error("没有背包时克隆出来也该是 nil")
	}
}

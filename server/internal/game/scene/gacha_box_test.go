package scene

import (
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// gachaScene 建一个只装了概率礼包物品定义的场景并加入一名玩家。
func gachaScene(t *testing.T, box *domain.GachaBoxDef) (*Scene, *fakeSink) {
	t.Helper()
	s := newTestScene(t, nil)
	items := map[domain.ItemID]domain.ItemDef{
		9001: {ID: 9001, Name: "测试礼包", Stackable: true, InventoryTab: 0, InventoryTabKnown: true,
			GachaBox: box},
		9101: {ID: 9101, Name: "测试奖励甲", Stackable: true, InventoryTab: 0, InventoryTabKnown: true},
		9102: {ID: 9102, Name: "测试奖励乙", Stackable: true, InventoryTab: 0, InventoryTabKnown: true},
		9103: {ID: 9103, Name: "测试奖励丙", Stackable: true, InventoryTab: 0, InventoryTabKnown: true},
		9201: {ID: 9201, Name: "测试钥匙", Stackable: true, InventoryTab: 0, InventoryTabKnown: true},
		9998: {ID: 9998, Name: "测试填充物", Stackable: true, InventoryTab: 0, InventoryTabKnown: true},
	}
	s.items = items
	sink := join(t, s, 1, 100, "甲", 100, 100)
	sink.take() // 清空进场事件
	return s, sink
}

// putItem 往玩家背包放 n 个指定物品。
func putItem(s *Scene, id domain.ItemID, n int32) {
	def, ok := s.itemDef(id)
	if !ok {
		panic("测试物品未注册")
	}
	s.entities[1].Player.Bag.Add(def, n)
}

// openBox 从 0 号格使用测试礼包。
func openBox(t *testing.T, s *Scene, sink *fakeSink) []event.Event {
	t.Helper()
	s.exec(UseItem{ID: 1, Item: 9001, Tab: 0, BagSlot: 0})
	s.step()
	return sink.take()
}

// noticeTexts 收集本轮 ServerNotice 文本。
func noticeTexts(evs []event.Event) []string {
	var out []string
	for _, e := range evs {
		if n, ok := e.(event.ServerNotice); ok {
			out = append(out, n.Text)
		}
	}
	return out
}

func joinedNotice(evs []event.Event, want string) bool {
	for _, t := range noticeTexts(evs) {
		if strings.Contains(t, want) {
			return true
		}
	}
	return false
}

// 必中奖励：无钥匙、chance=10000、数量固定，开箱后礼包消耗、奖励到账。
func TestGachaBoxGuaranteedReward(t *testing.T) {
	s, sink := gachaScene(t, &domain.GachaBoxDef{
		Rewards: []domain.GachaBoxReward{{Item: 9101, MinQty: 1, MaxQty: 1, ChanceBP: 10000}},
	})
	putItem(s, 9001, 1)
	evs := openBox(t, s, sink)
	if got := s.entities[1].Player.Bag.CountOf(9001); got != 0 {
		t.Errorf("必中礼包应被消耗，剩余 %d", got)
	}
	if got := s.entities[1].Player.Bag.CountOf(9101); got != 1 {
		t.Errorf("必中奖励应到账 1 个，实际 %d", got)
	}
	if !joinedNotice(evs, "打开成功") {
		t.Errorf("应提示打开成功，实际 %v", noticeTexts(evs))
	}
}

// 钥匙消耗：需要 2 把钥匙，成功后礼包扣 1、钥匙扣 2、奖励到账。
func TestGachaBoxConsumesKey(t *testing.T) {
	s, sink := gachaScene(t, &domain.GachaBoxDef{
		KeyItem: 9201, KeyQuantity: 2,
		Rewards: []domain.GachaBoxReward{{Item: 9101, MinQty: 1, MaxQty: 1, ChanceBP: 10000}},
	})
	putItem(s, 9001, 1)
	putItem(s, 9201, 2)
	openBox(t, s, sink)
	bag := s.entities[1].Player.Bag
	if got := bag.CountOf(9001); got != 0 {
		t.Errorf("礼包应被消耗，剩余 %d", got)
	}
	if got := bag.CountOf(9201); got != 0 {
		t.Errorf("2 把钥匙应被消耗，剩余 %d", got)
	}
	if got := bag.CountOf(9101); got != 1 {
		t.Errorf("奖励应到账 1 个，实际 %d", got)
	}
}

// 钥匙不足：需要 2 把只有 1 把，整体拒绝，礼包与钥匙都不扣。
func TestGachaBoxKeyShortage(t *testing.T) {
	s, sink := gachaScene(t, &domain.GachaBoxDef{
		KeyItem: 9201, KeyQuantity: 2,
		Rewards: []domain.GachaBoxReward{{Item: 9101, MinQty: 1, MaxQty: 1, ChanceBP: 10000}},
	})
	putItem(s, 9001, 1)
	putItem(s, 9201, 1)
	evs := openBox(t, s, sink)
	bag := s.entities[1].Player.Bag
	if got := bag.CountOf(9001); got != 1 {
		t.Errorf("钥匙不足时礼包不应被扣，实际剩 %d", got)
	}
	if got := bag.CountOf(9201); got != 1 {
		t.Errorf("钥匙不足时钥匙不应被扣，实际剩 %d", got)
	}
	if got := bag.CountOf(9101); got != 0 {
		t.Errorf("钥匙不足时不应发奖励，实际 %d", got)
	}
	if !joinedNotice(evs, "钥匙不足") {
		t.Errorf("应提示钥匙不足，实际 %v", noticeTexts(evs))
	}
}

// 背包门槛：MinFreeSlots=5 而背包全满，开箱被前置门槛拒绝，什么都不扣。
func TestGachaBoxSlotGate(t *testing.T) {
	s, sink := gachaScene(t, &domain.GachaBoxDef{
		MinFreeSlots: 5,
		Rewards:      []domain.GachaBoxReward{{Item: 9101, MinQty: 1, MaxQty: 1, ChanceBP: 10000}},
	})
	putItem(s, 9001, 1)
	bag := s.entities[1].Player.Bag
	for i := 0; i < bag.Cap(); i++ {
		if bag.At(i).Empty() {
			bag.Set(i, domain.Stack{Item: 9998, Count: 1})
		}
	}
	evs := openBox(t, s, sink)
	if got := s.entities[1].Player.Bag.CountOf(9001); got != 1 {
		t.Errorf("门槛拒绝时礼包不应被扣，实际剩 %d", got)
	}
	if !joinedNotice(evs, "背包剩余空间不足5格") {
		t.Errorf("应提示背包空间门槛，实际 %v", noticeTexts(evs))
	}
}

// 奖励放不下：三件奖励各占一格，背包全满时扣掉礼包和钥匙只释放两格，
// 第三件放不下 → 整体回滚（礼包、钥匙都不扣）。
func TestGachaBoxRollbackOnOverflow(t *testing.T) {
	s, sink := gachaScene(t, &domain.GachaBoxDef{
		KeyItem: 9201, KeyQuantity: 1,
		Rewards: []domain.GachaBoxReward{
			{Item: 9101, MinQty: 1, MaxQty: 1, ChanceBP: 10000},
			{Item: 9102, MinQty: 1, MaxQty: 1, ChanceBP: 10000},
			{Item: 9103, MinQty: 1, MaxQty: 1, ChanceBP: 10000},
		},
	})
	putItem(s, 9001, 1)
	putItem(s, 9201, 1)
	bag := s.entities[1].Player.Bag
	for i := 0; i < bag.Cap(); i++ {
		if bag.At(i).Empty() {
			bag.Set(i, domain.Stack{Item: 9998, Count: 1})
		}
	}
	evs := openBox(t, s, sink)
	if got := s.entities[1].Player.Bag.CountOf(9001); got != 1 {
		t.Errorf("放不下时礼包不应被扣，实际剩 %d", got)
	}
	if got := s.entities[1].Player.Bag.CountOf(9201); got != 1 {
		t.Errorf("放不下时钥匙不应被扣，实际剩 %d", got)
	}
	if got := s.entities[1].Player.Bag.CountOf(9101); got != 0 {
		t.Errorf("放不下时不应发奖励，实际 %d", got)
	}
	if !joinedNotice(evs, "背包空间不足") {
		t.Errorf("应提示背包空间不足，实际 %v", noticeTexts(evs))
	}
}

// 数量区间：min=2 max=5 必中，多次开箱数量都在区间内，且区间确实生效。
func TestGachaBoxQuantityRange(t *testing.T) {
	s, sink := gachaScene(t, &domain.GachaBoxDef{
		Rewards: []domain.GachaBoxReward{{Item: 9101, MinQty: 2, MaxQty: 5, ChanceBP: 10000}},
	})
	maxGot := int32(0)
	for i := 0; i < 8; i++ {
		putItem(s, 9001, 1)
		openBox(t, s, sink)
		got := s.entities[1].Player.Bag.CountOf(9101)
		if got < 2 || got > 5 {
			t.Fatalf("第 %d 次开箱数量 %d 超出 [2,5]", i+1, got)
		}
		if got > maxGot {
			maxGot = got
		}
		s.entities[1].Player.Bag.Remove(9101, got) // 清空奖励，避免堆叠影响计数
	}
	if maxGot <= 2 {
		t.Errorf("数量区间未生效：8 次开箱数量从未超过下限 2")
	}
}

// 极低概率：chance=1（万分之一）。固定 seed 下结果确定，按实际结果验证两条路径
// 之一的逻辑都正确（中→到账+打开成功；不中→无奖励+什么也没有获得）。
func TestGachaBoxNothingGainedOrHit(t *testing.T) {
	s, sink := gachaScene(t, &domain.GachaBoxDef{
		Rewards: []domain.GachaBoxReward{{Item: 9101, MinQty: 1, MaxQty: 1, ChanceBP: 1}},
	})
	putItem(s, 9001, 1)
	evs := openBox(t, s, sink)
	got := s.entities[1].Player.Bag.CountOf(9101)
	if got > 1 {
		t.Fatalf("chance=1 最多中一件，实际 %d", got)
	}
	if s.entities[1].Player.Bag.CountOf(9001) != 0 {
		t.Errorf("开箱后礼包应消耗")
	}
	if got == 1 && !joinedNotice(evs, "打开成功") {
		t.Errorf("命中后应提示打开成功，实际 %v", noticeTexts(evs))
	}
	if got == 0 && !joinedNotice(evs, "什么也没有获得") {
		t.Errorf("未命中应提示什么也没有获得，实际 %v", noticeTexts(evs))
	}
}

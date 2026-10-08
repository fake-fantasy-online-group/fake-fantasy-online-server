package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 钢剑: game_equipment 真值 —— 6 级, 单手, 需力量14/敏捷14, 攻击 8~14。
var 场景钢剑 = domain.ItemDef{ID: 4002, Name: "钢剑", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{
	Slot: int32(domain.SlotWeapon), LevelReq: 6, Durable: 100,
	Bonus: domain.Stats{MinAtk: 8, MaxAtk: 14},
	Need:  domain.Requirement{Level: 6, Base: domain.Base{STR: 14, AGI: 14}},
	Appearance: domain.EquipAppearance{
		Part: domain.AppearanceWeaponR, Model: 1, Known: true,
		AtkVariant: 1, WeaponCType: 2, AtkDist: 75, AttackKnown: true,
	},
}}

var 场景铁甲 = domain.ItemDef{ID: 5001, Name: "铁甲", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{
	Slot: int32(domain.SlotBody), Durable: 80,
	Affixes: []domain.Affix{
		{Attr: domain.AttrVIT, Value: 10},
		{Attr: domain.AttrCrit, Value: 500}, // 万分比 = 5%
	},
}}

var 场景盾 = domain.ItemDef{ID: 5100, Name: "圆木盾", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{
	Slot: int32(domain.SlotShield), Bonus: domain.Stats{Def: 5}}}

var 场景双手剑 = domain.ItemDef{ID: 5200, Name: "双手剑", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{
	Slot: int32(domain.SlotTwoHand), Bonus: domain.Stats{MinAtk: 20, MaxAtk: 30}}}

var 场景药 = domain.ItemDef{ID: 3002, Name: "牛肉面", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}

// equipScene 建一个能穿装备的场景, 玩家已进图。
func equipScene(t *testing.T) (*Scene, *fakeSink) {
	t.Helper()
	items := map[domain.ItemID]domain.ItemDef{
		场景钢剑.ID: 场景钢剑, 场景铁甲.ID: 场景铁甲,
		场景盾.ID: 场景盾, 场景双手剑.ID: 场景双手剑, 场景药.ID: 场景药,
	}
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Items: items})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	return s, sink
}

// 让角色够格穿钢剑。
func makeEligible(s *Scene) {
	ch := s.entities[1].Player.Char
	ch.Level = 10
	ch.Base = domain.Base{STR: 20, VIT: 24, INT: 4, SPI: 4, AGI: 20, DEX: 4}
}

func putInBag(s *Scene, slot int, d domain.ItemDef) {
	s.entities[1].Player.Bag.Set(slot, domain.Stack{
		Item: d.ID, Count: 1, Durability: d.Equip.Durable})
}

// 穿上装备: 从背包消失、进到槽位、属性跟着变。
func TestEquipMovesItemAndUpdatesStats(t *testing.T) {
	s, sink := equipScene(t)
	makeEligible(s)
	p := s.entities[1]
	s.refreshStats(p) // 让基准属性反映改过的六维
	before := p.Stats.MinAtk
	s.step() // 冲掉上面手动重算排入 outbox 的基准属性事件
	sink.take()

	putInBag(s, 0, 场景钢剑)
	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()

	if !p.Player.Bag.At(0).Empty() {
		t.Fatal("穿上之后背包那一格应变空")
	}
	worn := p.Player.Worn.At(domain.SlotWeapon)
	if worn.Item != 场景钢剑.ID {
		t.Fatalf("武器槽应是钢剑, 实际 %d", worn.Item)
	}
	if worn.Durability != 100 {
		t.Errorf("耐久应跟着装备走, 实际 %d", worn.Durability)
	}
	if p.Stats.MinAtk != before+8 {
		t.Fatalf("攻击应涨 8, 从 %d 到 %d", before, p.Stats.MinAtk)
	}
	evs := sink.take()
	if len(evs) != 4 {
		t.Fatalf("穿戴应依次发属性/外观/背包/新手提示四条事件，实际 %d: %T", len(evs), evs)
	}
	if _, ok := evs[0].(event.StatsChanged); !ok {
		t.Errorf("第一条应是 0x8007 对应 StatsChanged，实际 %T", evs[0])
	}
	ap, ok := evs[1].(event.AppearanceChanged)
	if !ok {
		t.Errorf("第二条应是 0x800a 对应 AppearanceChanged，实际 %T", evs[1])
	} else if ap.Appearance.EquipView != [6]uint16{0, 0, 0, 1, 0, 0} ||
		ap.Appearance.AtkVariant != 1 || ap.Appearance.WeaponCType != 2 || ap.Appearance.AtkDist != 75 {
		t.Errorf("钢剑外观快照不对: %+v", ap.Appearance)
	}
	if _, ok := evs[2].(event.InventorySnapshot); !ok {
		t.Errorf("第三条应是 0x8006 对应 InventorySnapshot，实际 %T", evs[2])
	}
	if tip, ok := evs[3].(event.NewbieTipRequested); !ok || tip.ID != 4 {
		t.Errorf("第四条应是穿戴提示 4，实际 %#v", evs[3])
	}
}

// 模拟重启后从 char_equipment 读回穿戴：characters.equip_view 即使是旧缓存，
// 也必须在 SelfEntered(0x8003) 和公开 Look 构造前由穿戴修正。
func TestEnterRecomputesAppearanceFromPersistedWorn(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000,
		Items: map[domain.ItemID]domain.ItemDef{场景钢剑.ID: 场景钢剑}})
	worn := domain.NewEquipSet()
	worn.Set(domain.SlotWeapon, domain.Stack{Item: 场景钢剑.ID, Count: 1, Durability: 77})
	ch := &domain.Character{ID: 100, Name: "甲", Level: 10,
		Base: domain.Base{STR: 20, VIT: 20, AGI: 20},
		Pos:  domain.Pos{MapID: 7, X: 100, Y: 100},
		Appear: domain.Appearance{
			Gender: 1, EquipView: [6]uint16{999, 999, 999, 999, 999, 999},
			AtkVariant: 9, WeaponCType: 9, AtkDist: 999,
		}}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Bag: domain.NewBag(10), Worn: worn, Sink: sink})
	s.step()

	want := [6]uint16{0, 0, 0, 1, 0, 0}
	if ch.Appear.EquipView != want || ch.Appear.AtkVariant != 1 ||
		ch.Appear.WeaponCType != 2 || ch.Appear.AtkDist != 75 {
		t.Fatalf("入场前未从持久化穿戴重算: %+v", ch.Appear)
	}
	if got := s.entities[1].Look.Appearance; got != ch.Appear {
		t.Fatalf("公开 Look 仍是旧外观: %+v / %+v", got, ch.Appear)
	}
	entered, ok := firstOf[event.SelfEntered](sink.take())
	if !ok || entered.Char.Appear != ch.Appear {
		t.Fatalf("SelfEntered 未携带重算结果: %+v", entered)
	}
}

// 词条里的六维要真的推到二级属性上, 而且血上限涨了当前血要跟着补。
func TestEquipAffixRaisesMaxHP(t *testing.T) {
	s, _ := equipScene(t)
	p := s.entities[1]
	beforeMax, beforeHP := p.MaxHP, p.HP

	putInBag(s, 0, 场景铁甲)
	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()

	if d := p.MaxHP - beforeMax; d != 10*domain.HPPerVIT {
		t.Fatalf("体质+10 应涨 %d 血上限, 实际涨了 %d", 10*domain.HPPerVIT, d)
	}
	if p.HP != beforeHP+(p.MaxHP-beforeMax) {
		t.Errorf("上限涨多少当前血就该补多少, 实际 %d", p.HP)
	}
	// 暴击率这类字段只从装备来
	if p.Stats.CritRate != 500 {
		t.Fatalf("暴击率应是 500(万分比), 实际 %d —— 词条没接上", p.Stats.CritRate)
	}
}

// 脱下要把加成一起脱掉, 而且不能把人脱死。
func TestUnequipRemovesBonusWithoutKilling(t *testing.T) {
	s, _ := equipScene(t)
	p := s.entities[1]

	putInBag(s, 0, 场景铁甲)
	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()
	withArmor := p.MaxHP

	p.HP = 1 // 残血时脱掉加血装备
	s.exec(Unequip{ID: 1, Slot: domain.SlotBody})
	s.step()

	if p.MaxHP >= withArmor {
		t.Fatalf("脱下之后血上限应降回去, 实际 %d(穿着时 %d)", p.MaxHP, withArmor)
	}
	if p.HP < 1 {
		t.Fatalf("脱装备不该把人脱死(那会变成自杀手段), 实际血量 %d", p.HP)
	}
	if !p.Alive() {
		t.Fatal("脱装备把人脱死了")
	}
	if p.Stats.CritRate != 0 {
		t.Errorf("脱下之后暴击率应归零, 实际 %d", p.Stats.CritRate)
	}
	if p.Player.Bag.CountOf(场景铁甲.ID) != 1 {
		t.Error("脱下的装备应回到背包")
	}
}

// 反复穿脱一件加血装备不能变成无限回血。
func TestEquipCycleIsNotInfiniteHeal(t *testing.T) {
	s, _ := equipScene(t)
	p := s.entities[1]
	putInBag(s, 0, 场景铁甲)

	p.HP = 50
	start := p.HP
	for i := 0; i < 5; i++ {
		s.exec(Equip{ID: 1, BagSlot: 0})
		s.step()
		s.exec(Unequip{ID: 1, Slot: domain.SlotBody})
		s.step()
	}
	if p.HP != start {
		t.Fatalf("穿脱五轮之后血量应回到 %d, 实际 %d —— 这就是无限回血漏洞", start, p.HP)
	}
}

// 门槛不够要被拒, 而且装备留在背包里。
func TestEquipRejectedByRequirement(t *testing.T) {
	s, sink := equipScene(t)
	p := s.entities[1]
	p.Player.Char.Level = 1 // 钢剑要 6 级
	putInBag(s, 0, 场景钢剑)
	sink.take()

	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectLevelTooLow {
		t.Fatalf("等级不够应回 RejectLevelTooLow, 实际 %+v", rej)
	}
	if p.Player.Bag.At(0).Empty() {
		t.Fatal("没穿成的装备应还在背包里")
	}
	if p.Player.Worn.Count() != 0 {
		t.Fatal("不该穿上")
	}
}

// 六维不够也要被拒 —— 装备是属性门槛不是职业限制。
func TestEquipRejectedByStats(t *testing.T) {
	s, sink := equipScene(t)
	ch := s.entities[1].Player.Char
	ch.Level = 10
	ch.Base = domain.Base{STR: 5, AGI: 5} // 钢剑要力量14/敏捷14
	putInBag(s, 0, 场景钢剑)
	sink.take()

	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectStatTooLow {
		t.Fatalf("六维不够应回 RejectStatTooLow, 实际 %+v", rej)
	}
}

// 门槛按暗黑式的当前有效六维判定；替换目标槽位的旧装备不参与门槛。
func TestRequirementUsesEquipBonus(t *testing.T) {
	s, sink := equipScene(t)
	p := s.entities[1]
	ch := s.entities[1].Player.Char
	ch.Level = 10
	ch.Base = domain.Base{STR: 10, AGI: 20} // 力量差 4 点

	// 先穿一件加 10 力量的戒指
	ring := domain.ItemDef{ID: 7001, Name: "力戒", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{
		Slot: int32(domain.SlotRing), Base: domain.Base{STR: 10}}}
	s.items[ring.ID] = ring
	putInBag(s, 0, ring)
	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()
	sink.take()

	// 戴着戒指后有效力量达到要求，可以穿钢剑。
	putInBag(s, 1, 场景钢剑)
	s.exec(Equip{ID: 1, BagSlot: 1})
	s.step()

	if _, ok := firstOf[event.Rejected](sink.take()); ok {
		t.Fatal("身上装备提供的有效属性应参与穿戴门槛判定")
	}
	if got := p.Player.Worn.At(domain.SlotWeapon).Item; got != 场景钢剑.ID {
		t.Fatalf("钢剑应穿上，实际槽位物品 %d", got)
	}
}

// 不是装备的东西穿不上。
func TestEquipNonEquipment(t *testing.T) {
	s, sink := equipScene(t)
	s.entities[1].Player.Bag.Set(0, domain.Stack{Item: 场景药.ID, Count: 5})
	sink.take()

	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectNotEquippable {
		t.Fatalf("药不是装备, 应回 RejectNotEquippable, 实际 %+v", rej)
	}
}

// 换装备: 旧的自动退回背包。
func TestEquipSwapsOldOneBackToBag(t *testing.T) {
	s, _ := equipScene(t)
	makeEligible(s)
	p := s.entities[1]

	putInBag(s, 0, 场景钢剑)
	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()

	// 再穿一把别的单手武器
	另一把 := 场景钢剑
	另一把.ID = 4003
	另一把.Name = "幻念之剑"
	s.items[另一把.ID] = 另一把
	putInBag(s, 1, 另一把)
	s.exec(Equip{ID: 1, BagSlot: 1})
	s.step()

	if p.Player.Worn.At(domain.SlotWeapon).Item != 另一把.ID {
		t.Fatal("应换成新武器")
	}
	if p.Player.Bag.CountOf(场景钢剑.ID) != 1 {
		t.Fatal("旧武器应退回背包")
	}
}

// 双手武器与单手、盾互斥: 穿双手要把那两样都退下来。
func TestTwoHandConflictsWithWeaponAndShield(t *testing.T) {
	s, _ := equipScene(t)
	makeEligible(s)
	p := s.entities[1]

	putInBag(s, 0, 场景钢剑)
	s.exec(Equip{ID: 1, BagSlot: 0})
	putInBag(s, 1, 场景盾)
	s.exec(Equip{ID: 1, BagSlot: 1})
	s.step()
	if p.Player.Worn.Count() != 2 {
		t.Fatalf("应先穿上单手+盾, 实际穿了 %d 件", p.Player.Worn.Count())
	}

	putInBag(s, 2, 场景双手剑)
	s.exec(Equip{ID: 1, BagSlot: 2})
	s.step()

	if p.Player.Worn.At(domain.SlotTwoHand).Item != 场景双手剑.ID {
		t.Fatal("应穿上双手剑")
	}
	if !p.Player.Worn.At(domain.SlotWeapon).Empty() {
		t.Error("单手武器应被脱下")
	}
	if !p.Player.Worn.At(domain.SlotShield).Empty() {
		t.Error("盾应被脱下")
	}
	if p.Player.Bag.CountOf(场景钢剑.ID) != 1 || p.Player.Bag.CountOf(场景盾.ID) != 1 {
		t.Error("脱下的两件都应回到背包")
	}
}

// 背包放不下退下来的装备时, 整件事作废 —— 不能让装备卡在"既不在身上也不在包里"。
func TestEquipAbortsWhenBagCannotHoldOldGear(t *testing.T) {
	s, sink := equipScene(t)
	makeEligible(s)
	p := s.entities[1]

	// 先穿上单手 + 盾
	putInBag(s, 0, 场景钢剑)
	s.exec(Equip{ID: 1, BagSlot: 0})
	putInBag(s, 1, 场景盾)
	s.exec(Equip{ID: 1, BagSlot: 1})
	s.step()

	// 背包缩到只有 1 格, 里面放双手剑: 换上双手要退 2 件, 只腾得出 1 格
	small := domain.NewBag(1)
	small.Set(0, domain.Stack{Item: 场景双手剑.ID, Count: 1})
	p.Player.Bag = small
	sink.take()

	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectBagFull {
		t.Fatalf("放不下应回 RejectBagFull, 实际 %+v", rej)
	}
	if p.Player.Worn.At(domain.SlotWeapon).Empty() || p.Player.Worn.At(domain.SlotShield).Empty() {
		t.Fatal("失败的换装不该把已穿的脱掉")
	}
	if p.Player.Bag.At(0).Item != 场景双手剑.ID {
		t.Fatal("双手剑应还在背包里")
	}
}

// 背包满时脱不下来。
func TestUnequipRejectedWhenBagFull(t *testing.T) {
	s, sink := equipScene(t)
	makeEligible(s)
	p := s.entities[1]
	putInBag(s, 0, 场景钢剑)
	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()

	full := domain.NewBag(1)
	full.Set(0, domain.Stack{Item: 场景药.ID, Count: 1})
	p.Player.Bag = full
	sink.take()

	s.exec(Unequip{ID: 1, Slot: domain.SlotWeapon})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectBagFull {
		t.Fatalf("背包满了应回 RejectBagFull, 实际 %+v", rej)
	}
	if p.Player.Worn.At(domain.SlotWeapon).Empty() {
		t.Fatal("没脱成的装备应还穿在身上")
	}
}

// 装备穿在身上时跨图也要跟着走。
func TestWornFollowsTeleport(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	dst, _ := r.Ensure(domain.SceneID{MapID: 14})

	worn := domain.NewEquipSet()
	worn.Set(domain.SlotWeapon, domain.Stack{Item: 场景钢剑.ID, Count: 1, Durability: 77})
	src.Post(Enter{ID: 1, Worn: worn, Bag: domain.NewBag(10), Sink: &fakeSink{},
		Char: &domain.Character{ID: 100, Name: "甲", Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}})
	inspect(t, src, rig, func(*Scene) {})

	src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 14},
		At: domain.Pos{MapID: 14, X: 500, Y: 500}})
	inspect(t, src, rig, func(*Scene) {})
	inspect(t, dst, rig, func(s *Scene) {
		e := s.EntityAt(1)
		if e == nil || e.Player.Worn == nil {
			t.Fatal("人到了但装备没跟过来")
		}
		got := e.Player.Worn.At(domain.SlotWeapon)
		if got.Item != 场景钢剑.ID || got.Durability != 77 {
			t.Fatalf("武器与耐久都该跟着走, 实际 %+v", got)
		}
	})
}

// 换装备要标脏 —— 换了不落盘, 重启就变回去了。
func TestEquipMarksDirty(t *testing.T) {
	s, _ := equipScene(t)
	makeEligible(s)
	p := s.entities[1]
	putInBag(s, 0, 场景钢剑)
	p.Player.TakeDirty()

	s.exec(Equip{ID: 1, BagSlot: 0})
	s.step()
	if !p.Player.Dirty() {
		t.Fatal("穿装备没标脏")
	}

	p.Player.TakeDirty()
	s.exec(Unequip{ID: 1, Slot: domain.SlotWeapon})
	s.step()
	if !p.Player.Dirty() {
		t.Fatal("脱装备没标脏")
	}
}

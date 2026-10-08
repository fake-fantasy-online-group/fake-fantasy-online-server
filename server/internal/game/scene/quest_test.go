package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 取自 game_tasks 真值(SQL 核对过):
//
//	蜗壳笛(24)     小叶发布   1~10级  交蜗牛壳(4161)×20
//	糊涂的妈妈(22) 纪宓发布   1~10级  纯对话
var (
	蜗牛壳 = domain.ItemDef{ID: 4161, Name: "蜗牛壳", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
	铁护头 = domain.ItemDef{ID: 2001, Name: "铁护头", InventoryTab: 2, InventoryTabKnown: true, Stackable: false,
		Equip: &domain.EquipDef{Slot: int32(domain.SlotHead), Durable: 50}}

	交物任务 = domain.QuestDef{
		ID: 24, Name: "蜗壳笛", NPC: "小叶", Map: "长乐村",
		LevelMin: 1, LevelMax: 10, GoalItem: 蜗牛壳.ID, GoalQty: 20,
		Reward: domain.QuestReward{
			Exp: 80, Honor: 1, Money: domain.Money{Silver: 8, Copper: 50},
			Items: []domain.RewardItem{{Item: 铁护头.ID, Qty: 1}},
		},
	}
	对话任务 = domain.QuestDef{
		ID: 22, Name: "糊涂的妈妈", NPC: "纪宓", Map: "长乐村",
		LevelMin: 1, LevelMax: 10,
		Reward: domain.QuestReward{Exp: 27, Honor: 1, Money: domain.Money{Copper: 500}},
	}
	高级任务 = domain.QuestDef{
		ID: 900, Name: "高级委托", NPC: "小叶", LevelMin: 30, LevelMax: 40,
		Reward: domain.QuestReward{Exp: 5000},
	}
	重复任务 = domain.QuestDef{
		ID: 901, Name: "每日巡逻", NPC: "小叶", LevelMin: 1, LevelMax: 60,
		Repeatable: true, Reward: domain.QuestReward{Exp: 100},
	}
)

func questTable(defs ...domain.QuestDef) domain.QuestTable {
	t := domain.QuestTable{}
	for _, d := range defs {
		t[d.ID] = d
	}
	return t
}

// questScene 建一个有 NPC「小叶」站在 (120,100) 的场景, 玩家在 (100,100)。
func questScene(t *testing.T) (*Scene, *fakeSink) {
	t.Helper()
	s := New(Config{
		ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Seed: 31,
		Alloc:  domain.NewEntityAlloc(),
		Quests: questTable(交物任务, 对话任务, 高级任务, 重复任务),
		Items: map[domain.ItemID]domain.ItemDef{
			蜗牛壳.ID: 蜗牛壳, 铁护头.ID: 铁护头},
		Levels: rewardLevels(),
		NPCs: []domain.NPCSpawn{
			{Name: "小叶", Sprite: 601, Pos: domain.Pos{MapID: 7, X: 120, Y: 100}},
			{Name: "纪宓", Sprite: 602, Pos: domain.Pos{MapID: 7, X: 3000, Y: 3000}}, // 远处
		},
	})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	return s, sink
}

// NPC 是实体。要它是实体的理由只有一个: 接任务得判"你站在那个 NPC 面前"。
func TestNPCsAreEntities(t *testing.T) {
	s, _ := questScene(t)

	var npcs int
	for _, e := range s.entities {
		if e.Kind != domain.KindNPC {
			continue
		}
		npcs++
		if e.ID.SegKind() != domain.KindNPC {
			t.Errorf("NPC 的 id %d 不在 NPC 段里", e.ID)
		}
		if _, ok := s.aoi.CellOf(e.ID); !ok {
			t.Errorf("NPC %s 不在 AOI 里, 玩家看不见也靠不近", e.Name)
		}
		// NPC 不参与生死与战斗
		if e.Monster != nil || e.Player != nil {
			t.Errorf("NPC %s 不该带怪物/玩家的专有部分", e.Name)
		}
	}
	if npcs != 2 {
		t.Fatalf("应有 2 个 NPC 实体, 实际 %d", npcs)
	}
}

// 玩家进图应看到附近的 NPC 出场。
func TestPlayerSeesNPC(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000,
		Alloc: domain.NewEntityAlloc(),
		NPCs:  []domain.NPCSpawn{{Name: "小叶", Sprite: 601, Pos: domain.Pos{MapID: 7, X: 120, Y: 100}}}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()

	sp, ok := firstOf[event.EntitySpawned](sink.take())
	if !ok {
		t.Fatal("玩家应看到 NPC 出场")
	}
	if sp.Kind != domain.KindNPC || sp.Name != "小叶" {
		t.Fatalf("出场的应是 NPC 小叶, 实际 %+v", sp)
	}
}

// 接任务: 当前地图里有对应 NPC 就接得到。
func TestAcceptQuest(t *testing.T) {
	s, sink := questScene(t)
	ch := s.entities[1].Player.Char

	s.exec(AcceptQuest{ID: 1, Quest: 交物任务.ID, NPC: "小叶"})
	s.step()

	ev, ok := firstOf[event.QuestAccepted](sink.take())
	if !ok {
		t.Fatal("应接得上任务")
	}
	if ev.Quest != int32(交物任务.ID) || ev.Name != "蜗壳笛" {
		t.Errorf("事件内容不对: %+v", ev)
	}
	if !ch.Quests.Active(交物任务.ID) {
		t.Fatal("任务本里应记上")
	}
}

// 不校验距离：只要发布任务的 NPC 在当前地图里，隔得再远也能接。
func TestAcceptQuestDoesNotCheckDistance(t *testing.T) {
	s, sink := questScene(t)

	// 纪宓在 (3000,3000), 玩家在 (100,100)
	s.exec(AcceptQuest{ID: 1, Quest: 对话任务.ID, NPC: "纪宓"})
	s.step()

	ev, ok := firstOf[event.QuestAccepted](sink.take())
	if !ok || ev.Quest != int32(对话任务.ID) {
		t.Fatalf("不应按距离拒绝当前地图 NPC 的任务, 实际 %+v", ev)
	}
	if !s.entities[1].Player.Char.Quests.Active(对话任务.ID) {
		t.Fatal("任务应记进任务本")
	}
}

// 站在小叶面前接不了纪宓的任务 —— 光靠得近不够, NPC 与任务还要对得上。
func TestAcceptQuestFromWrongNPC(t *testing.T) {
	s, sink := questScene(t)

	// 玩家就在小叶旁边, 但报的是纪宓发布的任务
	s.exec(AcceptQuest{ID: 1, Quest: 对话任务.ID, NPC: "小叶"})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectNoTarget {
		t.Fatalf("NPC 与任务对不上应被拒, 实际 %+v", rej)
	}
	if s.entities[1].Player.Char.Quests.Active(对话任务.ID) {
		t.Fatal("没接成不该记进任务本")
	}
}

// 等级门槛只判下限；LevelMax 是客户端展示用的推荐等级上界。
func TestAcceptQuestLevelMinimum(t *testing.T) {
	s, sink := questScene(t)
	ch := s.entities[1].Player.Char

	ch.Level = 1
	s.exec(AcceptQuest{ID: 1, Quest: 高级任务.ID, NPC: "小叶"}) // 要 30 级
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectLevelTooLow {
		t.Fatalf("等级不够应回 RejectLevelTooLow, 实际 %+v", rej)
	}

	ch.Level = 50
	s.exec(AcceptQuest{ID: 1, Quest: 交物任务.ID, NPC: "小叶"}) // 上限 10 级
	s.step()
	accepted, ok := firstOf[event.QuestAccepted](sink.take())
	if !ok || accepted.Quest != int32(交物任务.ID) {
		t.Fatalf("等级超过推荐上界仍应能接任务, 实际 %+v", accepted)
	}
}

// 同一个任务不能接两次。
func TestAcceptQuestTwice(t *testing.T) {
	s, sink := questScene(t)
	s.exec(AcceptQuest{ID: 1, Quest: 对话任务.ID, NPC: "纪宓"})
	// 把玩家挪到纪宓旁边
	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 2980, Y: 3000}})
	s.step()
	s.exec(AcceptQuest{ID: 1, Quest: 对话任务.ID, NPC: "纪宓"})
	s.step()
	sink.take()

	s.exec(AcceptQuest{ID: 1, Quest: 对话任务.ID, NPC: "纪宓"})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectQuestActive {
		t.Fatalf("重复接应回 RejectQuestActive, 实际 %+v", rej)
	}
}

// 交纯对话任务: 接了就能交, 奖励到手。
func TestCompleteTalkQuest(t *testing.T) {
	s, sink := questScene(t)
	p := s.entities[1]
	ch := p.Player.Char
	ch.Level = 3 // 3 级升 4 级要 270 经验, 27 点不会触发升级
	ch.Quests = domain.QuestLog{}
	ch.Quests.Accept(对话任务)
	// 玩家要站在纪宓旁边
	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 2980, Y: 3000}})
	s.step()
	sink.take()

	s.exec(CompleteQuest{ID: 1, Quest: 对话任务.ID})
	s.step()

	evs := sink.take()
	if _, ok := firstOf[event.QuestCompleted](evs); !ok {
		t.Fatal("应能交任务")
	}
	if !ch.Quests.Done(对话任务.ID) {
		t.Fatal("任务本里应标成已完成")
	}
	if ch.Exp != 27 {
		t.Errorf("应给 27 经验, 实际 %d", ch.Exp)
	}
	if ch.Honor != 1 {
		t.Errorf("应给 1 名誉, 实际 %d", ch.Honor)
	}
	if ch.Money.Copper != 500 {
		t.Errorf("应给 500 铜, 实际 %+v", ch.Money)
	}
}

// 交物任务: 东西不够交不了。
func TestCompleteItemQuestNotEnough(t *testing.T) {
	s, sink := questScene(t)
	p := s.entities[1]
	p.Player.Char.Quests = domain.QuestLog{}
	p.Player.Char.Quests.Accept(交物任务)
	p.Player.Bag.Add(蜗牛壳, 5) // 要 20 个
	sink.take()

	s.exec(CompleteQuest{ID: 1, Quest: 交物任务.ID})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectQuestNotDone {
		t.Fatalf("东西不够应回 RejectQuestNotDone, 实际 %+v", rej)
	}
	if p.Player.Bag.CountOf(蜗牛壳.ID) != 5 {
		t.Error("没交成不该扣东西")
	}
}

// 交物任务: 够了就扣东西发奖励。
func TestCompleteItemQuest(t *testing.T) {
	s, sink := questScene(t)
	p := s.entities[1]
	ch := p.Player.Char
	ch.Level = 3
	ch.Quests = domain.QuestLog{}
	ch.Quests.Accept(交物任务)
	p.Player.Bag.Add(蜗牛壳, 25)
	sink.take()

	s.exec(CompleteQuest{ID: 1, Quest: 交物任务.ID})
	s.step()

	if _, ok := firstOf[event.QuestCompleted](sink.take()); !ok {
		t.Fatal("应能交任务")
	}
	if got := p.Player.Bag.CountOf(蜗牛壳.ID); got != 5 {
		t.Fatalf("应扣掉 20 个蜗牛壳, 剩 5, 实际 %d", got)
	}
	if p.Player.Bag.CountOf(铁护头.ID) != 1 {
		t.Fatal("奖励物品应进背包")
	}
	if ch.Exp != 80 {
		t.Errorf("应给 80 经验, 实际 %d", ch.Exp)
	}
	if ch.Money.Silver != 8 || ch.Money.Copper != 50 {
		t.Errorf("钱不对: %+v", ch.Money)
	}
}

// **背包塞不下奖励时, 不能先把任务物品扣掉。**
// 反过来的话玩家白丢东西, 任务还没完成。
func TestCompleteQuestAbortsIfBagFull(t *testing.T) {
	s, sink := questScene(t)
	p := s.entities[1]
	p.Player.Char.Quests = domain.QuestLog{}
	p.Player.Char.Quests.Accept(交物任务)

	// 1 格背包, 装着 25 个蜗牛壳。交掉 20 个之后那一格还有 5 个, 腾不出位置放铁护头
	small := domain.NewBag(1)
	small.Add(蜗牛壳, 25)
	p.Player.Bag = small
	sink.take()

	s.exec(CompleteQuest{ID: 1, Quest: 交物任务.ID})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectBagFull {
		t.Fatalf("装不下奖励应回 RejectBagFull, 实际 %+v", rej)
	}
	if p.Player.Bag.CountOf(蜗牛壳.ID) != 25 {
		t.Fatal("失败的交付不该扣任务物品 —— 那会让玩家白丢东西")
	}
	if p.Player.Char.Quests.Done(交物任务.ID) {
		t.Fatal("没交成不该标完成")
	}
}

// 没接的任务交不了。
func TestCompleteQuestNotAccepted(t *testing.T) {
	s, sink := questScene(t)
	s.exec(CompleteQuest{ID: 1, Quest: 交物任务.ID})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectQuestNotActive {
		t.Fatalf("没接的任务应回 RejectQuestNotActive, 实际 %+v", rej)
	}
}

// 做过的非重复任务不能再接; 可重复的可以。
func TestRepeatableQuest(t *testing.T) {
	s, sink := questScene(t)
	ch := s.entities[1].Player.Char
	ch.Quests = domain.QuestLog{}

	// 非重复的做完再接 -> 拒
	ch.Quests.Accept(交物任务)
	ch.Quests.Finish(交物任务.ID)
	s.exec(AcceptQuest{ID: 1, Quest: 交物任务.ID, NPC: "小叶"})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectQuestDone {
		t.Fatalf("做过的非重复任务应回 RejectQuestDone, 实际 %+v", rej)
	}

	// 可重复的做完能再接
	ch.Quests.Accept(重复任务)
	ch.Quests.Finish(重复任务.ID)
	s.exec(AcceptQuest{ID: 1, Quest: 重复任务.ID, NPC: "小叶"})
	s.step()
	if _, ok := firstOf[event.QuestAccepted](sink.take()); !ok {
		t.Fatal("可重复任务应能再接")
	}
}

// 交任务要标脏 —— 不落盘重启就白做了。
func TestQuestMarksDirty(t *testing.T) {
	s, _ := questScene(t)
	p := s.entities[1]
	p.Player.Char.Quests = domain.QuestLog{}
	p.Player.TakeDirty()

	s.exec(AcceptQuest{ID: 1, Quest: 交物任务.ID, NPC: "小叶"})
	s.step()
	if !p.Player.Dirty() {
		t.Fatal("接任务没标脏")
	}
}

// 没配任务表的场景不该崩。
func TestNoQuestTableIsSafe(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000})
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	s.exec(AcceptQuest{ID: 1, Quest: 24, NPC: "小叶"})
	s.exec(CompleteQuest{ID: 1, Quest: 24})
	s.step() // 不崩就算过
}

// 放弃任务 = **当没接过**, 不是"做完了"。
// 标成完成的话, 一个不可重复任务就白少一次机会, NPC 头顶还会显示成做过了。
func TestAbandonQuestPutsItBack(t *testing.T) {
	s, sink := questScene(t)
	ch := s.entities[1].Player.Char

	s.exec(AcceptQuest{ID: 1, Quest: 交物任务.ID, NPC: "小叶"})
	s.step()
	sink.take()

	s.exec(AbandonQuest{ID: 1, Quest: 交物任务.ID})
	s.step()

	if ch.Quests.Active(交物任务.ID) {
		t.Fatal("放弃之后任务本里还挂着")
	}
	if ch.Quests.Done(交物任务.ID) {
		t.Fatal("放弃被记成了已完成 —— 那是白吃掉一次不可重复任务的机会")
	}
	// 客户端那边是整份覆盖, 必须重发任务本, 否则界面上还挂着
	log, ok := firstOf[event.ActiveQuests](sink.take())
	if !ok {
		t.Fatal("放弃之后没重发任务本")
	}
	for _, q := range log.List {
		if q.ID == 交物任务.ID {
			t.Fatal("重发的任务本里还有这条")
		}
	}
	// 放弃了就该能重新接
	s.exec(AcceptQuest{ID: 1, Quest: 交物任务.ID, NPC: "小叶"})
	s.step()
	if !ch.Quests.Active(交物任务.ID) {
		t.Fatal("放弃之后重新接不上了")
	}
}

// 没接过 / 已经做完的, 放弃要被拒 —— 尤其是做完的:
// 删掉那条记录等于把不可重复任务的奖励变成能反复领。
func TestAbandonRejectsUnknownAndFinished(t *testing.T) {
	s, sink := questScene(t)
	ch := s.entities[1].Player.Char

	s.exec(AbandonQuest{ID: 1, Quest: 交物任务.ID}) // 根本没接过
	s.step()
	if !hasEvent[event.Rejected](sink.take()) {
		t.Fatal("放弃一个没接过的任务该有拒绝回执")
	}

	ch.Quests = domain.QuestLog{}
	ch.Quests.Accept(交物任务)
	ch.Quests.Finish(交物任务.ID)
	s.exec(AbandonQuest{ID: 1, Quest: 交物任务.ID})
	s.step()
	if !hasEvent[event.Rejected](sink.take()) {
		t.Fatal("放弃一个已完成的任务该被拒")
	}
	if !ch.Quests.Done(交物任务.ID) {
		t.Fatal("已完成记录被放弃删掉了 —— 不可重复任务能反复领奖励了")
	}
}

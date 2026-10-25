package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
)

func wingSceneFixture(t *testing.T) (*Scene, *fakeSink, *cubeTestBackend) {
	t.Helper()
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	c := domain.WingProgressionConfig{Version: 1, Quest: 1900030002, Starter: 20609, Entrance: domain.EndgameRiftEntranceView{MapID: 7, NPCID: 96, NPC: "幻翼域使者", Script: "N2619LC"}}
	for _, w := range []domain.WingItemRule{{Item: 20609, Tier: 1, Series: "light"}, {Item: 20611, Tier: 2, Series: "light"}, {Item: 22863, Tier: 1, Series: "dark"}, {Item: 22864, Tier: 2, Series: "dark"}} {
		c.Items = append(c.Items, w)
		s.items[w.Item] = domain.ItemDef{ID: w.Item, Name: "光之翼", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotBag), Durable: 100, RefineLimit: 20}}
	}
	s.items[25029] = domain.ItemDef{ID: 25029, Name: "神奇的羽毛", Stackable: true, InventoryTab: 0, InventoryTabKnown: true}
	var err error
	s.wingRules, err = domain.NewWingProgressionRules(c, s.items)
	if err != nil {
		t.Fatal(err)
	}
	s.quests[c.Quest] = domain.QuestDef{ID: c.Quest, Name: "幻翼初展", NPC: "巧匠左慈", Prerequisites: []domain.QuestID{portableCubeTestQuest}, Steps: []domain.QuestStep{{Kind: domain.QuestStepTalk, To: "幻翼域使者"}}, Reward: domain.QuestReward{Items: []domain.RewardItem{{Item: 20609, Qty: 1}}}}
	p.Player.Char.Quests.Finish(c.Quest)
	p.Pos.MapID = 7
	s.entities[79] = &entity.Entity{ID: 79, Kind: domain.KindNPC, Name: "幻翼域使者", Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: 96, Script: "N2619LC"}}
	p.Player.QuestNPC = "幻翼域使者"
	return s, sink, b
}
func TestWingGuideAtomicallyGrantsOnce(t *testing.T) {
	s, sink, b := wingSceneFixture(t)
	p := s.players[1]
	q := s.quests[1900030002]
	delete(p.Player.Char.Quests, q.ID)
	p.Player.Char.Quests.Accept(q)
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	if b.commits != 1 || p.Player.Char.Quests.Done(q.ID) || p.Player.Bag.CountOf(20609) != 0 {
		t.Fatal("starter published early")
	}
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	if b.commits != 1 {
		t.Fatal("duplicate click")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if !p.Player.Char.Quests.Done(q.ID) || p.Player.Bag.CountOf(20609) != 1 {
		t.Fatal("starter not durable")
	}
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.Item == 20609 && (!st.Bound || st.UID <= 0) {
			t.Fatal("starter not bound instance")
		}
	})
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	if b.commits != 1 {
		t.Fatal("claimed twice")
	}
}
func TestWingGuideFullBagKeepsQuest(t *testing.T) {
	s, _, b := wingSceneFixture(t)
	p := s.players[1]
	q := s.quests[1900030002]
	delete(p.Player.Char.Quests, q.ID)
	p.Player.Char.Quests.Accept(q)
	for i := 200; i < 300; i++ {
		p.Player.Bag.Set(i, domain.Stack{Item: 1, Count: 1})
	}
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	if b.commits != 0 || p.Player.Char.Quests.Done(q.ID) {
		t.Fatal("full bag lost starter")
	}
}

func TestWingGuideStorageFailureAndRetry(t *testing.T) {
	s, sink, b := wingSceneFixture(t)
	p := s.players[1]
	q := s.quests[domain.WingGuideQuestID]
	delete(p.Player.Char.Quests, q.ID)
	p.Player.Char.Quests.Accept(q)
	s.onCompleteQuest(CompleteQuest{ID: p.ID, Quest: q.ID})
	if b.commits != 1 {
		t.Fatal("missing transaction")
	}
	b.result <- domain.EndgameCommitResult{Err: fmt.Errorf("storage failure")}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if p.Player.Char.Quests.Done(q.ID) || p.Player.Bag.CountOf(20609) != 0 {
		t.Fatal("failure published gift")
	}
	s.onCompleteQuest(CompleteQuest{ID: p.ID, Quest: q.ID})
	if b.commits != 2 {
		t.Fatal("gift not retryable")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if !p.Player.Char.Quests.Done(q.ID) || p.Player.Bag.CountOf(20609) != 1 {
		t.Fatal("gift/quest diverged")
	}
}
func TestWingGuideRejectsWrongNPC(t *testing.T) {
	for _, mutate := range []func(*Scene){func(s *Scene) { s.entities[79].NPC.Script = "wrong" }, func(s *Scene) { s.entities[79].NPC.ConfigID = 73 }, func(s *Scene) { s.players[1].Pos.X += 481 }, func(s *Scene) { s.id.Instance = 1 }} {
		s, _, b := wingSceneFixture(t)
		p := s.players[1]
		q := s.quests[domain.WingGuideQuestID]
		delete(p.Player.Char.Quests, q.ID)
		p.Player.Char.Quests.Accept(q)
		mutate(s)
		s.onCompleteQuest(CompleteQuest{ID: p.ID, Quest: q.ID})
		if b.commits != 0 || p.Player.Char.Quests.Done(q.ID) {
			t.Fatal("forged NPC granted gift")
		}
	}
}

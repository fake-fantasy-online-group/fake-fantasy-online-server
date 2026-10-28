package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func configurePowerCodexFixture(s *Scene, p *entity.Entity) {
	s.items[domain.PowerCodexItemID] = domain.ItemDef{ID: domain.PowerCodexItemID, Name: "万象秘典", InventoryTabKnown: true, UseAction: "power_codex"}
	s.quests[domain.PowerCodexQuestID] = domain.QuestDef{ID: domain.PowerCodexQuestID, Name: "万象留痕", NPC: "巧匠左慈", RecoveryItem: domain.PowerCodexItemID, Steps: []domain.QuestStep{{Kind: domain.QuestStepTalk, To: "巧匠左慈"}}, Reward: domain.QuestReward{Items: []domain.RewardItem{{Item: domain.PowerCodexItemID, Qty: 1}}}}
	if p.Player.Char.Quests == nil {
		p.Player.Char.Quests = domain.QuestLog{}
	}
}
func TestPowerCodexQuestNeedsRecordAndAcceptsOnlyOnce(t *testing.T) {
	s, _, _ := cubeSceneFixture(t)
	p := s.players[1]
	configurePowerCodexFixture(s, p)
	if s.acceptPowerCodexQuest(p.Player.Char, "{}") {
		t.Fatal("accepted without salvage record")
	}
	payload, err := domain.WriteLegendaryPowerBook("{}", domain.LegendaryPowerBook{"TY-A01": 9000})
	if err != nil {
		t.Fatal(err)
	}
	before := s.snapshotOf(p).Clone()
	if !s.acceptPowerCodexQuest(before.Char, payload) {
		t.Fatal("candidate not accepted")
	}
	if p.Player.Char.Quests.Active(domain.PowerCodexQuestID) {
		t.Fatal("candidate leaked live quest")
	}
	if !s.acceptPowerCodexQuest(p.Player.Char, payload) || s.acceptPowerCodexQuest(p.Player.Char, payload) {
		t.Fatal("must accept exactly once")
	}
	p.Player.Char.Quests.Finish(domain.PowerCodexQuestID)
	if s.acceptPowerCodexQuest(p.Player.Char, payload) {
		t.Fatal("completed quest repeated")
	}
}
func TestPowerCodexQuestUnavailableUntilAutoAccepted(t *testing.T) {
	s, _, _ := cubeSceneFixture(t)
	p := s.players[1]
	configurePowerCodexFixture(s, p)
	offers := []domain.Offer{{ID: domain.PowerCodexQuestID}, {ID: 123}}
	if got := s.filterPowerCodexOffers(p, offers); len(got) != 1 || got[0].ID != 123 {
		t.Fatal(got)
	}
	q, _, ok := s.powerCodexQuest()
	if !ok {
		t.Fatal("valid config")
	}
	p.Player.Char.Quests.Accept(q)
	if got := s.filterPowerCodexOffers(p, []domain.Offer{{ID: q.ID}}); len(got) != 1 {
		t.Fatal(got)
	}
}

func TestPowerCodexQuestGrantAtomicAndFullBag(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	configurePowerCodexFixture(s, p)
	s.id.MapID = 7
	prepareCubeArtisan(s)
	q := s.quests[domain.PowerCodexQuestID]
	p.Player.Char.Quests.Accept(q)
	bag := p.Player.Bag
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Set(0, domain.Stack{Item: 1001, Count: 1})
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	cubeTake(s, sink)
	if p.Player.Char.Quests.Done(q.ID) || b.commits != 0 {
		t.Fatal("full bag completed quest")
	}
	p.Player.Bag = bag
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	if b.commits != 1 || p.Player.Char.Quests.Done(q.ID) || p.Player.Bag.CountOf(domain.PowerCodexItemID) != 0 {
		t.Fatal("award before durable commit")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if !p.Player.Char.Quests.Done(q.ID) || !s.hasPowerCodex(p) {
		t.Fatal("book and quest did not commit together")
	}
	cubeTake(s, sink)
	s.sendEndgameSnapshot(p)
	found := false
	for _, ev := range cubeTake(s, sink) {
		if msg, ok := ev.(event.EndgameMessage); ok {
			for _, item := range msg.Message.Items {
				if item.ItemID == int32(domain.PowerCodexItemID) {
					uid, valid := powerUID(item.UID)
					found = valid && uid > 0 && item.Bound
				}
			}
		}
	}
	if !found {
		t.Fatal("awarded codex missing from UID-based client snapshot")
	}

}

func TestPowerCodexRecoveryRejectsWarehouseDuplicate(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	configurePowerCodexFixture(s, p)
	s.id.MapID = 7
	prepareCubeArtisan(s)
	p.Player.Char.Quests.Finish(domain.PowerCodexQuestID)
	p.Player.Warehouse = domain.NewWarehouse(1, 10, 99)
	p.Player.Warehouse.Set(0, 0, domain.Stack{Item: domain.PowerCodexItemID, Count: 1, Bound: true})
	s.recoverPowerCodex(p, domain.EndgameRequest{Op: "powerbook.recover", RequestID: "codex-recovery-test", ExpectedRevision: "0", NPCEntityID: 78})
	cubeTake(s, sink)
	if b.commits != 0 {
		t.Fatal("warehouse duplicate issued")
	}
}

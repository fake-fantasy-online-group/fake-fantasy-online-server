package scene

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const portableCubeTestItem domain.ItemID = 90121
const portableCubeTestQuest domain.QuestID = 90122

func configureCubeToolFixture(s *Scene, p *entity.Entity) {
	s.items[portableCubeTestItem] = domain.ItemDef{ID: portableCubeTestItem, Name: "测试随身鼎", InventoryTab: 0, InventoryTabKnown: true, UseAction: "cube"}
	s.quests = domain.QuestTable{portableCubeTestQuest: {ID: portableCubeTestQuest, Name: "测试问鼎", NPC: "试炼小仙", RequireTrialCompletion: true, RecoveryItem: portableCubeTestItem, RecoveryLore: "旧器入鼎。", Steps: []domain.QuestStep{{Kind: domain.QuestStepRiftComplete, To: "巧匠左慈"}}, Reward: domain.QuestReward{Items: []domain.RewardItem{{Item: portableCubeTestItem, Qty: 1}}}}}
	if p.Player.Char.Quests == nil {
		p.Player.Char.Quests = domain.QuestLog{}
	}
	p.Player.Char.Quests.Finish(portableCubeTestQuest)
	p.Player.Bag.Set(98, domain.Stack{Item: portableCubeTestItem, Count: 1, Bound: true})
}
func prepareCubeArtisan(s *Scene) {
	p := s.players[1]
	p.Pos.MapID = s.id.MapID
	s.entities[78] = &entity.Entity{ID: 78, Kind: domain.KindNPC, Name: "巧匠左慈", Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: 73, Script: "N611FY"}}
	p.Player.QuestNPC = "巧匠左慈"
}
func TestPortableCubeRejectsLegacyNPCAndAllowsTravel(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	inputCubeNPC(s)
	r := inputCubeQuoteRequest()
	r.NPCEntityID = 77
	s.onEndgameCube(p, r)
	msg, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if msg.Message.Error != "tool_required" || b.commits != 0 {
		t.Fatal("legacy NPC authorized", msg)
	}
	p.Pos.X += 10000
	p.Player.QuestNPC = ""
	delete(s.entities, 77)
	r.NPCEntityID = 0
	s.onEndgameCube(p, r)
	msg, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !msg.Message.OK {
		t.Fatal("portable tool depended on NPC or proximity", msg)
	}
	before := p.Player.Bag.Clone()
	s.onUseItem(UseItem{ID: 1, Item: portableCubeTestItem, Tab: 0, BagSlot: 98})
	if !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatal("using permanent tool changed inventory")
	}
}
func TestPortableCubeRecoveryOwnershipCapacityAndAtomicity(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	prepareCubeArtisan(s)
	req := domain.EndgameRequest{Op: "cube.recover", RequestID: "cube-recover-test-001", ExpectedRevision: "0", NPCEntityID: 78}
	s.recoverCubeTool(p, req)
	msg, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if msg.Message.Error != "already_owned" || b.commits != 0 {
		t.Fatal("duplicate recovery accepted")
	}
	tool := p.Player.Bag.At(98)
	p.Player.Bag.Set(98, domain.Stack{})
	p.Player.Warehouse = domain.NewWarehouse(1, 10, 99)
	p.Player.Warehouse.Set(0, 0, tool)
	s.recoverCubeTool(p, req)
	msg, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if msg.Message.Error != "already_owned" || b.commits != 0 {
		t.Fatal("warehouse duplicate recovered")
	}
	p.Player.Warehouse.Set(0, 0, domain.Stack{})
	s.recoverCubeTool(p, req)
	if b.commits != 1 || p.Player.Bag.CountOf(portableCubeTestItem) != 0 {
		t.Fatal("grant visible before commit")
	}
	s.recoverCubeTool(p, req)
	if b.commits != 1 {
		t.Fatal("double grant")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(portableCubeTestItem) != 1 || !s.hasCubeTool(p) {
		t.Fatal("committed bound tool missing")
	}
}
func TestPortableCubeInitialGrantKeepsQuestUntilDurableAndRoom(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	prepareCubeArtisan(s)
	p.Player.Bag.Set(98, domain.Stack{})
	q := s.quests[portableCubeTestQuest]
	p.Player.Char.Quests.AcceptAt(q, 1)
	entry := p.Player.Char.Quests[q.ID]
	entry.RiftCompletionRunID = "eligible-completed-run"
	p.Player.Char.Quests[q.ID] = entry
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
	if b.commits != 1 || p.Player.Char.Quests.Done(q.ID) {
		t.Fatal("quest published before grant persisted")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if !p.Player.Char.Quests.Done(q.ID) || !s.hasCubeTool(p) {
		t.Fatal("quest and bound item not committed together")
	}
}

func TestPortableCubeRecoveryRejectsForgedContext(t *testing.T) {
	for name, alter := range map[string]func(*Scene){
		"unfinished quest": func(s *Scene) { delete(s.players[1].Player.Char.Quests, portableCubeTestQuest) },
		"missing NPC":      func(s *Scene) { delete(s.entities, 78) },
		"wrong NPC":        func(s *Scene) { s.entities[78].Name = "other" },
		"not database NPC": func(s *Scene) { s.entities[78].NPC.ConfigID = 0 },
		"no dialogue":      func(s *Scene) { s.players[1].Player.QuestNPC = "" },
		"different map":    func(s *Scene) { s.entities[78].Pos.MapID++ },
		"out of range":     func(s *Scene) { s.players[1].Pos.X += 481 },
		"instance copy":    func(s *Scene) { s.id.Instance = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			s, sink, b := cubeSceneFixture(t)
			prepareCubeArtisan(s)
			p := s.players[1]
			p.Player.Bag.Set(98, domain.Stack{})
			alter(s)
			s.recoverCubeTool(p, domain.EndgameRequest{Op: "cube.recover", RequestID: "cube-recover-forge-01", ExpectedRevision: "0", NPCEntityID: 78})
			msg, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if !ok || msg.Message.OK || b.commits != 0 || p.Player.Bag.CountOf(portableCubeTestItem) != 0 {
				t.Fatal("forged recovery granted", msg)
			}
		})
	}
}
func TestPortableCubeRecoveryRollbackAndReceiptReplay(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	prepareCubeArtisan(s)
	p.Player.Bag.Set(98, domain.Stack{})
	req := domain.EndgameRequest{Op: "cube.recover", RequestID: "cube-recover-rollback-01", ExpectedRevision: "0", NPCEntityID: 78}
	before := p.Player.Bag.Clone()
	s.onEndgame(EndgameRequest{ID: 1, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("no recovery transaction")
	}
	b.result <- domain.EndgameCommitResult{Err: errors.New("rollback")}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if !reflect.DeepEqual(before, p.Player.Bag) || !p.Player.Char.Quests.Done(portableCubeTestQuest) {
		t.Fatal("rollback changed character")
	}
	s.onEndgame(EndgameRequest{ID: 1, Request: req})
	s.finishEndgame(false)
	if b.commits != 2 {
		t.Fatal("definite failure not retryable")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	after := p.Player.Bag.Clone()
	p.Player.QuestNPC = ""
	delete(s.entities, 78)
	s.onEndgame(EndgameRequest{ID: 1, Request: req})
	s.finishEndgame(false)
	msg, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || !msg.Message.OK || b.commits != 2 || !reflect.DeepEqual(after, p.Player.Bag) {
		t.Fatal("receipt replay depended on live NPC/revision or granted twice", msg)
	}
	req.NPCEntityID = 99
	s.onEndgame(EndgameRequest{ID: 1, Request: req})
	s.finishEndgame(false)
	msg, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if msg.Message.Error != "request_id_conflict" || b.commits != 2 {
		t.Fatal("changed replay request accepted", msg)
	}
}
func TestPortableCubeInitialGrantRollbackAndUnavailableQuest(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	prepareCubeArtisan(s)
	p.Player.Bag.Set(98, domain.Stack{})
	q := s.quests[portableCubeTestQuest]
	p.Player.Char.Quests.AcceptAt(q, 1)
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	cubeTake(s, sink)
	if b.commits != 0 || p.Player.Char.Quests.Done(q.ID) {
		t.Fatal("uncompleted rift quest granted")
	}
	e := p.Player.Char.Quests[q.ID]
	e.RiftCompletionRunID = "eligible-completed-run"
	p.Player.Char.Quests[q.ID] = e
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	if b.commits != 1 {
		t.Fatal("no initial transaction")
	}
	b.result <- domain.EndgameCommitResult{Err: errors.New("rollback")}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if p.Player.Char.Quests.Done(q.ID) || p.Player.Bag.CountOf(portableCubeTestItem) != 0 {
		t.Fatal("failed commit consumed entitlement")
	}
	s.onCompleteQuest(CompleteQuest{ID: 1, Quest: q.ID})
	if b.commits != 2 {
		t.Fatal("initial reward retry rejected")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if !p.Player.Char.Quests.Done(q.ID) || p.Player.Bag.CountOf(portableCubeTestItem) != 1 {
		t.Fatal("initial retry failed")
	}
}
func TestPortableCubeAcceptRequiresPermanentTrial(t *testing.T) {
	s, sink, _ := cubeSceneFixture(t)
	p := s.players[1]
	q := s.quests[portableCubeTestQuest]
	delete(p.Player.Char.Quests, q.ID)
	s.entities[79] = &entity.Entity{ID: 79, Kind: domain.KindNPC, Name: q.NPC, NPC: &entity.NPCInfo{ConfigID: 3}}
	p.Player.QuestNPC = q.NPC
	s.onAcceptQuest(AcceptQuest{ID: 1, Quest: q.ID})
	cubeTake(s, sink)
	if p.Player.Char.Quests.Active(q.ID) {
		t.Fatal("trial prerequisite bypassed")
	}
	p.Player.Char.Trial.CompletedOnce = true
	s.onAcceptQuest(AcceptQuest{ID: 1, Quest: q.ID})
	cubeTake(s, sink)
	e := p.Player.Char.Quests[q.ID]
	if !p.Player.Char.Quests.Active(q.ID) || e.AcceptedAtMS <= 0 {
		t.Fatal("permanent qualification or accepted timestamp missing")
	}
}

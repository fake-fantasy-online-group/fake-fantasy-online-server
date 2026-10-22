package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestQuestProfessionWeaponsAtomicAndOnce(t *testing.T) {
	for _, race := range []domain.Race{domain.Warrior, domain.Swordsman, domain.Assassin, domain.Healer, domain.Warlock} {
		for _, capacity := range []int{1, 4} {
			s, _ := questScene(t)
			p := s.entities[1]
			ch := p.Player.Char
			ch.Race = race
			ch.EmploymentKnown = true
			ch.Employed = false
			ch.Quests = domain.QuestLog{}
			rewards := map[domain.Race][]domain.QuestEquipmentReward{
				domain.Warrior: {{Item: 1620}}, domain.Swordsman: {{Item: 1021}, {Item: 1520}},
				domain.Assassin: {{Item: 1820}, {Item: 1922}}, domain.Healer: {{Item: 1320}}, domain.Warlock: {{Item: 1320}},
			}
			for _, rs := range rewards {
				for _, r := range rs {
					s.items[r.Item] = domain.ItemDef{ID: r.Item, Name: "职业武器", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: 4}}
				}
			}
			def := domain.QuestDef{ID: 42, Name: "巡城最终奖励", NPC: "小叶", Steps: []domain.QuestStep{{Kind: domain.QuestStepTalk, To: "小叶"}}, ProfessionRewards: rewards}
			s.quests[42] = def
			ch.Quests.Accept(def)
			p.Player.QuestNPC = "小叶"
			p.Player.Bag = domain.NewBag(capacity)
			var rejection event.RejectReason
			s.completeQuestStep(p, def, func(r event.RejectReason) { rejection = r })
			want := rewards[race]
			if capacity < len(want) {
				if rejection != event.RejectBagFull || ch.Quests.Done(42) {
					t.Fatalf("race %v: capacity failure committed quest", race)
				}
				for _, r := range want {
					if p.Player.Bag.CountOf(r.Item) != 0 {
						t.Fatal("partially granted weapons")
					}
				}
				continue
			}
			if !ch.Quests.Done(42) {
				t.Fatalf("race %v did not finish: %v", race, rejection)
			}
			for rr, rs := range rewards {
				for _, r := range rs {
					n := p.Player.Bag.CountOf(r.Item)
					allowed := rr == race || ((race == domain.Healer || race == domain.Warlock) && (rr == domain.Healer || rr == domain.Warlock))
					if allowed && n != 1 || !allowed && n != 0 {
						t.Fatalf("race %v item %v count %v", race, r.Item, n)
					}
				}
			}
			// Replayed completion cannot award another copy or mutate the shared template.
			p.Player.QuestNPC = "小叶"
			s.completeQuestStep(p, def, func(r event.RejectReason) { rejection = r })
			for _, r := range want {
				if p.Player.Bag.CountOf(r.Item) != 1 {
					t.Fatal("duplicate reward")
				}
			}
			if ch.Quests.CanAccept(def, 60, "小叶") != domain.AcceptAlreadyDone {
				t.Fatal("completed nonrepeatable quest reaccepted")
			}
			if len(s.quests[42].Reward.Items) != 0 {
				t.Fatal("shared reward template mutated")
			}
		}
	}
}

func TestQuestProfessionWeaponsOnlyFinalStep(t *testing.T) {
	s, _ := questScene(t)
	p := s.entities[1]
	ch := p.Player.Char
	def := domain.QuestDef{ID: 42, NPC: "小叶", Steps: []domain.QuestStep{{Kind: domain.QuestStepTalk, To: "小叶"}, {Kind: domain.QuestStepTalk, To: "小叶"}}, ProfessionRewards: map[domain.Race][]domain.QuestEquipmentReward{ch.Race: {{Item: 铁护头.ID}}}}
	ch.Quests = domain.QuestLog{}
	ch.Quests.Accept(def)
	p.Player.QuestNPC = "小叶"
	s.completeQuestStep(p, def, func(r event.RejectReason) { t.Fatalf("unexpected reject %v", r) })
	if ch.Quests.Done(42) || p.Player.Bag.CountOf(铁护头.ID) != 0 {
		t.Fatal("intermediate step granted final weapon")
	}
	p.Player.QuestNPC = "小叶"
	s.completeQuestStep(p, def, func(r event.RejectReason) { t.Fatalf("unexpected reject %v", r) })
	if !ch.Quests.Done(42) || p.Player.Bag.CountOf(铁护头.ID) != 1 {
		t.Fatal("final step did not grant weapon")
	}
}

func TestQuestProfessionRewardPatrolPrerequisites(t *testing.T) {
	log := domain.QuestLog{}
	second := domain.QuestDef{ID: 41, Prerequisites: []domain.QuestID{40}}
	final := domain.QuestDef{ID: 42, Prerequisites: []domain.QuestID{41}}
	if log.CanAccept(second, 60, "") != domain.AcceptPrerequisite || log.CanAccept(final, 60, "") != domain.AcceptPrerequisite {
		t.Fatal("patrol chain bypassed")
	}
	log.Accept(domain.QuestDef{ID: 40})
	log.Finish(40)
	if log.CanAccept(second, 60, "") != domain.AcceptOK || log.CanAccept(final, 60, "") != domain.AcceptPrerequisite {
		t.Fatal("patrol two prerequisite drift")
	}
	log.Accept(second)
	log.Finish(41)
	if log.CanAccept(final, 60, "") != domain.AcceptOK {
		t.Fatal("final patrol unavailable")
	}
}

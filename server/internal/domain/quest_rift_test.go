package domain

import "testing"

func riftQuestDef() QuestDef {
	return QuestDef{ID: 1900030001, NPC: "试炼小仙", RequireTrialCompletion: true,
		Steps: []QuestStep{{Kind: QuestStepRiftComplete, To: "巧匠左慈"}}}
}

func TestQuestRiftCompletionAcceptanceBoundaryAndReaccept(t *testing.T) {
	def := riftQuestDef()
	all, log := QuestTable{def.ID: def}, QuestLog{}
	log.AcceptAt(def, 100)
	for _, at := range []int64{0, 99, 100} {
		if len(log.RecordRiftCompletion(all, "old-run", at)) != 0 || log.DeliverableAt(def, "巧匠左慈", nil) {
			t.Fatalf("pre-acceptance run credited: %d", at)
		}
	}
	if got := log.RecordRiftCompletion(all, "eligible-run", 101); len(got) != 1 || got[0] != def.ID {
		t.Fatal("completion was not credited", got)
	}
	if !log.DeliverableAt(def, "巧匠左慈", nil) || log.DeliverableAt(def, "试炼小仙", nil) || log[def.ID].Progress != 0 {
		t.Fatal("completion bypassed NPC or advanced the turn-in step")
	}
	if len(log.RecordRiftCompletion(all, "duplicate-run", 102)) != 0 || log[def.ID].RiftCompletionRunID != "eligible-run" {
		t.Fatal("duplicate completion changed the frozen evidence")
	}
	snapshot := log.Clone()
	log.Abandon(def.ID)
	log.AcceptAt(def, 200)
	log.RecordRiftCompletion(all, "eligible-run", 101)
	if log[def.ID].RiftCompletionRunID != "" || snapshot[def.ID].RiftCompletionRunID != "eligible-run" {
		t.Fatal("reaccept reused earlier completion or clone lost evidence")
	}
	log.RecordRiftCompletion(all, "next-run", 201)
	log.Finish(def.ID)
	if len(log.RecordRiftCompletion(all, "later-run", 202)) != 0 || !log.Done(def.ID) {
		t.Fatal("finished quest credited again")
	}
}

func TestQuestRiftCompletionFailsClosedWithoutAcceptance(t *testing.T) {
	def := riftQuestDef()
	log := QuestLog{}
	log.Accept(def)
	if got := log.RecordRiftCompletion(QuestTable{def.ID: def}, "run", 1000); len(got) != 0 {
		t.Fatal("legacy zero acceptance must not credit old runs")
	}
	def.Steps[0].Kind = QuestStepTalk
	log.AcceptAt(def, 100)
	if got := log.RecordRiftCompletion(QuestTable{def.ID: def}, "run", 1000); len(got) != 0 {
		t.Fatal("unrelated task received rift event")
	}
}

func TestQuestRiftProgressAndPermanentTrialPrerequisite(t *testing.T) {
	def := riftQuestDef()
	all := QuestTable{def.ID: def}
	ch := &Character{Level: 60, Quests: QuestLog{}}
	if got := OffersOf(all, "试炼小仙", ch, nil); len(got) != 0 {
		t.Fatal("new character qualified", got)
	}
	ch.Trial.CompletedOnce = true
	if got := OffersOf(all, "试炼小仙", ch, nil); len(got) != 1 {
		t.Fatal("permanent trial qualification was lost", got)
	}
	ch.Quests.AcceptAt(def, 100)
	before := ActiveQuestsOf(all, ch, nil)
	if len(before) != 1 || before[0].Ready || before[0].Progress != "完整通关演劫洞天 0/1次" {
		t.Fatal(before)
	}
	ch.Quests.RecordRiftCompletion(all, "run", 200)
	after := ActiveQuestsOf(all, ch, nil)
	if len(after) != 1 || !after[0].Ready || after[0].Progress != "完整通关演劫洞天 1/1次" {
		t.Fatal(after)
	}
	ch.Trial.Day = 20260101
	ch.Trial.DailyCompleted = 0
	ch.Trial.Cycle = 0
	if !ch.HasCompletedTrial() {
		t.Fatal("day/cycle reset removed permanent qualification")
	}
	for _, p := range []TrialProgress{{DailyCompleted: 1}, {Cycle: 1}} {
		ch.Trial = p
		if !ch.HasCompletedTrial() {
			t.Fatal("legacy settlement proof ignored", p)
		}
	}
}

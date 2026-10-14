package domain

import "testing"

func TestPortableToolQuestUsesConfiguredRecipientDialogue(t *testing.T) {
	q := QuestDef{ID: 1, NPC: "publisher", Desc: "introduction", RecoveryItem: 123, Steps: []QuestStep{{Kind: QuestStepRiftComplete, To: "recipient", Text: "go complete the rift", Say: "take this permanent tool"}}}
	ch := &Character{Level: 1, Quests: QuestLog{}}
	all := QuestTable{q.ID: q}
	offers := OffersOf(all, "publisher", ch, nil)
	if len(offers) != 1 || offers[0].Desc != q.Desc {
		t.Fatal("publisher introduction lost", offers)
	}
	ch.Quests.AcceptAt(q, 1)
	offers = OffersOf(all, "recipient", ch, nil)
	if len(offers) != 1 || offers[0].Desc != q.Steps[0].Text || offers[0].State != OfferActive {
		t.Fatal("unfinished task dialogue wrong", offers)
	}
	ch.Quests.RecordRiftCompletion(all, "complete-run", 2)
	offers = OffersOf(all, "recipient", ch, nil)
	if len(offers) != 1 || offers[0].Desc != q.Steps[0].Say || offers[0].State != OfferReady {
		t.Fatal("handover dialogue not shown", offers)
	}
	q.RecoveryItem = 0
	all[q.ID] = q
	offers = OffersOf(all, "recipient", ch, nil)
	if offers[0].Desc != q.Desc {
		t.Fatal("unrelated quest presentation changed")
	}
}

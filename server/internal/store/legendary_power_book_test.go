package store

import (
	"context"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func powerBookSalvagePlan(t *testing.T, m *Memory, s domain.Snapshot) domain.EndgameCommitPlan {
	t.Helper()
	st := legendaryStoreFixture()
	st.Legendary.PowerID = "TY-A01"
	st.Legendary.PowerRollBP = 8123
	s.Bag.Set(0, st)
	if err := m.SaveSnapshot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	book := domain.LegendaryPowerBook{}
	if _, err := book.RecordSalvagedPower(st); err != nil {
		t.Fatal(err)
	}
	payload, err := domain.WriteLegendaryPowerBook(`{"rift":{"tier":10}}`, book)
	if err != nil {
		t.Fatal(err)
	}
	after := s.Clone()
	after.Bag.Set(0, domain.Stack{})
	return domain.EndgameCommitPlan{Before: s, Snapshot: after, State: domain.EndgameState{Revision: 1, Payload: payload}, Receipt: domain.EndgameReceipt{CharID: s.Char.ID, RequestID: "powerbook-salvage-0001", Operation: "cube.execute", Digest: strings.Repeat("a", 64), Revision: 1, Response: `{"kind":"cube.result","ok":true,"revision":"1"}`}}
}

func TestPowerBookStoreSalvageAtomicFaults(t *testing.T) {
	for _, stage := range []string{"state", "snapshot", "receipt"} {
		t.Run(stage, func(t *testing.T) {
			m, s := endgameFixture(t)
			plan := powerBookSalvagePlan(t, m, s)
			f := &endgameFaultStore{Memory: m, stage: stage}
			got := awaitEndgame(t, NewEndgameBackend(f, nil).Commit(plan))
			if got.Err == nil {
				t.Fatal("injected failure did not fail")
			}
			loaded := durableSnapshot(t, m, plan.Before)
			if loaded.Bag.At(0).Legendary.PowerID != "TY-A01" {
				t.Fatal("failed salvage lost source item")
			}
			state, err := m.LoadEndgameState(context.Background(), s.Char.ID)
			if err != nil {
				t.Fatal(err)
			}
			book, err := domain.ReadLegendaryPowerBook(state.Payload)
			if err != nil || len(book) != 0 || state.Revision != 0 {
				t.Fatal("failed salvage retained record", book, state, err)
			}
			receipt, _ := m.LoadEndgameReceipt(context.Background(), s.Char.ID, plan.Receipt.RequestID)
			if receipt != nil {
				t.Fatal("failed transaction retained receipt")
			}
		})
	}
}

func TestPowerBookStoreLostAckReplayAndRelog(t *testing.T) {
	m, s := endgameFixture(t)
	plan := powerBookSalvagePlan(t, m, s)
	f := &endgameFaultStore{Memory: m, lostAck: true}
	b := NewEndgameBackend(f, nil)
	got := awaitEndgame(t, b.Commit(plan))
	if got.Err != nil {
		t.Fatal("lost ack not recovered", got.Err)
	}
	for i := 0; i < 3; i++ {
		got = awaitEndgame(t, b.Commit(plan))
		if got.Err != nil || !got.Replayed {
			t.Fatal("replay", got)
		}
	}
	loaded := durableSnapshot(t, m, plan.Before)
	if !loaded.Bag.At(0).Empty() {
		t.Fatal("successful salvage source returned")
	}
	state, err := m.LoadEndgameState(context.Background(), s.Char.ID)
	if err != nil {
		t.Fatal(err)
	}
	book, err := domain.ReadLegendaryPowerBook(state.Payload)
	if err != nil || len(book) != 1 || state.Revision != 1 {
		t.Fatal("record/revision changed on replay", book, state, err)
	}
	if roll, ok := book["TY-A01"]; !ok || roll != 8123 {
		t.Fatal("record missing after relog")
	}
	// A second character never inherits another character's record.
	a, err := m.CreateAccount(context.Background(), "powerbook-other", "hash")
	if err != nil {
		t.Fatal(err)
	}
	ch := &domain.Character{AccountID: a.ID, Name: "other", Level: 50, BagSlots: 400}
	if err = m.CreateChar(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	other, err := m.LoadEndgameState(context.Background(), ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherBook, err := domain.ReadLegendaryPowerBook(other.Payload)
	if err != nil || len(otherBook) != 0 {
		t.Fatal("book crossed character boundary")
	}
}

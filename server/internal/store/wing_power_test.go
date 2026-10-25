package store

import (
	"context"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestWingPowerSnapshotPersistenceAndValidation(t *testing.T) {
	m, s := endgameFixture(t)
	st := domain.Stack{UID: 9007199254740993, Item: 20609, Count: 1, InstanceKind: domain.ItemInstanceEquipment}
	for i, id := range []string{"TY-A01", "TY-D01", "TY-U01"} {
		st.WingPowers[i] = domain.WingPower{PowerID: id, SourceTier: domain.LegendaryTierTrue, SourceLevel: 100}
	}
	s.Bag.Set(0, st)
	if err := m.SaveSnapshot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	loaded, err := m.LoadBag(context.Background(), s.Char.ID, 400)
	if err != nil || loaded.At(0) != st {
		t.Fatal("powers lost", err)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var got domain.Stack
	if err = json.Unmarshal(raw, &got); err != nil || got != st {
		t.Fatal("snapshot JSON lost powers", err)
	}
	clone := s.Clone()
	next := clone.Bag.At(0)
	next.WingPowers[0] = domain.WingPower{}
	clone.Bag.Set(0, next)
	if s.Bag.At(0) != st {
		t.Fatal("clone aliased powers")
	}
	for _, mutate := range []func(*domain.Stack){func(s *domain.Stack) { s.WingPowers[0].PowerID = "TY-D01" }, func(s *domain.Stack) { s.WingPowers[0].SourceTier = "unknown" }, func(s *domain.Stack) { s.WingPowers[0].SourceLevel = 0 }, func(s *domain.Stack) { s.Count = 2 }, func(s *domain.Stack) { s.Legendary = legendaryStoreFixture().Legendary }} {
		bad := st
		mutate(&bad)
		if validateEndgameItem(bad, nil) == nil {
			t.Fatal("invalid wing accepted")
		}
	}
}

func TestWingPowerPostgresRoundTrip(t *testing.T) {
	p := testStore(t)
	ctx := context.Background()
	st := domain.Stack{UID: domain.NewItemInstanceID(), Item: 20609, Count: 1, InstanceKind: domain.ItemInstanceEquipment}
	for i, id := range []string{"TY-A01", "TY-D01", "TY-U01"} {
		st.WingPowers[i] = domain.WingPower{PowerID: id, SourceLevel: 100}
	}
	if err := saveItemInstances(ctx, p.catalogQuery(), "wing-power-test", st); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = p.pool.Exec(context.Background(), `DELETE FROM item_instances WHERE uid=$1`, st.UID) })
	loaded, err := loadItemInstances(ctx, p.catalogQuery(), []int64{st.UID})
	if err != nil || loaded[st.UID] != st {
		t.Fatal("SQL power roundtrip", err)
	}
	st.WingPowers[1] = domain.WingPower{}
	if err = saveItemInstances(ctx, p.catalogQuery(), "wing-power-test", st); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadItemInstances(ctx, p.catalogQuery(), []int64{st.UID})
	if err != nil || loaded[st.UID] != st {
		t.Fatal("SQL category overwrite", err)
	}
}

func TestWingPowerAtomicConsumptionFaultsAndReplay(t *testing.T) {
	for _, stage := range []string{"state", "snapshot", "receipt", "lost_ack"} {
		t.Run(stage, func(t *testing.T) {
			m, s := endgameFixture(t)
			wing := s.Bag.At(0)
			donor := legendaryStoreFixture()
			donor.UID++
			s.Bag.Set(1, donor)
			if err := m.SaveSnapshot(context.Background(), s); err != nil {
				t.Fatal(err)
			}
			plan := endgamePlan(s)
			after := wing
			after.WingPowers[0] = domain.WingPower{PowerID: "TY-A01", SourceLevel: 100}
			plan.Snapshot.Bag.Set(0, after)
			plan.Snapshot.Bag.Set(1, domain.Stack{})
			f := &endgameFaultStore{Memory: m, stage: stage, lostAck: stage == "lost_ack"}
			backend := NewEndgameBackend(f, nil)
			got := awaitEndgame(t, backend.Commit(plan))
			durable := durableSnapshot(t, m, s)
			if stage != "lost_ack" {
				if got.Err == nil || durable.Bag.At(0) != wing || durable.Bag.At(1) != donor {
					t.Fatal("partial power write or donor consumption", got)
				}
			} else {
				if got.Err != nil || !got.Replayed || durable.Bag.At(0) != after || !durable.Bag.At(1).Empty() {
					t.Fatal("lost ack did not reconcile", got)
				}
				again := awaitEndgame(t, backend.Commit(plan))
				if again.Err != nil || !again.Replayed || durableSnapshot(t, m, s).Bag.At(0) != after {
					t.Fatal("repeat extracted twice", again)
				}
			}
		})
	}
}

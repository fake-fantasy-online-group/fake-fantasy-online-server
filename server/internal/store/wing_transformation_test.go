package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestWingTransformationAtomicFaultsAndReplay(t *testing.T) {
	for _, stage := range []string{"state", "snapshot", "receipt", "lost_ack"} {
		t.Run(stage, func(t *testing.T) {
			m, s := endgameFixture(t)
			before := s.Bag.At(0)
			before.Item = 20609
			before.RefineLevel = 3
			before.WingPowers = domain.WingPowers{{PowerID: "TY-A01", SourceLevel: 100}, {PowerID: "TY-D01", SourceLevel: 100}, {PowerID: "TY-U01", SourceLevel: 100}}
			s.Bag.Set(0, before)
			feathers := domain.Stack{Item: 25029, Count: 8}
			s.Bag.Set(1, feathers)
			if err := m.SaveSnapshot(context.Background(), s); err != nil {
				t.Fatal(err)
			}
			p := endgamePlan(s)
			after := before
			after.Item = 20611
			after.Bound = true
			after.Endgame.Revision++
			p.Snapshot.Bag.Set(0, after)
			p.Snapshot.Bag.Set(1, domain.Stack{})
			f := &endgameFaultStore{Memory: m, stage: stage, lostAck: stage == "lost_ack"}
			b := NewEndgameBackend(f, nil)
			got := awaitEndgame(t, b.Commit(p))
			durable := durableSnapshot(t, m, s)
			if stage != "lost_ack" {
				if got.Err == nil || durable.Bag.At(0) != before || durable.Bag.At(1) != feathers {
					t.Fatal("partial transformation/material debit", got)
				}
			} else {
				if got.Err != nil || !got.Replayed || durable.Bag.At(0) != after || !durable.Bag.At(1).Empty() {
					t.Fatal("transformation lost-ack recovery", got)
				}
				again := awaitEndgame(t, b.Commit(p))
				if again.Err != nil || !again.Replayed || durableSnapshot(t, m, s).Bag.At(0) != after {
					t.Fatal("transformation replay", again)
				}
			}
		})
	}
}
func TestWingTransformationRejectsLosingPowerOrMultipleTargets(t *testing.T) {
	_, s := endgameFixture(t)
	p := endgamePlan(s)
	before := s.Bag.At(0)
	before.WingPowers[0] = domain.WingPower{PowerID: "TY-A01", SourceLevel: 100}
	p.Before.Bag.Set(0, before)
	after := before
	after.Item++
	after.Bound = true
	after.Endgame.Revision++
	after.WingPowers = domain.WingPowers{}
	p.Snapshot.Bag.Set(0, after)
	if _, _, err := wingTransformationOf(p); err == nil {
		t.Fatal("lost power accepted")
	}
	after.WingPowers = before.WingPowers
	p.Snapshot.Bag.Set(0, after)
	second := before
	second.UID++
	p.Before.Bag.Set(1, second)
	second.Item++
	second.Bound = true
	second.Endgame.Revision++
	p.Snapshot.Bag.Set(1, second)
	if _, _, err := wingTransformationOf(p); err == nil {
		t.Fatal("two transformations accepted")
	}
}

func TestWingTransformationLegacyKindAndPoweredHydration(t *testing.T) {
	_, s := endgameFixture(t)
	before := s.Bag.At(0)
	before.InstanceKind = domain.ItemInstanceNone
	s.Bag.Set(0, before)
	p := endgamePlan(s)
	after := before
	after.Item++
	after.Bound = true
	after.InstanceKind = domain.ItemInstanceEquipment
	after.Endgame.Revision++
	p.Snapshot.Bag.Set(0, after)
	if _, _, err := wingTransformationOf(p); err != nil {
		t.Fatal("legacy transformation rejected", err)
	}
	after.WingPowers[0] = domain.WingPower{PowerID: "TY-A01", SourceLevel: 100}
	cached := after
	cached.InstanceKind = domain.ItemInstanceNone
	hydrated, err := hydrateStack(cached, map[int64]domain.Stack{after.UID: after})
	if err != nil || hydrated.InstanceKind != domain.ItemInstanceEquipment || validateWingPowerItem(hydrated) != nil {
		t.Fatal("powered wing lost persistent kind", err)
	}
	after.WingPowers = domain.WingPowers{}
	hydrated, err = hydrateStack(cached, map[int64]domain.Stack{after.UID: after})
	if err != nil || hydrated.InstanceKind != domain.ItemInstanceNone {
		t.Fatal("ordinary legacy hydration contract changed", err)
	}
}

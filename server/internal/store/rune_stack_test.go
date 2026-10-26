package store

import (
	"context"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func persistedRuneFixture() domain.Stack {
	st := domain.Stack{UID: 123, Item: domain.FirstAstralRune, Count: 37, InstanceKind: domain.ItemInstanceSocketCard, Bound: true}
	st.Card = domain.CardInstanceState{Initialized: true, Count: 1, Bound: true}
	st.Card.Effects[0] = domain.CardEffect{Op: 1, Attr: 17, Mode: 2, Probability: 100, Value: 1234}
	return st
}

func TestSaveAstralRuneStackKeepsSingleFactAndEffects(t *testing.T) {
	st := persistedRuneFixture()
	q := &positionRecordingTx{}
	if err := saveItemInstances(context.Background(), q, "test", st); err != nil {
		t.Fatal(err)
	}
	if q.calls != 2 {
		t.Fatalf("want item + card fact writes, got %d", q.calls)
	}
	if q.args[0] != st.UID || q.args[1] != int16(st.Card.Count) || q.args[6].([]int32)[0] != 1234 {
		t.Fatalf("lost card snapshot: %#v", q.args)
	}
}

func TestHydrateAstralRuneLocationQuantityAndOldSingleton(t *testing.T) {
	st := persistedRuneFixture()
	fact := st
	fact.Count = 1
	for _, n := range []int32{1, 37, domain.MaxStack} {
		cached := domain.Stack{UID: st.UID, Item: st.Item, Count: n}
		got, err := hydrateStack(cached, map[int64]domain.Stack{st.UID: fact})
		want := st
		want.Count = n
		if err != nil || got != want {
			t.Fatalf("quantity %d: got %+v err %v", n, got, err)
		}
	}
}

func TestRejectNonRuneInstanceStacksAndInvalidQuantities(t *testing.T) {
	st := persistedRuneFixture()
	cases := []domain.Stack{}
	for _, n := range []int32{0, -1, domain.MaxStack + 1} {
		v := st
		v.Count = n
		cases = append(cases, v)
	}
	v := st
	v.Item = domain.FirstAstralRune - 1
	cases = append(cases, v)
	v = st
	v.InstanceKind = domain.ItemInstanceEquipment
	cases = append(cases, v)
	v = st
	v.RefineLevel = 1
	cases = append(cases, v)
	for _, bad := range cases {
		fact := bad
		fact.Count = 1
		if _, err := hydrateStack(bad, map[int64]domain.Stack{bad.UID: fact}); err == nil {
			t.Fatalf("hydrated invalid stack %+v", bad)
		}
		if bad.Count > 0 {
			q := &positionRecordingTx{}
			if err := saveItemInstances(context.Background(), q, "test", bad); err == nil || q.calls != 0 {
				t.Fatalf("saved invalid stack %+v calls=%d err=%v", bad, q.calls, err)
			}
		}
	}
}

func TestSaveRuneCardSnapshotFlagsIndependentOfRoot(t *testing.T) {
	for _, bound := range []bool{false, true} {
		for _, locked := range []bool{false, true} {
			st := persistedRuneFixture()
			st.Bound, st.Locked = !bound, !locked
			st.Card.Bound, st.Card.Locked = bound, locked
			q := &positionRecordingTx{}
			if err := saveItemInstances(context.Background(), q, "test", st); err != nil {
				t.Fatal(err)
			}
			if q.args[7] != bound || q.args[8] != locked {
				t.Fatalf("card flags replaced with root flags: %#v", q.args)
			}
		}
	}
}

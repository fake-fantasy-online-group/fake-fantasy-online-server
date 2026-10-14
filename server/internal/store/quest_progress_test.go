package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"testing"
)

func TestQuestRiftProgressSnapshotRoundtrip(t *testing.T) {
	m, s := endgameFixture(t)
	ctx := context.Background()
	entry := domain.QuestEntry{State: domain.QuestActive, AcceptedAtMS: 1720000000000, RiftCompletionRunID: "complete-run", Kills: map[domain.MonsterID]int32{10: 1}}
	s.Char.Quests = domain.QuestLog{1900030001: entry}
	s.Char.Trial.CompletedOnce = true
	if err := m.SaveSnapshot(ctx, s); err != nil {
		t.Fatal(err)
	}
	// Mutating the live aggregate after write must not rewrite saved proof.
	delete(s.Char.Quests, 1900030001)
	saved, err := m.LoadQuests(ctx, s.Char.ID)
	if err != nil || !reflect.DeepEqual(saved[1900030001], entry) {
		t.Fatal("lost quest event fields", saved, err)
	}
	c, err := m.CharByName(ctx, s.Char.Name)
	if err != nil || !c.Trial.CompletedOnce {
		t.Fatal("lost permanent qualification", c, err)
	}
}

func TestTrialQualificationCannotBeRevokedByStaleSnapshot(t *testing.T) {
	for _, path := range []string{"snapshot", "character", "transaction"} {
		t.Run(path, func(t *testing.T) {
			m, s := endgameFixture(t)
			ctx := context.Background()
			s.Char.Trial.DailyCompleted = 1 // Legacy proof is promoted at write.
			if err := m.SaveSnapshot(ctx, s); err != nil {
				t.Fatal(err)
			}
			s.Char.Trial = domain.TrialProgress{}
			var err error
			switch path {
			case "snapshot":
				err = m.SaveSnapshot(ctx, s)
			case "character":
				err = m.SaveChar(ctx, s.Char)
			case "transaction":
				err = m.WithTx(ctx, func(tx Store) error { return tx.SaveChar(ctx, s.Char) })
			}
			if err != nil {
				t.Fatal(err)
			}
			c, err := m.CharByName(ctx, s.Char.Name)
			if err != nil || !c.Trial.CompletedOnce {
				t.Fatal("stale/reset progress revoked qualification", c, err)
			}
		})
	}
}

type trialQualificationRow struct{}

func (trialQualificationRow) Scan(dest ...any) error {
	if err := (roundingCharRow{raw: []byte("{}")}).Scan(dest...); err != nil {
		return err
	}
	*dest[len(dest)-1].(*bool) = true // trial_completed_once is appended in charCols.
	return nil
}

func TestTrialQualificationPostgresBinding(t *testing.T) {
	for _, progress := range []domain.TrialProgress{{CompletedOnce: true}, {DailyCompleted: 1}, {Cycle: 1}} {
		q := &roundingSaveQuery{}
		if err := saveChar(context.Background(), q, &domain.Character{ID: 7, Trial: progress}); err != nil {
			t.Fatal(err)
		}
		if len(q.args) != 61 || q.args[60] != true {
			t.Fatal("permanent trial proof was not written", q.args)
		}
	}
	loaded, err := scanChar(trialQualificationRow{})
	if err != nil || !loaded.Trial.CompletedOnce {
		t.Fatal("permanent trial proof was not read", loaded, err)
	}
}

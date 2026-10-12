package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestV5CatalogVersionStorageDoesNotRewriteLegacy(t *testing.T) {
	for _, version := range []int32{0, 5} {
		st := domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{UniqueID: 1001, UniqueRollBP: 10123, UniqueContentVersion: version, FormationContentVersion: version}}
		before := st
		q := &revisionRecordingQuerier{}
		if err := saveEndgameItem(context.Background(), q, st); err != nil {
			t.Fatal(err)
		}
		if st != before || len(q.args) != 7 || q.args[3] != int32(10123) || q.args[5] != version || q.args[6] != version {
			t.Fatal("version or saved roll changed", q.args)
		}
	}
	legacy := domain.Stack{UID: 42, Item: 15043, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{UniqueID: 15043, UniqueRollBP: 10123}}
	if err := validateEndgameItem(legacy); err != nil {
		t.Fatal("rejected excluded legacy instance", err)
	}
	legacy.Endgame.UniqueContentVersion = 5
	if validateEndgameItem(legacy) == nil {
		t.Fatal("V5 outside259 accepted")
	}
	legacy.Item = 1001
	legacy.Endgame.UniqueID = 1001
	legacy.Endgame.UniqueContentVersion = 6
	if validateEndgameItem(legacy) == nil {
		t.Fatal("unknown future version silently consumed")
	}
}

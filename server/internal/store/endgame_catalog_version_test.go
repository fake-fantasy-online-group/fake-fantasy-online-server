package store

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestV5CatalogVersionStorageDoesNotRewriteLegacy(t *testing.T) {
	for _, version := range []int32{0, 5, domain.UniqueContentVersion} {
		st := domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{UniqueID: 1001, UniqueRollBP: 10123, UniqueContentVersion: version, FormationContentVersion: version}}
		before := st
		q := &revisionRecordingQuerier{}
		if err := saveEndgameItem(context.Background(), q, st); err != nil {
			t.Fatal(err)
		}
		if st != before || len(q.args) != 10 || q.args[3] != int32(10123) || q.args[5] != version || q.args[6] != version || q.args[7] != int32(0) {
			t.Fatal("version or saved roll changed", q.args)
		}
	}
	legacy := domain.Stack{UID: 42, Item: 15043, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{UniqueID: 15043, UniqueRollBP: 10123}}
	if err := validateEndgameItem(legacy, catalogfixture.EndgameCatalog()); err != nil {
		t.Fatal("rejected excluded legacy instance", err)
	}
	legacy.Endgame.UniqueContentVersion = 5
	if validateEndgameItem(legacy, catalogfixture.EndgameCatalog()) == nil {
		t.Fatal("V5 outside259 accepted")
	}
	legacy.Item = 1001
	legacy.Endgame.UniqueID = 1001
	legacy.Endgame.UniqueContentVersion = domain.UniqueContentVersion + 1
	if validateEndgameItem(legacy, catalogfixture.EndgameCatalog()) == nil {
		t.Fatal("unknown future version silently consumed")
	}
}

func (q *revisionRecordingQuerier) EndgameCatalog() *domain.EndgameCatalog {
	return catalogfixture.EndgameCatalog()
}

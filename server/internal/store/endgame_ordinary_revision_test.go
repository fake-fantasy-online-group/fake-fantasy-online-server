package store

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestOrdinaryCubeSocketRevisionDurableReplayAndCAS(t *testing.T) {
	m, s := endgameFixture(t)
	s.Char.Money = domain.Money{Silver: 100}
	s.Bag.Set(0, domain.Stack{})
	def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 1001, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: 4, LevelReq: 50, Durable: 100}}
	s.Bag.Set(200, domain.NewStack(def, 1))
	target := s.Bag.At(200)
	target.UID = 42
	s.Bag.Set(200, target)
	s.Bag.Set(100, domain.Stack{Item: domain.CubeStarMarrow, Count: 100})
	s.Bag.Set(101, domain.Stack{Item: domain.CubePurifiedDust, Count: 100})
	runes := astralRuneItemsFixture(7)
	for i, index := range []int{0, 5} {
		rune := domain.NewStack(runes[index], 1)
		rune.UID = int64(501 + i)
		s.Bag.Set(110+i, rune)
	}
	if err := m.SaveSnapshot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	backend := NewEndgameBackend(m, nil)
	money := int64(100000)
	items := map[domain.ItemID]domain.ItemDef{1001: def}
	for _, d := range cubeMaterialDefinitionsFixture() {
		items[d.ID] = d
	}
	for _, d := range runes {
		items[d.ID] = d
	}
	domain.AttachEndgameCatalog(items, catalogfixture.EndgameCatalog())
	for revision := int64(1); revision <= 2; revision++ {
		req := domain.EndgameRequest{RecipeID: domain.CubeOpenSocket, TargetUID: "42"}
		if revision == 2 {
			req.RecipeID = domain.CubeInscribeArray
			req.ItemUIDs = []string{"501", "502"}
		}
		cube, err := domain.PlanCube(domain.CubeContext{Catalog: catalogfixture.EndgameCatalog(), Rules: cubeRulesFixture(items), Level: 60, Bag: s.Bag, Money: money, Items: items, Intn: func(int) int { return 1 }}, req, true)
		if err != nil {
			t.Fatal(err)
		}
		p := endgamePlan(s)
		p.Run = nil
		p.Snapshot = s.Clone()
		p.Snapshot.Bag = cube.Bag
		p.Snapshot.Char.Money = domain.Money{Silver: cube.Money / 1000, Copper: cube.Money % 1000}
		p.ExpectedRevision = revision - 1
		p.State.Revision = revision
		p.Receipt.Revision = revision
		p.Receipt.RequestID = fmt.Sprintf("ordinary-socket-%02d", revision)
		got := awaitEndgame(t, backend.Commit(p))
		if got.Err != nil {
			t.Fatalf("ordinary cube mutation revision %d: %v", revision, got.Err)
		}
		loaded := durableSnapshot(t, m, s)
		st := loaded.Bag.At(200)
		if st.SocketCount != 2 || st.Endgame.Revision != revision || st.Endgame.UniqueID != 0 {
			t.Fatalf("lost socket/revision: %+v", st)
		}
		if !reflect.DeepEqual(loaded.Bag, cube.Bag) || loaded.Char.Money != p.Snapshot.Char.Money {
			t.Fatal("material or coin debit lost")
		}
		replay := awaitEndgame(t, backend.Commit(p))
		if replay.Err != nil || !replay.Replayed || replay.Authoritative.Bag.At(200).Endgame.Revision != revision {
			t.Fatalf("replay: %+v", replay)
		}
		p.Receipt.RequestID += "-stale"
		stale := awaitEndgame(t, backend.Commit(p))
		if !errors.Is(stale.Err, ErrEndgameRevision) {
			t.Fatalf("stale CAS accepted: %+v", stale)
		}
		if !reflect.DeepEqual(durableSnapshot(t, m, s), loaded) {
			t.Fatal("replay/stale request changed durable resources")
		}
		s = loaded
		money = cube.Money
	}
}

func TestEndgameOrdinaryMetadataValidation(t *testing.T) {
	valid := domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{Revision: 3}}
	if err := validateEndgameItem(valid, nil); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*domain.Stack){
		"negative revision":    func(s *domain.Stack) { s.Endgame.Revision = -1 },
		"ordinary unique roll": func(s *domain.Stack) { s.Endgame.UniqueRollBP = 8000 },
		"missing UID":          func(s *domain.Stack) { s.UID = 0 },
		"stacked equipment":    func(s *domain.Stack) { s.Count = 2 },
		"card revision":        func(s *domain.Stack) { s.InstanceKind = domain.ItemInstanceSocketCard },
		"unique mismatch":      func(s *domain.Stack) { s.Endgame.UniqueID = 1002; s.Endgame.UniqueRollBP = 10000 },
		"unique no roll":       func(s *domain.Stack) { s.Endgame.UniqueID = 1001 },
	} {
		t.Run(name, func(t *testing.T) {
			s := valid
			change(&s)
			if validateEndgameItem(s, nil) == nil {
				t.Fatal("accepted corrupt metadata")
			}
		})
	}
}

// This catches the second half of the regression even without a network DB:
// accepting the metadata must not silently delete the ordinary revision on save.
type revisionRecordingQuerier struct {
	querier
	statement string
	args      []any
}

func (q *revisionRecordingQuerier) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q.statement = sql
	q.args = args
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func TestSaveEndgameOrdinaryRevision(t *testing.T) {
	q := &revisionRecordingQuerier{}
	st := domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{Revision: 2}}
	if err := saveEndgameItem(context.Background(), q, st); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(q.statement, "INSERT INTO equipment_endgame_state") || len(q.args) != 7 || q.args[1] != int64(2) || q.args[2] != int32(0) || q.args[3] != int32(0) || q.args[4] != int32(0) {
		t.Fatalf("revision was not saved: %s %v", q.statement, q.args)
	}
	st.Endgame = domain.EndgameItemState{}
	if err := saveEndgameItem(context.Background(), q, st); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(q.statement, "DELETE") {
		t.Fatal("legacy zero metadata should need no overlay")
	}
}

func TestFormationRollStorageValidationAndExactSave(t *testing.T) {
	for _, roll := range []int32{0, 8000, 10417, 12000} {
		st := domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{FormationRollBP: roll}}
		q := &revisionRecordingQuerier{}
		if err := saveEndgameItem(context.Background(), q, st); err != nil {
			t.Fatal(err)
		}
		if roll != 0 && (len(q.args) != 7 || q.args[4] != roll) {
			t.Fatal("stored roll changed", q.args)
		}
	}
	for _, roll := range []int32{-1, 1, 7999, 12001} {
		st := domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{FormationRollBP: roll}}
		if validateEndgameItem(st, nil) == nil {
			t.Fatal("invalid formation roll accepted", roll)
		}
	}
}

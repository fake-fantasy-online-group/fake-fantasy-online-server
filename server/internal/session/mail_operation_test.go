package session

import (
	"context"
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type sessionMailOperationStore struct {
	committed         bool
	before, candidate domain.Snapshot
	commits, resolves int
}

func (f *sessionMailOperationStore) CommitMailOperation(_ context.Context, _ store.MailOperation, snap domain.Snapshot) error {
	f.commits++
	f.candidate = snap
	return errors.New("lost commit acknowledgement")
}
func (f *sessionMailOperationStore) ResolveMailOperation(_ context.Context, _ store.MailOperation, before domain.Snapshot) (bool, error) {
	f.resolves++
	f.before = before
	return f.committed, nil
}
func TestMailWritebackDeliversOnlyResolvedBusinessOutcome(t *testing.T) {
	for _, committed := range []bool{false, true} {
		wb := store.NewWriteBack(store.NewMemory(), time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		go wb.Run(context.Background())
		s := &Session{deps: Deps{WriteBack: wb}}
		before := domain.Snapshot{Char: &domain.Character{ID: 11, AccountID: 11, Money: domain.Money{Copper: 100}}}
		candidate := before.Clone()
		candidate.Char.Money.Copper = 50
		backend := &sessionMailOperationStore{committed: committed}
		err := s.commitReservedMail(backend, store.MailOperation{ID: "operation"}, scene.MailReserveResult{Snapshot: candidate, Reservation: scene.MailReservation{Before: before}})
		if (err == nil) != committed {
			t.Fatalf("committed=%v err=%v", committed, err)
		}
		if backend.commits != 1 || backend.resolves != 1 || backend.before.Char.Money.Copper != 100 || backend.candidate.Char.Money.Copper != 50 {
			t.Fatal("reconciliation did not retain baseline/candidate")
		}
		if err := wb.Close(time.Second); err != nil {
			t.Fatal(err)
		}
	}
}
func TestMailTimedOutPreparationRollsBackLateReservation(t *testing.T) {
	s := &Session{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	reply := make(chan scene.MailReserveResult, 1)
	cancel := make(chan struct{})
	returned := make(chan bool, 1)
	go func() { _, ok := s.awaitMailReservation(domain.SceneID{}, 1, reply, cancel); returned <- ok }()
	select {
	case ok := <-returned:
		if ok {
			t.Fatal("timeout accepted reservation")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("prepare did not time out")
	}
	select {
	case <-cancel:
	default:
		t.Fatal("late prepare not canceled")
	}
	result := make(chan bool, 1)
	done := make(chan struct{})
	reply <- scene.MailReserveResult{Reservation: scene.MailReservation{OperationID: "late", Result: result, Done: done}}
	select {
	case commit := <-result:
		if commit {
			t.Fatal("late candidate committed")
		}
	case <-time.After(time.Second):
		t.Fatal("late reservation abandoned")
	}
	close(done)
}

func (f *sessionMailOperationStore) CheckMailOperations(context.Context) error { return nil }

type unavailableMailSchema struct {
	store.Store
	store.MailStore
	sessionMailOperationStore
	checks int
}

func (f *unavailableMailSchema) CheckMailOperations(context.Context) error {
	f.checks++
	return &pgconn.PgError{Code: "42P01", Message: "relation mail_operation_receipts does not exist"}
}
func TestMailMissingReceiptSchemaRejectsBeforeReservation(t *testing.T) {
	backend := &unavailableMailSchema{}
	sink := &capSink{}
	s := &Session{sink: sink, log: slog.New(slog.NewTextHandler(io.Discard, nil)), stage: StageInGame, char: &domain.Character{ID: 1}, deps: Deps{
		Store: backend, MailRule: domain.MailRule{Capacity: 10, ExpireDays: 7, MaxAttach: 5}, WriteBack: store.NewWriteBack(store.NewMemory(), time.Second, nil),
		// No router: reaching actor preparation would panic, rather than silently
		// reserve assets while the receipts table is absent.
	}}
	s.onSendMail(protocol.Request{S1: "recipient", Money: 1})
	s.onMailAction(protocol.Request{U8: mailActionClaim, MailID: 1})
	if backend.checks != 2 || backend.commits != 0 || backend.resolves != 0 {
		t.Fatal("missing schema reached economic operation")
	}
	if len(sink.pkts) != 2 {
		t.Fatal("missing schema did not clearly notify client")
	}
}

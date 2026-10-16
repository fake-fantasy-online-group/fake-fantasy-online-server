package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrCommitUnknown means COMMIT may have reached the database. It is never a
// business rejection and must not release reservations or report a refund.
var ErrCommitUnknown = errors.New("store: transaction commit outcome unknown")

func classifyCommitError(err error) error {
	if err == nil || errors.Is(err, pgx.ErrTxCommitRollback) || errors.Is(err, pgx.ErrTxClosed) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code != "40003" {
		return err
	}
	return fmt.Errorf("%w: %w", ErrCommitUnknown, err)
}

// CommitSnapshots is exclusively for absolute snapshot replacement. Unlike a
// general mutation, replaying these writes has no additive/external side effect.
// If COMMIT is uncertain, preserve both ownership lanes and keep replacing the
// same candidate until a successful transaction proves it durable.
func (w *WriteBack) CommitSnapshots(a domain.Snapshot, b *domain.Snapshot) <-chan error {
	settle := func(ctx context.Context, raw Store, snaps ...domain.Snapshot) error {
		attempt := func(c context.Context) error {
			return raw.WithTx(c, func(tx Store) error {
				for _, snap := range snaps {
					if err := tx.SaveSnapshot(c, snap); err != nil {
						return err
					}
				}
				return nil
			})
		}
		err := attempt(ctx)
		if !errors.Is(err, ErrCommitUnknown) {
			return err
		}
		delay := 50 * time.Millisecond
		for {
			w.log.Warn("快照事务提交结果待确认，保留角色物权", "char", a.Char.ID, "err", err)
			time.Sleep(delay)
			retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.attemptTimeout)
			err = attempt(retryCtx)
			cancel()
			if err == nil {
				return nil
			}
			// A later definite failure cannot disprove an earlier successful COMMIT.
			delay = min(delay*2, 3*time.Second)
		}
	}
	if b == nil {
		return w.CommitMutation(a, func(ctx context.Context, raw Store, snap domain.Snapshot) error { return settle(ctx, raw, snap) })
	}
	return w.CommitPairMutation(a, *b, func(ctx context.Context, raw Store, a, b domain.Snapshot) error { return settle(ctx, raw, a, b) })
}

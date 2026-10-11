package store

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"time"
)

type endgameBackend struct {
	raw    Store
	writer *WriteBack
}

func NewEndgameBackend(raw Store, writer *WriteBack) domain.EndgameBackend {
	return &endgameBackend{raw: raw, writer: writer}
}
func endgameStore(s Store) (EndgameStore, error) {
	e, ok := s.(EndgameStore)
	if !ok {
		return nil, fmt.Errorf("endgame: unsupported store")
	}
	return e, nil
}
func endgameContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
func (b *endgameBackend) Load(id int64) <-chan domain.EndgameLoadResult {
	ch := make(chan domain.EndgameLoadResult, 1)
	go func() {
		defer close(ch)
		ctx, cancel := endgameContext()
		defer cancel()
		e, err := endgameStore(b.raw)
		var v domain.EndgameState
		if err == nil {
			v, err = e.LoadEndgameState(ctx, id)
		}
		ch <- domain.EndgameLoadResult{State: v, Err: err}
	}()
	return ch
}
func (b *endgameBackend) LoadReceipt(id int64, key string) <-chan domain.EndgameReceiptLoadResult {
	ch := make(chan domain.EndgameReceiptLoadResult, 1)
	go func() {
		defer close(ch)
		ctx, cancel := endgameContext()
		defer cancel()
		e, err := endgameStore(b.raw)
		var v *domain.EndgameReceipt
		if err == nil {
			v, err = e.LoadEndgameReceipt(ctx, id, key)
		}
		ch <- domain.EndgameReceiptLoadResult{Receipt: v, Err: err}
	}()
	return ch
}
func (b *endgameBackend) LoadRun(id string) <-chan domain.EndgameRunLoadResult {
	ch := make(chan domain.EndgameRunLoadResult, 1)
	go func() {
		defer close(ch)
		ctx, cancel := endgameContext()
		defer cancel()
		e, err := endgameStore(b.raw)
		var v *domain.EndgameRunRecord
		if err == nil {
			v, err = e.LoadEndgameRun(ctx, id)
		}
		ch <- domain.EndgameRunLoadResult{Run: v, Err: err}
	}()
	return ch
}

func (b *endgameBackend) Commit(p domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	ch := make(chan domain.EndgameCommitResult, 1)
	// Freeze all reference-bearing data before returning to the actor.
	p.Before = p.Before.Clone()
	p.Snapshot = p.Snapshot.Clone()
	if p.Run != nil {
		v := *p.Run
		p.Run = &v
	}
	go func() {
		defer close(ch)
		if err := validateEndgamePlan(p); err != nil {
			ch <- domain.EndgameCommitResult{Err: err}
			return
		}
		if _, err := endgameStore(b.raw); err != nil {
			ch <- domain.EndgameCommitResult{Err: err}
			return
		}
		var result domain.EndgameCommitResult
		mutation := func(ctx context.Context, st Store, _ domain.Snapshot) error {
			return st.WithTx(ctx, func(tx Store) error {
				e, err := endgameStore(tx)
				if err != nil {
					return err
				}
				if err = e.LockEndgameCharacter(ctx, p.Receipt.CharID); err != nil {
					return err
				}
				prior, err := e.LoadEndgameReceipt(ctx, p.Receipt.CharID, p.Receipt.RequestID)
				if err != nil {
					return err
				}
				state, err := e.LoadEndgameState(ctx, p.Receipt.CharID)
				if err != nil {
					return err
				}
				if prior != nil {
					return endgameReplay(ctx, tx, e, p, state, *prior, &result)
				}
				if state.Revision != p.ExpectedRevision {
					return endgameRecover(ctx, e, p, state, ErrEndgameRevision, &result)
				}
				if err = e.SaveEndgameState(ctx, p.Receipt.CharID, p.State, p.ExpectedRevision); err != nil {
					return err
				}
				if err = tx.SaveSnapshot(ctx, p.Snapshot); err != nil {
					return err
				}
				if p.Run != nil {
					if err = e.LockEndgameRun(ctx, p.Run.RunID); err != nil {
						return err
					}
					if err = e.SaveEndgameRun(ctx, *p.Run, p.ExpectedRunRevision); err != nil {
						return err
					}
				}
				if err = e.SaveEndgameReceipt(ctx, p.Receipt); err != nil {
					return err
				}
				result = domain.EndgameCommitResult{State: p.State, Receipt: p.Receipt}
				return nil
			})
		}
		var err error
		if b.writer != nil {
			err = <-b.writer.CommitMutation(p.Snapshot, mutation)
		} else {
			ctx, cancel := endgameContext()
			err = mutation(ctx, b.raw, p.Snapshot)
			cancel()
		}
		if err != nil {
			result = b.reconcile(p, err)
		}
		ch <- result
	}()
	return ch
}
func validateEndgamePlan(p domain.EndgameCommitPlan) error {
	if p.Before.Char == nil || p.Snapshot.Char == nil || p.Before.Char.ID != p.Snapshot.Char.ID || p.Before.Char.AccountID != p.Snapshot.Char.AccountID || p.Receipt.CharID != p.Snapshot.Char.ID {
		return fmt.Errorf("endgame: invalid snapshot ownership")
	}
	if p.ExpectedRevision < 0 || p.State.Revision != p.ExpectedRevision+1 || p.Receipt.Revision != p.State.Revision {
		return ErrEndgameRevision
	}
	if len(p.Receipt.RequestID) < 16 || len(p.Receipt.RequestID) > 64 || len(p.Receipt.Digest) != 64 {
		return fmt.Errorf("endgame: invalid receipt")
	}
	if err := validateEndgameSnapshot(p.Before); err != nil {
		return err
	}
	return validateEndgameSnapshot(p.Snapshot)
}
func endgameRecover(ctx context.Context, e EndgameStore, p domain.EndgameCommitPlan, state domain.EndgameState, cause error, result *domain.EndgameCommitResult) error {
	snap, err := e.LoadEndgameSnapshot(ctx, p.Before)
	if err != nil {
		return err
	}
	*result = domain.EndgameCommitResult{State: state, RequiresReload: true, Authoritative: &snap, Err: cause}
	return nil
}
func endgameReplay(ctx context.Context, tx Store, e EndgameStore, p domain.EndgameCommitPlan, state domain.EndgameState, prior domain.EndgameReceipt, result *domain.EndgameCommitResult) error {
	if prior.Digest != p.Receipt.Digest || prior.Operation != p.Receipt.Operation {
		return endgameRecover(ctx, e, p, state, ErrEndgameReceiptConflict, result)
	}
	if state.Revision != p.ExpectedRevision {
		if err := endgameRecover(ctx, e, p, state, nil, result); err != nil {
			return err
		}
	} else {
		if err := tx.SaveSnapshot(ctx, p.Before); err != nil {
			return err
		}
		*result = domain.EndgameCommitResult{State: state}
	}
	result.Receipt = prior
	result.Replayed = true
	return nil
}

// A failed COMMIT can already have committed. Keep ownership frozen until a new
// transaction takes the same character lock and proves receipt presence/absence.
// Never report a refund/rollback on a mere connection error.
func (b *endgameBackend) reconcile(p domain.EndgameCommitPlan, cause error) domain.EndgameCommitResult {
	delay := 50 * time.Millisecond
	for {
		var result domain.EndgameCommitResult
		ctx, cancel := endgameContext()
		err := b.raw.WithTx(ctx, func(tx Store) error {
			e, err := endgameStore(tx)
			if err != nil {
				return err
			}
			if err = e.LockEndgameCharacter(ctx, p.Receipt.CharID); err != nil {
				return err
			}
			prior, err := e.LoadEndgameReceipt(ctx, p.Receipt.CharID, p.Receipt.RequestID)
			if err != nil {
				return err
			}
			state, err := e.LoadEndgameState(ctx, p.Receipt.CharID)
			if err != nil {
				return err
			}
			if prior != nil {
				return endgameReplay(ctx, tx, e, p, state, *prior, &result)
			}
			if state.Revision != p.ExpectedRevision {
				return endgameRecover(ctx, e, p, state, ErrEndgameRevision, &result)
			}
			// CommitMutation coalesces prior ordinary saves. Preserve that baseline only
			// after proving the failed operation absent and the revision still current.
			if err = tx.SaveSnapshot(ctx, p.Before); err != nil {
				return err
			}
			result = domain.EndgameCommitResult{State: state, Err: cause}
			return nil
		})
		cancel()
		if err == nil {
			return result
		}
		time.Sleep(delay)
		if delay < 3*time.Second {
			delay *= 2
		}
	}
}

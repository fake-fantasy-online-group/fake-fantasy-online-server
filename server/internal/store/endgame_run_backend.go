package store

import (
	"bytes"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"time"
)

func (b *endgameBackend) SaveRun(v domain.EndgameRunRecord, expected int64) <-chan domain.EndgameRunSaveResult {
	ch := make(chan domain.EndgameRunSaveResult, 1)
	go func() {
		defer close(ch)
		if _, err := endgameStore(b.raw); err != nil {
			ch <- domain.EndgameRunSaveResult{Err: err}
			return
		}
		if expected < 0 || v.Revision != expected+1 || !json.Valid([]byte(v.Payload)) {
			ch <- domain.EndgameRunSaveResult{Err: ErrEndgameRevision}
			return
		}
		ctx, cancel := endgameContext()
		err := b.raw.WithTx(ctx, func(tx Store) error {
			e, err := endgameStore(tx)
			if err != nil {
				return err
			}
			if err = e.LockEndgameRun(ctx, v.RunID); err != nil {
				return err
			}
			return e.SaveEndgameRun(ctx, v, expected)
		})
		cancel()
		if err == nil {
			ch <- domain.EndgameRunSaveResult{Run: &v}
			return
		}
		cause := err
		delay := 50 * time.Millisecond
		for {
			var current *domain.EndgameRunRecord
			ctx, cancel = endgameContext()
			err = b.raw.WithTx(ctx, func(tx Store) error {
				e, err := endgameStore(tx)
				if err != nil {
					return err
				}
				if err = e.LockEndgameRun(ctx, v.RunID); err != nil {
					return err
				}
				current, err = e.LoadEndgameRun(ctx, v.RunID)
				return err
			})
			cancel()
			if err == nil {
				if sameEndgameRun(current, &v) {
					ch <- domain.EndgameRunSaveResult{Run: current}
				} else {
					ch <- domain.EndgameRunSaveResult{Run: current, Err: cause}
				}
				return
			}
			time.Sleep(delay)
			if delay < 3*time.Second {
				delay *= 2
			}
		}
	}()
	return ch
}
func sameEndgameRun(a, b *domain.EndgameRunRecord) bool {
	if a == nil || b == nil || a.RunID != b.RunID || a.Revision != b.Revision || a.State != b.State {
		return false
	}
	decode := func(s string) any {
		var v any
		d := json.NewDecoder(bytes.NewBufferString(s))
		d.UseNumber()
		if d.Decode(&v) != nil {
			return nil
		}
		return v
	}
	return reflect.DeepEqual(decode(a.Payload), decode(b.Payload))
}

var _ domain.EndgameBackend = (*endgameBackend)(nil)
var _ domain.EndgameRunBackend = (*endgameBackend)(nil)
var _ EndgameStore = (*Postgres)(nil)
var _ EndgameStore = (*pgTx)(nil)
var _ EndgameStore = (*Memory)(nil)
var _ EndgameStore = (*txView)(nil)

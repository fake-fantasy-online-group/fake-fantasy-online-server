package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (t *txView) LockEndgameCharacter(_ context.Context, id int64) error {
	if t.m.chars[id] == nil {
		return domain.ErrCharNotFound
	}
	return nil
}
func (t *txView) LockEndgameRun(context.Context, string) error { return nil }
func (m *Memory) LockEndgameCharacter(context.Context, int64) error {
	return fmt.Errorf("endgame: transaction required")
}
func (m *Memory) LockEndgameRun(context.Context, string) error {
	return fmt.Errorf("endgame: transaction required")
}
func (t *txView) LoadEndgameState(_ context.Context, id int64) (domain.EndgameState, error) {
	v := t.m.endgameStates[id]
	if v.Payload == "" {
		v.Payload = "{}"
	}
	return v, nil
}
func (t *txView) SaveEndgameState(_ context.Context, id int64, v domain.EndgameState, expected int64) error {
	if t.m.chars[id] == nil {
		return domain.ErrCharNotFound
	}
	if expected < 0 || v.Revision != expected+1 || t.m.endgameStates[id].Revision != expected {
		return ErrEndgameRevision
	}
	if !json.Valid([]byte(v.Payload)) {
		return fmt.Errorf("endgame: invalid state JSON")
	}
	t.m.endgameStates[id] = v
	return nil
}
func (t *txView) LoadEndgameReceipt(_ context.Context, id int64, key string) (*domain.EndgameReceipt, error) {
	v, ok := t.m.endgameReceipts[endgameReceiptKey{id, key}]
	if !ok {
		return nil, nil
	}
	return &v, nil
}
func (t *txView) SaveEndgameReceipt(_ context.Context, v domain.EndgameReceipt) error {
	key := endgameReceiptKey{v.CharID, v.RequestID}
	if _, ok := t.m.endgameReceipts[key]; ok {
		return ErrEndgameReceiptConflict
	}
	if t.m.chars[v.CharID] == nil || len(v.RequestID) < 16 || len(v.RequestID) > 64 || len(v.Digest) != 64 || v.Revision <= 0 || !json.Valid([]byte(v.Response)) {
		return fmt.Errorf("endgame: invalid receipt")
	}
	t.m.endgameReceipts[key] = v
	return nil
}
func (t *txView) LoadEndgameRun(_ context.Context, id string) (*domain.EndgameRunRecord, error) {
	v, ok := t.m.endgameRuns[id]
	if !ok {
		return nil, nil
	}
	return &v, nil
}
func (t *txView) SaveEndgameRun(_ context.Context, v domain.EndgameRunRecord, expected int64) error {
	if expected < 0 || v.Revision != expected+1 || t.m.endgameRuns[v.RunID].Revision != expected {
		return ErrEndgameRevision
	}
	if len(v.RunID) < 1 || len(v.RunID) > 64 || !json.Valid([]byte(v.Payload)) {
		return fmt.Errorf("endgame: invalid run")
	}
	t.m.endgameRuns[v.RunID] = v
	return nil
}

type endgameReceiptKey struct {
	CharID    int64
	RequestID string
}

func (m *Memory) LoadEndgameSnapshot(context.Context, domain.Snapshot) (domain.Snapshot, error) {
	return domain.Snapshot{}, fmt.Errorf("endgame: transaction required")
}
func (t *txView) LoadEndgameSnapshot(_ context.Context, template domain.Snapshot) (domain.Snapshot, error) {
	if template.Char == nil {
		return domain.Snapshot{}, domain.ErrCharNotFound
	}
	id := template.Char.ID
	c := t.m.chars[id].Clone()
	if c == nil {
		return domain.Snapshot{}, domain.ErrCharNotFound
	}
	if a := t.m.accByID[c.AccountID]; a != nil {
		c.Caiyu = a.Caiyu
	}
	c.Skills = t.m.skills[id].Clone()
	c.Quests = t.m.quests[id].Clone()
	c.Pets = domain.ClonePets(t.m.pets[id])
	return domain.Snapshot{Char: c, Bag: t.m.bags[id].Clone(), Worn: t.m.worn[id].Clone(), ChangeSet: t.m.changeSets[id].Clone(), Warehouse: t.m.warehouses[id].Clone(), Wardrobe: t.m.wardrobes[id].Clone(), Stall: t.m.stalls[id].Clone()}, nil
}

func (m *Memory) LoadEndgameState(ctx context.Context, id int64) (domain.EndgameState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return (&txView{m: m}).LoadEndgameState(ctx, id)
}

func (m *Memory) SaveEndgameState(ctx context.Context, id int64, v domain.EndgameState, expected int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return (&txView{m: m}).SaveEndgameState(ctx, id, v, expected)
}

func (m *Memory) LoadEndgameReceipt(ctx context.Context, id int64, key string) (*domain.EndgameReceipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return (&txView{m: m}).LoadEndgameReceipt(ctx, id, key)
}

func (m *Memory) SaveEndgameReceipt(ctx context.Context, v domain.EndgameReceipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return (&txView{m: m}).SaveEndgameReceipt(ctx, v)
}

func (m *Memory) LoadEndgameRun(ctx context.Context, id string) (*domain.EndgameRunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return (&txView{m: m}).LoadEndgameRun(ctx, id)
}

func (m *Memory) SaveEndgameRun(ctx context.Context, v domain.EndgameRunRecord, expected int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return (&txView{m: m}).SaveEndgameRun(ctx, v, expected)
}

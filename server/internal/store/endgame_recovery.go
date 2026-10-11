package store

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (p *Postgres) LoadEndgameSnapshot(context.Context, domain.Snapshot) (domain.Snapshot, error) {
	return domain.Snapshot{}, fmt.Errorf("endgame: transaction required")
}

// Caller holds the character row lock. Reuse every authoritative native reader;
// never reconstruct a candidate from receipts or overwrite unrelated inventory.
func (t *pgTx) LoadEndgameSnapshot(ctx context.Context, template domain.Snapshot) (s domain.Snapshot, err error) {
	if template.Char == nil {
		return s, domain.ErrCharNotFound
	}
	s.Char, err = scanChar(t.tx.QueryRow(ctx, `SELECT `+charCols+` FROM characters WHERE id=$1`, template.Char.ID))
	if err != nil {
		return
	}
	id := s.Char.ID
	s.Char.Skills, err = t.LoadSkills(ctx, id)
	if err != nil {
		return
	}
	s.Char.Quests, err = t.LoadQuests(ctx, id)
	if err != nil {
		return
	}
	s.Char.Pets, err = t.LoadPets(ctx, id)
	if err != nil {
		return
	}
	s.Bag, err = t.LoadBag(ctx, id, int(s.Char.BagSlots))
	if err != nil {
		return
	}
	s.Worn, err = t.LoadEquips(ctx, id)
	if err != nil {
		return
	}
	s.ChangeSet, err = t.LoadChangeSet(ctx, id)
	if err != nil {
		return
	}
	if template.Warehouse != nil {
		s.Warehouse, err = t.LoadWarehouse(ctx, id, domain.WarehouseRule{InitialPages: 1, MaxPages: 255, SlotsPerPage: template.Warehouse.SlotsPerPage(), MaxStack: domain.MaxStack})
		if err != nil {
			return
		}
	}
	s.Wardrobe, err = loadWardrobeState(ctx, t.tx, id, 1, 65535)
	if err != nil {
		return
	}
	s.Stall, err = t.LoadStall(ctx, id)
	return
}

package store

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// SetItemDefinitions injects the same authored definitions used by the scene.
// Startup only: copying the map prevents callers from changing policy in flight.
func SetItemDefinitions(s Store, defs map[domain.ItemID]domain.ItemDef) error {
	if defs == nil {
		return fmt.Errorf("store: missing item definitions")
	}
	cp := make(map[domain.ItemID]domain.ItemDef, len(defs))
	for id, d := range defs {
		cp[id] = d
	}
	switch x := s.(type) {
	case *Postgres:
		x.itemDefinitions = cp
	case *Memory:
		x.itemDefinitions = cp
	case *pgTx:
		x.itemDefinitions = cp
		x.tx = &endgameCatalogTx{Tx: x.tx, catalog: x.endgameCatalog, items: cp}
	case *txView:
		x.m.itemDefinitions = cp
	default:
		return fmt.Errorf("store: unsupported item definitions injection")
	}
	return nil
}
func (q endgameCatalogQuerier) ItemDefinitions() map[domain.ItemID]domain.ItemDef { return q.items }
func (q *endgameCatalogTx) ItemDefinitions() map[domain.ItemID]domain.ItemDef     { return q.items }
func normalizeStoredBinding(q querier, st domain.Stack) domain.Stack {
	if p, ok := q.(interface {
		ItemDefinitions() map[domain.ItemID]domain.ItemDef
	}); ok {
		return normalizeBindingFromDefinitions(p.ItemDefinitions(), st)
	}
	return st
}
func normalizeBindingFromDefinitions(defs map[domain.ItemID]domain.ItemDef, st domain.Stack) domain.Stack {
	return domain.NormalizeStackBindings(st, func(id domain.ItemID) (domain.ItemDef, bool) { d, ok := defs[id]; return d, ok })
}

func normalizeBindingSnapshot(s domain.Snapshot, defs map[domain.ItemID]domain.ItemDef) domain.Snapshot {
	s = s.Clone()
	norm := func(st domain.Stack) domain.Stack { return normalizeBindingFromDefinitions(defs, st) }
	if s.Bag != nil {
		s.Bag.Each(func(slot int, st domain.Stack) { s.Bag.Set(slot, norm(st)) })
	}
	if s.Warehouse != nil {
		s.Warehouse.Each(func(page, slot int, st domain.Stack) { s.Warehouse.Set(page, slot, norm(st)) })
	}
	if s.Worn != nil {
		s.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) { s.Worn.Set(slot, norm(st)) })
	}
	if s.ChangeSet != nil {
		s.ChangeSet.Each(func(slot domain.EquipSlot, st domain.Stack) { s.ChangeSet.Set(slot, norm(st)) })
	}
	if s.Stall != nil {
		for i := range s.Stall.Items {
			s.Stall.Items[i].Stack = norm(s.Stall.Items[i].Stack)
		}
	}
	return s
}

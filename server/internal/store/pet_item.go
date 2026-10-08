package store

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Every owning pet is represented exactly once in its owner's bag or sell stall.
// This guard runs before writes, so a missing association cannot become a save.
func validatePetItems(s domain.Snapshot) error {
	if s.Char == nil || s.Bag == nil {
		return nil
	}
	carriers := map[int64]domain.Stack{}
	bad := false
	add := func(st domain.Stack) {
		if st.InstanceKind != domain.ItemInstancePet {
			return
		}
		if st.UID <= 0 || st.Count != 1 {
			bad = true
		}
		if _, dup := carriers[st.UID]; dup {
			bad = true
		}
		carriers[st.UID] = st
	}
	s.Bag.Each(func(_ int, st domain.Stack) { add(st) })
	if s.Stall != nil && s.Stall.Type == domain.StallSell {
		for _, it := range s.Stall.Items {
			add(it.Stack)
		}
	}
	if bad {
		return fmt.Errorf("invalid or duplicate pet item")
	}
	for _, p := range s.Char.Pets {
		if p.ItemUID <= 0 {
			return fmt.Errorf("pet %d missing item UID", p.ID)
		}
		if _, ok := carriers[p.ItemUID]; !ok {
			return fmt.Errorf("pet %d has no owned item", p.ID)
		}
		delete(carriers, p.ItemUID)
	}
	if len(carriers) != 0 {
		return fmt.Errorf("pet item has no matching pet")
	}
	return nil
}

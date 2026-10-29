package domain

// ItemBindingMode is an authored type-level rule, not a restriction inferred
// from CanTrade/CanMail. Unknown definitions retain their state until audited.
// Real equipment always retains per-instance bind-on-equip semantics.
type ItemBindingMode uint8

const (
	ItemBindingUnspecified ItemBindingMode = iota
	ItemBindingFixedUnbound
	ItemBindingFixedBound
)

// NormalizeItemBinding repairs only the binding bit of a known non-equipment
// definition. Quantity, identity, effects, instance state and locks are untouched.
func NormalizeItemBinding(def ItemDef, st Stack) Stack {
	if def.ID != st.Item || def.Equip != nil || def.FixedBinding == ItemBindingUnspecified {
		return st
	}
	switch def.FixedBinding {
	case ItemBindingFixedUnbound:
		st.Bound = false
	case ItemBindingFixedBound:
		st.Bound = true
	default:
		return st
	}
	if st.InstanceKind == ItemInstanceSocketCard && st.Card.Initialized {
		st.Card.Bound = st.Bound
	}
	return st
}

// NormalizeStackBindings also applies the rules to owned embedded cards/spirits,
// without changing the parent equipment's own binding or any lock/rolled state.
func NormalizeStackBindings(st Stack, lookup func(ItemID) (ItemDef, bool)) Stack {
	if lookup == nil {
		return st
	}
	if def, ok := lookup(st.Item); ok {
		st = NormalizeItemBinding(def, st)
	}
	for i, id := range st.Sockets {
		if id == 0 || !st.SocketCards[i].Initialized {
			continue
		}
		if def, ok := lookup(id); ok {
			card := NormalizeItemBinding(def, Stack{Item: id, InstanceKind: ItemInstanceSocketCard, Bound: st.SocketCards[i].Bound, Card: st.SocketCards[i]})
			st.SocketCards[i] = card.Card
		}
	}
	if st.FusedSpirit.Item != 0 {
		if def, ok := lookup(st.FusedSpirit.Item); ok {
			child := NormalizeItemBinding(def, st.FusedSpirit.Stack())
			st.FusedSpirit.Bound = child.Bound
		}
	}
	return st
}

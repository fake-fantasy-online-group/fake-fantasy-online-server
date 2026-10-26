package domain

// IsAstralRuneStack recognizes a homogeneous stack of astral cards. Its UID
// identifies the owned stack, not every unit. Card snapshots are never rerolled.
// Ordinary socket cards and equipment remain individual instances.
func IsAstralRuneStack(st Stack) bool {
	if st.Item < FirstAstralRune || st.Item > LastAstralRune || st.UID <= 0 || st.InstanceKind != ItemInstanceSocketCard || st.Count <= 0 || st.Count > MaxStack || !st.Card.Initialized || int(st.Card.Count) > len(st.Card.Effects) {
		return false
	}
	clean := Stack{UID: st.UID, Item: st.Item, Count: st.Count, InstanceKind: st.InstanceKind, Bound: st.Bound, Locked: st.Locked, Card: st.Card}
	return clean == st
}

// SplitOwnedStack allocates a fresh identity only when part of a rune stack
// moves to another owner/location. Whole-stack moves retain their identity.
// Pure consumption should use RemoveAt, without manufacturing a new identity.
func SplitOwnedStack(st Stack, count int32) (remaining, taken Stack, ok bool) {
	if st.Empty() || count <= 0 || count > st.Count {
		return st, Stack{}, false
	}
	taken = st
	taken.Count = count
	remaining = st
	remaining.Count -= count
	if remaining.Count == 0 {
		remaining = Stack{}
	} else if st.UID != 0 {
		if !IsAstralRuneStack(st) {
			return st, Stack{}, false
		}
		taken.UID = NewItemInstanceID()
	}
	return remaining, taken, true
}

func (b *Bag) addAstralRuneStack(def ItemDef, incoming Stack) int32 {
	if !IsAstralRuneStack(incoming) || !def.Stackable || def.InstanceKind != ItemInstanceSocketCard {
		return incoming.Count
	}
	start, end := bagRange(def, len(b.slots))
	for _, existing := range b.slots {
		if !existing.Empty() && existing.UID == incoming.UID {
			return incoming.Count
		}
	}
	remaining := incoming
	for i := start; i < end; i++ {
		dst := &b.slots[i]
		if !IsAstralRuneStack(*dst) || dst.UID == incoming.UID || !SameStackInstance(*dst, incoming) || dst.Count >= MaxStack {
			continue
		}
		n := MaxStack - dst.Count
		if n > remaining.Count {
			n = remaining.Count
		}
		dst.Count += n
		remaining.Count -= n
		if remaining.Count == 0 {
			return 0
		}
	}
	for i := start; i < end; i++ {
		if b.slots[i].Empty() {
			b.slots[i] = remaining
			return 0
		}
	}
	return remaining.Count
}

// AstralRuneTransferProtected also respects protection carried by the immutable
// card snapshot, including legacy snapshots whose root flags differ.
func AstralRuneTransferProtected(st Stack) bool {
	return IsAstralRuneStack(st) && (st.Card.Bound || st.Card.Locked)
}

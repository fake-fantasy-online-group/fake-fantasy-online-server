package domain

// These plans never publish state or write storage. The scene commits candidate
// clones and a durable idempotency receipt together before exposing success.
type AstralSocketPlan struct {
	Bag    *Bag
	Worn   *EquipSet
	Target Stack
	Rune   Stack
	Socket uint8
}
type AstralSocketError string

func (e AstralSocketError) Error() string { return string(e) }

type astralTarget struct {
	stack    Stack
	def      ItemDef
	bagSlot  int
	wornSlot EquipSlot
	worn     bool
}

func findAstralTarget(bag *Bag, worn *EquipSet, defs func(ItemID) (ItemDef, bool), uid int64) (astralTarget, error) {
	if uid <= 0 || bag == nil || defs == nil {
		return astralTarget{}, AstralSocketError("not_found")
	}
	var found astralTarget
	count := 0
	for i := 0; i < bag.Cap(); i++ {
		st := bag.At(i)
		if !st.Empty() && st.UID == uid {
			found = astralTarget{stack: st, bagSlot: i}
			count++
		}
	}
	worn.Each(func(slot EquipSlot, st Stack) {
		if st.UID == uid {
			found = astralTarget{stack: st, wornSlot: slot, worn: true}
			count++
		}
	})
	if count != 1 {
		return astralTarget{}, AstralSocketError("not_owner")
	}
	def, ok := defs(found.stack.Item)
	if !ok || def.Equip == nil || (found.stack.InstanceKind != ItemInstanceEquipment && found.stack.InstanceKind != ItemInstanceNone) || found.stack.Count != 1 {
		return astralTarget{}, AstralSocketError("invalid_socket")
	}
	if found.stack.Locked {
		return astralTarget{}, AstralSocketError("locked")
	}
	found.def = def
	found.stack.InstanceKind = ItemInstanceEquipment
	return found, nil
}
func astralUIDCount(bag *Bag, worn *EquipSet, uid int64) int {
	count := 0
	visit := func(st Stack) {
		if st.Empty() {
			return
		}
		if st.UID == uid {
			count++
		}
		for _, id := range st.SocketUIDs {
			if id == uid {
				count++
			}
		}
	}
	for i := 0; i < bag.Cap(); i++ {
		visit(bag.At(i))
	}
	worn.Each(func(_ EquipSlot, st Stack) { visit(st) })
	return count
}
func validateAstralTarget(t astralTarget, hole uint8) error {
	st := t.stack
	if st.SocketCount > 5 || int(st.SocketCount) > EquipmentSocketCapacity(t.def, st) || hole >= st.SocketCount {
		return AstralSocketError("invalid_socket")
	}
	seen := map[int64]bool{}
	for i := range st.Sockets {
		if i >= int(st.SocketCount) {
			if st.Sockets[i] != 0 || st.SocketUIDs[i] != 0 {
				return AstralSocketError("invalid_socket")
			}
			continue
		}
		if st.Sockets[i] == 0 && st.SocketUIDs[i] == 0 {
			continue
		}
		if st.Sockets[i] <= 0 || st.SocketUIDs[i] <= 0 || !st.SocketCards[i].Initialized || seen[st.SocketUIDs[i]] {
			return AstralSocketError("invalid_socket")
		}
		seen[st.SocketUIDs[i]] = true
	}
	return nil
}
func setAstralTarget(p *AstralSocketPlan, t astralTarget) {
	if t.worn {
		p.Worn.Set(t.wornSlot, p.Target)
	} else {
		p.Bag.Set(t.bagSlot, p.Target)
	}
}
func PlanAstralInsert(bag *Bag, worn *EquipSet, defs func(ItemID) (ItemDef, bool), targetUID, runeUID int64, hole uint8) (AstralSocketPlan, error) {
	t, err := findAstralTarget(bag, worn, defs, targetUID)
	if err != nil {
		return AstralSocketPlan{}, err
	}
	if err = validateAstralTarget(t, hole); err != nil {
		return AstralSocketPlan{}, err
	}
	if t.stack.Sockets[hole] != 0 || runeUID <= 0 || runeUID == targetUID || astralUIDCount(bag, worn, runeUID) != 1 {
		return AstralSocketPlan{}, AstralSocketError("invalid_rune")
	}
	slot := -1
	var rune Stack
	for i := 0; i < bag.Cap(); i++ {
		st := bag.At(i)
		if !st.Empty() && st.UID == runeUID {
			slot = i
			rune = st
			break
		}
	}
	if slot < 0 {
		return AstralSocketPlan{}, AstralSocketError("not_owner")
	}
	rd, ok := defs(rune.Item)
	if _, astral := AstralRune(rune.Item); !ok || !astral || rd.InstanceKind != ItemInstanceSocketCard || rune.InstanceKind != ItemInstanceSocketCard || rune.Count != 1 || !rune.Card.Initialized {
		return AstralSocketPlan{}, AstralSocketError("invalid_rune")
	}
	if rune.Locked || rune.Card.Locked {
		return AstralSocketPlan{}, AstralSocketError("locked")
	}
	p := AstralSocketPlan{Bag: bag.Clone(), Worn: worn.Clone(), Target: t.stack, Rune: rune, Socket: hole}
	if !p.Bag.RemoveAt(slot, 1) {
		return AstralSocketPlan{}, AstralSocketError("invalid_rune")
	}
	card := rune.Card
	card.Bound = rune.Bound || rune.Card.Bound || t.stack.Bound
	card.Locked = false
	p.Target.Bound = p.Target.Bound || card.Bound
	p.Target.Sockets[hole] = rune.Item
	p.Target.SocketUIDs[hole] = rune.UID
	p.Target.SocketCards[hole] = card
	p.Rune.Bound = card.Bound
	p.Rune.Card = card
	setAstralTarget(&p, t)
	return p, nil
}
func PlanAstralRemove(bag *Bag, worn *EquipSet, defs func(ItemID) (ItemDef, bool), targetUID int64, hole uint8) (AstralSocketPlan, error) {
	t, err := findAstralTarget(bag, worn, defs, targetUID)
	if err != nil {
		return AstralSocketPlan{}, err
	}
	if err = validateAstralTarget(t, hole); err != nil {
		return AstralSocketPlan{}, err
	}
	id, uid, card := t.stack.Sockets[hole], t.stack.SocketUIDs[hole], t.stack.SocketCards[hole]
	if _, ok := AstralRune(id); !ok || uid <= 0 || astralUIDCount(bag, worn, uid) != 1 || !card.Initialized {
		return AstralSocketPlan{}, AstralSocketError("invalid_rune")
	}
	if card.Locked {
		return AstralSocketPlan{}, AstralSocketError("locked")
	}
	def, ok := defs(id)
	if !ok || def.InstanceKind != ItemInstanceSocketCard {
		return AstralSocketPlan{}, AstralSocketError("invalid_rune")
	}
	rune := Stack{UID: uid, Item: id, Count: 1, InstanceKind: ItemInstanceSocketCard, Card: card, Bound: card.Bound, Locked: card.Locked}
	p := AstralSocketPlan{Bag: bag.Clone(), Worn: worn.Clone(), Target: t.stack, Rune: rune, Socket: hole}
	if p.Bag.AddStack(def, rune) != 0 {
		return AstralSocketPlan{}, AstralSocketError("bag_full")
	}
	p.Target.Sockets[hole] = 0
	p.Target.SocketUIDs[hole] = 0
	p.Target.SocketCards[hole] = CardInstanceState{}
	setAstralTarget(&p, t)
	return p, nil
}

package store

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Snapshot all persistent memory state, not just endgame rows. This gives error
// and panic rollback the same semantics as PostgreSQL transactions.
func copyMap[K comparable, V any](in map[K]V, clone func(V) V) map[K]V {
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = clone(v)
	}
	return out
}
func copyValues[K comparable, V any](in map[K]V) map[K]V {
	return copyMap(in, func(v V) V { return v })
}
func (m *Memory) transactionCopy() *Memory {
	out := NewMemory()
	out.accByID = copyMap(m.accByID, func(v *Account) *Account { cp := *v; return &cp })
	for name, a := range m.accounts {
		out.accounts[name] = out.accByID[a.ID]
	}
	out.chars = copyMap(m.chars, func(v *domain.Character) *domain.Character { return v.Clone() })
	out.bags = copyMap(m.bags, func(v *domain.Bag) *domain.Bag { return v.Clone() })
	out.warehouses = copyMap(m.warehouses, func(v *domain.Warehouse) *domain.Warehouse { return v.Clone() })
	out.wardrobes = copyMap(m.wardrobes, func(v *domain.Wardrobe) *domain.Wardrobe { return v.Clone() })
	out.stalls = copyMap(m.stalls, func(v *domain.Stall) *domain.Stall { return v.Clone() })
	out.worn = copyMap(m.worn, func(v *domain.EquipSet) *domain.EquipSet { return v.Clone() })
	out.changeSets = copyMap(m.changeSets, func(v *domain.ChangeSet) *domain.ChangeSet { return v.Clone() })
	out.skills = copyMap(m.skills, func(v domain.Learned) domain.Learned { return v.Clone() })
	out.quests = copyMap(m.quests, func(v domain.QuestLog) domain.QuestLog { return v.Clone() })
	out.pets = copyMap(m.pets, domain.ClonePets)
	out.friends = copyMap(m.friends, func(v map[int64]string) map[int64]string { return copyValues(v) })
	out.blocks = copyMap(m.blocks, func(v map[int64]uint8) map[int64]uint8 { return copyValues(v) })
	out.seenTips = copyMap(m.seenTips, func(v map[int32]struct{}) map[int32]struct{} { return copyValues(v) })
	out.charName = copyValues(m.charName)
	out.endgameStates = copyValues(m.endgameStates)
	out.endgameReceipts = copyValues(m.endgameReceipts)
	out.endgameRuns = copyValues(m.endgameRuns)
	out.nextAcc, out.nextChar = m.nextAcc, m.nextChar
	return out
}
func (m *Memory) restoreTransaction(b *Memory) {
	m.accounts = b.accounts
	m.accByID = b.accByID
	m.chars = b.chars
	m.bags = b.bags
	m.warehouses = b.warehouses
	m.wardrobes = b.wardrobes
	m.stalls = b.stalls
	m.worn = b.worn
	m.changeSets = b.changeSets
	m.skills = b.skills
	m.quests = b.quests
	m.pets = b.pets
	m.friends = b.friends
	m.blocks = b.blocks
	m.seenTips = b.seenTips
	m.charName = b.charName
	m.endgameStates = b.endgameStates
	m.endgameReceipts = b.endgameReceipts
	m.endgameRuns = b.endgameRuns
	m.nextAcc = b.nextAcc
	m.nextChar = b.nextChar
}

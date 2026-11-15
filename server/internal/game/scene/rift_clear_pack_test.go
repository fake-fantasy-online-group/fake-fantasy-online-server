package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func registerPackAndPotions(s *Scene) {
	domain.RegisterRiftClearPack(s.items)
	for _, reward := range domain.RiftClearPackPotionRewards() {
		s.items[reward.Item] = domain.ItemDef{ID: reward.Item, Name: "药水", Stackable: true, InventoryTab: 0, InventoryTabKnown: true}
	}
}

func TestRiftClearPackGrantedOnceOnSuccessfulClear(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	registerPackAndPotions(s)
	s.rift.data.RewardSourceLevel = 60
	for _, m := range s.rift.data.Run.Members {
		m.Eligible = true
	}
	s.generateEndgameRiftGrants()
	grant := s.rift.data.Grants[100]
	if countPack(grant) != 1 || !grant.Items[len(grant.Items)-1].Bound {
		t.Fatalf("clear pack missing or unbound %+v", grant.Items)
	}
	before := append([]domain.Stack(nil), grant.Items...)
	s.generateEndgameRiftGrants()
	if countPack(s.rift.data.Grants[100]) != 1 || len(s.rift.data.Grants[100].Items) != len(before) {
		t.Fatal("clear pack generated twice")
	}
	// Ineligible members, or catalogs without the pack, must not invent a grant.
	s.rift.data.GrantsReady = false
	s.rift.data.Grants = nil
	s.rift.data.Run.Members[100].Eligible = false
	s.generateEndgameRiftGrants()
	if _, ok := s.rift.data.Grants[100]; ok {
		t.Fatal("ineligible member received a pack")
	}
}

func TestRiftClearPackSkippedWhenCatalogMissing(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.rift.data.RewardSourceLevel = 60
	for _, m := range s.rift.data.Run.Members {
		m.Eligible = true
	}
	s.generateEndgameRiftGrants()
	if countPack(s.rift.data.Grants[100]) != 0 {
		t.Fatal("missing catalog still granted a pack")
	}
}

func countPack(g endgameRiftGrant) int {
	n := 0
	for _, st := range g.Items {
		if st.Item == domain.RiftClearPackItemID {
			n += int(st.Count)
		}
	}
	return n
}

func TestRiftClearPackOpenUsesOpenerClassAndRollsBackWhenFull(t *testing.T) {
	s, sink := gachaScene(t, nil)
	registerPackAndPotions(s)
	warriorPiece := domain.HardDungeonItemOffset + 8005
	warlockPiece := domain.HardDungeonItemOffset + 8401
	s.items[warriorPiece] = domain.ItemDef{ID: warriorPiece, Name: "真*战士甲", HardDungeonOnly: true, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Need: domain.Requirement{Professions: 1 << domain.Warrior, ProfessionRestricted: true}}}
	s.items[warlockPiece] = domain.ItemDef{ID: warlockPiece, Name: "真*术士袍", HardDungeonOnly: true, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Need: domain.Requirement{Professions: 1 << domain.Warlock, ProfessionRestricted: true}}}
	pack := domain.NewRiftClearPackItem()
	s.items[domain.RiftClearPackItemID] = pack
	p := s.entities[1]
	p.Player.Char.Race = domain.Warlock
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	p.Player.Bag = domain.NewBag(80)
	p.Player.Bag.Add(pack, 1)

	riftClearPackTrueSetRoll = func(*Scene) bool { return true }
	t.Cleanup(func() { riftClearPackTrueSetRoll = nil })
	s.exec(UseItem{ID: 1, Item: domain.RiftClearPackItemID, Tab: 0, BagSlot: 0})
	s.step()
	evs := sink.take()
	if p.Player.Bag.CountOf(domain.RiftClearPackItemID) != 0 || p.Player.Bag.CountOf(3121) == 0 || p.Player.Bag.CountOf(3151) == 0 {
		t.Fatal("potion open failed", noticeTexts(evs))
	}
	if p.Player.Bag.CountOf(warlockPiece) != 1 || p.Player.Bag.CountOf(warriorPiece) != 0 {
		t.Fatal("true set must match opener class only")
	}
	if !joinedNotice(evs, "打开成功") {
		t.Fatal("success notice", noticeTexts(evs))
	}

	// A miss must never grant another class set either.
	p.Player.Bag = domain.NewBag(80)
	p.Player.Bag.Add(pack, 1)
	p.Player.Char.Race = domain.Warrior
	riftClearPackTrueSetRoll = func(*Scene) bool { return false }
	s.exec(UseItem{ID: 1, Item: domain.RiftClearPackItemID, Tab: 0, BagSlot: 0})
	s.step()
	if p.Player.Bag.CountOf(warriorPiece) != 0 || p.Player.Bag.CountOf(warlockPiece) != 0 || p.Player.Bag.CountOf(3121) == 0 {
		t.Fatal("miss still granted a true set or lost potions")
	}

	// Worst-case pre-check refuses before rolling when even max potions cannot fit.
	p.Player.Bag = domain.NewBag(14)
	p.Player.Bag.Add(pack, 1)
	for i := 2; i < 14; i++ {
		p.Player.Bag.Add(s.items[9998], 1)
	}
	p.Player.Char.Race = domain.Warlock
	riftClearPackTrueSetRoll = func(*Scene) bool { return true }
	sink.take()
	s.exec(UseItem{ID: 1, Item: domain.RiftClearPackItemID, Tab: 0, BagSlot: 0})
	s.step()
	evs = sink.take()
	if p.Player.Bag.CountOf(domain.RiftClearPackItemID) != 1 || p.Player.Bag.CountOf(3121) != 0 || p.Player.Bag.CountOf(warlockPiece)+p.Player.Bag.CountOf(warriorPiece) != 0 {
		t.Fatal("full bag consumed the pack or leaked rewards")
	}
	if !joinedNotice(evs, "背包空间不足") {
		t.Fatal("full bag notice", noticeTexts(evs))
	}
}

func TestRiftClearPackOpenWithExistingStacksBelowThirtyTwoFreeSlots(t *testing.T) {
	s, sink := gachaScene(t, nil)
	registerPackAndPotions(s)
	warlockPiece := domain.HardDungeonItemOffset + 8401
	s.items[warlockPiece] = domain.ItemDef{ID: warlockPiece, Name: "真*术士袍", HardDungeonOnly: true, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Need: domain.Requirement{Professions: 1 << domain.Warlock, ProfessionRestricted: true}}}
	pack := domain.NewRiftClearPackItem()
	s.items[domain.RiftClearPackItemID] = pack
	p := s.entities[1]
	p.Player.Char.Race = domain.Warlock
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	p.Player.Bag = domain.NewBag(20)
	if p.Player.Bag.Add(pack, 1) != 0 {
		t.Fatal("pack")
	}
	for _, reward := range domain.RiftClearPackPotionRewards() {
		if p.Player.Bag.Add(s.items[reward.Item], reward.MaxQty) != 0 {
			t.Fatal("seed", reward.Item)
		}
	}
	nextFiller := domain.ItemID(99001)
	for p.Player.Bag.FreeSlots() > 1 {
		def := domain.ItemDef{ID: nextFiller, InventoryTab: 0, InventoryTabKnown: true}
		s.items[nextFiller] = def
		if p.Player.Bag.Add(def, 1) != 0 {
			t.Fatal("filler", nextFiller)
		}
		nextFiller++
	}
	if p.Player.Bag.FreeSlots() != 1 {
		t.Fatalf("setup free slots %d", p.Player.Bag.FreeSlots())
	}
	riftClearPackTrueSetRoll = func(*Scene) bool { return true }
	t.Cleanup(func() { riftClearPackTrueSetRoll = nil })
	s.exec(UseItem{ID: 1, Item: domain.RiftClearPackItemID, Tab: 0, BagSlot: 0})
	s.step()
	evs := sink.take()
	if p.Player.Bag.CountOf(domain.RiftClearPackItemID) != 0 || p.Player.Bag.CountOf(3121) < 3+5 || p.Player.Bag.CountOf(warlockPiece) != 1 {
		t.Fatal("stack-aware open failed", p.Player.Bag.FreeSlots(), p.Player.Bag.CountOf(3121), p.Player.Bag.CountOf(warlockPiece), noticeTexts(evs))
	}
	if !joinedNotice(evs, "打开成功") {
		t.Fatal("success notice", noticeTexts(evs))
	}
	foundBound := false
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.Item == warlockPiece && st.Bound && st.Count == 1 {
			foundBound = true
		}
	})
	if !foundBound {
		t.Fatal("true set piece was not bound")
	}
}

func TestCancelledRiftPersistsLongchengAndLeavesLiveRuns(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	p := s.players[1]
	p.Pos = domain.Pos{MapID: s.id.MapID, X: 12, Y: 34}
	pos, inst := s.persistedPosition(p)
	if pos != p.Pos || inst != s.id.Instance {
		t.Fatal("live rift persist", pos, inst)
	}
	s.rift.data.Run.Phase = domain.RiftCompleted
	pos, inst = s.persistedPosition(p)
	if pos != p.Pos || inst != s.id.Instance {
		t.Fatal("completed settlement persist", pos, inst)
	}
	s.rift.data.Run.Phase = domain.RiftAborted
	pos, inst = s.persistedPosition(p)
	if pos != domain.LongchengSpawn || inst != 0 {
		t.Fatal("cancelled persist", pos, inst)
	}
}

func TestCancelledRiftNeedsCityReturnAndLiveDestination(t *testing.T) {
	s, _, now := riftFixture(t)
	_ = now
	r := NewRouter(func(id domain.SceneID) (*Scene, error) { return s, nil }, nil)
	id := domain.SceneID{MapID: 88, Instance: 9}
	riftPos := domain.Pos{MapID: 88, X: 1, Y: 2}
	own := domain.CharID(100)
	r.InstallRiftAdmissionForTest(RiftAdmissionTestSeed{ID: id, MapID: 88, Phase: domain.RiftActive, Char: own, AttachScene: true, Return: domain.Pos{MapID: 3, X: 4, Y: 5}})
	if dest, _, ok := r.ActiveRiftDestination(own); !ok || dest != id {
		t.Fatal("live destination lost", dest, ok)
	}
	if r.CancelledRiftNeedsCityReturn(own, id, riftPos) {
		t.Fatal("live admission treated as cancelled")
	}
	r.riftAdmissions[id].Data.Run.Phase = domain.RiftAborted
	if _, _, ok := r.ActiveRiftDestination(own); ok {
		t.Fatal("aborted run still offered as a destination")
	}
	if !r.CancelledRiftNeedsCityReturn(own, id, riftPos) || !r.CancelledRiftNeedsCityReturn(own, domain.SceneID{MapID: 88}, riftPos) {
		t.Fatal("cancelled admission did not request 龙城")
	}
	if r.CancelledRiftNeedsCityReturn(own, domain.SceneID{}, domain.Pos{MapID: 3, X: 4, Y: 5}) {
		t.Fatal("already-left return map forced back to 龙城")
	}
}

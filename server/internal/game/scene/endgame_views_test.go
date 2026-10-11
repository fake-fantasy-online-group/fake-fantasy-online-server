package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
)

func endgameMessages(s *Scene, sink *fakeSink) []domain.EndgameResponse {
	s.flush()
	out := []domain.EndgameResponse{}
	for _, e := range sink.take() {
		if x, ok := e.(event.EndgameMessage); ok {
			out = append(out, x.Message)
		}
	}
	return out
}
func TestEndgameSnapshotNativeCoordinatesAndStableRandom(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 0, 0)
	sink.take()
	p := s.players[1]
	s.endgamePlayer(p).loaded = true
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	s.items = map[domain.ItemID]domain.ItemDef{123: {ID: 123, Name: "符", InventoryTab: 2, InventoryTabKnown: true,
		Equip: &domain.EquipDef{Slot: 25, Need: domain.Requirement{Level: 45, Professions: 24}}}}
	p.Player.Bag.Set(299, domain.Stack{Item: 123, UID: 9007199254740993, Count: 1})
	p.Player.Worn.Set(25, domain.Stack{Item: 123, UID: 9007199254740994, Count: 1})
	s.rng = rand.New(rand.NewSource(42))
	want := rand.New(rand.NewSource(42)).Int63()
	s.sendEndgameSnapshot(p)
	rs := endgameMessages(s, sink)
	if len(rs) != 1 || len(rs[0].Items) != 2 {
		t.Fatalf("bad snapshot: %+v", rs)
	}
	a, b := rs[0].Items[0], rs[0].Items[1]
	if a.UID != "9007199254740993" || a.Tab != 2 || a.Slot != 99 || a.Location != "bag" || b.Location != "equipped" || b.Slot != 25 {
		t.Fatal(a, b)
	}
	if a.Level != 45 || a.Profession != 24 || b.Level != 45 || b.Profession != 24 {
		t.Fatal("snapshot omitted bag/equipped requirements", a, b)
	}
	if s.rng.Int63() != want {
		t.Fatal("UI observation consumed gameplay RNG")
	}
	old := rs[0].SnapshotID
	s.sendEndgameSnapshot(p)
	if endgameMessages(s, sink)[0].SnapshotID == old {
		t.Fatal("snapshot ID reused")
	}
}
func TestEndgameSnapshotEmptyAndChunkEnvelope(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 0, 0)
	sink.take()
	p := s.players[1]
	s.endgamePlayer(p).loaded = true
	s.sendEndgameSnapshot(p)
	rs := endgameMessages(s, sink)
	if len(rs) != 1 || rs[0].Chunks != 1 || rs[0].Chunk != 0 || len(rs[0].Items) != 0 {
		t.Fatal(rs)
	}
	s.items = map[domain.ItemID]domain.ItemDef{123: {ID: 123, Name: "符"}}
	p.Player.Bag = domain.NewBag(100)
	for i := 0; i < 65; i++ {
		p.Player.Bag.Set(i, domain.Stack{Item: 123, UID: int64(i + 1), Count: 1})
	}
	s.sendEndgameSnapshot(p)
	rs = endgameMessages(s, sink)
	if len(rs) != 3 {
		t.Fatal(len(rs))
	}
	n := 0
	for i, r := range rs {
		if int(r.Chunk) != i || r.Chunks != 3 || r.SnapshotID != rs[0].SnapshotID || len(r.Items) > 32 {
			t.Fatal(r)
		}
		n += len(r.Items)
	}
	if n != 65 {
		t.Fatal(n)
	}
}

func TestEndgameItemRequirementsUseNativeCharacterGate(t *testing.T) {
	s := newTestScene(t, nil)
	u := domain.UniqueCatalog()[0]
	s.items = map[domain.ItemID]domain.ItemDef{
		123: {ID: 123, Name: "双法系装备", Equip: &domain.EquipDef{ResourceLevel: 99, LevelReq: 60,
			Need: domain.Requirement{Level: 45, Professions: 24, ProfessionRestricted: true}}},
		u.BaseID: {ID: u.BaseID, Name: u.BaseName, Equip: &domain.EquipDef{ResourceLevel: u.ResourceLevel, LevelReq: u.LevelRequirement,
			Need: domain.Requirement{Level: u.LevelRequirement, Professions: u.Professions, ProfessionRestricted: !u.Unrestricted}}},
		124: {ID: 124, Name: "材料"},
		125: {ID: 125, Name: "初行者装备", Equip: &domain.EquipDef{Need: domain.Requirement{
			Level: 7, ProfessionRestricted: true, BeginnerAllowed: true}}},
	}
	plain := s.endgameItem(domain.Stack{UID: 1, Item: 123, Count: 1}, "bag", 0)
	if plain.Level != 45 || plain.Profession != 24 || plain.UniqueRollBP != 0 {
		t.Fatalf("owned view lost the real gate or invented a roll: %+v", plain)
	}
	// 24 is the two native class bits (8|16), never the fourth/fifth ordinal.
	if plain.Profession != int32(1<<uint8(domain.Healer)|1<<uint8(domain.Warlock)) {
		t.Fatal("profession wire value is not the native bitmask")
	}
	st := domain.Stack{UID: 2, Item: u.BaseID, Count: 1}
	if !domain.ApplyUnique(&st, s.items[u.BaseID], func(n int) int { return n - 1 }) {
		t.Fatal("unique fixture did not awaken")
	}
	owned := s.endgameItem(st, "equipped", u.Slot)
	if owned.Level != u.LevelRequirement || owned.Level <= 0 || owned.Profession != int32(u.Professions) ||
		owned.Profession == 0 || owned.UniqueRollBP != domain.UniqueRollMaxBP {
		t.Fatalf("owned unique requirements/potency not preserved: %+v", owned)
	}
	material := s.endgameItem(domain.Stack{UID: 3, Item: 124, Count: 1}, "bag", 1)
	if material.Level != 0 || material.Profession != 0 || material.UniqueRollBP != 0 {
		t.Fatal("invented requirements for non-equipment")
	}
	beginner := s.endgameItem(domain.Stack{UID: 4, Item: 125, Count: 1}, "bag", 2)
	if beginner.Level != 7 || beginner.Profession != 0 {
		t.Fatal("zero profession mask was replaced with a made-up class")
	}
}

func TestEndgameUniqueCatalogEveryPageBoundedAndComplete(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 0, 0)
	sink.take()
	p := s.players[1]
	s.endgamePlayer(p).loaded = true
	cursor := ""
	seen := map[int32]bool{}
	for page := 0; page < 100; page++ {
		s.sendEndgameCatalog(p, domain.EndgameRequest{Op: "catalog", Catalog: "uniques", Cursor: cursor})
		rs := endgameMessages(s, sink)
		if len(rs) != 1 || !rs[0].OK {
			t.Fatal(rs)
		}
		r := rs[0]
		b, _ := json.Marshal(r)
		if len(b) > domain.EndgameMaxJSONBytes || len(r.Items) > 32 {
			t.Fatal("oversize page")
		}
		for _, x := range r.Items {
			if seen[x.UniqueID] {
				t.Fatal("duplicate")
			}
			seen[x.UniqueID] = true
			u, ok := domain.UniqueForBase(domain.ItemID(x.ItemID))
			if !ok || x.UniqueID != u.ID || x.Name != u.Name || x.Level != u.LevelRequirement ||
				x.Profession != int32(u.Professions) || x.UniqueRollBP != 10000 {
				t.Fatalf("catalog identity/requirement/reference mapping differs for %d: %+v", x.ItemID, x)
			}
			if !reflect.DeepEqual(x.Stats, endgameStats(u.Affixes)) || x.Trait != uniqueTraitDescription(u.Traits) {
				t.Fatalf("catalog values do not match the 100%% reference for %d", x.ItemID)
			}
		}
		if r.NextCursor == "" {
			break
		}
		if n, err := strconv.Atoi(r.NextCursor); err != nil || n <= (page*32) {
			t.Fatal("nonadvancing")
		}
		cursor = r.NextCursor
	}
	if len(seen) != domain.UniqueCatalogCount {
		t.Fatal(len(seen))
	}
}

package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"reflect"
	"testing"
)

func astralPetFixture(t *testing.T) (*Scene, *entity.Entity, *entity.Entity, *fakeSink, domain.ItemDef, domain.Stack, domain.ItemDef, domain.Stack) {
	t.Helper()
	base := domain.UniqueCatalog()[0].BaseID
	gd := domain.ItemDef{ID: base, Name: "普通原型", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), Durable: 30}}
	rd := domain.AstralRuneItems(7)[0]
	gear := domain.NewStack(gd, 1)
	if !domain.ApplyUnique(&gear, gd, func(int) int { return 123 }) {
		t.Fatal("unique fixture")
	}
	gear.UID = 98001
	gear.Durability = 19
	gear.DurabilityWearRaw = 17
	gear.Bound = true
	gear.Locked = true
	gear.RefineLevel = 3
	gear.SocketCount = 2
	gear.Sockets[0] = rd.ID
	gear.SocketUIDs[0] = 98002
	gear.SocketCards[0] = rd.CardDefaults
	gear.SocketCards[0].Effects[0].Value = 17
	gear.SocketCards[0].Bound = true
	gear.RolledAffixCount = 1
	gear.RolledAffixes[0] = domain.InstanceAffix{CardID: 7, Affix: domain.Affix{Attr: domain.AttrSTR, Value: 7}}
	gear.WashCount = 1
	gear.WashAffixes[0] = domain.Affix{Attr: domain.AttrCrit, Value: 99}
	gear.FusedAppearance = 601
	gear.FusedSoul = 602
	gear.FusedDragon = 603
	rune := domain.NewStack(rd, 1)
	rune.UID = 98003
	rune.Bound = true
	rune.Card.Bound = true
	rune.Card.Effects[0].Value = 23
	s := New(Config{ID: domain.SceneID{MapID: 7}, Alloc: domain.NewEntityAlloc(), SaveEvery: 10000, Items: map[domain.ItemID]domain.ItemDef{gd.ID: gd, rd.ID: rd}})
	sink := join(t, s, 1, 100, "拾宝者", 100, 100)
	owner := s.players[1]
	owner.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	pet := &entity.Entity{ID: 2, Name: "拾宝宠", Kind: domain.KindPet, Pos: owner.Pos, Pet: &entity.Pet{Owner: owner.ID}}
	astralTake(s, sink)
	return s, owner, pet, sink, gd, gear, rd, rune
}
func petSavedDrop(t *testing.T, s *Scene, owner *entity.Entity, def domain.ItemDef, st domain.Stack) *entity.Entity {
	t.Helper()
	s.spawnGeneratedLoot(def, st, owner.Pos, owner.ID)
	for _, e := range s.ground {
		if e.Drop.Stack.UID == st.UID && e.Drop.Stack.Item == st.Item {
			return e
		}
	}
	t.Fatal("drop not spawned")
	return nil
}
func TestPetTakePreservesUniqueAndRune(t *testing.T) {
	for _, kind := range []string{"unique", "rune"} {
		t.Run(kind, func(t *testing.T) {
			s, owner, pet, sink, gd, gear, rd, rune := astralPetFixture(t)
			def, st := gd, gear
			if kind == "rune" {
				def, st = rd, rune
			}
			drop := petSavedDrop(t, s, owner, def, st)
			name := itemDisplayName(def, st)
			pet.Pet.BagWarned = true
			s.petTake(pet, owner, drop)
			if _, ok := s.ground[drop.ID]; ok {
				t.Fatal("ground retained completed drop")
			}
			found := 0
			owner.Player.Bag.Each(func(_ int, got domain.Stack) {
				if got.UID == st.UID {
					found++
					if !reflect.DeepEqual(got, st) {
						t.Fatal("whole Stack changed", got, st)
					}
				}
			})
			if found != 1 {
				t.Fatal("original UID count", found)
			}
			notice, ok := firstOf[event.PetPickedUp](astralTake(s, sink))
			if !ok || notice.Name != name || notice.Item != int32(st.Item) || pet.Pet.BagWarned {
				t.Fatal("wrong pickup notice/warning", notice)
			}
		})
	}
}
func TestPetTakeFullBagLeavesRealInstance(t *testing.T) {
	for _, kind := range []string{"unique", "rune"} {
		t.Run(kind, func(t *testing.T) {
			s, owner, pet, sink, gd, gear, rd, rune := astralPetFixture(t)
			def, st := gd, gear
			if kind == "rune" {
				def, st = rd, rune
			}
			first, last := owner.Player.Bag.RangeFor(def)
			for i := first; i < last; i++ {
				owner.Player.Bag.Set(i, domain.Stack{Item: 990001, Count: 1})
			}
			before := owner.Player.Bag.Clone()
			drop := petSavedDrop(t, s, owner, def, st)
			s.petTake(pet, owner, drop)
			s.petTake(pet, owner, drop)
			if s.ground[drop.ID] != drop || drop.Drop.Count != st.Count || !reflect.DeepEqual(drop.Drop.Stack, st) || !reflect.DeepEqual(before, owner.Player.Bag) {
				t.Fatal("full bag changed/drop lost")
			}
			evs := astralTake(s, sink)
			if countOf[event.Rejected](evs) != 1 || countOf[event.PetPickedUp](evs) != 0 || !pet.Pet.BagWarned {
				t.Fatal("wrong full bag notifications")
			}
		})
	}
}
func TestPetTakePartialStackDoesNotDuplicateOrUnbind(t *testing.T) {
	s, owner, pet, sink, _, _, _, _ := astralPetFixture(t)
	def := domain.ItemDef{ID: 990010, Name: "绑定材料", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
	s.items[def.ID] = def
	original := domain.Stack{Item: def.ID, Count: 10, Bound: true}
	for i := 100; i < 200; i++ {
		owner.Player.Bag.Set(i, domain.Stack{Item: 990001, Count: 1})
	}
	owner.Player.Bag.Set(100, domain.Stack{Item: def.ID, Count: domain.MaxStack - 2, Bound: true})
	drop := petSavedDrop(t, s, owner, def, original)
	s.petTake(pet, owner, drop)
	astralTake(s, sink)
	if owner.Player.Bag.At(100).Count != domain.MaxStack || drop.Drop.Count != 8 || drop.Drop.Stack.Count != 8 || !drop.Drop.Stack.Bound || s.ground[drop.ID] != drop {
		t.Fatal("partial transfer wrong")
	}
	owner.Player.Bag.Set(101, domain.Stack{})
	s.petTake(pet, owner, drop)
	if _, ok := s.ground[drop.ID]; ok {
		t.Fatal("completed drop retained")
	}
	got := owner.Player.Bag.At(101)
	if got.Count != 8 || got.Item != def.ID || !got.Bound || owner.Player.Bag.CountOf(def.ID) != domain.MaxStack+8 {
		t.Fatal("retry duplicated/unbound", got)
	}
	if notice, ok := firstOf[event.PetPickedUp](astralTake(s, sink)); !ok || notice.Name != def.Name {
		t.Fatal("no completed notice")
	}
}
func TestPetTakeBusyGuardRetainsWholeStack(t *testing.T) {
	s, owner, pet, sink, def, st, _, _ := astralPetFixture(t)
	drop := petSavedDrop(t, s, owner, def, st)
	before := owner.Player.Bag.Clone()
	owner.Player.TradeBusy = true
	astralTake(s, sink)
	s.petTake(pet, owner, drop)
	if s.ground[drop.ID] != drop || !reflect.DeepEqual(st, drop.Drop.Stack) || !reflect.DeepEqual(before, owner.Player.Bag) || pet.Pet.BagWarned || countOf[event.PetPickedUp](astralTake(s, sink)) != 0 {
		t.Fatal("busy guard allowed mutation")
	}
}

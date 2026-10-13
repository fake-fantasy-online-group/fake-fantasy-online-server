package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"reflect"
	"sort"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func magicFindGearV5(t *testing.T, s *Scene, p *entity.Entity) (domain.EquipSlot, domain.Stack, int64) {
	t.Helper()
	for _, u := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
		var mf int64
		for _, e := range u.TypedEffects {
			if e.Kind == "magic_find_pct" {
				mf += int64(e.Value)
			}
		}
		if mf <= 0 {
			continue
		}
		def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: u.BaseID, InstanceKind: domain.ItemInstanceEquipment, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: u.Slot, Durable: 100}}
		s.items[def.ID] = def
		st := domain.NewStack(def, 1)
		if !domain.ApplyUnique(&st, def, func(int) int { return 2000 }) {
			t.Fatal("MF fixture promotion failed")
		}
		slot := domain.EquipSlot(u.Slot)
		p.Player.Worn.Set(slot, st)
		return slot, st, mf
	}
	t.Fatal("catalog has no MF equipment")
	return 0, domain.Stack{}, 0
}

func TestLootContextV5ResponsibleOwnerAndEffectiveEquipment(t *testing.T) {
	s, _ := lootScene(t, nil)
	join(t, s, 1, 100, "killer", 100, 100)
	join(t, s, 2, 200, "party member", 100, 100)
	p := s.players[1]
	slot, st, want := magicFindGearV5(t, s, p)
	_, _, _ = magicFindGearV5(t, s, s.players[2])
	for _, source := range []*entity.Entity{
		{ID: 71, Kind: domain.KindPet, Pet: &entity.Pet{Owner: p.ID}},
		{ID: 72, Kind: domain.KindTrap, Trap: &entity.Trap{Owner: p.ID}},
		{ID: 73, Kind: domain.KindMonster, Monster: &entity.Monster{Summoner: p.ID}},
		{ID: 74, Kind: domain.KindTrap, Trap: &entity.Trap{Owner: 71}},
	} {
		s.entities[source.ID] = source
	}
	for _, source := range []domain.EntityID{1, 71, 72, 73, 74} {
		context := s.freezeLootContextV5(source)
		if context.Owner != p.ID || context.MagicFind != want || context.Version != 5 {
			t.Fatalf("source%d context=%+v want MF%d once", source, context, want)
		}
	}
	frozen := s.freezeLootContextV5(72)
	broken := st
	broken.Durability = 0
	p.Player.Worn.Set(slot, broken)
	if s.freezeLootContextV5(72).MagicFind != 0 || frozen.MagicFind != want {
		t.Fatal("broken gear counted or frozen context changed")
	}
	p.Player.Worn.Set(slot, st)
	def := s.items[st.Item]
	def.Equip.Need.Level = 2
	if s.magicFindV5(p) != 0 {
		t.Fatal("under-level gear contributed MF")
	}
	def.Equip.Need.Level = 0
	def.Equip.Need.Base.STR = 1_000_000
	if s.magicFindV5(p) != 0 {
		t.Fatal("unmet attribute requirement contributed MF")
	}
	def.Equip.Need.Base.STR = 0
	def.Equip.Need.ProfessionRestricted = true
	def.Equip.Need.Professions = 16
	if s.magicFindV5(p) != 0 {
		t.Fatal("unmet class requirement contributed MF")
	}
	def.Equip.Need.ProfessionRestricted = false
	def.Equip.Need.Professions = 0
	p.Player.Worn.Set(slot, domain.Stack{})
	p.Player.Worn.Set(domain.SlotGlove, st)
	if slot != domain.SlotGlove && s.magicFindV5(p) != 0 {
		t.Fatal("wrong slot contributed MF")
	}
	delete(s.players, 1)
	delete(s.entities, 1)
	if c := s.freezeLootContextV5(72); c.Owner != 1 || c.MagicFind != 0 {
		t.Fatal("absent owner borrowed a party member's MF", c)
	}
	s.entities[75] = &entity.Entity{ID: 75, Trap: &entity.Trap{Owner: 76}}
	s.entities[76] = &entity.Entity{ID: 76, Trap: &entity.Trap{Owner: 75}}
	if c := s.freezeLootContextV5(75); c.Owner != 0 || c.MagicFind != 0 {
		t.Fatal("owner cycle did not fail closed", c)
	}
}

func magicFindRollTableV5() domain.EquipmentRollTable {
	options := []domain.EquipmentRollOption{}
	for _, attr := range []int32{domain.AttrSTR, domain.AttrINT, domain.AttrVIT, domain.AttrAGI, domain.AttrDEX} {
		options = append(options, domain.EquipmentRollOption{Affix: domain.Affix{Attr: attr, Value: 7}, Weight: 1})
	}
	return domain.EquipmentRollTable{Counts: map[domain.ColorProfile][]domain.WeightedAffixCount{1: {{Count: 0, Weight: 60}, {Count: 1, Weight: 20}, {Count: 2, Weight: 10}, {Count: 3, Weight: 5}, {Count: 4, Weight: 3}, {Count: 5, Weight: 1}}}, Options: map[domain.EquipmentRollKey][]domain.EquipmentRollOption{{}: options}}
}

func TestLootV5PetTrapAndSummonActualDeathDropsBelongToOwner(t *testing.T) {
	for _, kind := range []domain.EntityKind{domain.KindPet, domain.KindTrap, domain.KindMonster} {
		s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
		join(t, s, 1, 100, "owner", 100, 100)
		_, _, _ = magicFindGearV5(t, s, s.players[1])
		source := &entity.Entity{ID: 71, Kind: kind, HP: 100}
		switch kind {
		case domain.KindPet:
			source.Pet = &entity.Pet{Owner: 1}
			s.pets[source.ID] = source
		case domain.KindTrap:
			source.Trap = &entity.Trap{Owner: 1}
		case domain.KindMonster:
			source.Monster = &entity.Monster{Summoner: 1}
		}
		s.entities[source.ID] = source
		s.entities[mid].HP = 0
		s.onDeath(s.entities[mid], source.ID)
		if len(s.ground) != 1 {
			t.Fatal("missing death-path drop", kind)
		}
		for _, drop := range s.ground {
			if drop.Drop.Owner != 1 {
				t.Fatal("drop protection used temporary source instead of owner", kind, drop.Drop.Owner)
			}
		}
	}
}

func TestLootV5PeriodicDamageRetainsOwnerAfterSummonLeaves(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	join(t, s, 1, 100, "owner", 100, 100)
	source := &entity.Entity{ID: 71, Kind: domain.KindMonster, HP: 100, Monster: &entity.Monster{Summoner: 1}}
	s.entities[source.ID] = source
	target := s.entities[mid]
	def := domain.StatusDef{ID: 999999, Level: 1, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -10000}}}
	if !s.applyStatusDef(target, def, source.ID) {
		t.Fatal("could not apply periodic damage fixture")
	}
	delete(s.entities, source.ID)
	var active domain.Active
	target.Status.Each(func(a domain.Active) {
		if a.Def.ID == def.ID {
			active = a
		}
	})
	if active.Source != 1 {
		t.Fatal("periodic source retained ephemeral summon ID", active.Source)
	}
	s.tickStatus(target, active)
	if len(s.ground) != 1 || target.Alive() {
		t.Fatal("periodic source did not execute death/drop path")
	}
	for _, drop := range s.ground {
		if drop.Drop.Owner != 1 {
			t.Fatal("periodic kill lost stable owner", drop.Drop.Owner)
		}
	}
}

func TestLootContextV5FreezesBeforeKillLevelUp(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 999002, RatePct: 100, RollAffixes: true}})
	join(t, s, 1, 100, "killer", 100, 100)
	p := s.players[1]
	_, gear, mf := magicFindGearV5(t, s, p)
	s.items[gear.Item].Equip.Need.Level = 2
	s.levels = domain.NewLevelTable(map[int32]int64{1: 1, 2: 10000, 3: 20000})
	def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999002, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon)}}
	s.items[def.ID] = def
	s.equipmentRoll = magicFindRollTableV5()
	var expected domain.Stack
	found := false
	for seed := int64(1); seed < 10000; seed++ {
		s.lootSeed = seed
		a, b := domain.NewStack(def, 1), domain.NewStack(def, 1)
		s.rollNaturalEquipmentV5(def, &a, s.entities[mid], 0, 1, true, domain.LootContextV5{})
		s.rollNaturalEquipmentV5(def, &b, s.entities[mid], 0, 1, true, domain.LootContextV5{MagicFind: mf})
		if a.RolledAffixCount != b.RolledAffixCount {
			expected, found = a, true
			break
		}
	}
	if !found {
		t.Fatal("could not find distinguishing deterministic MF vector")
	}
	s.entities[mid].HP = 0
	s.onDeath(s.entities[mid], p.ID)
	if p.Player.Char.Level != 2 || s.magicFindV5(p) != mf {
		t.Fatal("kill did not activate level-gated fixture", p.Player.Char.Level)
	}
	if len(s.ground) != 1 {
		t.Fatal("missing actual death-path drop")
	}
	for _, drop := range s.ground {
		st := drop.Drop.Stack
		if st.RolledAffixCount != expected.RolledAffixCount || st.RolledAffixes != expected.RolledAffixes {
			t.Fatal("kill XP retroactively changed loot MF", st, expected)
		}
	}
}

func TestLootV5MFDoesNotChangeItemsSocketsRunesOrBossGuarantee(t *testing.T) {
	makeScene := func() (*Scene, domain.EntityID) {
		s, mid := lootScene(t, []domain.DropEntry{
			{Item: 999001, RatePct: 100, RollAffixes: true},
			{Choices: []domain.ItemID{999001, 999002}, RatePct: 60, RollAffixes: true},
			{Item: 掉落树枝.ID, RatePct: 60},
		})
		for _, id := range []domain.ItemID{999001, 999002} {
			s.items[id] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), LevelReq: 50}}
		}
		for _, rune := range astralRuneItemsFixture(1) {
			s.items[rune.ID] = rune
		}
		md := 必掉的怪
		md.Level = 60
		s.defs = spawn.MapDefs{md.ID: md}
		s.entities[mid].Monster.Kind = domain.MonsterBoss
		s.equipmentRoll = magicFindRollTableV5()
		s.bossFixedDrops = domain.BossFixedDropTable{{MapID: s.id.MapID, Monster: md.ID}: {Minimum: 2, Pool: []domain.BossFixedDropChoice{{Item: 999001, Weight: 1, RollAffixes: true}, {Item: 999002, Weight: 1, RollAffixes: false}}}}
		return s, mid
	}
	a, aid := makeScene()
	b, bid := makeScene()
	changed := false
	sawRune := false
	for i := 0; i < 400; i++ {
		a.tick, b.tick = domain.Tick(i), domain.Tick(i)
		a.rollLootWithContextV5(a.entities[aid], domain.LootContextV5{MagicFind: 0})
		b.rollLootWithContextV5(b.entities[bid], domain.LootContextV5{MagicFind: 800})
		x, y := sortedLootStacksV5(a), sortedLootStacksV5(b)
		if len(x) != len(y) || len(x) < 2 {
			t.Fatal("MF changed count or boss guarantee", len(x), len(y))
		}
		for j := range x {
			if x[j].Item != y[j].Item || x[j].Count != y[j].Count || x[j].SocketCount != y[j].SocketCount {
				t.Fatal("MF changed item/socket/rune stream", i, x[j], y[j])
			}
			if x[j].Item >= domain.FirstAstralRune && x[j].Item <= domain.LastAstralRune {
				sawRune = true
			}
			if x[j].RolledAffixCount != y[j].RolledAffixCount {
				changed = true
			}
			if j < 2 && x[j].Item == 999002 && (x[j].RolledAffixCount != 0 || y[j].RolledAffixCount != 0) {
				t.Fatal("fixed RollAffixes=false was ignored")
			}
			if int(x[j].SocketCount)+int(x[j].RolledAffixCount) > 5 || int(y[j].SocketCount)+int(y[j].RolledAffixCount) > 5 {
				t.Fatal("exceeded attribute quota")
			}
		}
		// Only ground state is relevant here; keep the fixtures bounded.
		for id := range a.ground {
			delete(a.entities, id)
			a.aoi.Leave(a.ground[id])
		}
		for id := range b.ground {
			delete(b.entities, id)
			b.aoi.Leave(b.ground[id])
		}
		a.ground = map[domain.EntityID]*entity.Entity{}
		b.ground = map[domain.EntityID]*entity.Entity{}
		a.outbox = nil
		b.outbox = nil
	}
	if !changed || !sawRune {
		t.Fatal("fixture did not exercise MF quality and rune drops", changed, sawRune)
	}
}

func sortedLootStacksV5(s *Scene) []domain.Stack {
	ids := make([]domain.EntityID, 0, len(s.ground))
	for id := range s.ground {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]domain.Stack, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.ground[id].Drop.Stack)
	}
	return out
}

func TestLootV5RNGDomainsAndPickupDoNotReroll(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 999002, RatePct: 100, RollAffixes: true}})
	join(t, s, 1, 100, "killer", 120, 100)
	p := s.players[1]
	_, _, _ = magicFindGearV5(t, s, p)
	s.items[999002] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999002, InstanceKind: domain.ItemInstanceEquipment, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon)}}
	s.equipmentRoll = magicFindRollTableV5()
	quality := s.lootRandomV5(mid, 0, 0x56350001)
	before := s.lootRandomV5(mid, 0, 0x56350003)
	for i := 0; i < 1000; i++ {
		quality.Intn(100)
		s.rng.Intn(100)
	}
	after := s.lootRandomV5(mid, 0, 0x56350003)
	for i := 0; i < 20; i++ {
		if before.Intn(100000) != after.Intn(100000) {
			t.Fatal("unrelated draws disturbed unique stream")
		}
	}
	s.rollLoot(s.entities[mid], p.ID)
	for id, drop := range s.ground {
		expected := drop.Drop.Stack
		p.Player.Worn = domain.NewEquipSet()
		p.Pos = drop.Pos
		s.onPickUp(PickUp{ID: p.ID, Target: id})
		if _, exists := s.ground[id]; exists {
			t.Fatal("could not pick up frozen loot")
		}
		found := false
		p.Player.Bag.Each(func(_ int, st domain.Stack) {
			if st.UID == expected.UID {
				found = reflect.DeepEqual(st, expected)
			}
		})
		if !found {
			t.Fatal("pickup/gear swap changed the frozen instance")
		}
	}
}

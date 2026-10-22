package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestDungeonEquipmentSourceGateAndRiftExclusion(t *testing.T) {
	const base domain.ItemID = 1321
	hardID := domain.HardDungeonItemOffset + base
	original := domain.ItemDef{ID: base, Name: "任意改名", Equip: &domain.EquipDef{}}
	hard := domain.ItemDef{ID: hardID, Name: "任意改名2", HardDungeonOnly: true, DungeonBaseID: base, Equip: &domain.EquipDef{}}
	dungeon := domain.DungeonDef{ID: 20701}
	s := &Scene{id: domain.SceneID{MapID: 20701, Instance: 1}, dungeon: &dungeon, dungeons: domain.DungeonTable{20701: dungeon}, items: map[domain.ItemID]domain.ItemDef{base: original, hardID: hard}, dungeonDifficulty: domain.DungeonHard}
	def, ok := s.dungeonLootItem(base, 3049)
	if !ok || def.ID != hardID {
		t.Fatal("hard source not mapped", def.ID, ok)
	}
	for _, mon := range []domain.MonsterID{1, 3055} {
		def, ok = s.dungeonLootItem(base, mon)
		if !ok || def.ID != base {
			t.Fatal("unrelated monster changed")
		}
	}
	if _, ok = s.dungeonLootItem(hardID, 3049); ok {
		t.Fatal("direct hard ID accepted")
	}
	s.dungeonDifficulty = domain.DungeonNormal
	def, ok = s.dungeonLootItem(base, 3049)
	if !ok || def.ID != base {
		t.Fatal("normal changed")
	}
	s.dungeonDifficulty = domain.DungeonHard
	s.rift = &endgameRiftState{}
	def, ok = s.dungeonLootItem(base, 3049)
	if !ok || def.ID != base {
		t.Fatal("rift upgraded original")
	}
	if _, ok = s.dungeonLootItem(hardID, 3049); ok {
		t.Fatal("rift inherited hard drop")
	}
	// The final spawn helper also refuses a hard item, before touching any allocator.
	live, _ := lootScene(t, nil)
	live.rift = &endgameRiftState{}
	live.spawnGeneratedLoot(hard, domain.NewStack(hard, 1), domain.Pos{MapID: 14, X: 100, Y: 100}, 0)
	if len(live.ground) != 0 {
		t.Fatal("rift spawned hard item")
	}
}
func TestOrdinarySetDoesNotEnableEnhancedSkills(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 8)
	if s.classSetPieces(p, 5) != 8 {
		t.Fatal("enhanced fixture")
	}
	suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
	suit.EnhancedClass = 0
	if s.classSetPieces(p, 5) != 0 || s.classSetDragonActive(p) {
		t.Fatal("ordinary suit retained enhanced skills")
	}
}

func TestDungeonEquipmentCannotLeakThroughRiftExplicitOrSpecialReward(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	hardID := domain.HardDungeonItemOffset + 1321
	var hard domain.ItemDef
	for _, def := range s.items {
		if def.Equip != nil {
			hard = def
			break
		}
	}
	hard.ID = hardID
	hard.HardDungeonOnly = true
	s.items[hardID] = hard
	r := s.rift.data.Run
	r.Tier = 10
	r.Timely = true
	r.Rules.NormalBossEquipment = 10
	// Intentionally bypass startup validation to model a stale/malformed saved run.
	r.Rules.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []domain.MonsterID{3049}, Items: []domain.ItemID{hardID}}}
	r.Rules.DustItem = hardID
	for char, m := range r.Members {
		m.Eligible = true
		m.Level = 60
		r.Members[char] = m
	}
	if st := s.rollEndgameRiftNormalBossEquipment(100, 0, []domain.ItemID{hardID}, 0); !st.Empty() {
		t.Fatal("boss extra accepted hard item")
	}
	s.generateEndgameRiftGrants()
	if !s.rift.data.GrantsReady {
		t.Fatal("grant generation did not run")
	}
	for _, grant := range s.rift.data.Grants {
		for _, st := range grant.Items {
			if st.Item == hardID {
				t.Fatal("hard item leaked through explicit/dust reward")
			}
		}
	}
}

func TestDungeonAncientSupplementDoesNotDuplicateOrLeak(t *testing.T) {
	dungeon := domain.DungeonDef{ID: 109}
	s := &Scene{id: domain.SceneID{MapID: 20901, Instance: 1}, dungeon: &dungeon, dungeons: domain.DungeonTable{20901: dungeon}, items: map[domain.ItemID]domain.ItemDef{}}
	original := []domain.DropEntry{{Item: 1326, RatePct: 0.12}, {Item: 1626, RatePct: 0.12}, {Item: 1826, RatePct: 0.12}, {Item: 1928, RatePct: 0.12}}
	got := s.withLostTempleAncientDrops(original, 3111)
	if len(got) != 12 || len(original) != 4 || len(s.withLostTempleAncientDrops(got, 3111)) != 12 {
		t.Fatal("duplicate/modified pool")
	}
	for _, e := range got {
		if e.RatePct != 0.12 {
			t.Fatal("old or new rate changed", e)
		}
		hard := domain.HardDungeonItemOffset + e.Item
		s.items[e.Item] = domain.ItemDef{ID: e.Item}
		s.items[hard] = domain.ItemDef{ID: hard, HardDungeonOnly: true, DungeonBaseID: e.Item}
		for _, difficulty := range []domain.DungeonDifficulty{domain.DungeonNormal, domain.DungeonHard} {
			s.dungeonDifficulty = difficulty
			def, ok := s.dungeonLootItem(e.Item, 3111)
			want := e.Item
			if difficulty == domain.DungeonHard {
				want = hard
			}
			if !ok || def.ID != want {
				t.Fatal("version mismatch", e.Item, difficulty)
			}
		}
	}
	s.rift = &endgameRiftState{}
	if len(s.withLostTempleAncientDrops(original, 3111)) != 4 {
		t.Fatal("rift inherited additions")
	}
	s.rift = nil
	s.id.MapID = 23800
	s.dungeons[23800] = dungeon
	if len(s.withLostTempleAncientDrops(original, 3111)) != 4 {
		t.Fatal("other native map inherited additions")
	}
	s.id.MapID = 20901
	s.id.Instance = 0
	if len(s.withLostTempleAncientDrops(original, 3111)) != 4 {
		t.Fatal("public map inherited additions")
	}
}

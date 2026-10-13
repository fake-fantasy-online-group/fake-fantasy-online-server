package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestUniqueGroundPresentationAllMapsAndSnapshots(t *testing.T) {
	u := domain.UniqueCatalog(catalogfixture.EndgameCatalog())[0]
	st := domain.Stack{UID: 999, Item: u.BaseID, Count: 1, Endgame: domain.EndgameItemState{UniqueID: u.ID, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}}
	if (&Scene{endgameCatalog: catalogfixture.EndgameCatalog()}).groundUniqueID(st) != u.ID {
		t.Fatal("valid unique identity missing")
	}
	ordinary := st
	ordinary.Endgame = domain.EndgameItemState{}
	if (&Scene{endgameCatalog: catalogfixture.EndgameCatalog()}).groundUniqueID(ordinary) != 0 {
		t.Fatal("base ID misclassified as unique")
	}
	yellow := ordinary
	yellow.RolledAffixCount = 5
	yellowDef := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: yellow.Item, Equip: &domain.EquipDef{}}
	if equipmentQuality(yellowDef, yellow) != 4 || (&Scene{endgameCatalog: catalogfixture.EndgameCatalog()}).groundUniqueID(yellow) != 0 {
		t.Fatal("ordinary five-affix yellow item must never be unique")
	}
	r := domain.AstralRecipes(catalogfixture.EndgameCatalog())[0]
	fd := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999001, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(r.Slots[0]), LevelReq: 100, Durable: 10}}
	formation := domain.NewStack(fd, 1)
	formation.Endgame.FormationContentVersion = domain.UniqueContentVersion
	formation.SocketCount = uint8(len(r.Runes))
	for i, id := range r.Runes {
		formation.Sockets[i], formation.SocketUIDs[i] = id, int64(100+i)
		formation.SocketCards[i].Initialized = true
	}
	if _, ok := domain.MatchAstralRecipe(fd, formation); !ok {
		t.Fatal("invalid formation fixture")
	}
	if (&Scene{endgameCatalog: catalogfixture.EndgameCatalog()}).groundUniqueID(formation) != 0 {
		t.Fatal("real runeword misclassified as unique")
	}
	malformed := st
	malformed.Endgame.UniqueRollBP = 0
	if (&Scene{endgameCatalog: catalogfixture.EndgameCatalog()}).groundUniqueID(malformed) != 0 {
		t.Fatal("malformed unique marker accepted")
	}
	var previous int64
	for _, mapID := range []int32{14, 920, 20001} {
		s := New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: mapID}})
		e := &entity.Entity{ID: 12, Kind: domain.KindDrop, Drop: &entity.Drop{Item: st.Item, Count: 1, Stack: st}}
		fresh := s.freshGroundSpawnEvent(e)
		if fresh.Ground == nil || !fresh.Ground.FreshDrop || fresh.Ground.UniqueID != u.ID || fresh.Ground.DropToken <= previous {
			t.Fatal("fresh identity incomplete", fresh.Ground)
		}
		previous = fresh.Ground.DropToken
		snapshot := s.spawnEvent(e)
		if snapshot.Ground.FreshDrop || snapshot.Ground.DropToken != previous || snapshot.Ground.UniqueID != u.ID {
			t.Fatal("snapshot audio edge")
		}
		if s.freshGroundSpawnEvent(e).Ground.FreshDrop {
			t.Fatal("re-emission created second fresh edge")
		}
	}
}

func TestNativeLootCreationEmitsFreshGroundOnce(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	killMonster(t, s, mid)
	ev, ok := firstOf[event.EntitySpawned](sink.take())
	if !ok || ev.Ground == nil || !ev.Ground.FreshDrop || ev.Ground.DropToken <= 0 || ev.Ground.UniqueID != 0 {
		t.Fatal("ordinary creation missing fresh lifetime", ev)
	}
	for _, e := range s.ground {
		if s.spawnEvent(e).Ground.FreshDrop {
			t.Fatal("late observer snapshot is fresh")
		}
	}
}

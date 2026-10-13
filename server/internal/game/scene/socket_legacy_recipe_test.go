package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Representative shipped ov_enchase row 45: 10 fire copper, 1000 coin,
// 80% success, one additional hole. Cube recipes must not replace this route.
func TestNativeSocketFormulaRemainsIncremental(t *testing.T) {
	for _, seed := range []int64{1, 3, 8, 14} {
		s, _, _ := cubeSceneFixture(t)
		p := s.players[1]
		def := s.items[1001]
		def.Equip.Type = 1
		def.Equip.Tier = 99
		s.items[1001] = def
		s.items[4006] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 4006, Name: "火铜", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
		p.Player.Char.Skills = domain.Learned{11002: 1}
		p.Player.Bag.Set(102, domain.Stack{Item: 4006, Count: 20})
		recipe := domain.SocketRecipe{RowNo: 45, Hole: 0, EquipClass: 1, EquipTier: 99, DrillCost: 1000, InlayCost: 1000, RequiredSkill: 11002, ProtectItem: 4478, SuccessRate: 80, Materials: []domain.CraftMaterial{{Item: 4006, Qty: 10}}}
		s.sockets = domain.SocketTable{[3]int32{0, 1, 99}: recipe}
		info := s.socketInfo(p, socketTargetRef{kind: 0, tab: 2, bagSlot: 200})
		if !info.CanDrill || info.NextCost != 1000 || info.NextSuccess != 80 || len(info.Materials) != 1 || info.Materials[0].Item != 4006 || info.Materials[0].Need != 10 {
			t.Fatal("native formula changed", info)
		}
		rng := rand.New(rand.NewSource(seed))
		success := rng.Intn(100) < 80
		s.rng = rand.New(rand.NewSource(seed))
		before := p.Player.Bag.At(200)
		s.onDrillItem(DrillItem{ID: p.ID, TargetKind: 0, Tab: 2, BagSlot: 200, Execute: true})
		after := p.Player.Bag.At(200)
		want := uint8(0)
		if success {
			want = 1
		}
		if after.SocketCount != want || after.UID != before.UID || p.Player.Bag.CountOf(4006) != 10 || p.Player.Char.Money != moneyFromInventory(499000) {
			t.Fatal("native single-hole chance or consumption changed")
		}
		if p.Player.Bag.CountOf(domain.CubeStarMarrow) != 500 || p.Player.Bag.CountOf(domain.CubePurifiedDust) != 500 {
			t.Fatal("native route consumed new cube materials")
		}
	}
}

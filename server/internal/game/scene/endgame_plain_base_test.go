package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestAstralInnateAffixEquipRelogAndCombat(t *testing.T) {
	for _, affixed := range []bool{false, true} {
		s, _, _ := astralSceneFixture(t)
		p := s.players[1]
		def := s.items[999001]
		if affixed {
			def.Equip.Affixes = []domain.Affix{{Attr: domain.AttrSTR, Value: 7}}
			s.items[def.ID] = def
		}
		recipe, _ := domain.AstralRecipeByID(1)
		target := p.Player.Bag.At(200)
		target.Bound = true
		target.SocketCount = uint8(len(recipe.Runes))
		target.RefineLevel = 3
		for i, id := range recipe.Runes {
			target.Sockets[i] = id
			target.SocketUIDs[i] = int64(501 + i)
			target.SocketCards[i] = s.items[id].CardDefaults
		}
		p.Player.Bag.Set(200, target)
		s.onEquip(Equip{ID: p.ID, BagSlot: 200})
		if p.Player.Worn.At(domain.SlotWeapon) != target {
			t.Fatal("plain-base gating unexpectedly removed valid equipment")
		}
		view := s.endgameItem(target, "worn", int32(domain.SlotWeapon))
		if (view.FormationID != 0) == affixed {
			t.Fatalf("snapshot formation affixed=%v: %+v", affixed, view)
		}
		if (len(domain.ActiveAstralRecipes(s.functionalWorn(p), s.itemDef, p.Player.Char)) > 0) == affixed {
			t.Fatal("equip activation inconsistent")
		}
		ch := *p.Player.Char
		ch.ID = 102
		loaded := newTestScene(t, nil)
		loaded.items = s.items
		loaded.exec(Enter{ID: 2, Char: &ch, Worn: p.Player.Worn.Clone(), Bag: domain.NewBag(domain.DefaultBagSlots), Sink: &fakeSink{}})
		actual := loaded.players[2]
		if actual.Player.Worn.At(domain.SlotWeapon) != target || actual.Stats != p.Stats {
			t.Fatal("relog lost identity/runes or changed combat stats")
		}
		if (len(domain.ActiveAstralRecipes(loaded.functionalWorn(actual), loaded.itemDef, actual.Player.Char)) > 0) == affixed {
			t.Fatal("relog formation eligibility differs")
		}
		// Reversed rune order is a no-formation oracle with identical standalone effects.
		reverse := target
		reverse.Sockets[0], reverse.Sockets[1] = reverse.Sockets[1], reverse.Sockets[0]
		reverse.SocketUIDs[0], reverse.SocketUIDs[1] = reverse.SocketUIDs[1], reverse.SocketUIDs[0]
		reverse.SocketCards[0], reverse.SocketCards[1] = reverse.SocketCards[1], reverse.SocketCards[0]
		actual.Player.Worn.Set(domain.SlotWeapon, reverse)
		loaded.refreshStats(actual)
		if affixed && actual.Stats != p.Stats {
			t.Fatal("innate-affixed base gained hidden formation combat benefit")
		}
		if !affixed && actual.Stats == p.Stats {
			t.Fatal("plain upgraded base lost formation benefit")
		}
	}
}

package scene

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Follow the same completed item across the native equip command, persisted-worn
// recovery, combat sources, native tooltip and sidecar snapshot at the boundary.
func TestAstralAllFormationLevelGatesAndViews(t *testing.T) {
	for _, recipe := range domain.AstralRecipes() {
		maxRune := int32(0)
		for _, id := range recipe.Runes {
			rune, _ := domain.AstralRune(id)
			maxRune = max(maxRune, rune.MinLevel)
		}
		if recipe.MinLevel != maxRune {
			t.Fatalf("recipe %d has independent level %d, want %d", recipe.ID, recipe.MinLevel, maxRune)
		}
		for _, baseLevel := range []int32{0, maxRune + 3} {
			required := max(baseLevel, maxRune)
			for _, level := range []int32{required - 1, required} {
				t.Run(fmt.Sprintf("recipe%d/base%d/player%d", recipe.ID, baseLevel, level), func(t *testing.T) {
					s, sink, _ := astralSceneFixture(t)
					p := s.players[1]
					p.Player.Char.Level = level
					p.Player.Char.EmploymentKnown, p.Player.Char.Employed = true, true
					for race := domain.Warrior; race <= domain.Warlock; race++ {
						if recipe.Professions == 0 || recipe.Professions&(1<<uint8(race)) != 0 {
							p.Player.Char.Race = race
							break
						}
					}
					slot := recipe.Slots[0]
					def := s.items[999001]
					def.Equip.Slot, def.Equip.Need.Level = int32(slot), baseLevel
					def.Equip.Bonus.MaxHP = 100
					s.items[def.ID] = def
					st := domain.NewStack(def, 1)
					st.SocketCount = uint8(len(recipe.Runes))
					for i, id := range recipe.Runes {
						rune := domain.NewStack(s.items[id], 1)
						st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = id, rune.UID, rune.Card
					}
					p.Player.Bag, p.Player.Worn = domain.NewBag(domain.DefaultBagSlots), domain.NewEquipSet()
					p.Player.Bag.Set(200, st)
					ready := level >= required
					if got := domain.EquipmentRequiredLevel(def, st); got != required {
						t.Fatalf("required %d want %d", got, required)
					}
					if s.equipmentBlockedFor(p, def, st) == ready {
						t.Fatal("native blocked flag disagrees with level")
					}
					tip := s.ownedItemTooltip(def, st, p)
					requirement := fmt.Sprintf("需要等级: %d", required)
					if strings.Count(tip, requirement) != 1 || strings.Contains(tip, "\x04"+requirement) == ready {
						t.Fatalf("native requirement/color mismatch: %q", tip)
					}
					view := s.endgameItem(st, "bag", 0)
					if view.Level != required || view.FormationID != recipe.ID {
						t.Fatalf("bag view mismatch: %+v", view)
					}
					s.refreshStats(p)
					naked := p.Stats
					astralTake(s, sink)
					s.onEquip(Equip{ID: p.ID, BagSlot: 200})
					if got := p.Player.Worn.At(slot).UID == st.UID; got != ready {
						t.Fatal("native equip boundary mismatch")
					}
					if !ready {
						rejected := false
						for _, ev := range astralTake(s, sink) {
							if r, ok := ev.(event.Rejected); ok && r.Reason == event.RejectLevelTooLow {
								rejected = true
							}
						}
						if !rejected || p.Player.Bag.At(200) != st {
							t.Fatal("underlevel equip was not rejected atomically")
						}
						// Simulate an item loaded from persisted worn state. It must be inert,
						// without destroying its saved rune identities or mutating its sockets.
						p.Player.Bag.Set(200, domain.Stack{})
						p.Player.Worn.Set(slot, st)
					}
					s.refreshStats(p)
					functional := s.functionalWorn(p)
					if got := functional.At(slot).UID == st.UID; got != ready {
						t.Fatal("persisted-worn functional boundary mismatch")
					}
					active := domain.ActiveAstralRecipes(functional, s.itemDef, p.Player.Char)
					if (len(active) == 1) != ready {
						t.Fatal("formation activation disagrees with equip")
					}
					if (!ready && p.Stats != naked) || (ready && p.Stats == naked) {
						t.Fatal("combat stats disagree with level gate")
					}
					traits := s.uniqueTraits(p)
					if (!ready && len(traits) != 0) || (ready && len(traits) != len(recipe.Traits)) {
						t.Fatal("combat trait gate mismatch")
					}
					astralTake(s, sink)
					s.sendEndgameSnapshot(p)
					responses := endgameMessages(s, sink)
					if len(responses) != 1 || len(responses[0].Items) != 1 {
						t.Fatalf("snapshot shape: %+v", responses)
					}
					item := responses[0].Items[0]
					if item.Level != required || item.Active != ready || item.FormationID != recipe.ID {
						t.Fatalf("equipped view mismatch: %+v", item)
					}
					if !ready && item.InactiveReason != requirement {
						t.Fatalf("wrong inactive reason: %s", item.InactiveReason)
					}
					if ready && (len(responses[0].Formations) != 1 || responses[0].Formations[0].MinLevel != maxRune) {
						t.Fatal("active recipe wire level differs from ingredients")
					}
					saved := p.Player.Worn.At(slot)
					if saved.Sockets != st.Sockets || saved.SocketUIDs != st.SocketUIDs || saved.SocketCards != st.SocketCards {
						t.Fatal("level evaluation rewrote saved runes")
					}
				})
			}
		}
	}
}

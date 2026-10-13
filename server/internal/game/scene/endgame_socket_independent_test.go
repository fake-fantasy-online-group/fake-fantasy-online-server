package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Exercise real native one-card commits and refreshStats after every insertion.
// The expected standalone contribution is built directly from saved card effects,
// with no formation matcher or equipment socket-stat reader in that oracle.
func TestAstralNativeEachRuneWorksBeforeFinalFormation(t *testing.T) {
	fixture, _, _ := astralSceneFixture(t)
	chosen := map[int]domain.AstralRecipe{}
	for _, recipe := range domain.AstralRecipes(fixture.endgameCatalog) {
		if recipe.Professions == 0 || recipe.Professions&(1<<uint8(domain.Warrior)) != 0 {
			if _, ok := chosen[len(recipe.Runes)]; !ok {
				chosen[len(recipe.Runes)] = recipe
			}
		}
	}
	for count := 2; count <= 5; count++ {
		recipe, ok := chosen[count]
		if !ok {
			t.Fatal("missing formation length", count)
		}
		for _, mode := range []string{"valid", "reversed", "innate_affix", "wrong_part", "wrong_profession", "extra_hole"} {
			if (mode == "innate_affix" || mode == "extra_hole") && count == 5 {
				continue
			}
			if mode == "wrong_profession" && recipe.Professions == 0 {
				continue
			}
			t.Run(fmt.Sprintf("%d/%s", count, mode), func(t *testing.T) {
				s, _, backend := astralSceneFixture(t)
				p := s.players[1]
				p.Player.Char.Level = 100
				p.Player.Char.EmploymentKnown = true
				p.Player.Char.Employed = true
				p.Player.Char.Base = domain.Base{STR: 200, VIT: 200, INT: 200, SPI: 200, AGI: 200, DEX: 200}
				def := s.items[999001]
				equip := *def.Equip
				equip.Slot = int32(recipe.Slots[0])
				equip.Need.Level = 10
				equip.Bonus = domain.Stats{}
				def.Equip = &equip
				if mode == "innate_affix" {
					def.Equip.Affixes = []domain.Affix{{Attr: domain.AttrVIT, Value: 7}}
				}
				if mode == "wrong_part" {
					for _, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotHead, domain.SlotBody} {
						allowed := false
						for _, x := range recipe.Slots {
							if slot == x {
								allowed = true
							}
						}
						if !allowed {
							def.Equip.Slot = int32(slot)
							break
						}
					}
					if def.Equip.Slot == int32(recipe.Slots[0]) {
						t.Fatal("wrong part fixture")
					}
				}
				if mode == "wrong_profession" {
					for race := domain.Warrior; race <= domain.Warlock; race++ {
						if recipe.Professions&(1<<uint8(race)) == 0 {
							p.Player.Char.Race = race
							break
						}
					}
					if recipe.Professions&(1<<uint8(p.Player.Char.Race)) != 0 {
						t.Skip("formation supports all professions")
					}
				}
				s.items[def.ID] = def
				gear := p.Player.Bag.At(200)
				gear.SocketCount = uint8(count)
				if mode == "extra_hole" {
					gear.SocketCount++
				}
				p.Player.Bag.Set(200, domain.Stack{})
				p.Player.Worn = domain.NewEquipSet()
				slot := domain.EquipSlot(def.Equip.Slot)
				p.Player.Worn.Set(slot, gear)
				ids := append([]domain.ItemID(nil), recipe.Runes...)
				if mode == "reversed" {
					ids[0], ids[1] = ids[1], ids[0]
				}
				for i, id := range ids {
					r := domain.NewStack(s.items[id], 1)
					r.UID = int64(1000 + i)
					p.Player.Bag.Set(100+i, r)
				}
				s.refreshStats(p)
				baseline := p.Stats
				s.exec(DrillItem{ID: p.ID, TargetKind: 1, EquipSlot: uint8(slot)})
				var runeAffixes []domain.Affix
				maxLevel := def.Equip.Need.Level
				for i, id := range ids {
					rune := p.Player.Bag.At(100 + i)
					runeAffixes = append(runeAffixes, rune.Card.StaticAffixes()...)
					rd, _ := domain.AstralRune(id, s.endgameCatalog)
					if rd.MinLevel > maxLevel {
						maxLevel = rd.MinLevel
					}
					cmd := InlayItem{ID: p.ID, TargetKind: 1, EquipSlot: uint8(slot), Hole: uint8(i), RuneTab: 1, RuneBagSlot: int32(100 + i)}
					s.exec(cmd)
					s.finishEndgame(false)
					if backend.commits != i+1 {
						t.Fatalf("individual card %d was rejected", i+1)
					}
					nativeAstralSuccess(s, backend)
					st := p.Player.Worn.At(slot)
					if got := domain.EquipmentRequiredLevel(def, st); got != maxLevel {
						t.Fatalf("step%d max level got%d want%d", i+1, got, maxLevel)
					}
					active := domain.ActiveAstralRecipes(p.Player.Worn, s.itemDef, p.Player.Char)
					wantFormation := mode == "valid" && i+1 == count
					if (len(active) > 0) != wantFormation {
						t.Fatalf("step%d array activation got%v want%v", i+1, active, wantFormation)
					}
					passive := append([]domain.Affix(nil), def.Equip.Affixes...)
					passive = append(passive, runeAffixes...)
					if wantFormation {
						passive = append(passive, domain.AstralItemBonuses(def, st, active[0].Recipe)...)
					}
					_, want := domain.Compute(domain.StatSource{Base: p.Player.Char.EffectiveBase(), Level: p.Player.Char.Level, Innate: characterInnateStats(p.Player.Char), Passive: passive})
					if p.Stats != want {
						t.Fatalf("step%d individual rune stats lost/formation extras leaked: got%+v want%+v", i+1, p.Stats, want)
					}
					if i == 0 && p.Stats == baseline {
						t.Fatal("first card did not independently affect real equipment stats")
					}
				}
			})
		}
	}
}

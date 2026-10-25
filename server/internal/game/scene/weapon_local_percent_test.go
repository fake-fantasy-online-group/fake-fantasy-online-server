package scene

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestWeaponAttackPercentNativeEquipPanelCombatAndReload(t *testing.T) {
	for _, magic := range []bool{false, true} {
		name := "physical"
		if magic {
			name = "magic"
		}
		t.Run(name, func(t *testing.T) {
			weapon := domain.ItemDef{ID: 99001, Name: "局部武器", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{
				Slot: int32(domain.SlotWeapon), Durable: 100,
				Bonus:         domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100},
				Affixes:       []domain.Affix{{Attr: domain.AttrAtk, Value: 50, Mode: domain.ModePercent}, {Attr: domain.AttrMAtk, Value: 75, Mode: domain.ModePercent}},
				RefineEffects: map[int32]domain.RefineEffect{1: {Bonuses: []domain.Affix{{Attr: domain.AttrAtk, Value: 7}, {Attr: domain.AttrMAtk, Value: 9}}}},
			}}
			armor := domain.ItemDef{ID: 99002, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), Bonus: domain.Stats{MinAtk: 40, MaxAtk: 40, MAtk: 60}}}
			items := map[domain.ItemID]domain.ItemDef{weapon.ID: weapon, armor.ID: armor}
			spell := 火弹术
			spell.Effect.DamagePct, spell.MPCost = 100, 1
			cfg := Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10000, Seed: 12345, Items: items, Skills: skillTable(spell)}
			s := New(cfg)
			ch := &domain.Character{ID: 100, Name: "甲", Level: 100, Race: domain.Warlock, Employed: true, EmploymentKnown: true,
				Base: domain.Base{STR: 200, INT: 400, VIT: 20, SPI: 100, DEX: 1 << 21},
				Pos:  domain.Pos{MapID: 7, X: 100, Y: 100}, Skills: domain.Learned{spell.ID: 1}}
			bag := domain.NewBag(400)
			st := domain.Stack{UID: 1, Item: weapon.ID, Count: 1, RefineLevel: 1, Durability: 100, MaxDurability: 100}
			bag.Set(200, st)
			bag.Set(201, domain.Stack{UID: 2, Item: armor.ID, Count: 1})
			sink := &fakeSink{}
			s.exec(Enter{ID: 1, Char: ch, Bag: bag, Worn: domain.NewEquipSet(), Sink: sink})
			s.exec(Equip{ID: 1, BagSlot: 201})
			s.exec(Equip{ID: 1, BagSlot: 200})
			s.step()
			p := s.players[1]
			assertStats := func(wantPhysical, wantMagic int32) {
				t.Helper()
				if p.Stats.MinAtk != wantPhysical || p.Stats.MaxAtk != wantPhysical || p.Stats.MAtk != wantMagic {
					t.Fatalf("server stats got %+v, want %d/%d", p.Stats, wantPhysical, wantMagic)
				}
				panel := s.attributeSnapshot(p)
				for _, values := range [][]int32{panel.Current, panel.Total} {
					if values[7] != wantPhysical || values[34] != wantPhysical || values[15] != wantMagic {
						t.Fatal("panel diverged from combat", values)
					}
				}
			}
			assertStats(297, 444) // own base * local %, plus independent refine/armor/character
			// Keep the existing tooltip contract: unmodified base+refine and a
			// separate authored percentage line, rather than folding it twice.
			tip := s.ownedItemTooltip(weapon, p.Player.Worn.At(domain.SlotWeapon), p)
			if !strings.Contains(tip, "攻击 107~107") || !strings.Contains(tip, "魔法攻击 109") || !strings.Contains(tip, affixText(weapon.Equip.Affixes[0])) || !strings.Contains(tip, affixText(weapon.Equip.Affixes[1])) {
				t.Fatal("tooltip numeric interpretation changed", tip)
			}
			sink.take()
			monster := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			if magic {
				s.exec(UseSkill{ID: p.ID, Skill: spell.ID, Target: monster.ID})
			} else {
				s.exec(Attack{ID: p.ID, Target: monster.ID})
			}
			var hit event.DamageDealt
			found := false
			for i := 0; i < 20 && !found; i++ {
				s.step()
				hit, found = firstOf[event.DamageDealt](sink.take())
			}
			wantDamage := int32(297)
			if magic {
				wantDamage = 444
			}
			if !found || hit.Amount != wantDamage || hit.Flag.Has(event.DamageMagic) != magic || monster.HP != 10000-wantDamage {
				t.Fatalf("native combat did not consume local stats: hit %+v, hp %d, want %d", hit, monster.HP, wantDamage)
			}
			before := p.Player.Worn.At(domain.SlotWeapon)
			s.exec(Unequip{ID: p.ID, Slot: domain.SlotWeapon})
			assertStats(140, 260)
			weaponSlot := -1
			p.Player.Bag.Each(func(slot int, item domain.Stack) {
				if item.UID == before.UID {
					weaponSlot = slot
				}
			})
			if weaponSlot < 0 {
				t.Fatal("unequip lost weapon")
			}
			s.exec(Equip{ID: p.ID, BagSlot: weaponSlot})
			assertStats(297, 444)
			if p.Player.Worn.At(domain.SlotWeapon) != before {
				t.Fatal("re-equip mutated percentages")
			}
			persisted, err := json.Marshal(before)
			if err != nil {
				t.Fatal(err)
			}
			var restored domain.Stack
			if err := json.Unmarshal(persisted, &restored); err != nil {
				t.Fatal(err)
			}
			reloadedWorn := domain.NewEquipSet()
			reloadedWorn.Set(domain.SlotWeapon, restored)
			reloadedWorn.Set(domain.SlotBody, p.Player.Worn.At(domain.SlotBody))
			reloadedChar := *ch
			s = New(cfg)
			s.exec(Enter{ID: 1, Char: &reloadedChar, Bag: domain.NewBag(400), Worn: reloadedWorn, Sink: &fakeSink{}})
			p = s.players[1]
			assertStats(297, 444)
			if p.Player.Worn.At(domain.SlotWeapon) != before {
				t.Fatal("entry rewrote saved weapon")
			}
		})
	}
}

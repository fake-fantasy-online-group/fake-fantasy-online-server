package scene

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func namedEquipmentTooltipFixture(t *testing.T) (map[domain.ItemID]domain.ItemDef, []domain.DungeonEquipmentWeapon) {
	t.Helper()
	raw, err := os.ReadFile("../../domain/dungeon_equipment_manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Weapons []domain.DungeonEquipmentWeapon
		Sets    []domain.DungeonEquipmentSet
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	items := map[domain.ItemID]domain.ItemDef{}
	for _, w := range manifest.Weapons {
		items[w.BaseID] = domain.ItemDef{ID: w.BaseID, Name: w.Name, Description: "旧器的原有来历。", Price: 100,
			InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: nativePresentationEquip(int32(domain.SlotWeapon))}
	}
	for _, set := range manifest.Sets {
		suit := &domain.EquipmentSuit{ID: set.BaseID, Name: "原套装"}
		for _, id := range set.Members {
			suit.Groups = append(suit.Groups, []domain.ItemID{id})
			items[id] = domain.ItemDef{ID: id, Name: "套装部件", Equip: &domain.EquipDef{Suit: suit}}
		}
	}
	if err := domain.RegisterDungeonEquipment(items); err != nil {
		t.Fatal(err)
	}
	return items, manifest.Weapons
}

func assertNamedEffectRows(t *testing.T, tip string, effects []string) {
	t.Helper()
	if strings.Count(tip, "\x06") != len(effects) {
		t.Fatalf("wrong number of orange effects in %q", tip)
	}
	for _, effect := range effects {
		row := "\x06" + effect
		if strings.Count(tip, row) != 1 || !strings.Contains(effect, "：") || strings.Contains(tip, "威能名称") {
			t.Fatalf("missing, duplicated or unnamed effect %q in %q", row, tip)
		}
		for _, metadata := range []string{"孔数", "旧器的原有来历。", "（绑定物品不可交易"} {
			if idx := strings.Index(tip, metadata); idx >= 0 && strings.Index(tip, row) >= idx {
				t.Fatalf("effect is after %s instead of closing attributes: %q", metadata, tip)
			}
		}
	}
}

func TestNamedEquipmentTooltipEveryTierAndInertOriginal(t *testing.T) {
	items, weapons := namedEquipmentTooltipFixture(t)
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	for id, def := range items {
		s.items[id] = def
	}
	active := 0
	for _, w := range weapons {
		ids := []domain.ItemID{w.BaseID}
		if w.HardID != 0 {
			ids = append(ids, w.HardID)
		}
		for _, id := range ids {
			def := items[id]
			effects := domain.DungeonWeaponEffectLines(id)
			if len(effects) > 0 {
				active++
			}
			for _, native := range []bool{false, true} {
				for _, durability := range []int32{0, 100} {
					t.Run(fmt.Sprintf("%d/native=%t/durability=%d", id, native, durability), func(t *testing.T) {
						p.Player.EndgameNativeStats = native
						st := domain.NewStack(def, 1)
						st.UID, st.Bound, st.Durability, st.SocketCount, st.RefineLevel = 123, true, durability, 1, 3
						before := st
						tip := s.ownedItemTooltip(def, st, p)
						assertNamedEffectRows(t, tip, effects)
						if strings.Count(tip, def.Description) != 1 || st != before || strings.TrimPrefix(itemDisplayName(def, st), "\x05") != def.Name+"+3" {
							t.Fatalf("lore, instance or refined name changed on %d", id)
						}
						if tip != s.ownedItemTooltip(def, st, p) {
							t.Fatal("repeat tooltip is not stable")
						}
					})
				}
			}
		}
	}
	if active != 48 {
		t.Fatalf("got %d active weapons, want 48", active)
	}
}

func TestNamedEquipmentTooltipReachesInventoryEquipPreviewShopAndGround(t *testing.T) {
	items, _ := namedEquipmentTooltipFixture(t)
	s, sink := equipScene(t)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(10)
	var fixture []map[string]interface{}
	for _, id := range []domain.ItemID{1021, 1800001023, 1800001027, 1800001323, 1019, 1023} {
		def := items[id]
		s.items[id] = def
		st := domain.NewStack(def, 1)
		st.UID = int64(id)
		p.Player.Bag.Set(0, st)
		p.Player.Worn.Set(domain.SlotWeapon, st)
		snap, err := s.inventorySnapshot(p)
		if err != nil || len(snap.Items) != 1 || len(snap.Equipped) != 1 {
			t.Fatal(snap, err)
		}
		tip := snap.Items[0].Desc
		assertNamedEffectRows(t, tip, domain.DungeonWeaponEffectLines(id))
		if tip != snap.Equipped[0].Desc || tip != s.itemTooltip(def, st) || tip != s.itemTooltipForViewer(def, st, p, nil) {
			t.Fatalf("bag/equipped/preview format differs for %d", id)
		}
		packets := protocol.Encode(p.ID, snap)
		if len(packets) != 1 || !bytes.Contains(packets[0], protocol.NewW(0).Str(tip).Bytes()[2:]) {
			t.Fatal("tooltip changed on native wire")
		}
		s.shops = domain.ShopTable{1: {ID: 1, Items: []domain.ItemID{id}}}
		s.entities[300] = &entity.Entity{ID: 300, Kind: domain.KindNPC, NPC: &entity.NPCInfo{Sell: 1}}
		s.onOpenShop(OpenShop{ID: p.ID, Shop: 1})
		s.flush()
		shop, ok := firstOf[event.ShopItems](sink.take())
		if !ok || len(shop.Items) != 1 || shop.Items[0].Desc != tip || shop.Items[0].Name != def.Name {
			t.Fatal("shop tooltip/name differs", shop)
		}
		drop := &entity.Entity{ID: 600, Kind: domain.KindDrop, Name: itemDisplayName(def, st), Drop: &entity.Drop{Item: id, Count: 1, Stack: st}}
		ground := s.spawnEvent(drop)
		if ground.Ground == nil || ground.Name != snap.Items[0].Name || ground.Ground.Quality != snap.Items[0].Quality || ground.Ground.UniqueID != 0 {
			t.Fatal("ground name/quality/identity changed", ground)
		}
		fixture = append(fixture, map[string]interface{}{"itemId": id, "name": def.Name, "quality": snap.Items[0].Quality, "tooltip": tip, "snapshot": s.endgameItem(st, "bag", 0)})
	}
	if path := os.Getenv("NAMED_EQUIPMENT_TOOLTIP_FIXTURE_OUT"); path != "" {
		raw, err := json.MarshalIndent(map[string]interface{}{"source": "Production tooltip/inventory/sidecar outputs from deterministic synthetic native templates; not a Unity screenshot or live capture.", "items": fixture}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNamedEquipmentEffectNamesMatchExistingNativeSkills(t *testing.T) {
	_, weapons := namedEquipmentTooltipFixture(t)
	skills, _ := realV5SkillFixtures(t)
	raw, err := os.ReadFile("testdata/special_weapon_approved_branches.json")
	if err != nil {
		t.Fatal(err)
	}
	var branches []struct {
		Item       domain.ItemID
		Profession int32
		Skills     []domain.SkillID
	}
	if err := json.Unmarshal(raw, &branches); err != nil {
		t.Fatal(err)
	}
	for _, w := range weapons {
		for class, name := range w.EffectNames {
			found := false
			for _, branch := range branches {
				if branch.Item != w.BaseID || class == "药师" && branch.Profession != 4 || class == "术士" && branch.Profession != 5 {
					continue
				}
				for _, id := range branch.Skills {
					if skill, ok := skills.Get(id, 1); ok && skill.Name == name {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("%d/%s uses unverified skill name %q", w.BaseID, class, name)
			}
		}
	}
}

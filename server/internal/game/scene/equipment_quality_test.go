package scene

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func TestEquipmentQualityPaletteKeepsAttributeRanks(t *testing.T) {
	for n, want := range []uint8{0, 2, 6, 3, 1, 4, 4} {
		for _, source := range []string{"fixed", "rolled", "socket"} {
			if n > 5 && source != "fixed" {
				continue
			}
			def := domain.ItemDef{Equip: &domain.EquipDef{}}
			var st domain.Stack
			switch source {
			case "fixed":
				def.Equip.Affixes = make([]domain.Affix, n)
			case "rolled":
				st.RolledAffixCount = uint8(n)
			case "socket":
				st.SocketCount = uint8(n)
			}
			if got := equipmentQuality(def, st); got != want {
				t.Fatalf("%s attributes=%d quality=%d, want %d", source, n, got, want)
			}
		}
	}
	if got := equipmentQuality(domain.ItemDef{}, domain.Stack{RolledAffixCount: 2}); got != 0 {
		t.Fatalf("non-equipment acquired a quality: %d", got)
	}
}

func TestTwoAttributeQualityReachesNativeViewsAndWire(t *testing.T) {
	s, sink := equipScene(t)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(10)
	var refs []ChatShareRef
	var defs []domain.ItemDef
	for i, count := range []int{1, 2} {
		affixes := []domain.Affix{{Attr: domain.AttrSTR, Value: 7}, {Attr: domain.AttrVIT, Value: 9}}
		def := domain.ItemDef{ID: domain.ItemID(990001 + i), Name: fmt.Sprintf("品质样本%d", count),
			InstanceKind: domain.ItemInstanceEquipment, InventoryTab: 2, InventoryTabKnown: true, Price: 100,
			Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), Durable: 100, Affixes: affixes[:count]}}
		st := domain.NewStack(def, 1)
		st.UID = int64(900001 + i)
		s.items[def.ID] = def
		p.Player.Bag.Set(i, st)
		refs = append(refs, ChatShareRef{Tab: 2, Slot: int32(i)})
		defs = append(defs, def)
	}
	p.Player.Worn.Set(domain.SlotWeapon, p.Player.Bag.At(1))
	snap, err := s.inventorySnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Items) != 2 || len(snap.Equipped) != 1 || snap.Equipped[0].Quality != 6 {
		t.Fatalf("bad inventory/equipment quality: %+v", snap)
	}
	packets := protocol.Encode(p.ID, snap)
	if len(packets) != 1 || binary.LittleEndian.Uint16(packets[0]) != protocol.SCInventory {
		t.Fatal("inventory failed real encoding")
	}
	packet := packets[0]
	for i, want := range []uint8{2, 6} {
		item := snap.Items[i]
		prefix := "\x032"
		if i == 1 {
			prefix = "\x05"
		}
		if item.Quality != want || !strings.Contains(item.Desc, prefix+"力量 +7") || strings.Contains(item.Desc, "\x035") {
			t.Fatalf("item %d palette changed: %+v", i, item)
		}
		// Actual desc field is followed by sell/durability/avatar and blocked/bound.
		field := protocol.NewW(0).Str(item.Desc).Bytes()[2:]
		start := bytes.Index(packet, field)
		qualityOffset := start + len(field) + 18
		if start < 0 || qualityOffset >= len(packet) || packet[qualityOffset] != want {
			t.Fatalf("inventory item %d quality truncated on wire", i)
		}
		st := p.Player.Bag.At(i)
		drop := &entity.Entity{ID: domain.EntityID(101 + i), Kind: domain.KindDrop, Name: item.Name,
			Drop: &entity.Drop{Item: st.Item, Count: 1, Stack: st}}
		spawn := s.spawnEvent(drop)
		groundPackets := protocol.Encode(p.ID, spawn)
		if spawn.Ground.Quality != want || len(groundPackets) == 0 || groundPackets[0][len(groundPackets[0])-1] != want {
			t.Fatal("ground quality differs from inventory", spawn.Ground)
		}
	}
	// Equipped quality extension precedes only the empty fused-description count.
	if !bytes.HasSuffix(packet, []byte{1, 6, 0, 0}) {
		t.Fatal("equipped quality extension truncated")
	}
	reply := make(chan ChatShareResult, 1)
	s.onResolveChatShares(ResolveChatShares{ID: p.ID, Refs: refs, Reply: reply})
	shares := <-reply
	if shares.Reason != "" || len(shares.Shares) != 2 {
		t.Fatal(shares)
	}
	for i, share := range shares.Shares {
		item := snap.Items[i]
		if share.Quality != item.Quality || share.Desc != item.Desc || share.Name != item.Name {
			t.Fatal("chat differs from inventory", share)
		}
		wire := protocol.ChannelChat(1, "合成样本", 0, 0, "装备", protocol.ChatShareView{ItemID: int32(share.Item), Quality: share.Quality, Name: share.Name, Desc: share.Desc})
		field := protocol.NewW(0).Str(share.Name).Str(share.Desc).Bytes()[2:]
		start := bytes.Index(wire, field)
		if start <= 0 || wire[start-1] != item.Quality {
			t.Fatal("chat quality truncated on wire")
		}
	}
	p.Player.Stall = &domain.Stall{Items: []domain.StallItem{{Stack: p.Player.Bag.At(1), UnitPrice: 100}}}
	stall := s.stallContents(p, p)
	if len(stall.Rows) != 1 || stall.Rows[0].Quality != 6 || stall.Rows[0].Desc != snap.Items[1].Desc {
		t.Fatal("stall differs from inventory", stall)
	}
	// NPC shops have no quality field: preserve structure and shared description.
	s.shops = domain.ShopTable{1: {ID: 1, Items: []domain.ItemID{defs[1].ID}}}
	s.entities[300] = &entity.Entity{ID: 300, Kind: domain.KindNPC, NPC: &entity.NPCInfo{Sell: 1}}
	s.onOpenShop(OpenShop{ID: p.ID, Shop: 1})
	s.flush()
	shop, ok := firstOf[event.ShopItems](sink.take())
	if !ok || len(shop.Items) != 1 || shop.Items[0].Desc != snap.Items[1].Desc || len(protocol.Encode(p.ID, shop)) != 1 {
		t.Fatal("NPC shop lost shared light-blue description", shop)
	}
	if dir := os.Getenv("EQUIPMENT_QUALITY_FIXTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		fixture, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		for name, content := range map[string][]byte{"inventory.packet.bin": packet, "inventory.payload.bin": packet[2:], "inventory.json": fixture} {
			if err := os.WriteFile(filepath.Join(dir, name), content, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

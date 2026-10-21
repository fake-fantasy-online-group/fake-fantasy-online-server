package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestPlayerExperienceItemFriendlyMinimumLevel(t *testing.T) {
	for _, item := range []struct {
		id         domain.ItemID
		minimum    int32
		experience int64
	}{{12090, 12, 5881}, {12091, 12, 7842}, {12092, 12, 9802}, {25355, 53, 1604613}} {
		for _, level := range []int32{10, item.minimum - 1, item.minimum, item.minimum + 1, item.minimum + 3} {
			t.Run(fmt.Sprintf("item%d/level%d", item.id, level), func(t *testing.T) {
				s := newTestScene(t, nil)
				sink := join(t, s, 1, 100, "experience", 100, 100)
				p := s.players[1]
				p.Player.Char.Level = level
				p.Player.Char.Exp = 7
				s.levels = domain.NewLevelTable(map[int32]int64{level: 100000000})
				def := domain.ItemDef{ID: item.id, InventoryTab: 1, InventoryTabKnown: true, Stackable: true, CooldownSec: 10, CooldownGroup: 123, PlayerExperience: map[int32]int64{item.minimum: item.experience, item.minimum + 2: item.experience}}
				s.items = map[domain.ItemID]domain.ItemDef{def.ID: def}
				if p.Player.Bag.Add(def, 2) != 0 {
					t.Fatal("add item")
				}
				cubeTake(s, sink)
				s.onUseItem(UseItem{ID: 1, Item: def.ID, Tab: 0xff, BagSlot: -1})
				notices := []event.ServerNotice{}
				for _, e := range cubeTake(s, sink) {
					if n, ok := e.(event.ServerNotice); ok {
						notices = append(notices, n)
					}
				}
				if level == item.minimum {
					if p.Player.Char.Exp != 7+item.experience || p.Player.Bag.CountOf(def.ID) != 1 || p.Player.ItemReadyAt(123, s.tick) {
						t.Fatal("minimum-level use no longer grants/consumes/starts cooldown")
					}
					if len(notices) != 0 {
						t.Fatalf("unexpected notices: %+v", notices)
					}
					return
				}
				want := "该物品暂不可使用"
				if level < item.minimum {
					want = fmt.Sprintf("%d级及以上可使用", item.minimum)
				}
				if len(notices) != 1 || notices[0].Text != want {
					t.Fatalf("notices %+v, want %q", notices, want)
				}
				if p.Player.Char.Exp != 7 || p.Player.Char.Level != level || p.Player.Bag.CountOf(def.ID) != 2 || !p.Player.ItemReadyAt(123, s.tick) {
					t.Fatal("rejected use changed XP, level, inventory, or cooldown")
				}
			})
		}
	}
}
func TestPlainPeachFriendlyUnavailableMessage(t *testing.T) {
	for _, id := range []domain.ItemID{3634, 999991} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			s := newTestScene(t, nil)
			sink := join(t, s, 1, 100, "peach", 100, 100)
			p := s.players[1]
			p.Player.Char.Level = 10
			p.Player.Char.Exp = 7
			reason := "原客户端脚本主体未随静态数据提供"
			def := domain.ItemDef{ID: id, InventoryTab: 1, InventoryTabKnown: true, Stackable: true, CooldownSec: 10, CooldownGroup: 123, UseDisabledReason: reason}
			s.items = map[domain.ItemID]domain.ItemDef{id: def}
			if p.Player.Bag.Add(def, 2) != 0 {
				t.Fatal("add item")
			}
			cubeTake(s, sink)
			s.onUseItem(UseItem{ID: 1, Item: id, Tab: 0xff, BagSlot: -1})
			notice, ok := firstOf[event.ServerNotice](cubeTake(s, sink))
			want := reason
			if id == 3634 {
				want = "该物品暂不可使用"
			}
			if !ok || notice.Text != want {
				t.Fatalf("notice %+v, want %q", notice, want)
			}
			if p.Player.Char.Exp != 7 || p.Player.Char.Level != 10 || p.Player.Bag.CountOf(id) != 2 || !p.Player.ItemReadyAt(123, s.tick) {
				t.Fatal("rejected use changed state")
			}
		})
	}
}

package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestAstralEveryRuneEquipUseAndRelogRequirements(t *testing.T) {
	for _, r := range domain.AstralRunes(catalogfixture.EndgameCatalog()) {
		t.Run(r.Name, func(t *testing.T) {
			s, sink, _ := astralSceneFixture(t)
			p := s.players[1]
			target := p.Player.Bag.At(200)
			target.Sockets[0] = r.Item
			target.SocketUIDs[0] = 51
			target.SocketCards[0] = s.items[r.Item].CardDefaults
			target.InstanceKind = domain.ItemInstanceNone // Legacy store hydration remains supported.
			for _, level := range []int32{11, 12, 59, 60, r.MinLevel - 1, r.MinLevel} {
				p.Player.Char.Level = level
				p.Player.Worn = domain.NewEquipSet()
				p.Player.Bag.Set(200, target)
				s.onEquip(Equip{ID: p.ID, BagSlot: 200})
				if got := !p.Player.Worn.At(domain.SlotWeapon).Empty(); got != (level >= r.MinLevel) {
					t.Fatalf("equip level%d need%d got%v", level, r.MinLevel, got)
				}
				if level < r.MinLevel && p.Player.Bag.At(200) != target {
					t.Fatal("rejected equip mutated gear")
				}
				if got := s.equipmentBlockedFor(p, s.items[target.Item], target); got != (level < r.MinLevel) {
					t.Fatal("native blocked mismatch")
				}
				if v := s.endgameItem(target, "bag", 0); v.Level != r.MinLevel {
					t.Fatal("snapshot requirement")
				}
				if tip := s.ownedItemTooltip(s.items[target.Item], target, p); !strings.Contains(tip, fmt.Sprintf("需要等级: %d", r.MinLevel)) {
					t.Fatal("native gear requirement missing", tip)
				}
			}
			card := domain.NewStack(s.items[r.Item], 1)
			p.Player.Bag.Set(100, card)
			p.Player.Char.Level = r.MinLevel - 1
			astralTake(s, sink)
			s.onUseItem(UseItem{ID: p.ID, Item: r.Item, Tab: 1, BagSlot: 100})
			if rejected, ok := firstOf[event.Rejected](astralTake(s, sink)); !ok || rejected.Reason != event.RejectLevelTooLow {
				t.Fatal("native use bypass")
			}
			if p.Player.Bag.At(100) != card {
				t.Fatal("use consumed card")
			}
			if tip := s.ownedItemTooltip(s.items[r.Item], card, p); !strings.Contains(tip, fmt.Sprintf("需要等级: %d", r.MinLevel)) {
				t.Fatal("native rune requirement missing")
			}

			// Exercise the real Enter/relogin path with an already persisted worn item.
			worn := domain.NewEquipSet()
			worn.Set(domain.SlotWeapon, target)
			ch := *p.Player.Char
			ch.Level = r.MinLevel - 1
			ch.ID = 102
			low := newTestScene(t, nil)
			low.items = s.items
			low.exec(Enter{ID: 2, Char: &ch, Worn: worn.Clone(), Bag: domain.NewBag(domain.DefaultBagSlots), Sink: &fakeSink{}})
			loaded := low.players[2]
			if loaded.Player.Worn.At(domain.SlotWeapon) != target || !low.functionalWorn(loaded).At(domain.SlotWeapon).Empty() {
				t.Fatal("relogin lost identity or activated low-level rune")
			}
			without := *loaded.Player.Char
			bare := newTestScene(t, nil)
			bare.items = s.items
			bare.exec(Enter{ID: 3, Char: &without, Worn: domain.NewEquipSet(), Bag: domain.NewBag(domain.DefaultBagSlots), Sink: &fakeSink{}})
			if loaded.Stats != bare.players[3].Stats {
				t.Fatal("relogin allowed underlevel combat stats")
			}
			loaded.Player.Char.Level = r.MinLevel
			low.refreshStats(loaded)
			if low.functionalWorn(loaded).At(domain.SlotWeapon).UID != target.UID {
				t.Fatal("level-up did not restore saved gear")
			}
		})
	}
}

func TestAstralSocketSceneBoundaryHasNoCommitOnRejection(t *testing.T) {
	for _, tc := range []struct {
		id    domain.ItemID
		level int32
		want  bool
	}{{domain.FirstAstralRune, 11, false}, {domain.FirstAstralRune, 12, true}, {domain.LastAstralRune, 59, false}, {domain.LastAstralRune, 60, true}} {
		s, _, backend := astralSceneFixture(t)
		p := s.players[1]
		p.Player.Char.Level = tc.level
		card := domain.NewStack(s.items[tc.id], 1)
		card.UID = 51
		p.Player.Bag.Set(100, card)
		before := p.Player.Bag.Clone()
		s.onEndgameSocket(p, domain.EndgameRequest{Op: "socket.insert", RequestID: "requirements-test-0001", ExpectedRevision: "0", TargetUID: "42", RuneUID: "51", Socket: 0})
		if (backend.commits == 1) != tc.want || !reflect.DeepEqual(before, p.Player.Bag) {
			t.Fatalf("level%d commit%d want%v", tc.level, backend.commits, tc.want)
		}
	}
}

func TestAstralCatalogAndChannelReasonsAreAuthoritative(t *testing.T) {
	s, sink, _ := astralSceneFixture(t)
	p := s.players[1]
	s.sendEndgameCatalog(p, domain.EndgameRequest{Catalog: "runes"})
	rows := 0
	for _, ev := range astralTake(s, sink) {
		if message, ok := ev.(event.EndgameMessage); ok {
			for _, r := range message.Message.Runes {
				def, _ := domain.AstralRune(domain.ItemID(r.ItemID), catalogfixture.EndgameCatalog())
				if r.MinLevel != def.MinLevel {
					t.Fatal("catalog missing level")
				}
				rows++
			}
		}
	}
	if rows != 28 {
		t.Fatal("missing rune catalog rows", rows)
	}
	recipe, _ := domain.AstralRecipeByID(3, catalogfixture.EndgameCatalog()) // Universal guard array allows head and body.
	makeGear := func(id domain.ItemID, slot domain.EquipSlot) domain.Stack {
		def := s.items[999001]
		eq := *def.Equip
		eq.Slot = int32(slot)
		def.Equip = &eq
		def.ID = id
		s.items[id] = def
		st := domain.NewStack(def, 1)
		st.SocketCount = 2
		for i, id := range recipe.Runes {
			card := domain.NewStack(s.items[id], 1)
			st.Sockets[i] = id
			st.SocketUIDs[i] = card.UID
			st.SocketCards[i] = card.Card
		}
		return st
	}
	head := makeGear(999002, domain.SlotHead)
	body := makeGear(999003, domain.SlotBody)
	p.Player.Worn = domain.NewEquipSet()
	p.Player.Worn.Set(domain.SlotHead, head)
	p.Player.Worn.Set(domain.SlotBody, body)
	s.sendEndgameSnapshot(p)
	active, suppressed := 0, 0
	for _, ev := range astralTake(s, sink) {
		if message, ok := ev.(event.EndgameMessage); ok {
			for _, v := range message.Message.Items {
				if v.FormationID != recipe.ID {
					continue
				}
				if v.Active {
					active++
				} else if v.InactiveReason == "同类宿阵优先级未满足" {
					suppressed++
				}
			}
		}
	}
	if active != 1 || suppressed != 1 {
		t.Fatalf("active%d suppressed%d", active, suppressed)
	}
	p.Player.Char.Level = recipe.MinLevel - 1
	s.sendEndgameSnapshot(p)
	for _, ev := range astralTake(s, sink) {
		if message, ok := ev.(event.EndgameMessage); ok {
			for _, v := range message.Message.Items {
				if v.FormationID == recipe.ID && (v.Active || !strings.HasPrefix(v.InactiveReason, "需要等级:")) {
					t.Fatal("underlevel misreported as channel suppression", v)
				}
			}
		}
	}
}

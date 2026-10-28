package scene

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func powerBookSceneFixture(t *testing.T) (*Scene, *fakeSink, *cubeTestBackend) {
	t.Helper()
	s, sink, b := cubeSceneFixture(t)
	s.players[1].Player.EndgamePowerCodex = true
	d := s.items[1001]
	d.LegendaryEnabled = true
	d.Equip.LevelReq = 100
	d.Equip.Slot = int32(domain.SlotWeapon)
	s.items[1001] = d
	st := s.players[1].Player.Bag.At(200)
	st.Legendary.PowerID = "TY-A01"
	for i, a := range []int32{domain.AttrSTR, domain.AttrINT, domain.AttrVIT, domain.AttrAGI, domain.AttrDEX, domain.AttrSPI} {
		st.Legendary.Affixes[i] = domain.InstanceAffix{Affix: domain.Affix{Attr: a, Value: 3}}
		st.Legendary.Bounds[i] = domain.LegendaryAffixBounds{Min: 2, Max: 4}
	}
	s.players[1].Player.Bag.Set(200, st)
	dbook := domain.ItemDef{ID: domain.PowerCodexItemID, Name: "万象秘典", InventoryTabKnown: true}
	dscroll := domain.ItemDef{ID: domain.PowerScrollItemID, Name: "威能卷轴", InventoryTabKnown: true}
	s.items[dbook.ID] = dbook
	s.items[dscroll.ID] = dscroll
	s.players[1].Player.Char.Quests.Finish(domain.PowerCodexQuestID)
	s.players[1].Player.Bag.Set(5, domain.Stack{Item: dbook.ID, UID: 91, Count: 1, Bound: true})
	return s, sink, b
}

func TestPowerBookCubeSalvageAtomicAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			s, sink, b := powerBookSceneFixture(t)
			p := s.players[1]
			s.endgamePlayer(p).state.Payload = `{"other":{"preserve":true}}`
			before := p.Player.Bag.Clone()
			s.onEndgameCube(p, domain.EndgameRequest{Op: "cube.quote", RequestID: "salvage-power-preview", RecipeID: domain.CubeSalvageUnique, TargetUID: "42", ExpectedRevision: "0"})
			ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if !ok || ev.Message.Quote == nil {
				t.Fatal("salvage quote", ev)
			}
			if book, _ := domain.ReadLegendaryPowerBook(s.endgamePlayer(p).state.Payload); len(book) != 0 {
				t.Fatal("quote recorded power")
			}
			s.onEndgameCube(p, cubeExecute(*ev.Message.Quote))
			if b.commits != 1 {
				t.Fatal("missing atomic commit", cubeTake(s, sink))
			}
			book, err := domain.ReadLegendaryPowerBook(b.plan.State.Payload)
			if err != nil || len(book) != 1 {
				t.Fatal("record not in same transaction", book, err)
			}
			if !b.plan.Snapshot.Bag.At(200).Empty() || !reflect.DeepEqual(before, p.Player.Bag) {
				t.Fatal("inventory not isolated until commit")
			}
			if current, _ := domain.ReadLegendaryPowerBook(s.endgamePlayer(p).state.Payload); len(current) != 0 {
				t.Fatal("live book mutated before commit")
			}
			if fail {
				b.result <- domain.EndgameCommitResult{Err: errors.New("injected storage failure")}
			} else {
				b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			}
			s.finishEndgame(false)
			current, err := domain.ReadLegendaryPowerBook(s.endgamePlayer(p).state.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if fail {
				if len(current) != 0 || !reflect.DeepEqual(before, p.Player.Bag) {
					t.Fatal("failed commit consumed item or recorded power")
				}
			} else {
				if len(current) != 1 || !p.Player.Bag.At(200).Empty() {
					t.Fatal("successful transaction not applied")
				}
			}
		})
	}
}

func TestPowerBookImprintReceiptAndRevision(t *testing.T) {
	s, sink, b := powerBookSceneFixture(t)
	p := s.players[1]
	payload, err := domain.WriteLegendaryPowerBook("{}", domain.LegendaryPowerBook{"TY-A02": 0})
	if err != nil {
		t.Fatal(err)
	}
	s.endgamePlayer(p).state.Payload = payload
	p.Player.Bag.Set(6, domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 10000}})
	req := domain.EndgameRequest{Op: "powerbook.apply", RequestID: "imprint-power-0001", ExpectedRevision: "0", TargetUID: "42", RuneUID: "92"}
	before := p.Player.Bag.At(200)
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("imprint commit missing", cubeTake(s, sink))
	}
	if b.plan.Snapshot.Bag.At(200).Legendary.PowerID != "TY-A02" || p.Player.Bag.At(200) != before {
		t.Fatal("imprint not isolated")
	}
	if b.plan.State.Payload != payload {
		t.Fatal("imprint consumed book")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	after := p.Player.Bag.At(200)
	expected := before
	expected.Legendary.PowerID = "TY-A02"
	if domain.LegendaryPowerHasRoll("TY-A02") {
		expected.Legendary.PowerRollBP = 10000
	}
	expected.Endgame.Revision++
	if after != expected {
		t.Fatal("imprint changed unrelated facts", after, expected)
	}
	b.receipt = &b.plan.Receipt
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 || p.Player.Bag.At(200) != after {
		t.Fatal("replay mutated twice")
	}
	req.RequestID = "imprint-power-0002"
	b.receipt = nil
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("stale revision accepted")
	}
}

func TestPowerBookPagesAllRecordsAndPacketBudget(t *testing.T) {
	s, sink, _ := powerBookSceneFixture(t)
	p := s.players[1]
	book := domain.LegendaryPowerBook{}
	for _, power := range domain.LegendaryPowers() {
		book[power.ID] = 0
	}
	payload, err := domain.WriteLegendaryPowerBook(`{"keep":7}`, book)
	if err != nil {
		t.Fatal(err)
	}
	s.endgamePlayer(p).state.Payload = payload
	seen := map[string]bool{}
	maxPageBytes, pages := 0, 0
	cursor := ""
	for {
		s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", Cursor: cursor})
		ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if !ok || !ev.Message.OK {
			t.Fatal("page", ev)
		}
		raw, err := json.Marshal(ev.Message)
		if err != nil || len(raw) > domain.EndgameMaxJSONBytes {
			t.Fatal("oversized power page", len(raw), err)
		}
		pages++
		if len(raw) > maxPageBytes {
			maxPageBytes = len(raw)
		}
		if len(ev.Message.Powers) > powerBookPageSize {
			t.Fatal("page size")
		}
		for _, power := range ev.Message.Powers {
			if seen[power.PowerID] {
				t.Fatal("duplicate page entry")
			}
			seen[power.PowerID] = true
		}
		if ev.Message.NextCursor == "" {
			break
		}
		cursor = ev.Message.NextCursor
	}
	t.Logf("%d powers across %d pages, largest JSON page %d bytes (budget %d)", len(seen), pages, maxPageBytes, domain.EndgameMaxJSONBytes)
	if len(seen) != len(book) {
		t.Fatal("records omitted", len(seen), len(book))
	}
	for _, cursor := range []string{"-1", "01", "999", "bad"} {
		s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list", Cursor: cursor})
		ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if ev.Message.OK || ev.Message.Error != "cursor" {
			t.Fatal("bad cursor accepted", cursor)
		}
	}
}

func TestPowerBookOnlyActualSalvageRecordsAndCorruptionBlocks(t *testing.T) {
	s, _, _ := powerBookSceneFixture(t)
	bag := s.players[1].Player.Bag
	plan := domain.CubePlan{Recipe: domain.EndgameRecipeView{Operation: "wing.extract"}, ConsumedUIDs: []int64{42}}
	if got, err := powerBookAfterCube("{}", bag, plan); err != nil || got != "{}" {
		t.Fatal("wing extraction recorded as salvage")
	}
	plan.Recipe.Operation = "salvage.unique"
	plan.ConsumedUIDs = []int64{43}
	if got, err := powerBookAfterCube("{}", bag, plan); err != nil || got != "{}" {
		t.Fatal("unconsumed gear recorded")
	}
	plan.ConsumedUIDs = []int64{42}
	if _, err := powerBookAfterCube(`{"powerBook":null}`, bag, plan); err == nil {
		t.Fatal("corrupt book silently replaced")
	}
}

func TestPowerBookAndRiftProgressDoNotOverwriteEachOther(t *testing.T) {
	book := domain.LegendaryPowerBook{"TY-A01": 8123, "SS-U06": 0}
	payload, err := domain.WriteLegendaryPowerBook("{}", book)
	if err != nil {
		t.Fatal(err)
	}
	payload = withRiftProfile(payload, endgameRiftProfile{UnlockedTier: 42})
	loaded, err := domain.ReadLegendaryPowerBook(payload)
	if err != nil || !reflect.DeepEqual(loaded, book) || riftProfile(payload).UnlockedTier != 42 {
		t.Fatal("rift progress overwrote book", loaded, err)
	}
	book["TY-A01"] = 9900
	payload, err = domain.WriteLegendaryPowerBook(payload, book)
	if err != nil || riftProfile(payload).UnlockedTier != 42 {
		t.Fatal("book overwrote rift progress", err)
	}
}

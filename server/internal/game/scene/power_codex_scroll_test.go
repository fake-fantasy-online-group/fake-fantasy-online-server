package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestPowerCodexExtractSnapshotAndFailureAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s, _, b := powerBookSceneFixture(t)
		p := s.players[1]
		s.endgamePlayer(p).state.Payload = `{"powerBook":{"TY-A02":8123}}`
		req := domain.EndgameRequest{Op: "powerbook.extract", RequestID: "extract-scroll-00001", ExpectedRevision: "0", PowerID: "TY-A02"}
		s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
		s.finishEndgame(false)
		if b.commits != 1 {
			t.Fatal("extract missing")
		}
		var scroll domain.Stack
		b.plan.Snapshot.Bag.Each(func(_ int, st domain.Stack) {
			if st.Item == domain.PowerScrollItemID {
				scroll = st
			}
		})
		if scroll.UID <= 0 || !scroll.Bound || scroll.Count != 1 || scroll.Endgame.PowerScrollID != "TY-A02" || scroll.Endgame.PowerScrollRollBP != 8123 {
			t.Fatal("snapshot", scroll)
		}
		if p.Player.Bag.CountOf(domain.PowerScrollItemID) != 0 {
			t.Fatal("published before storage")
		}
		if fail {
			b.result <- domain.EndgameCommitResult{Err: errors.New("storage failure")}
		} else {
			b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
		}
		s.finishEndgame(false)
		want := int32(1)
		if fail {
			want = 0
		}
		if p.Player.Bag.CountOf(domain.PowerScrollItemID) != want {
			t.Fatal("bad commit application")
		}
		if !fail {
			b.receipt = &b.plan.Receipt
			s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
			s.finishEndgame(false)
			if b.commits != 1 || p.Player.Bag.CountOf(domain.PowerScrollItemID) != 1 {
				t.Fatal("replay duplicated scroll")
			}
		}
	}
}
func TestPowerCodexApplyUsesScrollNotNewRecordAndEquippedTarget(t *testing.T) {
	s, _, b := powerBookSceneFixture(t)
	p := s.players[1]
	p.Player.Bag.Set(6, domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}})
	gear := p.Player.Bag.At(200)
	p.Player.Bag.Set(200, domain.Stack{})
	p.Player.Worn.Set(domain.SlotWeapon, gear)
	s.endgamePlayer(p).state.Payload = `{"powerBook":{"TY-A02":10000}}`
	req := domain.EndgameRequest{Op: "powerbook.apply", RequestID: "apply-scroll-00001", ExpectedRevision: "0", RuneUID: "92", TargetUID: "42"}
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 || b.plan.Snapshot.Worn.At(domain.SlotWeapon).Legendary.PowerRollBP != 8123 || !b.plan.Snapshot.Bag.At(6).Empty() {
		t.Fatal("wrong scroll transaction")
	}
	if p.Player.Bag.At(6).Empty() || p.Player.Worn.At(domain.SlotWeapon) != gear {
		t.Fatal("premature mutation")
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if !p.Player.Bag.At(6).Empty() || p.Player.Worn.At(domain.SlotWeapon).Legendary.PowerRollBP != 8123 {
		t.Fatal("apply failed")
	}
}
func TestPowerCodexGatesAndFailuresNeverConsume(t *testing.T) {
	for _, kind := range []string{"locked", "foreign", "wrongtarget", "legacy", "nobook"} {
		s, sink, b := powerBookSceneFixture(t)
		p := s.players[1]
		scroll := domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
		req := domain.EndgameRequest{Op: "powerbook.apply", RequestID: "rejected-scroll-00001", ExpectedRevision: "0", RuneUID: "92", TargetUID: "42"}
		switch kind {
		case "locked":
			scroll.Locked = true
		case "foreign":
			req.RuneUID = "999"
		case "wrongtarget":
			req.TargetUID = "91"
		case "legacy":
			req.Op = "powerbook.imprint"
			req.RuneUID = ""
			req.PowerID = "TY-A02"
		case "nobook":
			p.Player.Bag.Set(5, domain.Stack{})
			req.Op = "powerbook.extract"
			req.TargetUID = ""
			req.RuneUID = ""
			req.PowerID = "TY-A02"
		}
		p.Player.Bag.Set(6, scroll)
		s.endgamePlayer(p).state.Payload = `{"powerBook":{"TY-A02":8123}}`
		s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
		s.finishEndgame(false)
		ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if !ok || ev.Message.OK || b.commits != 0 || p.Player.Bag.At(6) != scroll {
			t.Fatal(kind, ev, b.commits)
		}
	}
}
func TestPowerCodexLockedCatalogAndAllClassSlots(t *testing.T) {
	s, sink, _ := powerBookSceneFixture(t)
	p := s.players[1]
	s.onEndgamePowerBook(p, domain.EndgameRequest{Op: "powerbook.list"})
	ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ev.Message.OK || len(ev.Message.Powers) == 0 {
		t.Fatal("empty locked catalog")
	}
	for _, v := range ev.Message.Powers {
		if v.Unlocked || v.RollBP != 0 || len(v.Slots) == 0 {
			t.Fatal(v)
		}
	}
	power, _ := domain.LegendaryPower("SS-A01")
	slots := powerCodexSlots(power)
	found := false
	for _, slot := range slots {
		if slot == "戒指" {
			found = true
		}
	}
	if !found {
		t.Fatal("class powers falsely restricted")
	}
}

func TestPowerCodexScrollSnapshotCapabilityGate(t *testing.T) {
	s, sink, _ := powerBookSceneFixture(t)
	p := s.players[1]
	st := domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
	view := s.endgameItem(st, "bag", 6)
	for _, negotiated := range []bool{false, true} {
		p.Player.EndgamePowerCodex = negotiated
		s.endgameReply(p, domain.EndgameResponse{Kind: "snapshot", OK: true, Items: []domain.EndgameItemView{view}})
		ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if (ev.Message.Items[0].PowerScroll != nil) != negotiated {
			t.Fatal("capability gate", negotiated)
		}
	}
}
func TestPowerCodexPureMechanicScrollKeepsZeroRoll(t *testing.T) {
	s, _, b := powerBookSceneFixture(t)
	p := s.players[1]
	s.endgamePlayer(p).state.Payload = `{"powerBook":{"SS-U06":0}}`
	s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "powerbook.extract", RequestID: "pure-scroll-000001", ExpectedRevision: "0", PowerID: "SS-U06"}})
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("pure extract missing")
	}
	b.plan.Snapshot.Bag.Each(func(_ int, st domain.Stack) {
		if st.Item == domain.PowerScrollItemID && (!domain.LegendaryPowerRollValid(st.Endgame.PowerScrollID, st.Endgame.PowerScrollRollBP) || st.Endgame.PowerScrollRollBP != 0) {
			t.Fatal("pure mechanic roll", st.Endgame)
		}
	})
}
func TestPowerCodexResultNoticeExactlyOnce(t *testing.T) {
	s, sink, _ := powerBookSceneFixture(t)
	p := s.players[1]
	for _, r := range []domain.EndgameResponse{{Kind: "powerbook.apply", Error: "requirements", Message: "材料不符"}, {Kind: "powerbook.recover", Error: "bag_full", Message: "行囊已满"}, {Kind: "powerbook.result", OK: true, Message: "威能已附着"}} {
		s.endgameReply(p, r)
		n := 0
		for _, ev := range cubeTake(s, sink) {
			if _, ok := ev.(event.NewbieTextNotice); ok {
				n++
			}
		}
		if n != 1 {
			t.Fatal(r.Kind, n)
		}
	}
}
func TestPowerCodexOldClientNeverReceivesNewListSchema(t *testing.T) {
	s, sink, b := powerBookSceneFixture(t)
	p := s.players[1]
	p.Player.EndgamePowerCodex = false
	for _, op := range []string{"powerbook.list", "powerbook.extract", "powerbook.apply", "powerbook.recover"} {
		s.onEndgamePowerBook(p, domain.EndgameRequest{Op: op})
		ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if ev.Message.OK || ev.Message.Error != "update_required" || len(ev.Message.Powers) != 0 || b.commits != 0 {
			t.Fatal(op, ev)
		}
	}
}

package scene

import (
	"errors"
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestWingPowerScrollBagWornAtomicFailureReplay(t *testing.T) {
	for _, worn := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("worn=%v/fail=%v", worn, fail), func(t *testing.T) {
				s, _, b := powerBookSceneFixture(t)
				wings, _, _ := wingSceneFixture(t)
				s.wingRules = wings.wingRules
				s.items[20609] = wings.items[20609]
				p := s.players[1]
				wing := p.Player.Bag.At(200)
				wing.Item = 20609
				wing.Legendary = domain.LegendaryInstance{}
				wing.WingPowers[1] = domain.WingPower{PowerID: "TY-D01", PowerRollBP: 8234, SourceLevel: 100, SourceTier: domain.LegendaryTierTrue}
				wing.WingPowers[2] = domain.WingPower{PowerID: "TY-U01", PowerRollBP: 8345, SourceLevel: 100}
				p.Player.Bag.Set(200, wing)
				if worn {
					p.Player.Bag.Set(200, domain.Stack{})
					p.Player.Worn.Set(domain.SlotBag, wing)
				}
				scroll := domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
				p.Player.Bag.Set(6, scroll)
				req := domain.EndgameRequest{Op: "powerbook.apply", RequestID: "wing-scroll-00001", ExpectedRevision: "0", RuneUID: "92", TargetUID: "42"}
				s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
				s.finishEndgame(false)
				if b.commits != 1 {
					t.Fatal("no transaction")
				}
				changed := b.plan.Snapshot.Bag.At(200)
				if worn {
					changed = b.plan.Snapshot.Worn.At(domain.SlotBag)
				}
				if changed.WingPowers[0].PowerID != "TY-A02" || changed.WingPowers[0].PowerRollBP != 8123 || changed.WingPowers[1] != wing.WingPowers[1] || changed.WingPowers[2] != wing.WingPowers[2] || !b.plan.Snapshot.Bag.At(6).Empty() {
					t.Fatal("wrong candidate", changed)
				}
				live := p.Player.Bag.At(200)
				if worn {
					live = p.Player.Worn.At(domain.SlotBag)
				}
				if live != wing || p.Player.Bag.At(6) != scroll {
					t.Fatal("premature mutation")
				}
				// A repeated click while the commit is pending cannot consume twice.
				s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
				s.finishEndgame(false)
				if b.commits != 1 {
					t.Fatal("duplicate pending commit")
				}
				if fail {
					b.result <- domain.EndgameCommitResult{Err: errors.New("storage failure")}
				} else {
					b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
				}
				s.finishEndgame(false)
				live = p.Player.Bag.At(200)
				if worn {
					live = p.Player.Worn.At(domain.SlotBag)
				}
				if fail {
					if live != wing || p.Player.Bag.At(6) != scroll {
						t.Fatal("failure lost item")
					}
					return
				}
				if live != changed || !p.Player.Bag.At(6).Empty() {
					t.Fatal("commit missing")
				}
				b.receipt = &b.plan.Receipt
				s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
				s.finishEndgame(false)
				if b.commits != 1 {
					t.Fatal("replay double applied")
				}
			})
		}
	}
}

func TestWingPowerScrollRejectedTargetKeepsScroll(t *testing.T) {
	for _, name := range []string{"locked", "wrong-class", "not-wing", "duplicate-target"} {
		t.Run(name, func(t *testing.T) {
			s, _, b := powerBookSceneFixture(t)
			wings, _, _ := wingSceneFixture(t)
			s.wingRules = wings.wingRules
			s.items[20609] = wings.items[20609]
			p := s.players[1]
			wing := p.Player.Bag.At(200)
			wing.Item = 20609
			wing.Legendary = domain.LegendaryInstance{}
			scroll := domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
			switch name {
			case "locked":
				wing.Locked = true
			case "wrong-class":
				scroll.Endgame.PowerScrollID = "ZS-A01"
				p.Player.Char.Race = domain.Healer
				p.Player.Char.EmploymentKnown = true
				p.Player.Char.Employed = true
			case "not-wing":
				wing.Item = 1001
			case "duplicate-target":
				p.Player.Worn.Set(domain.SlotBag, wing)
			}
			p.Player.Bag.Set(200, wing)
			p.Player.Bag.Set(6, scroll)
			req := domain.EndgameRequest{Op: "powerbook.apply", RequestID: "bad-wing-scroll-0001", ExpectedRevision: "0", RuneUID: "92", TargetUID: "42"}
			s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
			s.finishEndgame(false)
			if b.commits != 0 || p.Player.Bag.At(200) != wing || p.Player.Bag.At(6) != scroll {
				t.Fatal("rejected operation mutated items")
			}
		})
	}
}

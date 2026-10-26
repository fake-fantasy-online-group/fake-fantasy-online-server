package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func resetCrystalFixture(t *testing.T) (*Scene, *dispatchEndgameBackend) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	s.skills = v5TestSkillTable(10401, 5, domain.SkillEffect{})
	ch.Level = 35
	ch.Race = domain.Warlock
	ch.Skills = domain.Learned{10401: 3, domain.SkillAlchemy: 90}
	ch.SkillPoints = 31
	ch.SkillPointsKnown = true
	ch.SkillAllocationVersion = 5
	s.items = make(map[domain.ItemID]domain.ItemDef)
	s.items[3380] = domain.ItemDef{ID: 3380, Name: "遗忘水晶", Stackable: true, InventoryTabKnown: true, InventoryTab: 1, UseAction: "skill.reset"}
	p.Player.Bag.Add(s.items[3380], 2)
	b := newDispatchBackend()
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = true
	return s, b
}
func TestSkillResetCrystalAtomicConsumeAndNoDoubleRefund(t *testing.T) {
	s, b := resetCrystalFixture(t)
	p := s.players[1]
	slot := -1
	p.Player.Bag.Each(func(i int, st domain.Stack) {
		if st.Item == 3380 {
			slot = i
		}
	})
	if slot < 0 {
		t.Fatal("no crystal")
	}
	cmd := UseItem{ID: 1, Item: 3380, Tab: 1, BagSlot: slot}
	s.onUseItem(cmd)
	if b.commits != 1 || p.Player.Char.SkillPoints != 31 || p.Player.Bag.CountOf(3380) != 2 {
		t.Fatal("published early")
	}
	s.onUseItem(cmd)
	if b.commits != 1 {
		t.Fatal("duplicate in-flight commit")
	}
	plan := b.plan
	if n := len(plan.Receipt.RequestID); n < 16 || n > 64 {
		t.Fatalf("invalid durable receipt ID length %d", n)
	}
	if plan.Snapshot.Char.SkillPoints != 34 || plan.Snapshot.Char.Skills[10401] != 0 || plan.Snapshot.Bag.CountOf(3380) != 1 || plan.Snapshot.Char.Skills[domain.SkillAlchemy] != 90 {
		t.Fatal("bad candidate", plan.Snapshot.Char)
	}
	b.commit <- domain.EndgameCommitResult{State: plan.State, Receipt: plan.Receipt}
	s.finishEndgame(false)
	if p.Player.Char.SkillPoints != 34 || p.Player.Bag.CountOf(3380) != 1 {
		t.Fatal("not published")
	}
	s.onUseItem(cmd)
	if b.commits != 1 || p.Player.Bag.CountOf(3380) != 1 {
		t.Fatal("empty duplicate consumed")
	}
}
func TestSkillResetCrystalStorageFailureKeepsPointsAndCrystal(t *testing.T) {
	s, b := resetCrystalFixture(t)
	p := s.players[1]
	slot := -1
	p.Player.Bag.Each(func(i int, st domain.Stack) {
		if st.Item == 3380 {
			slot = i
		}
	})
	s.useSkillResetCrystal(p, slot)
	b.commit <- domain.EndgameCommitResult{Err: errors.New("rollback")}
	s.finishEndgame(false)
	if p.Player.Char.SkillPoints != 31 || p.Player.Char.Skills[10401] != 3 || p.Player.Bag.CountOf(3380) != 2 {
		t.Fatal("rollback lost data")
	}
}

func TestSkillResetCrystalRecoveredReplayCleansStatusButPreservesCooldown(t *testing.T) {
	s, b := resetCrystalFixture(t)
	p := s.players[1]
	p.Status.Apply(domain.StatusDef{ID: 199930, SourceSkillID: 10401, DurationSec: 120}, s.tick, p.ID)
	p.Status.Apply(domain.StatusDef{ID: 199931, SourceSkillID: 99999, DurationSec: 120}, s.tick, p.ID)
	p.Player.StartSkillCooldown(10401, s.tick, 100)
	slot := -1
	p.Player.Bag.Each(func(i int, st domain.Stack) {
		if st.Item == 3380 {
			slot = i
		}
	})
	s.useSkillResetCrystal(p, slot)
	plan := b.plan
	if n := len(plan.Receipt.RequestID); n < 16 || n > 64 {
		t.Fatalf("invalid durable receipt ID length %d", n)
	}
	auth := plan.Snapshot.Clone()
	b.commit <- domain.EndgameCommitResult{State: plan.State, Receipt: plan.Receipt, Authoritative: &auth, Replayed: true, RequiresReload: true}
	s.finishEndgame(false)
	if p.Status.Has(199930) || !p.Status.Has(199931) {
		t.Fatal("recovery retained removed skill status or erased unrelated status")
	}
	if p.Player.SkillReadyAt(10401, s.tick) {
		t.Fatal("reset cleared cooldown")
	}
	if p.Player.Char.SkillPoints != 34 || p.Player.Bag.CountOf(3380) != 1 {
		t.Fatal("bad replay snapshot")
	}
	s.useSkillResetCrystal(p, slot)
	if b.commits != 1 {
		t.Fatal("replayed reset consumed again")
	}
}

func TestSkillResetCrystalInvalidHistoryOrLockedDoesNotConsume(t *testing.T) {
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid-budget", true: "locked"}[locked], func(t *testing.T) {
			s, b := resetCrystalFixture(t)
			p := s.players[1]
			slot := -1
			p.Player.Bag.Each(func(i int, st domain.Stack) {
				if st.Item == 3380 {
					slot = i
				}
			})
			if locked {
				st := p.Player.Bag.At(slot)
				st.Locked = true
				p.Player.Bag.Set(slot, st)
			} else {
				p.Player.Char.SkillPoints++
			}
			s.useSkillResetCrystal(p, slot)
			if b.commits != 0 || p.Player.Bag.CountOf(3380) != 2 || p.Player.Char.Skills[10401] != 3 {
				t.Fatal("invalid reset mutated")
			}
		})
	}
}

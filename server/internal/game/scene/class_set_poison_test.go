package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
)

func classPoisonScene(t *testing.T, pieces int) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s, mid, _ := statusScene(t)
	p := beefUp(s)
	p.Player.Char.Race = domain.Assassin
	suit := &domain.EquipmentSuit{ID: 1}
	slots := []domain.EquipSlot{domain.SlotHead, domain.SlotBody, domain.SlotWeapon, domain.SlotShoe, domain.SlotGlove, domain.SlotNeck, domain.SlotRing, domain.SlotBag}
	for i, slot := range slots {
		id := domain.ItemID(95000 + i)
		suit.Groups = append(suit.Groups, []domain.ItemID{id})
		s.items[id] = domain.ItemDef{ID: id, Equip: &domain.EquipDef{Slot: int32(slot), Suit: suit}}
		if i < pieces {
			p.Player.Worn.Set(slot, domain.Stack{Item: id, Count: 1})
		}
	}
	if got := s.classSetPieces(p, 1); got != int32(pieces) {
		t.Fatalf("fixture pieces %d != %d", got, pieces)
	}
	p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
	p.Status.Apply(domain.StatusDef{ID: 1018, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 120, PoisonOnHitChance: 100, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true}, s.tick, p.ID)
	s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}] = domain.StatusDef{ID: domain.StatusPoison, Level: 21, Name: "中毒", TypeKnown: true, Type: domain.StatusBad, DurationSec: 60, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	target := s.entities[mid]
	target.Status = domain.NewStatusSet()
	target.HP, target.MaxHP = 1000000, 1000000
	target.Stats.StatusResist = 0
	return s, p, target
}
func poisonTicks(s *Scene, n int) {
	for i := 0; i < n; i++ {
		s.tick += domain.Ticks(2000)
		s.stepStatus()
	}
}
func TestClassSetPoisonFourAddsAttackOnceOverLifetime(t *testing.T) {
	for _, pieces := range []int{3, 4, 5, 6, 8} {
		t.Run(string(rune('0'+pieces)), func(t *testing.T) {
			s, p, target := classPoisonScene(t, pieces)
			before := target.HP
			if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
				t.Fatal("poison failed")
			}
			p.Stats.MinAtk, p.Stats.MaxAtk = 9000, 9000
			p.Player.Worn = domain.NewEquipSet()
			poisonTicks(s, 30)
			want := int32(300)
			if pieces >= 4 {
				want += 1000
			}
			if got := before - target.HP; got != want {
				t.Fatalf("total=%d want %d", got, want)
			}
			if target.Status.Has(domain.StatusPoison) {
				t.Fatal("poison did not expire")
			}
		})
	}
}
func TestClassSetPoisonSixBurstsRemainingBeforeRefresh(t *testing.T) {
	s, p, target := classPoisonScene(t, 6)
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("initial poison")
	}
	poisonTicks(s, 10)
	before := target.HP
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("reapply")
	}
	if got := before - target.HP; got != 173 {
		t.Fatalf("burst=%d want floor(867/5)=173", got)
	}
	before = target.HP
	poisonTicks(s, 30)
	if got := before - target.HP; got != 1300 {
		t.Fatalf("refreshed total=%d", got)
	}
}
func TestClassSetPoisonBurstRequiresSuccessAndLiveBalance(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		s, p, target := classPoisonScene(t, 6)
		s.applyCombatTriggeredStatus(p, target, domain.StatusPoison)
		s.tick += domain.Ticks(60000)
		before := target.HP
		if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
			t.Fatal("reapply")
		}
		if target.HP != before {
			t.Fatal("expired poison burst")
		}
	})
	t.Run("blocked", func(t *testing.T) {
		s, p, target := classPoisonScene(t, 6)
		s.applyCombatTriggeredStatus(p, target, domain.StatusPoison)
		s.statusOverlay = domain.StatusOverlay{{Existing: domain.StatusPoison, Incoming: domain.StatusPoison}: domain.OverlayBlock}
		before := target.HP
		if s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
			t.Fatal("blocked apply succeeded")
		}
		if target.HP != before {
			t.Fatal("blocked poison burst")
		}
	})
	t.Run("unrelated poison", func(t *testing.T) {
		s, p, target := classPoisonScene(t, 6)
		target.Status.Apply(中毒, s.tick, 999)
		before := target.HP
		s.applyCombatTriggeredStatus(p, target, domain.StatusPoison)
		if target.HP != before {
			t.Fatal("unrelated poison burst")
		}
	})
	t.Run("removed set", func(t *testing.T) {
		s, p, target := classPoisonScene(t, 6)
		s.applyCombatTriggeredStatus(p, target, domain.StatusPoison)
		p.Player.Worn = domain.NewEquipSet()
		before := target.HP
		s.applyCombatTriggeredStatus(p, target, domain.StatusPoison)
		if target.HP != before {
			t.Fatal("unequipped set burst")
		}
	})
}
func TestClassSetPoisonOtherAssassinAndFatalBurst(t *testing.T) {
	s, p, target := classPoisonScene(t, 6)
	poison := s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}]
	poison.ClassSetPoison = true
	target.Status.Apply(poison, s.tick, 999)
	before := target.HP
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("reapply")
	}
	if before-target.HP != 60 {
		t.Fatalf("other assassin burst=%d", before-target.HP)
	}
	target.HP = 1
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("fatal reapply")
	}
	if target.Alive() {
		t.Fatal("fatal burst survived")
	}
}

func TestClassSetPoisonBrokenPieceAndMissDoNotBurst(t *testing.T) {
	s, p, target := classPoisonScene(t, 6)
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("initial")
	}
	st := p.Player.Worn.At(domain.SlotGlove)
	d := s.items[st.Item]
	d.Equip.Durable = 10
	s.items[st.Item] = d
	st.MaxDurability = 10
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotGlove, st)
	if got := s.classSetPieces(p, 1); got != 5 {
		t.Fatalf("broken count=%d", got)
	}
	before := target.HP
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("broken can still poison")
	}
	if target.HP != before {
		t.Fatal("broken six-piece burst")
	}
	p.Status.Remove(1018)
	if s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("missing carrier succeeded")
	}
	if target.HP != before {
		t.Fatal("failed trigger burst")
	}
}

func TestClassSetPoisonFractionalAttackAndSourceDeath(t *testing.T) {
	s, p, target := classPoisonScene(t, 4)
	p.Stats.MinAtk, p.Stats.MaxAtk = 100, 103
	before := target.HP
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("initial")
	}
	p.HP = 0
	delete(s.entities, p.ID)
	poisonTicks(s, 30)
	if got := before - target.HP; got != 1310 {
		t.Fatalf("snapshot total=%d want 1310", got)
	}
}

func TestClassSetPoisonBasicAttackReachesRuntime(t *testing.T) {
	s, p, target := classPoisonScene(t, 4)
	p.Stats.Hit = 1 << 20
	p.Pos.MapID = target.Pos.MapID
	s.exec(Attack{ID: p.ID, Target: target.ID})
	s.step()
	if !target.Status.Has(domain.StatusPoison) {
		t.Fatal("accepted basic attack did not apply poison")
	}
	before := target.HP
	poisonTicks(s, 30)
	if got := before - target.HP; got != 1300 {
		t.Fatalf("basic-attack DOT total=%d", got)
	}
}

// A delayed frame emits only one due tick and schedules from now, not from the
// old deadline. Reapplication before that frame must not cash out skipped ticks.
func TestClassSetPoisonDelayedTickBurstDoesNotCatchUp(t *testing.T) {
	s, p, target := classPoisonScene(t, 6)
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("initial")
	}
	s.tick += domain.Ticks(15000)
	var old domain.Active
	target.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoison {
			old = a
		}
	})
	// The same stored status, if allowed to run from this delayed frame, has ticks
	// at 15,17,...,59 seconds: 23 fixed ticks plus the same rational bonus carry.
	wantRemaining := int64(23*10 + 1000*23/30)
	if got := classSetPoisonRemaining(old, s.tick, target.MaxHP); got != wantRemaining {
		t.Fatalf("delayed balance=%d want %d", got, wantRemaining)
	}
	before := target.HP
	if !s.applyCombatTriggeredStatus(p, target, domain.StatusPoison) {
		t.Fatal("reapply")
	}
	if got := before - target.HP; got != int32(wantRemaining/5) {
		t.Fatalf("delayed burst=%d want %d", got, wantRemaining/5)
	}
}

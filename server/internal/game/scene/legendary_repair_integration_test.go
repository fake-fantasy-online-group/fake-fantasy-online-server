package scene

import (
	"fmt"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func nativeRepairAlly(t *testing.T, s *Scene, p *entity.Entity, id domain.EntityID, x float64) *entity.Entity {
	t.Helper()
	join(t, s, id, int64(100+id), "combination ally", x, 100)
	a := s.players[id]
	p.Player.Party, a.Player.Party = 77, 77
	a.MaxHP, a.HP = 1000, 1
	return a
}

func nativeRepairSet(t *testing.T, s *Scene, p *entity.Entity, suitID int32, pieces int) {
	t.Helper()
	suit := &domain.EquipmentSuit{ID: suitID, EnhancedClass: suitID}
	slots := []domain.EquipSlot{domain.SlotNeck, domain.SlotRing, domain.SlotGlove, domain.SlotShoe, domain.SlotBag, domain.SlotFace, domain.SlotBody, domain.SlotHead, domain.SlotWeapon}
	n := 0
	for i, slot := range slots {
		if p.Player.Worn.At(slot).Item != 0 {
			continue
		}
		id := domain.ItemID(975000 + i)
		suit.Groups = append(suit.Groups, []domain.ItemID{id})
		s.items[id] = domain.ItemDef{ID: id, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100, Suit: suit}}
		p.Player.Worn.Set(slot, domain.Stack{UID: int64(id), Item: id, Count: 1, Durability: 100, MaxDurability: 100})
		n++
		if n == pieces {
			break
		}
	}
	s.invalidateCombatAttributes(p)
	if got := s.classSetPieces(p, suitID); got != int32(pieces) {
		t.Fatal("set pieces inactive", got, pieces)
	}
}

func TestLegendaryRepairCirclePrimaryTripleComposition(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10206: 1}, "CK-A06", "CK-A07", "TY-A04")
	side := putMonster(s, 901, 100000, domain.Stats{}, 790, 100)
	nativeRepairCast(t, s, p, 10206, m.ID)
	if got := 100000 - m.HP; got != 290 {
		t.Fatalf("selected primary lost A07 after A06 circle: damage=%d want290", got)
	}
	if got := 100000 - side.HP; got != 217 {
		t.Fatalf("final circle expansion/secondary classification: damage=%d want217", got)
	}
}

func TestLegendaryRepairNativeBaseAndSetControls(t *testing.T) {
	for _, pieces := range []int{0, 2, 4} {
		t.Run(fmt.Sprintf("healer_set_%d", pieces), func(t *testing.T) {
			s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1, 10319: 1})
			if pieces > 0 {
				nativeRepairSet(t, s, p, 2, pieces)
			}
			a := nativeRepairAlly(t, s, p, 2, 140)
			p.HP = 1
			nativeRepairCast(t, s, p, 10301, a.ID)
			if a.HP <= 1 {
				t.Fatal("ordinary explicit ally heal regressed")
			}
			if pieces >= 2 && p.HP <= 1 {
				t.Fatal("ordinary primary heal lost permitted set copy")
			}
			if pieces == 0 && p.HP != 1 {
				t.Fatal("ordinary heal acquired unsolicited self copy")
			}
			if pieces == 4 && m.HP == 100000 {
				t.Fatal("ordinary four-piece heal-to-bolt conversion regressed")
			}
		})
	}
	for _, targetSelf := range []bool{false, true} {
		s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1})
		p.HP = 1
		target := domain.EntityID(0)
		if targetSelf {
			target = p.ID
		}
		nativeRepairCast(t, s, p, 10301, target)
		if p.HP != 1101 {
			t.Fatalf("ordinary self heal explicit=%v got%d want1101", targetSelf, p.HP)
		}
	}
}

func TestLegendaryRepairLightSplitPrimarySecondaryAndFullRadius(t *testing.T) {
	run := func(echo bool) (int32, []int32) {
		powers := []string{"YS-A01", "TY-A04"}
		if echo {
			powers = append(powers, "YS-A02")
		}
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10319: 1}, powers...)
		pos := m.Pos
		pos.X = 400
		s.aoi.Move(m, pos)
		var sides []*entity.Entity
		for i, x := range []float64{410, 420, 430, 230, 620, 653} {
			sides = append(sides, putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, x, 100))
		}
		nativeRepairCast(t, s, p, 10319, m.ID)
		var losses []int32
		for _, side := range sides {
			losses = append(losses, 100000-side.HP)
		}
		return 100000 - m.HP, losses
	}
	primary, sides := run(false)
	echo, echoSides := run(true)
	if primary != 210 || echo != 388 {
		t.Fatalf("primary/echo damage %d/%d want210/388", primary, echo)
	}
	for i := 0; i < 5; i++ {
		if sides[i] != 126 || echoSides[i] != 126 {
			t.Fatalf("secondary %d generated an echo or lost native 60%%: %v/%v", i, sides, echoSides)
		}
	}
	for i := 5; i < len(sides); i++ {
		if sides[i] != 0 || echoSides[i] != 0 {
			t.Fatalf("secondary exceeded the amplified native radius: %v/%v", sides, echoSides)
		}
	}
}

func TestLegendaryRepairHealChildImplicitSelfAndSetIsolation(t *testing.T) {
	for _, pieces := range []int{0, 2, 4} {
		t.Run(fmt.Sprintf("set_%d", pieces), func(t *testing.T) {
			s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1, 10319: 1}, "YS-U04", "YS-A23", "YS-A01")
			if pieces > 0 {
				nativeRepairSet(t, s, p, 2, pieces)
			}
			a, b := nativeRepairAlly(t, s, p, 2, 140), nativeRepairAlly(t, s, p, 3, 150)
			for i := 0; i < 6; i++ {
				putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, 140+float64(i)*10, 100)
			}
			p.HP = 1
			nativeRepairCast(t, s, p, 10301, 0)
			if a.HP <= 1 || b.HP <= 1 {
				t.Fatal("implicit self heal failed to expand to allies", a.HP, b.HP)
			}
			if p.HP != 1101 {
				t.Fatalf("secondary healing recursively copied by set%d: ownerHP=%d want1101", pieces, p.HP)
			}
			if m.HP == 100000 {
				t.Fatal("primary healing did not generate its permitted child")
			}
			if pieces < 4 {
				n := 0
				for _, e := range s.entities {
					if e.Kind == domain.KindMonster && e.HP < 100000 {
						n++
					}
				}
				if n != 7 {
					t.Fatalf("healing-derived A01 child hit%d enemies want7", n)
				}
			}
			before := m.HP
			s.stepCastsAt(s.now().Add(3*time.Minute), false)
			if m.HP != before || s.equipmentState().classSetTriggered != 0 {
				t.Fatal("healing/set/child chain recurred")
			}
		})
	}
}

func TestLegendaryRepairFireballChildCenterTriple(t *testing.T) {
	for _, reprise := range []bool{false, true} {
		powers := []string{"SS-A04", "SS-A06"}
		if reprise {
			powers = append(powers, "SS-A30")
		}
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10402: 1, 10420: 1}, powers...)
		if reprise {
			nativeRepairCast(t, s, p, 10420, p.ID)
		}
		before := p.MP
		// Native instant-cast status refreshes the character's actual attributes.
		// Preserve that consumer instead of resetting the fixture back to MAtk=100.
		main := int32(int64(p.Stats.MAtk) * 195 * 120 / 10000)
		want := main + main*70/100
		if reprise {
			want += main * 80 / 100
		}
		nativeRepairCast(t, s, p, 10402, m.ID)
		if got := 100000 - m.HP; got != want {
			t.Fatalf("reprise=%v child classified center as ring: damage=%d want%d", reprise, got, want)
		}
		if paid := before - p.MP; paid != 64 {
			t.Fatalf("children charged native skill cost again: %d", paid)
		}
	}
}

func TestLegendaryRepairFreezeChildBurnSettlementPreserved(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10421: 1, 10428: 1, 10409: 1}, "SS-A24", "SS-A11")
	nativeRepairCast(t, s, p, 10421, m.ID)
	if !m.Status.HasEffective(1036) {
		t.Fatal("native high-heat burn absent")
	}
	before := m.HP
	nativeRepairCast(t, s, p, 10428, m.ID)
	if m.HP >= before || !m.Status.HasEffective(1004) || !m.Status.HasEffective(1036) {
		t.Fatal("previously valid A24+A11 freeze/burn composition regressed", before, m.HP, m.Status)
	}
}

func TestLegendaryRepairTrapTargetBonusesAndNativeCost(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "CK-A13", "TY-A30", "TY-A31")
	// Native poison/control states isolate target-conditional damage. They are
	// applied to the monster, never enabling an unreachable player PvP target.
	poison := s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}]
	if poison.ID == 0 {
		t.Fatal("native poison anchor missing")
	}
	m.Status.Apply(poison, s.tick, p.ID)
	control := s.statuses[domain.StatusKey{ID: 1002, Level: 27}]
	m.Status.Apply(control, s.tick, p.ID)
	before := p.MP
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10213, At: &m.Pos})
	if len(s.casts) == 0 {
		t.Fatalf("native trap placement rejected: %+v", s.outbox)
	}
	s.stepCastsAt(s.now().Add(time.Minute), false)
	s.stepCastsAt(s.now().Add(2*time.Minute), false)
	if len(s.traps) != 3 {
		t.Fatal("triple trap shape lost", len(s.traps))
	}
	s.stepTraps()
	if got := 100000 - m.HP; got != 1140 {
		t.Fatalf("three native bombs target bonus lost/doubled: damage=%d want1140", got)
	}
	if paid := before - p.MP; paid != 21 {
		t.Fatal("secondary trap charged extra mana", paid)
	}
	if len(s.traps) != 0 {
		t.Fatal("resolved native traps remain", len(s.traps))
	}
}

func TestLegendaryRepairBasicFanTargetModifiers(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{}, "TY-A01", "TY-A30", "TY-A31")
	p.Look.AtkDist = 100
	side := putMonster(s, 901, 100000, domain.Stats{}, 145, 110)
	for _, e := range []*entity.Entity{m, side} {
		if e.Status == nil {
			e.Status = domain.NewStatusSet()
		}
		poison := s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}]
		e.Status.Apply(poison, s.tick, p.ID)
		e.Status.Apply(s.statuses[domain.StatusKey{ID: 1002, Level: 27}], s.tick, p.ID)
	}
	s.onAttack(Attack{ID: p.ID, Target: m.ID, Blow: combat.Blow{GuaranteedHit: true}})
	if len(s.attacks) == 0 {
		t.Fatal("native basic attack rejected")
	}
	s.stepCombatAt(s.now().Add(time.Second), false)
	if got := 100000 - m.HP; got != 190 {
		t.Fatalf("basic primary target bonuses=%d want190", got)
	}
	if got := 100000 - side.HP; got != 133 {
		t.Fatalf("fan side target bonuses=%d want133", got)
	}
}

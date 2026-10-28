package scene

import (
	"slices"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestVariantMendNativeGroupHealAndReceivedPower(t *testing.T) {
	for _, group := range []bool{false, true} {
		for _, boosted := range []bool{false, true} {
			powers := []string{}
			if group {
				powers = append(powers, "YS-U04")
			}
			s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, powers...)
			wearEquipmentV5(t, s, p, 2056, 45454)
			p.Player.EndgameSkillVariants = true
			a := nativeRepairAlly(t, s, p, 2, 140)
			b := nativeRepairAlly(t, s, p, 3, 150)
			if boosted {
				b.Player.Char.Race, b.Player.Char.Level = domain.Healer, 100
				b.Player.Char.Employed, b.Player.Char.EmploymentKnown = true, true
				// Do not reuse nativeRepairEquip's source item IDs across players.
				item := domain.ItemDef{ID: 991112, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), LevelReq: 100, Durable: 100}}
				power, _ := domain.LegendaryPower("TY-D12")
				if !domain.LegendaryPowerEligible(power, item) {
					t.Fatal("received-heal power fixture slot")
				}
				s.items[item.ID] = item
				b.Player.Worn.Set(domain.SlotBody, domain.Stack{Item: item.ID, UID: 991112, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: "TY-D12"}})
				s.invalidateCombatAttributes(b)
			}
			p.HP, p.MaxHP = 100, 10000
			a.HP, a.MaxHP = 100, 10000
			b.HP, b.MaxHP = 100, 10000
			nativeRepairCast(t, s, p, 10301, a.ID)
			deposit, ok := s.equipmentState().variantMends[b.ID]
			if !group {
				if ok || b.HP != 100 {
					t.Fatal("no group power still healed/stored secondary", b.HP, ok)
				}
				continue
			}
			if !ok || deposit.amount <= 0 || b.HP <= 100 {
				t.Fatal("group secondary lost named immediate/delayed heal", b.HP, deposit)
			}
			if !slices.Contains(deposit.causalPath, s.combatEffectKey(p.ID, "variant:healer_guarded_mend")) {
				t.Fatal("delayed branch lost producer identity")
			}
			primary := s.equipmentState().variantMends[a.ID]
			if primary.amount != deposit.amount {
				t.Fatal("secondary changed nominal delayed budget", primary.amount, deposit.amount)
			}
			before := b.HP
			s.stepVariantMends(deposit.due.Add(-time.Millisecond))
			if b.HP != before {
				t.Fatal("deposit paid early")
			}
			s.stepVariantMends(deposit.due)
			want := deposit.amount
			if boosted {
				want = want * 140 / 100
			}
			if b.HP-before != want {
				t.Fatalf("group=%v receivedPower=%v delayed=%d want%d", group, boosted, b.HP-before, want)
			}
			settled := b.HP
			s.stepVariantMends(deposit.due.Add(time.Hour))
			if b.HP != settled || len(s.equipmentState().variantMends) != 0 {
				t.Fatal("delayed heal repeated")
			}
		}
	}
}

func TestVariantMendCausalCallbackDoesNotReDepositOrInventPrimary(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, "YS-U04")
	wearEquipmentV5(t, s, p, 2056, 45454)
	p.Player.EndgameSkillVariants = true
	p.HP, p.MaxHP = 100, 10000
	def, _ := s.skills.Get(10301, 1)
	s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, p.Player.Char.Skills)
	if def.EquipmentVariant == nil {
		t.Fatal("named equipment variant missing")
	}
	old := legendaryHandlers["YS-U04"]
	h := old
	calls, healedCalls := 0, 0
	h.Healed = func(c *legendaryContext, before, effective int32) {
		healedCalls++
		if c.Skill != 10301 || effective <= 0 || c.Target.HP != before+effective {
			t.Fatal("delayed effective-heal callback lost original identity/amount")
		}
	}
	h.HealResolved = func(c *legendaryContext, raw, effective, overflow int32) {
		calls++
		if c.Skill != 10301 || c.Primary != 0 {
			t.Fatal("delayed heal invented skill/accepted primary", c.Skill, c.Primary)
		}
		if got := s.startVariantMend(p, p, def, 100); got != 100 {
			t.Fatal("same variant split its own delayed reaction", got)
		}
	}
	legendaryHandlers["YS-U04"] = h
	t.Cleanup(func() { legendaryHandlers["YS-U04"] = old })
	original := make([]string, 1, 8)
	original[0] = "unrelated-parent"
	restore := s.withCombatCausalPath(original)
	s.startVariantMend(p, p, def, 1000)
	restore()
	deposit := s.equipmentState().variantMends[p.ID]
	original[0] = "mutated-parent"
	if deposit.causalPath[0] != "unrelated-parent" {
		t.Fatal("pending branch aliases caller path")
	}
	s.stepVariantMends(deposit.due)
	if calls != 1 || healedCalls != 1 || len(s.equipmentState().variantMends) != 0 {
		t.Fatal("callback absent or regenerated deposit", calls)
	}
	s.stepVariantMends(deposit.due.Add(time.Hour))
	if calls != 1 {
		t.Fatal("callback loop continued")
	}
	// A later independent cast is not denied by the finished branch's ancestry.
	s.startVariantMend(p, p, def, 1000)
	if len(s.equipmentState().variantMends) != 1 {
		t.Fatal("completed lineage blocked fresh cast")
	}
}

func TestVariantMendGroupDepositsCancelOnActualUnequip(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, "YS-U04")
	wearEquipmentV5(t, s, p, 2056, 45454)
	p.Player.EndgameSkillVariants = true
	a, b := nativeRepairAlly(t, s, p, 2, 140), nativeRepairAlly(t, s, p, 3, 150)
	nativeRepairCast(t, s, p, 10301, a.ID)
	if len(s.equipmentState().variantMends) < 2 {
		t.Fatal("group deposits absent")
	}
	hpA, hpB := a.HP, b.HP
	p.Player.Worn.Set(domain.SlotHead, domain.Stack{})
	s.stepVariantMends(s.now().Add(time.Hour))
	if a.HP != hpA || b.HP != hpB || len(s.equipmentState().variantMends) != 0 {
		t.Fatal("unequipped named item paid a deferred group heal")
	}
}

func TestVariantMendEntityLeaveReentryCancelsOnlyRelatedDeposits(t *testing.T) {
	for _, leavingSource := range []bool{false, true} {
		name := "target"
		if leavingSource {
			name = "source"
		}
		t.Run(name, func(t *testing.T) {
			s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1})
			wearEquipmentV5(t, s, p, 2056, 45454)
			p.Player.EndgameSkillVariants = true
			a := nativeRepairAlly(t, s, p, 2, 140)
			q := nativeRepairAlly(t, s, p, 3, 150)
			b := nativeRepairAlly(t, s, p, 4, 160)
			q.Player.Char.Race = domain.Healer
			q.Player.Char.Skills = domain.Learned{10301: 1}
			q.Player.EndgameSkillVariants = true
			wearEquipmentV5(t, s, q, 2056, 45455)
			def, _ := s.skills.Get(10301, 1)
			s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, p.Player.Char.Skills)
			other, _ := s.skills.Get(10301, 1)
			s.applyResolvedSkillModifiers(q, &other, q.Player.Char.Skills, q.Player.Char.Skills)
			if def.EquipmentVariant == nil || other.EquipmentVariant == nil {
				t.Fatal("native variants missing")
			}
			s.startVariantMend(p, a, def, 100)
			s.startVariantMend(q, b, other, 100)
			pending := s.equipmentState().variantMends[b.ID]
			if len(s.equipmentState().variantMends) != 2 || pending.amount <= 0 {
				t.Fatal("deposits missing")
			}
			departing := a
			if leavingSource {
				departing = p
			}
			// Transfer preserves IDs and can return before the next pending-heal tick.
			delete(s.entities, departing.ID)
			delete(s.players, departing.ID)
			s.removeEquipmentEntity(departing.ID)
			s.entities[departing.ID], s.players[departing.ID] = departing, departing
			beforeA, beforeB := a.HP, b.HP
			s.stepVariantMends(pending.due.Add(time.Second))
			if a.HP != beforeA {
				t.Fatalf("old %s deposit healed after same-ID reentry: %d -> %d", name, beforeA, a.HP)
			}
			if b.HP-beforeB != pending.amount {
				t.Fatalf("unrelated pending heal lost: got %d want %d", b.HP-beforeB, pending.amount)
			}
			if len(s.equipmentState().variantMends) != 0 {
				t.Fatal("settled deposits retained")
			}
		})
	}
}

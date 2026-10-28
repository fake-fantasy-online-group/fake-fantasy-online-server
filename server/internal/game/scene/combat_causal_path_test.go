package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestCombatCausalDamageDistinctProducersBothDirectionsAndSiblings(t *testing.T) {
	for _, settled := range []bool{false, true} {
		t.Run(map[bool]string{false: "mitigated", true: "settled"}[settled], func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, "ZS-A03")
			warriorEquip(t, s, p, "ZS-A04", domain.SlotBody)
			a, b := legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"]
			t.Cleanup(func() { legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"] = a, b })
			counts := map[string]int{}
			fn := func(c *legendaryContext) { counts[c.Power.ID]++; c.Damage(m, 10, false, settled) }
			legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", AfterHit: fn}
			legendaryHandlers["ZS-A04"] = legendaryHandler{revision: "synergy-v1", AfterHit: fn}
			m.HP, m.MaxHP = 100000, 100000
			for i := 0; i < 2; i++ {
				s.legendaryAfterHit(p, m, combat.Blow{Origin: domain.DamageNormalOrigin}, event.DamageDealt{Src: p.ID, Dst: m.ID, Amount: 1}, m.HP)
			}
			// Each independent root admits A→B and B→A, but never A→B→A.
			if counts["ZS-A03"] != 4 || counts["ZS-A04"] != 4 {
				t.Fatalf("branch counts %#v", counts)
			}
			if m.HP != 99920 {
				t.Fatalf("eight 10-point payloads HP=%d", m.HP)
			}
			if len(s.legendaryState().causalPath) != 0 || s.legendaryState().depth != 0 {
				t.Fatal("causal scope leaked")
			}
		})
	}
}

func TestCombatCausalHealManaDispelIndependentConsumers(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	warriorEquip(t, s, p, "ZS-A04", domain.SlotBody)
	a, b := legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"] = a, b })
	counts := map[string]int{}
	for _, id := range []string{"ZS-A03", "ZS-A04"} {
		legendaryHandlers[id] = legendaryHandler{revision: "synergy-v1", Healed: func(c *legendaryContext, before, n int32) {
			counts[c.Power.ID]++
			c.Scene.legendaryCasterManaRestored(p, p, domain.SkillDef{ID: 10002}, 1, 0)
		}, ManaRestored: func(c *legendaryContext, n, overflow int32) {
			counts[c.Power.ID]++
			c.Scene.legendaryCasterDispelled(p, m, domain.SkillDef{ID: 10002}, nil)
		}, Dispelled: func(c *legendaryContext, a []domain.Active) {
			counts[c.Power.ID]++
			c.Scene.legendaryCasterHealed(p, p, domain.SkillDef{ID: 10002}, 1, 1)
		}}
	}
	s.equipmentState().classSetTriggered = 1 // no blanket suppression for a set-produced event
	s.legendaryCasterHealed(p, p, domain.SkillDef{ID: 10002}, 1, 1)
	if counts["ZS-A03"] != 2 || counts["ZS-A04"] != 2 {
		t.Fatalf("cycle bounded consumers=%v", counts)
	}
	if len(s.legendaryState().causalPath) != 0 {
		t.Fatal("scope leaked")
	}
}

func TestCombatCausalIdentitySurvivesEntityReassignment(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A03")
	p.Player.Char.ID = 123456
	key := s.combatEffectKey(p.ID, "ZS-A03")
	old := p.ID
	delete(s.entities, old)
	p.ID = old + 100
	s.entities[p.ID] = p
	if key != s.combatEffectKey(p.ID, "ZS-A03") {
		t.Fatal("persistent character identity changed")
	}
	ok, restore := s.enterCombatEffect(p.ID, "ZS-A03")
	if !ok {
		t.Fatal("first denied")
	}
	defer restore()
	if ok, _ := s.enterCombatEffect(p.ID, "ZS-A03"); ok {
		t.Fatal("same effect reentered")
	}
	if ok, done := s.enterCombatEffect(p.ID, "ZS-A04"); !ok {
		t.Fatal("independent effect denied")
	} else {
		done()
	}
}

func TestCombatCausalChildProducerIdentityIncludesOwner(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	q := *p
	qp := *p.Player
	qc := *p.Player.Char
	qc.ID += 100
	qp.Char = &qc
	q.Player = &qp
	q.ID = p.ID + 100
	s.entities[q.ID] = &q
	s.players[q.ID] = &q
	// The same named effect on another actor is not the same causal producer.
	s.legendaryState().child = &legendaryChildFrame{Producers: map[string]bool{s.combatEffectKey(p.ID, "ZS-A03"): true}}
	defer func() { s.legendaryState().child = nil }()
	ctx := s.legendaryContexts(&q, m, combat.Blow{SkillID: 10011}, event.DamageDealt{})[0]
	d := domain.SkillDef{ID: 10011, HurtType: domain.HurtPhysical, Dist: 500, TargetEnemy: true, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
	before := m.HP
	if !ctx.ChildSkill(m.ID, d, legendaryChildOptions{TargetOnly: true}) || m.HP >= before {
		t.Fatal("different owner's effect was incorrectly treated as a cycle")
	}
}

func TestCombatCausalSetManaDeliversEventWithoutInventingSkill(t *testing.T) {
	s, p, _ := casterSetFixture(t, 2, 2)
	warriorEquip(t, s, p, "YS-U03", domain.SlotBody)
	p.Player.Char.Level = 100
	item := s.items[p.Player.Worn.At(domain.SlotBody).Item]
	item.Equip.Need = domain.Requirement{}
	s.items[item.ID] = item
	p.Player.Char.Skills[10302] = 1
	p.MP = 0
	s.stepCasterSets()
	s.tick += domain.Ticks(3000)
	original := legendaryHandlers["YS-U03"]
	defer func() { legendaryHandlers["YS-U03"] = original }()
	calls := 0
	h := original
	h.ManaRestored = func(c *legendaryContext, effective, overflow int32) {
		calls++
		if c.Skill != 0 || effective != 30 || overflow != 0 {
			t.Fatalf("passive mana event fabricated spell/data: skill%d amount%d overflow%d", c.Skill, effective, overflow)
		}
		if !s.combatEffectActive(p.ID, "set:periodic-mana") {
			t.Fatal("producer ancestry missing")
		}
		legendaryCasterRestoreMP(c, p, 10) // feedback cannot call this same consumer again
	}
	legendaryHandlers["YS-U03"] = h
	s.stepCasterSets()
	if calls != 1 || p.MP != 40 {
		t.Fatalf("bounded event calls%d mana%d", calls, p.MP)
	}
	// Authored U03 remains a specific 10302 overflow effect, not a free set proc.
	legendaryHandlers["YS-U03"] = original
	before := p.MP
	s.tick += domain.Ticks(3000)
	s.stepCasterSets()
	if p.MP != before+30 {
		t.Fatal("passive set invented an unrelated skill effect")
	}
	h.ManaRestored = func(*legendaryContext, int32, int32) { panic("test callback") }
	legendaryHandlers["YS-U03"] = h
	s.tick += domain.Ticks(3000)
	panicked := false
	func() { defer func() { panicked = recover() != nil }(); s.stepCasterSets() }()
	if !panicked || len(s.legendaryState().causalPath) != 0 {
		t.Fatal("panic leaked producer scope")
	}
}

func TestCombatCausalBeforeDamageStateKeepsDistinctPrimaryTargets(t *testing.T) {
	s, p, low := legendaryMeleeFixture(t, "JK-A23")
	high := putMonster(s, 999, 10000, domain.Stats{}, low.Pos.X+20, low.Pos.Y)
	low.MaxHP, low.HP = 1000, 200
	parent := combat.Blow{SkillID: 10123, SkillPct: 100, RootID: 77, PrimaryTarget: low.ID}
	child := parent
	child.PrimaryTarget = high.ID
	child.Origin = domain.DamageProcOrigin
	child.ProcDepth = 1
	s.legendaryBeforeDamage(p, low, parent, event.DamageDealt{}, 100, false)
	s.legendaryBeforeDamage(p, high, child, event.DamageDealt{}, 100, false)
	before := high.HP
	low.HP = 100
	s.legendaryAfterHit(p, low, parent, event.DamageDealt{Src: p.ID, Dst: low.ID, SkillID: 10123, Amount: 100}, 200)
	if high.HP >= before {
		t.Fatal("child's healthy primary overwrote parent's low-life burst condition")
	}
}

func TestCombatCausalLowLifeUsesEachStrikeBeforeHPNotNestedCurrentHP(t *testing.T) {
	s, p, target := legendaryMeleeFixture(t, "JK-A23")
	side := putMonster(s, 998, 10000, domain.Stats{}, target.Pos.X+20, target.Pos.Y)
	target.MaxHP, target.HP = 1000, 600
	b := combat.Blow{SkillID: 10123, SkillPct: 100, RootID: 78, PrimaryTarget: target.ID}
	s.legendaryBeforeDamage(p, target, b, event.DamageDealt{}, 200, false)
	target.HP = 400
	child := b
	child.Origin = domain.DamageProcOrigin
	child.ProcDepth = 1
	s.legendaryBeforeDamage(p, target, child, event.DamageDealt{}, 100, false)
	target.HP = 300
	before := side.HP
	s.legendaryAfterHit(p, target, b, event.DamageDealt{Src: p.ID, Dst: target.ID, SkillID: 10123, Amount: 200}, 600)
	if side.HP != before {
		t.Fatal("nested low-life hit retroactively enabled healthy parent burst")
	}
	s.legendaryAfterHit(p, target, child, event.DamageDealt{Src: p.ID, Dst: target.ID, SkillID: 10123, Amount: 100}, 400)
	if side.HP >= before {
		t.Fatal("eligible low-life child lost its independent burst")
	}
}

package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestStatusCausalPathMergeDoesNotMutateSnapshots(t *testing.T) {
	stored := make([]string, 1, 8)
	stored[0] = "A"
	current := []string{"B", "A"}
	got := mergeStatusCausalPath(stored, current)
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("merged %v", got)
	}
	got[0] = "changed"
	if stored[0] != "A" || current[0] != "B" {
		t.Fatal("snapshot mutated")
	}
}

func TestStatusCausalDelayedHealAllowsOtherPowerAndFreshRoot(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	s.items[900002] = domain.ItemDef{ID: 900002, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1}}}
	p.Player.Worn.Set(domain.SlotBody, domain.Stack{UID: 992, Item: 900002, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: "ZS-A04"}})
	oldA, oldB := legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"] = oldA, oldB })
	counts := map[string]int{}
	var saved domain.Active
	legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", Healed: func(c *legendaryContext, _ int32, _ int32) { counts[c.Power.ID]++ }}
	legendaryHandlers["ZS-A04"] = legendaryHandler{revision: "synergy-v1", Healed: func(c *legendaryContext, _ int32, _ int32) {
		counts[c.Power.ID]++
		// A's delayed heal triggers B, which creates another delayed heal.
		def := domain.StatusDef{ID: 998501, DurationSec: 30, IntervalSec: 1, TypeKnown: true, Type: domain.StatusGood, EffectHitBP: 10000, LegendaryPowerID: c.Power.ID, HPChanges: []domain.ResourceChange{{Value: 10}}}
		if !s.applyStatusDef(m, def, p.ID) {
			t.Fatal("generated status rejected")
		}
		m.Status.Each(func(a domain.Active) {
			if a.Def.ID == def.ID {
				saved = a
			}
		})
	}}
	a := domain.Active{Source: p.ID, Def: domain.StatusDef{ID: 998500, LegendaryPowerID: "ZS-A03", CombatCausalPath: []string{s.combatEffectKey(p.ID, "ZS-A03")}, HPChanges: []domain.ResourceChange{{Value: 10}}}}
	heal := func(target *entity.Entity, active domain.Active) {
		target.HP = target.MaxHP - 100
		s.tickStatus(target, active)
	}
	heal(m, a)
	if counts["ZS-A03"] != 0 || counts["ZS-A04"] != 1 {
		t.Fatalf("A -> B counts %v", counts)
	}
	if len(saved.Def.CombatCausalPath) != 2 {
		t.Fatalf("lost delayed ancestry %v", saved.Def.CombatCausalPath)
	}
	heal(m, saved)
	if counts["ZS-A03"] != 0 || counts["ZS-A04"] != 1 {
		t.Fatalf("A -> B -> A cycle escaped: %v", counts)
	}
	// An independently applied native recovery is a new root, not blacklisted.
	a.Def.CombatCausalPath = nil
	a.Def.LegendaryPowerID = ""
	heal(m, a)
	if counts["ZS-A03"] != 1 || counts["ZS-A04"] != 2 {
		t.Fatalf("new root starved %v", counts)
	}
	if len(s.legendaryState().causalPath) != 0 {
		t.Fatal("timer causal scope leaked")
	}
}

func TestStatusCausalDelayedDamageAndManaBranches(t *testing.T) {
	for _, kind := range []string{"damage", "mana"} {
		t.Run(kind, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, "ZS-A03")
			s.items[900002] = domain.ItemDef{ID: 900002, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1}}}
			p.Player.Worn.Set(domain.SlotBody, domain.Stack{UID: 992, Item: 900002, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: "ZS-A04"}})
			oldA, oldB := legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"]
			t.Cleanup(func() { legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"] = oldA, oldB })
			countA, countB := 0, 0
			saved := domain.Active{}
			base := domain.StatusDef{ID: 998511, DurationSec: 30, IntervalSec: 1, TypeKnown: true, Type: domain.StatusGood, EffectHitBP: 10000, LegendaryPowerID: "ZS-A04"}
			if kind == "damage" {
				base.HPChanges = []domain.ResourceChange{{Value: -10}}
				base.Type = domain.StatusBad
			} else {
				base.MPChanges = []domain.ResourceChange{{Value: 10}}
			}
			producer := func(c *legendaryContext) {
				countB++
				if !s.applyStatusDef(m, base, p.ID) {
					t.Fatal("B status rejected")
				}
				m.Status.Each(func(a domain.Active) {
					if a.Def.ID == base.ID {
						saved = a
					}
				})
			}
			handlerA, handlerB := legendaryHandler{revision: "synergy-v1"}, legendaryHandler{revision: "synergy-v1"}
			if kind == "damage" {
				handlerA.PeriodicResolved = func(*legendaryContext, domain.Active) { countA++ }
				handlerB.PeriodicResolved = func(c *legendaryContext, _ domain.Active) { producer(c) }
			} else {
				handlerA.ManaRestored = func(*legendaryContext, int32, int32) { countA++ }
				handlerB.ManaRestored = func(c *legendaryContext, _ int32, _ int32) { producer(c) }
			}
			legendaryHandlers["ZS-A03"], legendaryHandlers["ZS-A04"] = handlerA, handlerB
			original := domain.Active{Source: p.ID, Def: base}
			original.Def.ID = 998510
			original.Def.LegendaryPowerID = "ZS-A03"
			original.Def.CombatCausalPath = []string{s.combatEffectKey(p.ID, "ZS-A03")}
			m.MaxMP = 1000
			tick := func(a domain.Active) { m.MP = 0; s.tickStatus(m, a) }
			tick(original)
			if countA != 0 || countB != 1 {
				t.Fatalf("A -> B %d/%d", countA, countB)
			}
			// Saved status JSON retains the cycle barrier, not only a stack-local guard.
			data, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			var restored domain.Active
			if err = json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			tick(restored)
			if countA != 0 || countB != 1 {
				t.Fatalf("restored A -> B -> A escaped %d/%d", countA, countB)
			}
			// A later tick from the original producer is a sibling branch: B is allowed.
			tick(original)
			if countA != 0 || countB != 2 {
				t.Fatalf("sibling tick starved %d/%d", countA, countB)
			}
			original.Def.CombatCausalPath = nil
			original.Def.LegendaryPowerID = ""
			tick(original)
			if countA != 1 || countB != 3 {
				t.Fatalf("fresh root starved %d/%d", countA, countB)
			}
			if len(s.legendaryState().causalPath) != 0 {
				t.Fatal("scope leaked")
			}
			if kind == "damage" {
				found := false
				for _, out := range s.outbox {
					if hit, ok := out.ev.(event.DamageDealt); ok && hit.Dst == m.ID {
						found = true
						if !hit.Flag.Has(event.DamageMagic) {
							t.Fatal("periodic damage lost magic type")
						}
					}
				}
				if !found {
					t.Fatal("no damage event")
				}
			}
		})
	}
}

func TestStatusCausalDelayedTrapKeepsProducerAndImmutableSource(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "CK-A13", "CK-A26")
	oldA, oldB := legendaryHandlers["CK-A13"], legendaryHandlers["CK-A26"]
	t.Cleanup(func() { legendaryHandlers["CK-A13"], legendaryHandlers["CK-A26"] = oldA, oldB })
	countA, countB := 0, 0
	def, ok := s.skills.Get(10213, 1)
	if !ok {
		t.Fatal("trap skill absent")
	}
	source := s.captureCombatSourceV5(p, false)
	legendaryHandlers["CK-A13"] = legendaryHandler{revision: "synergy-v1", AfterHit: func(*legendaryContext) { countA++ }}
	legendaryHandlers["CK-A26"] = legendaryHandler{revision: "synergy-v1", AfterHit: func(c *legendaryContext) { countB++; s.placeAssassinTrap(p, def, m.Pos, 100, 100, false, source) }}
	restore := s.withCombatCausalPath([]string{s.combatEffectKey(p.ID, "CK-A13")})
	s.placeAssassinTrap(p, def, m.Pos, 100, 100, false, source)
	restore()
	if len(source.CombatCausalPath) != 0 {
		t.Fatal("accepted native source mutated")
	}
	if len(s.traps) != 1 {
		t.Fatalf("initial traps %d", len(s.traps))
	}
	var first *entity.Entity
	for _, tr := range s.traps {
		first = tr
	}
	if len(first.Trap.SourceOffense.CombatCausalPath) != 1 {
		t.Fatal("first trap ancestry missing")
	}
	s.triggerAssassinTrap(first, false)
	if countA != 0 || countB != 1 {
		t.Fatalf("delayed A -> B %d/%d", countA, countB)
	}
	if len(s.traps) != 1 {
		t.Fatalf("B child traps %d", len(s.traps))
	}
	var second *entity.Entity
	for _, tr := range s.traps {
		second = tr
	}
	if len(second.Trap.SourceOffense.CombatCausalPath) != 2 {
		t.Fatalf("child ancestry %v", second.Trap.SourceOffense.CombatCausalPath)
	}
	s.triggerAssassinTrap(second, false)
	if countA != 0 || countB != 1 || len(s.traps) != 0 {
		t.Fatalf("delayed cycle escaped %d/%d traps%d", countA, countB, len(s.traps))
	}
	if len(s.legendaryState().causalPath) != 0 {
		t.Fatal("trap scope leaked")
	}
}

func TestStatusCausalPlayerIdentityStableAfterEntityChange(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A03")
	oldID := p.ID
	oldKey := s.combatEffectKey(oldID, "ZS-A03")
	if p.Player.Char.ID == 0 {
		t.Fatal("fixture must use persistent character ID")
	}
	delete(s.entities, oldID)
	p.ID = oldID + 10000
	s.entities[p.ID] = p
	if key := s.combatEffectKey(p.ID, "ZS-A03"); key != oldKey {
		t.Fatalf("persistent key changed %q -> %q", oldKey, key)
	}
	restore := s.withCombatCausalPath([]string{oldKey})
	defer restore()
	if !s.combatEffectActive(p.ID, "ZS-A03") {
		t.Fatal("saved causal cycle barrier lost across entity change")
	}
}

func TestStatusCausalNativeLeechDoesNotReenterThroughPower(t *testing.T) {
	for _, kind := range []string{"life", "mana"} {
		t.Run(kind, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, "ZS-A03")
			old := legendaryHandlers["ZS-A03"]
			t.Cleanup(func() { legendaryHandlers["ZS-A03"] = old })
			reactions := 0
			handler := legendaryHandler{revision: "synergy-v1"}
			react := func(c *legendaryContext) { reactions++; c.Damage(m, 100, false, false) }
			def := domain.StatusDef{ID: 998530, DurationSec: 60}
			if kind == "life" {
				def.LifeStealPct = 100
				def.LifeStealChance = 100
				handler.Healed = func(c *legendaryContext, _ int32, _ int32) { react(c) }
			} else {
				def.ManaStealPct = 100
				def.ManaStealChance = 100
				handler.ManaRestored = func(c *legendaryContext, _ int32, _ int32) { react(c) }
			}
			legendaryHandlers["ZS-A03"] = handler
			p.Status.Apply(def, s.tick, p.ID)
			p.HP = 1000
			p.MaxHP = 10000
			p.MP = 0
			p.MaxMP = 10000
			ev := event.DamageDealt{Src: p.ID, Dst: m.ID, Amount: 100, Origin: domain.DamageNormalOrigin}
			beforeHP, beforeMP, beforeTarget := p.HP, p.MP, m.HP
			s.afterStrikeStatuses(p, m, ev, 100, 0)
			if reactions != 1 || beforeTarget-m.HP != 100 {
				t.Fatalf("leech -> power failed reactions=%d damage=%d", reactions, beforeTarget-m.HP)
			}
			if kind == "life" && p.HP-beforeHP != 100 {
				t.Fatalf("recursive life leech gained %d", p.HP-beforeHP)
			}
			if kind == "mana" && p.MP-beforeMP != 100 {
				t.Fatalf("recursive mana leech gained %d", p.MP-beforeMP)
			}
			s.afterStrikeStatuses(p, m, ev, 100, 0)
			if reactions != 2 {
				t.Fatalf("new attack leech starved %d", reactions)
			}
			if len(s.legendaryState().causalPath) != 0 {
				t.Fatal("leech scope leaked")
			}
		})
	}
}

func TestStatusCausalMissingSourceDoesNotImpersonateVictim(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	old := legendaryHandlers["ZS-A03"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"] = old })
	calls := 0
	legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", Healed: func(*legendaryContext, int32, int32) { calls++ }, PeriodicResolved: func(*legendaryContext, domain.Active) { calls++ }, ManaRestored: func(*legendaryContext, int32, int32) { calls++ }}
	delete(s.entities, m.ID)
	p.HP = p.MaxHP - 100
	p.MaxMP = 100
	p.MP = 0
	for _, amount := range []int32{-10, 10} {
		s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 998540, HPChanges: []domain.ResourceChange{{Value: amount}}, MPChanges: []domain.ResourceChange{{Value: 10}}}})
	}
	if calls != 0 {
		t.Fatalf("missing source impersonated victim and fired %d outgoing callbacks", calls)
	}
}

func TestStatusCausalNestedPeriodicPreservesCaller(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A03")
	old := legendaryHandlers["ZS-A03"]
	t.Cleanup(func() { legendaryHandlers["ZS-A03"] = old })
	count := 0
	legendaryHandlers["ZS-A03"] = legendaryHandler{revision: "synergy-v1", PeriodicResolved: func(*legendaryContext, domain.Active) { count++ }}
	active := domain.Active{Source: p.ID, Def: domain.StatusDef{ID: domain.StatusPoison, HPChanges: []domain.ResourceChange{{Value: -10}}}}
	restore := s.withCombatCausalPath([]string{s.combatEffectKey(p.ID, "ZS-A03")})
	s.tickStatus(m, active)
	restore()
	if count != 0 {
		t.Fatal("nested immediate periodic settlement discarded caller ancestry")
	}
	s.tickStatus(m, active)
	if count != 1 {
		t.Fatal("independent periodic root starved")
	}
}

func TestStatusCausalMagicDefenseAfterSourceLeaves(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		want  int32
	}{{"JK-D06", 10111, 65}, {"JK-D07", 10113, 50}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, tc.id, true)
			p.Stats = domain.Stats{}
			s.legendaryCastReleased(p, domain.SkillDef{ID: tc.skill}, 0, nil)
			delete(s.entities, m.ID)
			before := p.HP
			s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 998550, HPChanges: []domain.ResourceChange{{Value: -100}}}})
			if got := before - p.HP; got != tc.want {
				t.Fatalf("missing-source magic reduction got %d want %d", got, tc.want)
			}
		})
	}
}

func TestStatusCausalDifferentProducerTrapCanTriple(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "CK-A13", "CK-A26")
	def, ok := s.skills.Get(10213, 1)
	if !ok {
		t.Fatal("missing trap skill")
	}
	restore := s.withCombatCausalPath([]string{s.combatEffectKey(p.ID, "CK-A26")})
	s.placeAssassinTrap(p, def, m.Pos, 100, 100, false)
	restore()
	if len(s.traps) != 3 {
		t.Fatalf("different producer -> triple got%d want3 (never9)", len(s.traps))
	}
	side := 0
	for _, tr := range s.traps {
		if len(tr.Trap.SourceOffense.CombatCausalPath) == 2 {
			side++
		}
		if tr.Trap.SourceOffense.CombatCausalPath[0] != s.combatEffectKey(p.ID, "CK-A26") {
			t.Fatal("original producer lost")
		}
	}
	if side != 2 {
		t.Fatalf("triple side branches=%d", side)
	}
	if len(s.legendaryState().causalPath) != 0 {
		t.Fatal("placement scope leaked")
	}
}

func TestStatusCausalAcceptedTrapPlacementRestoresProducer(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "CK-A13")
	def, ok := s.skills.Get(10213, 1)
	if !ok {
		t.Fatal("missing trap skill")
	}
	source := s.captureCombatSourceV5(p, false)
	source.CombatCausalPath = []string{s.combatEffectKey(p.ID, "CK-A13")}
	s.placeAssassinTrap(p, def, m.Pos, 100, 100, false, source)
	if len(s.traps) != 1 {
		t.Fatalf("accepted source reentered same triple producer: %d traps", len(s.traps))
	}
	if len(s.legendaryState().causalPath) != 0 {
		t.Fatal("accepted placement scope leaked")
	}
}

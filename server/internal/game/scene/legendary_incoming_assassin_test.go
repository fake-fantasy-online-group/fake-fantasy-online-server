package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
	"time"
)

func incomingAssassinCondition(s *Scene, p, source *entity.Entity, id string) {
	switch id {
	case "CK-D01":
		p.Status.Apply(domain.StatusDef{ID: domain.StatusStealth, SourceSkillID: 10208, DurationSec: 30, Invisible: true}, s.tick, p.ID)
	case "CK-D02":
		source.Status.Apply(domain.StatusDef{ID: domain.StatusBlind, SourceSkillID: 10224, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrHit, Value: -10}}}, s.tick, p.ID)
	case "CK-D07":
		source.Status.Apply(domain.StatusDef{ID: domain.StatusStun, SourceSkillID: 10219, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun}, s.tick, p.ID)
	}
}
func incomingAssassinHit(s *Scene, p, source *entity.Entity, kind string, b combat.Blow) {
	switch kind {
	case "dot", "proc_dot":
		d := domain.StatusDef{ID: 199930, SourceSkillID: b.SkillID, DamageSourceRange: b.SourceSkillRange, DamageSourceTrap: b.Trap || b.SourceTrap, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -1000}}}
		if kind == "proc_dot" {
			d.LegendaryPowerID = "test-payload"
		}
		s.tickStatus(p, domain.Active{Source: source.ID, Def: d})
	case "reflection":
		s.emitReflectedDamage(source, p, 1000, false)
	default:
		if kind != "native" {
			b.Origin, b.ProcDepth = domain.DamageProcOrigin, 1
		}
		b.LegendaryChild = kind == "child"
		if kind == "set_proc" {
			s.equipmentState().classSetTriggered = 1
		}
		s.resolveStrikeStatuses(source, p, &combat.HitTracker{}, b)
	}
}
func TestLegendaryIncomingAssassinAllDamageConsumers(t *testing.T) {
	for _, id := range []string{"CK-D01", "CK-D02", "CK-D06", "CK-D07", "CK-D10"} {
		for _, kind := range []string{"native", "proc", "child", "set_proc", "dot", "proc_dot", "reflection"} {
			t.Run(id+"/"+kind, func(t *testing.T) {
				s, p, m := legendaryAssassinFixture(t, id)
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				incomingAssassinCondition(s, p, m, id)
				b := combat.Blow{SkillID: 199931, GuaranteedHit: true}
				if id == "CK-D06" {
					b.SourceSkillRange = 300
				}
				if id == "CK-D10" {
					b.Trap = true
				}
				before := p.HP
				incomingAssassinHit(s, p, m, kind, b)
				want := int32(800)
				// A mirror does not inherit the reflected attack's range or trap category.
				if kind == "reflection" && (id == "CK-D06" || id == "CK-D10") {
					want = 1000
				}
				if before-p.HP != want {
					t.Fatalf("loss%d want%d", before-p.HP, want)
				}
				if s.legendaryState().depth != 0 || s.legendaryState().child != nil {
					t.Fatal("recursive state leaked")
				}
			})
		}
	}
}
func TestLegendaryIncomingAssassinCombinationAndShieldOnce(t *testing.T) {
	for _, pair := range [][2]string{{"CK-D01", "CK-D02"}, {"CK-D01", "CK-D07"}, {"CK-D02", "CK-D07"}} {
		for _, kind := range []string{"native", "proc", "child", "dot", "proc_dot", "reflection"} {
			t.Run(pair[0]+"+"+pair[1]+"/"+kind, func(t *testing.T) {
				s, p, m := legendaryAssassinFixture(t, pair[0])
				incomingEquipPower(t, s, p, pair[1], domain.SlotHead)
				for _, id := range pair {
					incomingAssassinCondition(s, p, m, id)
				}
				p.Status.Apply(domain.StatusDef{ID: 199932, DurationSec: 30, PhysicalDamageReductionPct: 20, DamageShield: 100}, s.tick, p.ID)
				s.refreshStats(p)
				p.HP = p.MaxHP
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				before := p.HP
				incomingAssassinHit(s, p, m, kind, combat.Blow{GuaranteedHit: true})
				// Native20%, then two independent20% powers, then shield once.
				if before-p.HP != 412 || universalShieldValue(p) != 0 {
					t.Fatalf("loss%d shield%d want412/0", before-p.HP, universalShieldValue(p))
				}
			})
		}
	}
}

func TestLegendaryIncomingAssassinNativeStealthTiming(t *testing.T) {
	for _, kind := range []string{"native", "dot", "reflection", "owner_attacks"} {
		t.Run(kind, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, "CK-D01")
			s.skills, s.statuses = realV5SkillFixtures(t)
			incomingNativeCatalog(t, s)
			d, _ := s.skills.Get(10208, 1)
			if !s.applySkill(p, p, d, 0, false) || !entityInvisible(p) {
				t.Fatal("native stealth not applied")
			}
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
			if kind == "owner_attacks" {
				s.prepareBasicAttackStatuses(p, &combat.Blow{})
				if entityInvisible(p) {
					t.Fatal("attack did not consume native stealth")
				}
			}
			before := p.HP
			if kind == "native" {
				ev, raw, r := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
				s.afterStrikeStatuses(m, p, ev, raw, r)
			} else {
				incomingAssassinHit(s, p, m, kind, combat.Blow{GuaranteedHit: true})
			}
			want := int32(800)
			if kind == "owner_attacks" {
				want = 1000
			}
			if before-p.HP != want {
				t.Fatalf("loss%d want%d", before-p.HP, want)
			}
			// Follow the real native break rule, never add a new break/retention rule.
			stillInvisible := entityInvisible(p)
			before = p.HP
			incomingAssassinHit(s, p, m, "dot", combat.Blow{})
			want = 1000
			if stillInvisible {
				want = 800
			}
			if before-p.HP != want {
				t.Fatalf("post-transition loss%d want%d", before-p.HP, want)
			}
		})
	}
}
func TestLegendaryIncomingAssassinDebuffOwnerAndRemoval(t *testing.T) {
	for _, id := range []string{"CK-D02", "CK-D07"} {
		for _, kind := range []string{"dot", "proc", "reflection"} {
			t.Run(id+"/"+kind, func(t *testing.T) {
				s, p, m := legendaryAssassinFixture(t, id)
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				other := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
				incomingAssassinCondition(s, other, m, id)
				before := p.HP
				incomingAssassinHit(s, p, m, kind, combat.Blow{GuaranteedHit: true})
				if before-p.HP != 1000 {
					t.Fatal("borrowed other caster condition")
				}
				incomingAssassinCondition(s, p, m, id)
				before = p.HP
				incomingAssassinHit(s, p, m, kind, combat.Blow{GuaranteedHit: true})
				if before-p.HP != 800 {
					t.Fatal("own condition missing")
				}
				m.Status = domain.NewStatusSet()
				before = p.HP
				incomingAssassinHit(s, p, m, kind, combat.Blow{GuaranteedHit: true})
				if before-p.HP != 1000 {
					t.Fatal("condition survived removal")
				}
			})
		}
	}
}
func TestLegendaryIncomingAssassinRangedProvenance(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-D06")
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
	m.Look.AtkDist = 500
	for _, tc := range []struct {
		name string
		b    combat.Blow
		want int32
	}{
		{"native ranged basic", combat.Blow{}, 800},
		{"unknown skill", combat.Blow{SkillID: 199933}, 1000},
		{"unknown payload zero skill", combat.Blow{LegendaryPayload: true, Origin: domain.DamageProcOrigin, ProcDepth: 1}, 1000},
		{"unknown child zero skill", combat.Blow{LegendaryChild: true, Origin: domain.DamageProcOrigin, ProcDepth: 1}, 1000},
		{"ground zero skill", combat.Blow{Origin: domain.DamageAuraOrigin}, 1000},
		{"short skill", combat.Blow{SkillID: 199933, SourceSkillRange: 100}, 1000},
		{"ranged skill", combat.Blow{SkillID: 199933, SourceSkillRange: 101}, 800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.HP = 10000
			b := tc.b
			b.GuaranteedHit = true
			finish := s.beginEquipmentRoot(p.ID, m.ID, s.equipmentOffense(p))
			s.equipment.root.def = domain.SkillDef{ID: 199933, Dist: 900}
			ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
			finish()
			if ev.Amount != tc.want {
				t.Fatalf("amount%d want%d", ev.Amount, tc.want)
			}
		})
	}
	for _, distance := range []int32{100, 300} {
		p.HP = 10000
		d := domain.SkillDef{ID: 199934, Level: 1, Dist: distance, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
		if !s.applySkill(m, p, d, 0, false) {
			t.Fatal("monster skill rejected")
		}
		want := int32(1000)
		if distance > 100 {
			want = 800
		}
		if 10000-p.HP != want {
			t.Fatalf("monster range%d loss%d want%d", distance, 10000-p.HP, want)
		}
	}
}
func TestLegendaryIncomingAssassinRealMonsterTrapDOT(t *testing.T) {
	for _, triggered := range []bool{false, true} {
		t.Run(map[bool]string{false: "nontrap same skill", true: "actual trap"}[triggered], func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, "CK-D10")
			s.refreshStats(p)
			p.HP = p.MaxHP
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, Hit: 100000}
			status := domain.StatusDef{ID: 199935, Level: 1, DurationSec: 30, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -1000}}}
			s.statuses[domain.StatusKey{ID: status.ID, Level: 1}] = status
			d := domain.SkillDef{ID: 199936, Level: 1, Dist: 300, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}, Statuses: []domain.StatusApplication{{ID: status.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}}
			before := p.HP
			if triggered {
				s.placeTrap(m, d, p.Pos, 100, 100)
				s.stepTraps()
				if len(s.traps) != 0 {
					t.Fatal("trap did not resolve")
				}
			} else {
				s.applySkill(m, p, trapPayload(d), 100, false)
			}
			want := int32(1000)
			if triggered {
				want = 800
			}
			if before-p.HP != want {
				t.Fatalf("direct trap loss%d want%d", before-p.HP, want)
			}
			var a domain.Active
			p.Status.Each(func(x domain.Active) {
				if x.Def.ID == status.ID {
					a = x
				}
			})
			if a.Def.ID == 0 || a.Def.DamageSourceTrap != triggered || a.Def.DamageSourceRange != 300 {
				t.Fatalf("bad provenance:%+v", a.Def)
			}
			p.HP = p.MaxHP
			before = p.HP
			s.tickStatus(p, a)
			if before-p.HP != want {
				t.Fatalf("trap DOT loss%d want%d", before-p.HP, want)
			}
			delete(p.Player.Char.Skills, 10229)
			p.HP = p.MaxHP
			before = p.HP
			s.tickStatus(p, a)
			if before-p.HP != 1000 {
				t.Fatal("unlearned trap defense persisted")
			}
		})
	}
}
func TestLegendaryIncomingAssassinDOTProvenancePersistence(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-D06")
	incomingEquipPower(t, s, p, "CK-D10", domain.SlotHead)
	p.Stats = domain.Stats{}
	def := domain.StatusDef{ID: 199937, Level: 1, DurationSec: 30, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, DamageSourceRange: 300, DamageSourceTrap: true, HPChanges: []domain.ResourceChange{{Value: -1000}}}
	p.Status.Apply(def, s.tick, m.ID)
	wall := time.Unix(1700000000, 0)
	saved := domain.CloneSavedStatuses(p.Status.SaveForLogout(s.tick, wall, p.ID))
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []domain.SavedStatus
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	p.Status = domain.RestoreSavedStatuses(decoded, s.tick, wall, p.ID).Clone()
	var a domain.Active
	p.Status.Each(func(x domain.Active) {
		if x.Def.ID == def.ID {
			a = x
		}
	})
	if a.Source != 0 || a.Def.DamageSourceRange != 300 || !a.Def.DamageSourceTrap {
		t.Fatalf("restore lost or forged provenance:%+v", a)
	}
	before := p.HP
	s.tickStatus(p, a)
	if before-p.HP != 640 {
		t.Fatalf("restored loss%d want640", before-p.HP)
	}
	unknown := def
	unknown.DamageSourceRange = 0
	unknown.DamageSourceTrap = false
	res := p.Status.ApplyWithOverlay(unknown, s.tick, m.ID, domain.StatusOverlay{{Existing: def.ID, Incoming: def.ID}: domain.OverlayReplace})
	if !res.Applied {
		t.Fatal("replacement rejected")
	}
	p.Status.Each(func(x domain.Active) {
		if x.Def.ID == def.ID {
			a = x
		}
	})
	before = p.HP
	s.tickStatus(p, a)
	if before-p.HP != 1000 {
		t.Fatal("replacement borrowed old provenance")
	}
	var legacy domain.StatusDef
	if err = json.Unmarshal([]byte(`{"ID":199938,"SourceSkillID":10214,"HPChanges":[{"Value":-1000}]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	before = p.HP
	s.tickStatus(p, domain.Active{Def: legacy})
	if before-p.HP != 1000 {
		t.Fatal("legacy unknown guessed as ranged/trap")
	}
}
func TestLegendaryIncomingAssassinPersistentRefreshPreservesCategory(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-D06")
	s.skills, s.statuses = realV5SkillFixtures(t)
	incomingNativeCatalog(t, s)
	p.Player.Char.Skills[10214] = 5
	d, ok := s.skills.Get(10214, 5)
	if !ok || len(d.Statuses) == 0 {
		t.Fatal("native poison trap absent")
	}
	d.SnapshotRawRanks = domain.Learned{10214: 5}
	d.SnapshotHardRanks = d.SnapshotRawRanks.Clone()
	old, ok := s.resolveSkillStatusDef(d, d.Statuses[0], 0)
	if !ok {
		t.Fatal("native trap status unresolved")
	}
	old.DamageSourceRange = 300
	old.DamageSourceTrap = true
	p.Player.Char.Skills[10214] = 1
	for _, known := range []bool{true, false} {
		copy := old
		if !known {
			copy.DamageSourceRange = 0
			copy.DamageSourceTrap = false
		}
		got, keep, changed := s.resolvePersistentStatus(p, m, copy)
		if !keep || !changed || got.DamageSourceRange != copy.DamageSourceRange || got.DamageSourceTrap != copy.DamageSourceTrap {
			t.Fatalf("refresh changed category known%v keep%v changed%v got%+v", known, keep, changed, got)
		}
	}
}

// Legal player -> monster casts exercise actual payload/child mirror return.
// The pure defensive pass must work inside generation guards, never recurse.
func TestLegendaryIncomingAssassinActualPayloadMirror(t *testing.T) {
	for _, id := range []string{"CK-D01", "CK-D02", "CK-D07"} {
		for _, kind := range []string{"payload", "child"} {
			t.Run(id+"/"+kind, func(t *testing.T) {
				s, p, m := legendaryAssassinFixture(t, id)
				incomingAssassinCondition(s, p, m, id)
				p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
				p.Status.Apply(domain.StatusDef{ID: 199940, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, p.ID)
				m.Status.Apply(domain.StatusDef{ID: 199941, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, m.ID)
				c := assassinContext(s, p, m, 10401, 0)
				c.Def = domain.SkillDef{ID: 10401, Level: 1, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 300, HurtType: domain.HurtFire, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
				before, targetBefore := p.HP, m.HP
				if kind == "payload" {
					c.Damage(m, 1000, true, false)
				} else if !c.ChildSkill(m.ID, c.Def, legendaryChildOptions{BaseDamage: 1000, TargetOnly: true, SuppressGeneration: true}) {
					t.Fatal("child rejected")
				}
				if before-p.HP != 800 || m.HP != targetBefore {
					t.Fatalf("reflection loss%d targetloss%d", before-p.HP, targetBefore-m.HP)
				}
				if s.legendaryState().depth != 0 || s.legendaryState().child != nil {
					t.Fatal("recursive guard leaked")
				}
			})
		}
	}
}

func TestLegendaryIncomingAssassinActualRiftHazardIsNotRangedBasicOrTrap(t *testing.T) {
	for _, id := range []string{"CK-D06", "CK-D10"} {
		t.Run(id, func(t *testing.T) {
			s, m, p, _ := riftAbilityFixture(t, riftEruption)
			p.Player.Char.Race = domain.Assassin
			p.Player.Char.Level = 100
			p.Player.Char.Employed = true
			p.Player.Char.EmploymentKnown = true
			p.Player.Char.Skills = domain.Learned{10229: 1}
			m.Look.AtkDist = 900
			h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: domain.Stats{MinAtk: 100, MaxAtk: 100}, Pos: p.Pos}
			p.HP = 10000
			s.hitRiftHazard(h, p, 100)
			baseline := 10000 - p.HP
			incomingEquipPower(t, s, p, id, domain.SlotBody)
			p.HP = 10000
			s.hitRiftHazard(h, p, 100)
			if baseline <= 0 || 10000-p.HP != baseline {
				t.Fatalf("hazard misclassified before%d after%d", baseline, 10000-p.HP)
			}
		})
	}
}

func TestLegendaryIncomingAssassinTrapLineageDoesNotReplayNativeHalf(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-D10")
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
	s.skills = domain.SkillTable{}
	s.skills[domain.SkillKey{ID: 10229, Level: 1}] = domain.SkillDef{ID: 10229, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveModifiers: []domain.PassiveModifier{{Kind: domain.PassiveTrapHalfDamageChance, Value: 100}}}
	if s.trapHalfDamageChance(p) != 10000 {
		t.Fatal("native half fixture inactive")
	}
	for _, tc := range []struct {
		name string
		b    combat.Blow
		want int32
	}{
		{"native trap halves then fixed defense", combat.Blow{Trap: true}, 400},
		{"derived trap only fixed defense", combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, LegendaryPayload: true, SourceTrap: true}, 800},
		{"unknown proc gets neither", combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, LegendaryPayload: true}, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.HP = 10000
			b := tc.b
			b.GuaranteedHit = true
			ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
			if ev.Amount != tc.want {
				t.Fatalf("amount%d want%d", ev.Amount, tc.want)
			}
		})
	}
}

func TestLegendaryIncomingAssassinUnknownSourceDoesNotBorrowCondition(t *testing.T) {
	for _, id := range []string{"CK-D01", "CK-D02", "CK-D07"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, id)
			p.Stats = domain.Stats{}
			incomingAssassinCondition(s, p, m, id)
			before := p.HP
			s.tickStatus(p, domain.Active{Source: 0, Def: domain.StatusDef{ID: 199945, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
			want := int32(1000)
			if id == "CK-D01" {
				want = 800
			}
			if before-p.HP != want {
				t.Fatalf("unknown source loss%d want%d", before-p.HP, want)
			}
		})
	}
}

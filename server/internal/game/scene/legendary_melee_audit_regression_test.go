package scene

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func meleeAuditNativeScene(t *testing.T, race domain.Race, learned domain.Learned, powers ...string) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s, p, m := nativeRepairFixture(t, race, learned, powers...)
	stack := p.Player.Worn.At(domain.SlotWeapon)
	if stack.Empty() {
		stack = domain.Stack{UID: 969999, Item: 969999, Count: 1, Durability: 100, MaxDurability: 100}
		s.items[stack.Item] = domain.ItemDef{ID: stack.Item, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), Durable: 100}}
		p.Player.Worn.Set(domain.SlotWeapon, stack)
	}
	item := s.items[stack.Item]
	item.Equip.Type = 1
	if race == domain.Swordsman {
		item.Equip.Type = 6
	}
	item.Equip.AttackDurabilityCostRaw = 100
	s.items[stack.Item] = item
	return s, p, m
}

func meleeAuditCast(t *testing.T, s *Scene, p *entity.Entity, skill domain.SkillID, target domain.EntityID) domain.SkillDef {
	t.Helper()
	s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, Target: target})
	if len(s.casts) != 1 {
		t.Fatalf("native skill %d rejected: %+v", skill, s.outbox)
	}
	d := s.casts[0].def
	if !s.casts[0].releaseAtWall.IsZero() {
		s.stepCastsAt(s.casts[0].releaseAtWall, false)
	} else {
		s.stepCasts()
	}
	if len(s.casts) > 0 && !s.casts[0].resolveAtWall.IsZero() {
		s.stepCastsAt(s.casts[0].resolveAtWall, false)
	}
	if len(s.casts) != 0 {
		t.Fatalf("native skill %d did not settle", skill)
	}
	return d
}

func meleeAuditEquipSet(t *testing.T, s *Scene, p *entity.Entity, class int32) {
	t.Helper()
	suit := &domain.EquipmentSuit{ID: class, EnhancedClass: class}
	for i, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotBody, domain.SlotHead, domain.SlotGlove} {
		stack := p.Player.Worn.At(slot)
		if stack.Empty() {
			id := domain.ItemID(930000 + i)
			s.items[id] = domain.ItemDef{ID: id, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100}}
			stack = domain.Stack{UID: int64(id), Item: id, Count: 1, Durability: 100, MaxDurability: 100}
			p.Player.Worn.Set(slot, stack)
		}
		item := s.items[stack.Item]
		copy := *item.Equip
		copy.Suit = suit
		item.Equip = &copy
		s.items[stack.Item] = item
		suit.Groups = append(suit.Groups, []domain.ItemID{stack.Item})
	}
	s.invalidateCombatAttributes(p)
}

func TestLegendaryWarriorAuditArmorBeforeNativeHit(t *testing.T) {
	for _, tc := range []struct {
		power string
		skill domain.SkillID
	}{{"ZS-A07", 10005}, {"ZS-A25", 10022}, {"ZS-A25", 10023}} {
		for pieces := 0; pieces <= 3; pieces++ {
			t.Run(fmt.Sprintf("%s/%d/%d", tc.power, tc.skill, pieces), func(t *testing.T) {
				powers := []string{}
				if pieces > 0 {
					powers = append(powers, tc.power)
				}
				if pieces >= 2 {
					powers = append(powers, "ZS-A08")
				}
				if pieces == 3 {
					powers = append(powers, "ZS-D02")
				}
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{tc.skill: 1, 10009: 1}, powers...)
				p.Stats.Hit = 70
				m.Stats.Def = 100
				s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
				// Same next strike: old Def100 misses; native armor Def70/55/48 hits.
				tr := &combat.HitTracker{}
				tr.Engage(m.ID, 170, nil)
				tr.Roll(70, 100)
				s.hits[p.ID] = tr
				before := m.HP
				meleeAuditCast(t, s, p, tc.skill, m.ID)
				if pieces == 0 {
					if tc.skill != 10023 && m.HP != before {
						t.Fatal("baseline unexpectedly hit")
					}
					if m.Status.Has(10009) {
						t.Fatal("baseline borrowed armor")
					}
					return
				}
				if m.HP >= before || !m.Status.Has(10009) {
					t.Fatal("native hit did not benefit from preceding armor", m.HP, m.Stats)
				}
				want := int32(70)
				if pieces == 2 {
					want = 55
				}
				if pieces == 3 {
					want = 48
				}
				if m.Stats.Def != want {
					t.Fatalf("armor composition defense=%d want%d", m.Stats.Def, want)
				}
			})
		}
	}
}

func TestLegendaryWarriorAuditArmorImmunityAndUnlearned(t *testing.T) {
	for _, mode := range []string{"unlearned", "skill-immune", "status-immune", "miss"} {
		t.Run(mode, func(t *testing.T) {
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10005: 1, 10009: 1}, "ZS-A07")
			m.Stats.Def = 100
			def := domain.MonsterDef{ID: m.Monster.TypeID, Stats: m.Stats}
			if mode == "unlearned" {
				delete(p.Player.Char.Skills, 10009)
			}
			if mode == "skill-immune" {
				def.ImmuneSkills = map[domain.SkillID]struct{}{10009: {}}
			}
			if mode == "status-immune" {
				m.Status.Apply(domain.StatusDef{ID: 990001, Level: 1, DurationSec: 60, BlockHarmful: true}, s.tick, m.ID)
			}
			if mode == "miss" {
				p.Stats.Hit = 0
			}
			s.defs = spawn.MapDefs{m.Monster.TypeID: def}
			before := m.HP
			meleeAuditCast(t, s, p, 10005, m.ID)
			if applied := m.Status.Has(10009); applied != (mode == "miss") {
				t.Fatal("armor gating mismatch", applied)
			}
			if mode == "miss" && m.HP != before {
				t.Fatal("armor unexpectedly guarantees hits")
			}
		})
	}
}

func TestLegendaryWarriorAuditManaConversionCombinations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra []string
		want  int32
	}{{"single", nil, 220}, {"universal", []string{"TY-A19"}, 316}, {"three-drain", []string{"TY-A19", "ZS-U02"}, 388}, {"three-damage", []string{"TY-A19", "ZS-A09"}, 395}} {
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/empty=%v", tc.name, empty), func(t *testing.T) {
				powers := append([]string{"ZS-A10"}, tc.extra...)
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10013: 1}, powers...)
				m.MP, m.MaxMP = 1000, 1000
				if empty {
					m.MP = 0
				}
				before := m.HP
				d := meleeAuditCast(t, s, p, 10013, m.ID)
				if got := before - m.HP; got != tc.want {
					t.Fatalf("damage=%d want%d def=%+v", got, tc.want, d.DirectEffects())
				}
				if empty && m.MP != 0 {
					t.Fatal("negative mana")
				}
				if !empty {
					wantDrain := int32(60) * (10000 + d.LegendaryModifiers.ManaDrainBP) / 10000
					if m.MP != 1000-wantDrain {
						t.Fatal("conversion and actual drain differ", m.MP, wantDrain)
					}
				}
			})
		}
	}
}

func TestLegendaryWarriorAuditNoDurabilityWholeCast(t *testing.T) {
	for pieces := 0; pieces <= 3; pieces++ {
		for _, set := range []bool{false, true} {
			t.Run(fmt.Sprintf("powers=%d/set=%v", pieces, set), func(t *testing.T) {
				powers := []string{}
				if pieces > 0 {
					powers = append(powers, "ZS-A17")
				}
				if pieces >= 2 {
					powers = append(powers, "ZS-A16")
				}
				if pieces == 3 {
					powers = append(powers, "ZS-A29")
				}
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10003: 1}, powers...)
				if set {
					meleeAuditEquipSet(t, s, p, 5)
					if !s.classSetDragonActive(p) {
						t.Fatal("dragon set inactive")
					}
				}
				q := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
				before := p.Player.Worn.At(domain.SlotWeapon)
				mp, hp := p.MP, p.HP
				beforeM, beforeQ := m.HP, q.HP
				meleeAuditCast(t, s, p, 10003, m.ID)
				after := p.Player.Worn.At(domain.SlotWeapon)
				if pieces == 0 {
					if after.Durability != before.Durability-5 || after.DurabilityWearRaw != before.DurabilityWearRaw+100 {
						t.Fatal("native costs changed", before, after)
					}
				} else if before.Durability != after.Durability || before.DurabilityWearRaw != after.DurabilityWearRaw {
					t.Fatal("cast wore weapon", before, after)
				}
				if set {
					if p.MP != mp || p.HP != hp-classSetBloodCost(hp)+(beforeM-m.HP)/5+(beforeQ-q.HP)/5 {
						t.Fatal("set cost changed", p.MP, p.HP)
					}
				} else if p.MP != mp-35 {
					t.Fatal("native mana changed", p.MP)
				}
				if pieces >= 2 && q.HP == 10000 {
					t.Fatal("combined area did not hit")
				}
			})
		}
	}
}

// Zero draws visit the native 2% success branch without changing its probability.
type meleeAuditZeroSource struct{}

func (meleeAuditZeroSource) Int63() int64 { return 0 }
func (meleeAuditZeroSource) Seed(int64)   {}

func TestLegendarySwordsmanAuditNativeInstantDeathBurst(t *testing.T) {
	for _, mode := range []string{"instant-death", "ordinary-kill", "low-health", "immune", "miss", "two-powers", "three-powers", "class-set"} {
		t.Run(mode, func(t *testing.T) {
			powers := []string{"JK-A23"}
			if mode == "two-powers" || mode == "three-powers" {
				powers = append(powers, "JK-A30")
			}
			if mode == "three-powers" {
				powers = append(powers, "JK-A22")
			}
			s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10123: 1}, powers...)
			s.rng = rand.New(meleeAuditZeroSource{})
			if mode == "class-set" {
				meleeAuditEquipSet(t, s, p, 4)
			}
			if mode == "ordinary-kill" {
				m.HP = 100
				m.MaxHP = 100
			}
			if mode == "low-health" {
				m.HP = m.MaxHP / 2
			}
			if mode == "immune" {
				m.Status.Apply(domain.StatusDef{ID: 990001, Level: 1, DurationSec: 60, InstantDeathImmune: true}, s.tick, m.ID)
			}
			if mode == "miss" {
				p.Stats.Hit = 0
				m.Stats.Def = 100
			}
			q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			far := putMonster(s, 902, 10000, domain.Stats{}, 281, 100)
			d := meleeAuditCast(t, s, p, 10123, m.ID)
			if len(d.DirectEffects()) != 2 || d.DirectEffects()[1].ChanceBP != 200 {
				t.Fatal("native death chance changed")
			}
			want := int32(181)
			if mode == "immune" || mode == "miss" {
				want = 0
			}
			if mode == "three-powers" {
				want += 165 + 115
			}
			if got := 10000 - q.HP; got != want {
				t.Fatalf("burst amount/count=%d want%d", got, want)
			}
			if far.HP != 10000 {
				t.Fatal("burst escaped150 radius")
			}
			if mode != "immune" && mode != "miss" && m.Alive() {
				t.Fatal("native kill did not happen")
			}
		})
	}
}

func TestLegendaryWarriorAuditNativeA23PrimaryAndSet(t *testing.T) {
	for _, selected := range []bool{false, true} {
		for _, set := range []bool{false, true} {
			t.Run(fmt.Sprintf("selected=%v/set=%v", selected, set), func(t *testing.T) {
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10022: 1}, "ZS-A23", "ZS-A25")
				if set {
					meleeAuditEquipSet(t, s, p, 5)
				}
				target := domain.EntityID(0)
				if selected {
					target = m.ID
				}
				before := m.HP
				meleeAuditCast(t, s, p, 10022, target)
				want := int32(493)
				if set {
					want = 526
				}
				if got := before - m.HP; got != want {
					t.Fatalf("native bonus=%d want%d", got, want)
				}
			})
		}
	}
}

// A14's original specification uses the primary's actual damage plus absorption;
// retain the original primary-only break payload. Preserve that
// baseline while proving A13 expansion and native silence remain independent.
func TestLegendarySwordsmanAuditA14PrimaryShieldContract(t *testing.T) {
	for _, mode := range []string{"single", "expanded", "three-powers", "primary-unshielded", "all-unshielded", "shield-cap"} {
		t.Run(mode, func(t *testing.T) {
			powers := []string{"JK-A14"}
			if mode != "single" {
				powers = append(powers, "JK-A13")
			}
			if mode == "three-powers" {
				powers = append(powers, "JK-D07")
			}
			if mode == "shield-cap" {
				powers = append(powers, "TY-A40")
			}
			s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10113: 1}, powers...)
			q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			q.Status = domain.NewStatusSet()
			shield := domain.StatusDef{ID: 990001, Level: 1, DurationSec: 60, DamageShield: 10000}
			if mode != "primary-unshielded" && mode != "all-unshielded" {
				m.Status.Apply(shield, s.tick, m.ID)
			}
			if mode != "all-unshielded" {
				q.Status.Apply(shield, s.tick, q.ID)
			}
			if mode == "shield-cap" {
				p.Status.Apply(shield, s.tick, p.ID)
			}
			before := m.HP
			meleeAuditCast(t, s, p, 10113, m.ID)
			left := func(e *entity.Entity) int32 {
				var n int32
				e.Status.EachEffective(func(a domain.Active) { n += a.ShieldLeft })
				return n
			}
			direct := int32(263)
			if mode == "single" {
				direct = 155
			}
			if mode == "shield-cap" {
				direct = 368
			}
			switch mode {
			case "primary-unshielded", "all-unshielded":
				if before-m.HP != 394 {
					t.Fatal("primary unshielded damage changed", before-m.HP)
				}
			default:
				extra := min(direct*120/100, int32(400))
				if left(m) != 10000-direct-extra || m.HP != before {
					t.Fatal("primary shield contract changed", left(m), m.HP)
				}
			}
			if mode == "all-unshielded" {
				if q.HP != 10000-394 {
					t.Fatal("secondary unshielded bonus lost", q.HP)
				}
			} else if mode == "single" {
				if left(q) != 10000 {
					t.Fatal("single target expanded")
				}
			} else if left(q) != 10000-direct {
				t.Fatal("secondary shield received invented break", left(q))
			}
			if !m.Status.Has(1003) || mode != "single" && !q.Status.Has(1003) {
				t.Fatal("native silence lost")
			}
		})
	}
}

func TestLegendarySwordsmanAuditNativeTauntReachAndSet(t *testing.T) {
	for _, mode := range []string{"native", "radius", "three-powers", "class-set"} {
		t.Run(mode, func(t *testing.T) {
			powers := []string{}
			if mode != "native" {
				powers = append(powers, "JK-U04")
			}
			if mode == "three-powers" || mode == "class-set" {
				powers = append(powers, "JK-A25", "JK-D04")
			}
			s, p, _ := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10127: 1}, powers...)
			if mode == "class-set" {
				meleeAuditEquipSet(t, s, p, 4)
			}
			edge := putMonster(s, 901, 10000, domain.Stats{}, 280, 100)
			outside := putMonster(s, 902, 10000, domain.Stats{}, 281, 100)
			edgePos, outsidePos := edge.Pos, outside.Pos
			d := meleeAuditCast(t, s, p, 10127, 0)
			if edge.Pos != edgePos || outside.Pos != outsidePos {
				t.Fatal("radius-only taunt invented instantaneous gathering")
			}
			wantRadius := int32(100)
			if mode != "native" {
				wantRadius = 180
			}
			if d.Dist != 0 || d.Area.Center != domain.SkillAreaCenterCaster || d.Area.Radius != wantRadius {
				t.Fatal("invented remote taunt center or distance", d.Dist, d.Area)
			}
			if (edge.Monster.Target == p.ID) != (mode != "native") || outside.Monster.Target != 0 {
				t.Fatal("native radius admission changed")
			}
			if mode == "three-powers" || mode == "class-set" {
				if edge.HP != 9840 {
					t.Fatal("taunt bonus damage lost", edge.HP)
				}
				edge.Stats.MinAtk, edge.Stats.MaxAtk = 100, 100
				ev, _, _ := s.resolveStrikeStatuses(edge, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
				want := int32(50)
				if mode == "class-set" {
					want = 47
				}
				if ev.Amount != want {
					t.Fatal("taunted-enemy mitigation lost", ev.Amount, want)
				}
			}
			if mode == "class-set" && s.classSetUnrulyStacks(p) != 1 {
				t.Fatal("taunt set stack lost")
			}
		})
	}
}

func TestLegendarySwordsmanNativeDisplacementRemainsMonsterOnly(t *testing.T) {
	s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{}, "JK-U08")
	before := p.Pos
	if s.knockbackMonster(m, p, 100) || p.Pos != before {
		t.Fatal("equipped player displaced")
	}
	p.Player.Worn = &domain.EquipSet{}
	if s.knockbackMonster(m, p, 100) || p.Pos != before {
		t.Fatal("native path unexpectedly accepts players")
	}
	// Movement immunity must not widen native displacement eligibility.
	if !s.knockbackMonster(p, m, 100) {
		t.Fatal("the same native consumer must still move a monster")
	}
}

func TestLegendaryWarriorAuditD03NativeVulnerabilityWithoutArmor(t *testing.T) {
	s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10009: 1, 10012: 1}, "ZS-D03")
	// Native defense and passive vulnerability roll independently. This seed
	// resists the former and accepts the latter without replacing either payload.
	s.rng = rand.New(rand.NewSource(1))
	m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, StatusResist: 50}
	s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
	meleeAuditCast(t, s, p, 10009, m.ID)
	if m.Status.Has(10009) || !m.Status.Has(10012) {
		t.Fatal("did not exercise native independent status resistance branch")
	}
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	if ev.Amount != 100 {
		t.Fatalf("vulnerability alone incorrectly satisfies armor requirement: %d", ev.Amount)
	}
}

func TestLegendaryWarriorAuditD03ActualArmorSource(t *testing.T) {
	for _, mode := range []string{"own-native", "borrowed-a07", "borrowed-a25", "foreign", "expired"} {
		t.Run(mode, func(t *testing.T) {
			powers := []string{"ZS-D03"}
			skill := domain.SkillID(10009)
			if mode == "borrowed-a07" {
				powers = append(powers, "ZS-A07", "ZS-A08")
				skill = 10005
			}
			if mode == "borrowed-a25" {
				powers = append(powers, "ZS-A25", "ZS-A08")
				skill = 10022
			}
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10009: 1, skill: 1}, powers...)
			m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
			s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
			meleeAuditCast(t, s, p, skill, m.ID)
			if !m.Status.Has(10009) {
				t.Fatal("real native armor not applied")
			}
			if mode == "expired" {
				m.Status.Expire(s.tick + domain.Ticks(16000))
			}
			if mode == "foreign" {
				var armor domain.StatusDef
				m.Status.EachEffective(func(a domain.Active) {
					if a.Def.ID == 10009 {
						armor = a.Def
					}
				})
				m.Status = domain.NewStatusSet()
				m.Status.Apply(armor, s.tick, p.ID+100)
			}
			ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
			want := int32(50)
			if mode == "foreign" || mode == "expired" {
				want = 100
			}
			if ev.Amount != want {
				t.Fatalf("incoming=%d want%d", ev.Amount, want)
			}
		})
	}
}

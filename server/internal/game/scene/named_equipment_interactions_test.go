package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
	"time"
)

func TestNamedEquipmentProcSharedGroupIndependentAndCausal(t *testing.T) {
	for _, mode := range []string{"both", "first-in-path", "unequipped", "wrong-trigger"} {
		t.Run(mode, func(t *testing.T) {
			s := newCombatScene(t)
			join(t, s, 1, 100, "proc", 100, 100)
			p := s.players[1]
			p.Stats = domain.Stats{}
			m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			s.skills = skillTable(domain.SkillDef{ID: 10401, Level: 1, HurtType: domain.HurtFire, Dist: 450, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamageFlat: 100, DamagePctDenominator: 1, GuaranteedHit: true}})
			cached := s.equipmentEffects(p)
			for i, id := range []string{"first", "second"} {
				cached.effects = append(cached.effects, domain.ActiveEquipmentEffect{SourceUID: int64(10 + i), Effect: domain.EquipmentEffect{Kind: "proc", Proc: &domain.EquipmentProc{ID: id, SharedGroup: "same", Trigger: "on_direct_hit", SkillID: 10401, FixedRank: 1, ChanceBP: 10000, ICDMS: 4000, Payload: "damage", TargetRule: "enemy", ScaleBP: 10000, PVPScaleBP: 10000}}})
			}
			if mode == "first-in-path" {
				for _, a := range cached.effects {
					a.Effect.Proc.ICDMS = 0
				}
			}
			if mode == "unequipped" {
				cached.effects = nil
			}
			s.equipmentState().cache[p.ID] = cached
			blow := combat.Blow{SkillID: 10402, Magic: true}
			if mode == "first-in-path" {
				blow.EffectCausalPath = []string{s.combatEffectKey(p.ID, "equipment:first")}
			}
			hit := equipmentHit{src: p.ID, dst: m.ID, blow: blow, ev: event.DamageDealt{Amount: 1}}
			if mode == "wrong-trigger" {
				hit.ev.Amount = 0
			}
			s.executeEquipmentRoot(&equipmentRoot{id: 1, src: p.ID, primary: m.ID, hits: []equipmentHit{hit}})
			want := int32(200)
			if mode == "first-in-path" {
				want = 100
			}
			if mode == "unequipped" || mode == "wrong-trigger" {
				want = 0
			}
			if got := 10000 - m.HP; got != want {
				t.Fatalf("damage=%d want=%d", got, want)
			}
			if len(s.legendaryState().causalPath) != 0 {
				t.Fatal("causal path leaked")
			}
		})
	}
}

func TestNamedEquipmentProcReadinessIgnoresLegacySharedCooldown(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "proc", 100, 100)
	p := s.players[1]
	wearEquipmentV5(t, s, p, 1306, 9001)
	now := time.Unix(1700000000, 0)
	s.now = func() time.Time { return now }
	p.Player.Char.EquipmentProcCooldowns = map[string]int64{"global": now.Add(time.Hour).UnixMilli()}
	for _, a := range s.equipmentEffects(p).effects {
		if a.Effect.Proc != nil {
			p.Player.Char.EquipmentProcCooldowns["group:"+a.Effect.Proc.SharedGroup] = now.Add(time.Hour).UnixMilli()
		}
	}
	views := s.equipmentProcReadiness(p)
	if len(views) == 0 {
		t.Fatal("fixture has no proc")
	}
	for _, v := range views {
		if !v.Ready || v.RemainingMS != 0 || v.GlobalRemainingMS != 0 {
			t.Fatalf("stale legacy gate: %+v", v)
		}
	}
}

func TestNamedWeaponHealingReceivedAndCausalGuard(t *testing.T) {
	for _, base := range []domain.ItemID{1323, 1325} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/blocked%t", base, blocked), func(t *testing.T) {
				s, p, _, _ := universalTestContext(t, "TY-D12")
				p.MaxHP = 1000
				p.HP = 1
				d := domain.SkillDef{ID: 10320, SpecialWeaponBaseID: base}
				if base == 1325 {
					d.ID = 10316
				}
				if blocked {
					defer s.withCombatCausalPath([]string{s.combatEffectKey(p.ID, fmt.Sprintf("special-weapon:%d:heal", base))})()
				}
				s.specialWeaponHeal(p, p, 100, d)
				want := int32(141)
				if blocked {
					want = 1
				}
				if p.HP != want {
					t.Fatalf("HP=%d want=%d", p.HP, want)
				}
			})
		}
	}
}

func TestNamedWeaponStealCoexistsWithGroupSteal(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U02")
	other := putMonster(s, 9991, 10000, domain.Stats{}, 140, 100)
	other.Status = domain.NewStatusSet()
	for _, id := range []domain.EntityID{c.Target.ID, other.ID} {
		m := s.entities[id]
		m.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, m.ID)
		m.Status.AbsorbDamage(90)
	}
	// This removable native buff is outside the legendary defensive whitelist.
	other.Status.Apply(domain.StatusDef{ID: 1999001, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, Affixes: []domain.Affix{{Attr: domain.AttrDef, Value: 7}}}, s.tick, other.ID)
	d := domain.SkillDef{ID: 10433, SpecialWeaponBaseID: 1324, TargetEnemy: true, Effects: []domain.SkillEffect{{RemoveStatusIDs: []domain.StatusID{1021, 1999001}, DispelChanceBP: 10000}}}
	finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, s.equipmentOffense(c.Owner))
	s.equipment.root.def = d
	s.equipment.root.targets = []domain.EntityID{c.Target.ID, other.ID}
	s.applySkill(c.Owner, c.Target, d, 100, false)
	s.applySkill(c.Owner, other, d, 100, false)
	finish()
	count := 0
	c.Owner.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1021 {
			count++
			if a.ShieldLeft != 100 {
				t.Fatal("fresh shield downgraded", a.ShieldLeft)
			}
		}
	})
	if count != 1 || !c.Owner.Status.Has(1999001) || other.Status.Has(1999001) || other.Status.Has(1021) || c.Target.Status.Has(1021) {
		t.Fatal("native and legendary transfer failed", count)
	}
}

func TestNamedWeaponHealingActualCastReceivedPower(t *testing.T) {
	for _, base := range []domain.ItemID{1323, 1325} {
		for _, mode := range []string{"absent", "broken", "equipped"} {
			for _, power := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/power%t", base, mode, power), func(t *testing.T) {
					skill := domain.SkillID(10320)
					if base == 1325 {
						skill = 10316
					}
					s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{skill: 5})
					if mode != "absent" {
						specialWeaponEquip(t, s, p, base)
						if mode == "broken" {
							st := p.Player.Worn.At(domain.SlotWeapon)
							st.Durability = 0
							p.Player.Worn.Set(domain.SlotWeapon, st)
						}
					}
					if power {
						s.items[990999] = domain.ItemDef{ID: 990999, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), LevelReq: 100, Durable: 100}}
						p.Player.Worn.Set(domain.SlotBody, domain.Stack{UID: 990999, Item: 990999, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: "TY-D12"}})
					}
					s.invalidateCombatAttributes(p)
					s.refreshStats(p)
					p.HP = 1
					p.MP = p.MaxMP
					if base == 1323 {
						p.Status.Apply(domain.StatusDef{ID: domain.StatusDamageShield, Level: 1, DurationSec: 30, DamageShield: 100, TypeKnown: true, Type: domain.StatusGood}, s.tick, p.ID)
					} else {
						p.Status.Apply(domain.StatusDef{ID: 1006, Level: 1, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}, s.tick, p.ID)
					}
					want := int32(0)
					if mode == "equipped" {
						want = 60
						if base == 1325 {
							want = p.MaxHP * 3 / 100
						}
						if power {
							want = want * 140 / 100
						}
					}
					nativeRepairCast(t, s, p, skill, p.ID)
					if p.HP != 1+want {
						t.Fatalf("actual cast healed %d want %d", p.HP-1, want)
					}
				})
			}
		}
	}
}

// This is a reaction-pipeline contract test, not independent catalogue coverage:
// production IncomingHeal stays installed while observers verify both callbacks.
func TestNamedWeaponHealingReactionContract(t *testing.T) {
	original := legendaryHandlers["TY-D12"]
	defer func() { legendaryHandlers["TY-D12"] = original }()
	s, p, _, _ := universalTestContext(t, "TY-D12")
	p.HP = 1
	p.MaxHP = 1000
	d := domain.SkillDef{ID: 10320, SpecialWeaponBaseID: 1323}
	healed, resolved := 0, 0
	observed := original
	observed.Healed = func(c *legendaryContext, before, amount int32) {
		healed++
		if before != 1 || amount != 140 {
			t.Fatalf("healed event before=%d amount=%d", before, amount)
		}
		s.specialWeaponHeal(p, p, 100, d)
	}
	observed.HealResolved = func(c *legendaryContext, raw, effective, overflow int32) {
		resolved++
		if raw != 140 || effective != 140 || overflow != 0 {
			t.Fatalf("resolved event %d/%d/%d", raw, effective, overflow)
		}
	}
	legendaryHandlers["TY-D12"] = observed
	s.specialWeaponHeal(p, p, 100, d)
	if healed != 1 || resolved != 1 || p.HP != 141 || len(s.legendaryState().causalPath) != 0 {
		t.Fatalf("callbacks=%d/%d hp=%d path=%v", healed, resolved, p.HP, s.legendaryState().causalPath)
	}
}

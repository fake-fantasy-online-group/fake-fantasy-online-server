package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func legendaryAssassinFixture(t *testing.T, id string) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s := newCombatScene(t)
	s.statuses = domain.StatusTable{}
	s.alloc = domain.NewEntityAlloc()
	join(t, s, 1, 100, "assassin", 100, 100)
	p := s.players[1]
	p.Player.Char.Race = domain.Assassin
	p.Player.Char.Level = 100
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.Skills = domain.Learned{}
	for i := domain.SkillID(10201); i <= 10230; i++ {
		p.Player.Char.Skills[i] = 1
	}
	p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
	p.HP = 10000
	p.MaxHP = 10000
	p.MP = 500
	p.MaxMP = 1000
	slot := domain.SlotWeapon
	if id[3] == 'D' {
		slot = domain.SlotBody
	}
	if s.items == nil {
		s.items = map[domain.ItemID]domain.ItemDef{}
	}
	s.items[900002] = domain.ItemDef{ID: 900002, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 4}}}
	p.Player.Worn.Set(slot, domain.Stack{UID: 992, Item: 900002, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
	m := putMonster(s, 900, 100000, domain.Stats{}, 130, 100)
	m.Status = domain.NewStatusSet()
	s.monsters[m.ID] = m
	if len(s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})) != 1 {
		t.Fatal("inactive assassin fixture", id)
	}
	return s, p, m
}
func assassinContext(s *Scene, p, m *entity.Entity, skill domain.SkillID, root uint64) legendaryContext {
	c := s.legendaryContexts(p, m, combat.Blow{SkillID: int32(skill), RootID: root}, event.DamageDealt{Src: p.ID, Dst: m.ID, SkillID: int32(skill), Amount: 100})[0]
	return c
}

func assassinAddPower(s *Scene, p *entity.Entity, id string, slot domain.EquipSlot) {
	item := domain.ItemID(910000 + slot)
	s.items[item] = domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 4}}}
	p.Player.Worn.Set(slot, domain.Stack{UID: int64(2000 + slot), Item: item, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
}
func assassinNativeHits(s *Scene, p *entity.Entity, d domain.SkillDef, primary domain.EntityID, ids []domain.EntityID) {
	finish := s.beginEquipmentRoot(p.ID, primary, s.equipmentOffense(p))
	s.equipment.root.def = d
	s.equipment.root.targets = ids
	for _, id := range ids {
		s.applySkill(p, s.entities[id], s.legendaryTargetDefinition(p, d, s.entities[id], primary, ids), 100, false)
	}
	finish()
}
func TestLegendaryAssassinSynergyFanLifesteal(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A20")
	assassinAddPower(s, p, "CK-A19", domain.SlotHead)
	assassinAddPower(s, p, "CK-A22", domain.SlotBody)
	side := putMonster(s, 901, 10000, domain.Stats{}, 140, 120)
	s.monsters[side.ID] = side
	rear := putMonster(s, 902, 10000, domain.Stats{}, 70, 100)
	s.monsters[rear.ID] = rear
	d := domain.SkillDef{ID: 10209, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 80, RequiresInvisible: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.RequiresInvisible {
		t.Fatal("backstab still requires stealth")
	}
	ids := s.legendarySelectTargets(p, d, m.ID, []domain.EntityID{m.ID})
	if len(ids) != 2 {
		t.Fatalf("fan targets=%v", ids)
	}
	p.HP = 5000
	before := m.HP
	assassinNativeHits(s, p, d, m.ID, ids)
	if before-m.HP != 125 || side.HP != 9875 || rear.HP != 10000 {
		t.Fatalf("native fan damage: primary=%d side=%d rear=%d", before-m.HP, side.HP, rear.HP)
	}
	if p.HP != 5036 {
		t.Fatalf("effective hit lifesteal=%d want36", p.HP-5000)
	}
}
func TestLegendaryAssassinSynergyTripleInstantBomb(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A11")
	assassinAddPower(s, p, "CK-A12", domain.SlotHead)
	assassinAddPower(s, p, "CK-A13", domain.SlotBody)
	d := domain.SkillDef{ID: 10213, Dist: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &d)
	radius := d.DirectEffects()[0].TrapRadius
	if radius != 160 {
		t.Fatalf("additive radius=%d", radius)
	}
	before := m.HP
	mp := p.MP
	s.placeTrap(p, d, m.Pos, radius, 100)
	if got := before - m.HP; got != 690 {
		t.Fatalf("three native230 blasts=%d", got)
	}
	if len(s.traps) != 0 {
		t.Fatalf("instant children left %d traps", len(s.traps))
	}
	if p.MP != mp {
		t.Fatal("placement charged MP again")
	}
	s.stepTraps()
	if before-m.HP != 690 {
		t.Fatal("recursive or delayed bomb damage")
	}
}
func TestLegendaryAssassinSynergyCircleNativePoison(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A06")
	assassinAddPower(s, p, "CK-A10", domain.SlotHead)
	assassinAddPower(s, p, "CK-A09", domain.SlotBody)
	// This victim was behind the original cone and outside the original100 radius.
	m.Pos = domain.Pos{MapID: 7, X: -10, Y: 100}
	s.aoi.Move(m, domain.Pos{X: 130, Y: 100})
	p.Status.Apply(domain.StatusDef{ID: 1018, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 120, PoisonOnHitChance: 25, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true}, s.tick, p.ID)
	s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}] = domain.StatusDef{ID: domain.StatusPoison, Level: 21, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	d := domain.SkillDef{ID: 10206, Kind: domain.SkillArea, TargetEnemy: true, Radius: 100, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 60}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.legendaryPrepareSkill(p, &d)
	ids := s.resolveTargets(p, d, m.ID, nil)
	if len(ids) != 1 || ids[0] != m.ID {
		t.Fatalf("expanded native circle=%v", ids)
	}
	before := m.HP
	assassinNativeHits(s, p, d, m.ID, ids)
	if before-m.HP != 150 {
		t.Fatalf("native throw damage=%d", before-m.HP)
	}
	if !m.Status.Has(domain.StatusPoison) {
		t.Fatal("expanded target did not receive native quench poison")
	}
	before = m.HP
	poisonTicks(s, 1)
	if before-m.HP != 20 {
		t.Fatalf("same poison tick=%d", before-m.HP)
	}
	p.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == 1018 && a.Def.PoisonOnHitChance != 25 {
			t.Fatal("native ordinary hit probability mutated")
		}
	})
}

func TestLegendaryAssassinSynergyStandaloneBombAndNoChain(t *testing.T) {
	for _, id := range []string{"CK-A11", "CK-A12", "CK-A13"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, id)
			d := domain.SkillDef{ID: 10213, Dist: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100}}}
			s.legendaryPrepareSkill(p, &d)
			before := m.HP
			s.placeTrap(p, d, m.Pos, d.DirectEffects()[0].TrapRadius, 100)
			count := len(s.traps)
			want := 1
			if id == "CK-A11" {
				want = 0
			}
			if id == "CK-A13" {
				want = 3
			}
			if count != want {
				t.Fatalf("native placement count=%d want%d", count, want)
			}
			s.stepTraps()
			damage := int32(140)
			if id == "CK-A12" {
				damage = 190
			}
			if id == "CK-A13" {
				damage = 300
			}
			if before-m.HP != damage {
				t.Fatalf("native damage=%d want%d", before-m.HP, damage)
			}
			if len(s.traps) != 0 {
				t.Fatal("overlapping native traps failed to trigger")
			}
		})
	}
}
func TestLegendaryAssassinSynergyNativePoisonAmplifiesOnce(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A09")
	p.Status.Apply(domain.StatusDef{ID: 1018, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 120, PoisonOnHitChance: 100, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true}, s.tick, p.ID)
	s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}] = domain.StatusDef{ID: domain.StatusPoison, Level: 21, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	if !s.applyCombatTriggeredStatus(p, m, domain.StatusPoison) {
		t.Fatal("standalone native poison failed")
	}
	var copied domain.StatusDef
	m.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoison {
			copied = a.Def
		}
	})
	legendaryAssassinAmplifyPoison(&copied)
	if copied.LegendaryModifiers.DamageTickBP != 10000 {
		t.Fatalf("copied poison amplified twice: %+v", copied.LegendaryModifiers)
	}
	before := m.HP
	poisonTicks(s, 1)
	if before-m.HP != 20 {
		t.Fatalf("native basic poison tick=%d", before-m.HP)
	}
}
func TestLegendaryAssassinSynergyLifestealUsesEffectiveDamage(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A22")
	p.HP = 9999
	m.HP = 10
	d := domain.SkillDef{ID: 10209, Kind: domain.SkillSingle, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 1000}}
	assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
	if p.HP != 10000 {
		t.Fatalf("overkill/missing health clamp=%d", p.HP)
	}
}

func TestLegendaryAssassinSynergyStaticPreparation(t *testing.T) {
	for _, tc := range []struct {
		id             string
		skill          domain.SkillID
		damage, mp, cd int32
	}{{"CK-A03", 10221, 180, 100, 10000}, {"CK-A04", 10205, 150, 100, 10000}, {"CK-A17", 10209, 150, 100, 10000}, {"CK-A24", 10203, 150, 100, 10000}, {"CK-A29", 10210, 180, 100, 10000}, {"CK-U05", 10206, 100, 55, 10000}, {"CK-U07", 10213, 100, 100, 10000}, {"CK-U09", 10221, 100, 70, 8000}, {"CK-D06", 10213, 100, 70, 10000}, {"CK-D09", 10208, 100, 70, 10000}, {"CK-D07", 10219, 100, 100, 7000}, {"CK-A23", 10219, 135, 100, 6000}, {"CK-U03", 10218, 100, 0, 10000}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, tc.id)
			d := domain.SkillDef{ID: tc.skill, Kind: domain.SkillSingle, TargetEnemy: true, MPCost: 100, CooldownMS: 10000, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			s.legendaryPrepareSkill(p, &d, m.ID)
			if d.MPCost != tc.mp || d.CooldownMS != tc.cd {
				t.Fatalf("cost/cd=%d/%d", d.MPCost, d.CooldownMS)
			}
			if tc.skill == 10218 {
				return
			}
			before := m.HP
			assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
			if before-m.HP != tc.damage {
				t.Fatalf("native damage=%d want%d", before-m.HP, tc.damage)
			}
		})
	}
}
func TestLegendaryAssassinSynergyNativeStealthTrapBudget(t *testing.T) {
	s, p, _ := legendaryAssassinFixture(t, "CK-U08")
	stealth := domain.StatusDef{ID: domain.StatusStealth, Invisible: true, SourceSkillID: 10208, TypeKnown: true, Type: domain.StatusGood, DurationSec: 10, BreakOnAttack: true}
	if !s.applyStatusDef(p, stealth, p.ID) {
		t.Fatal("stealth apply")
	}
	for i := 0; i < 3; i++ {
		d := domain.SkillDef{ID: 10213}
		s.legendaryPrepareSkill(p, &d)
		if d.PreserveInvisibleChanceBP != 10000 {
			t.Fatalf("placement%d preserve%d", i, d.PreserveInvisibleChanceBP)
		}
		s.prepareSkillStatuses(p, &d)
		s.legendaryCasterAccepted(p, d, 0, nil, 0)
	}
	if !entityInvisible(p) {
		t.Fatal("trap consumed native stealth")
	}
	// A fresh native stealth instance restores exactly2, not an external timer.
	s.applyStatusDef(p, stealth, p.ID)
	var charges int32
	p.Status.EachEffective(func(a domain.Active) {
		if a.Def.Invisible {
			charges = a.Def.LegendaryStealthTrapCharges
			if a.ExpireAt-s.tick != domain.Ticks(15000) {
				t.Fatal("duration extension")
			}
		}
	})
	if charges != 0 {
		t.Fatal("fresh stealth budget", charges)
	}
}
func TestLegendaryAssassinSynergyPoisonDefenseAndSpread(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A08")
	assassinAddPower(s, p, "CK-A09", domain.SlotHead)
	next := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
	next.Status = domain.NewStatusSet()
	s.monsters[next.ID] = next
	d := domain.StatusDef{ID: domain.StatusPoison, Level: 21, ClassSetPoison: true, DurationSec: 10, IntervalSec: 2, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	s.applyStatusDef(m, d, p.ID)
	m.HP = 0
	s.legendaryAssassinDeath(m)
	var copy domain.Active
	next.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoison {
			copy = a
		}
	})
	if !copy.Def.LegendaryPoisonCopy || copy.Def.LegendaryModifiers.DamageTickBP != 10000 {
		t.Fatalf("wrong native copied poison: %+v", copy.Def)
	}
	before := next.HP
	poisonTicks(s, 1)
	if before-next.HP != 20 {
		t.Fatal("copied tick", before-next.HP)
	}
	last := putMonster(s, 902, 10000, domain.Stats{}, 150, 100)
	last.Status = domain.NewStatusSet()
	next.HP = 0
	s.legendaryAssassinDeath(next)
	if last.Status.Has(domain.StatusPoison) {
		t.Fatal("copied poison propagated recursively")
	}
}
func TestLegendaryAssassinSynergyEnemyAreaDisarm(t *testing.T) {
	s, p, _ := legendaryAssassinFixture(t, "CK-U03")
	enemy := putMonster(s, 905, 10000, domain.Stats{}, 150, 100)
	d := domain.SkillDef{ID: 10213}
	s.placeTrap(enemy, d, domain.Pos{MapID: 7, X: 140, Y: 100}, 50, 100)
	s.placeTrap(enemy, d, domain.Pos{MapID: 7, X: 160, Y: 100}, 50, 100)
	s.placeTrap(p, d, p.Pos, 50, 100)
	var target domain.EntityID
	for id, tr := range s.traps {
		if tr.Trap.Owner == enemy.ID {
			target = id
			tr.Trap.VisibleTo[p.ID] = struct{}{}
		}
	}
	s.disarmTrap(p, target, 7500)
	if len(s.traps) != 1 {
		t.Fatal("area enemy disarm failed", len(s.traps))
	}
	for _, tr := range s.traps {
		if tr.Trap.Owner != p.ID {
			t.Fatal("own bomb was remotely removed")
		}
	}
}

func TestLegendaryAssassinSynergyReplacementCritUsesNativeRules(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A02")
	p.Stats.Hit = 0
	m.Stats.Def = 1000000
	d := domain.SkillDef{ID: 10221, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	fx := d.DirectEffects()[0]
	if !fx.GuaranteedHit || !fx.GuaranteedCrit {
		t.Fatal("native guaranteed flags missing")
	}
	before := m.HP
	assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
	if before-m.HP != 200 {
		t.Fatalf("native crit=%d", before-m.HP)
	}
}
func TestLegendaryAssassinSynergyFrostNativePoisonSourceGuard(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A18")
	assassinAddPower(s, p, "CK-A09", domain.SlotHead)
	p.Status.Apply(domain.StatusDef{ID: 1018, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 120, PoisonOnHitChance: 25, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true}, s.tick, p.ID)
	s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}] = domain.StatusDef{ID: domain.StatusPoison, Level: 21, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	freeze := domain.StatusDef{ID: domain.StatusFreeze, Control: domain.ControlStun, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, BreakOnDamage: true, SourceSkillID: 10216}
	if !s.applyStatusDef(m, freeze, p.ID) || !m.Status.Has(domain.StatusPoison) {
		t.Fatal("native frost did not apply learned quench")
	}
	before := m.HP
	poisonTicks(s, 1)
	if before-m.HP != 20 || !m.Status.Has(domain.StatusFreeze) {
		t.Fatal("own native poison broke native freeze", before-m.HP)
	}

	// Learned quench suffices for A18 even when its active enchantment is absent.
	p.Status.Remove(domain.StatusPoisonHit)
	carrier := domain.StatusDef{ID: domain.StatusPoisonHit, Level: 1, DurationSec: 120, PoisonOnHitChance: 25, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true}
	s.statuses[domain.StatusKey{ID: domain.StatusPoisonHit, Level: 1}] = carrier
	s.skills = domain.SkillTable{{ID: 10211, Level: 1}: {ID: 10211, Level: 1, Statuses: []domain.StatusApplication{{ID: domain.StatusPoisonHit, Level: 1}}}}
	otherTarget := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
	otherTarget.Status = domain.NewStatusSet()
	if !s.applyStatusDef(otherTarget, freeze, p.ID) || !otherTarget.Status.Has(domain.StatusPoison) {
		t.Fatal("learned-only quench did not poison native freeze")
	}
	// Another poison owner cannot inherit the freeze protection.
	m.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == domain.StatusPoison })
	other := domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	s.applyStatusDef(m, other, 9999)
	poisonTicks(s, 1)
	if m.Status.Has(domain.StatusFreeze) {
		t.Fatal("foreign poison incorrectly protected freeze")
	}
}
func TestLegendaryAssassinSynergyDirectPullReplacesKnockback(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A30")
	assassinAddPower(s, p, "CK-A28", domain.SlotHead)
	m.Pos.X = 250
	s.aoi.Move(m, m.Pos)
	d := domain.SkillDef{ID: 10225, TargetEnemy: true, Kind: domain.SkillArea, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 200}, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}, {Kind: domain.EffectKnockback, KnockbackDistance: 120}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.Area.Radius != 300 || d.DirectEffects()[1].KnockbackDistance != 0 {
		t.Fatal("original knockback not replaced")
	}
	assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
	if got := m.Pos.X - p.Pos.X; got != 40 {
		t.Fatalf("direct pull distance=%f", got)
	}
}
func TestLegendaryAssassinSynergyStealthSpeedSharesStatus(t *testing.T) {
	s, p, _ := legendaryAssassinFixture(t, "CK-U10")
	d := domain.StatusDef{ID: domain.StatusStealth, Invisible: true, SourceSkillID: 10208, DurationSec: 10, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: 20, Mode: domain.ModePercent}}}
	s.applyStatusDef(p, d, p.ID)
	count, pct := 0, int32(0)
	p.Status.EachEffective(func(a domain.Active) {
		count++
		for _, af := range a.Def.Affixes {
			if af.Attr == domain.AttrMoveSpeed {
				pct += af.Value
			}
		}
	})
	if count != 1 || pct != 60 {
		t.Fatalf("native stealth count%d speed%d", count, pct)
	}
}

func TestLegendaryAssassinSynergyTheftPalmPreservesAttempt(t *testing.T) {
	for _, chance := range []int32{0, 10000} {
		t.Run(fmt.Sprint(chance), func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, "CK-A26")
			s.items[掉落树枝.ID] = 掉落树枝
			s.lootTable = map[domain.MonsterID][]domain.DropEntry{338: {{Item: 掉落树枝.ID, RatePct: 100}}}
			d := domain.SkillDef{ID: 10212, Level: 1, Kind: domain.SkillSingle, Name: "偷窃术", TargetEnemy: true, Dist: 75, Effects: []domain.SkillEffect{{Kind: domain.EffectStealItem, ChanceBP: chance, ChanceExplicit: true, TheftMaxSuccesses: 1}}}
			side := putMonster(s, 901, 10000, domain.Stats{}, 100, 250)
			s.monsters[side.ID] = side
			before := m.HP
			assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
			if before-m.HP != 240 || side.HP != 9760 {
				t.Fatalf("native palm primary%d side%d", before-m.HP, 10000-side.HP)
			}
			want := 0
			if chance == 10000 {
				want = 1
			}
			if len(s.ground) != want || int(m.Monster.TheftSuccesses) != want || side.Monster.TheftSuccesses != 0 {
				t.Fatal("palm changed theft loot or repeated theft", len(s.ground), m.Monster.TheftSuccesses, side.Monster.TheftSuccesses)
			}
			// Already-stolen or invalid target cannot produce another palm.
			m.Monster.TheftSuccesses = 1
			before = m.HP
			assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
			if m.HP != before {
				t.Fatal("invalid theft produced palm")
			}
		})
	}
}

func TestLegendaryAssassinSynergyCounterAreaNativeChanceAndHeal(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A24")
	assassinAddPower(s, p, "CK-A25", domain.SlotHead)
	assassinAddPower(s, p, "CK-D03", domain.SlotBody)
	p.HP = 5000
	s.skills = domain.SkillTable{{ID: 10203, Level: 1}: {ID: 10203, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveCounter: &domain.PassiveCounterAttack{ChanceBP: 10000, DamagePct: 100}}}
	side := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
	s.monsters[side.ID] = side
	at, ok := s.passiveCounterPending(p, m)
	if !ok {
		t.Fatal("native guaranteed chance fixture did not counter")
	}
	s.attacks = append(s.attacks, at)
	s.tick++
	before := m.HP
	s.stepCombatAt(s.now(), false)
	if before-m.HP != 150 || side.HP != 9850 || p.HP != 5044 {
		t.Fatalf("counter native damage/heal %d/%d/%d", before-m.HP, 10000-side.HP, p.HP-5000)
	}
	if len(s.attacks) != 0 {
		t.Fatal("area counter recursively queued counters")
	}
	d, _ := s.skills.Get(10203, 1)
	d.PassiveCounter = &domain.PassiveCounterAttack{ChanceBP: 0, DamagePct: 100}
	s.skills = domain.SkillTable{{ID: 10203, Level: 1}: d}
	if _, ok := s.passiveCounterPending(p, m); ok {
		t.Fatal("power made failed native counter guaranteed")
	}
}
func TestLegendaryAssassinSynergyGeometryAndPrimaryBonuses(t *testing.T) {
	for _, id := range []string{"CK-A01", "CK-A05", "CK-A07", "CK-A21", "CK-A27", "CK-U02", "CK-U04"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, id)
			d := domain.SkillDef{ID: 10221, Dist: 200, Radius: 100, TargetEnemy: true, Kind: domain.SkillSingle, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			switch id {
			case "CK-A05":
				d.ID = 10205
			case "CK-A07", "CK-A27":
				d.ID = 10206
				d.Kind = domain.SkillArea
				d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 60}
			case "CK-A21":
				d.ID = 10209
			case "CK-U02":
				d.ID = 10224
			case "CK-U04":
				d.ID = 10228
			}
			s.legendaryPrepareSkill(p, &d, m.ID)
			switch id {
			case "CK-A01":
				if d.Area.Shape != domain.SkillAreaRectangle || d.Area.Length != 200 {
					t.Fatal("native line missing")
				}
			case "CK-A05":
				for i := 0; i < 5; i++ {
					v := putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
					s.monsters[v.ID] = v
				}
				ids := s.legendarySelectTargets(p, d, m.ID, []domain.EntityID{m.ID})
				if len(ids) != 6 || ids[0] != m.ID {
					t.Fatal("primary plus3", ids)
				}
				assassinNativeHits(s, p, d, m.ID, ids)
				for _, v := range ids {
					if s.entities[v].HP != s.entities[v].MaxHP-100 {
						t.Fatal("secondary not native full damage")
					}
				}
			case "CK-A07", "CK-A21":
				before := m.HP
				assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
				want := int32(150)
				if id == "CK-A21" {
					want = 175
				}
				if before-m.HP != want {
					t.Fatal("primary bonus", before-m.HP)
				}
			case "CK-A27":
				if d.Dist != 300 || d.Area.Radius != 150 {
					t.Fatal("native range", d.Dist, d.Area.Radius)
				}
			case "CK-U02":
				if d.Dist != 300 || d.Area.Radius != 130 {
					t.Fatal("smoke native range")
				}
			case "CK-U04":
				if d.Radius != 150 {
					t.Fatal("detect range")
				}
				st := domain.Stats{MoveSpeed: 100}
				s.legendaryPassiveStats(p, &st)
				if st.MoveSpeed != 100 {
					t.Fatal("static speed")
				}
			}
		})
	}
}
func TestLegendaryAssassinSynergyNativeTrapVariants(t *testing.T) {
	for _, tc := range []struct {
		id             string
		skill          domain.SkillID
		damage, radius int32
	}{{"CK-A14", 10214, 100, 130}, {"CK-A16", 10215, 150, 130}, {"CK-A17", 10216, 150, 130}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, tc.id)
			d := domain.SkillDef{ID: tc.skill, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100}}}
			s.legendaryPrepareSkill(p, &d)
			if d.DirectEffects()[0].TrapRadius != tc.radius {
				t.Fatal("native radius")
			}
			before := m.HP
			s.placeTrap(p, d, m.Pos, tc.radius, 100)
			if tc.id == "CK-A14" && len(s.traps) != 0 {
				t.Fatal("poison mist not instant")
			}
			s.stepTraps()
			if before-m.HP != tc.damage {
				t.Fatal("trap damage", before-m.HP)
			}
		})
	}
	s, p, m := legendaryAssassinFixture(t, "CK-A15")
	d := domain.StatusDef{ID: domain.StatusPoison, SourceSkillID: 10214, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	s.applyStatusDef(m, d, p.ID)
	before := m.HP
	poisonTicks(s, 1)
	if before-m.HP != 20 {
		t.Fatal("same native trap poison", before-m.HP)
	}
}

func TestLegendaryAssassinSynergyNativeStaticDefenses(t *testing.T) {
	for _, id := range []string{"CK-D01", "CK-D02", "CK-D04", "CK-D06", "CK-D07", "CK-D10"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, id)
			m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
			b := combat.Blow{GuaranteedHit: true}
			switch id {
			case "CK-D01":
				p.Status.Apply(domain.StatusDef{ID: domain.StatusStealth, Invisible: true, SourceSkillID: 10208, DurationSec: 10}, s.tick, p.ID)
			case "CK-D02":
				m.Status.Apply(domain.StatusDef{ID: domain.StatusBlind, SourceSkillID: 10224, DurationSec: 10, Affixes: []domain.Affix{{Attr: domain.AttrHit, Value: -10}}}, s.tick, p.ID)
			case "CK-D04":
				m.Status.Apply(domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true, DurationSec: 10}, s.tick, p.ID)
			case "CK-D06":
				m.Look.AtkDist = 200
			case "CK-D07":
				s.legendaryCastReleased(p, domain.SkillDef{ID: 10219}, m.ID, []domain.EntityID{m.ID})
				m.Status.Apply(domain.StatusDef{ID: domain.StatusStun, Control: domain.ControlStun, SourceSkillID: 10219, DurationSec: 10}, s.tick, p.ID)
			case "CK-D10":
				b.Trap = true
			}
			ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
			want := int32(80)
			if id == "CK-D01" || id == "CK-D07" {
				want = 65
			}
			if ev.Amount != want {
				t.Fatalf("native mitigation=%d", ev.Amount)
			}
			if id == "CK-D01" {
				p.Status.Remove(domain.StatusStealth)
				ev, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
				if ev.Amount != 100 {
					t.Fatal("poststealth lingering mitigation")
				}
			}
			if id == "CK-D04" || id == "CK-D07" {
				m.Status = domain.NewStatusSet()
				if id == "CK-D07" {
					s.tick += domain.Ticks(8000)
				}
				ev, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
				if ev.Amount != 100 {
					t.Fatal("native status no longer exists but mitigation lingered")
				}
			}
		})
	}
	s, p, _ := legendaryAssassinFixture(t, "CK-D05")
	s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusStun, Control: domain.ControlStun, DurationSec: 10}, 0)
	p.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == domain.StatusStun && a.ExpireAt-s.tick != domain.Ticks(7000) {
			t.Fatal("fixed native control duration", a.ExpireAt-s.tick)
		}
	})
	s, p, _ = legendaryAssassinFixture(t, "CK-D09")
	st := domain.Stats{MaxHP: 1000}
	s.legendaryPassiveStats(p, &st)
	if st.MaxHP != 1200 {
		t.Fatal("static maxHP")
	}
}
func TestLegendaryAssassinSynergyNativeCleanseAndManaRefund(t *testing.T) {
	s, p, _ := legendaryAssassinFixture(t, "CK-D08")
	poison := domain.StatusDef{ID: domain.StatusPoison, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -100}}}
	p.Status.Apply(poison, s.tick, 900)
	before := p.HP
	poisonTicks(s, 1)
	if before-p.HP != 70 {
		t.Fatal("static poison reduction", before-p.HP)
	}
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10211}, p.ID, []domain.EntityID{p.ID}, 0)
	if p.Status.Has(domain.StatusPoison) {
		t.Fatal("active native quench did not cleanse")
	}
	s, p, _ = legendaryAssassinFixture(t, "CK-U01")
	p.MP = 200
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10208}, p.ID, []domain.EntityID{p.ID}, 20)
	if p.MP != 200 {
		t.Fatal("immediate refund actual paid ceiling", p.MP)
	}
}
func TestLegendaryAssassinSynergyBackstabNativeCooldown(t *testing.T) {
	for _, fatal := range []bool{false, true} {
		s, p, m := legendaryAssassinFixture(t, "CK-U06")
		p.Player.StartSkillCooldown(10208, s.tick, 100)
		if fatal {
			m.HP = 10
		}
		d := domain.SkillDef{ID: 10209, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
		want := domain.Tick(65)
		if fatal {
			want = 20
		}
		if !p.Player.SkillReadyAt(10208, s.tick+want) || p.Player.SkillReadyAt(10208, s.tick+want-1) {
			t.Fatalf("wrong cooldown boundary want%d", want)
		}
	}
}

func TestLegendaryAssassinSynergyImmuneStunSingleNativeCoefficient(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A23")
	m.Status.Apply(domain.StatusDef{ID: domain.StatusSpiritLock, DurationSec: 10, SuppressHarmful: true}, s.tick, m.ID)
	d := domain.SkillDef{ID: 10219, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	before := m.HP
	assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
	if before-m.HP != 295 {
		t.Fatalf("single native compensated damage=%d", before-m.HP)
	}
}
func TestLegendaryAssassinSynergyNativePullBoundAndSharedTrapRoot(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A28")
	m.Pos.X = 300
	s.aoi.Move(m, m.Pos)
	d := domain.SkillDef{ID: 10225, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	before := m.HP
	assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
	if before-m.HP != 145 || m.Pos.X != 180 {
		t.Fatal("native145damage and120pull", before-m.HP, m.Pos.X)
	}
	s, p, m = legendaryAssassinFixture(t, "CK-A13")
	finish := s.beginEquipmentRoot(p.ID, m.ID, s.equipmentOffense(p))
	root := s.equipment.root.id
	d = domain.SkillDef{ID: 10213, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100}}}
	s.placeTrap(p, d, m.Pos, 100, 100)
	children := 0
	for _, tr := range s.traps {
		if tr.Trap.LegendaryParentRootID != root {
			t.Fatal("lost accepted parent root")
		}
		if tr.Trap.LegendaryChildPlacement {
			children++
		}
	}
	finish()
	if children != 2 || len(s.traps) != 3 {
		t.Fatal("native placement lineage", children, len(s.traps))
	}
}

func TestLegendaryAssassinSynergyPersistentTrapKeepsAcceptedModifiers(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A12")
	base := domain.SkillDef{ID: 10213, Level: 1, TargetEnemy: true, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100}}}
	s.skills = domain.SkillTable{{ID: 10213, Level: 1}: base}
	d := base
	d.FormulaVersion = domain.SkillFormulaVersion
	d.SnapshotRawRanks = p.Player.Char.Skills.Clone()
	d.SnapshotHardRanks = p.Player.Char.Skills.Clone()
	s.legendaryPrepareSkill(p, &d)
	before := m.HP
	s.placeTrap(p, d, m.Pos, 130, 100)
	s.stepTraps()
	if before-m.HP != 190 {
		t.Fatalf("persistent native trap lost accepted modifier: %d", before-m.HP)
	}
}

func TestLegendaryAssassinSynergyNativeStealthRefreshKeepsConsumedBudget(t *testing.T) {
	for _, id := range []string{"CK-U08", "CK-U10"} {
		t.Run(id, func(t *testing.T) {
			s, p, _ := legendaryAssassinFixture(t, id)
			app := domain.StatusApplication{ID: domain.StatusStealth, Level: 1}
			base := domain.SkillDef{ID: 10208, Level: 1, Statuses: []domain.StatusApplication{app}}
			s.skills = domain.SkillTable{{ID: 10208, Level: 1}: base}
			s.statuses[domain.StatusKey{ID: domain.StatusStealth, Level: 1}] = domain.StatusDef{ID: domain.StatusStealth, Level: 1, DurationSec: 10, Invisible: true, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: 20, Mode: domain.ModePercent}}}
			p.Player.Char.Skills[10208] = 2
			d := base
			d.Level = 2
			d.FormulaVersion = domain.SkillFormulaVersion
			d.SnapshotRawRanks = domain.Learned{10208: 2}
			d.SnapshotHardRanks = domain.Learned{10208: 2}
			s.legendaryPrepareSkill(p, &d)
			old, ok := s.resolveSkillStatusDef(d, app, 0)
			if !ok {
				t.Fatal("native carrier resolve")
			}
			s.applyStatusDef(p, old, p.ID)
			p.Status.ConsumeLegendaryStealthTrap()
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.Invisible {
					old = a.Def
				}
			})
			p.Player.Char.Skills[10208] = 1
			next, keep, changed := s.resolvePersistentStatus(p, p, old)
			if !keep || !changed {
				t.Fatal("native lower-rank rebuild not reached")
			}
			wantCharges, wantDuration, wantSpeed := int32(0), int32(15), int32(20)
			if id == "CK-U10" {
				wantCharges, wantDuration, wantSpeed = 0, 10, 60
			}
			if next.LegendaryStealthTrapCharges != wantCharges || next.DurationSec != wantDuration {
				t.Fatalf("rebuild refilled budget/lost duration: %d/%d", next.LegendaryStealthTrapCharges, next.DurationSec)
			}
			speed := int32(0)
			for _, a := range next.Affixes {
				if a.Attr == domain.AttrMoveSpeed {
					speed += a.Value
				}
			}
			if speed != wantSpeed {
				t.Fatal("rebuild lost same-native stealth speed", speed)
			}
		})
	}
}

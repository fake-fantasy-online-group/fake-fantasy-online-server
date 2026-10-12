package scene

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// These are explicit legacy-regression fixtures. Removed V5 generation routes
// must not make previously owned traits or extended equipment slots disappear.
func legacyUniqueCatalogFixture(t *testing.T) []domain.UniqueDefinition {
	t.Helper()
	var out []domain.UniqueDefinition
	for _, id := range domain.UniqueAppearanceBaseIDs() {
		d, ok := domain.UniqueOf(domain.Stack{UID: int64(id), Item: id, Count: 1, Endgame: domain.EndgameItemState{UniqueID: int32(id), UniqueRollBP: 10000}})
		if !ok {
			t.Fatal("legacy catalog missing", id)
		}
		out = append(out, d)
	}
	return out
}
func uniqueTestGear(t *testing.T, kind string) (*Scene, *entity.Entity, domain.UniqueTrait) {
	t.Helper()
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	for _, d := range legacyUniqueCatalogFixture(t) {
		for _, tr := range d.Traits {
			if tr.Kind != kind {
				continue
			}
			s.items = map[domain.ItemID]domain.ItemDef{d.BaseID: {ID: d.BaseID, Name: d.BaseName, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment,
				Equip: &domain.EquipDef{Slot: d.Slot, Durable: 100}}}
			st := domain.Stack{UID: 987, Item: d.BaseID, Count: 1, MaxDurability: 100, Durability: 100, InstanceKind: domain.ItemInstanceEquipment}
			st.Endgame = domain.EndgameItemState{UniqueID: d.ID, UniqueRollBP: 10000}
			p.Player.Worn = domain.NewEquipSet()
			p.Player.Worn.Set(domain.EquipSlot(d.Slot), st)
			p.Player.Char.Level = 120
			p.Player.Char.Base = domain.Base{STR: 100, VIT: 100, INT: 100, AGI: 100, DEX: 100, SPI: 100}
			p.HP, p.MaxHP, p.MP, p.MaxMP = 1000, 1000, 1000, 1000
			return s, p, tr
		}
	}
	t.Fatal("missing trait", kind)
	return nil, nil, domain.UniqueTrait{}
}

func TestEveryUniquePowerAtExecutedBoundary(t *testing.T) {
	damage := []string{"healthy_assault", "desperate_assault", "execution", "ambush", "elite_hunter", "critical_focus", "fire_mastery", "ice_mastery", "holy_mastery", "single_focus", "area_focus", "invisible_focus", "basic_focus", "skill_focus"}
	for _, kind := range damage {
		t.Run(kind, func(t *testing.T) {
			s, p, tr := uniqueTestGear(t, kind)
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Hit: 100000}
			m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			blow := combat.Blow{GuaranteedHit: true}
			switch kind {
			case "desperate_assault":
				p.HP = p.MaxHP * tr.ThresholdPct / 100
			case "execution":
				m.HP = m.MaxHP * tr.ThresholdPct / 100
			case "elite_hunter":
				m.Monster.Kind = domain.MonsterElite
			case "critical_focus":
				p.Stats.CritRate = 10000
			case "fire_mastery":
				blow.Magic = true
				blow.HurtType = domain.HurtFire
				blow.SkillID = 10401
			case "ice_mastery":
				blow.Magic = true
				blow.HurtType = domain.HurtIce
				blow.SkillID = 10403
			case "holy_mastery":
				blow.Magic = true
				blow.HurtType = domain.HurtHoly
				blow.SkillID = 10302
			case "single_focus", "skill_focus":
				blow.SkillID = 10002
			case "area_focus":
				blow.SkillID = 10402
				blow.Area = true
			case "invisible_focus":
				p.Status = domain.NewStatusSet()
				p.Status.Apply(domain.StatusDef{ID: 99999, Level: 1, DurationSec: 100, Invisible: true, BreakOnAttack: true}, s.tick, p.ID)
				s.prepareBasicAttackStatuses(p, &blow)
				if entityInvisible(p) || !blow.StartedInvisible {
					t.Fatal("lost stealth start context")
				}
			}
			if tr.SkillID != 0 {
				blow.SkillID = int32(tr.SkillID)
			}
			p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.MAtk = 100, 100, 100
			beforeHP := m.HP
			plainSrc, plainDst := *p, *m
			// Removing stealth rebuilds geared Stats, including a nonzero crit rate.
			// Compare the same combat roll, not two independent random crits.
			base := combat.Strike(&plainSrc, &plainDst, &combat.HitTracker{}, blow, rand.New(rand.NewSource(1)))
			s.rng = rand.New(rand.NewSource(1))
			ctx := uniqueStrikeContext(p, m, blow, base, beforeHP)
			if domain.EvaluateUniqueTraits([]domain.UniqueTrait{tr}, ctx).DamageBP <= 0 {
				t.Fatal("test did not activate target power")
			}
			x := domain.EvaluateUniqueTraits(s.uniqueTraits(p), ctx)
			got, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, blow)
			want := domain.ApplyUniqueBP(base.Amount, x.DamageBP)
			if got.Amount != want || got.Amount <= base.Amount || m.HP != max(int32(0), beforeHP-want) {
				t.Fatalf("actual damage=%d base=%d want=%d", got.Amount, base.Amount, want)
			}
		})
	}
	for _, kind := range []string{"physical_guard", "magic_guard", "healthy_guard", "desperate_guard"} {
		t.Run(kind, func(t *testing.T) {
			s, p, tr := uniqueTestGear(t, kind)
			p.Stats = domain.Stats{}
			if kind == "desperate_guard" {
				p.HP = p.MaxHP * tr.ThresholdPct / 100
			}
			m := putMonster(s, 900, 10000, domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Hit: 100000}, 110, 100)
			blow := combat.Blow{GuaranteedHit: true, Magic: kind == "magic_guard"}
			beforeHP := p.HP
			a, b := *m, *p
			base := combat.Strike(&a, &b, &combat.HitTracker{}, blow, s.rng)
			x := domain.EvaluateUniqueTraits(s.uniqueTraits(p), domain.UniqueContext{HPPercent: uniquePercent(beforeHP, p.MaxHP), Magic: blow.Magic})
			got, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, blow)
			if got.Amount != domain.ApplyUniqueBP(base.Amount, -x.MitigationBP) || got.Amount >= base.Amount || p.HP != beforeHP-got.Amount {
				t.Fatal("incoming power did not affect actual pre-hit HP", got)
			}
		})
	}
	for _, kind := range []string{"healing_grace", "rescue_heal", "selfless_heal"} {
		t.Run(kind, func(t *testing.T) {
			s, p, _ := uniqueTestGear(t, kind)
			m := putMonster(s, 900, 1000, domain.Stats{}, 110, 100)
			m.HP = 1
			def := domain.SkillDef{ID: 10301, Effect: domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 100}}
			want := s.uniqueHealing(p, m, 100, def)
			s.applySkill(p, m, def, 0, false)
			if want <= 100 || m.HP != 1+want {
				t.Fatalf("actual healing=%d want=%d", m.HP-1, want)
			}
		})
	}
	for _, kind := range []string{"healing_cost", "spell_economy", "swift_cast"} {
		t.Run(kind, func(t *testing.T) {
			s, p, _ := uniqueTestGear(t, kind)
			def := domain.SkillDef{ID: 10301, Level: 1, Kind: domain.SkillSingle, HurtType: domain.HurtHoly, MPCost: 100, PrepareMS: 1000, TargetSelf: true, Effect: domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 100}}
			s.skills = skillTable(def)
			p.Player.Char.Skills = map[domain.SkillID]int32{def.ID: 1}
			want := def
			s.applyUniqueSkillModifiers(p, &want)
			s.onUseSkill(UseSkill{ID: p.ID, Skill: def.ID, Target: p.ID})
			if len(s.casts) != 1 || p.MP != 1000-want.MPCost || s.casts[0].def.PrepareMS != want.PrepareMS {
				t.Fatal("cast modifier missed actual admission", p.MP, len(s.casts))
			}
			if kind == "swift_cast" && want.PrepareMS >= 1000 || kind != "swift_cast" && want.MPCost >= 100 {
				t.Fatal("power not activated")
			}
			original, _ := s.skills.Get(def.ID, 1)
			if original.MPCost != 100 || original.PrepareMS != 1000 {
				t.Fatal("mutated global skill table")
			}
		})
	}
	t.Run("area_expansion", func(t *testing.T) {
		s, p, _ := uniqueTestGear(t, "area_expansion")
		def := domain.SkillDef{ID: 10402, Level: 1, Kind: domain.SkillArea, TargetEnemy: true, Dist: 500, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		modified := def
		s.applyUniqueSkillModifiers(p, &modified)
		if modified.Area.Radius <= 100 {
			t.Fatal("radius unchanged")
		}
		m := putMonster(s, 900, 1000, domain.Stats{}, p.Pos.X+float64(100+modified.Area.Radius)/2, p.Pos.Y)
		s.skills = skillTable(def)
		p.Player.Char.Skills = map[domain.SkillID]int32{def.ID: 1}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: def.ID, Target: m.ID})
		if len(s.casts) != 1 || len(s.casts[0].targets) != 1 || s.casts[0].targets[0] != m.ID {
			t.Fatal("expanded area did not select actual target")
		}
	})
	t.Run("trap_expansion", func(t *testing.T) {
		s, p, _ := uniqueTestGear(t, "trap_expansion")
		s.alloc = domain.NewEntityAlloc()
		def := domain.SkillDef{ID: 10210, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}}}
		cast := def
		s.applyUniqueSkillModifiers(p, &cast)
		s.placeTrap(p, cast, p.Pos, cast.Effects[0].TrapRadius, 0)
		if len(s.traps) != 1 || def.Effects[0].TrapRadius != 100 {
			t.Fatal("trap missing or global effect mutated")
		}
		for _, trap := range s.traps {
			if trap.Trap.Radius <= 100 {
				t.Fatal("spawned trap radius unchanged")
			}
		}
	})
	for _, kind := range []string{"kill_mend", "kill_mana"} {
		t.Run(kind, func(t *testing.T) {
			s, p, _ := uniqueTestGear(t, kind)
			p.HP, p.MP = 500, 500
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
			m := putMonster(s, 900, 1, domain.Stats{}, 110, 100)
			ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
			if !ev.Flag.Has(event.DamageFatal) || kind == "kill_mend" && p.HP <= 500 || kind == "kill_mana" && p.MP <= 500 {
				t.Fatal("fatal-hit recovery absent")
			}
			hp, mp := p.HP, p.MP
			s.onDeath(m, p.ID)
			if p.HP != hp || p.MP != mp {
				t.Fatal("kill recovery executed again during death lifecycle")
			}
		})
	}
}

func TestUniqueExtendedSlotsActualEquipAndUnequip(t *testing.T) {
	for slot := domain.EquipSlot(14); slot <= 25; slot++ {
		t.Run(string(rune('A'+slot)), func(t *testing.T) {
			var d domain.UniqueDefinition
			for _, candidate := range legacyUniqueCatalogFixture(t) {
				if candidate.Slot == int32(slot) {
					d = candidate
					break
				}
			}
			if d.BaseID == 0 {
				t.Fatal("no authoritative base for slot", slot)
			}
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "甲", 100, 100)
			p := s.players[1]
			def := domain.ItemDef{ID: d.BaseID, Name: d.BaseName, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment,
				Equip: &domain.EquipDef{Slot: d.Slot, Durable: 100, LevelReq: d.LevelRequirement, Need: domain.Requirement{Level: d.LevelRequirement, Sex: d.Sex, Professions: d.Professions, ProfessionRestricted: !d.Unrestricted, BeginnerAllowed: d.BeginnerAllowed}}}
			s.items = map[domain.ItemID]domain.ItemDef{d.BaseID: def}
			p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
			p.Player.Char.Level = 120
			p.Player.Char.EmploymentKnown = true
			p.Player.Char.Employed = true
			if d.Sex == 1 {
				p.Player.Char.Appear.Gender = 1
			}
			for race := domain.Race(0); race <= 4; race++ {
				if def.Equip.Need.AllowsProfession(race) {
					p.Player.Char.Race = race
					break
				}
			}
			st := domain.NewStack(def, 1)
			st.Endgame = domain.EndgameItemState{UniqueID: d.ID, UniqueRollBP: 10000}
			p.Player.Bag.Set(200, st)
			eligible := p.Player.Char.Race
			for wrong := domain.Race(0); wrong <= 4; wrong++ {
				if !def.Equip.Need.AllowsProfession(wrong) {
					p.Player.Char.Race = wrong
					s.onEquip(Equip{ID: p.ID, BagSlot: 200})
					if !p.Player.Worn.At(slot).Empty() || p.Player.Bag.At(200).UID != st.UID {
						t.Fatal("wrong-class high slot equipped")
					}
					break
				}
			}
			p.Player.Char.Race = eligible
			s.onEquip(Equip{ID: p.ID, BagSlot: 200})
			if p.Player.Worn.At(slot).UID != st.UID || !p.Player.Bag.At(200).Empty() {
				t.Fatal("high slot equip failed", slot)
			}
			s.onUnequip(Unequip{ID: p.ID, Slot: slot})
			returned := p.Player.Bag.At(200)
			if !p.Player.Worn.At(slot).Empty() || returned.UID != st.UID || returned.Endgame != st.Endgame {
				t.Fatal("unequip lost instance", slot)
			}
		})
	}
}

func TestUniqueGoldTooltipAndExtendedSlotSnapshot(t *testing.T) {
	s, p, _ := uniqueTestGear(t, "basic_focus")
	p.Player.Worn.Each(func(_ domain.EquipSlot, st domain.Stack) {
		def := s.items[st.Item]
		if equipmentQuality(def, st) != 4 {
			t.Fatal("not gold")
		}
		name, _ := domain.UniqueName(st)
		if itemDisplayName(def, st) != name || !strings.Contains(s.itemTooltip(def, st), "仙遗") {
			t.Fatal("name or tooltip absent")
		}
	})
	for slot := domain.EquipSlot(14); slot <= 25; slot++ {
		st := domain.Stack{UID: int64(slot) + 100, Item: 4002, Count: 1, InstanceKind: domain.ItemInstanceEquipment}
		s.items[4002] = domain.ItemDef{ID: 4002, Name: "高阶装备", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(slot)}}
		p.Player.Worn = domain.NewEquipSet()
		p.Player.Worn.Set(slot, st)
		snap, err := s.inventorySnapshot(p)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range snap.Equipped {
			if int32(e.Slot) == int32(slot) {
				found = true
			}
		}
		if !found {
			t.Fatal("higher slot missing", slot)
		}
	}
}

// Legacy stealth bonuses are captured before invisibility is consumed and apply
// to exactly that strike, equally for a normal hit and a critical hit.
func TestLegacyInvisibleFocusCapturedFirstHitNormalAndCritical(t *testing.T) {
	for _, crit := range []int32{0, 10000} {
		name := "normal"
		if crit != 0 {
			name = "critical"
		}
		t.Run(name, func(t *testing.T) {
			s, p, tr := uniqueTestGear(t, "invisible_focus")
			p.Status = domain.NewStatusSet()
			p.Status.Apply(domain.StatusDef{ID: 99999, Level: 1, DurationSec: 100, Invisible: true, BreakOnAttack: true}, s.tick, p.ID)
			blow := combat.Blow{GuaranteedHit: true}
			s.prepareBasicAttackStatuses(p, &blow)
			if entityInvisible(p) || !blow.StartedInvisible {
				t.Fatal("first strike lost captured invisibility")
			}
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000, CritRate: crit}
			m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			base := int32(100)
			if crit != 0 {
				base *= combat.CritMultiplier
			}
			ctx := uniqueStrikeContext(p, m, blow, event.DamageDealt{Amount: base}, m.HP)
			if domain.EvaluateUniqueTraits([]domain.UniqueTrait{tr}, ctx).DamageBP <= 0 {
				t.Fatal("stealth power not active")
			}
			want := domain.ApplyUniqueBP(base, domain.EvaluateUniqueTraits(s.uniqueTraits(p), ctx).DamageBP)
			got, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, blow)
			if got.Amount != want {
				t.Fatalf("first strike=%d want=%d", got.Amount, want)
			}
			next := combat.Blow{GuaranteedHit: true}
			s.prepareBasicAttackStatuses(p, &next)
			if next.StartedInvisible {
				t.Fatal("stealth context leaked to second strike")
			}
			ctx = uniqueStrikeContext(p, m, next, event.DamageDealt{Amount: base}, m.HP)
			if domain.EvaluateUniqueTraits([]domain.UniqueTrait{tr}, ctx).DamageBP != 0 {
				t.Fatal("consumed stealth power still active")
			}
			want = domain.ApplyUniqueBP(base, domain.EvaluateUniqueTraits(s.uniqueTraits(p), ctx).DamageBP)
			got, _, _ = s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, next)
			if got.Amount != want {
				t.Fatalf("second strike=%d want=%d", got.Amount, want)
			}
		})
	}
}

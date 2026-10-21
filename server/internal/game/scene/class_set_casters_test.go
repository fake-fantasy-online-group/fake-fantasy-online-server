package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"os"
	"testing"
	"time"
)

func casterSetEquip(t *testing.T, s *Scene, p *entity.Entity, suitID int32, pieces int) {
	t.Helper()
	if suitID == 3 {
		p.Player.Char.Race = domain.Warlock
	} else {
		p.Player.Char.Race = domain.Healer
	}
	p.Player.Char.Level = 60
	p.Player.Char.EmploymentKnown = false
	if s.items == nil {
		s.items = map[domain.ItemID]domain.ItemDef{}
	}
	suit := &domain.EquipmentSuit{ID: suitID}
	slots := []domain.EquipSlot{domain.SlotHead, domain.SlotNeck, domain.SlotWeapon, domain.SlotGlove, domain.SlotRing, domain.SlotBody, domain.SlotShoe, domain.SlotBag}
	for i, slot := range slots {
		id := domain.ItemID(980000 + int(suitID)*100 + i)
		suit.Groups = append(suit.Groups, []domain.ItemID{id})
		d := domain.ItemDef{ID: id, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100, Suit: suit}}
		s.items[id] = d
		if i < pieces {
			st := domain.NewStack(d, 1)
			st.UID = int64(id)
			p.Player.Worn.Set(slot, st)
		}
	}
	s.invalidateCombatAttributes(p)
	if got := s.classSetPieces(p, suitID); got != int32(pieces) {
		t.Fatalf("set pieces=%d want %d", got, pieces)
	}
}

func casterSetFixture(t *testing.T, suitID int32, pieces int) (*Scene, *entity.Entity, domain.EntityID) {
	s, mons := skillScene(t)
	join(t, s, 1, 100, "caster", 100, 100)
	caster(s, domain.Learned{10401: 1, 10409: 1, 10301: 1, 10319: 1, 10402: 1})
	p := s.entities[1]
	casterSetEquip(t, s, p, suitID, pieces)
	ice := 火弹术
	ice.ID = 10409
	ice.Name = "冰箭术"
	ice.HurtType = domain.HurtIce
	ice.Effect.DamagePct = 100
	light := 火弹术
	light.ID = 10319
	light.Name = "光弹术"
	light.Prof = 4
	light.HurtType = domain.HurtHoly
	light.Effect.DamagePct = 100
	s.skills[domain.SkillKey{ID: 10409, Level: 1}] = ice
	s.skills[domain.SkillKey{ID: 10319, Level: 1}] = light
	p.Stats.MAtk = 100
	p.MaxHP = 2000
	p.HP = 1000
	p.MaxMP = 1000
	p.MP = 1000
	return s, p, mons[0]
}

func TestCasterSetsMageAlternationExpiresAndDoesNotStack(t *testing.T) {
	s, p, tid := casterSetFixture(t, 3, 2)
	target := s.entities[tid]
	target.HP = 100000
	target.MaxHP = 100000
	target.Stats.MDef = 0
	ice, _ := s.skills.Get(10409, 1)
	s.startCasterSetCast(p, ice)
	before := target.HP
	s.applySkill(p, target, 火弹术, 0, false)
	if got := before - target.HP; got != 312 {
		t.Fatalf("fire +50%% got %d", got)
	}
	s.startCasterSetCast(p, ice)
	s.startCasterSetCast(p, ice)
	before = target.HP
	s.applySkill(p, target, 火弹术, 0, false)
	if got := before - target.HP; got != 312 {
		t.Fatalf("refresh stacked: %d", got)
	}
	s.tick = domain.Ticks(3000)
	before = target.HP
	s.applySkill(p, target, 火弹术, 0, false)
	if got := before - target.HP; got != 208 {
		t.Fatalf("expired damage %d", got)
	}
	s.startCasterSetCast(p, ice)
	p.Player.Worn = domain.NewEquipSet()
	before = target.HP
	s.applySkill(p, target, 火弹术, 0, false)
	if got := before - target.HP; got != 208 {
		t.Fatalf("unequipped damage %d", got)
	}
}

func TestCasterSetsMageFourEachHitOnceAndNoRecursiveCastOrCost(t *testing.T) {
	s, p, tid := casterSetFixture(t, 3, 4)
	target := s.entities[tid]
	target.HP = 100000
	target.MaxHP = 100000
	target.Stats.MDef = 0
	before, mp := target.HP, p.MP
	s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid, tid})
	if got := before - target.HP; got != 208 {
		t.Fatalf("expected one firebolt, damage=%d", got)
	}
	if p.MP != mp || len(s.casts) != 0 || s.equipmentState().classSetTriggered != 0 {
		t.Fatal("generated spell charged/queued/recurred")
	}
	if !p.Player.SkillReadyAt(10401, s.tick) {
		t.Fatal("generated spell started cooldown")
	}
	target.HP = 0
	s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid})
	if target.HP != 0 {
		t.Fatal("dead target changed")
	}
}

func TestCasterSetsMageActualCastHitsAllTargets(t *testing.T) {
	s, mons := skillScene(t, domain.Pos{MapID: 14, X: 140, Y: 100}, domain.Pos{MapID: 14, X: 160, Y: 100})
	sink := join(t, s, 1, 100, "mage", 100, 100)
	caster(s, domain.Learned{10401: 1, 10402: 1})
	p := s.entities[1]
	casterSetEquip(t, s, p, 3, 4)
	p.Stats.MAtk = 100
	for _, id := range mons {
		s.entities[id].HP = 10000
		s.entities[id].MaxHP = 10000
		s.entities[id].Stats.MDef = 0
	}
	sink.take()
	s.exec(UseSkill{ID: 1, Skill: 10402, Target: mons[0]})
	s.step()
	hits := map[domain.EntityID]int{}
	for _, e := range sink.take() {
		if d, ok := e.(event.DamageDealt); ok && d.SkillID == 10401 {
			hits[d.Dst]++
		}
	}
	if len(hits) != 2 {
		t.Fatalf("extra spell target counts %+v", hits)
	}
	for _, n := range hits {
		if n != 1 {
			t.Fatalf("repeat count %d", n)
		}
	}
	if p.MP != 936 {
		t.Fatalf("only original 64 MP should be spent: %d", p.MP)
	}
}

func TestCasterSetsHealerCopiesNominalHealEvenFullAlly(t *testing.T) {
	s, p, _ := casterSetFixture(t, 2, 2)
	join(t, s, 2, 101, "ally", 120, 100)
	ally := s.entities[2]
	p.Player.Party = 7
	ally.Player.Party = 7
	p.HP = 100
	p.MaxHP = 2000
	ally.MaxHP = 1000
	ally.HP = 1000
	heal := 治愈术
	heal.Effect = domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 1000}
	s.applySkill(p, ally, heal, 0, false)
	if p.HP != 1100 || ally.HP != 1000 {
		t.Fatalf("nominal copy self=%d ally=%d", p.HP, ally.HP)
	}
	p.HP = 100
	s.applySkill(p, p, heal, 0, false)
	if p.HP != 1100 {
		t.Fatalf("self heal duplicated %d", p.HP)
	}
	p.HP = 100
	ally.HP = 0
	s.applySkill(p, ally, heal, 0, false)
	if p.HP != 100 || ally.HP != 0 {
		t.Fatal("dead ally caused copy or resurrection")
	}
}

func TestCasterSetsManaEveryThreeSecondsNoOfflineOrRemovalGrant(t *testing.T) {
	s, p, _ := casterSetFixture(t, 2, 2)
	p.MP = 0
	s.stepCasterSets()
	s.tick = 29
	s.stepCasterSets()
	if p.MP != 0 {
		t.Fatal("early mana")
	}
	s.tick = 30
	s.stepCasterSets()
	if p.MP != 30 {
		t.Fatalf("mana=%d", p.MP)
	}
	s.stepCasterSets()
	if p.MP != 30 {
		t.Fatal("same tick double regen")
	}
	p.MP = 999
	s.tick = 60
	s.stepCasterSets()
	if p.MP != 1000 {
		t.Fatal("mana cap")
	}
	p.MP = 0
	p.Player.Worn = domain.NewEquipSet()
	s.tick = 90
	s.stepCasterSets()
	if p.MP != 0 {
		t.Fatal("removed set regenerated")
	}
	if s.equipmentState().classSetCasters[p.ID] != nil {
		t.Fatal("removed runtime retained")
	}
	casterSetEquip(t, s, p, 2, 2)
	p.HP = 0
	s.tick = 120
	s.stepCasterSets()
	if p.MP != 0 {
		t.Fatal("dead mana")
	}
}

func TestCasterSetsHealerDamageHealsSingleRadiusWithoutRecursiveCopy(t *testing.T) {
	s, p, tid := casterSetFixture(t, 2, 4)
	join(t, s, 2, 101, "ally", 150, 100)
	ally := s.entities[2]
	p.Player.Party = 7
	ally.Player.Party = 7
	p.HP = 100
	p.MaxHP = 1000
	ally.HP = 100
	ally.MaxHP = 1000
	light, _ := s.skills.Get(10319, 1)
	c := pendingCast{def: light, classSetArea: s.captureCasterSetArea(p, light, tid, nil)}
	s.finishCasterSetCast(p, c, []domain.EntityID{tid})
	if p.HP != 300 || ally.HP != 300 {
		t.Fatalf("one heal each, no copied generated heal self=%d ally=%d", p.HP, ally.HP)
	}
	if s.equipmentState().classSetTriggered != 0 || len(s.casts) != 0 {
		t.Fatal("recursive payload")
	}
}

func TestCasterSetsHealerHealingDamagesEnemiesNotAllies(t *testing.T) {
	s, p, tid := casterSetFixture(t, 2, 4)
	join(t, s, 2, 101, "ally", 150, 100)
	ally := s.entities[2]
	p.Player.Party = 7
	ally.Player.Party = 7
	ally.HP = 1000
	ally.MaxHP = 1000
	target := s.entities[tid]
	target.Stats.MDef = 0
	before := target.HP
	c := pendingCast{def: 治愈术, classSetArea: s.captureCasterSetArea(p, 治愈术, ally.ID, nil)}
	s.finishCasterSetCast(p, c, []domain.EntityID{ally.ID})
	if before-target.HP != 100 {
		t.Fatalf("light damage=%d", before-target.HP)
	}
	if ally.HP != 1000 {
		t.Fatal("ally damaged")
	}
}

func TestCasterSetsHealerAreaUsesOriginalShapeOnce(t *testing.T) {
	s, p, _ := casterSetFixture(t, 2, 4)
	join(t, s, 2, 101, "inside", 170, 100)
	join(t, s, 3, 102, "outside", 250, 100)
	for _, id := range []domain.EntityID{1, 2, 3} {
		q := s.entities[id]
		q.Player.Party = 7
		q.HP = 100
		q.MaxHP = 1000
	}
	area := 火球术
	area.Area.Center = domain.SkillAreaCenterCaster
	area.Area.Radius = 100
	c := pendingCast{def: area, classSetArea: s.captureCasterSetArea(p, area, 0, nil)}
	s.finishCasterSetCast(p, c, nil)
	if p.HP != 300 || s.entities[2].HP != 300 || s.entities[3].HP != 100 {
		t.Fatalf("original area self=%d inside=%d outside=%d", p.HP, s.entities[2].HP, s.entities[3].HP)
	}
}

func TestCasterSetsNewThresholdsAndBrokenPiece(t *testing.T) {
	for _, pieces := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		t.Run(string(rune('0'+pieces)), func(t *testing.T) {
			s, p, tid := casterSetFixture(t, 3, pieces)
			target := s.entities[tid]
			target.HP = 100000
			target.MaxHP = 100000
			target.Stats.MDef = 0
			ice, _ := s.skills.Get(10409, 1)
			s.startCasterSetCast(p, ice)
			before := target.HP
			s.applySkill(p, target, 火弹术, 0, false)
			want := int32(208)
			if pieces >= 2 {
				want = 312
			}
			if before-target.HP != want {
				t.Fatalf("%d pieces lower tier got%d want%d", pieces, before-target.HP, want)
			}
			before = target.HP
			s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid})
			extra := int32(0)
			if pieces >= 4 {
				extra = 312
			}
			if before-target.HP != extra {
				t.Fatalf("%d pieces upper tier got%d want%d", pieces, before-target.HP, extra)
			}
		})
	}
	s, p, tid := casterSetFixture(t, 3, 4)
	target := s.entities[tid]
	target.HP = 100000
	target.MaxHP = 100000
	st := p.Player.Worn.At(domain.SlotHead)
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotHead, st)
	before := target.HP
	s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid})
	if target.HP != before {
		t.Fatal("broken fourth piece still granted upper tier")
	}
}

func TestCasterSetsHealerActualDamageAndHealCast(t *testing.T) {
	s, p, tid := casterSetFixture(t, 2, 4)
	join(t, s, 2, 101, "ally", 150, 100)
	ally := s.entities[2]
	p.Player.Party = 7
	ally.Player.Party = 7
	p.HP = 100
	p.MaxHP = 1000
	ally.HP = 100
	ally.MaxHP = 1000
	target := s.entities[tid]
	target.Stats.MDef = 0
	target.HP = 10000
	target.MaxHP = 10000
	s.exec(UseSkill{ID: 1, Skill: 10319, Target: tid})
	s.stepCasts()
	if p.HP != 300 || ally.HP != 300 {
		t.Fatalf("actual damage cast did not heal once each: %d %d", p.HP, ally.HP)
	}
	before := target.HP
	s.exec(UseSkill{ID: 1, Skill: 10301, Target: 2})
	s.stepCasts()
	if before-target.HP != 100 {
		t.Fatalf("actual heal cast missing light bolt: %d", before-target.HP)
	}
	if p.HP != 500 || ally.HP != 500 {
		t.Fatalf("actual healing copy wrong: %d %d", p.HP, ally.HP)
	}
}

func TestCasterSetsAllOriginalMageElementDamage(t *testing.T) {
	b, err := os.ReadFile("../../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []domain.SkillDef
	if err = json.Unmarshal(b, &defs); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, def := range defs {
		if def.Prof != 5 || def.Level != 1 || !def.HasDamageEffect() || (def.HurtType != domain.HurtFire && def.HurtType != domain.HurtIce) {
			continue
		}
		count++
		t.Run(def.Name, func(t *testing.T) {
			s, p, tid := casterSetFixture(t, 3, 2)
			target := s.entities[tid]
			target.HP = 100000
			target.MaxHP = 100000
			target.Stats.MDef = 0
			before := target.HP
			s.applySkill(p, target, def, 0, false)
			base := before - target.HP
			opposite := domain.SkillDef{HurtType: domain.HurtFire}
			if def.HurtType == domain.HurtFire {
				opposite.HurtType = domain.HurtIce
			}
			s.startCasterSetCast(p, opposite)
			before = target.HP
			s.applySkill(p, target, def, 0, false)
			boost := before - target.HP
			if base <= 0 || boost != base*3/2 {
				t.Fatalf("base=%d buffed=%d", base, boost)
			}
		})
	}
	if count != 8 {
		t.Fatalf("expected 8 native elemental damage skills, got %d", count)
	}
}

func TestCasterSetsPeriodicAllyHealingCopiesNominal(t *testing.T) {
	s, p, _ := casterSetFixture(t, 2, 2)
	join(t, s, 2, 101, "ally", 120, 100)
	ally := s.entities[2]
	p.Player.Party = 7
	ally.Player.Party = 7
	p.HP = 100
	p.MaxHP = 1000
	ally.HP = 1000
	ally.MaxHP = 1000
	s.tickStatus(ally, domain.Active{Source: p.ID, Def: domain.StatusDef{ID: 700001, HPChanges: []domain.ResourceChange{{Value: 100, Mode: domain.ModeAbsolute}}}})
	if p.HP != 200 || ally.HP != 1000 {
		t.Fatalf("periodic nominal copy self=%d ally=%d", p.HP, ally.HP)
	}
}

func TestCasterSetsElementBonusIsDamageNotSpellCoefficient(t *testing.T) {
	s, p, tid := casterSetFixture(t, 3, 2)
	dst := s.entities[tid]
	dst.HP = 10000
	dst.MaxHP = 10000
	dst.Stats.MDef = 60
	before := dst.HP
	s.applySkill(p, dst, 火弹术, 0, false)
	base := before - dst.HP
	ice, _ := s.skills.Get(10409, 1)
	s.startCasterSetCast(p, ice)
	before = dst.HP
	s.applySkill(p, dst, 火弹术, 0, false)
	if got := before - dst.HP; got != base*3/2 {
		t.Fatalf("post-defense base=%d buffed=%d", base, got)
	}
}

func TestCasterSetsLogoutAndImmediateUnequipClearRuntime(t *testing.T) {
	s, p, _ := casterSetFixture(t, 3, 2)
	ice, _ := s.skills.Get(10409, 1)
	s.startCasterSetCast(p, ice)
	s.exec(Leave{ID: p.ID, Reason: "set test"})
	if s.equipmentState().classSetCasters[p.ID] != nil {
		t.Fatal("logout retained elemental grant")
	}
	join(t, s, 1, 100, "returning", 100, 100)
	p = s.entities[1]
	casterSetEquip(t, s, p, 3, 2)
	if s.casterSetOutgoingDamage(p, domain.HurtFire, 100) != 100 {
		t.Fatal("relogin restored old grant")
	}
	s.startCasterSetCast(p, ice)
	p.Player.Worn = domain.NewEquipSet()
	s.refreshStats(p)
	if s.equipmentState().classSetCasters[p.ID] != nil {
		t.Fatal("unequip did not clear grant immediately")
	}
}

func TestCasterSetsGroupNominalHealingCopiesEachAllyOnce(t *testing.T) {
	s, p, _ := casterSetFixture(t, 2, 2)
	join(t, s, 2, 101, "ally1", 120, 100)
	join(t, s, 3, 102, "ally2", 130, 100)
	p.Player.Party = 7
	p.HP = 100
	p.MaxHP = 2000
	heal := 治愈术
	heal.Effect = domain.SkillEffect{Kind: domain.EffectHeal, HealPctOfMax: 10}
	for _, id := range []domain.EntityID{2, 3} {
		q := s.entities[id]
		q.Player.Party = 7
		q.MaxHP = 1000
		q.HP = 1000
		s.applySkill(p, q, heal, 0, false)
	}
	if p.HP != 300 {
		t.Fatalf("each full ally should grant nominal100 once: %d", p.HP)
	}
}

func TestCasterSetsGeneratedPayloadUsesCurrentLearnedRank(t *testing.T) {
	s, p, tid := casterSetFixture(t, 3, 4)
	dst := s.entities[tid]
	dst.HP = 10000
	dst.MaxHP = 10000
	dst.Stats.MDef = 0
	fire := 火弹术
	fire.Level = 2
	fire.Effect.DamagePct = 300
	s.skills[domain.SkillKey{ID: 10401, Level: 2}] = fire
	p.Player.Char.Skills[10401] = 2
	s.invalidateCombatAttributes(p)
	before := dst.HP
	s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid})
	if before-dst.HP != 300 {
		t.Fatalf("did not use learned rank2: %d", before-dst.HP)
	}
	delete(p.Player.Char.Skills, 10401)
	s.invalidateCombatAttributes(p)
	before = dst.HP
	s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid})
	if dst.HP != before {
		t.Fatal("unlearned firebolt silently invented")
	}
}

func TestCasterSetsNativeRejuvenationCastTriggersOneReciprocalBolt(t *testing.T) {
	b, err := os.ReadFile("../../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []domain.SkillDef
	if err = json.Unmarshal(b, &defs); err != nil {
		t.Fatal(err)
	}
	var rejuvenation domain.SkillDef
	for _, d := range defs {
		if d.ID == 10303 && d.Level == 1 {
			rejuvenation = d
			break
		}
	}
	if rejuvenation.ID == 0 || len(rejuvenation.DirectEffects()) != 0 || len(rejuvenation.Statuses) != 1 {
		t.Fatalf("native rejuvenation changed: %+v", rejuvenation)
	}
	s, p, tid := casterSetFixture(t, 2, 4)
	join(t, s, 2, 101, "ally", 150, 100)
	ally := s.entities[2]
	p.Player.Party = 7
	ally.Player.Party = 7
	p.HP = 100
	p.MaxHP = 1000
	ally.HP = 100
	ally.MaxHP = 1000
	// Original 回春术 status: 120 seconds, one 14-HP restoration every second.
	hot := domain.StatusDef{ID: 10303, Level: 1, Name: "回春", TypeKnown: true, Type: domain.StatusGood, DurationSec: 120, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: 14, Mode: domain.ModeAbsolute}}}
	s.statuses[domain.StatusKey{ID: hot.ID, Level: 1}] = hot
	s.skills[domain.SkillKey{ID: 10303, Level: 1}] = rejuvenation
	p.Player.Char.Skills[10303] = 1
	s.invalidateCombatAttributes(p)
	target := s.entities[tid]
	target.HP = 10000
	target.MaxHP = 10000
	target.Stats.MDef = 0
	before := target.HP
	s.exec(UseSkill{ID: p.ID, Skill: 10303, Target: ally.ID})
	s.stepCastsAt(s.now().Add(2*time.Second), false)
	// The original 450ms impact delay starts on release, not on admission.
	s.stepCastsAt(s.now().Add(3*time.Second), false)
	if before-target.HP != 100 {
		t.Fatalf("native HoT cast missing exactly one bolt: %d", before-target.HP)
	}
	var active domain.Active
	ally.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == 10303 {
			active = a
		}
	})
	if active.Def.ID == 0 {
		t.Fatal("native HoT was not applied")
	}
	before = target.HP
	// Status installation recalculates the fixture character stats. Reset only
	// test HP headroom before checking subsequent tick payloads.
	ally.MaxHP, ally.HP = 1000, 100
	s.tickStatus(ally, active)
	s.tickStatus(ally, active)
	if target.HP != before {
		t.Fatal("HoT ticks recursively fired bolts")
	}
	if p.HP != 128 || ally.HP != 128 {
		t.Fatalf("periodic heal/copy expected28: %d/%d", p.HP, ally.HP)
	}
}

func TestCasterSetsNonHealingBuffDoesNotTriggerReciprocalBolt(t *testing.T) {
	s, p, tid := casterSetFixture(t, 2, 4)
	p.Player.Char.Skills[祝福术.ID] = 1
	s.invalidateCombatAttributes(p)
	target := s.entities[tid]
	target.HP = 10000
	target.MaxHP = 10000
	before := target.HP
	s.exec(UseSkill{ID: p.ID, Skill: 祝福术.ID, Target: p.ID})
	s.stepCasts()
	if target.HP != before {
		t.Fatal("ordinary stat buff triggered light bolt")
	}
}

func TestCasterSetsFailedHealingStatusDoesNotTriggerBolt(t *testing.T) {
	s, p, tid := casterSetFixture(t, 2, 4)
	target := s.entities[tid]
	target.HP = 10000
	before := target.HP
	def := domain.SkillDef{ID: 10303, Level: 1, Kind: domain.SkillSingle, TargetSelf: true, Statuses: []domain.StatusApplication{{ID: 10303, Level: 1, ChanceExplicit: true, ChanceBP: 0}}}
	s.statuses[domain.StatusKey{ID: 10303, Level: 1}] = domain.StatusDef{ID: 10303, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 120, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: 14, Mode: domain.ModeAbsolute}}}
	finish := s.beginEquipmentRoot(p.ID, p.ID, s.equipmentOffense(p))
	s.applySkill(p, p, def, 0, false)
	s.finishCasterSetCast(p, pendingCast{def: def, classSetArea: s.captureCasterSetArea(p, def, p.ID, nil)}, []domain.EntityID{p.ID})
	finish()
	if target.HP != before {
		t.Fatal("failed periodic heal status caused reciprocal bolt")
	}
}

func TestCasterSetsGeneratedSpellDoesNotDoubleStatusDuration(t *testing.T) {
	s, p, tid := casterSetFixture(t, 3, 4)
	target := s.entities[tid]
	target.HP = 10000
	target.MaxHP = 10000
	ice, _ := s.skills.Get(10409, 1)
	ice.Statuses = []domain.StatusApplication{{ID: 1004, Level: 1, ChanceBP: 10000, DurationSec: 10}}
	s.statuses[domain.StatusKey{ID: 1004, Level: 1}] = domain.StatusDef{ID: 1004, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10}
	s.emitCasterSetPayload(p, target, ice)
	var expiry domain.Tick
	target.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == 1004 {
			expiry = a.ExpireAt
		}
	})
	if expiry-s.tick != domain.Ticks(10000) {
		t.Fatalf("generated status duration=%d ticks", expiry-s.tick)
	}
}

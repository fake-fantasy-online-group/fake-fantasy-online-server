package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"os"
	"testing"
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
	s, p, tid := casterSetFixture(t, 3, 4)
	target := s.entities[tid]
	target.HP = 100000
	target.MaxHP = 100000
	target.Stats.MDef = 0
	ice, _ := s.skills.Get(10409, 1)
	s.startCasterSetCast(p, ice)
	before := target.HP
	s.applySkill(p, target, 火弹术, 100, false)
	if got := before - target.HP; got != 312 {
		t.Fatalf("fire +50%% got %d", got)
	}
	s.startCasterSetCast(p, ice)
	s.startCasterSetCast(p, ice)
	before = target.HP
	s.applySkill(p, target, 火弹术, 100, false)
	if got := before - target.HP; got != 312 {
		t.Fatalf("refresh stacked: %d", got)
	}
	s.tick = domain.Ticks(3000)
	before = target.HP
	s.applySkill(p, target, 火弹术, 100, false)
	if got := before - target.HP; got != 208 {
		t.Fatalf("expired damage %d", got)
	}
	s.startCasterSetCast(p, ice)
	p.Player.Worn = domain.NewEquipSet()
	before = target.HP
	s.applySkill(p, target, 火弹术, 100, false)
	if got := before - target.HP; got != 208 {
		t.Fatalf("unequipped damage %d", got)
	}
}

func TestCasterSetsMageEightEachHitOnceAndNoRecursiveCastOrCost(t *testing.T) {
	s, p, tid := casterSetFixture(t, 3, 6)
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
	casterSetEquip(t, s, p, 3, 6)
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
	s, p, _ := casterSetFixture(t, 2, 4)
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
	s.applySkill(p, ally, heal, 100, false)
	if p.HP != 1100 || ally.HP != 1000 {
		t.Fatalf("nominal copy self=%d ally=%d", p.HP, ally.HP)
	}
	p.HP = 100
	s.applySkill(p, p, heal, 100, false)
	if p.HP != 1100 {
		t.Fatalf("self heal duplicated %d", p.HP)
	}
	p.HP = 100
	ally.HP = 0
	s.applySkill(p, ally, heal, 100, false)
	if p.HP != 100 || ally.HP != 0 {
		t.Fatal("dead ally caused copy or resurrection")
	}
}

func TestCasterSetsManaEveryThreeSecondsNoOfflineOrRemovalGrant(t *testing.T) {
	s, p, _ := casterSetFixture(t, 2, 4)
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
	casterSetEquip(t, s, p, 2, 4)
	p.HP = 0
	s.tick = 120
	s.stepCasterSets()
	if p.MP != 0 {
		t.Fatal("dead mana")
	}
}

func TestCasterSetsHealerDamageHealsSingleRadiusWithoutRecursiveCopy(t *testing.T) {
	s, p, tid := casterSetFixture(t, 2, 6)
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
	s, p, tid := casterSetFixture(t, 2, 6)
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
	s, p, _ := casterSetFixture(t, 2, 6)
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
	for _, pieces := range []int{3, 4, 5, 6, 8} {
		t.Run(string(rune('0'+pieces)), func(t *testing.T) {
			s, p, tid := casterSetFixture(t, 3, pieces)
			target := s.entities[tid]
			target.HP = 100000
			target.MaxHP = 100000
			target.Stats.MDef = 0
			ice, _ := s.skills.Get(10409, 1)
			s.startCasterSetCast(p, ice)
			before := target.HP
			s.applySkill(p, target, 火弹术, 100, false)
			want := int32(208)
			if pieces >= 4 {
				want = 312
			}
			if before-target.HP != want {
				t.Fatalf("%d pieces lower tier got%d want%d", pieces, before-target.HP, want)
			}
			before = target.HP
			s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid})
			extra := int32(0)
			if pieces >= 6 {
				extra = 312
			}
			if before-target.HP != extra {
				t.Fatalf("%d pieces upper tier got%d want%d", pieces, before-target.HP, extra)
			}
		})
	}
	s, p, tid := casterSetFixture(t, 3, 6)
	target := s.entities[tid]
	target.HP = 100000
	target.MaxHP = 100000
	st := p.Player.Worn.At(domain.SlotHead)
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotHead, st)
	before := target.HP
	s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{tid})
	if target.HP != before {
		t.Fatal("broken sixth piece still granted upper tier")
	}
}

func TestCasterSetsHealerActualDamageAndHealCast(t *testing.T) {
	s, p, tid := casterSetFixture(t, 2, 6)
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
			s, p, tid := casterSetFixture(t, 3, 4)
			target := s.entities[tid]
			target.HP = 100000
			target.MaxHP = 100000
			target.Stats.MDef = 0
			before := target.HP
			s.applySkill(p, target, def, 100, false)
			base := before - target.HP
			opposite := domain.SkillDef{HurtType: domain.HurtFire}
			if def.HurtType == domain.HurtFire {
				opposite.HurtType = domain.HurtIce
			}
			s.startCasterSetCast(p, opposite)
			before = target.HP
			s.applySkill(p, target, def, 100, false)
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
	s, p, _ := casterSetFixture(t, 2, 4)
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
	s, p, tid := casterSetFixture(t, 3, 4)
	dst := s.entities[tid]
	dst.HP = 10000
	dst.MaxHP = 10000
	dst.Stats.MDef = 60
	before := dst.HP
	s.applySkill(p, dst, 火弹术, 100, false)
	base := before - dst.HP
	ice, _ := s.skills.Get(10409, 1)
	s.startCasterSetCast(p, ice)
	before = dst.HP
	s.applySkill(p, dst, 火弹术, 100, false)
	if got := before - dst.HP; got != base*3/2 {
		t.Fatalf("post-defense base=%d buffed=%d", base, got)
	}
}

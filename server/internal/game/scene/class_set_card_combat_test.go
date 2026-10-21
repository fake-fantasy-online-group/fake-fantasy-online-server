package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

func setCardFixture(t *testing.T, suitID int32, attrs ...domain.Affix) (*Scene, *entity.Entity, *entity.Entity) {
	s, p := meleeSetFixture(t, suitID, 8)
	switch suitID {
	case 1:
		p.Player.Char.Race = domain.Assassin
	case 2:
		p.Player.Char.Race = domain.Healer
	case 3:
		p.Player.Char.Race = domain.Warlock
	}
	suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
	for _, a := range attrs {
		suit.Bonuses = append(suit.Bonuses, domain.EquipmentSuitBonus{Pieces: 8, Affix: a, Probability: 100})
	}
	s.invalidateCombatAttributes(p)
	p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000, Hit: 1000000}
	p.HP = 10000
	p.MaxHP = 10000
	m := putMonster(s, 900, 1000000, domain.Stats{}, 110, 100)
	s.rng = rand.New(rand.NewSource(1))
	return s, p, m
}
func TestClassSetCardPhysicalSkillTrapAndCritical(t *testing.T) {
	s, p, m := setCardFixture(t, 1, domain.Affix{Attr: domain.AttrSuitPhysicalSkillBP, Value: 1800}, domain.Affix{Attr: domain.AttrSuitTrapDamageBP, Value: 1800}, domain.Affix{Attr: domain.AttrSuitPhysicalCritDamageBP, Value: 1500})
	for _, tc := range []struct {
		name string
		b    combat.Blow
		want int32
	}{
		{"ordinary", combat.Blow{}, 1000}, {"skill", combat.Blow{SkillID: 10201}, 1180}, {"trap only once", combat.Blow{SkillID: 10201, Trap: true}, 1180}, {"magic", combat.Blow{SkillID: 10401, Magic: true}, 1000}, {"magic trap", combat.Blow{SkillID: 10201, Trap: true, Magic: true}, 1180}, {"proc", combat.Blow{SkillID: 10201, Origin: domain.DamageProcOrigin}, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.HP = 1000000
			ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, tc.b)
			if ev.Amount != tc.want {
				t.Fatalf("got%d want%d", ev.Amount, tc.want)
			}
		})
	}
	p.Stats.CritRate = 10000
	p.Stats.MCritRate = 10000
	ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{})
	if ev.Amount != 2150 {
		t.Fatalf("physical crit=%d want2150", ev.Amount)
	}
	ev, _, _ = s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{Magic: true})
	if ev.Amount != 2000 {
		t.Fatalf("magic crit got%d", ev.Amount)
	}
}
func TestClassSetCardElementsAndMitigationCap(t *testing.T) {
	s, p, m := setCardFixture(t, 3, domain.Affix{Attr: domain.AttrSuitFireDamageBP, Value: 1800}, domain.Affix{Attr: domain.AttrSuitIceDamageBP, Value: 1800}, domain.Affix{Attr: domain.AttrSuitFireMitigationBP, Value: 800})
	for _, tc := range []struct{ hurt, want int32 }{{domain.HurtFire, 1180}, {domain.HurtIce, 1180}, {domain.HurtHoly, 1000}, {0, 1000}} {
		ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: tc.hurt})
		if ev.Amount != tc.want {
			t.Fatalf("hurt%d amount%d", tc.hurt, ev.Amount)
		}
	}
	m.Stats.MAtk = 1000
	p.Stats.MDef = 0
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 920 {
		t.Fatalf("fire defense%d", ev.Amount)
	}
	// Custom cards feed the shared mitigation bucket rather than a second multiplier.
	suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
	suit.Bonuses = append(suit.Bonuses, domain.EquipmentSuitBonus{Pieces: 8, Probability: 100, Affix: domain.Affix{Attr: domain.AttrSuitFireMitigationBP, Value: 5000}})
	ev, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 550 {
		t.Fatalf("45%% cap got%d", ev.Amount)
	}
}
func TestClassSetCardHealingSourceAndSnapshot(t *testing.T) {
	s, p, m := setCardFixture(t, 2, domain.Affix{Attr: domain.AttrSuitHealingBP, Value: 1500})
	if got := s.classSetCardHealing(p, 1000); got != 1150 {
		t.Fatalf("healing%d", got)
	}
	source := s.captureCombatSourceV5(p, false)
	p.Player.Worn.Set(domain.SlotBody, domain.Stack{})
	s.invalidateCombatAttributes(p)
	current := s.captureCombatSourceV5(p, false)
	if source.SuitBonuses.HealingBP != 1500 || sourceSuitBonuses(source, current).HealingBP != 0 {
		t.Fatal("persistent source lesser snapshot failed")
	}
	// A foreign HoT whose owner disappeared must not borrow recipient's set.
	p.Player.Worn.Set(domain.SlotBody, domain.Stack{Item: 800006, Count: 1, Durability: 100, MaxDurability: 100})
	s.invalidateCombatAttributes(p)
	p.HP = 100
	p.MaxHP = 10000
	a := domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 1200, SkillSnapshotID: 10303, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: 100}}}}
	delete(s.entities, m.ID)
	s.tickStatus(p, a)
	if p.HP != 200 {
		t.Fatalf("foreign missing source borrowed recipient bonus, HP%d", p.HP)
	}
}
func TestClassSetCardTooltipFunctionalActivation(t *testing.T) {
	s, p, _ := setCardFixture(t, 1, domain.Affix{Attr: domain.AttrSuitPhysicalSkillBP, Value: 1800})
	suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
	var lines []string
	s.appendSuitTooltip(&lines, suit, p)
	if !strings.Contains(strings.Join(lines, "\n"), "\x05物理技能伤害 +18%") {
		t.Fatal("missing active custom text")
	}
	def := s.items[p.Player.Worn.At(domain.SlotHead).Item]
	def.Equip.Need.Level = 999
	s.invalidateCombatAttributes(p)
	lines = nil
	s.appendSuitTooltip(&lines, suit, p)
	if !strings.Contains(strings.Join(lines, "\n"), "\x02物理技能伤害 +18%") || s.classSetCardCombat(p).PhysicalSkillBP != 0 {
		t.Fatal("invalid requirement remained active")
	}
}

func TestClassSetCardEveryApprovedEightPieceNumericalEffect(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		suit, attr, value, hurt int32
		magic, outgoing         bool
		want                    int32
	}{
		{"warrior physical skill", 5, domain.AttrSuitPhysicalSkillBP, 1800, 0, false, true, 1180},
		{"sword physical skill", 4, domain.AttrSuitPhysicalSkillBP, 1600, 0, false, true, 1160},
		{"warrior physical mitigation", 5, domain.AttrSuitPhysicalMitigationBP, 800, 0, false, false, 1380},
		{"sword physical mitigation", 4, domain.AttrSuitPhysicalMitigationBP, 800, 0, false, false, 920},
		{"sword fire", 4, domain.AttrSuitFireMitigationBP, 600, domain.HurtFire, true, false, 940},
		{"sword ice", 4, domain.AttrSuitIceMitigationBP, 600, domain.HurtIce, true, false, 940},
		{"sword holy", 4, domain.AttrSuitHolyMitigationBP, 600, domain.HurtHoly, true, false, 940},
		{"sword dark", 4, domain.AttrSuitDarkMitigationBP, 600, domain.HurtDark, true, false, 940},
		{"warlock ice mitigation", 3, domain.AttrSuitIceMitigationBP, 800, domain.HurtIce, true, false, 920},
		{"healer holy", 2, domain.AttrSuitHolyDamageBP, 2500, domain.HurtHoly, true, true, 1250},
		{"healer dark mitigation", 2, domain.AttrSuitDarkMitigationBP, 1000, domain.HurtDark, true, false, 900},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, m := setCardFixture(t, tc.suit, domain.Affix{Attr: tc.attr, Value: tc.value})
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000, Hit: 1000000}
			src, dst := p, m
			if !tc.outgoing {
				src, dst = m, p
			}
			ev, _, _ := s.resolveStrikeStatuses(src, dst, &combat.HitTracker{}, combat.Blow{Magic: tc.magic, HurtType: tc.hurt, SkillID: 10001})
			if ev.Amount != tc.want {
				t.Fatalf("amount%d want%d", ev.Amount, tc.want)
			}
		})
	}
	s, p, _ := setCardFixture(t, 3, domain.Affix{Attr: domain.AttrMCrit, Value: 300})
	s.refreshStats(p)
	if p.Stats.MCritRate != 300 {
		t.Fatalf("warlock native MCrit +3pp got%d", p.Stats.MCritRate)
	}
}

func TestClassSetCardActualHealCopiesEnhancedNominalOnlyOnce(t *testing.T) {
	s, p, targetID := casterSetFixture(t, 2, 8)
	suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
	suit.Bonuses = []domain.EquipmentSuitBonus{{Pieces: 8, Probability: 100, Affix: domain.Affix{Attr: domain.AttrSuitHealingBP, Value: 1500}}}
	join(t, s, 2, 200, "friend", 110, 100)
	friend := s.players[2]
	p.Player.Party = 7
	friend.Player.Party = 7
	p.HP = 100
	p.MaxHP = 10000
	friend.HP = 100
	friend.MaxHP = 10000
	def := domain.SkillDef{ID: 10301, TargetTeam: true, Effect: domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 1000}}
	if !s.applySkill(p, friend, def, 0, false) {
		t.Fatal("heal failed")
	}
	if p.HP != 1250 || friend.HP != 1250 {
		t.Fatalf("nominal once copy self%d friend%d", p.HP, friend.HP)
	}
	p.HP, friend.HP = 100, 100
	s.emitCasterSetPayload(p, friend, def)
	if p.HP != 100 || friend.HP != 1250 {
		t.Fatalf("generated heal must enhance once without self-copy: self%d friend%d", p.HP, friend.HP)
	}
	_ = targetID
}

func TestClassSetCardTrapPeriodicSnapshotAndUnrelatedPoison(t *testing.T) {
	s, p, m := setCardFixture(t, 1, domain.Affix{Attr: domain.AttrSuitTrapDamageBP, Value: 1800})
	status := domain.StatusDef{ID: 1001, Level: 1, DurationSec: 60, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -12}}, ChanceBP: 10000, ChanceExplicit: true}
	s.statuses = domain.StatusTable{{ID: 1001, Level: 1}: status}
	def := domain.SkillDef{ID: 10214, Statuses: []domain.StatusApplication{{ID: 1001, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}}
	if !s.applySkill(p, m, def, 0, true) {
		t.Fatal("trap status failed")
	}
	p.Player.Worn = domain.NewEquipSet()
	delete(s.entities, p.ID)
	before := m.HP
	for i := 1; i <= 30; i++ {
		s.tick += domain.Ticks(2000)
		for _, a := range m.Status.DueTicks(s.tick) {
			s.tickStatus(m, a)
		}
	}
	if before-m.HP != 424 {
		t.Fatalf("trap total%d want424 floor(424.8)", before-m.HP)
	}
	if status.Affixes[0].Value != -12 {
		t.Fatal("shared status mutated")
	}
	// Native non-trap periodic poison remains exactly 360.
	m.Status = domain.NewStatusSet()
	m.Status.Apply(status, s.tick, m.ID)
	before = m.HP
	for i := 1; i <= 30; i++ {
		s.tick += domain.Ticks(2000)
		for _, a := range m.Status.DueTicks(s.tick) {
			s.tickStatus(m, a)
		}
	}
	if before-m.HP != 360 {
		t.Fatalf("ordinary poison changed %d", before-m.HP)
	}
}

func TestClassSetCardNativeRejuvenationEnhancedOnce(t *testing.T) {
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
	s, p, tid := casterSetFixture(t, 2, 8)
	suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
	suit.Bonuses = []domain.EquipmentSuitBonus{{Pieces: 8, Probability: 100, Affix: domain.Affix{Attr: domain.AttrSuitHealingBP, Value: 1500}}}
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
	if active.Def.ID == 0 || !active.Def.SkillApplied {
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
	if p.HP != 132 || ally.HP != 132 {
		t.Fatalf("periodic heal/copy expected32: %d/%d", p.HP, ally.HP)
	}
}

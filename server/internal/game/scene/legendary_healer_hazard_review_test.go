package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"testing"
	"time"
)

func TestReviewYS09ActualRiftHazard(t *testing.T) {
	s, m, p, _ := riftAbilityFixture(t, riftEruption)
	p.Player.Char.Race = domain.Healer
	p.Player.Char.Level = 100
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	s.skills, s.statuses = nativeRepairTables(t)
	p.Player.Char.Skills = domain.Learned{10330: 1}
	p.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	p.MP, p.MaxMP = 10000, 10000
	nativeRepairCast(t, s, p, 10330, p.ID)
	if !legendaryCasterHasSkill(p, 10330) {
		t.Fatal("native thorns condition absent")
	}
	h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: domain.Stats{MinAtk: 100, MaxAtk: 100}, Pos: p.Pos}
	p.HP = 10000
	p.Stats = domain.Stats{}
	s.hitRiftHazard(h, p, 100)
	base := 10000 - p.HP
	incomingEquipPower(t, s, p, "YS-D09", domain.SlotBody)
	if len(domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)) != 1 {
		t.Fatal("invalid power")
	}
	p.HP = 10000
	p.Stats = domain.Stats{}
	s.hitRiftHazard(h, p, 100)
	got := 10000 - p.HP
	if got != base {
		t.Fatalf("real aura hazard is not basic: base=%d equipped=%d", base, got)
	}
}

func TestCasterReviewThornsStillMitigatesActualBasic(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10330: 1}, "YS-D09")
	nativeRepairCast(t, s, p, 10330, p.ID)
	p.Stats = domain.Stats{}
	p.HP, p.MaxHP = 10000, 10000
	m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
	s.attacks = append(s.attacks, pendingAttack{src: m.ID, dst: p.ID, blow: combat.Blow{GuaranteedHit: true}})
	s.stepCombatAt(s.now().Add(time.Second), false)
	if got := int32(10000) - p.HP; got != 65 {
		t.Fatal("native monster basic lost thorns mitigation", got)
	}
}

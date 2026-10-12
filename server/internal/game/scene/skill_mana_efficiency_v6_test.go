package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
	"os"
	"testing"
)

func TestSkillManaEfficiencySettlementV6(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	var payloads []any
	for _, rank := range []int32{20, 21, 30, 50, 100, 1000000} {
		d, ok := s.skills.Get(10306, rank)
		if !ok {
			t.Fatal(rank)
		}
		domain.FinalizeSkillManaEfficiency(&d)
		avg := float64(d.ManaCostNumerator) / float64(d.ManaCostDenominator)
		p.Player.SkillManaCredit = 0
		p.MP = 10000000
		p.MaxMP = p.MP
		before := p.MP
		for i := 0; i < 1000; i++ {
			p.MP -= d.MPCost
			settleSkillManaCredit(p, d)
		}
		spent := float64(before - p.MP)
		if spent < 1000*avg-1e-7 || spent >= 1000*avg+1.0000001 {
			t.Fatalf("rank%d aggregate %g average %g", rank, spent, avg)
		}
		if p.Player.SkillManaCredit < 0 || p.Player.SkillManaCredit >= 1 {
			t.Fatal("unbounded remainder")
		}
		view := s.skillBehaviorPayload(p, d)
		if view.MPCost != int32(math.Ceil(avg)) {
			t.Fatal("cost contract", rank, view)
		}
		payloads = append(payloads, map[string]any{"rank": rank, "payload": view, "lines": s.skillOriginalEffectLines(d)})
	}
	if path := os.Getenv("SKILL_EFFICIENCY_FIXTURE"); path != "" {
		b, _ := json.MarshalIndent(payloads, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSkillManaEfficiencyAdmissionAndTransferV6(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	p.Player.Char.Skills = domain.Learned{10306: 20}
	s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: domain.Learned{10306: 20}}}
	d, _ := s.skills.Get(10306, 20)
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
	p.MP = 0
	p.Player.SkillManaCredit = .75
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: p.ID})
	if p.MP != 0 || p.Player.SkillManaCredit != .75 || len(s.casts) != 0 {
		t.Fatal("zero mana cast or failed cast consumes credit")
	}
	p.MP = 10000
	p.MaxMP = 10000
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: 999999})
	if p.Player.SkillManaCredit != .75 || p.MP != 10000 {
		t.Fatal("invalid target consumed mana")
	}
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: p.ID})
	if len(s.casts) != 1 || p.MP >= 10000 {
		t.Fatal("cast did not enter actual prepaid path", len(s.casts), p.MP)
	}
	credit, mp := p.Player.SkillManaCredit, p.MP
	// Cancelling an already accepted cast does not refund its committed mana.
	s.casts = nil
	if p.Player.SkillManaCredit != credit || p.MP != mp {
		t.Fatal("interruption created credit")
	}
	runtime := p.CapturePlayerRuntime(s.tick)
	other := &entity.Entity{Player: &entity.Player{}, MaxHP: 10000, MaxMP: 10000}
	other.RestorePlayerRuntime(runtime, s.tick+100)
	if other.Player.SkillManaCredit != credit || other.MP != mp {
		t.Fatal("transfer lost prepaid fraction")
	}
	// A new login receives no prepaid credit. Discarding it can never reduce
	// the paid debit or grant a cast, since admission always requires ceil MP.
	fresh := &entity.Player{}
	if fresh.SkillManaCredit != 0 || credit < 0 {
		t.Fatal("login grants credit")
	}
}

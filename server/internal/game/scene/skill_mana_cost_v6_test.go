package scene

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"os"
	"strings"
	"testing"
)

func TestSkillManaCostMatchesEveryActualDebitV6(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	var payloads []any
	previous := int32(10000000)
	for _, rank := range []int32{5, 20, 21, 30, 50, 100, 1000000} {
		p.Player.Char.Skills = domain.Learned{10306: min(rank, 20)}
		s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: domain.Learned{10306: rank}}}
		d, ok := s.skills.Get(10306, rank)
		if !ok {
			t.Fatal(rank)
		}
		s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
		if d.MPCost <= 0 || d.MPCost > previous {
			t.Fatalf("rank%d cost %d after %d", rank, d.MPCost, previous)
		}
		previous = d.MPCost
		view := s.skillBehaviorPayload(p, d)
		lines := s.skillOriginalEffectLines(d)
		if view.MPCost != d.MPCost || !strings.Contains(strings.Join(lines, "\n"), fmt.Sprintf("消耗: %d点法力", d.MPCost)) || strings.Contains(strings.Join(lines, ""), "效率") {
			t.Fatal("shown/gated/paid mana mismatch", rank, view, lines)
		}
		p.MP = 10000000
		p.MaxMP = p.MP
		for i := 0; i < 100; i++ {
			s.casts = nil
			s.tick += 1000000
			before := p.MP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: p.ID})
			if len(s.casts) != 1 || before-p.MP != d.MPCost {
				t.Fatalf("rank%d cast%d debit%d expected%d queue%d", rank, i, before-p.MP, d.MPCost, len(s.casts))
			}
		}
		payloads = append(payloads, map[string]any{"rank": rank, "payload": view, "lines": lines})
	}
	if previous != 1 {
		t.Fatal("positive mana floor", previous)
	}
	if path := os.Getenv("SKILL_MANA_FIXTURE"); path != "" {
		b, _ := json.MarshalIndent(payloads, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSkillManaCostAdmissionTransferAndCancelV6(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	p.Player.Char.Skills = domain.Learned{10306: 20}
	s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: domain.Learned{10306: 20}}}
	d, _ := s.skills.Get(10306, 20)
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
	p.MP = d.MPCost - 1
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: p.ID})
	if p.MP != d.MPCost-1 || len(s.casts) != 0 {
		t.Fatal("insufficient mana accepted")
	}
	p.MP = 10000
	p.MaxMP = 10000
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: 999999})
	if p.MP != 10000 {
		t.Fatal("invalid target consumed mana")
	}
	p.MP = d.MPCost
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: p.ID})
	if len(s.casts) != 1 || p.MP != 0 {
		t.Fatal("exact displayed mana failed to pay exactly", len(s.casts), p.MP)
	}
	s.casts = nil
	if p.MP != 0 {
		t.Fatal("cancellation gave refund")
	}
	runtime := p.CapturePlayerRuntime(s.tick)
	other := &entity.Entity{Player: &entity.Player{}, MaxHP: 10000, MaxMP: 10000}
	other.RestorePlayerRuntime(runtime, s.tick+100)
	if other.MP != 0 {
		t.Fatal("transfer changed mana")
	}
	s.tick += 1000000
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10306, Target: p.ID})
	if len(s.casts) != 0 || p.MP != 0 {
		t.Fatal("zero-mana free cast")
	}
}

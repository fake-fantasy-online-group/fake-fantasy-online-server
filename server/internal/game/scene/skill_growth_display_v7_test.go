package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Every line is a separately bounded production response. This is a curve and
// presentation fixture, not a claim that all high ranks coexist in a legal build.
func TestSkillGrowthDisplayV7RuntimeFixtures(t *testing.T) {
	var packets []string
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		if id == 10212 {
			continue
		} // Explicitly unresolved theft remains untouched.
		identity, _ := domain.V5SkillIdentity(id, catalogfixture.SkillProgressionCatalog())
		s, p := behaviorAcceptancePlayer(t, identity.Profession)
		policy, _ := domain.LookupSkillGrowthPolicy(id, catalogfixture.SkillProgressionCatalog())
		rankSet := map[int32]bool{20: true, 100: true}
		for _, cap := range []int32{policy.AttributeCapRank, policy.EfficiencyCapRank, policy.RangeCapRank, policy.ProtectionCapRank, policy.AttackCapRank} {
			if cap > 0 {
				rankSet[cap], rankSet[cap+1] = true, true
			}
		}
		if id == 10210 || id == 10211 {
			rankSet[5], rankSet[6] = true, true
		}
		if id == 10016 {
			rankSet[11], rankSet[12] = true, true
		}
		var ranks []int
		for rank := range rankSet {
			ranks = append(ranks, int(rank))
		}
		sort.Ints(ranks)
		for _, rank := range ranks {
			hard := min(int32(rank), int32(19))
			p.Player.Char.Skills = domain.Learned{id: hard}
			raw := domain.Learned{id: int32(rank)}
			def, ok := s.skills.Get(id, int32(rank))
			next, nextOK := s.skills.Get(id, int32(rank+1))
			if !ok || !nextOK {
				t.Fatalf("unresolved display fixture %d/%d", id, rank)
			}
			s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, raw)
			s.applyResolvedSkillModifiers(p, &next, p.Player.Char.Skills, raw)
			contract := s.skillBehaviorIdentity(id)
			contract.Current, contract.Next = s.skillBehaviorPayload(p, def), s.skillBehaviorPayload(p, next)
			currentLines, nextLines := s.skillOriginalEffectLines(def), s.skillOriginalEffectLines(next)
			for i, lines := range [][]string{currentLines, nextLines} {
				value := []domain.SkillDef{def, next}[i]
				if value.Kind.Active() && (len(lines) < 4 || lines[1] != fmt.Sprintf("消耗: %d点法力", value.MPCost) && !(value.MPCost == 0 && lines[1] == "消耗: 无") || !strings.HasPrefix(lines[2], "技能吟唱:") || !strings.HasPrefix(lines[3], "技能间隔:")) {
					t.Fatalf("original four rows lost: %d/%d %v", id, value.Level, lines)
				}
				for _, banned := range []string{"法力效率", "抵抗概率降至", "参数", "成功抵抗", "减伤触发后"} {
					if strings.Contains(strings.Join(lines, "\n"), banned) {
						t.Fatalf("obsolete display %d/%d: %v", id, value.Level, lines)
					}
				}
			}
			training, native := s.skillSynergyViews(id, p.Player.Char.Skills, raw)
			view := domain.SkillRankView{ID: id, HardRank: hard, GrantRank: "0", SoftRank: fmt.Sprint(int32(rank) - hard), EffectiveRank: fmt.Sprint(rank), NextEffectiveRank: fmt.Sprint(rank + 1), CanCast: true, CanInvest: true,
				CurrentEffectLines: currentLines, NextEffectLines: nextLines, BlockedReasons: []string{}, TrainingSynergies: training, NativeModifierLines: native, Contract: contract}
			response := domain.EndgameResponse{Kind: "skill.ranks", OK: true, Revision: "1", SkillRanks: []domain.SkillRankView{view}, Result: map[string]string{"hardCap": "20", "playerCap": "100", "petCap": "80", "formulaVersion": "5", "unspentPoints": "1"}}
			encoded, err := json.Marshal(response)
			if err != nil || len(encoded) >= 61440 {
				t.Fatalf("unbounded fixture %d/%d: %d %v", id, rank, len(encoded), err)
			}
			packets = append(packets, string(encoded))
		}
	}
	if path := os.Getenv("SKILL_GROWTH_DISPLAY_FIXTURE"); path != "" {
		if err := os.WriteFile(path, []byte(strings.Join(packets, "\n")+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%d production response samples covering 148 resolved skills, policy boundaries and ranks20/100", len(packets))
}

func TestSkillGrowthDisplayV7FixedProcAndIndependentHit(t *testing.T) {
	s, _ := behaviorAcceptancePlayer(t, 3)
	var poisonTotals []string
	for _, rank := range []int32{5, 6, 20, 100} {
		assassin, _ := s.skills.Get(10210, rank)
		lines := s.skillOriginalEffectLines(assassin)
		if !strings.Contains(lines[0], "13%几率发生即死判定") {
			t.Fatal("base death chance was replaced by final success chance", lines)
		}
		death, ok := skillDescriptionEffect(assassin, domain.EffectInstantDeath)
		if !ok || death.ChanceBP != 1300 {
			t.Fatal("incorrect assassination fixture", death)
		}
		if rank > 5 && lines[4] != skillEffectHitLine(death.EffectHitBP) {
			t.Fatal("local hit missing after native four lines", lines)
		}
		poison, _ := s.skills.Get(10211, rank)
		state, ok := s.skillDescriptionStatus(poison.Statuses[0])
		if !ok || state.PoisonOnHitChance != 50 {
			t.Fatal("poison base chance grew", state)
		}
		values, _ := s.skillDescriptionValues(poison)
		if values[1] != "50" || values[2] != "60" {
			t.Fatal("poison chance or duration changed", values)
		}
		poisonTotals = append(poisonTotals, values[3])
	}
	if poisonTotals[0] == poisonTotals[1] || poisonTotals[1] == poisonTotals[2] || poisonTotals[2] == poisonTotals[3] {
		t.Fatal("poison damage tooltip stopped following real periodic damage", poisonTotals)
	}
}

func TestSkillGrowthDisplayV7PassiveDamageRemainsOnRecipient(t *testing.T) {
	skills, states := nativeRestoredSceneFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, states
	for _, tc := range []struct {
		source, target domain.SkillID
		rank           int32
	}{
		{10423, 10405, 6}, {10422, 10421, 16}, {10019, 10017, 7}, {10201, 10209, 6},
	} {
		raw := domain.Learned{tc.source: tc.rank, tc.target: 1}
		_, native := s.skillSynergyViews(tc.target, raw, raw)
		want := fmt.Sprintf("已学被动使本技能伤害提升%d%%", domain.SkillGrowthDamageBonusPct(tc.source, tc.rank, tc.target, catalogfixture.SkillProgressionCatalog()))
		if len(native) == 0 || native[0] != want {
			t.Fatalf("recipient misses actual source growth %d→%d: %v", tc.source, tc.target, native)
		}
		_, sourceNative := s.skillSynergyViews(tc.source, raw, raw)
		if len(sourceNative) != 0 {
			t.Fatal("source tooltip acquired outgoing target list", sourceNative)
		}
	}
}

func TestSkillGrowthDisplayV7IncludesExplicitLocalHitCarrier(t *testing.T) {
	s, _ := behaviorAcceptancePlayer(t, 1)
	def, _ := s.skills.Get(10016, 12)
	def.Statuses[0].EffectHitBP = 125
	state, ok := s.skillDescriptionStatus(def.Statuses[0])
	if !ok || state.EffectHitBP != 625 {
		t.Fatal("display lost explicit carrier accuracy", state.EffectHitBP)
	}
	lines := s.skillOriginalEffectLines(def)
	if lines[4] != "本技能效果命中 +6.25%" {
		t.Fatal("display differs from combined local accuracy", lines)
	}
	trigger := domain.SkillDef{ID: 10019, Level: 12, PassiveTriggers: []domain.PassiveTriggerStatus{{Status: def.Statuses[0].ID, StatusLevel: def.Statuses[0].Level, EffectHitBP: 125}}}
	triggerState, ok := s.skillDescriptionTrigger(trigger, 0)
	if !ok || triggerState.EffectHitBP != 125 {
		t.Fatal("passive trigger display lost explicit local carrier", triggerState.EffectHitBP)
	}
}

package scene

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Actual authored equipment, validated hard allocation and server-resolved text.
// This complements (rather than impersonates) client synthetic +7 stress cases.
func TestV5HighEquippedSkillTooltipFixture(t *testing.T) {
	skills, statuses := v5PacketActualSkills(t)
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "高等级面板", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	s.skills, s.statuses, s.items = skills, statuses, v5PacketActualEquipment(t)
	ch.Race, ch.Level, ch.SkillAllocationVersion = domain.Warlock, 100, 5
	ch.Appear.Gender = 0
	ch.EmploymentKnown, ch.Employed, ch.SkillPointsKnown = true, true, true
	ch.Base = domain.StartingBase(ch.Race)
	for level := int32(2); level <= 100; level++ {
		ch.Base = ch.Base.Add(domain.GrowthAt(ch.Race, level))
	}
	ch.Base.STR += 40
	ch.Base.INT += 40
	ch.Base.SPI += 40
	ch.Base.VIT += 40
	ch.Base.AGI += 20
	ch.Base.DEX += 18
	p.HP, p.MaxHP, p.MP, p.MaxMP = 10000, 10000, 10000, 10000
	ch.Skills = domain.Learned{}
	for _, id := range domain.V5SkillIDs() {
		identity, _ := domain.V5SkillIdentity(id)
		if identity.Profession == 5 {
			ch.Skills[id] = 1
		}
	}
	for id, rank := range (domain.Learned{10405: 19, 10410: 19, 10402: 10, 10424: 5, 10423: 5, 10409: 10, 10431: 3, 10427: 3}) {
		ch.Skills[id] = rank
	}
	ch.SkillPoints = 99
	for _, rank := range ch.Skills {
		ch.SkillPoints -= rank
	}
	if err := domain.ValidateSkillAllocation(100, 5, ch.Skills, ch.SkillPoints); err != nil {
		t.Fatal(err)
	}
	groups := v5PacketGearCandidates(t, s, p)
	selected := map[domain.SkillID]domain.SkillRankView{}
	maxSoft := map[domain.SkillID]int{}
	selectedGear := map[domain.SkillID][]string{}
	expectedCurrent := map[domain.SkillID]string{}
	expectedNext := map[domain.SkillID]string{}
	var enumerate func(int, []v5PacketGear)
	enumerate = func(index int, gear []v5PacketGear) {
		if index < len(groups) {
			for _, g := range groups[index] {
				enumerate(index+1, append(gear, g))
			}
			return
		}
		p.Player.Worn = domain.NewEquipSet()
		labels := []string{}
		for _, g := range gear {
			p.Player.Worn.Set(g.slot, g.stack)
			labels = append(labels, g.label)
		}
		if s.functionalWorn(p).Count() != len(gear) {
			t.Fatal("inactive fixture equipment", labels)
		}
		views, err := s.skillRanksForDisplay(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []domain.SkillID{10405, 10410} {
			view := views[id]
			soft, err := strconv.Atoi(view.SoftRank)
			if err != nil {
				t.Fatal(err)
			}
			if soft > maxSoft[id] {
				maxSoft[id] = soft
				selected[id] = view
				selectedGear[id] = append([]string(nil), labels...)
				raw := s.rawSkillLevels(p)
				for offset, output := range []map[domain.SkillID]string{expectedCurrent, expectedNext} {
					def, ok := skills.Get(id, 19+int32(soft)+int32(offset))
					if !ok {
						t.Fatal("effective rank unavailable", id, soft, offset)
					}
					s.applyTrainingSynergiesWithHard(p, &def, ch.Skills)
					s.applyPassiveSkillModifiersWithLevels(p, &def, raw)
					s.applyUniqueSkillModifiers(p, &def)
					if def.Effect.DamageFlat != 0 {
						t.Fatal("high rank gained invented flat damage", id, def.Effect)
					}
					output[id] = "造成" + skillFractionText(def.Effect.DamagePct, def.Effect.DamagePctNumerator, def.Effect.DamagePctDenominator) + "%"
				}
			}
		}
	}
	enumerate(0, nil)
	rows := []domain.SkillRankView{}
	for _, id := range []domain.SkillID{10405, 10410} {
		view, ok := selected[id]
		if !ok || view.HardRank != 19 || maxSoft[id] < 1 || view.EffectiveRank != strconv.Itoa(19+maxSoft[id]) || view.NextEffectiveRank != strconv.Itoa(20+maxSoft[id]) || !view.CanInvest || len(view.CurrentEffectLines) < 4 || len(view.NextEffectLines) < 4 {
			t.Fatal("invalid actual high-equipment preview", id, view)
		}

		current, next := strings.Join(view.CurrentEffectLines, "\n"), strings.Join(view.NextEffectLines, "\n")
		if current == next || !strings.Contains(current, expectedCurrent[id]) || !strings.Contains(next, expectedNext[id]) {
			t.Fatal("complete original tooltip disagrees with effective damage", id, current, next, expectedCurrent[id], expectedNext[id])
		}
		for _, text := range []string{current, next} {
			for _, label := range []string{"消耗:", "技能吟唱:", "技能间隔:"} {
				if !strings.Contains(text, label) {
					t.Fatal("complete tooltip lost original client field", id, label, text)
				}
			}
			if strings.Contains(text, "固定伤害") || strings.Contains(text, "伤害倍率") {
				t.Fatal("tooltip replaced original prose", id, text)
			}
		}
		t.Logf("skill%d hard19 soft%d effective%s next%s unspent%d gear%v; source rows%v", id, maxSoft[id], view.EffectiveRank, view.NextEffectiveRank, ch.SkillPoints, selectedGear[id], view.TrainingSynergies)
		rows = append(rows, view)
	}
	if path := os.Getenv("SKILL_HIGH_EQUIPPED_FIXTURE_PATH"); path != "" {
		data, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

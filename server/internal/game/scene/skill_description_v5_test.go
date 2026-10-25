package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"strings"
	"testing"
)

func TestOriginalSkillDescriptionAll745RanksRemainVerbatim(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	count := 0
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		for rank := int32(1); rank <= 5; rank++ {
			original, ok := skills[domain.SkillKey{ID: id, Level: rank}]
			if !ok {
				t.Fatalf("missing original %d/%d", id, rank)
			}
			def, ok := skills.Get(id, rank)
			if !ok {
				t.Fatalf("unresolved %s rank%d", original.Name, rank)
			}
			values, known := s.skillDescriptionValues(def)
			if !known || len(values) != len(skillDescriptionNumber.FindAllStringIndex(original.Description, -1)) {
				t.Fatalf("unverified template %s rank%d: %q values%v", original.Name, rank, original.Description, values)
			}
			lines := s.skillOriginalEffectLines(def)
			expectedDescription := original.Description
			if id == 10229 || id == 10230 {
				expectedDescription = strings.ReplaceAll(expectedDescription, "，此技能尚无法作用", "")
			}
			expected := []string{expectedDescription}
			if original.Kind.Active() {
				cost, cast := fmt.Sprintf("%d点法力", original.MPCost), skillMillisecondsText(original.PrepareMS)
				if original.MPCost == 0 {
					cost = "无"
				}
				if original.PrepareMS == 0 {
					cast = "瞬发"
				}
				expected = append(expected, "消耗: "+cost, "技能吟唱: "+cast, "技能间隔: "+skillMillisecondsText(original.CooldownMS))
			}
			// Only the two elemental shields now expose lifetime at every rank.
			if id == 10425 || id == 10435 {
				expected = append(expected, fmt.Sprintf("保护持续时间: %d秒", 30*rank))
			}
			// Maintenance alone reaches its original boundary before rank five.
			if id == 10024 && rank == 5 {
				expected = append(expected, "体质 +1")
			}
			if !reflect.DeepEqual(lines, expected) {
				t.Fatalf("original full block differs %s rank%d: got%v want%v", original.Name, rank, lines, expected)
			}
			count++
		}
	}
	if count != 745 {
		t.Fatal("incomplete original profession coverage", count)
	}
}

func TestOriginalSkillDescriptionHighRanksRetainCompleteProse(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		original := skills[domain.SkillKey{ID: id, Level: 5}]
		if id == 10229 || id == 10230 {
			original.Description = strings.ReplaceAll(original.Description, "，此技能尚无法作用", "")
		}
		for _, rank := range []int32{6, 20, 21, 50, 100} {
			def, ok := skills.Get(id, rank)
			if !ok {
				t.Fatalf("%s rank%d failed", original.Name, rank)
			}
			lines := s.skillOriginalEffectLines(def)
			if skillDescriptionNumber.ReplaceAllString(lines[0], "#") != skillDescriptionNumber.ReplaceAllString(original.Description, "#") {
				t.Fatalf("lost original meaning %s rank%d: %s", original.Name, rank, lines[0])
			}
			for _, bad := range []string{"[V5]", "适用技能：", "其余效果不变", "固定伤害+0", "伤害倍率"} {
				if strings.Contains(strings.Join(lines, ""), bad) {
					t.Fatalf("generated replacement leaked %s rank%d: %v", original.Name, rank, lines)
				}
			}
		}
	}
	checks := []struct {
		id         domain.SkillID
		rank       int32
		name, want string
	}{
		{10002, 20, "连环枪法", "低级枪系攻击技能，对敌人造成225%伤害。"},
		{10102, 20, "罡气剑", "造成187%伤害，震退敌人。"},
		{10205, 20, "连续投掷", "远程技能，对敌人造成245%的伤害。"},
		{10319, 20, "光弹术", "对目标造成495%的伤害。"},
		{10401, 20, "火弹术", "对敌人造成360%伤害。"},
		{10001, 20, "枪法修炼", "提高枪系攻击技能的攻击力40%。"},
		{10301, 20, "治愈术", "目标恢复最大生命的100%+670点当前生命。"},
	}
	for _, tt := range checks {
		def, _ := skills.Get(tt.id, tt.rank)
		if def.Name != tt.name {
			t.Fatal("wrong named fixture", def.Name, tt.name)
		}
		if got := s.skillOriginalEffectLines(def)[0]; got != tt.want {
			t.Errorf("%s rank%d got%s want%s", tt.name, tt.rank, got, tt.want)
		}
	}
}

func TestOriginalSkillDescriptionIncomingModifiersRefreshOnlyRecipient(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "术士", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	ch.Race, ch.Level, ch.SkillAllocationVersion, ch.SkillPoints = domain.Warlock, 100, 5, 90
	ch.Skills = domain.Learned{10401: 1, 10402: 3, 10426: 2}
	s.skills, s.statuses = skills, statuses
	views, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	fire := views[10401]
	if fire.CurrentEffectLines[0] != "对敌人造成230.88%伤害。" {
		t.Fatal("火弹术 did not retain wording with exact recipient coefficient", fire.CurrentEffectLines)
	}
	if fire.NextEffectLines[0] != "对敌人造成239.76%伤害。" || len(fire.NextEffectLines) != 4 {
		t.Fatal("next rank is not a complete original block", fire.NextEffectLines)
	}
	if len(fire.NextChangeLines) != 0 {
		t.Fatal("redundant compact summaries returned", fire.NextChangeLines)
	}
	if len(fire.TrainingSynergies) != 1 || fire.TrainingSynergies[0].SourceName != "火球术" || fire.TrainingSynergies[0].PerPointValue != 100 || fire.TrainingSynergies[0].InvestedHardPoints != 3 || fire.TrainingSynergies[0].ContributionValue != 300 {
		t.Fatal("missing incoming source/perpoint", fire.TrainingSynergies)
	}
	if !reflect.DeepEqual(fire.NativeModifierLines, []string{"已学被动使本技能伤害提升8%"}) {
		t.Fatal("original green detail lost", fire.NativeModifierLines)
	}
	mastery := views[10426]
	if !reflect.DeepEqual(mastery.CurrentEffectLines, []string{"火弹术，火球术的攻击力增加8%。"}) {
		t.Fatal("original passive prose replaced with outgoing list", mastery.CurrentEffectLines)
	}
	if len(mastery.TrainingSynergies) != 0 {
		t.Fatal("source card has outgoing rows", mastery.TrainingSynergies)
	}
	ch.Skills = domain.Learned{}
	views, err = s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(views[10401].CurrentEffectLines) != 0 || views[10401].NextEffectLines[0] != "对敌人造成208%伤害。" {
		t.Fatal("unlearned card lost first-rank preview", views[10401])
	}
}

func TestOriginalSkillDescriptionBoundedChanceAndControlUseActualFields(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for _, tt := range []struct {
		id   domain.SkillID
		want string
	}{
		{10210, "100%命中，造成1390%伤害，13%几率发生即死判定。"},
		{10432, "100%几率瞬间移动至指定点。"},
		{10016, "使敌人石化15秒。"},
	} {
		def, ok := skills.Get(tt.id, 100)
		if !ok {
			t.Fatal(tt.id)
		}
		got := s.skillOriginalEffectLines(def)[0]
		if got != tt.want {
			t.Errorf("%s: got%s want%s", def.Name, got, tt.want)
		}
	}
}

func TestOriginalSkillDescriptionControlContinuesBeforeEffectHit(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for _, tt := range []struct {
		rank        int32
		description string
		pressure    bool
	}{
		{5, "使敌人石化9秒。", false}, {6, "使敌人石化10秒。", false}, {11, "使敌人石化15秒。", false}, {12, "使敌人石化15秒。", true},
	} {
		def, ok := skills.Get(10016, tt.rank)
		if !ok {
			t.Fatal(tt.rank)
		}
		lines := s.skillOriginalEffectLines(def)
		if lines[0] != tt.description {
			t.Fatalf("石化之枪 rank%d: %v", tt.rank, lines)
		}
		joined := strings.Join(lines, "\n")
		if strings.Contains(joined, "本技能效果命中") != tt.pressure {
			t.Fatalf("pressure appears before actual duration boundary rank%d: %v", tt.rank, lines)
		}
		if tt.pressure && !strings.Contains(joined, "本技能效果命中 +5%") {
			t.Fatal("wrong actual pressure coefficient", lines)
		}
		if strings.Contains(joined, "抵抗概率降至") || len(lines) < 4 || !strings.HasPrefix(lines[1], "消耗:") || !strings.HasPrefix(lines[3], "技能间隔:") {
			t.Fatal("old pressure text or original block order changed", lines)
		}
	}
}

func TestOriginalSkillDescriptionGrowthAttributesUseActualEffectiveRank(t *testing.T) {
	skills, statuses := nativeRestoredSceneFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for _, tc := range []struct {
		id   domain.SkillID
		cap  int32
		name string
	}{
		{10024, 4, "体质"}, {10222, 10, "体质"}, {10229, 11, "体质"}, {10230, 11, "体质"},
		{10314, 17, "智慧"}, {10415, 17, "智慧"}, {10416, 5, "智慧"}, {10329, 5, "智慧"}, {10420, 5, "智慧"},
		{10306, 5, "精神"}, {10316, 5, "精神"}, {10318, 5, "精神"}, {10433, 5, "精神"}, {10218, 7, "精神"},
	} {
		for _, delta := range []int32{0, 1, 2, 50} {
			def, ok := skills.Get(tc.id, tc.cap+delta)
			if !ok {
				t.Fatalf("missing skill %d/%d", tc.id, tc.cap+delta)
			}
			lines := s.skillOriginalEffectLines(def)
			joined := strings.Join(lines, "\n")
			wanted := fmt.Sprintf("%s +%d", tc.name, delta)
			if delta == 0 && strings.Contains(joined, tc.name+" +") || delta > 0 && (lines[len(lines)-1] != wanted || strings.Count(joined, wanted) != 1) {
				t.Fatalf("%s rank%d actual attribute missing/duplicated: %v", def.Name, def.Level, lines)
			}
			for _, old := range []string{"法力效率", "恢复法力", "暗器时，普通攻击伤害额外", "减伤触发后", "参数", "抵抗概率降至"} {
				if strings.Contains(joined, old) {
					t.Fatalf("obsolete growth %s rank%d: %v", def.Name, def.Level, lines)
				}
			}
		}
	}
}

func TestOriginalSkillDescriptionHealingUsesActualRationalTrainingOutput(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "药师", 100, 100)
	p := s.players[1]
	p.Player.Char.Race = domain.Healer
	p.Player.Char.Skills = domain.Learned{10304: 1, 10301: 3, 10303: 2, 10307: 4}
	s.skills, s.statuses = skills, statuses
	def, ok := skills.Get(10304, 1)
	if !ok {
		t.Fatal("missing 群疗术")
	}
	s.applyTrainingSynergies(p, &def)
	lines := s.skillOriginalEffectLines(def)
	if lines[0] != "以自己为中心半径306像素的圆形范围内，队伍成员恢复最大生命的5.15%+189.52点生命值。" {
		t.Fatal("healing fraction/flat/geometry lost in original prose", lines)
	}
	if len(lines) != 4 {
		t.Fatal("full healing detail block lost", lines)
	}
}

func TestOriginalSkillDescriptionOnlyClosedEvasionWarningsAreRemoved(t *testing.T) {
	skills, statuses := nativeRestoredSceneFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for _, tc := range []struct {
		id   domain.SkillID
		want string
	}{
		{10229, "当触发陷阱时有100%的几率只受一半伤害。"},
		{10230, "被魔法击中时有100%几率只受一半伤害。"},
	} {
		def, _ := skills.Get(tc.id, 12)
		lines := s.skillOriginalEffectLines(def)
		if lines[0] != tc.want || lines[1] != "体质 +1" {
			t.Fatal("verified half-damage description changed unrelated prose", lines)
		}
	}
	unknown := domain.SkillDef{ID: 10212, Description: "原机制未闭合，此技能尚无法作用。"}
	if got := s.skillDescriptionForDisplay(unknown, unknown); got != unknown.Description {
		t.Fatal("unresolved warning stripped", got)
	}
}

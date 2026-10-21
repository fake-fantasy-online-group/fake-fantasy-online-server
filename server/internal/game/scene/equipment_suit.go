package scene

import (
	"fmt"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) appendSuitTooltip(lines *[]string, suit *domain.EquipmentSuit, owner *entity.Entity, equipmentTextQuality ...uint8) {
	if suit == nil {
		return
	}
	var worn *domain.EquipSet
	if owner != nil && owner.Player != nil {
		worn = owner.Player.Worn
	}
	pieces, members := suit.WornMembers(worn, s.itemDef)
	headerQuality := uint8(0)
	if len(equipmentTextQuality) != 0 {
		headerQuality = equipmentTextQuality[0]
	}
	headerLine := func(text string) string {
		if headerQuality == 0 {
			return "\x030" + text
		}
		return qualityColoredLine(headerQuality, text)
	}
	*lines = append(*lines, "", headerLine(
		fmt.Sprintf("套装: %s (%d/%d)", suit.Name, pieces, len(suit.Groups))))
	type memberLine struct {
		name   string
		active bool
	}
	var memberLines []memberLine
	byName := make(map[string]int)
	for _, group := range suit.Groups {
		for _, id := range group {
			def, ok := s.itemDef(id)
			if !ok {
				continue
			}
			name := suitMemberName(def.Name)
			if index, exists := byName[name]; exists {
				memberLines[index].active = memberLines[index].active || members[id]
				continue
			}
			byName[name] = len(memberLines)
			memberLines = append(memberLines, memberLine{name: name, active: members[id]})
		}
	}
	for _, member := range memberLines {
		color := "\x02"
		if member.active {
			color = "\x05"
		}
		*lines = append(*lines, color+"  "+member.name)
	}
	if suit.ID >= 1 && suit.ID <= 5 {
		*lines = append(*lines, headerLine("套装属性"))
		for _, tier := range []int32{2, 4, 6, 8} {
			color := "\x02"
			if s.classSetPieces(owner, suit.ID) >= tier {
				color = "\x05"
			}
			if extra := classSetSkillText(suit.ID, tier); extra != "" {
				*lines = append(*lines, color+extra)
			}
			for _, bonus := range suit.Bonuses {
				if bonus.Pieces != tier {
					continue
				}
				text, lineColor := equipmentSuitBonusText(bonus), color
				if bonus.DisabledReason != "" {
					text += "（" + bonus.DisabledReason + "）"
					lineColor = "\x02"
				}
				*lines = append(*lines, lineColor+text)
			}
		}
		return
	}
	shownSkills := map[int32]bool{}
	for _, bonus := range suit.Bonuses {
		text := equipmentSuitBonusText(bonus)
		color := "\x02"
		if bonus.DisabledReason != "" {
			text += "（" + bonus.DisabledReason + "）"
		} else if pieces >= bonus.Pieces && (domain.SuitCombatAffixName(bonus.Affix.Attr) == "" || s.classSetPieces(owner, suit.ID) >= bonus.Pieces) {
			color = "\x01"
		}
		*lines = append(*lines, color+fmt.Sprintf("[%d件] %s", bonus.Pieces, text))
		if extra := classSetSkillText(suit.ID, bonus.Pieces); extra != "" && !shownSkills[bonus.Pieces] {
			shownSkills[bonus.Pieces] = true
			skillColor := "\x02"
			if s.classSetPieces(owner, suit.ID) >= bonus.Pieces {
				skillColor = "\x01"
			}
			*lines = append(*lines, skillColor+fmt.Sprintf("[%d件] %s", bonus.Pieces, extra))
		}
	}
}

// 性别只区分可替代款，不是套装成员名称的一部分；其它括号说明保留。
func suitMemberName(name string) string {
	name = strings.TrimSpace(name)
	for _, suffix := range []string{"(男)", "(女)", "（男）", "（女）", "(男）", "(女）", "（男)", "（女)"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(name, suffix))
		}
	}
	return name
}

func equipmentSuitBonusText(bonus domain.EquipmentSuitBonus) string {
	if name := domain.SuitCombatAffixName(bonus.Affix.Attr); name != "" {
		unit := "%"
		if bonus.Affix.Attr == domain.AttrSuitPhysicalCritDamageBP {
			unit = "个百分点"
		}
		return fmt.Sprintf("%s +%g%s", name, float64(bonus.Affix.Value)/100, unit)
	}
	if text, ok := equipmentAffixLine(bonus.Affix); ok && bonus.Affix.Mode != 4 {
		return text
	}
	if text := strings.TrimSpace(bonus.Description); text != "" {
		if bonus.Probability > 0 && bonus.Probability < 100 {
			return fmt.Sprintf("%d%%几率: %s", bonus.Probability, text)
		}
		return text
	}
	// 名称来自原始词条及已有资料，只有显示含义，不据此猜结算公式。
	name := map[int32]string{
		21: "躲避", 37: "法力恢复", 49: "伤害抗性", 104: "魔法抗性",
		124: "护甲", 125: "魔抗", 441: "攻击时生命回复",
		459: "金行属性", 461: "水行属性", 462: "火行属性",
	}[bonus.Affix.Attr]
	if name == "" {
		name = fmt.Sprintf("效果 %d", bonus.Affix.Attr)
	}
	suffix := ""
	if bonus.Affix.Mode == domain.ModePercent {
		suffix = "%"
	}
	text := fmt.Sprintf("%s %+d%s", name, bonus.Affix.Value, suffix)
	if bonus.Probability > 0 && bonus.Probability < 100 {
		text = fmt.Sprintf("%d%%几率: %s", bonus.Probability, text)
	}
	return text
}

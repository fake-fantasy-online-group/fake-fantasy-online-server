package scene

import (
	"fmt"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) appendSuitTooltip(lines *[]string, suit *domain.EquipmentSuit, owner *entity.Entity) {
	if suit == nil {
		return
	}
	var worn *domain.EquipSet
	if owner != nil && owner.Player != nil {
		worn = owner.Player.Worn
	}
	pieces, members := suit.WornMembers(worn, s.itemDef)
	*lines = append(*lines, "", qualityColoredLine(2,
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
	for _, bonus := range suit.Bonuses {
		text := equipmentSuitBonusText(bonus)
		color := "\x02"
		if bonus.DisabledReason != "" {
			text += "（" + bonus.DisabledReason + "）"
		} else if pieces >= bonus.Pieces {
			color = "\x01"
		}
		*lines = append(*lines, color+fmt.Sprintf("[%d件] %s", bonus.Pieces, text))
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

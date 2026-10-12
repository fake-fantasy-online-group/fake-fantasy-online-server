package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
	"strings"
)

func (s *Scene) sendGrowthV5Skills(p *entity.Entity, req domain.EndgameRequest) {
	if p == nil || p.Player == nil || !p.Player.EndgameGrowthV5 {
		s.endgameError(p, req, "update_required", "请更新客户端后查看成长技能。")
		return
	}
	snapshot, err := s.skillRanksForDisplay(p)
	if err != nil {
		s.endgameError(p, req, "skill_state", "技能状态暂时无法解析。")
		return
	}
	rows := make([]domain.SkillRankView, 0, len(snapshot))
	for _, row := range snapshot {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	s.endgameReply(p, domain.EndgameResponse{Kind: "skill.ranks", RequestID: req.RequestID, OK: true, SkillRanks: rows, Result: map[string]string{"hardCap": "20", "playerCap": "100", "petCap": "80", "unspentPoints": fmt.Sprint(p.Player.Char.SkillPoints), "formulaVersion": "5"}})
}

func equipmentElementName(e string) string {
	switch e {
	case "fire":
		return "火系"
	case "ice":
		return "冰系"
	case "holy":
		return "圣系"
	case "dark":
		return "暗系"
	}
	return e
}
func equipmentPercent(bp int32) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(bp)/100), "0"), ".") + "%"
}

// equipmentV5Lines keeps ordinary bonuses distinct from triggered/special effects.
// The caller owns their native colors and placement; never infer effect type
// from a localized display string.
func (s *Scene) equipmentV5Lines(effects []domain.EquipmentEffect, bonuses []domain.SkillRankBonus) (attributes, specialEffects []string) {
	for _, b := range bonuses {
		scope := ""
		switch b.Scope {
		case "all":
			scope = "所有战斗技能"
		case "class":
			scope = map[int32]string{1: "战士", 2: "剑客", 3: "刺客", 4: "药师", 5: "术士"}[b.Profession] + "技能"
		case "branch":
			scope = map[int32]string{1: "战士", 2: "剑客", 3: "刺客", 4: "药师", 5: "术士"}[b.Profession] + map[string]string{"left": "左系主修", "right": "右系主修", "auxiliary": "辅助", "aux": "辅助"}[b.Branch] + "技能"
		case "school":
			scope = map[string]string{"flame": "火系技能", "frost": "冰系技能", "healing": "治疗技能", "trap": "陷阱技能"}[b.School]
		case "single":
			if id, ok := domain.V5SkillIdentity(b.SkillID); ok {
				scope = id.Name
			} else {
				scope = fmt.Sprintf("技能%d", b.SkillID)
			}
		}
		if scope != "" {
			attributes = append(attributes, fmt.Sprintf("%s +%d级", scope, b.Levels))
		}
	}
	for _, e := range effects {
		text := ""
		el := equipmentElementName(e.Element)
		switch e.Kind {
		case "magic_find_pct":
			text = fmt.Sprintf("魔法装备获取几率提升 +%d%%", e.Value)
		case "element_damage_bp":
			text = el + "伤害 +" + equipmentPercent(e.Value)
		case "enemy_element_pierce_bp":
			text = "敌人" + el + "抗性 -" + equipmentPercent(e.Value)
		case "element_resist_bp":
			text = el + "抗性 +" + equipmentPercent(e.Value)
		case "element_mitigation_bp":
			text = el + "伤害减免 " + equipmentPercent(e.Value)
		case "element_absorb_bp":
			text = el + "伤害吸收 " + equipmentPercent(e.Value)
		case "proc":
			if e.Proc != nil {
				v := e.Proc
				when := "攻击命中时"
				if v.Trigger == "on_direct_damage_taken" {
					when = "受到伤害时"
				}
				if v.Trigger == "on_attack" {
					when = "攻击时"
				}
				name := fmt.Sprintf("技能%d", v.SkillID)
				if id, ok := domain.V5SkillIdentity(v.SkillID); ok {
					name = id.Name
				}
				if v.RequireMagic {
					when = "法术命中时"
				}
				if v.HPBelowPct > 0 {
					when = fmt.Sprintf("生命不高于%d%%时，%s", v.HPBelowPct, when)
				}
				target := ""
				if v.TargetRule == "self" {
					target = "对自身"
				}
				if v.Payload == "damage" && v.MaxTargets == 1 {
					target = "对单个目标"
				}
				text = fmt.Sprintf("%s有%s概率%s释放%d级%s（冷却%g秒）", when, equipmentPercent(v.ChanceBP), target, v.FixedRank, name, float64(v.ICDMS)/1000)
				if v.ScaleBP != 10000 {
					text += "，效力" + equipmentPercent(v.ScaleBP)
				}
			}
		case "aura":
			if e.Aura != nil {
				a := e.Aura
				benefit := "物理伤害减免 " + equipmentPercent(a.Value)
				if a.Modifier == "element_resist_bp" {
					benefit = equipmentElementName(a.Element) + "抗性 +" + equipmentPercent(a.Value)
				}
				text = fmt.Sprintf("赋予%d级%s光环：%s（范围%d，含自身最多%d人）", a.FixedRank, a.VisualKey, benefit, a.RadiusPX, a.MaxPlayers)
				if a.IncludeActivePet {
					text += "，惠及出战宠物"
				}
			}
		}
		if text != "" {
			if e.Kind == "proc" || e.Kind == "aura" {
				specialEffects = append(specialEffects, text)
			} else {
				attributes = append(attributes, text)
			}
		}
	}
	return attributes, specialEffects
}

package scene

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/big"
	"strconv"
	"strings"
)

func skillFractionText(fallback int32, n, d int64) string {
	if d <= 0 {
		return strconv.Itoa(int(fallback))
	}
	return strings.TrimRight(strings.TrimRight(new(big.Rat).SetFrac64(n, d).FloatString(2), "0"), ".")
}
func skillAttrText(attr int32) string {
	switch attr {
	case domain.AttrAtk:
		return "攻击力"
	case domain.AttrMAtk:
		return "魔法攻击"
	case domain.AttrDef:
		return "防御"
	case domain.AttrMDef:
		return "魔法防御"
	case domain.AttrMaxHP:
		return "最大生命"
	case domain.AttrMaxMP:
		return "最大法力"
	case domain.AttrMoveSpeed:
		return "移动速度"
	case domain.AttrSTR:
		return "力量"
	case domain.AttrINT:
		return "智力"
	case domain.AttrVIT:
		return "体质"
	case domain.AttrSPI:
		return "精神"
	case domain.AttrDEX:
		return "灵巧"
	case domain.AttrAGI:
		return "敏捷"
	}
	return fmt.Sprintf("属性%d", attr)
}
func skillAffixText(a domain.Affix) string { return affixText(a) }
func skillTargetName(id domain.SkillID) string {
	if id == 0 {
		return "自身"
	}
	if d, ok := domain.V5SkillIdentity(id); ok {
		return d.Name
	}
	return "技能"
}
func (s *Scene) skillStatusName(id domain.StatusID) string {
	for _, d := range s.statuses {
		if d.ID == id && d.Name != "" {
			return d.Name
		}
	}
	return "异常状态"
}
func (s *Scene) skillStatusNames(ids []domain.StatusID) string {
	var names []string
	for _, id := range ids {
		names = append(names, s.skillStatusName(id))
	}
	return strings.Join(names, "、")
}
func (s *Scene) skillEffectLines(def domain.SkillDef) []string {
	out := []string{fmt.Sprintf("有效等级%d；法力消耗%d；吟唱%d毫秒；冷却%d毫秒", def.Level, def.MPCost, def.PrepareMS, def.CooldownMS)}
	for _, e := range def.DirectEffects() {
		switch e.Kind {
		case domain.EffectDamage:
			out = append(out, fmt.Sprintf("伤害倍率%s%%；固定伤害%+d", skillFractionText(e.DamagePct, e.DamagePctNumerator, e.DamagePctDenominator), e.DamageFlat))
		case domain.EffectHeal:
			out = append(out, fmt.Sprintf("恢复目标最大生命%s%% + %s点生命", skillFractionText(e.HealPctOfMax, e.HealPctNumerator, e.HealPctDenominator), skillFractionText(e.HealFlat, e.HealFlatNumerator, e.HealFlatDenominator)))
		case domain.EffectRestoreMP:
			out = append(out, fmt.Sprintf("恢复法力%d + 攻击力%d%%", e.RestoreMPFlat, e.SourceAttackPct))
		case domain.EffectRestoreHP:
			out = append(out, fmt.Sprintf("恢复生命%d + 攻击力%d%%", e.HealFlat, e.SourceAttackPct))
		case domain.EffectDamageMP:
			out = append(out, fmt.Sprintf("削减法力%d + 攻击力%d%%", e.DamageFlat, e.SourceAttackPct))
		case domain.EffectDamageMPMax:
			out = append(out, fmt.Sprintf("削减目标最大法力%d%%", e.MaxResourcePct))
		case domain.EffectTaunt:
			out = append(out, fmt.Sprintf("嘲讽强度%d", e.TauntPower))
		case domain.EffectResurrect:
			out = append(out, fmt.Sprintf("复活倒下的队友；恢复生命%d%%；返还本次死亡损失%d%%", e.HealPctOfMax, e.LossRefundPct))
		case domain.EffectInstantDeath:
			out = append(out, fmt.Sprintf("即死概率%s（受免疫限制）", equipmentPercent(e.ChanceBP)))
		case domain.EffectTeleportPoint, domain.EffectTeleportTarget:
			out = append(out, fmt.Sprintf("位移成功率%s；最大距离%d", equipmentPercent(e.ChanceBP), def.Dist))
		case domain.EffectPlaceTrap:
			out = append(out, fmt.Sprintf("陷阱半径%d", e.TrapRadius))
		case domain.EffectDetectTrap, domain.EffectDisarmTrap:
			out = append(out, fmt.Sprintf("陷阱侦测/解除成功率%s；范围%d", equipmentPercent(e.ChanceBP), def.Radius))
		case domain.EffectRevealInvisible:
			out = append(out, fmt.Sprintf("破隐范围%d", e.RevealRadius))
		case domain.EffectKnockback:
			out = append(out, fmt.Sprintf("击退%d；概率%s", e.KnockbackDistance, equipmentPercent(e.ChanceBP)))
		}
		if len(e.RemoveStatusIDs) > 0 {
			out = append(out, fmt.Sprintf("驱散%s；成功率%s", s.skillStatusNames(e.RemoveStatusIDs), equipmentPercent(e.DispelChanceBP)))
		}
		if e.ClearHarmful {
			out = append(out, "移除有害状态")
		}
	}
	if def.ProgressionShieldFlat > 0 {
		out = append(out, fmt.Sprintf("施术保护盾：吸收%d伤害，持续5秒", def.ProgressionShieldFlat))
	}
	for _, app := range def.Statuses {
		d, ok := s.statuses.Get(app.ID, app.Level)
		if !ok {
			continue
		}
		d.Affixes = overrideStatusAffixes(d.Affixes, app.ExtraAffixes)
		applyProgressionStatus(app, &d)
		if app.DurationSec > 0 {
			d.DurationSec = app.DurationSec
		}
		out = append(out, fmt.Sprintf("%s：持续%d秒", d.Name, d.DurationSec))
		for _, a := range d.Affixes {
			out = append(out, skillAffixText(a))
		}
		if d.ProgressionAttackFlat > 0 {
			out = append(out, fmt.Sprintf("下一击附加固定伤害%d", d.ProgressionAttackFlat))
		}
		if d.DamageShield > 0 {
			out = append(out, fmt.Sprintf("护盾吸收%d", d.DamageShield))
		}
		for _, c := range d.HPChanges {
			unit := "点"
			if c.Mode == domain.ModePercent {
				unit = "%"
			}
			out = append(out, fmt.Sprintf("每%d秒生命%+d%s", d.IntervalSec, c.Value, unit))
		}
		if d.PhysicalReflectFlat > 0 || d.MagicReflectFlat > 0 {
			out = append(out, fmt.Sprintf("物理/法术固定反伤%d/%d", d.PhysicalReflectFlat, d.MagicReflectFlat))
		}
	}
	for _, a := range def.PassiveStats {
		line := skillAffixText(domain.Affix{Attr: a.Attr, Value: a.Value, Mode: a.Mode})
		if a.RequiredEquipType > 0 {
			line += "（装备" + map[int32]string{1: "枪", 2: "剑", 8: "双刃", 32: "暗器", 105: "盾牌"}[a.RequiredEquipType] + "时）"
		}
		out = append(out, line)
	}
	for _, m := range def.PassiveModifiers {
		label := "效果增幅"
		switch m.Kind {
		case domain.PassiveEffectSourceAttackPct:
			label = "攻击转化比例%"
		case domain.PassiveIncomingMagicHalfChance:
			label = "法术伤害减半概率%"
		case domain.PassiveStealthDurationSec:
			label = "隐身时间（秒）"
		case domain.PassiveStealthMoveSpeedPct:
			label = "隐身移动速度%"
		case domain.PassiveStealthDamagePct:
			label = "隐身伤害加成%"
		case domain.PassiveTrapHalfDamageChance:
			label = "陷阱伤害减半概率%"
		case domain.PassiveStatusPhysicalDamageTakenPct:
			label = "目标物理易伤%"
		case domain.PassiveCastInterruptReductionPct:
			label = "吟唱打断抵抗%"
		case domain.PassiveSkillDamagePct:
			label = "伤害加成%"
		case domain.PassiveSkillDamageFlat:
			label = "固定伤害"
		case domain.PassiveEffectRestoreFlat:
			label = "固定资源量"
		case domain.PassiveCastTimeReductionPct:
			label = "吟唱减免%"
		case domain.PassiveSkillDurabilityReductionPct, domain.PassiveAttackDurabilityReductionPct:
			label = "耐久消耗减免%"
		case domain.PassivePreserveInvisibleChance:
			label = "保留隐身概率%"
		}
		out = append(out, fmt.Sprintf("%s：%s %+d", skillTargetName(m.TargetSkill), label, m.Value))
	}
	for _, m := range def.PassiveStatusAffixes {
		out = append(out, fmt.Sprintf("%s：%s", skillTargetName(m.TargetSkill), skillAffixText(m.Affix)))
	}
	for _, m := range def.PassiveResists {
		out = append(out, fmt.Sprintf("%s抗性%+d%%", s.skillStatusName(m.Status), m.Pct))
	}
	if def.PassiveCounter != nil {
		out = append(out, fmt.Sprintf("反击概率%s；伤害倍率%d%%", equipmentPercent(def.PassiveCounter.ChanceBP), def.PassiveCounter.DamagePct))
	}
	for _, m := range def.PassiveTriggers {
		out = append(out, fmt.Sprintf("%s附加%s：概率%s，持续%d秒", skillTargetName(m.TargetSkill), s.skillStatusName(m.Status), equipmentPercent(m.ChanceBP), m.DurationSec))
	}
	if def.TrainingEliteDamageBP > 0 {
		out = append(out, fmt.Sprintf("对精英伤害加成%s", equipmentPercent(def.TrainingEliteDamageBP)))
	}
	if def.Area.Valid() {
		out = append(out, fmt.Sprintf("范围：半径%d，长%d，宽%d，角度%d", def.Area.Radius, def.Area.Length, def.Area.Width, def.Area.Angle))
	}
	return out
}

// skillRanksForDisplay enriches a pure raw snapshot once. It never feeds display
// or already-modified outputs back into the resolver or synergy source graph.
func (s *Scene) skillRanksForDisplay(p *entity.Entity) (domain.SkillResolutionSnapshot, error) {
	ranks, err := s.skillRanks(p)
	if err != nil {
		return nil, err
	}
	raw := domain.Learned{}
	for id, r := range ranks {
		if r.CanCast {
			n, e := r.EngineRank()
			if e == nil {
				raw[id] = n
			}
		}
	}
	ch := p.Player.Char
	for id, r := range ranks {
		r.CurrentEffectLines = []string{}
		r.NextEffectLines = []string{}
		r.BlockedReasons = []string{}
		if !s.skills.V5Enabled(id) {
			ranks[id] = r
			continue
		}
		evaluate := func(n int32) []string {
			def, ok := s.skills.Get(id, n)
			if !ok {
				return []string{"技能数值配置无法解析"}
			}
			s.applyTrainingSynergiesWithHard(p, &def, ch.Skills)
			s.applyPassiveSkillModifiersWithLevels(p, &def, raw)
			s.applyUniqueSkillModifiers(p, &def)
			return s.skillEffectLines(def)
		}
		rank, e := r.EngineRank()
		if e != nil {
			r.BlockedReasons = append(r.BlockedReasons, "等级超过结算数值表示范围")
		} else if rank > 0 {
			r.CurrentEffectLines = evaluate(rank)
			if rank < 2147483647 {
				r.NextEffectLines = evaluate(rank + 1)
			}
		} else {
			n, ok := new(big.Int).SetString(r.SoftRank, 10)
			if ok {
				n.Add(n, big.NewInt(1))
				if n.IsInt64() && n.Int64() <= 2147483647 {
					r.NextEffectLines = evaluate(int32(n.Int64()))
				}
			}
		}

		if _, _, reason, ok := s.skillUpgradeTarget(p, id); !ok {
			r.BlockedReasons = append(r.BlockedReasons, skillInvestReason(reason))
		}
		if ch.SkillPoints < 1 {
			r.BlockedReasons = append(r.BlockedReasons, "技能点不足")
		}
		if ch.SkillAllocationVersion != 5 {
			r.BlockedReasons = append(r.BlockedReasons, "技能分配迁移未完成")
		}
		if !p.Alive() || !canAct(p) || endgameEconomicBusy(p) {
			r.BlockedReasons = append(r.BlockedReasons, "当前状态不能分配技能点")
		}
		r.CanInvest = len(r.BlockedReasons) == 0
		ranks[id] = r
	}
	return ranks, nil
}

// Hooked to stat/loadout refresh, not each attack. A digest suppresses unchanged
// snapshots from status refreshes and keeps sidecar authority current on breakage.
func (s *Scene) maybeSendSkillRanks(p *entity.Entity) {
	if p == nil || p.Player == nil || !p.Player.EndgameGrowthV5 {
		return
	}
	ranks, err := s.skillRanks(p)
	if err != nil {
		return
	}
	var worn []domain.Stack
	s.functionalWorn(p).Each(func(_ domain.EquipSlot, st domain.Stack) { worn = append(worn, st) })
	payload, _ := json.Marshal(struct {
		Ranks               domain.SkillResolutionSnapshot
		Hard                domain.Learned
		Worn                []domain.Stack
		Level, Points       int32
		Alive, CanAct, Busy bool
		Revision            int64
	}{ranks, p.Player.Char.Skills, worn, p.Player.Char.Level, p.Player.Char.SkillPoints, p.Alive(), canAct(p), endgameEconomicBusy(p), s.endgamePlayer(p).state.Revision})
	digest := sha256.Sum256(payload)
	if s.skillViewDigests == nil {
		s.skillViewDigests = map[domain.EntityID][32]byte{}
	}
	if s.skillViewDigests[p.ID] == digest {
		return
	}
	s.skillViewDigests[p.ID] = digest
	s.sendGrowthV5Skills(p, domain.EndgameRequest{Op: "skill.ranks"})
}

func skillInvestReason(reason event.RejectReason) string {
	switch reason {
	case event.RejectMaxLevel:
		return "投入等级已满20级"
	case event.RejectLevelTooLow:
		return "角色等级不足"
	case event.RejectWrongProf:
		return "职业不符或尚未就职"
	case event.RejectSkillPrerequisite:
		return "请先投入前置技能"
	}
	return "当前不能提升该技能"
}

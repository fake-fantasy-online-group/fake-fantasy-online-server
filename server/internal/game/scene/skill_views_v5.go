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
	if d, ok := s.statuses.Get(id, 1); ok && d.Name != "" {
		return d.Name
	}
	var name string
	var level int32
	for _, d := range s.statuses {
		if d.ID == id && d.Name != "" && (name == "" || d.Level < level || d.Level == level && d.Name < name) {
			name, level = d.Name, d.Level
		}
	}
	if name != "" {
		return name
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
	out := []string{}
	for _, e := range def.DirectEffects() {
		switch e.Kind {
		case domain.EffectDamage:
			parts := []string{}
			if e.DamagePct != 0 || e.DamagePctNumerator != 0 {
				parts = append(parts, "伤害倍率"+skillFractionText(e.DamagePct, e.DamagePctNumerator, e.DamagePctDenominator)+"%")
			}
			if e.DamageFlat != 0 {
				parts = append(parts, fmt.Sprintf("固定伤害%+d", e.DamageFlat))
			}
			if len(parts) > 0 {
				out = append(out, strings.Join(parts, "，"))
			}
		case domain.EffectHeal:
			parts := []string{}
			if e.HealPctOfMax != 0 || e.HealPctNumerator != 0 {
				parts = append(parts, "目标最大生命的"+skillFractionText(e.HealPctOfMax, e.HealPctNumerator, e.HealPctDenominator)+"%")
			}
			if e.HealFlat != 0 || e.HealFlatNumerator != 0 {
				parts = append(parts, skillFractionText(e.HealFlat, e.HealFlatNumerator, e.HealFlatDenominator)+"点生命")
			}
			if len(parts) > 0 {
				out = append(out, "恢复"+strings.Join(parts, " + "))
			}
		case domain.EffectRestoreMP:
			out = append(out, skillResourceText("恢复法力", e.RestoreMPFlat, e.SourceAttackPct))
		case domain.EffectRestoreHP:
			out = append(out, skillResourceText("恢复生命", e.HealFlat, e.SourceAttackPct))
		case domain.EffectDamageMP:
			out = append(out, skillResourceText("削减法力", e.DamageFlat, e.SourceAttackPct))
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
		d, ok := s.statuses.ResolveSkillApplication(app)
		if !ok {
			continue
		}
		d.Affixes = overrideStatusAffixes(d.Affixes, app.ExtraAffixes)
		applyProgressionStatus(app, &d)
		if app.DurationSec > 0 && app.ProgressionRanks <= 0 {
			d.DurationSec = app.DurationSec
		}
		out = append(out, fmt.Sprintf("%s：持续%d秒", d.Name, d.DurationSec))
		if d.ControlPower > 0 {
			out = append(out, skillControlResistanceLine(d))
		}
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
	out = append(out, skillPassiveModifierLines(def.PassiveModifiers)...)
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
		name, duration := s.skillStatusName(m.Status), m.DurationSec
		controlLine := ""
		if state, ok := s.statuses.ResolveSkillApplication(domain.StatusApplication{ID: m.Status, Level: m.StatusLevel, ProgressionRanks: m.ProgressionRanks, ProgressionFirstLevel: m.ProgressionFirstLevel, DurationSec: m.DurationSec, ProgressionFirstDurationSec: m.ProgressionFirstDurationSec, ProgressionBaseDurationSec: m.ProgressionBaseDurationSec}); ok {
			if state.Name != "" {
				name = state.Name
			}
			if duration <= 0 || m.ProgressionRanks > 0 {
				duration = state.DurationSec
			}
			if state.ControlPower > 0 {
				controlLine = skillControlResistanceLine(state)
			}
		}
		line := fmt.Sprintf("%s附加%s：概率%s", skillTargetName(m.TargetSkill), name, equipmentPercent(m.ChanceBP))
		if duration > 0 {
			line += fmt.Sprintf("，持续%d秒", duration)
		}
		out = append(out, line)
		if controlLine != "" {
			out = append(out, controlLine)
		}
	}
	if def.TrainingEliteDamageBP > 0 {
		out = append(out, fmt.Sprintf("对精英伤害加成%s", equipmentPercent(def.TrainingEliteDamageBP)))
	}
	if def.Area.Valid() {
		parts := []string{}
		if def.Area.Radius > 0 {
			parts = append(parts, fmt.Sprintf("半径%d", def.Area.Radius))
		}
		if def.Area.Length > 0 {
			parts = append(parts, fmt.Sprintf("长%d", def.Area.Length))
		}
		if def.Area.Width > 0 {
			parts = append(parts, fmt.Sprintf("宽%d", def.Area.Width))
		}
		if def.Area.Angle > 0 {
			parts = append(parts, fmt.Sprintf("角度%d", def.Area.Angle))
		}
		out = append(out, "范围："+strings.Join(parts, "，"))
	}
	if def.Kind.Active() {
		cost, cast := fmt.Sprintf("%d点法力", def.MPCost), skillMillisecondsText(def.PrepareMS)
		if def.MPCost == 0 {
			cost = "无"
		}
		if def.PrepareMS == 0 {
			cast = "瞬发"
		}
		out = append(out, "消耗："+cost, "技能吟唱："+cast, "技能间隔："+skillMillisecondsText(def.CooldownMS))
	}
	return out
}

func skillMillisecondsText(ms int32) string {
	return skillFractionText(0, int64(ms), 1000) + "秒"
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
	bonuses := s.skillRankBonuses(p)
	for id, r := range ranks {
		r.CurrentEffectLines = []string{}
		r.NextEffectLines = []string{}
		r.BlockedReasons = []string{}
		r.NextEffectiveRank = ""
		if ident, ok := domain.V5SkillIdentity(id); ok && r.HardRank < domain.SkillHardPointCap {
			r.NextRequiredLevel = ident.UnlockLevel + r.HardRank
		}
		r.TrainingSynergies, r.NativeModifierLines = s.skillSynergyViews(id, ch.Skills, raw)
		if !s.skills.V5Enabled(id) {
			ranks[id] = r
			continue
		}
		r.NextChangeLines = nil
		evaluate := func(n int32) []string {
			def, ok := s.skills.Get(id, n)
			if !ok {
				return []string{"技能数值配置无法解析"}
			}
			s.applyTrainingSynergiesWithHard(p, &def, ch.Skills)
			s.applyPassiveSkillModifiersWithLevels(p, &def, raw)
			s.applyUniqueSkillModifiers(p, &def)
			return s.skillOriginalEffectLines(def)
		}
		rank, e := r.EngineRank()
		if e != nil {
			r.BlockedReasons = append(r.BlockedReasons, "等级超过结算数值表示范围")
		} else if rank > 0 {
			r.CurrentEffectLines = evaluate(rank)
			if rank < 2147483647 {
				next := big.NewInt(int64(rank) + 1)
				// A granted but uninvested skill can start accepting equipment ranks
				// on its first hard point. Read all eligible authored bonuses then.
				if r.HardRank == 0 && r.GrantRank != "0" {
					ident, _ := domain.V5SkillIdentity(id)
					next = firstSkillInvestmentRank(r.GrantRank, ident, bonuses)
				}
				if next.IsInt64() && next.Int64() <= 2147483647 {
					r.NextEffectiveRank = next.String()
					r.NextEffectLines = evaluate(int32(next.Int64()))
				}
			}
		} else {
			n, ok := new(big.Int).SetString(r.SoftRank, 10)
			if ok {
				n.Add(n, big.NewInt(1))
				if n.IsInt64() && n.Int64() <= 2147483647 {
					r.NextEffectiveRank = n.String()
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

// firstSkillInvestmentRank follows the resolver's hard+grant+eligible-soft rule.
func firstSkillInvestmentRank(grant string, ident domain.SkillIdentity, bonuses []domain.SkillRankBonus) *big.Int {
	n, ok := new(big.Int).SetString(grant, 10)
	if !ok {
		n = new(big.Int)
	}
	n.Add(n, big.NewInt(1))
	seen := map[string]bool{}
	for _, b := range bonuses {
		if !seen[b.SourceID] && b.Matches(ident, ident.Profession) {
			seen[b.SourceID] = true
			n.Add(n, big.NewInt(b.Levels))
		}
	}
	return n
}

func skillResourceText(label string, flat, attackPct int32) string {
	parts := []string{}
	if flat != 0 {
		parts = append(parts, fmt.Sprintf("%d点", flat))
	}
	if attackPct != 0 {
		parts = append(parts, fmt.Sprintf("攻击力的%d%%", attackPct))
	}
	if len(parts) == 0 {
		return label + "0点"
	}
	return label + strings.Join(parts, " + ")
}

func skillPassiveModifierText(m domain.PassiveModifier) string {
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
	unit := ""
	if strings.HasSuffix(label, "%") {
		label = strings.TrimSuffix(label, "%")
		unit = "%"
	}
	if strings.HasSuffix(label, "（秒）") {
		label = strings.TrimSuffix(label, "（秒）")
		unit = "秒"
	}

	return fmt.Sprintf("%s %+d%s", label, m.Value, unit)
}

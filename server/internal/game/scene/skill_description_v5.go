package scene

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

var skillDescriptionNumber = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

// skillOriginalEffectLines preserves the complete authored description and the
// three original active-skill detail rows. Numeric replacements are bound to
// known semantic slots below, never inferred from a matching number or name.
// In particular, native passive prose is retained without expanding its target
// membership into a newly generated outgoing-synergy list.
func (s *Scene) skillOriginalEffectLines(def domain.SkillDef) []string {
	baseline, ok := s.skills[domain.SkillKey{ID: def.ID, Level: min(def.Level, 5)}]
	if !ok {
		baseline = def
	}
	description := s.skillDescriptionForDisplay(baseline, def)
	if description == "" {
		return s.skillEffectLines(def)
	}
	lines := []string{description}
	if scoped := s.skillScopedStatusDescription(def); scoped != "" {
		lines = append(lines, scoped)
	}
	if random := s.skillRandomStatusDescription(def); random != "" {
		lines = append(lines, random)
	}
	var effectHitLines []string
	if def.Level > 5 {
		seen := map[string]bool{}
		appendLine := func(line string) {
			if !seen[line] {
				seen[line] = true
				effectHitLines = append(effectHitLines, line)
			}
		}
		add := func(state domain.StatusDef, found bool) {
			if !found {
				return
			}
			if state.EffectHitBP > 0 {
				appendLine(skillEffectHitLine(state.EffectHitBP))
			}
			if state.FreezeAttackerEffectHitBP > 0 {
				appendLine("反击冻结效果命中 +" + skillFractionText(0, int64(state.EffectHitBP)+int64(state.FreezeAttackerEffectHitBP), 100) + "%")
			}
		}
		// Appended statuses come from incoming passives; their extra detail is
		// shown in the recipient modifier section, not the original level block.
		for _, app := range def.Statuses[:min(len(def.Statuses), len(baseline.Statuses))] {
			add(s.skillDescriptionStatus(app))
		}
		for i := range def.PassiveTriggers {
			add(s.skillDescriptionTrigger(def, i))
		}
		for _, effect := range def.DirectEffects() {
			if effect.EffectHitBP > 0 {
				appendLine(skillEffectHitLine(effect.EffectHitBP))
			}
		}
	}
	if def.Kind.Active() {
		cost, cast := fmt.Sprintf("%d点法力", def.MPCost), skillMillisecondsText(def.PrepareMS)
		if def.MPCost == 0 {
			cost = "无"
		}
		if def.PrepareMS == 0 {
			cast = "瞬发"
		}
		lines = append(lines, "消耗: "+cost, "技能吟唱: "+cast, "技能间隔: "+skillMillisecondsText(def.CooldownMS))
	}
	lines = append(lines, effectHitLines...)
	if policy, ok := domain.LookupSkillGrowthPolicy(def.ID, s.skills.ProgressionCatalog()); ok && policy.RangeCapRank > 0 && def.Level > policy.RangeCapRank {
		switch def.ID {
		case 10127, 10228:
			if def.Area.Radius > 0 {
				lines = append(lines, "作用半径: "+skillNumber(def.Area.Radius)+"像素")
			} else if def.Radius > 0 {
				lines = append(lines, "作用半径: "+skillNumber(def.Radius)+"像素")
			}
		}
	}
	// These three authored descriptions omit protection time altogether. Keep
	// their original prose and expose only the duration the resolver actually uses.
	if policy, ok := domain.LookupSkillGrowthPolicy(def.ID, s.skills.ProgressionCatalog()); ok && policy.ProtectionCapRank > 0 && def.Level > policy.ProtectionCapRank {
		switch def.ID {
		case 10417, 10425, 10435:
			if len(def.Statuses) > 0 {
				if state, found := s.skillDescriptionStatus(def.Statuses[0]); found {
					lines = append(lines, "保护持续时间: "+skillNumber(state.DurationSec)+"秒")
				}
			}
		}
	}
	lines = append(lines, s.passiveOverflowEffectLines(def)...)
	return lines
}

func skillEffectHitLine(effectHitBP int32) string {
	return "本技能效果命中 +" + skillBasisPoints(effectHitBP) + "%"
}

func (s *Scene) skillDescriptionForDisplay(baseline, def domain.SkillDef) string {
	description := baseline.Description
	// These original placeholders are obsolete: both native half-damage paths
	// are implemented and tested. Do not strip warnings on any other skill.
	if def.ID == 10229 || def.ID == 10230 {
		description = strings.ReplaceAll(description, "，此技能尚无法作用", "")
	}
	old, known := s.skillDescriptionValues(baseline)
	next, nextKnown := s.skillDescriptionValues(def)
	slots := skillDescriptionNumber.FindAllStringIndex(description, -1)
	// Fail closed on a changed template: a slot must have a verified meaning in
	// both definitions. Tests cover every authored class-skill template and rank.
	if !known || !nextKnown || len(slots) != len(old) || len(next) != len(old) {
		return description
	}
	for i := len(slots) - 1; i >= 0; i-- {
		if old[i] == "" || next[i] == "" || old[i] == next[i] {
			continue
		}
		p := slots[i]
		description = description[:p[0]] + next[i] + description[p[1]:]
	}
	return description
}

func skillNumber(value int32) string { return strconv.FormatInt(int64(value), 10) }
func skillMagnitude(value int32) string {
	n := int64(value)
	if n < 0 {
		n = -n
	}
	return strconv.FormatInt(n, 10)
}
func skillBasisPoints(value int32) string { return skillFractionText(0, int64(value), 100) }
func skillDescriptionEffect(def domain.SkillDef, kind domain.EffectKind) (domain.SkillEffect, bool) {
	for _, e := range def.DirectEffects() {
		if e.Kind == kind {
			return e, true
		}
	}
	return domain.SkillEffect{}, false
}
func skillDescriptionModifier(def domain.SkillDef, kind domain.PassiveModifierKind) string {
	for _, m := range def.PassiveModifiers {
		if m.Kind == kind {
			return skillNumber(m.Value)
		}
	}
	return ""
}
func skillDescriptionAffix(state domain.StatusDef, attr int32) string {
	for _, a := range state.Affixes {
		if a.Attr == attr {
			return skillMagnitude(a.Value)
		}
	}
	return ""
}
func (s *Scene) skillDescriptionStatus(app domain.StatusApplication) (domain.StatusDef, bool) {
	return s.resolveSkillStatusDef(domain.SkillDef{}, app, 0)
}
func (s *Scene) skillDescriptionTrigger(def domain.SkillDef, index int) (domain.StatusDef, bool) {
	if index >= len(def.PassiveTriggers) {
		return domain.StatusDef{}, false
	}
	m := def.PassiveTriggers[index]
	return s.skillDescriptionStatus(domain.StatusApplication{Progression: def.Progression, ID: m.Status, Level: m.StatusLevel, ChanceBP: m.ChanceBP, ChanceExplicit: m.ChanceExplicit, EffectHitBP: m.EffectHitBP, SourceSkill: def.ID, DurationSec: m.DurationSec, ProgressionRanks: m.ProgressionRanks, ProgressionFirstLevel: m.ProgressionFirstLevel, ProgressionFirstDurationSec: m.ProgressionFirstDurationSec, ProgressionBaseDurationSec: m.ProgressionBaseDurationSec})
}
func skillDescriptionPeriodicTotal(d domain.StatusDef) string {
	if d.IntervalSec <= 0 {
		return ""
	}
	var tick int64
	for _, c := range d.HPChanges {
		if c.Mode == domain.ModeAbsolute {
			tick += int64(c.Value)
		}
	}
	for _, a := range d.Affixes {
		if a.Attr == domain.AttrHPRegen && a.Mode == domain.ModeAbsolute {
			tick += int64(a.Value)
		}
	}
	if tick < 0 {
		tick = -tick
	}
	return strconv.FormatInt(tick*int64(d.DurationSec/d.IntervalSec), 10)
}

// The ordered slots refer to the literal original prose, including constants
// such as guaranteed-hit percentages. Empty slots intentionally leave authored
// text unchanged when the original structured definition has no closed effect.
func (s *Scene) skillDescriptionValues(d domain.SkillDef) ([]string, bool) {
	damage := ""
	if e, ok := skillDescriptionEffect(d, domain.EffectDamage); ok {
		damage = skillFractionText(e.DamagePct, e.DamagePctNumerator, e.DamagePctDenominator)
	}
	state := func(index int) (domain.StatusDef, bool) {
		if index >= len(d.Statuses) {
			return domain.StatusDef{}, false
		}
		return s.skillDescriptionStatus(d.Statuses[index])
	}
	statusField := func(index int, field func(domain.StatusDef) string) string {
		if x, ok := state(index); ok {
			return field(x)
		}
		return ""
	}
	duration := func(index int) string {
		return statusField(index, func(x domain.StatusDef) string { return skillNumber(x.DurationSec) })
	}
	affix := func(index int, attr int32) string {
		return statusField(index, func(x domain.StatusDef) string { return skillDescriptionAffix(x, attr) })
	}
	triggerField := func(index int, field func(domain.StatusDef) string) string {
		if x, ok := s.skillDescriptionTrigger(d, index); ok {
			return field(x)
		}
		return ""
	}
	triggerDuration := func(index int) string {
		return triggerField(index, func(x domain.StatusDef) string { return skillNumber(x.DurationSec) })
	}
	triggerChance := func(index int) string {
		if index < len(d.PassiveTriggers) {
			return skillBasisPoints(d.PassiveTriggers[index].ChanceBP)
		}
		return ""
	}
	effectValue := func(kind domain.EffectKind, field func(domain.SkillEffect) string) string {
		if x, ok := skillDescriptionEffect(d, kind); ok {
			return field(x)
		}
		return ""
	}
	chance := func(kind domain.EffectKind) string {
		return effectValue(kind, func(x domain.SkillEffect) string { return skillBasisPoints(x.ChanceBP) })
	}
	sourceAttack := func(kind domain.EffectKind) string {
		return effectValue(kind, func(x domain.SkillEffect) string { return skillNumber(x.SourceAttackPct) })
	}
	passive := func(kind domain.PassiveModifierKind) string { return skillDescriptionModifier(d, kind) }
	radius := skillNumber(d.Area.Radius)
	triggerEffectChance := func() string {
		if len(d.PassiveTriggerEffects) > 0 {
			return skillBasisPoints(d.PassiveTriggerEffects[0].Effect.ChanceBP)
		}
		return ""
	}
	statusChance := func(index int) string {
		if index < len(d.Statuses) {
			return skillBasisPoints(d.Statuses[index].ChanceBP)
		}
		return ""
	}
	switch d.ID {
	case 10001, 10101, 10118, 10424, 10426, 10431:
		return []string{passive(domain.PassiveSkillDamagePct)}, true
	case 10002, 10017, 10102, 10117, 10205, 10223, 10317, 10319, 10401, 10408, 10409:
		return []string{damage}, true
	case 10212:
		if effect, ok := d.TheftEffect(); ok {
			return []string{skillBasisPoints(effect.ChanceBP)}, true
		}
		return []string{damage}, true
	case 10003:
		return []string{effectValue(domain.EffectDamage, func(x domain.SkillEffect) string { return skillNumber(x.WeaponDurabilityCostPct) }), damage}, true
	case 10004, 10210:
		return []string{"", damage, chance(domain.EffectInstantDeath)}, true
	case 10005, 10021:
		return []string{skillNumber(d.Area.Width), skillNumber(d.Area.Length), damage}, true
	case 10006, 10206:
		return []string{skillNumber(d.Area.Angle), radius, damage}, true
	case 10007, 10022, 10121, 10125, 10225, 10312, 10313, 10402, 10405, 10412:
		return []string{radius, damage}, true
	case 10008, 10025, 10026, 10108:
		if len(d.PassiveResists) > 0 {
			return []string{skillNumber(d.PassiveResists[0].Pct)}, true
		}
		return []string{""}, true
	case 10009:
		return []string{damage, duration(0), affix(0, domain.AttrDef)}, true
	case 10010:
		return []string{duration(0), affix(0, domain.AttrAtk), affix(0, domain.AttrMoveSpeed), affix(0, domain.AttrDef)}, true
	case 10011, 10130:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.NextBasicAttackPct) })}, true
	case 10012:
		return []string{triggerDuration(0), triggerField(0, func(x domain.StatusDef) string { return skillNumber(x.PhysicalDamageTakenPct) })}, true
	case 10013:
		return []string{damage, sourceAttack(domain.EffectDamageMP)}, true
	case 10014, 10112, 10120:
		return []string{passive(domain.PassiveEffectSourceAttackPct)}, true
	case 10015, 10023, 10116, 10209:
		return []string{"", damage}, true
	case 10016, 10114, 10428:
		return []string{duration(0)}, true
	case 10018:
		return []string{duration(0), affix(0, domain.AttrMoveSpeed)}, true
	case 10019:
		return []string{triggerDuration(0), triggerField(0, func(x domain.StatusDef) string { return skillDescriptionAffix(x, domain.AttrMoveSpeed) }), triggerChance(0)}, true
	case 10020, 10226:
		return []string{triggerEffectChance()}, true
	case 10024, 10105, 10222, 10227, 10306, 10316, 10318, 10429, 10433:
		return []string{}, true
	case 10027, 10111, 10113, 10122, 10126, 10219, 10221:
		return []string{damage, duration(0)}, true
	case 10028, 10127:
		return []string{radius}, true
	case 10029, 10128, 10413:
		return []string{radius, duration(0)}, true
	case 10030:
		return []string{duration(0), ""}, true
	case 10103:
		return []string{damage, duration(0), affix(0, domain.AttrHit)}, true
	case 10104:
		return []string{damage, sourceAttack(domain.EffectRestoreHP)}, true
	case 10106, 10202, 10204:
		if len(d.PassiveStats) > 0 {
			return []string{skillNumber(d.PassiveStats[0].Value)}, true
		}
		return []string{""}, true
	case 10107:
		return []string{duration(0), statusField(0, func(x domain.StatusDef) string { return skillNumber(x.PhysicalDamageReductionPct) })}, true
	case 10109:
		return []string{duration(0), affix(0, domain.AttrDef)}, true
	case 10110:
		return []string{triggerChance(0), triggerDuration(0)}, true
	case 10115:
		return []string{duration(0), affix(0, domain.AttrDef), affix(0, domain.AttrAtk)}, true
	case 10119:
		return []string{damage, sourceAttack(domain.EffectRestoreMP)}, true
	case 10123, 10414:
		return []string{damage, chance(domain.EffectInstantDeath)}, true
	case 10124:
		return []string{duration(0), affix(0, domain.AttrAtk), affix(0, domain.AttrDef)}, true
	case 10129:
		return []string{duration(0), statusField(0, func(x domain.StatusDef) string { return skillNumber(x.AttackCap) })}, true
	case 10201:
		values := []string{passive(domain.PassivePreserveInvisibleChance), triggerChance(0)}
		// Migration73 adds the approved stun duration in the original sentence.
		// Keep two-slot historical templates compatible until their data is updated.
		if len(d.PassiveTriggers) > 0 && len(skillDescriptionNumber.FindAllStringIndex(d.Description, -1)) == 3 {
			values = append(values, triggerDuration(0))
		}
		return values, true
	case 10203:
		if d.PassiveCounter != nil {
			return []string{skillBasisPoints(d.PassiveCounter.ChanceBP)}, true
		}
		return []string{""}, true
	case 10207:
		return []string{passive(domain.PassiveStealthDurationSec), passive(domain.PassiveStealthMoveSpeedPct), passive(domain.PassiveStealthDamagePct)}, true
	case 10208:
		dmg := statusField(0, func(x domain.StatusDef) string { return skillNumber(x.StealthDamagePct) })
		if strings.Contains(d.Description, "移动速度") {
			return []string{duration(0), affix(0, domain.AttrMoveSpeed), dmg}, true
		}
		return []string{duration(0), dmg}, true
	case 10211:
		poisonDuration, poisonTotal := "", ""
		if x, ok := state(0); ok {
			if poison, found := s.statuses.ResolveSkillApplication(domain.StatusApplication{
				ID: domain.StatusPoison, Level: x.PoisonOnHitLevel,
				ProgressionRanks: x.PoisonOnHitProgressionRanks, ProgressionFirstLevel: x.PoisonOnHitFirstLevel,
				ProgressionFirstDurationSec: x.PoisonOnHitFirstDurationSec, ProgressionBaseDurationSec: x.PoisonOnHitBaseDurationSec,
			}); found {
				poisonDuration = skillNumber(poison.DurationSec)
				poisonTotal = skillDescriptionPeriodicTotal(poison)
			}
		}
		return []string{duration(0), statusField(0, func(x domain.StatusDef) string { return skillNumber(x.PoisonOnHitChance) }), poisonDuration, poisonTotal}, true
	case 10213:
		return []string{effectValue(domain.EffectPlaceTrap, func(x domain.SkillEffect) string { return skillNumber(x.TrapRadius) }), damage}, true
	case 10214:
		return []string{effectValue(domain.EffectPlaceTrap, func(x domain.SkillEffect) string { return skillNumber(x.TrapRadius) }), damage, duration(0), statusField(0, skillDescriptionPeriodicTotal)}, true
	case 10215, 10216:
		return []string{effectValue(domain.EffectPlaceTrap, func(x domain.SkillEffect) string { return skillNumber(x.TrapRadius) }), damage, duration(0)}, true
	case 10217:
		return []string{effectValue(domain.EffectPlaceTrap, func(x domain.SkillEffect) string { return skillNumber(x.TrapRadius) }), damage, effectValue(domain.EffectDamageMPMax, func(x domain.SkillEffect) string { return skillNumber(x.MaxResourcePct) })}, true
	case 10218:
		return []string{chance(domain.EffectDisarmTrap)}, true
	case 10220:
		return []string{passive(domain.PassivePreserveInvisibleChance)}, true
	case 10224:
		return []string{affix(0, domain.AttrHit)}, true
	case 10228:
		return []string{chance(domain.EffectDetectTrap)}, true
	case 10229:
		return []string{passive(domain.PassiveTrapHalfDamageChance)}, true
	case 10230:
		return []string{passive(domain.PassiveIncomingMagicHalfChance)}, true
	case 10301:
		return []string{effectValue(domain.EffectHeal, func(x domain.SkillEffect) string {
			return skillFractionText(x.HealPctOfMax, x.HealPctNumerator, x.HealPctDenominator)
		}), effectValue(domain.EffectHeal, func(x domain.SkillEffect) string {
			return skillFractionText(x.HealFlat, x.HealFlatNumerator, x.HealFlatDenominator)
		})}, true
	case 10302:
		return []string{effectValue(domain.EffectRestoreMP, func(x domain.SkillEffect) string { return skillNumber(x.RestoreMPFlat) })}, true
	case 10303:
		return []string{duration(0), statusField(0, func(x domain.StatusDef) string {
			if len(x.HPChanges) > 0 {
				return skillMagnitude(x.HPChanges[0].Value)
			}
			return ""
		})}, true
	case 10304:
		return []string{radius, effectValue(domain.EffectHeal, func(x domain.SkillEffect) string {
			return skillFractionText(x.HealPctOfMax, x.HealPctNumerator, x.HealPctDenominator)
		}), effectValue(domain.EffectHeal, func(x domain.SkillEffect) string {
			return skillFractionText(x.HealFlat, x.HealFlatNumerator, x.HealFlatDenominator)
		})}, true
	case 10305:
		return []string{effectValue(domain.EffectResurrect, func(x domain.SkillEffect) string { return skillNumber(x.HealPctOfMax) }), effectValue(domain.EffectResurrect, func(x domain.SkillEffect) string { return skillNumber(x.LossRefundPct) })}, true
	case 10307:
		return []string{radius, statusField(0, func(x domain.StatusDef) string { return skillFractionText(0, int64(x.DurationSec), 60) }), affix(0, domain.AttrAtk), affix(0, domain.AttrMAtk)}, true
	case 10308:
		return []string{radius, statusField(0, func(x domain.StatusDef) string { return skillNumber(x.DamageShield) })}, true
	case 10309:
		return []string{effectValue(domain.EffectHeal, func(x domain.SkillEffect) string {
			return skillFractionText(x.HealPctOfMax, x.HealPctNumerator, x.HealPctDenominator)
		}), effectValue(domain.EffectResurrect, func(x domain.SkillEffect) string { return skillNumber(x.LossRefundPct) })}, true
	case 10310:
		return []string{duration(0), affix(0, domain.AttrMoveSpeed)}, true
	case 10311:
		return []string{radius, duration(0), affix(0, domain.AttrMoveSpeed)}, true
	case 10314:
		return []string{passive(domain.PassiveCastInterruptReductionPct), "", passive(domain.PassiveCastTimeReductionPct)}, true
	case 10315:
		return []string{radius, affix(0, domain.AttrAtkSpeed)}, true
	case 10320:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.DamageShield) })}, true
	case 10321:
		return []string{radius, statusField(0, func(x domain.StatusDef) string { return skillFractionText(0, int64(x.DurationSec), 60) }), statusField(0, func(x domain.StatusDef) string { return skillNumber(x.PhysicalDamageReductionPct) })}, true
	case 10322, 10418:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.NextStatusDurationPct) })}, true
	case 10324:
		return []string{duration(0), affix(0, domain.AttrAtk)}, true
	case 10325:
		return []string{duration(0), affix(0, domain.AttrAtkSpeed)}, true
	case 10326:
		return []string{radius, damage, duration(0)}, true
	case 10327:
		return []string{effectValue(domain.EffectRevealInvisible, func(x domain.SkillEffect) string { return skillNumber(x.RevealRadius) })}, true
	case 10328, 10419:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.NextSkillDamagePct) })}, true
	case 10329, 10420:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.NextCastTimeReductionPct) })}, true
	case 10330:
		return []string{duration(0), statusField(0, func(x domain.StatusDef) string { return skillNumber(x.PhysicalReflectPct) })}, true
	case 10410:
		return []string{radius, damage, statusChance(0), duration(0)}, true
	case 10411:
		return []string{damage, statusChance(0), duration(0)}, true
	case 10415:
		return []string{passive(domain.PassiveCastTimeReductionPct)}, true
	case 10416:
		return []string{passive(domain.PassiveCastInterruptReductionPct), ""}, true
	case 10417:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.MagicFullReflectChancePct) })}, true
	case 10421:
		return []string{duration(0), statusField(0, skillDescriptionPeriodicTotal)}, true
	case 10422:
		if len(d.PassiveStatusAffixes) > 0 {
			return []string{skillMagnitude(d.PassiveStatusAffixes[0].Affix.Value)}, true
		}
		return []string{""}, true
	case 10423:
		level := ""
		if len(d.PassiveTriggers) > 0 {
			level = skillNumber(d.PassiveTriggers[0].StatusLevel)
		}
		return []string{triggerChance(0), level}, true
	case 10425:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.PhysicalDamageReductionPct) }), statusField(0, func(x domain.StatusDef) string { return skillNumber(x.PhysicalReflectPct) })}, true
	case 10427:
		return []string{triggerChance(0), triggerField(0, func(x domain.StatusDef) string { return skillDescriptionAffix(x, domain.AttrMoveSpeed) })}, true
	case 10430:
		return []string{triggerChance(0), triggerDuration(0)}, true
	case 10432:
		return []string{chance(domain.EffectTeleportPoint)}, true
	case 10435:
		return []string{statusField(0, func(x domain.StatusDef) string { return skillNumber(x.PhysicalDamageReductionPct) }), statusField(0, func(x domain.StatusDef) string { return skillNumber(x.FreezeAttackerChance) })}, true
	}
	return nil, false
}

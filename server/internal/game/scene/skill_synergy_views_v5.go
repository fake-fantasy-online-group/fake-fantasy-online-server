package scene

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// skillSynergyViews reads the same immutable raw/hard snapshots as the cast path.
// Zero-invested training sources remain visible so the player can plan a build.
func (s *Scene) skillSynergyViews(target domain.SkillID, hard, raw domain.Learned) ([]domain.SkillTrainingSynergyView, []string) {
	training := []domain.SkillTrainingSynergyView{}
	native := []string{}
	seenNative := map[domain.SkillID]bool{}
	for _, edge := range domain.V5SkillSynergies {
		if edge.Target != target {
			continue
		}
		source, _ := domain.V5SkillIdentity(edge.Source)
		if edge.RankMode == "hard" {
			label := map[string]string{
				"damage": "伤害倍率", "elite_damage": "对精英伤害倍率",
				"heal_power": "治疗效果", "heal_flat": "固定治疗",
				"mana_economy": "法力消耗减免", "radius": "作用半径",
				"width": "作用宽度", "freeze_duration": "冻结持续时间",
			}[edge.Effect]
			value, unit := edge.PerPointBP, "percent_bp"
			if edge.Effect == "heal_flat" {
				value, unit = value/100, "flat"
			}
			h := hard.LevelOf(edge.Source)
			training = append(training, domain.SkillTrainingSynergyView{
				SourceID: edge.Source, SourceName: source.Name,
				Effect: edge.Effect, EffectLabel: label, PerPointValue: value,
				Unit: unit, InvestedHardPoints: h, ContributionValue: value * h,
				EquipmentApplies: false,
			})
			continue
		}
		if seenNative[edge.Source] {
			continue
		}
		seenNative[edge.Source] = true
		rank := raw.LevelOf(edge.Source)
		if rank <= 0 {
			// The planning section lists hard-point sources; legacy bonuses appear
			// here only when learned, matching the original compact tooltip.
			continue
		}
		def, ok := s.skills.Get(edge.Source, rank)
		if !ok {
			continue
		}
		filtered := domain.SkillDef{Kind: domain.SkillPassive}
		for _, m := range def.PassiveModifiers {
			if m.TargetSkill == target {
				filtered.PassiveModifiers = append(filtered.PassiveModifiers, m)
			}
		}
		for _, m := range def.PassiveStatusAffixes {
			if m.TargetSkill == target {
				filtered.PassiveStatusAffixes = append(filtered.PassiveStatusAffixes, m)
			}
		}
		for _, m := range def.PassiveTriggers {
			if m.TargetSkill == target {
				if m.DurationSec <= 0 {
					if state, found := s.statuses.Get(m.Status, m.StatusLevel); found {
						m.DurationSec = state.DurationSec
					}
				}
				filtered.PassiveTriggers = append(filtered.PassiveTriggers, m)
			}
		}
		for _, m := range def.PassiveTriggerEffects {
			if m.TargetSkill == target {
				filtered.Effects = append(filtered.Effects, m.Effect)
			}
		}
		lines := s.skillEffectLines(filtered)
		for i := range lines {
			lines[i] = strings.TrimPrefix(lines[i], skillTargetName(target)+"：")
			lines[i] = strings.TrimPrefix(lines[i], skillTargetName(target))
		}
		if len(lines) > 0 {
			native = append(native, fmt.Sprintf("%s（%d级）：%s", source.Name, rank, strings.Join(lines, "；")))
		}
	}
	return training, native
}

// Group equal effects by their exact target membership, then group effects that
// share a target set. Long weapon mastery target lists appear only once per level.
// No target is inferred from profession or weapon, and none is discarded.
func skillPassiveModifierLines(modifiers []domain.PassiveModifier) []string {
	type effectGroup struct {
		modifier domain.PassiveModifier
		targets  []domain.SkillID
	}
	var effects []effectGroup
	positions := map[domain.PassiveModifier]int{}
	for _, m := range modifiers {
		key := m
		key.TargetSkill = 0
		index, ok := positions[key]
		if !ok {
			index = len(effects)
			positions[key] = index
			effects = append(effects, effectGroup{modifier: m})
		}
		found := false
		for _, id := range effects[index].targets {
			if id == m.TargetSkill {
				found = true
				break
			}
		}
		if !found {
			effects[index].targets = append(effects[index].targets, m.TargetSkill)
		}
	}
	type targetGroup struct {
		targets []domain.SkillID
		lines   []string
	}
	groups := []targetGroup{}
	byTargets := map[string]int{}
	for _, effect := range effects {
		sort.Slice(effect.targets, func(i, j int) bool { return effect.targets[i] < effect.targets[j] })
		key := fmt.Sprint(effect.targets)
		index, ok := byTargets[key]
		if !ok {
			index = len(groups)
			byTargets[key] = index
			groups = append(groups, targetGroup{targets: effect.targets})
		}
		groups[index].lines = append(groups[index].lines, skillPassiveModifierText(effect.modifier))
	}
	out := []string{}
	for _, group := range groups {
		names := []string{}
		for _, id := range group.targets {
			names = append(names, skillTargetName(id))
		}
		if len(names) == 1 {
			out = append(out, names[0]+"："+strings.Join(group.lines, "；"))
		} else {
			out = append(out, "适用技能："+strings.Join(names, "、"), strings.Join(group.lines, "；"))
		}
	}
	return out
}

// skillEffectLineGroups retains semantic context when making a compact next-level
// preview. A status name/duration stays with its affixes; a precise passive target
// set stays with its values. Full effect lines remain available to API consumers.
func (s *Scene) skillEffectLineGroups(def domain.SkillDef, lines []string) [][]string {
	statusBlocks := [][]string{}
	for _, app := range def.Statuses {
		block := s.skillEffectLines(domain.SkillDef{Kind: domain.SkillPassive, Statuses: []domain.StatusApplication{app}})
		if len(block) > 0 {
			statusBlocks = append(statusBlocks, block)
		}
	}
	groups := [][]string{}
	for i := 0; i < len(lines); {
		length := 1
		for _, block := range statusBlocks {
			if len(block) > len(lines)-i {
				continue
			}
			matches := true
			for j, line := range block {
				if lines[i+j] != line {
					matches = false
					break
				}
			}
			if matches {
				length = len(block)
				break
			}
		}
		if strings.HasPrefix(lines[i], "适用技能：") && i+1 < len(lines) {
			length = 2
		}
		groups = append(groups, append([]string(nil), lines[i:i+length]...))
		i += length
	}
	return groups
}

func (s *Scene) skillNextChangeLines(currentDef, nextDef domain.SkillDef, current, next []string) []string {
	if len(current) == 0 {
		return append([]string(nil), next...)
	}
	old := map[string]int{}
	for _, group := range s.skillEffectLineGroups(currentDef, current) {
		old[strings.Join(group, "\x00")]++
	}
	out := []string{}
	unchanged := false
	for _, group := range s.skillEffectLineGroups(nextDef, next) {
		key := strings.Join(group, "\x00")
		if old[key] > 0 {
			old[key]--
			unchanged = true
			continue
		}
		out = append(out, group...)
	}
	if unchanged {
		if len(out) == 0 {
			out = append(out, "效果不变")
		} else {
			out = append(out, "其余效果不变")
		}
	}
	return out
}

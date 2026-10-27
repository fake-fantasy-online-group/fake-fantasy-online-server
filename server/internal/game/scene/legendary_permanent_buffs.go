package scene

import (
	"fmt"
	"reflect"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// These powers replace a learned preparation, not the skill investment itself.
var legendaryPermanentSkills = map[string]domain.SkillID{
	"SS-U03": 10418, "SS-D10": 10419, "SS-A30": 10420, "SS-U08": 10417,
	"YS-U02": 10322, "YS-A29": 10328, "YS-A30": 10329, "CK-A10": 10211,
}

func (s *Scene) syncLegendaryPermanentBuffs(owner *entity.Entity) {
	if owner == nil || owner.Player == nil || owner.Player.Char == nil || s.skills == nil || s.statuses == nil {
		return
	}
	rt := s.legendaryState()
	if rt.permanentSyncing {
		return
	}
	rt.permanentSyncing = true
	defer func() { rt.permanentSyncing = false }()
	wanted := map[string]domain.ActiveLegendaryPower{}
	if owner.Alive() {
		for _, power := range domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char) {
			skill := legendaryPermanentSkills[power.Power.ID]
			if skill != 0 && owner.Player.Char.Skills[skill] > 0 && legendaryHandlerCurrent(power.Power.ID) {
				wanted[power.Power.ID] = power
			}
		}
	}
	if owner.Status != nil {
		removed := owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
			if d.LegendaryPermanentPower == "" {
				return false
			}
			_, ok := wanted[d.LegendaryPermanentPower]
			return !ok
		})
		if len(removed) > 0 {
			s.statusesRemoved(owner, removed)
		}
	}
	if len(wanted) == 0 {
		return
	}
	raw := s.rawSkillLevels(owner)
	for _, power := range domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char) {
		if _, ok := wanted[power.Power.ID]; !ok {
			continue
		}
		skill := legendaryPermanentSkills[power.Power.ID]
		def, ok := s.skills.Get(skill, raw[skill])
		if !ok {
			continue
		}
		s.applyResolvedSkillModifiers(owner, &def, owner.Player.Char.Skills, raw)
		def.SnapshotRawRanks = raw.Clone()
		def.SnapshotHardRanks = owner.Player.Char.Skills.Clone()
		// Reconstruct the learned native preparation, not a simulated cast:
		// feeding an already-active concentration back through PrepareSkill
		// would amplify its own next-status multiplier on every replacement.
		for _, source := range domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char) {
			if legendaryHandlerCurrent(source.Power.ID) {
				def.LegendarySources = append(def.LegendarySources, domain.LegendarySourceRef{PowerID: source.Power.ID, SourceUID: source.SourceUID, PowerRollBP: source.PowerRollBP})
			}
		}
		for _, app := range def.Statuses {
			status, ok := s.resolveSkillStatusDef(def, app, 0)
			if !ok || status.Harmful() {
				continue
			}
			exists := false
			if owner.Status != nil {
				owner.Status.Each(func(a domain.Active) {
					// A separate named weapon can change this preparation without
					// changing its learned rank or the legendary power's source UID.
					if a.Def.ID == status.ID && a.Source == owner.ID &&
						a.Def.LegendaryPermanentPower == power.Power.ID && a.Def.LegendarySourceUID == power.SourceUID &&
						a.Def.SpecialWeaponSourceBase == status.SpecialWeaponSourceBase &&
						a.Def.SpecialWeaponBuffBonusPct == status.SpecialWeaponBuffBonusPct &&
						a.Def.SpecialWeaponStatusPowerPct == status.SpecialWeaponStatusPowerPct &&
						a.Def.SkillSnapshotRank == def.Level && reflect.DeepEqual(a.Def.SkillSnapshotRaw, def.SnapshotRawRanks) &&
						reflect.DeepEqual(a.Def.SkillSnapshotHard, def.SnapshotHardRanks) && reflect.DeepEqual(a.Def.LegendarySources, def.LegendarySources) {
						exists = true
					}
				})
			}
			if exists {
				continue
			}
			// A rank/source change replaces only this equipment-owned copy. Manual
			// states of the same native ID still obey the native overlay policy.
			if owner.Status != nil {
				owner.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == status.ID && d.LegendaryPermanentPower == power.Power.ID })
			}
			status.LegendaryPermanentPower = power.Power.ID
			status.LegendarySourceUID = power.SourceUID
			status.Permanent = true
			switch status.ID {
			case 1026:
				status.Desc = fmt.Sprintf("装备期间持续生效，使符合条件的技能状态持续时间提高%d%%，不随施法消耗。", status.NextStatusDurationPct)
			case 1027:
				status.Desc = fmt.Sprintf("装备期间持续生效，使符合条件的伤害技能伤害提高%d%%，不随施法消耗。", status.NextSkillDamagePct)
			case 1028:
				status.Desc = fmt.Sprintf("装备期间持续生效，使符合条件的技能吟唱时间缩短%d%%，不随施法消耗。", status.NextCastTimeReductionPct)
			}
			s.applyStatusDef(owner, status, owner.ID)
		}
	}
}

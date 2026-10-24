package domain

import "reflect"

// StripMovementImpairments removes movement penalties, not the whole status.
// BlockMove is the verified root mechanic; ControlStun also represents freeze
// and petrify and must never be interpreted as a removable root. StatusSlow is
// not sufficient evidence: its 11..15 levels carry an attack-speed penalty.
// The returned empty flag is meaningful only when changed is true.
func (d *StatusDef) StripMovementImpairments() (changed, empty bool) {
	if d == nil {
		return false, false
	}
	hardControl := d.Control == ControlStun || d.ID == StatusStun || d.ID == StatusFreeze || d.ID == StatusPetrify
	if d.BlockMove && !hardControl {
		d.BlockMove, d.Immobile = false, 0
		changed = true
	}
	for _, a := range d.Affixes {
		if a.Attr == AttrMoveSpeed && a.Value < 0 {
			changed = true
			break
		}
	}
	if !changed {
		return false, false
	}
	// Never mutate the catalog, an already captured receipt, or another layer.
	affixes := make([]Affix, 0, len(d.Affixes))
	for _, a := range d.Affixes {
		if a.Attr != AttrMoveSpeed || a.Value >= 0 {
			affixes = append(affixes, a)
		}
	}
	d.Affixes = affixes
	return true, movementResidualEmpty(*d)
}

// Remove only proven empty movement states. Ignore a whitelist of provenance,
// presentation and application metadata; any other nonzero field, including a
// future StatusDef payload, conservatively retains the instance. This avoids
// treating a slow+stun/poison/shield/vulnerability package as entirely immune.
func movementResidualEmpty(d StatusDef) bool {
	d.ID, d.Level, d.Name, d.Desc = 0, 0, "", ""
	d.Type, d.TypeKnown = 0, false
	d.Icon, d.IconKnown, d.SourceSkillID, d.Hidden = 0, false, 0, false
	d.Permanent, d.DurationSec, d.LegendaryDurationMS = false, 0, 0
	d.ControlPower, d.EffectHitBP, d.ChanceBP, d.ChanceExplicit = 0, 0, 0, false
	d.SourceEffectHitBP, d.SourceEffectHitKnown = 0, false
	d.OverlayStrength, d.LegacyOverlayLevelBP = nil, 0
	d.SkillApplied, d.SkillSnapshotID, d.SkillSnapshotRank = false, 0, 0
	d.SkillSnapshotRaw, d.SkillSnapshotHard = nil, nil
	d.DamageSourceRange, d.DamageSourceTrap, d.DungeonDamageScaleBP = 0, false, 0
	d.LegendarySources, d.LegendaryPowerID, d.LegendarySourceUID = nil, "", 0
	d.LegendaryVisualStage = ""
	// Scaling an absent payload cannot create an effect; the explicit flat
	// grants can, until finalization has already consumed them into the fields.
	m := d.LegendaryModifiers
	d.LegendaryModifiers = LegendaryStatusModifiers{}
	if !m.Applied {
		d.LegendaryModifiers.MoveSpeedBonusPct = m.MoveSpeedBonusPct
		d.LegendaryModifiers.PhysicalReductionBonusPct = m.PhysicalReductionBonusPct
		d.LegendaryModifiers.MagicReductionBonusPct = m.MagicReductionBonusPct
	}
	if len(d.Affixes) == 0 {
		d.Affixes = nil
	}
	if len(d.HPChanges) == 0 {
		d.HPChanges = nil
	}
	if len(d.MPChanges) == 0 {
		d.MPChanges = nil
	}
	return reflect.ValueOf(d).IsZero()
}

// StripMovementImpairments cleans every stored layer, including suppressed
// ones, without restarting expiry/ticks or replenishing shields and balances.
func (s *StatusSet) StripMovementImpairments() (changed bool, removed []StatusID) {
	if s == nil {
		return false, nil
	}
	for _, id := range s.sortedIDs() {
		list := s.act[id]
		kept := list[:0]
		for _, a := range list {
			modified, empty := a.Def.StripMovementImpairments()
			changed = changed || modified
			if !modified || !empty {
				kept = append(kept, a)
			}
		}
		if len(kept) == 0 {
			delete(s.act, id)
			removed = append(removed, id)
		} else {
			s.act[id] = kept
		}
	}
	return changed, removed
}

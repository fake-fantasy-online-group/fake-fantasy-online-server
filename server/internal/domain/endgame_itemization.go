package domain

// ReservedSkillHardPointCap is the future skill-tree spending ceiling. It is
// deliberately not wired to Learned, skill costs, prerequisites or SkillTable:
// legacy skills keep their existing levels until that separate system is built.
const ReservedSkillHardPointCap int32 = 20

// ReservedSkillAttribute describes future equipment soft levels. These are NOT
// Affixes, do not grant/learn skills, do not satisfy hard-point requirements and
// are never rendered among active item bonuses. Effective soft levels may later
// exceed 20; hard points alone will count for prerequisites and synergies.
type ReservedSkillAttribute struct {
	Scope  string // all, class, tree, skill
	Target string // empty for all, stable future definition key otherwise
	Levels int32
}

func (b ReservedSkillAttribute) Valid() bool {
	if b.Levels < 1 || b.Levels > 3 {
		return false
	}
	switch b.Scope {
	case "all":
		return b.Target == ""
	case "class", "tree", "skill":
		return b.Target != ""
	}
	return false
}

// SupportedEndgameAffix is an audited allowlist for NEW authored active effects.
// Equipment HPRegen currently has no combat consumer; crit-percent modes are
// discarded by applyAffix. MaxWeight has no server capacity/encumbrance gate.
// These may not be advertised as new active catalog benefits.
// Old instance snapshots are not rewritten by this content validation.
func SupportedEndgameAffix(a Affix) bool {
	if a.Value == 0 || (a.Mode != ModeAbsolute && a.Mode != ModePercent) {
		return false
	}
	switch a.Attr {
	case AttrCrit, AttrMCrit:
		return a.Mode == ModeAbsolute
	case AttrAtkSpeed:
		// Player basic attacks consume AtkSpeedPct, not the monster/pet ms field.
		return a.Mode == ModePercent
	case AttrSTR, AttrINT, AttrVIT, AttrAGI, AttrDEX, AttrSPI,
		AttrDef, AttrMAtk, AttrMDef, AttrHit, AttrMoveSpeed,
		AttrMinAtk, AttrMaxAtk, AttrMaxHP, AttrMaxMP,
		AttrAtk, AttrPhysRes, AttrMagicRes, AttrStatusRes:
		return true
	}
	return false
}

// SupportedAstralRuneAffix permits the one snapshot-backed regeneration consumer.
// It does not enable ordinary equipment regeneration or another rune's Attr38.
func SupportedAstralRuneAffix(item ItemID, a Affix) bool {
	return SupportedEndgameAffix(a) || (item == LastAstralRune && a.Attr == AttrHPRegen && a.Mode == ModeAbsolute && a.Value > 0)
}

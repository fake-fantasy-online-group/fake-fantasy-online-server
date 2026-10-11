package domain

import "fmt"

// This is an extra-attribute budget, independent of native base stats, rune
// snapshots, saved yellow affixes, conditional traits and future skill levels.
const MinBeneficialEndgameExtraAffixes = 5

func validEndgamePowerTier(tier string) bool {
	switch tier {
	case "transition", "highvalue", "builddefining", "godlike":
		return true
	}
	return false
}

// BeneficialEndgameAffix includes only implemented, nonzero bonuses. Attack
// interval has the opposite sign: a negative Attr25 increases player speed.
func BeneficialEndgameAffix(a Affix) bool {
	if !SupportedEndgameAffix(a) {
		return false
	}
	if a.Attr == AttrAtkSpeed {
		return a.Value < 0
	}
	return a.Value > 0
}

// validateEndgameExtraAffixes intentionally has no old six-line ceiling. The
// supported, distinct attribute IDs bound the slice. Curses are permitted as
// extra tradeoffs, but cannot pad the five-benefit minimum.
func validateEndgameExtraAffixes(affixes []Affix, unique bool) error {
	seen := make(map[int32]bool, len(affixes))
	beneficial := 0
	percentAttack := false
	for _, a := range affixes {
		if !SupportedEndgameAffix(a) {
			return fmt.Errorf("unsupported extra affix %+v", a)
		}
		if seen[a.Attr] {
			return fmt.Errorf("duplicate extra Attr %d", a.Attr)
		}
		seen[a.Attr] = true
		if a.Mode == ModePercent && (a.Attr == AttrMinAtk || a.Attr == AttrMaxAtk || a.Attr == AttrAtk) {
			if percentAttack {
				return fmt.Errorf("duplicate percentage attack effect")
			}
			percentAttack = true
		}
		if unique {
			value := int64(a.Value)
			if value < 0 {
				value = -value
			}
			// Both benefit and inverse-curse multipliers span 80–120%. Check
			// using int64 before scaleUniqueAffix can truncate or overflow.
			if value*int64(UniqueRollMinBP)/10000 == 0 {
				return fmt.Errorf("extra affix vanishes at minimum potency: %+v", a)
			}
			if value*int64(UniqueRollMaxBP)/10000 > 2147483647 {
				return fmt.Errorf("extra affix overflows at maximum potency: %+v", a)
			}
		}
		if BeneficialEndgameAffix(a) {
			beneficial++
		}
	}
	if beneficial < MinBeneficialEndgameExtraAffixes {
		return fmt.Errorf("at least five beneficial distinct extra attributes required, got %d", beneficial)
	}
	return nil
}

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
// Resist multipliers need another source's flat resistance and can be zero on
// a fresh base, so new authored resistance lines must provide absolute values.
// These may not be advertised as new active catalog benefits.
// Old instance snapshots are not rewritten by this content validation.
func SupportedEndgameAffix(a Affix) bool {
	if a.Value == 0 || (a.Mode != ModeAbsolute && a.Mode != ModePercent) {
		return false
	}
	switch a.Attr {
	case AttrCrit, AttrMCrit, AttrPhysRes, AttrMagicRes, AttrStatusRes:
		return a.Mode == ModeAbsolute
	case AttrAtkSpeed:
		// Player basic attacks consume AtkSpeedPct, not the monster/pet ms field.
		return a.Mode == ModePercent
	case AttrSTR, AttrINT, AttrVIT, AttrAGI, AttrDEX, AttrSPI,
		AttrDef, AttrMAtk, AttrMDef, AttrHit, AttrMoveSpeed,
		AttrMinAtk, AttrMaxAtk, AttrMaxHP, AttrMaxMP,
		AttrAtk:
		return true
	}
	return false
}

// SupportedAstralRuneAffix permits the one snapshot-backed regeneration consumer.
// It does not enable ordinary equipment regeneration or another rune's Attr38.
func SupportedAstralRuneAffix(item ItemID, a Affix) bool {
	return SupportedEndgameAffix(a) || (item == LastAstralRune && a.Attr == AttrHPRegen && a.Mode == ModeAbsolute && a.Value > 0)
}

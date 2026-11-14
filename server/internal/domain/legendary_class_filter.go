package domain

import "strings"

// LegendaryPowerVisibleToClass is the power-book, tooltip, and drop-pool
// profession filter. Comma-separated official class names are visible to each
// listed profession. Unemployed or unknown characters see only 全职业.
// Former typified universal powers now follow their current Class for both
// new generation and already-owned items, including extracted wing sockets.
func LegendaryPowerVisibleToClass(powerClass, characterClass string) bool {
	if powerClass == "全职业" {
		return true
	}
	if characterClass == "" {
		return false
	}
	powerMask := legendaryClassMask(powerClass)
	charMask := legendaryClassMask(characterClass)
	return powerMask != 0 && charMask != 0 && powerMask&charMask != 0
}

func parseLegendaryClassParts(class string) uint8 {
	var mask uint8
	for _, part := range strings.Split(class, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "全职业" {
			return 0
		}
		var bit uint8
		switch part {
		case "战士":
			bit = 1
		case "剑客":
			bit = 2
		case "刺客":
			bit = 4
		case "药师":
			bit = 8
		case "术士":
			bit = 16
		default:
			return 0
		}
		if mask&bit != 0 {
			return 0
		}
		mask |= bit
	}
	return mask
}

// allowsOnItem keeps a profession-restricted base from receiving a class power
// the current recipient cannot wear. Multi-class powers still require the
// recipient to be one of the listed professions.
func (recipient LegendaryRecipientClass) allowsOnItem(power LegendaryPowerDefinition, def ItemDef) bool {
	if !recipient.allows(power) {
		return false
	}
	if power.Class == "全职业" {
		return true
	}
	e := def.Equip
	if e == nil {
		return false
	}
	if e.Need.ProfessionRestricted || e.Need.Professions != 0 {
		return uint8(recipient)&uint8(e.Need.Professions) != 0
	}
	return true
}

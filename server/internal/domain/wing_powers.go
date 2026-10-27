package domain

import (
	"fmt"
	"strings"
)

// WingPower preserves a consumed legendary's power, not its six rolled affixes
// or intrinsic tier multiplier. PowerRollBP preserves the donor’s immutable
// authored strength; SourceTier is provenance, never a potency multiplier.
type WingPower struct {
	PowerID     string
	PowerRollBP int32         `json:",omitempty"`
	SourceTier  LegendaryTier `json:",omitempty"`
	SourceLevel int32
}

// Value arrays keep snapshot/Bag.Clone isolation and stable A/D/U ordering.
type WingPowers [3]WingPower

func WingPowerIndex(p LegendaryPowerDefinition) (int, bool) {
	parts := strings.Split(p.ID, "-")
	if len(parts) != 2 || len(parts[1]) < 1 {
		return 0, false
	}
	switch parts[1][0] {
	case 'A':
		return 0, p.Category == "攻击"
	case 'D':
		return 1, p.Category == "防御"
	case 'U':
		return 2, p.Category == "通用"
	}
	return 0, false
}
func (w WingPowers) Validate() error {
	for i, v := range w {
		if v.PowerID == "" {
			if v != (WingPower{}) {
				return fmt.Errorf("wing power provenance without power")
			}
			continue
		}
		p, ok := LegendaryPower(v.PowerID)
		index, category := WingPowerIndex(p)
		if !ok || !LegendaryPowerRollValid(v.PowerID, v.PowerRollBP) || !category || index != i || !v.SourceTier.Valid() || !LegendaryPowerSourceLevelEligible(p, v.SourceLevel) {
			return fmt.Errorf("invalid wing power slot %d", i)
		}
	}
	return nil
}

// The caller verifies that target is an authored progression wing and consumes
// the exact selected source in the same transaction as the returned wing write.
func ExtractWingPower(target, source Stack, sourceDef ItemDef) (Stack, error) {
	if target.UID <= 0 || target.Count != 1 || target.Locked || target.Legendary.PowerID != "" || target.Endgame.UniqueID != 0 || target.InstanceKind != ItemInstanceEquipment || target.UID == source.UID {
		return target, fmt.Errorf("羽翼不可承载威能")
	}
	if err := target.WingPowers.Validate(); err != nil {
		return target, err
	}
	if source.UID <= 0 || source.Count != 1 || source.Locked || source.InstanceKind != ItemInstanceEquipment || source.Item != sourceDef.ID || !sourceDef.LegendaryEnabled || source.Endgame.UniqueID != 0 || source.RolledAffixCount != 0 || source.SocketCount != 0 || source.WingPowers != (WingPowers{}) || source.Legendary.Validate() != nil {
		return target, fmt.Errorf("须投入一件完好的暗金器物")
	}
	p, ok := LegendaryPower(source.Legendary.PowerID)
	index, category := WingPowerIndex(p)
	if !ok || !category || !LegendaryPowerEligibleForItem(p, sourceDef, source) || !LegendaryPowerExecutable(p.ID) {
		return target, fmt.Errorf("此暗金器物的威能无法提取")
	}
	target.WingPowers[index] = WingPower{PowerID: p.ID, PowerRollBP: source.Legendary.PowerRollBP, SourceTier: source.Legendary.Tier, SourceLevel: LegendaryGenerationLevel(sourceDef, source)}
	return target, nil
}

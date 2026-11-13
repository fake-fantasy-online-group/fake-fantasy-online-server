package domain

// These presentation IDs are shared across all producers. Runtime status IDs,
// source ownership and equipment UIDs remain untouched and independently keyed.
const (
	DOTBleed             = "bleed"
	DOTPoison            = "poison"
	DOTBurn              = "burn"
	DOTRadiant           = "radiant"
	DOTBleedIcon   int32 = 190001
	DOTPoisonIcon  int32 = 190002
	DOTBurnIcon    int32 = 190003
	DOTRadiantIcon int32 = 190004
)

func GenericDOTIdentity(family string) (string, int32) {
	switch family {
	case DOTBleed:
		return "流血", DOTBleedIcon
	case DOTPoison:
		return "中毒", DOTPoisonIcon
	case DOTBurn:
		return "燃烧", DOTBurnIcon
	case DOTRadiant:
		return "灼光", DOTRadiantIcon
	}
	return "", 0
}

func GenericDOTFamily(d StatusDef) string {
	if !d.DamageOverTime() || d.IntervalSec <= 0 {
		return ""
	}
	if name, _ := GenericDOTIdentity(d.DOTFamily); name != "" {
		return d.DOTFamily
	}
	// Legacy saved equipment states migrate presentation only, not damage snapshots.
	if d.SynergyDOTNew && d.SynergyDOTVersion > 0 {
		switch d.LegendaryPowerID {
		case "ZS-A01", "JK-A01":
			return DOTBleed
		case "YS-A03":
			return DOTRadiant
		}
	}
	if d.ID == StatusPoison {
		return DOTPoison
	}
	if d.Name == "燃烧" {
		return DOTBurn
	}
	return ""
}

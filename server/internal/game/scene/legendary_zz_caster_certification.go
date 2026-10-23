package scene

// These IDs passed the per-ID caster PvE/native-event matrix and current
// client metadata contract tests. This is not Unity render or PvP certification.
func init() {
	for _, id := range []string{
		"SS-A01", "SS-A02", "SS-A03", "SS-A04", "SS-A05", "SS-A06", "SS-A07", "SS-A08", "SS-A09", "SS-A10",
		"SS-A11", "SS-A12", "SS-A13", "SS-A14", "SS-A15", "SS-A16", "SS-A17", "SS-A18", "SS-A19", "SS-A20",
		"SS-A21", "SS-A22", "SS-A23", "SS-A24", "SS-A25", "SS-A26", "SS-A27", "SS-A28", "SS-A29", "SS-A30",
		"SS-D01", "SS-D02", "SS-D03", "SS-D04", "SS-D05", "SS-D06", "SS-D07", "SS-D08", "SS-D09", "SS-D10",
		"SS-U01", "SS-U02", "SS-U03", "SS-U04", "SS-U05", "SS-U06", "SS-U07", "SS-U08", "SS-U09", "SS-U10",
		"YS-A01", "YS-A02", "YS-A03", "YS-A04", "YS-A05", "YS-A06", "YS-A07", "YS-A08", "YS-A09", "YS-A10",
		"YS-A11", "YS-A12", "YS-A13", "YS-A14", "YS-A15", "YS-A16", "YS-A17", "YS-A18", "YS-A19", "YS-A20",
		"YS-A21", "YS-A22", "YS-A23", "YS-A24", "YS-A25", "YS-A26", "YS-A27", "YS-A28", "YS-A29", "YS-A30",
		"YS-D01", "YS-D02", "YS-D03", "YS-D04", "YS-D05", "YS-D06", "YS-D07", "YS-D08", "YS-D09", "YS-D10",
		"YS-U01", "YS-U02", "YS-U03", "YS-U04", "YS-U05", "YS-U06", "YS-U07", "YS-U08", "YS-U09", "YS-U10",
	} {
		certifyLegendaryPower(id)
	}
}

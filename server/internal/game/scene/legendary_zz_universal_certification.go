package scene

// Explicit server/PvE mechanics certification after per-ID behavior tests.
// This is not visual or PvP acceptance. A35 remains withheld pending the
// client native-attack identity/effective-range contract tests.
func init() {
	for _, id := range []string{
		"TY-A01", "TY-A02", "TY-A03", "TY-A04", "TY-A05", "TY-A06", "TY-A07", "TY-A08", "TY-A09", "TY-A10",
		"TY-A11", "TY-A12", "TY-A13", "TY-A14", "TY-A15", "TY-A16", "TY-A17", "TY-A18", "TY-A19", "TY-A20",
		"TY-A21", "TY-A22", "TY-A23", "TY-A24", "TY-A25", "TY-A26", "TY-A27", "TY-A28", "TY-A29", "TY-A30",
		"TY-A31", "TY-A32", "TY-A33", "TY-A34", "TY-A36", "TY-A37", "TY-A38", "TY-A39", "TY-A40", "TY-D01",
		"TY-D02", "TY-D03", "TY-D04", "TY-D05", "TY-D06", "TY-D07", "TY-D08", "TY-D09", "TY-D10", "TY-D11",
		"TY-D12", "TY-D13", "TY-D14", "TY-D15", "TY-D16", "TY-D17", "TY-D18", "TY-D19", "TY-D20", "TY-U01",
		"TY-U02", "TY-U03", "TY-U04", "TY-U05", "TY-U06", "TY-U07", "TY-U08", "TY-U09", "TY-U10", "TY-U11",
		"TY-U12", "TY-U13", "TY-U14", "TY-U15", "TY-U16", "TY-U17", "TY-U18", "TY-U19", "TY-U20",
	} {
		certifyLegendaryPower(id)
	}
}

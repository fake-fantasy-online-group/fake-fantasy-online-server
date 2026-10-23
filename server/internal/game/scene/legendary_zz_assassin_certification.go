package scene

// CK50 certification follows the explicit per-ID mechanics/test inventory in
// docs/legendary/assassin-synergy-coverage.json. Registration alone is not proof.
// This init is sorted after the isolated handler files; pending identities are
// never inferred from design prose or automatically certified by registration.
func init() {
	for _, id := range []string{
		"CK-A01", "CK-A02", "CK-A03", "CK-A04", "CK-A05", "CK-A06", "CK-A07", "CK-A08", "CK-A09", "CK-A10",
		"CK-A11", "CK-A12", "CK-A13", "CK-A14", "CK-A15", "CK-A16", "CK-A17", "CK-A18", "CK-A19", "CK-A20",
		"CK-A21", "CK-A22", "CK-A23", "CK-A24", "CK-A25", "CK-A26", "CK-A27", "CK-A28", "CK-A29", "CK-A30",
		"CK-D01", "CK-D02", "CK-D03", "CK-D04", "CK-D05", "CK-D06", "CK-D07", "CK-D08", "CK-D09", "CK-D10",
		"CK-U01", "CK-U02", "CK-U03", "CK-U04", "CK-U05", "CK-U06", "CK-U07", "CK-U08", "CK-U09", "CK-U10",
	} {
		certifyLegendaryPower(id)
	}
}

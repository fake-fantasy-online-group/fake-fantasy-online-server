package scene

// PvE mechanics certified by docs/legendary/swordsman-runtime-coverage.json.
// This does not certify PvP pull coverage or Unity visual rendering.
func init() {
	for _, id := range []string{"JK-A01", "JK-A02", "JK-A03", "JK-A04", "JK-A05", "JK-A06", "JK-A07", "JK-A08", "JK-A09", "JK-A10", "JK-A11", "JK-A12", "JK-A13", "JK-A14", "JK-A15", "JK-A16", "JK-A17", "JK-A18", "JK-A19", "JK-A20", "JK-A21", "JK-A22", "JK-A23", "JK-A24", "JK-A25", "JK-A26", "JK-A27", "JK-A28", "JK-A29", "JK-A30", "JK-D01", "JK-D02", "JK-D03", "JK-D04", "JK-D05", "JK-D06", "JK-D07", "JK-D08", "JK-D09", "JK-D10", "JK-U01", "JK-U02", "JK-U03", "JK-U04", "JK-U05", "JK-U06", "JK-U07", "JK-U08", "JK-U09", "JK-U10"} {
		certifyLegendaryPower(id)
	}
}

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLegendaryPowerCurrentValueAll330AuthoredPrimaryParameters(t *testing.T) {
	specs := LegendaryPowerRollSpecs()
	if len(specs) != 330 {
		t.Fatal("catalog changed", len(specs))
	}
	for _, spec := range specs {
		for _, roll := range []int32{0, 7500, 8123, 10000} {
			got := LegendaryPowerCurrentValueText(spec.ID, roll)
			if len(spec.Parameters) == 0 {
				if got != "" {
					t.Fatal("functional value", spec.ID, got)
				}
				continue
			}
			p := spec.Parameters[0]
			effective := roll
			if effective == 0 {
				effective = 10000
			}
			value := int32(int64(p.Base) * int64(effective) / 10000)
			if value < 1 {
				value = 1
			}
			if low := max(int32(1), int32(int64(p.Base)*7500/10000)); value < low || value > p.Base {
				t.Fatalf("%s/%d current outside authored primary range", spec.ID, roll)
			}
			want := powerRollNumber(value, p.Divisor) + p.Unit
			if got != want {
				t.Fatalf("%s/%d got %s want %s", spec.ID, roll, got, want)
			}
			low := powerRollNumber(max(int32(1), int32(int64(p.Base)*7500/10000)), p.Divisor) + p.Unit
			high := powerRollNumber(p.Base, p.Divisor) + p.Unit
			// Value and range share authored order, independent of effect prose order.
			if pair := LegendaryPowerPrimaryRangeText(spec.ID); pair != low+"～"+high {
				t.Fatalf("%s primary range mismatch %q vs %s～%s", spec.ID, pair, low, high)
			}
		}
	}
	for _, tc := range []struct {
		id   string
		roll int32
		want string
	}{
		{"TY-A01", 8123, "56%"}, {"TY-A01", 7500, "52%"}, {"TY-A01", 0, "70%"},
		{"SS-A01", 8123, "56.86%"}, {"SS-A05", 8123, "60%"}, {"SS-A08", 8123, "44%"}, {"SS-A13", 8123, "69%"},
		{"SS-U06", 0, ""}, {"unknown", 8123, ""}, {"TY-A01", 1, ""},
	} {
		if got := LegendaryPowerCurrentValueText(tc.id, tc.roll); got != tc.want {
			t.Errorf("%+v got %q", tc, got)
		}
	}
}

func TestPowerBookCurrentValueOptionalJSON(t *testing.T) {
	for _, value := range []string{"", "56%"} {
		raw, err := json.Marshal(EndgamePowerBookView{CurrentValueText: value})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"currentValueText"`) != (value != "") {
			t.Fatal(string(raw))
		}
	}
}

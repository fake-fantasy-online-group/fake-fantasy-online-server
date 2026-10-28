package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const LegendaryPowerRollMinBP int32 = 7500
const LegendaryPowerRollMaxBP int32 = 10000

// One immutable draw drives all explicitly authored beneficial parameters of
// a power. Zero is legacy gear: its existing full-strength behavior is retained.
// Triggers, learning gates, immunity, free casts and structural transformations
// are not parameters and must never be weakened by this draw.
type LegendaryPowerRollParameter struct {
	Key     string `json:"key"`
	Text    string `json:"text"`
	Base    int32  `json:"base"`
	Divisor int32  `json:"divisor,omitempty"`
	Unit    string `json:"unit,omitempty"`
}
type LegendaryPowerRollSpec struct {
	ID             string                        `json:"id"`
	OmitWhenRolled []string                      `json:"omit_when_rolled,omitempty"`
	FixedReason    string                        `json:"fixed_reason,omitempty"`
	Parameters     []LegendaryPowerRollParameter `json:"parameters"`
}

//go:embed legendary_rolls/*.json
var legendaryRollFS embed.FS
var legendaryRollSpecs = loadLegendaryRollSpecs()

func loadLegendaryRollSpecs() map[string]LegendaryPowerRollSpec {
	files, err := legendaryRollFS.ReadDir("legendary_rolls")
	if err != nil {
		panic(err)
	}
	out := map[string]LegendaryPowerRollSpec{}
	for _, file := range files {
		raw, err := legendaryRollFS.ReadFile("legendary_rolls/" + file.Name())
		if err != nil {
			panic(err)
		}
		var specs []LegendaryPowerRollSpec
		if err := json.Unmarshal(raw, &specs); err != nil {
			panic(err)
		}
		for _, spec := range specs {
			power, ok := LegendaryPower(spec.ID)
			if !ok {
				panic("unknown power roll: " + spec.ID)
			}
			if _, exists := out[spec.ID]; exists {
				panic("duplicate power roll: " + spec.ID)
			}
			if len(spec.Parameters) == 0 && spec.FixedReason == "" {
				panic("missing fixed power reason: " + spec.ID)
			}
			for _, clause := range spec.OmitWhenRolled {
				if clause == "" || !strings.Contains(power.Effect, clause) {
					panic("invalid rolled omission: " + spec.ID)
				}
			}
			seen := map[string]bool{}
			for _, p := range spec.Parameters {
				if seen[p.Text] {
					panic("duplicate power roll description: " + spec.ID)
				}
				seen[p.Text] = true
				if p.Key == "" || p.Base <= 0 || p.Divisor < 0 || p.Text == "" || !strings.Contains(power.Effect, p.Text) || !strings.Contains(p.Text, powerRollNumber(p.Base, p.Divisor)+p.Unit) {
					panic(fmt.Sprintf("invalid power roll parameter %s: %+v", spec.ID, p))
				}
			}
			out[spec.ID] = spec
		}
	}
	return out
}

func LegendaryPowerHasRoll(id string) bool { return len(legendaryRollSpecs[id].Parameters) > 0 }
func LegendaryPowerRollValid(id string, roll int32) bool {
	if _, ok := LegendaryPower(id); !ok {
		return false
	}
	return roll == 0 || LegendaryPowerHasRoll(id) && roll >= LegendaryPowerRollMinBP && roll <= LegendaryPowerRollMaxBP
}
func EffectiveLegendaryPowerRollBP(id string, roll int32) int32 {
	if roll == 0 {
		return LegendaryPowerRollMaxBP
	}
	if !LegendaryPowerRollValid(id, roll) {
		return 0
	}
	return roll
}
func LegendaryPowerRollValue(id, key string, base, roll int32) int32 {
	if base <= 0 {
		return base
	}
	for _, p := range legendaryRollSpecs[id].Parameters {
		if p.Key == key && p.Base == base {
			return max(int32(1), int32(int64(base)*int64(EffectiveLegendaryPowerRollBP(id, roll))/10000))
		}
	}
	return base
}
func powerRollNumber(value, divisor int32) string {
	if divisor <= 1 {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(float64(value)/float64(divisor), 'f', -1, 64)
}

// Equipment descriptions show current values only. The codex exposes ranges
// separately through LegendaryPowerRangeText.
func LegendaryPowerForRoll(id string, roll int32) (LegendaryPowerDefinition, bool) {
	p, ok := LegendaryPower(id)
	if !ok || !LegendaryPowerRollValid(id, roll) {
		return LegendaryPowerDefinition{}, false
	}
	if roll == 0 {
		return p, true
	}
	for _, clause := range legendaryRollSpecs[id].OmitWhenRolled {
		p.Effect = strings.ReplaceAll(p.Effect, clause, "")
	}
	for _, parameter := range legendaryRollSpecs[id].Parameters {
		old := powerRollNumber(parameter.Base, parameter.Divisor) + parameter.Unit
		value := powerRollNumber(LegendaryPowerRollValue(id, parameter.Key, parameter.Base, roll), parameter.Divisor) + parameter.Unit
		replacement := strings.Replace(parameter.Text, old, value, 1)
		p.Effect = strings.ReplaceAll(p.Effect, parameter.Text, replacement)
	}
	return p, true
}

// LegendaryPowerCurrentValueText is the first authored beneficial parameter,
// not the roll-quality percentage or the first number in the effect prose.
// Owned legacy roll zero retains its existing full-strength interpretation.
// Callers must check ownership separately; absent records are not legacy gear.
func LegendaryPowerCurrentValueText(id string, roll int32) string {
	if !LegendaryPowerRollValid(id, roll) || !LegendaryPowerHasRoll(id) {
		return ""
	}
	parameter := legendaryRollSpecs[id].Parameters[0]
	value := LegendaryPowerRollValue(id, parameter.Key, parameter.Base, roll)
	return powerRollNumber(value, parameter.Divisor) + parameter.Unit
}

// LegendaryPowerPrimaryRangeText matches CurrentValueText's authored parameter
// even when effect prose mentions another parameter first (for example SS-A13).
func LegendaryPowerPrimaryRangeText(id string) string {
	if !LegendaryPowerHasRoll(id) {
		return ""
	}
	parameter := legendaryRollSpecs[id].Parameters[0]
	low := LegendaryPowerRollValue(id, parameter.Key, parameter.Base, LegendaryPowerRollMinBP)
	return powerRollNumber(low, parameter.Divisor) + parameter.Unit + "～" + powerRollNumber(parameter.Base, parameter.Divisor) + parameter.Unit
}

// LegendaryPowerRangeText renders authored parameter bounds with their effect
// context. It does not expose the internal roll-quality percentage as a stat.
func LegendaryPowerRangeText(id string) string {
	power, ok := LegendaryPower(id)
	if !ok || !LegendaryPowerHasRoll(id) {
		return ""
	}
	effect := power.Effect
	for _, parameter := range legendaryRollSpecs[id].Parameters {
		high := powerRollNumber(parameter.Base, parameter.Divisor) + parameter.Unit
		low := powerRollNumber(LegendaryPowerRollValue(id, parameter.Key, parameter.Base, LegendaryPowerRollMinBP), parameter.Divisor) + parameter.Unit
		replacement := strings.Replace(parameter.Text, high, low+"～"+high, 1)
		effect = strings.ReplaceAll(effect, parameter.Text, replacement)
	}
	var lines []string
	for _, clause := range strings.FieldsFunc(effect, func(r rune) bool {
		return r == '。' || r == '；' || r == '，'
	}) {
		if strings.Contains(clause, "～") {
			lines = append(lines, clause)
		}
	}
	return strings.Join(lines, "\n")
}

func LegendaryPowerRollSpecs() []LegendaryPowerRollSpec {
	out := make([]LegendaryPowerRollSpec, 0, len(legendaryRollSpecs))
	for _, spec := range legendaryRollSpecs {
		spec.Parameters = append([]LegendaryPowerRollParameter(nil), spec.Parameters...)
		spec.OmitWhenRolled = append([]string(nil), spec.OmitWhenRolled...)
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (v *LegendaryInstance) RollPower(rng func(int) int) bool {
	if v == nil {
		return false
	}
	if _, ok := LegendaryPower(v.PowerID); !ok {
		return false
	}
	if !LegendaryPowerHasRoll(v.PowerID) {
		v.PowerRollBP = 0
		return true
	}
	if rng == nil {
		return false
	}
	n := rng(int(LegendaryPowerRollMaxBP - LegendaryPowerRollMinBP + 1))
	if n < 0 || n > int(LegendaryPowerRollMaxBP-LegendaryPowerRollMinBP) {
		return false
	}
	v.PowerRollBP = LegendaryPowerRollMinBP + int32(n)
	return true
}

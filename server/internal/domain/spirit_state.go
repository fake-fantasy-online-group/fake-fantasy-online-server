package domain

import "fmt"

const (
	SpiritMaxAttributes    = 4
	SpiritMaxRolls         = 15
	SpiritMinIncrement     = 10
	SpiritPerfectIncrement = 20
)

// SpiritState is value-owned instance data, not a template lookup. Fixed arrays
// deliberately preserve Stack comparability and snapshot copy isolation.
// Each attribute has fifteen slots; a roll of twenty is permanent. Removing
// ordinary rolls never creates a sixteenth slot or rerolls a permanent value.
type SpiritState struct {
	// LegacyFixed identifies an existing fixed-template soul preserved through
	// separation. It must never be reinterpreted as a newly sealed dynamic soul.
	LegacyFixed bool
	Kind        uint8
	AttrCount   uint8
	Attrs       [SpiritMaxAttributes]int32
	Rolls       [SpiritMaxAttributes][SpiritMaxRolls]uint8
	// LastSlots uses 1..15, with zero denoting a row not rolled by the last
	// operation (for example a row that was already full).
	LastSlots [SpiritMaxAttributes]uint8
	LastValid bool
}

type SpiritWeight struct {
	Attr   int32
	Weight int32
}

func spiritAttributeAllowed(attr int32) bool {
	switch attr {
	case AttrSTR, AttrAGI, AttrSPI, AttrVIT, AttrINT, AttrAtk, AttrMAtk,
		AttrPhysRes, AttrDEX, AttrMagicRes, AttrDef, AttrMDef:
		return true
	}
	return false
}

func (s SpiritState) Validate() error {
	if s.Kind == 0 {
		if s != (SpiritState{}) {
			return fmt.Errorf("spirit: empty kind has instance data")
		}
		return nil
	}
	if s.Kind > 9 || s.AttrCount > SpiritMaxAttributes || s.AttrCount == 1 {
		return fmt.Errorf("spirit: invalid kind or attribute count")
	}
	if s.Kind == 1 && s.AttrCount != 0 && s.AttrCount != SpiritMaxAttributes {
		return fmt.Errorf("spirit: shoes require four attributes")
	}
	if s.LegacyFixed && s != (SpiritState{Kind: s.Kind, LegacyFixed: true}) {
		return fmt.Errorf("spirit: legacy fixed soul contains dynamic data")
	}
	seen := map[int32]bool{}
	last := false
	for row := range s.Attrs {
		if row >= int(s.AttrCount) {
			if s.Attrs[row] != 0 || s.Rolls[row] != ([SpiritMaxRolls]uint8{}) || s.LastSlots[row] != 0 {
				return fmt.Errorf("spirit: unused attribute contains data")
			}
			continue
		}
		if !spiritAttributeAllowed(s.Attrs[row]) || seen[s.Attrs[row]] {
			return fmt.Errorf("spirit: invalid or repeated attribute")
		}
		seen[s.Attrs[row]] = true
		for _, value := range s.Rolls[row] {
			if value != 0 && (value < SpiritMinIncrement || value > SpiritPerfectIncrement) {
				return fmt.Errorf("spirit: invalid roll value")
			}
		}
		index := s.LastSlots[row]
		if index > SpiritMaxRolls || index > 0 && (!s.LastValid || s.Rolls[row][index-1] == 0) {
			return fmt.Errorf("spirit: invalid last-operation slot")
		}
		last = last || index != 0
	}
	if s.LastValid != last {
		return fmt.Errorf("spirit: inconsistent last-operation marker")
	}
	return nil
}

func (s SpiritState) FilledCounts() (out [SpiritMaxAttributes]int32) {
	for row := 0; row < int(s.AttrCount) && row < len(out); row++ {
		for _, value := range s.Rolls[row] {
			if value != 0 {
				out[row]++
			}
		}
	}
	return
}

func (s SpiritState) Levels() [SpiritMaxAttributes]int32 { return s.FilledCounts() }

func (s SpiritState) PerfectCounts() (out [SpiritMaxAttributes]int32) {
	for row := 0; row < int(s.AttrCount) && row < len(out); row++ {
		for _, value := range s.Rolls[row] {
			if value == SpiritPerfectIncrement {
				out[row]++
			}
		}
	}
	return
}

func (s SpiritState) PerfectLevels() [SpiritMaxAttributes]int32 { return s.PerfectCounts() }

// Level reports the highest occupied row. After a reset different attributes
// can retain different numbers of perfect rolls. Consumers MUST use CanProcess
// rather than Level < 15: other rows may still have room to catch up.
func (s SpiritState) Level() int32 {
	var level int32
	for _, count := range s.FilledCounts() {
		if count > level {
			level = count
		}
	}
	return level
}

func (s SpiritState) Values() (out [SpiritMaxAttributes]int32) {
	for row := 0; row < int(s.AttrCount) && row < len(out); row++ {
		for _, value := range s.Rolls[row] {
			out[row] += int32(value)
		}
	}
	return
}

func (s SpiritState) PermanentValues() (out [SpiritMaxAttributes]int32) {
	for row, count := range s.PerfectCounts() {
		out[row] = count * SpiritPerfectIncrement
	}
	return
}

func (s SpiritState) LastValues() (out [SpiritMaxAttributes]int32) {
	if !s.LastValid {
		return
	}
	for row, slot := range s.LastSlots {
		if row < int(s.AttrCount) && slot > 0 && slot <= SpiritMaxRolls {
			out[row] = int32(s.Rolls[row][slot-1])
		}
	}
	return
}

func (s SpiritState) Affixes() []Affix {
	if s.Validate() != nil || s.AttrCount == 0 {
		return nil
	}
	values := s.Values()
	out := make([]Affix, 0, int(s.AttrCount))
	for row := 0; row < int(s.AttrCount); row++ {
		out = append(out, Affix{Attr: s.Attrs[row], Value: values[row], Mode: ModeAbsolute})
	}
	return out
}

func (s SpiritState) CanUnseal() bool {
	return !s.LegacyFixed && s.Kind != 0 && s.Validate() == nil && s.Level() == 0
}

func (s SpiritState) CanProcess() bool {
	if s.LegacyFixed || s.Kind == 0 || s.AttrCount == 0 || s.Validate() != nil {
		return false
	}
	for row, count := range s.FilledCounts() {
		if row < int(s.AttrCount) && count < SpiritMaxRolls {
			return true
		}
	}
	return false
}

func (s SpiritState) CanRollback() bool {
	if s.LegacyFixed || !s.LastValid || s.Validate() != nil {
		return false
	}
	for _, value := range s.LastValues() {
		if value != 0 && value < SpiritPerfectIncrement {
			return true
		}
	}
	return false
}

func (s SpiritState) CanReset() bool {
	if s.LegacyFixed || s.Kind == 0 || s.Validate() != nil {
		return false
	}
	for row := 0; row < int(s.AttrCount); row++ {
		for _, value := range s.Rolls[row] {
			if value != 0 && value < SpiritPerfectIncrement {
				return true
			}
		}
	}
	return false
}

func spiritRandom(random func(int) int, n int) (int, error) {
	if random == nil || n <= 0 {
		return 0, fmt.Errorf("spirit: missing random source")
	}
	v := random(n)
	if v < 0 || v >= n {
		return 0, fmt.Errorf("spirit: random source out of bounds")
	}
	return v, nil
}

// UnsealSpirit samples distinct attributes using the supplied audited weights.
// Shoes request fourOnly; other types use 60/35/5 percent for 2/3/4 attributes.
func UnsealSpirit(kind uint8, weights []SpiritWeight, fourOnly bool, random func(int) int) (SpiritState, error) {
	var zero SpiritState
	if kind == 0 || kind > 9 || kind == 1 && !fourOnly || len(weights) < SpiritMaxAttributes || len(weights) > 12 {
		return zero, fmt.Errorf("spirit: invalid unseal configuration")
	}
	pool := append([]SpiritWeight(nil), weights...)
	seen := map[int32]bool{}
	var total int64
	for _, entry := range pool {
		if !spiritAttributeAllowed(entry.Attr) || seen[entry.Attr] || entry.Weight <= 0 {
			return zero, fmt.Errorf("spirit: invalid attribute weights")
		}
		seen[entry.Attr] = true
		total += int64(entry.Weight)
	}
	if total > 1<<30 {
		return zero, fmt.Errorf("spirit: attribute weights overflow")
	}
	count := 4
	if !fourOnly {
		r, err := spiritRandom(random, 100)
		if err != nil {
			return zero, err
		}
		switch {
		case r < 60:
			count = 2
		case r < 95:
			count = 3
		}
	}
	out := SpiritState{Kind: kind, AttrCount: uint8(count)}
	for row := 0; row < count; row++ {
		r, err := spiritRandom(random, int(total))
		if err != nil {
			return zero, err
		}
		for index, entry := range pool {
			if r < int(entry.Weight) {
				out.Attrs[row] = entry.Attr
				total -= int64(entry.Weight)
				pool = append(pool[:index], pool[index+1:]...)
				break
			}
			r -= int(entry.Weight)
		}
	}
	if err := out.Validate(); err != nil {
		return zero, err
	}
	return out, nil
}

// Process changes a copy only. If a random source fails after some rows were
// considered, no partial candidate is returned and the receiver is unchanged.
func (s SpiritState) Process(random func(int) int) (SpiritState, error) {
	if !s.CanProcess() {
		return s, fmt.Errorf("spirit: no available processing slot")
	}
	out := s
	out.LastSlots = [SpiritMaxAttributes]uint8{}
	out.LastValid = false
	for row := 0; row < int(out.AttrCount); row++ {
		for index, value := range out.Rolls[row] {
			if value != 0 {
				continue
			}
			r, err := spiritRandom(random, SpiritPerfectIncrement-SpiritMinIncrement+1)
			if err != nil {
				return s, err
			}
			out.Rolls[row][index] = uint8(SpiritMinIncrement + r)
			out.LastSlots[row] = uint8(index + 1)
			out.LastValid = true
			break
		}
	}
	return out, out.Validate()
}

func (s SpiritState) Rollback() (SpiritState, error) {
	if !s.CanRollback() {
		return s, fmt.Errorf("spirit: no reversible last processing result")
	}
	out := s
	for row, slot := range out.LastSlots {
		if slot != 0 && out.Rolls[row][slot-1] != SpiritPerfectIncrement {
			out.Rolls[row][slot-1] = 0
		}
	}
	out.LastSlots = [SpiritMaxAttributes]uint8{}
	out.LastValid = false
	return out, out.Validate()
}

func (s SpiritState) Reset() (SpiritState, error) {
	if !s.CanReset() {
		return s, fmt.Errorf("spirit: no ordinary processing result to reset")
	}
	out := s
	for row := range out.Rolls {
		for index, value := range out.Rolls[row] {
			if value != SpiritPerfectIncrement {
				out.Rolls[row][index] = 0
			}
		}
	}
	out.LastSlots = [SpiritMaxAttributes]uint8{}
	out.LastValid = false
	return out, out.Validate()
}

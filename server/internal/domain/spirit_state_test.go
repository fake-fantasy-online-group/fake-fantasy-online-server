package domain

import (
	"encoding/json"
	"math/rand"
	"testing"
)

func spiritStateFixture() SpiritState {
	return SpiritState{Kind: 2, AttrCount: 4, Attrs: [4]int32{AttrSTR, AttrAGI, AttrSPI, AttrINT}}
}

func spiritStateSequence(values ...int) func(int) int {
	index := 0
	return func(n int) int {
		if index >= len(values) {
			panic("unexpected random call")
		}
		v := values[index]
		index++
		return v
	}
}

func TestSpiritStateMixedPerfectRollbackAndReset(t *testing.T) {
	s := spiritStateFixture()
	before := s
	processed, err := s.Process(spiritStateSequence(10, 1, 8, 10))
	if err != nil || processed.Values() != ([4]int32{20, 11, 18, 20}) || processed.LastValues() != processed.Values() {
		t.Fatalf("process = %+v, %v", processed, err)
	}
	if s != before || processed.Level() != 1 || !processed.CanRollback() {
		t.Fatal("mutation or incorrect latest operation")
	}
	rolled, err := processed.Rollback()
	if err != nil || rolled.Values() != ([4]int32{20, 0, 0, 20}) || rolled.Levels() != ([4]int32{1, 0, 0, 1}) {
		t.Fatalf("rollback = %+v, %v", rolled, err)
	}
	if rolled.CanRollback() || rolled.CanReset() || rolled.CanUnseal() || rolled.LastValid || rolled.LastSlots != ([4]uint8{}) {
		t.Fatal("permanent rows must survive and latest receipt must be spent")
	}
	if again, err := rolled.Rollback(); err == nil || again != rolled {
		t.Fatal("repeated rollback changed state")
	}
	next, err := rolled.Process(spiritStateSequence(0, 10, 2, 9))
	if err != nil || next.Values() != ([4]int32{30, 20, 12, 39}) {
		t.Fatalf("second process = %+v, %v", next, err)
	}
	reset, err := next.Reset()
	if err != nil || reset.Values() != ([4]int32{20, 20, 0, 20}) || reset.PermanentValues() != reset.Values() {
		t.Fatalf("reset = %+v, %v", reset, err)
	}
	if reset.Attrs != before.Attrs || reset.Level() != 1 || reset.LastValid || reset.LastSlots != ([4]uint8{}) {
		t.Fatal("reset modified attributes or left a stale latest operation")
	}
	if again, err := reset.Reset(); err == nil || again != reset {
		t.Fatal("repeated no-op reset changed permanent state")
	}
}

func TestSpiritStateFifteenRollCapAndPartialCatchup(t *testing.T) {
	s := spiritStateFixture()
	for i := 0; i < 15; i++ {
		var err error
		s, err = s.Process(spiritStateSequence(10, 0, 0, 0))
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.CanProcess() || s.Level() != 15 || s.Values() != ([4]int32{300, 150, 150, 150}) {
		t.Fatalf("bad cap %+v", s.Values())
	}
	if unchanged, err := s.Process(func(int) int { panic("full spirit used RNG") }); err == nil || unchanged != s {
		t.Fatal("full spirit processed")
	}
	reset, err := s.Reset()
	if err != nil || !reset.CanProcess() || reset.Level() != 15 || reset.Levels() != ([4]int32{15, 0, 0, 0}) {
		t.Fatalf("catch-up state %+v %v", reset, err)
	}
	// A full permanent row receives neither a roll nor a sixteenth increment.
	next, err := reset.Process(spiritStateSequence(10, 10, 10))
	if err != nil || next.LastSlots[0] != 0 || next.Values() != ([4]int32{300, 20, 20, 20}) || next.CanRollback() {
		t.Fatalf("catch-up result %+v %v", next, err)
	}
	for i := 1; i < 15; i++ {
		next, err = next.Process(spiritStateSequence(10, 10, 10))
		if err != nil {
			t.Fatal(err)
		}
	}
	if next.Values() != ([4]int32{300, 300, 300, 300}) || next.CanProcess() || next.CanReset() || next.CanRollback() {
		t.Fatalf("permanent full state invalid: %+v", next)
	}
}

func TestSpiritStateRandomOperationsCannotInflateOrLosePerfectValues(t *testing.T) {
	rng := rand.New(rand.NewSource(427))
	s := spiritStateFixture()
	for step := 0; step < 3000; step++ {
		before := s
		var err error
		switch rng.Intn(3) {
		case 0:
			s, err = s.Process(rng.Intn)
		case 1:
			s, err = s.Rollback()
		case 2:
			s, err = s.Reset()
		}
		if err != nil && s != before {
			t.Fatalf("failed operation changed state at %d", step)
		}
		if s.Validate() != nil || s.Attrs != before.Attrs {
			t.Fatalf("invalid state at %d", step)
		}
		beforePerfect, afterPerfect := before.PermanentValues(), s.PermanentValues()
		values := s.Values()
		for row := 0; row < 4; row++ {
			if afterPerfect[row] < beforePerfect[row] || values[row] > 300 || values[row] < afterPerfect[row] {
				t.Fatalf("permanent loss or inflation at %d: %+v -> %+v", step, beforePerfect, values)
			}
		}
	}
}

func TestSpiritStateUnsealCountBoundariesAndNoDuplicateAttributes(t *testing.T) {
	weights := []SpiritWeight{{AttrSTR, 1}, {AttrAGI, 1}, {AttrINT, 1}, {AttrSPI, 1}}
	for _, tc := range []struct{ roll, count int }{{0, 2}, {59, 2}, {60, 3}, {94, 3}, {95, 4}, {99, 4}} {
		calls := 0
		s, err := UnsealSpirit(2, weights, false, func(n int) int {
			calls++
			if calls == 1 {
				return tc.roll
			}
			return 0
		})
		if err != nil || int(s.AttrCount) != tc.count || !s.CanUnseal() || s.CanRollback() || s.CanReset() || !s.CanProcess() {
			t.Fatalf("count roll %d: %+v %v", tc.roll, s, err)
		}
		seen := map[int32]bool{}
		for row := 0; row < tc.count; row++ {
			if seen[s.Attrs[row]] {
				t.Fatal("duplicate weighted attribute")
			}
			seen[s.Attrs[row]] = true
		}
	}
	shoe, err := UnsealSpirit(1, weights, true, spiritStateSequence(3, 2, 1, 0))
	if err != nil || shoe.AttrCount != 4 || shoe.Attrs != ([4]int32{AttrSPI, AttrINT, AttrAGI, AttrSTR}) {
		t.Fatalf("shoe forced four = %+v %v", shoe, err)
	}
	if weights[0].Attr != AttrSTR || weights[0].Weight != 1 || len(weights) != 4 {
		t.Fatal("unseal mutated shared configuration")
	}
}

func TestSpiritStateOrdinaryResetReturnsToZeroWithoutReselectingAttributes(t *testing.T) {
	s := spiritStateFixture()
	attrs := s.Attrs
	for i := 0; i < 15; i++ {
		var err error
		s, err = s.Process(func(int) int { return 9 })
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.Level() != 15 || s.CanProcess() || !s.CanReset() {
		t.Fatal("ordinary full state did not reach its cap")
	}
	reset, err := s.Reset()
	if err != nil || reset.Level() != 0 || reset.Values() != ([4]int32{}) || reset.Attrs != attrs || !reset.CanUnseal() || !reset.CanProcess() {
		t.Fatalf("ordinary reset: %+v %v", reset, err)
	}
}

func TestSpiritStateUnsealRejectsMalformedWeightsAndRandom(t *testing.T) {
	valid := []SpiritWeight{{AttrSTR, 1}, {AttrAGI, 1}, {AttrINT, 1}, {AttrSPI, 1}}
	cases := [][]SpiritWeight{
		nil,
		{{AttrSTR, 1}, {AttrSTR, 1}, {AttrINT, 1}, {AttrSPI, 1}},
		{{AttrSTR, 1}, {AttrAGI, -1}, {AttrINT, 1}, {AttrSPI, 1}},
		{{AttrSTR, 1}, {AttrAGI, 1}, {AttrMaxHP, 1}, {AttrSPI, 1}},
		{{AttrSTR, 1 << 30}, {AttrAGI, 1 << 30}, {AttrINT, 1}, {AttrSPI, 1}},
	}
	for _, weights := range cases {
		if _, err := UnsealSpirit(2, weights, false, func(int) int { panic("bad configuration used RNG") }); err == nil {
			t.Fatalf("invalid weights accepted: %+v", weights)
		}
	}
	for _, kind := range []uint8{0, 10, 255} {
		if _, err := UnsealSpirit(kind, valid, true, nil); err == nil {
			t.Fatal("invalid kind accepted")
		}
	}
	for _, random := range []func(int) int{nil, func(int) int { return -1 }, func(n int) int { return n }, spiritStateSequence(0, 0, 3)} {
		if out, err := UnsealSpirit(2, valid, false, random); err == nil || out != (SpiritState{}) {
			t.Fatalf("invalid RNG returned unsealed partial result: %+v %v", out, err)
		}
	}
}

func TestSpiritStateInvalidInputAndRandomFailureAreAtomic(t *testing.T) {
	valid := spiritStateFixture()
	cases := []SpiritState{{Kind: 10}, {Kind: 1, AttrCount: 1}, {Kind: 0, AttrCount: 2}}
	bad := valid
	bad.Attrs[1] = bad.Attrs[0]
	cases = append(cases, bad)
	bad = valid
	bad.Attrs[2] = AttrMaxHP
	cases = append(cases, bad)
	bad = valid
	bad.Rolls[0][0] = 9
	cases = append(cases, bad)
	bad = valid
	bad.Rolls[0][0] = 21
	cases = append(cases, bad)
	bad = valid
	bad.LastValid = true
	cases = append(cases, bad)
	bad = valid
	bad.LastSlots[0] = 16
	cases = append(cases, bad)
	bad = valid
	bad.LastValid, bad.LastSlots[0] = true, 1
	cases = append(cases, bad)
	bad = valid
	bad.AttrCount = 2 // unused rows retain data
	cases = append(cases, bad)
	for index, bad := range cases {
		if bad.Validate() == nil || bad.CanProcess() || bad.CanReset() || bad.CanUnseal() || bad.CanRollback() {
			t.Fatalf("invalid input %d accepted: %+v", index, bad)
		}
	}
	for _, random := range []func(int) int{nil, func(int) int { return -1 }, func(n int) int { return n }, spiritStateSequence(0, 0, 11)} {
		if out, err := valid.Process(random); err == nil || out != valid {
			t.Fatal("invalid RNG produced partial state")
		}
	}
	if (SpiritState{}).Validate() != nil || (SpiritState{Kind: 2}).Validate() != nil || !(SpiritState{Kind: 2}).CanUnseal() {
		t.Fatal("empty or sealed state rejected")
	}
}

func TestSpiritStateComparableCopyAndJSONRoundTrip(t *testing.T) {
	s, err := spiritStateFixture().Process(spiritStateSequence(0, 1, 2, 10))
	if err != nil {
		t.Fatal(err)
	}
	copy := s
	copy.Rolls[0][0] = 20
	if s.Rolls[0][0] == copy.Rolls[0][0] {
		t.Fatal("copy aliases original")
	}
	set := map[SpiritState]bool{s: true}
	if !set[s] {
		t.Fatal("state lost comparable identity")
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SpiritState
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded != s || decoded.Validate() != nil {
		t.Fatalf("round trip %s: %+v %v", encoded, decoded, err)
	}
	if got := s.Affixes(); len(got) != 4 || got[0] != (Affix{Attr: AttrSTR, Value: 10, Mode: ModeAbsolute}) {
		t.Fatalf("affixes %+v", got)
	}
}

func TestSpiritStateLegacyFixedPreservedAndNotProcessable(t *testing.T) {
	for kind := uint8(1); kind <= 9; kind++ {
		state := SpiritState{Kind: kind, LegacyFixed: true}
		if err := state.Validate(); err != nil {
			t.Fatal(err)
		}
		if state.CanUnseal() || state.CanProcess() || state.CanReset() || state.CanRollback() {
			t.Fatal("legacy fixed became dynamic")
		}
		if next, err := state.Process(func(int) int { return 0 }); err == nil || next != state {
			t.Fatal("process changed legacy")
		}
		if next, err := state.Rollback(); err == nil || next != state {
			t.Fatal("rollback changed legacy")
		}
		if next, err := state.Reset(); err == nil || next != state {
			t.Fatal("reset changed legacy")
		}
		b, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		var restored SpiritState
		if err = json.Unmarshal(b, &restored); err != nil || restored != state {
			t.Fatalf("legacy roundtrip %v", err)
		}
		bad := state
		bad.AttrCount = 2
		bad.Attrs = [4]int32{AttrAtk, AttrMAtk}
		if bad.Validate() == nil {
			t.Fatal("mixed legacy and dynamic accepted")
		}
	}
	if (SpiritState{LegacyFixed: true}).Validate() == nil {
		t.Fatal("legacy kind zero accepted")
	}
}

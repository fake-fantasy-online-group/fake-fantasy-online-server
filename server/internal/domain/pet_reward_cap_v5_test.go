package domain

import (
	"math"
	"reflect"
	"testing"
)

func TestV5PetRewardCapPreservesLegalOverOwnerProgress(t *testing.T) {
	def := 海龟
	table := NewPetLevelTable(v5ProgressionTestCurve(PetLevelCap))
	prefix := PetPrefixDef{GrowthOdd: Base{STR: 2}, FreePointsPerLevel: 2}
	pet := PetInstance{Level: 64, Base: def.BaseAt(64), HP: 40, MP: 50}
	// XP beads may legally put a pet above its owner's ordinary reward cap.
	bead := pet.AddExpWithPrefix(def, prefix, table, table.Need(64)+37, PetExperienceItemCap(def, 60, 10))
	if bead.Reason != "" || pet.Level != 65 || pet.Exp != 37 {
		t.Fatalf("bead setup failed: %+v, %+v", bead, pet)
	}
	before := pet
	for _, gain := range []int64{100, math.MaxInt64} {
		got := pet.AddExpWithPrefix(def, prefix, table, gain, PetCapFor(def, 60))
		if got.Reason != "" || got.AppliedExp != 0 || got.DiscardedExp != gain || !got.Overflowed || got.FromLevel != 65 || got.ToLevel != 65 || got.Levels != 0 {
			t.Fatalf("legal over-owner pet rejected or rewarded: %+v", got)
		}
		if !reflect.DeepEqual(pet, before) {
			t.Fatalf("capped reward changed saved pet: before=%+v after=%+v", before, pet)
		}
	}
	// Once the owner catches up, the existing within-level balance still applies.
	got := pet.AddExp(def, table, table.Need(65)-pet.Exp, PetCapFor(def, 66))
	if got.Reason != "" || pet.Level != 66 || pet.Exp != 0 || got.Levels != 1 {
		t.Fatalf("reward did not resume after owner caught up: %+v, %+v", got, pet)
	}
}

func TestV5PetRewardCapRejectsInvalidProgress(t *testing.T) {
	for _, tc := range []struct {
		name            string
		level, max, cap int32
		exp             int64
	}{
		{name: "absolute cap", level: 81, max: 100, cap: 60, exp: 37},
		{name: "species cap", level: 31, max: 30, cap: 20, exp: 37},
		{name: "negative progress", level: 65, max: 80, cap: 60, exp: -1},
		{name: "invalid level", level: 0, max: 80, cap: 60},
		{name: "invalid reward cap", level: 65, max: 80, cap: 0, exp: 37},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := 海龟
			def.MaxLevel = tc.max
			pet := PetInstance{Level: tc.level, Exp: tc.exp, Base: def.BaseAt(tc.level)}
			before := pet
			got := pet.AddExp(def, NewPetLevelTable(v5ProgressionTestCurve(PetLevelCap)), 100, tc.cap)
			if got.Reason != "invalid_progress" || got.AppliedExp != 0 || !reflect.DeepEqual(pet, before) {
				t.Fatalf("invalid save was accepted or changed: %+v, %+v", got, pet)
			}
		})
	}
}

func TestV5PetRewardCapCannotExceedHardLimit(t *testing.T) {
	for _, maxLevel := range []int32{30, 100} {
		def := 海龟
		def.MaxLevel = maxLevel
		hardCap := PetCapFor(def, PetLevelCap)
		table := NewPetLevelTable(v5ProgressionTestCurve(PetLevelCap))
		pet := PetInstance{Level: hardCap - 1, Base: def.BaseAt(hardCap - 1)}
		got := pet.AddExp(def, table, table.Need(pet.Level)+37, 100)
		if got.Reason != "" || pet.Level != hardCap || pet.Exp != 0 || got.DiscardedExp != 37 {
			t.Fatalf("hard cap %d not enforced: %+v, %+v", hardCap, got, pet)
		}
	}
}

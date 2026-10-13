package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
)

// ValidateExperienceCurve rejects incomplete, nonpositive, or decreasing input.
// The cap row is validated for reference-data integrity, not an upgrade to cap+1.
func ValidateExperienceCurve(need map[int32]int64, cap int32) error {
	if cap < 1 {
		return fmt.Errorf("invalid progression cap %d", cap)
	}
	var previous int64
	for lv := int32(1); lv <= cap; lv++ {
		n, ok := need[lv]
		if !ok || n <= 0 || n < previous {
			return fmt.Errorf("invalid progression edge %d: need=%d previous=%d", lv, n, previous)
		}
		previous = n
	}
	return nil
}

// settleExperience is atomic even if an edge is missing halfway through a gain.
// No historical lifetime-total column participates in the current-level balance.
func settleExperience(level int32, progress, gain int64, cap int32, need func(int32) int64) (GainResult, int32, int64) {
	res := GainResult{FromLevel: level, ToLevel: level}
	oldLevel, oldProgress := level, progress
	if gain <= 0 {
		return res, level, progress
	}
	if level < 1 || level > cap || progress < 0 || cap < 1 {
		res.Reason = "invalid_progress"
		return res, oldLevel, oldProgress
	}
	if level >= cap {
		res.Overflowed = true
		res.DiscardedExp = gain
		return res, level, progress
	}
	if gain > math.MaxInt64-progress {
		res.Reason = "experience_overflow"
		return res, oldLevel, oldProgress
	}
	progress += gain
	for level < cap {
		n := need(level)
		if n <= 0 {
			res.Reason = "missing_experience_edge"
			return res, oldLevel, oldProgress
		}
		if progress < n {
			break
		}
		progress -= n
		level++
	}
	res.Levels = level - oldLevel
	res.ToLevel = level
	res.AppliedExp = gain
	if level == cap {
		res.DiscardedExp = progress
		// Existing within-level progress can never exceed the reached-cap cost on a valid save.
		if res.DiscardedExp > gain {
			res.DiscardedExp = gain
		}
		res.AppliedExp = gain - res.DiscardedExp
		res.Overflowed = progress > 0
		progress = 0
	}
	return res, level, progress
}

// ExperienceWithBonus preserves the historical floor(exp*bonus/100) order while
// checking multiplication and addition in Int64. Negative bonuses are rejected.
func ExperienceWithBonus(exp int64, bonus int32) (int64, error) {
	if exp < 0 || bonus < 0 {
		return 0, fmt.Errorf("invalid experience multiplier")
	}
	b := int64(bonus)
	if b == 0 || exp == 0 {
		return exp, nil
	}
	q, r := exp/100, exp%100
	if q > math.MaxInt64/b {
		return 0, fmt.Errorf("experience bonus overflow")
	}
	extra := q*b + r*b/100
	if extra < 0 || exp > math.MaxInt64-extra {
		return 0, fmt.Errorf("experience addition overflow")
	}
	return exp + extra, nil
}

func PetRewardExperience(base int64, bonus int32) (int64, error) {
	if base < 0 {
		return 0, fmt.Errorf("negative reward experience")
	}
	if bonus < 0 {
		bonus = 0
	}
	return ExperienceWithBonus(SharedExp(base), bonus)
}

func PetExperienceItemCap(d PetDef, ownerLevel, maxAboveOwner int32) int32 {
	limit := int64(ownerLevel) + int64(maxAboveOwner)
	if limit < 1 {
		limit = 1
	}
	if limit > PetLevelCap {
		limit = PetLevelCap
	}
	return PetCapFor(d, int32(limit))
}

// ProgressionRules is immutable after startup. The hashes bind validated
// per-level inputs to the migration version; no lifetime-total column is read.
type ProgressionRules struct {
	player *LevelTable
	pet    *PetLevelTable
}

func NewProgressionRules(player *LevelTable, pet *PetLevelTable, playerHash, petHash string) (*ProgressionRules, error) {
	if player == nil || pet == nil {
		return nil, fmt.Errorf("missing progression curves")
	}
	players, pets := map[int32]int64{}, map[int32]int64{}
	for l := int32(1); l <= LevelCap; l++ {
		players[l] = player.Need(l)
	}
	for l := int32(1); l <= PetLevelCap; l++ {
		pets[l] = pet.Need(l)
	}
	if err := ValidateExperienceCurve(players, LevelCap); err != nil {
		return nil, err
	}
	if err := ValidateExperienceCurve(pets, PetLevelCap); err != nil {
		return nil, err
	}
	if experienceCurveHash(player.Need, LevelCap) != playerHash || experienceCurveHash(pet.Need, PetLevelCap) != petHash {
		return nil, fmt.Errorf("progression checksum mismatch")
	}
	return &ProgressionRules{player: player, pet: pet}, nil
}
func experienceCurveHash(need func(int32) int64, cap int32) string {
	h := sha256.New()
	for l := int32(1); l <= cap; l++ {
		fmt.Fprintf(h, "%d:%d\n", l, need(l))
	}
	return hex.EncodeToString(h.Sum(nil))
}

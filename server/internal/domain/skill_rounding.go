package domain

import (
	"fmt"
	"math"
	"math/bits"
)

// SkillRoundingCredit is an already-paid fraction of one mana point or one
// cooldown tick. It belongs to one character and one skill, never a catalogue.
// Zero/zero is the compatible empty state of characters created before this rule.
type SkillRoundingCredit struct {
	Numerator   int64 `json:"n"`
	Denominator int64 `json:"d"`
}

type SkillRoundingState struct {
	Mana     SkillRoundingCredit `json:"mana"`
	Cooldown SkillRoundingCredit `json:"cooldown"`
}

const maxSkillRoundingDenominator int64 = (int64(math.MaxInt32) + 50) * TickMS

func skillRoundingAllowed(id SkillID) bool {
	switch id {
	case 10018, 10227, 10301, 10305, 10309, 10432:
		return true
	}
	return false
}

func ValidateSkillRoundingCredits(states map[SkillID]SkillRoundingState) error {
	for id, state := range states {
		if !skillRoundingAllowed(id) {
			return fmt.Errorf("skill rounding: unsupported skill %d", id)
		}
		for _, c := range []SkillRoundingCredit{state.Mana, state.Cooldown} {
			if c == (SkillRoundingCredit{}) {
				continue
			}
			if c.Denominator <= 0 || c.Denominator > maxSkillRoundingDenominator || c.Numerator < 0 || c.Numerator >= c.Denominator {
				return fmt.Errorf("skill rounding: invalid credit for skill %d", id)
			}
		}
	}
	return nil
}

// RoundSkillCost previews a positive rational cost without consuming credit.
// Starting from zero, stable rates sum to ceil(N*n/d), never below one per use.
// When a rate's denominator changes, conversion rounds existing credit down:
// re-equipping can forfeit less than 1/d, but can never manufacture a refund.
func RoundSkillCost(n, d int64, old SkillRoundingCredit) (int64, SkillRoundingCredit) {
	if n <= 0 || d <= 0 {
		return 0, old
	}
	n = max(n, d)
	credit := int64(0)
	if old.Denominator > 0 && old.Numerator >= 0 && old.Numerator < old.Denominator {
		hi, lo := bits.Mul64(uint64(old.Numerator), uint64(d))
		q, _ := bits.Div64(hi, lo, uint64(old.Denominator))
		credit = int64(q)
	}
	whole, rem := n/d, n%d
	if rem > credit {
		whole++
		credit += d - rem
	} else {
		credit -= rem
	}
	return whole, SkillRoundingCredit{Numerator: credit, Denominator: d}
}

// PreviewSkillRounding applies only the six approved post-cap continuations.
// Authored levels and absent costs keep their previous behaviour. Fractional
// cooldowns are paid in actual 100ms ticks, including the phase-0 receipt.
func PreviewSkillRounding(def *SkillDef, old SkillRoundingState) SkillRoundingState {
	next := old
	if def == nil || !skillRoundingAllowed(def.ID) || def.ManaCostReductionRanks <= 0 {
		return next
	}
	if def.ManaCostFractionNumerator > 0 && def.ManaCostFractionDenominator > 0 {
		v, c := RoundSkillCost(def.ManaCostFractionNumerator, def.ManaCostFractionDenominator, old.Mana)
		def.MPCost = int32(v)
		next.Mana = c
	}
	n, d := def.CooldownFractionNumeratorMS, def.CooldownFractionDenominator
	if n > 0 && d > 0 && (def.CooldownRoundingApplied || int64(def.CooldownMS) <= (n+d-1)/d) {
		if variant := def.EquipmentVariant; variant != nil && variant.Behavior == VariantInstantCast && variant.Params.MinCooldownMS > 0 {
			// Variant minimums are also actual tick floors; accumulated credit
			// must never shorten the authored equipment-form restriction.
			n = max(n, int64(Ticks(int(variant.Params.MinCooldownMS)))*d*TickMS)
		}
		v, c := RoundSkillCost(n, d*TickMS, old.Cooldown)
		def.CooldownMS = SaturateInt32(v * TickMS)
		next.Cooldown = c
		def.CooldownRoundingApplied = true
	}
	def.RoundingPreviewReady, def.RoundingPreviewNext = true, next
	return next
}

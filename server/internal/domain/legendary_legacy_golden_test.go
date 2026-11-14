package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
)

// Golden recaptured after typified source-policy decoupling: typified class-limited
// powers (TY-U/D with a profession Class) stay in the typified A/D/U pool and keep
// typified source-level rules (sourceLevel>0; catalog MinLevel is metadata). They
// were incorrectly excluded from source 43 rolls when MinLevel>43 (notably TY-U17
// and TY-D11). Recipient class still filters activation and generation. Previous
// hashes: 1e650f40/2652489 = 3aa18e6f3d8e16a22a6b39836d88c30e8cdde60707418b608627e0befec5c209;
// post-reclass+MinLevel-coupling = 7b73f2ee859c9f7fa93f9f1ad742c1943bf391917940815bd2057523f06e0ccc.
func TestLegendaryLegacyPublishedGolden(t *testing.T) {
	def, table := sourceLevelFixture(t)
	hash := sha256.New()
	for _, slot := range []EquipSlot{SlotHead, SlotFace, SlotShoe, SlotWeapon, SlotBag, SlotRing} {
		def.Equip.Slot = int32(slot)
		def.Equip.Need.Professions = 31
		for _, recipient := range []LegendaryRecipientClass{0, 1, 2, 4, 8, 16} {
			for seed := int64(0); seed < 20; seed++ {
				for _, variants := range []bool{false, true} {
					st := NewStack(def, 1)
					rng := rand.New(rand.NewSource(seed))
					var bounds []int
					next := func(n int) int { bounds = append(bounds, n); return rng.Intn(n) }
					var ok bool
					if variants {
						ok = table.ApplyLegendaryForRecipientAtLevel(&st, def, 43, recipient, next)
					} else {
						ok = table.ApplyLegendaryLegacyForRecipientAtLevel(&st, def, 43, recipient, next)
					}
					data, _ := json.Marshal(struct {
						OK     bool
						Power  LegendaryInstance
						Source int32
						Bounds []int
						Next   int64
					}{ok, st.Legendary, st.Endgame.SourceLevel, bounds, rng.Int63()})
					hash.Write(data)
				}
			}
		}
	}
	got := fmt.Sprintf("%x", hash.Sum(nil))
	const want = "4e4790a6c95916c5161b291aa37b11b00cd56390b8b1052f57b5538b88e6441c"
	if got != want {
		t.Fatalf("published generation/RNG changed: got %s want %s", got, want)
	}
}

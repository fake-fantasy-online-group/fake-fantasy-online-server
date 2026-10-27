package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestLegendaryPowerRollFrozenRiftAndNewGeneration(t *testing.T) {
	s, old, _, _ := splitPowerRiftFixture(t)
	fresh := *old
	loot := *old.Loot
	loot.Version, loot.LegendaryPowerRolls = 5, true
	fresh.Version, fresh.LootVersion, fresh.Loot = 17, 5, &loot
	identity := endgameRiftIdentity{Race: domain.Warlock, Gender: 0, Employed: true}
	const recipient domain.LegendaryRecipientClass = 16
	for _, rules := range []*domain.RiftRules{old, &fresh} {
		pool := s.riftCumulativeLegendaryPool(rules, 43, 43, 65, identity, recipient)
		prepareCumulativeRun(s, rules, "power-roll-freeze", 150, true, 43)
		numeric := [2]int{}
		for i := int32(0); i < 256; i++ {
			results := []domain.Stack{s.rollEndgameRiftCumulativeLegendary(100, i, pool, recipient), s.rollEndgameRiftNormalBossEquipment(100, i, pool, 0, recipient)}
			for path, st := range results {
				if st.Legendary.PowerID == "" {
					t.Fatalf("missing legendary version=%d path=%d", rules.Version, path)
				}
				if rules.Version < 17 || !domain.LegendaryPowerHasRoll(st.Legendary.PowerID) {
					if st.Legendary.PowerRollBP != 0 {
						t.Fatalf("legacy/fixed gained roll: %+v", st.Legendary)
					}
				} else {
					numeric[path]++
					if st.Legendary.PowerRollBP < 7500 || st.Legendary.PowerRollBP > 10000 {
						t.Fatalf("new numeric roll invalid: %+v", st.Legendary)
					}
				}
			}
		}
		if rules.Version == 17 && (numeric[0] == 0 || numeric[1] == 0) {
			t.Fatalf("new paths did not produce numeric rolls: %v", numeric)
		}
	}
}

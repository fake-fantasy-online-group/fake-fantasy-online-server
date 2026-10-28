package scene

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestLifeRewardFrozenRiftAndReadyGrant(t *testing.T) {
	s, base, _, _ := splitPowerRiftFixture(t)
	old := *base
	l5 := *base.Loot
	l5.Version = 5
	l5.LegendaryPowerRolls = true
	old.Version = 17
	old.LootVersion = 5
	old.Loot = &l5
	fresh := old
	l6 := l5
	l6.Version = 6
	l6.RetiredCubeMaterials = true
	fresh.Version = 18
	fresh.LootVersion = 6
	fresh.Loot = &l6
	var historical string
	for _, rules := range []*domain.RiftRules{&old, &fresh} {
		prepareCumulativeRun(s, rules, "life-reward-freeze", 30, true, 43)
		s.generateEndgameRiftGrants()
		if !s.rift.data.GrantsReady {
			t.Fatal("not ready")
		}
		grant := s.rift.data.Grants[100]
		kept := []domain.Stack{}
		dust := false
		for _, st := range grant.Items {
			if st.Item == domain.CubePurifiedDust {
				dust = true
				continue
			}
			kept = append(kept, st)
		}
		if rules.Version == 17 {
			if !dust {
				t.Fatal("old entitlement removed")
			}
			raw, _ := json.Marshal(preSpiritRiftProjection(t, normalizeRiftStacks(s.rift.data.Grants[100].Items)))
			historical = fmt.Sprintf("%x", sha256.Sum256(raw))
			if historical != "3a3b812093c609ea50451784866658430c74d4a4f56605dcfb872e34bda02363" {
				t.Fatal("historical golden changed", historical)
			}
			prepareCumulativeRun(s, rules, "life-reward-freeze", 30, true, 43)
			s.generateEndgameRiftGrants()
			replay, _ := json.Marshal(preSpiritRiftProjection(t, normalizeRiftStacks(s.rift.data.Grants[100].Items)))
			if fmt.Sprintf("%x", sha256.Sum256(replay)) != historical {
				t.Fatal("old replay changed")
			}
		} else {
			if dust {
				t.Fatal("new dust produced")
			}
			if len(kept) == 0 {
				t.Fatal("new equipment/runes lost")
			}
		}
		before, _ := json.Marshal(s.rift.data.Grants)
		s.rift.data.Run.Rules = &fresh
		s.generateEndgameRiftGrants()
		after, _ := json.Marshal(s.rift.data.Grants)
		if string(before) != string(after) {
			t.Fatal("ready grant rewritten")
		}
	}
}

func TestLifeRewardNativeKillAndTheftRetireMaterials(t *testing.T) {
	for _, retired := range []bool{false, true} {
		s, mid := lootScene(t, []domain.DropEntry{{Item: domain.CubePurifiedDust, RatePct: 100}, {Item: 掉落树枝.ID, RatePct: 100}})
		s.items[domain.CubePurifiedDust] = domain.ItemDef{ID: domain.CubePurifiedDust, Stackable: true}
		fixture, _, _, _ := splitPowerRiftFixture(t)
		for id, def := range fixture.items {
			if _, ok := s.items[id]; !ok {
				s.items[id] = def
			}
		}
		old := *s.lootRules.Active()
		newer := old
		newer.Version = 5
		newer.LegendaryPowerPolicy = 1
		newer.LegendaryPowerRolls = true
		if retired {
			newer.Version = 6
			newer.RetiredCubeMaterials = true
		}
		var err error
		s.lootRules, err = domain.NewLootRuleTable(newer.Version, []domain.LootRuleSet{old, newer}, s.items)
		if err != nil {
			t.Fatal(err)
		}
		entries := s.theftEntries(必掉的怪.ID)
		want := 2
		if retired {
			want = 1
		}
		if len(entries) != want {
			t.Fatal("theft pool", len(entries), want)
		}
		s.rollLoot(s.entities[mid], 0)
		if len(s.ground) != want {
			t.Fatal("kill rewards", len(s.ground), want)
		}
	}
}

// Preserve the original golden rather than blessing a new hash: only the
// three zero-valued fields introduced by a7f4c22 are excluded. Nonzero spirit
// metadata on historical loot is still a hard failure.
func preSpiritRiftProjection(t *testing.T, stacks []domain.Stack) []preSpiritRiftStack {
	t.Helper()
	result := make([]preSpiritRiftStack, len(stacks))
	for i, st := range stacks {
		if st.Spirit != (domain.SpiritState{}) || st.FusedSpirit != (domain.SpiritAttachment{}) || st.SpiritAppearance != 0 {
			t.Fatal("legacy run gained spirit metadata")
		}
		result[i] = preSpiritRiftStack{Stack: st}
	}
	return result
}

type preSpiritRiftStack struct {
	domain.Stack
	Spirit           *domain.SpiritState      `json:",omitempty"`
	FusedSpirit      *domain.SpiritAttachment `json:",omitempty"`
	SpiritAppearance *domain.ItemID           `json:",omitempty"`
}

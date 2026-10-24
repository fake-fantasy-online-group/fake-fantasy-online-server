package store

import (
	"context"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func storedLegendaryTier(tier domain.LegendaryTier) domain.Stack {
	st := legendaryStoreFixture()
	st.Legendary.Tier = tier
	if tier == domain.LegendaryTierSecret {
		for i, a := range st.Legendary.Affixes {
			st.Legendary.Bounds[i] = domain.LegendaryAffixBounds{Min: a.Affix.Value, Max: a.Affix.Value}
		}
	}
	return st
}

func TestLegendaryTierMemoryReloadAndEndgameReplay(t *testing.T) {
	for _, tier := range []domain.LegendaryTier{domain.LegendaryTierNormal, domain.LegendaryTierTrue, domain.LegendaryTierSecret} {
		t.Run(string(tier), func(t *testing.T) {
			m, s := endgameFixture(t)
			st := storedLegendaryTier(tier)
			s.Bag.Set(0, st)
			if err := m.SaveSnapshot(context.Background(), s); err != nil {
				t.Fatal(err)
			}
			if got := durableSnapshot(t, m, s).Bag.At(0); got != st {
				t.Fatal("reload lost tier")
			}
			s.Bag.Set(0, domain.Stack{})
			s.Worn.Set(domain.SlotWeapon, st)
			if err := m.SaveSnapshot(context.Background(), s); err != nil {
				t.Fatal(err)
			}
			worn, err := m.LoadEquips(context.Background(), s.Char.ID)
			if err != nil || worn.At(domain.SlotWeapon) != st {
				t.Fatal("equipped reload", err)
			}
			s.Worn.Set(domain.SlotWeapon, domain.Stack{})
			s.Bag.Set(0, st)
			if err = m.SaveSnapshot(context.Background(), s); err != nil {
				t.Fatal(err)
			}
			p := endgamePlan(s)
			p.Snapshot.Bag.Set(0, st)
			backend := NewEndgameBackend(m, nil)
			result := awaitEndgame(t, backend.Commit(p))
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			result = awaitEndgame(t, backend.Commit(p))
			if result.Err != nil || !result.Replayed || result.Authoritative == nil || result.Authoritative.Bag.At(0) != st {
				t.Fatal("receipt replay lost immutable tier", result)
			}
		})
	}
}

func TestLegendaryTierStoredJSONBackwardCompatibility(t *testing.T) {
	for _, tier := range []domain.LegendaryTier{domain.LegendaryTierNormal, domain.LegendaryTierTrue, domain.LegendaryTierSecret} {
		st := storedLegendaryTier(tier)
		// PostgreSQL stores this exact domain object as JSONB, independently of all
		// location tables. Loading old JSON leaves Tier at its zero/ordinary value.
		raw, err := json.Marshal(st.Legendary)
		if err != nil {
			t.Fatal(err)
		}
		loaded := st
		loaded.Legendary = domain.LegendaryInstance{}
		if err = json.Unmarshal(raw, &loaded.Legendary); err != nil || loaded != st || validateLegendaryItem(loaded) != nil {
			t.Fatal("JSONB contract", err)
		}
	}
}

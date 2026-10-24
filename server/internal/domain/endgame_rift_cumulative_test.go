package domain

import "testing"

func TestRiftCumulativeLegendaryPlanBoundaries(t *testing.T) {
	old := riftRulesFixture()
	fresh := cloneRiftRules(*old)
	fresh.Version, fresh.CumulativeLegendaryPerTier, fresh.NormalBossEquipment = 12, 10, 10
	fresh.Equipment, fresh.TimelyEquipment = 0, 0
	fresh.UniqueBaseChance, fresh.UniquePerTier, fresh.UniqueCap = 0, 0, 0
	for _, tier := range []int32{1, 9, 10, 11, 15, 99, 100, 149, 150} {
		for _, timely := range []bool{false, true} {
			p := PlanRiftRewards(tier, timely, &fresh)
			if p.Equipment != 0 || p.UniqueChance != 0 || p.NormalBossEquipment != 10 || p.GuaranteedLegendaries != tier/10 || p.ExtraLegendaryChance != tier%10*10 {
				t.Fatalf("tier%d timely%v: %+v", tier, timely, p)
			}
			legacy := PlanRiftRewards(tier, timely, old)
			if p.Rune != legacy.Rune || p.Dust != legacy.Dust {
				t.Fatal("unrelated rewards changed")
			}
			for draw := 0; draw < 100; draw++ {
				calls := 0
				got := p.CumulativeLegendaryCount(func(n int) int {
					calls++
					if n != 100 {
						t.Fatal(n)
					}
					return draw
				})
				want := tier / 10
				if int32(draw) < tier%10*10 {
					want++
				}
				if got != want || (tier%10 == 0 && calls != 0) || (tier%10 != 0 && calls != 1) {
					t.Fatalf("tier%d draw%d got%d want%d calls%d", tier, draw, got, want, calls)
				}
			}
		}
	}
}

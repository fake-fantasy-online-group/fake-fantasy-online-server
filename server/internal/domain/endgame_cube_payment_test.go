package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestCubeWingAutomaticPaymentAndChange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mats   []CubeMaterialDef
		want   []CubeMaterialDef
		change int32
	}{
		{"exact preferred", []CubeMaterialDef{{25029, 3}, {25028, 2}}, []CubeMaterialDef{{25029, 2}, {25028, 2}}, 0},
		{"high change", []CubeMaterialDef{{25029, 5}}, []CubeMaterialDef{{25029, 3}}, 15},
		{"mixed least value", []CubeMaterialDef{{25029, 3}, {25027, 8}}, []CubeMaterialDef{{25029, 3}}, 15},
		{"low surplus", []CubeMaterialDef{{25027, 100}}, []CubeMaterialDef{{25027, 60}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, count := range []int32{-1, 0, 1, 2, 2147483647} {
				ctx := wingCubeFixture(t)
				setWingAvailable(ctx.Bag, tc.mats)
				req := wingInputs(42, tc.mats...)
				for i := range req.CubeInputs {
					req.CubeInputs[i].Count = count
				}
				before := ctx.Bag.Clone()
				rs, ps, err := MatchCubeInputs(ctx, req)
				if err != nil || len(ps) != 1 {
					t.Fatalf("count %d: %v", count, err)
				}
				p, err := PlanCube(ctx, rs[0], true)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(ctx.Bag, before) {
					t.Fatal("preview mutated bag")
				}
				for _, m := range tc.want {
					got := before.CountOf(m.ItemID) - p.Bag.CountOf(m.ItemID)
					if m.ItemID == 25027 {
						got += tc.change
					}
					if got != m.Count {
						t.Fatalf("debit %d=%d want%d", m.ItemID, got, m.Count)
					}
				}
				if tc.change > 0 && (len(p.Outputs) != 1 || p.Outputs[0].Count != tc.change || !strings.Contains(p.Description, "返还")) {
					t.Fatalf("missing change preview: %+v", p)
				}
			}
		})
	}
}
func TestCubeWingAmbiguityUsesOnlyExactLegacyIntent(t *testing.T) {
	ctx := wingCubeFixture(t)
	s := ctx.Bag.At(200)
	s.Item = 22863
	ctx.Bag.Set(200, s)
	for _, tc := range []struct {
		n      int32
		target ItemID
	}{{60, 22864}, {201, 20609}, {1, 0}, {0, 0}, {-1, 0}, {200, 0}} {
		_, ps, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25027, tc.n}))
		if tc.target == 0 {
			if CubeErrorCode(err) != "configuration" {
				t.Fatalf("%d: %v", tc.n, err)
			}
		} else if err != nil || ps[0].Target.Item != tc.target {
			t.Fatalf("%d: %v", tc.n, err)
		}
	}
	for _, tc := range []struct {
		n  int32
		id int32
	}{{1, 900090}, {5, 900091}, {2, 0}, {0, 0}} {
		rs, _, err := MatchCubeInputs(ctx, wingInputs(0, CubeMaterialDef{25028, tc.n}))
		if tc.id == 0 {
			if CubeErrorCode(err) != "configuration" {
				t.Fatal(err)
			}
		} else if err != nil || rs[0].RecipeID != tc.id {
			t.Fatalf("%d:%v", tc.n, err)
		}
	}
}

func TestCubeWingChangeCapacityBindingAndLocks(t *testing.T) {
	for _, bound := range []bool{false, true} {
		ctx := wingCubeFixture(t)
		ctx.Bag.Each(func(i int, st Stack) {
			if st.Item == 25029 {
				st.Count = 4
				st.Bound = bound
				ctx.Bag.Set(i, st)
			}
		})
		_, plans, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 1}))
		if err != nil || len(plans[0].Outputs) != 1 || plans[0].Outputs[0].Bound != bound {
			t.Fatalf("refund binding %v: %v", bound, err)
		}
	}
	ctx := wingCubeFixture(t)
	for i := 100; i < 200; i++ {
		ctx.Bag.Set(i, Stack{Item: 11682, Count: 1})
	}
	ctx.Bag.Set(110, Stack{Item: 25029, Count: 4, Bound: true})
	before := ctx.Bag.Clone()
	_, _, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 1}))
	if CubeErrorCode(err) != "bag_full" || !reflect.DeepEqual(before, ctx.Bag) {
		t.Fatalf("change capacity not atomic: %v", err)
	}
	st := ctx.Bag.At(110)
	st.Locked = true
	ctx.Bag.Set(110, st)
	_, _, err = MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 1}))
	if CubeErrorCode(err) != "materials" {
		t.Fatalf("locked selected high: %v", err)
	}
}

func TestCubeWingPaymentSolverMatchesExhaustiveMinimum(t *testing.T) {
	ctx := wingCubeFixture(t)
	recipe, _ := ctx.WingRules.Recipe(900001)
	for low := int32(0); low <= 12; low++ {
		for mid := int32(0); mid <= 12; mid++ {
			for high := int32(0); high <= 4; high++ {
				available := map[ItemID]int32{}
				if low > 0 {
					available[25027] = low
				}
				if mid > 0 {
					available[25028] = mid
				}
				if high > 0 {
					available[25029] = high
				}
				payment, ok, _ := chooseWingMaterials(ctx.WingRules, recipe, available)
				bestValue, bestCount := int32(999), int32(999)
				for l := int32(0); l <= low; l++ {
					for m := int32(0); m <= mid; m++ {
						for h := int32(0); h <= high; h++ {
							v := l + m*5 + h*25
							n := l + m + h
							if v >= 60 && (v < bestValue || v == bestValue && n < bestCount) {
								bestValue, bestCount = v, n
							}
						}
					}
				}
				if bestValue == 999 {
					if ok {
						t.Fatal("unaffordable succeeded")
					}
					continue
				}
				v := payment[25027] + payment[25028]*5 + payment[25029]*25
				n := payment[25027] + payment[25028] + payment[25029]
				if !ok || v != bestValue || n != bestCount {
					t.Fatalf("%v => %v (%v), want %d/%d", available, payment, ok, bestValue, bestCount)
				}
			}
		}
	}
}

package domain

import (
	"reflect"
	"strconv"
	"testing"
)

func TestWingValueAllUpgradeDenominationsAndActualDebits(t *testing.T) {
	for high := int32(0); high <= 2; high++ {
		for mid := int32(0); mid <= (60-high*25)/5; mid++ {
			low := 60 - high*25 - mid*5
			ctx := wingCubeFixture(t)
			var mats []CubeMaterialDef
			for _, m := range []CubeMaterialDef{{25027, low}, {25028, mid}, {25029, high}} {
				if m.Count > 0 {
					mats = append(mats, m)
				}
			}
			setWingAvailable(ctx.Bag, mats)
			before := ctx.Bag.Clone()
			reqs, plans, err := MatchCubeInputs(ctx, wingInputs(42, mats...))
			if err != nil || len(plans) != 1 {
				t.Fatalf("%v: %v", mats, err)
			}
			plan, err := PlanCube(ctx, reqs[0], true)
			if err != nil || !reflect.DeepEqual(plan.Bag, plans[0].Bag) || plan.Target.Item != 20611 {
				t.Fatalf("%v: execute %v", mats, err)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("planning mutated bag")
			}
			if len(plan.Materials) != len(mats) {
				t.Fatal("preview lists authored rather than actual input")
			}
			for _, m := range mats {
				if plan.Bag.CountOf(m.ItemID) != before.CountOf(m.ItemID)-m.Count {
					t.Fatalf("wrong debit %v", m)
				}
			}
		}
	}
}
func TestWingValueMarkersAndMismatches(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source ItemID
		mats   []CubeMaterialDef
		target ItemID
		code   string
	}{
		{"dark marker", 20609, []CubeMaterialDef{{25027, 200}, {11682, 1}}, 22863, ""},
		{"light marker low", 22863, []CubeMaterialDef{{25027, 201}}, 20609, ""},
		{"light marker mixed", 22863, []CubeMaterialDef{{25027, 1}, {25028, 40}}, 20609, ""},
		{"light marker missing chooses upgrade", 22863, []CubeMaterialDef{{25029, 8}}, 22864, ""},
		{"light marker not free", 22863, []CubeMaterialDef{{25027, 200}}, 0, "configuration"},
		{"dark extra marker left over", 20609, []CubeMaterialDef{{25029, 8}, {11682, 2}}, 22863, ""},
		{"dark missing marker chooses upgrade", 20609, []CubeMaterialDef{{25029, 8}}, 20611, ""},
		{"upgrade short", 20609, []CubeMaterialDef{{25027, 59}}, 0, "insufficient_count"},
		{"upgrade surplus left over", 20609, []CubeMaterialDef{{25027, 61}}, 20611, ""},
		{"upgrade rare is not value", 20609, []CubeMaterialDef{{25027, 59}, {11682, 1}}, 0, "insufficient_count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := wingCubeFixture(t)
			st := ctx.Bag.At(200)
			st.Item = tc.source
			ctx.Bag.Set(200, st)
			setWingAvailable(ctx.Bag, tc.mats)
			before := ctx.Bag.Clone()
			reqs, plans, err := MatchCubeInputs(ctx, wingInputs(42, tc.mats...))
			if tc.code != "" {
				if CubeErrorCode(err) != tc.code {
					t.Fatalf("want %s got %v", tc.code, err)
				}
				if !reflect.DeepEqual(before, ctx.Bag) {
					t.Fatal("rejected input consumed")
				}
				return
			}
			if err != nil || len(plans) != 1 || plans[0].Target.Item != tc.target {
				t.Fatalf("%v %+v", err, plans)
			}
			if _, err = PlanCube(ctx, reqs[0], true); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestWingValueInternalPlanCannotBypassChecks(t *testing.T) {
	ctx := wingCubeFixture(t)
	reqs, _, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25027, 60}))
	if err != nil {
		t.Fatal(err)
	}
	base := reqs[0]
	for _, tc := range []struct {
		name   string
		inputs []CubeInput
	}{
		{"short", []CubeInput{{ItemID: 25027, Count: 59}}},
		{"excess", []CubeInput{{ItemID: 25027, Count: 61}}},
		{"duplicate", []CubeInput{{ItemID: 25027, Count: 30}, {ItemID: 25027, Count: 30}}},
		{"negative", []CubeInput{{ItemID: 25027, Count: -1}}},
		{"uid", []CubeInput{{ItemID: 25027, Count: 60, UID: "42"}}},
		{"rare", []CubeInput{{ItemID: 11682, Count: 60}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.CubeInputs = tc.inputs
			if _, err := PlanCube(ctx, r, true); err == nil {
				t.Fatal("invalid material plan accepted")
			}
		})
	}
	other := base
	other.CubeInputs = []CubeInput{{ItemID: 25028, Count: 12}}
	if CubeFingerprint(ctx.Bag, ctx.Money, base) == CubeFingerprint(ctx.Bag, ctx.Money, other) {
		t.Fatal("fingerprint omitted denomination selection")
	}
	st := ctx.Bag.At(110)
	st.Locked = true
	ctx.Bag.Set(110, st)
	if _, err := PlanCube(ctx, base, true); err == nil {
		t.Fatal("locked materials consumed")
	}
}

func TestWingValueEveryConfiguredEdgeAcceptsLowDenomination(t *testing.T) {
	base := wingCubeFixture(t)
	for _, recipe := range base.WingRules.Recipes() {
		if recipe.Operation == "wing.feather" {
			continue
		}
		t.Run(strconv.Itoa(int(recipe.ID)), func(t *testing.T) {
			ctx := wingCubeFixture(t)
			if recipe.ProfessionMask == 24 {
				ctx.Character.Race = Warlock
			}
			st := ctx.Bag.At(200)
			st.Item = recipe.Source
			ctx.Bag.Set(200, st)
			var low int32
			var rare int32
			for _, m := range recipe.Materials {
				switch m.ItemID {
				case 25027:
					low += m.Count
				case 25028:
					low += 5 * m.Count
				case 25029:
					low += 25 * m.Count
				case 11682:
					rare = m.Count
				}
			}
			mats := []CubeMaterialDef{{25027, low}}
			if rare > 0 {
				mats = append(mats, CubeMaterialDef{11682, rare})
			}
			reqs, plans, err := MatchCubeInputs(ctx, wingInputs(42, mats...))
			if err != nil || len(reqs) != 1 || reqs[0].RecipeID != recipe.ID || plans[0].Target.Item != recipe.Target {
				t.Fatalf("recipe %d: %v %+v", recipe.ID, err, reqs)
			}
		})
	}
}

func setWingAvailable(bag *Bag, mats []CubeMaterialDef) {
	for _, m := range mats {
		bag.Each(func(i int, st Stack) {
			if st.Item == m.ItemID {
				st.Count = m.Count
				bag.Set(i, st)
			}
		})
	}
}

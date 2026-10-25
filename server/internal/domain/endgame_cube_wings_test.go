package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
)

func wingCubeFixture(t *testing.T) CubeContext {
	t.Helper()
	ctx := cubeFixture()
	raw, err := os.ReadFile("../../config/cube-wing-progression-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg WingProgressionConfig
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, w := range cfg.Items {
		ctx.Items[w.Item] = ItemDef{ID: w.Item, Name: "native wing", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{Slot: int32(SlotBag), Durable: 100, AscensionOnly: w.Ascended, RefineLimit: 10}}
	}
	for _, id := range cfg.FeatherIDs {
		ctx.Items[id] = ItemDef{ID: id, Name: "native feather", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
	}
	ctx.WingRules, err = NewWingProgressionRules(cfg, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Character = &Character{Race: Warrior, EmploymentKnown: true, Employed: true}
	st := ctx.Bag.At(200)
	st.Item = 20609
	ctx.Bag.Set(200, st)
	for i, id := range cfg.FeatherIDs {
		ctx.Bag.Set(110+i, Stack{Item: id, Count: 5000, Bound: true})
	}
	return ctx
}
func wingInputs(uid int64, mats ...CubeMaterialDef) EndgameRequest {
	req := EndgameRequest{Op: "cube.quote"}
	if uid > 0 {
		req.CubeInputs = []CubeInput{{UID: strconv.FormatInt(uid, 10), Count: 1}}
	}
	for _, m := range mats {
		req.CubeInputs = append(req.CubeInputs, CubeInput{ItemID: m.ItemID, Count: m.Count})
	}
	return req
}
func TestWingCubeConfiguredChainsCostsAndPreservation(t *testing.T) {
	ctx := wingCubeFixture(t)
	ctx.Bag.At(200)
	want := []ItemID{20611, 20613, 20615, 20617, 20621, 20625, 21905}
	for _, id := range want {
		source := ctx.Bag.At(200)
		var recipe WingCubeRecipeDef
		for _, r := range ctx.WingRules.Recipes() {
			if r.Source == source.Item && r.Target == id {
				recipe = r
				break
			}
		}
		before := ctx.Bag.Clone()
		reqs, plans, err := MatchCubeInputs(ctx, wingInputs(42, recipe.Materials...))
		if err != nil || len(reqs) != 1 || len(plans) != 1 {
			t.Fatalf("%d: %v", id, err)
		}
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("quote mutated bag")
		}
		plan, err := PlanCube(ctx, reqs[0], true)
		if err != nil {
			t.Fatal(err)
		}
		got := plan.Target
		if got.Item != id || got.UID != 42 || got.RefineLevel != source.RefineLevel || got.FusedAppearance != source.FusedAppearance || got.FusedSoul != source.FusedSoul || got.FusedDragon != source.FusedDragon || got.WingPowers != source.WingPowers || got.Endgame.Revision != source.Endgame.Revision+1 {
			t.Fatalf("lost identity: %+v", got)
		}
		ctx.Bag = plan.Bag
	}
	if !ctx.Items[21905].Equip.AscensionOnly {
		t.Fatal("god wear flag waived")
	}
}
func TestWingCubeProfessionForkAndExistingVariant(t *testing.T) {
	for _, tc := range []struct {
		race Race
		want ItemID
	}{{Warrior, 20617}, {Swordsman, 20617}, {Assassin, 20617}, {Healer, 20618}, {Warlock, 20618}} {
		ctx := wingCubeFixture(t)
		ctx.Character.Race = tc.race
		st := ctx.Bag.At(200)
		st.Item = 20615
		ctx.Bag.Set(200, st)
		_, p, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 32}))
		if err != nil || p[0].Target.Item != tc.want {
			t.Fatalf("%v: %v %+v", tc, err, p)
		}
	}
	ctx := wingCubeFixture(t)
	ctx.Character.Employed = false
	st := ctx.Bag.At(200)
	st.Item = 20615
	ctx.Bag.Set(200, st)
	if _, _, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 32})); CubeErrorCode(err) != "requirements" {
		t.Fatal(err)
	}
	// An owned magic variant remains magic even when the current job is physical.
	ctx.Character.Employed = true
	st.Item = 20618
	ctx.Bag.Set(200, st)
	_, p, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 60}))
	if err != nil || p[0].Target.Item != 20622 {
		t.Fatal(err)
	}
}
func TestWingCubeConversionNeeds200PlusOneAndUsesCommonUpgradeFeathers(t *testing.T) {
	ctx := wingCubeFixture(t)
	_, p, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 8}, CubeMaterialDef{11682, 1}))
	if err != nil || p[0].Target.Item != 22863 {
		t.Fatalf("%v %+v", err, p)
	}
	ctx.Bag = p[0].Bag
	_, p, err = MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 2}, CubeMaterialDef{25028, 2}))
	if err != nil || p[0].Target.Item != 22864 {
		t.Fatal(err)
	}
	for _, m := range [][]CubeMaterialDef{{{25029, 7}, {11682, 1}}, {{25029, 8}, {11682, 2}}} {
		before := ctx.Bag.Clone()
		_, _, err = MatchCubeInputs(ctx, wingInputs(42, m...))
		if err == nil || !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("failed conversion mutated bag")
		}
	}
}
func TestWingCubeFeatherExchangeAndNoRareMint(t *testing.T) {
	ctx := wingCubeFixture(t)
	_, p, err := MatchCubeInputs(ctx, wingInputs(0, CubeMaterialDef{25027, 5}))
	if err != nil || len(p) != 1 || p[0].Outputs[0].Item != 25028 {
		t.Fatalf("%v %+v", err, p)
	}
	for _, r := range ctx.WingRules.Recipes() {
		if r.Operation == "wing.feather" {
			for _, o := range r.Outputs {
				if o.ItemID == 11682 {
					t.Fatal("rare feather minted")
				}
			}
		}
	}
}
func TestWingCubeRejectsMalformedAndStaleInputsWithoutMutation(t *testing.T) {
	for _, mutate := range []func(*CubeContext, *EndgameRequest){
		func(c *CubeContext, r *EndgameRequest) { r.CubeInputs = append(r.CubeInputs, r.CubeInputs[0]) },
		func(c *CubeContext, r *EndgameRequest) { st := c.Bag.At(200); st.Locked = true; c.Bag.Set(200, st) },
		func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[0].UID = "042" },
		func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[0].Count = 2 },
		func(c *CubeContext, r *EndgameRequest) { c.Bag.Set(200, Stack{}) },
	} {
		ctx := wingCubeFixture(t)
		req := wingInputs(42, CubeMaterialDef{25029, 2}, CubeMaterialDef{25028, 2})
		mutate(&ctx, &req)
		before := ctx.Bag.Clone()
		_, _, err := MatchCubeInputs(ctx, req)
		if err == nil || !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("invalid input accepted or mutated")
		}
	}
}

func TestWingCubeRejectsAmbiguousOrRareMintConfiguration(t *testing.T) {
	ctx := wingCubeFixture(t)
	for _, mutate := range []func(*WingProgressionConfig){
		func(c *WingProgressionConfig) {
			for i := range c.Recipes {
				if c.Recipes[i].Operation == "wing.feather" {
					c.Recipes[i].Outputs[0].ItemID = 11682
					return
				}
			}
		},
		func(c *WingProgressionConfig) {
			for i := range c.Recipes {
				if c.Recipes[i].Operation == "wing.feather" {
					c.Recipes[i].Outputs[0].Count = 999
					return
				}
			}
		},
		func(c *WingProgressionConfig) {
			for i := range c.Recipes {
				if c.Recipes[i].ProfessionMask == 24 {
					c.Recipes[i].ProfessionMask = 7
					return
				}
			}
		},
	} {
		cfg := ctx.WingRules.Config()
		mutate(&cfg)
		if _, err := NewWingProgressionRules(cfg, ctx.Items); err == nil {
			t.Fatal("unsafe recipe configuration accepted")
		}
	}
}

func TestWingCubeAcceptsProvenLegacyEquipmentKind(t *testing.T) {
	ctx := wingCubeFixture(t)
	st := ctx.Bag.At(200)
	st.InstanceKind = ItemInstanceNone
	ctx.Bag.Set(200, st)
	_, plans, err := MatchCubeInputs(ctx, wingInputs(42, CubeMaterialDef{25029, 2}, CubeMaterialDef{25028, 2}))
	if err != nil || plans[0].Target.Item != 20611 || plans[0].Target.InstanceKind != ItemInstanceEquipment {
		t.Fatalf("legacy wing failed: %v", err)
	}
}

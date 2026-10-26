package domain

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestWingMaterialSnapshotUnionPreservesLegacyAndSortsOnce(t *testing.T) {
	ctx := wingCubeFixture(t)
	ctx.Items[RiftKeyItemID] = ItemDef{ID: RiftKeyItemID, Name: "符钥"}
	legacy := CubeSnapshotMaterialIDs(ctx.Rules, ctx.Catalog)
	got := CubeSnapshotMaterialIDs(ctx.Rules, ctx.Catalog, ctx.WingRules)
	if len(got) != len(legacy)+4 {
		t.Fatalf("wing materials omitted or duplicated: %v", got)
	}
	seen := map[ItemID]bool{}
	for i, id := range got {
		if seen[id] || i > 0 && got[i-1] >= id {
			t.Fatal("not sorted and unique", got)
		}
		seen[id] = true
	}
	for _, id := range append(legacy, 25027, 25028, 25029, 11682) {
		if !seen[id] {
			t.Fatal("missing material", id)
		}
	}
	if !reflect.DeepEqual(got, CubeSnapshotMaterialIDs(ctx.Rules, ctx.Catalog, nil, ctx.WingRules, ctx.WingRules)) {
		t.Fatal("duplicate recipe tables change inventory")
	}
	if !reflect.DeepEqual(legacy, CubeSnapshotMaterialIDs(ctx.Rules, ctx.Catalog, nil)) {
		t.Fatal("nil wings changed legacy tables")
	}
	if err := ValidateCubeSnapshotMaterials(ctx.Items, ctx.Catalog, ctx.Rules, ctx.WingRules); err != nil {
		t.Fatal(err)
	}
}

func TestWingMaterialSnapshotBoundIncludesWingRows(t *testing.T) {
	for _, n := range []int{64, 65} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			ctx := wingCubeFixture(t)
			ctx.Items[RiftKeyItemID] = ItemDef{ID: RiftKeyItemID, Name: "符钥"}
			count := len(CubeSnapshotMaterialIDs(ctx.Rules, ctx.Catalog, ctx.WingRules))
			defs := ctx.Rules.Recipes()
			for i := count; i < n; i++ {
				id := ItemID(1999000000 + i)
				ctx.Items[id] = ItemDef{ID: id, Name: "额外材料", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
				defs[0].Materials = append(defs[0].Materials, CubeMaterialDef{ItemID: id, Count: 1})
			}
			rules, err := NewCubeRuleTable(defs, ctx.Items)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(CubeSnapshotMaterialIDs(rules, ctx.Catalog, ctx.WingRules)); got != n {
				t.Fatal(got)
			}
			err = ValidateCubeSnapshotMaterials(ctx.Items, ctx.Catalog, rules, ctx.WingRules)
			if (err == nil) != (n == 64) {
				t.Fatalf("row bound %d: %v", n, err)
			}
		})
	}
	ctx := wingCubeFixture(t)
	ctx.Items[RiftKeyItemID] = ItemDef{ID: RiftKeyItemID, Name: "符钥"}
	for _, id := range []ItemID{25027, 25028, 25029} {
		d := ctx.Items[id]
		d.Name = strings.Repeat("a", 6000)
		ctx.Items[id] = d
	}
	if err := ValidateCubeSnapshotMaterials(ctx.Items, ctx.Catalog, ctx.Rules); err != nil {
		t.Fatal("legacy fixture itself invalid", err)
	}
	if err := ValidateCubeSnapshotMaterials(ctx.Items, ctx.Catalog, ctx.Rules, ctx.WingRules); err == nil || !strings.Contains(err.Error(), "16KiB") {
		t.Fatalf("wing byte budget not enforced: %v", err)
	}
}

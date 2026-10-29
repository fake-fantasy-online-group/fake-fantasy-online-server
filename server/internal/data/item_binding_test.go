package data

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

type fixedBindingQuery struct{ rows *v5SingleUserRows }

func (q fixedBindingQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "game_items") || !strings.Contains(sql, "game_equipment") || !strings.Contains(sql, "fixed_binding IS NOT NULL") {
		return nil, fmt.Errorf("binding loader must use both explicit definition sources")
	}
	return q.rows, nil
}
func fixedBindingRows(values ...[]any) *v5SingleUserRows {
	out := &v5SingleUserRows{at: -1}
	for _, row := range values {
		var raw []json.RawMessage
		for _, v := range row {
			b, _ := json.Marshal(v)
			raw = append(raw, b)
		}
		out.values = append(out.values, raw)
	}
	return out
}
func TestFixedItemBindingLoaderRejectsEquipmentUnknownAndDuplicates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		def   domain.ItemDef
		rows  [][]any
		valid bool
	}{
		{"rune", domain.ItemDef{ID: 1900028001, InstanceKind: domain.ItemInstanceSocketCard}, [][]any{{int32(1900028001), false, "authored transferable rune"}}, true},
		{"scroll-kind1", domain.ItemDef{ID: 1900028303, InstanceKind: domain.ItemInstanceEquipment}, [][]any{{int32(1900028303), true, "permanently bound scroll"}}, true},
		{"wearable", domain.ItemDef{ID: 9, Equip: &domain.EquipDef{}}, [][]any{{int32(9), false, "bad"}}, false},
		{"missing", domain.ItemDef{ID: 9}, [][]any{{int32(10), false, "bad"}}, false},
		{"empty-source", domain.ItemDef{ID: 9}, [][]any{{int32(9), false, ""}}, false},
		{"duplicate", domain.ItemDef{ID: 9}, [][]any{{int32(9), false, "a"}, {int32(9), true, "b"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defs := ItemDefs{tc.def.ID: tc.def}
			err := loadFixedItemBindings(context.Background(), fixedBindingQuery{fixedBindingRows(tc.rows...)}, defs)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t err=%v", tc.valid, err)
			}
			if tc.valid && defs[tc.def.ID].FixedBinding == domain.ItemBindingUnspecified {
				t.Fatal("rule not installed")
			}
		})
	}
}

func TestFixedItemBindingAuditedMigrationAndLoaderAgree(t *testing.T) {
	raw, err := testfixture.ReadMigrationJSON("000000_init-127.sql", "$fixed_item_bindings$")
	if err != nil {
		t.Fatal(err)
	}
	var rules []struct {
		ID     domain.ItemID `json:"id"`
		Bound  bool          `json:"bound"`
		Source string        `json:"source"`
		Table  string        `json:"table"`
	}
	if err = json.Unmarshal(raw, &rules); err != nil || len(rules) != 66 {
		t.Fatal("audited seed missing", err)
	}
	defs := ItemDefs{}
	var values [][]any
	gearRows := 0
	for _, rule := range rules {
		if _, ok := defs[rule.ID]; ok {
			t.Fatal("duplicate rule")
		}
		if rule.Table == "game_equipment" {
			gearRows++
		}
		defs[rule.ID] = domain.ItemDef{ID: rule.ID}
		values = append(values, []any{int32(rule.ID), rule.Bound, rule.Source})
	}
	if gearRows != 9 {
		t.Fatal("dynamic spirit source count changed")
	}
	if err = loadFixedItemBindings(context.Background(), fixedBindingQuery{fixedBindingRows(values...)}, defs); err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules {
		st := domain.Stack{Item: rule.ID, Count: 1, Bound: !rule.Bound, Locked: true}
		got := domain.NormalizeItemBinding(defs[rule.ID], st)
		if got.Bound != rule.Bound || !got.Locked {
			t.Fatal("migration/runtime rule mismatch")
		}
	}
	for id := domain.ItemID(3334); id <= 3338; id++ {
		if _, ok := defs[id]; ok {
			t.Fatal("ambiguous consignment cards must remain unaudited")
		}
	}
}

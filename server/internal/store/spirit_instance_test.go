package store

import (
	"context"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

func TestSpiritStateStrictPersistedDecode(t *testing.T) {
	state := domain.SpiritState{Kind: 2, AttrCount: 2, Attrs: [4]int32{domain.AttrSTR, domain.AttrINT}}
	state.Rolls[0][0] = 20
	state.Rolls[1][0] = 11
	raw, _ := json.Marshal(state)
	got, err := decodeSpiritState(raw)
	if err != nil || got != state {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
	for _, name := range []string{"extra_attr", "extra_row", "extra_roll", "short_roll", "extra_last", "unknown", "null", "trailing", "bad_roll"} {
		t.Run(name, func(t *testing.T) {
			var obj map[string]any
			_ = json.Unmarshal(raw, &obj)
			switch name {
			case "extra_attr":
				obj["Attrs"] = append(obj["Attrs"].([]any), float64(domain.AttrAGI))
			case "extra_row":
				obj["Rolls"] = append(obj["Rolls"].([]any), make([]any, 15))
			case "extra_roll":
				rows := obj["Rolls"].([]any)
				rows[0] = append(rows[0].([]any), float64(20))
			case "short_roll":
				rows := obj["Rolls"].([]any)
				rows[0] = rows[0].([]any)[:14]
			case "extra_last":
				obj["LastSlots"] = append(obj["LastSlots"].([]any), float64(1))
			case "unknown":
				obj["Future"] = true
			case "null":
				obj["Attrs"] = nil
			case "bad_roll":
				obj["Rolls"].([]any)[0].([]any)[0] = float64(21)
			}
			bad, _ := json.Marshal(obj)
			if name == "trailing" {
				bad = append(bad, []byte(" {}")...)
			}
			if _, err := decodeSpiritState(bad); err == nil {
				t.Fatalf("accepted corrupt state %s", bad)
			}
		})
	}
}

type spiritLoadQuerier struct {
	revisionRecordingQuerier
	rows    []*powerScrollRows
	queries []string
}

func (q *spiritLoadQuerier) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	q.queries = append(q.queries, sql)
	r := q.rows[0]
	q.rows = q.rows[1:]
	return r, nil
}
func TestSpiritNestedPersistedKindValidation(t *testing.T) {
	state := domain.SpiritState{Kind: 2, AttrCount: 2, Attrs: [4]int32{domain.AttrSTR, domain.AttrINT}}
	raw, _ := json.Marshal(state)
	for _, kind := range []int16{4, 1, 2, 3, 0} {
		q := &spiritLoadQuerier{rows: []*powerScrollRows{{read: true}, {values: []any{int64(10), domain.ItemID(0), int64(11), domain.ItemID(47028), kind, false, false, raw}}}}
		items := map[int64]domain.Stack{10: {UID: 10, Item: 999, Count: 1, InstanceKind: domain.ItemInstanceEquipment, FusedSoul: 47028}}
		err := loadSpiritInstances(context.Background(), q, items)
		if (err == nil) != (kind == 4) {
			t.Fatalf("kind=%d err=%v", kind, err)
		}
		if !strings.Contains(q.queries[1], "i.instance_kind") {
			t.Fatal("query omitted real nested kind")
		}
		if kind == 4 && items[10].FusedSpirit.State != state {
			t.Fatal("lost nested state")
		}
	}
}

func TestSpiritLegacyStateDecodeAndLocationHydration(t *testing.T) {
	state := domain.SpiritState{Kind: 9, LegacyFixed: true}
	raw, _ := json.Marshal(state)
	got, err := decodeSpiritState(raw)
	if err != nil || got != state {
		t.Fatalf("legacy flag lost: %+v %v", got, err)
	}
	spirit := domain.Stack{UID: 11, Item: 8038, Count: 1, InstanceKind: domain.ItemInstanceSpirit, Spirit: state}
	gear := domain.Stack{UID: 10, Item: 999, Count: 1, InstanceKind: domain.ItemInstanceEquipment, FusedSoul: 8038, FusedSpirit: domain.SpiritAttachmentOf(spirit), SpiritAppearance: 13698}
	// All bag/worn/warehouse/mail location readers hydrate through this helper.
	for _, fact := range []domain.Stack{spirit, gear} {
		cached := domain.Stack{UID: fact.UID, Item: fact.Item, Count: 1, InstanceKind: fact.InstanceKind}
		got, err := hydrateStack(cached, map[int64]domain.Stack{fact.UID: fact})
		if err != nil || got != fact {
			t.Fatalf("location lost spirit metadata: %+v %v", got, err)
		}
	}
}

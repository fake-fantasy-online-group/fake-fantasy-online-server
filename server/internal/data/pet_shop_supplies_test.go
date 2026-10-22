package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

const petSuppliesUpgrade = "../../migrations/upgrades/000101_20261022_pet_merchant_supplies.sql"

func petSupplyFixture(t *testing.T) ItemDefs {
	t.Helper()
	raw, err := os.ReadFile("testdata/pet-shop-items.json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []domain.ItemDef
	if err = json.Unmarshal(raw, &defs); err != nil {
		t.Fatal(err)
	}
	native, err := os.ReadFile("../../migrations/000000_init-3.sql")
	if err != nil {
		t.Fatal(err)
	}
	out := ItemDefs{}
	for _, def := range defs {
		if !strings.Contains(string(native), fmt.Sprintf("  (%d, '%s', %d,", def.ID, def.Name, def.Price)) {
			t.Fatalf("fixture diverged from native item: %+v", def)
		}
		out[def.ID] = def
	}
	if len(out) != 12 {
		t.Fatal("need exactly 12 native foods/medicines")
	}
	return out
}

// This test exercises the loader independently of PostgreSQL. It is not proof
// that the upgrade SQL executed; the opt-in real PostgreSQL test below is that gate.
type petShopQuery struct{ rows *v5SingleUserRows }

func (q petShopQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "COALESCE(s.stock_source_shop_id, s.shop_id)") {
		return nil, fmt.Errorf("stock aliases must resolve in SQL")
	}
	return q.rows, nil
}
func TestPetSuppliesLoadShops(t *testing.T) {
	items := petSupplyFixture(t)
	stock := []domain.ItemID{3382, 3388, 3383, 3384, 3389}
	for _, id := range stock {
		items[id] = domain.ItemDef{ID: id, Price: 1}
	}
	for id := domain.ItemID(3020); id <= 3031; id++ {
		stock = append(stock, id)
	}
	rows := &v5SingleUserRows{at: -1}
	for _, shop := range []int{1003, 1303, 1403, 1503, 1903, 2102, 2602, 3003} {
		for slot, id := range stock {
			raw, _ := json.Marshal([]any{shop, true, true, 10000, 30000, 0, "inferred", slot, id})
			var values []json.RawMessage
			_ = json.Unmarshal(raw, &values)
			rows.values = append(rows.values, values)
		}
	}
	shops, count, err := LoadShops(context.Background(), petShopQuery{rows}, items)
	if err != nil {
		t.Fatal(err)
	}
	if count != 8*17 || len(shops) != 8 {
		t.Fatalf("shops=%d rows=%d", len(shops), count)
	}
	for id, shop := range shops {
		if !reflect.DeepEqual(shop.Items, stock) {
			t.Fatalf("shop %d: %v", id, shop.Items)
		}
	}
}

func TestPetSuppliesUpgradeScope(t *testing.T) {
	petSupplyFixture(t)
	raw, err := os.ReadFile(petSuppliesUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, guard := range []string{"BEGIN;", "COMMIT;", "ARRAY[1003,1101,1201]", "generate_series(3020,3031)", "WHERE NOT EXISTS", "i.price > 0", "name='道具商人清悠'", "name='道具商人杜若'", "stock_source_shop_id=1003", "RAISE EXCEPTION", "row_number() OVER (ORDER BY id)", "strpos(source_note,marker)=0"} {
		if !strings.Contains(sql, guard) {
			t.Fatalf("missing scope/replay guard %s", guard)
		}
	}
	for _, bad := range []string{"UPDATE game_items", "DELETE FROM", "UPDATE game_shops SET stock_source_shop_id=1003", "schema_migrations"} {
		if strings.Contains(sql, bad) {
			t.Fatal("unexpected destructive/broad update", bad)
		}
	}
}

func TestPetSuppliesUpgradePostgres(t *testing.T) {
	dsn := os.Getenv("FANTASY_PET_SHOP_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN required; SQL execution/idempotence not validated")
	}
	ctx := context.Background()
	raw, err := os.ReadFile(petSuppliesUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"normal", "partial", "bad_price", "bad_source", "bad_slots", "missing_item", "renamed_npc", "late_local_conflict"} {
		t.Run(scenario, func(t *testing.T) {
			items := petSupplyFixture(t)
			conn, err := pgx.Connect(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(ctx)
			// Connection-local tables shadow any real tables. No permanent/player writes.
			_, err = conn.Exec(ctx, `CREATE TEMP TABLE game_items(id INT PRIMARY KEY,name TEXT,price BIGINT);
 CREATE TEMP TABLE map_npcs(sell_list INT,name TEXT);
 CREATE TEMP TABLE game_shops(shop_id INT PRIMARY KEY,stock_source_shop_id INT,source_kind TEXT NOT NULL DEFAULT 'inferred',source_note TEXT NOT NULL DEFAULT '',allow_sell BOOL DEFAULT TRUE,allow_repair BOOL DEFAULT TRUE,repair_price_bp INT DEFAULT 10000,special_repair_price_bp INT DEFAULT 30000,special_repair_nianli INT DEFAULT 0);
 CREATE TEMP TABLE game_shop_items(shop_id INT REFERENCES game_shops,slot INT CHECK(slot>=0 AND slot<120),item_id INT,PRIMARY KEY(shop_id,slot),UNIQUE(shop_id,item_id));
 INSERT INTO game_shops(shop_id,stock_source_shop_id) VALUES(1001,NULL),(1003,NULL),(1101,1001),(1201,1001),(1301,1001),(1303,1003),(1403,1003),(1503,1003),(1903,1003),(2102,1003),(2602,1003),(3003,1003);
 INSERT INTO map_npcs VALUES(1101,'道具商人清悠'),(1201,'道具商人杜若');
 INSERT INTO game_shop_items VALUES(1001,0,3002),(1003,0,3382),(1003,1,3388),(1003,2,3383),(1003,3,3384),(1003,4,3389);
 INSERT INTO game_items VALUES(3002,'fixture',10),(3382,'fixture',1),(3388,'fixture',1),(3383,'fixture',1),(3384,'fixture',1),(3389,'fixture',1);`)
			if err != nil {
				t.Fatal(err)
			}
			for _, def := range items {
				if _, err = conn.Exec(ctx, "INSERT INTO game_items VALUES($1,$2,$3)", def.ID, def.Name, def.Price); err != nil {
					t.Fatal(err)
				}
			}
			mutations := map[string]string{"late_local_conflict": "INSERT INTO game_shop_items VALUES(1201,0,3002)", "partial": "INSERT INTO game_shop_items VALUES(1003,5,3023)", "bad_price": "UPDATE game_items SET price=0 WHERE id=3023", "bad_source": "UPDATE game_shops SET stock_source_shop_id=1001 WHERE shop_id=1303", "bad_slots": "UPDATE game_shop_items SET slot=9 WHERE shop_id=1003 AND slot=4", "missing_item": "DELETE FROM game_items WHERE id=3031", "renamed_npc": "UPDATE map_npcs SET name='renamed' WHERE sell_list=1101"}
			if mutation := mutations[scenario]; mutation != "" {
				if _, err = conn.Exec(ctx, mutation); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := func() string {
				var s string
				err := conn.QueryRow(ctx, `SELECT jsonb_build_array((SELECT jsonb_agg(t ORDER BY shop_id) FROM game_shops t),(SELECT jsonb_agg(t ORDER BY shop_id,slot) FROM game_shop_items t),(SELECT jsonb_agg(t ORDER BY id) FROM game_items t))::TEXT`).Scan(&s)
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			itemSnapshot := func() string {
				var result string
				if err := conn.QueryRow(ctx, "SELECT jsonb_agg(t ORDER BY id)::TEXT FROM game_items t").Scan(&result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			originalItems := itemSnapshot()
			before := snapshot()
			_, err = conn.Exec(ctx, string(raw))
			if scenario != "normal" && scenario != "partial" {
				if err == nil {
					t.Fatal("expected atomic rejection")
				}
				_, _ = conn.Exec(ctx, "ROLLBACK")
				if snapshot() != before {
					t.Fatal("failed upgrade changed state")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if itemSnapshot() != originalItems {
				t.Fatal("item definitions/prices changed")
			}
			after := snapshot()
			if _, err = conn.Exec(ctx, string(raw)); err != nil {
				t.Fatal(err)
			}
			if snapshot() != after {
				t.Fatal("replay not idempotent")
			}
			for _, id := range []domain.ItemID{3002, 3382, 3388, 3383, 3384, 3389} {
				items[id] = domain.ItemDef{ID: id, Price: 1}
			}
			shops, _, err := LoadShops(ctx, conn, items)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []int32{1003, 1303, 1403, 1503, 1903, 2102, 2602, 3003, 1101, 1201} {
				if id == 1101 || id == 1201 {
					if len(shops[id].Items) == 0 || shops[id].Items[0] != 3002 {
						t.Fatal("beginner shop original prefix changed")
					}
				} else {
					prefix := []domain.ItemID{3382, 3388, 3383, 3384, 3389}
					if len(shops[id].Items) < len(prefix) || !reflect.DeepEqual(shops[id].Items[:len(prefix)], prefix) {
						t.Fatal("capture tool prefix changed")
					}
				}
				want := 17
				if id == 1101 || id == 1201 {
					want = 13
				}
				if len(shops[id].Items) != want {
					t.Fatalf("shop%d count%d", id, len(shops[id].Items))
				}
				for item := domain.ItemID(3020); item <= 3031; item++ {
					if !shops[id].Contains(item) {
						t.Fatalf("shop%d missing%d", id, item)
					}
				}
			}
			if !reflect.DeepEqual(shops[1001].Items, []domain.ItemID{3002}) || !reflect.DeepEqual(shops[1301].Items, shops[1001].Items) {
				t.Fatal("unrelated shared grocery stock changed")
			}
		})
	}
}

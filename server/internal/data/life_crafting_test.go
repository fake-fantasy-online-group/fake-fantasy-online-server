package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

type lifeCraftQuery struct{ rows *v5SingleUserRows }

func (q lifeCraftQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "FROM game_craft_extra_recipes") || !strings.Contains(sql, "ORDER BY show_type, row_no") {
		return nil, fmt.Errorf("loader must preserve route ordering")
	}
	return q.rows, nil
}

// Pure loader contract test; PostgreSQL migration execution remains a separate gate.
func TestLifeFurnaceLoaderAllNativeRoutes(t *testing.T) {
	rows := &v5SingleUserRows{at: -1}
	items := ItemDefs{}
	add := func(row int, product, material, qty int32) {
		items[domain.ItemID(product)] = domain.ItemDef{ID: domain.ItemID(product)}
		items[domain.ItemID(material)] = domain.ItemDef{ID: domain.ItemID(material)}
		values := []any{row, product, 0, 0, 0, 0, 0, 0, 100, 0, 1, int64(qty)<<16 | int64(material), 0, 0, 0, 0, 1, 0}
		out := []json.RawMessage{}
		for _, v := range values {
			raw, _ := json.Marshal(v)
			out = append(out, raw)
		}
		rows.values = append(rows.values, out)
	}
	for r := 0; r < 28; r++ {
		for f := 0; f < 10; f++ {
			band := 1 + f/3
			add(5200+r*10+f, int32(1900028001+r), int32(4401+f), int32((50*(r%7+1)+band-1)/band))
		}
	}
	for o := 0; o < 10; o++ {
		for k := 0; k < 2; k++ {
			product, cost := int32(4501), 200
			if k == 1 {
				product, cost = 4478, 600
			}
			band := 1 + o/3
			add(5480+o*2+k, product, int32(4001+3*o), int32((cost+band-1)/band))
		}
	}
	table, err := LoadCraftRecipes(context.Background(), lifeCraftQuery{rows}, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Recipes(0)) != 300 || len(table.ByProduct[0]) != 30 {
		t.Fatal("missing routes/products")
	}
	for id, routes := range table.ByProduct[0] {
		if len(routes) != 10 {
			t.Fatal(id, len(routes))
		}
		for _, r := range routes {
			if r.SuccessRate != 100 || r.Cost != 0 || len(r.Materials) != 1 || r.Materials[0].Qty <= 0 {
				t.Fatalf("invalid route %+v", r)
			}
		}
	}
	low := table.ProductRecipes(0, domain.FirstAstralRune)
	high := table.ProductRecipes(0, domain.FirstAstralRune+6)
	if low[0].Materials[0].Qty != 50 || high[0].Materials[0].Qty != 350 || high[9].Materials[0].Qty != 88 {
		t.Fatal("grade/band costs differ")
	}
}
func TestLifeCraftMigrationKeepsFreshAndUpgradeIdentical(t *testing.T) {
	seed, err := os.ReadFile("../../migrations/000000_init-116.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000116_20261027_life_crafting.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(upgrade) != "BEGIN;\n"+string(seed)+"COMMIT;\n" {
		t.Fatal("non-atomic/different upgrade")
	}
	for _, fragment := range []string{"id IN (1003,1006,1010,1011,1012)", "'life.salvage.normal'", "'life.salvage.unique'", "generate_series(0,27)", "generate_series(0,9)", "(13201,1,4501,30)", "(13202,1,4478,10)", "(13101,1,1900028001,3)"} {
		if !strings.Contains(string(seed), fragment) {
			t.Fatal("missing migration rule", fragment)
		}
	}
	for _, bad := range []string{"UPDATE char_", "DELETE FROM char_", "UPDATE game_rift_rule_versions", "UPDATE game_loot_rule_versions"} {
		if strings.Contains(string(seed), bad) {
			t.Fatal("mutates owned/historical state", bad)
		}
	}
}

// All writes below target connection-local temporary tables, never a live schema.
func TestLifeCraftMigrationPostgresAtomicReplay(t *testing.T) {
	dsn := os.Getenv("FANTASY_LIFE_CRAFT_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN required; SQL execution not yet verified")
	}
	ctx := context.Background()
	seed, _ := os.ReadFile("../../migrations/upgrades/000116_20261027_life_crafting.sql")
	cube, _ := os.ReadFile("../../migrations/000000_init-61.sql")
	cubeDDL := strings.Split(string(cube), "DO $migration$")[0]
	cubeDDL = strings.ReplaceAll(cubeDDL, "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE")
	craft, _ := os.ReadFile("../../migrations/000000_init-37.sql")
	start := strings.Index(string(craft), "CREATE TABLE IF NOT EXISTS game_craft_extra_recipes (")
	craftDDL := strings.Split(string(craft)[start:], "INSERT INTO game_craft_extra_recipes(")[0]
	craftDDL = strings.ReplaceAll(craftDDL, "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE")
	for _, collision := range []bool{false, true} {
		t.Run(fmt.Sprint("collision_", collision), func(t *testing.T) {
			conn, err := pgx.Connect(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(ctx)
			_, err = conn.Exec(ctx, `CREATE TEMP TABLE game_items(id INT PRIMARY KEY);
   INSERT INTO game_items SELECT generate_series(1900028001,1900028028);
   INSERT INTO game_items SELECT generate_series(1900028201,1900028203);
   INSERT INTO game_items SELECT generate_series(4401,4410);
   INSERT INTO game_items SELECT 4001+3*i FROM generate_series(0,9)i;
   INSERT INTO game_items VALUES(4501),(4478);`+cubeDDL+craftDDL+`
   ALTER TABLE game_cube_recipes ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;
   INSERT INTO game_cube_recipes VALUES
   (1001,'普通分解','salvage.normal','旧',true,0,0,0,0,1,true),
   (1002,'暗金分解','salvage.unique','旧',true,0,0,0,0,1,true),
   (1006,'开窍引星','socket.open','旧',true,0,0,0,1,0,true),
   (1010,'淬器成真','unique.awaken','旧',true,0,0,0,1,0,true);
   INSERT INTO game_cube_outputs VALUES(1001,1,1900028201,8,0,0,1),(1002,1,1900028203,1,0,0,1);
   INSERT INTO game_cube_materials VALUES(1006,1,1900028201,20),(1010,1,1900028203,4);`)
			if err != nil {
				t.Fatal(err)
			}
			if collision {
				_, err = conn.Exec(ctx, `INSERT INTO game_craft_extra_recipes(row_no,product_item_id)VALUES(5200,999)`)
				if err != nil {
					t.Fatal(err)
				}
			}
			snapshot := func() string {
				var out string
				err := conn.QueryRow(ctx, `SELECT jsonb_build_array((SELECT jsonb_agg(r ORDER BY id) FROM game_cube_recipes r),(SELECT jsonb_agg(m ORDER BY recipe_id,seq) FROM game_cube_materials m),(SELECT jsonb_agg(o ORDER BY recipe_id,seq) FROM game_cube_outputs o),(SELECT jsonb_agg(c ORDER BY row_no) FROM game_craft_extra_recipes c))::text`).Scan(&out)
				if err != nil {
					t.Fatal(err)
				}
				return out
			}
			before := snapshot()
			_, err = conn.Exec(ctx, string(seed))
			if collision {
				if err == nil {
					t.Fatal("expected collision rollback")
				}
				_, _ = conn.Exec(ctx, "ROLLBACK")
				if snapshot() != before {
					t.Fatal("failed migration changed data")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after := snapshot()
			_, err = conn.Exec(ctx, string(seed))
			if err != nil {
				t.Fatal(err)
			}
			if snapshot() != after {
				t.Fatal("migration replay changed state")
			}
			var n int
			err = conn.QueryRow(ctx, `SELECT count(*) FROM game_craft_extra_recipes`).Scan(&n)
			if err != nil || n != 300 {
				t.Fatal(n, err)
			}
		})
	}
}

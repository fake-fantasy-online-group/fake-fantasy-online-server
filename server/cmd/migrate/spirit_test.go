package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestSpirit120121InitUpgradeParityAndNoOwnedStateWrites(t *testing.T) {
	body, _, parts, err := loadInit("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	if parts < 122 {
		t.Fatalf("expected at least122 initializerparts, got%d", parts)
	}
	for _, p := range []struct {
		n    int
		name string
	}{{120, "spirit_instances"}, {121, "spirit_appearance_recipes"}} {
		init, err := os.ReadFile(fmt.Sprintf("../../migrations/000000_init-%d.sql", p.n))
		if err != nil {
			t.Fatal(err)
		}
		upgrade, err := os.ReadFile(fmt.Sprintf("../../migrations/upgrades/%06d_20261028_%s.sql", p.n, p.name))
		if err != nil {
			t.Fatal(err)
		}
		if string(upgrade) != "BEGIN;\n"+string(init)+"COMMIT;\n" {
			t.Fatalf("%d init/upgrade divergence", p.n)
		}
		if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK);`).Match(init) {
			t.Fatal("nested transaction")
		}
		if !bytes.Contains(body, init) {
			t.Fatalf("%d omitted from initializer", p.n)
		}
		if regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+(?:characters|character_items|item_instances|spirit_instances|equipment_spirit_fusions|equipment_wing_power_state|equipment_endgame_state)\b`).Match(init) {
			t.Fatal("migration must not rewrite owned state")
		}
	}
}
func TestSpirit121AppearanceRecipeCoverageAndGuards(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000000_init-121.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	rows := regexp.MustCompile(`(?m)^\((56\d\d),([^\n]+)\)[,;]$`).FindAllStringSubmatch(sql, -1)
	if len(rows) != 22 {
		t.Fatalf("expected22 routes got%d", len(rows))
	}
	products := map[int]int{}
	ids := map[int]bool{}
	for _, row := range rows {
		fields := strings.Split(row[1]+","+row[2], ",")
		if len(fields) != 18 {
			t.Fatal("recipe field count")
		}
		values := make([]int, len(fields))
		for i, s := range fields {
			values[i], err = strconv.Atoi(s)
			if err != nil {
				t.Fatal(err)
			}
		}
		if ids[values[0]] {
			t.Fatal("duplicate recipe")
		}
		ids[values[0]] = true
		products[values[1]]++
		if values[2] != 0 || values[8] != 100 || values[16] != 1 {
			t.Fatal("exchange must be fee-free, guaranteed, one output")
		}
		if values[1] == 42685 || values[1] == 42686 {
			gold := 42007
			if values[1] == 42686 {
				gold = 42011
			}
			if values[10] != 2 || values[11] != (1<<16)|gold || values[12] != (1<<16)|36322 {
				t.Fatal("dark upgrade must consume gold appearance, not double ingredients")
			}
		} else if values[10] != 3 || values[11]>>16 != 100 || values[12]>>16 != 20 || values[13]>>16 != 1 {
			t.Fatal("gold ingredient counts")
		}
	}
	expected := []int{13707, 13708, 14908, 14909, 14919, 14923, 25104, 25105, 42007, 42011, 42643, 42644, 42654, 42655, 42685, 42686, 42982, 42983, 43192, 43193}
	if len(products) != len(expected) {
		t.Fatal("unexpected product")
	}
	for _, id := range expected {
		if products[id] == 0 {
			t.Fatal("missing product", id)
		}
	}
	for _, id := range []int{43021, 43022, 43023, 43024, 43025, 43026, 43028, 43029, 43030, 43031, 43032, 43033, 43034, 43035, 43036, 43037, 43038, 43041, 43042, 43043, 43044} {
		if products[id] != 0 {
			t.Fatal("unsupported appearance", id)
		}
	}
	for _, guard := range []string{"JOIN gamedata.ov_combine", "IS DISTINCT FROM to_jsonb(s)", "ON CONFLICT (row_no) DO NOTHING"} {
		if !strings.Contains(sql, guard) {
			t.Fatal("missing collision/idempotency guard", guard)
		}
	}
}

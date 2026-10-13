package scene

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Explicit opt-in only: use a disposable migrated single-user test cluster.
// All production loader queries run in READ ONLY transactions; no socket,
// running game server, fixture replacement, or gameplay-row change is needed.
func TestSkillMechanismsPostgresKnockbackReachesSettlement(t *testing.T) {
	path := os.Getenv("FANTASY_SKILL_MECHANISMS_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated PostgreSQL data directory required")
	}
	bin := os.Getenv("PG_BIN")
	if bin == "" {
		t.Fatal("PG_BIN required for explicit single-user validation")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing a cluster with an active/stale postmaster.pid; use an idle disposable copy")
	}
	q := &skillMechanismsSingleUserQuerier{bin: bin, path: path, database: "postgres"}
	ctx := context.Background()
	skills, _, err := data.LoadSkills(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	for rank := int32(1); rank <= 5; rank++ {
		def, ok := skills.Get(10223, rank)
		if !ok {
			t.Fatalf("missing configured skill 10223/%d", rank)
		}
		var distance, chance int32
		rows, err := q.Query(ctx, fmt.Sprintf("SELECT flat_value,chance_bp FROM game_skill_effects WHERE skill_id=%d AND skill_level=%d AND effect_kind='knockback'", def.ID, rank))
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatal("configured knockback row missing")
		}
		if err := rows.Scan(&distance, &chance); err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			t.Fatal("unexpected duplicate knockback")
		}
		rows.Close()

		if distance <= 0 || chance != 10000 {
			t.Fatalf("test requires configured certain knockback, got distance=%d chance=%d", distance, chance)
		}
		for _, mode := range []string{"clear", "wall", "miss", "fatal", "immune"} {
			t.Run(fmt.Sprintf("rank%d/%s", rank, mode), func(t *testing.T) {
				s := newCombatScene(t)
				sink := join(t, s, 1, 1, "observer", 100, 100)
				src := putMonster(s, 800, 100000, sureHit, 100, 100)
				dst := putMonster(s, 801, 100000, domain.Stats{}, 120, 100)
				before := dst.Pos
				switch mode {
				case "wall":
					s.walkable = func(p domain.Pos) bool { return p.X <= before.X+30 }
				case "miss":
					src.Stats.Hit = 0
					dst.Stats.Def = 1 << 20
				case "fatal":
					dst.HP, dst.MaxHP = 1, 1
				case "immune":
					s.defs = spawn.MapDefs{dst.Monster.TypeID: {ID: dst.Monster.TypeID, ImmuneSkills: map[domain.SkillID]struct{}{def.ID: {}}}}
				}
				hp := dst.HP
				sink.take()
				landed := s.applySkill(src, dst, def, 0, false)
				s.flush()
				wantDistance := float64(distance)
				if mode == "wall" {
					wantDistance = math.Min(wantDistance, 30)
				}
				if mode == "miss" || mode == "fatal" || mode == "immune" {
					wantDistance = 0
				}
				if got := dst.Pos.X - before.X; got != wantDistance || dst.Pos.Y != before.Y {
					t.Fatalf("LoadSkills -> applySkill moved %.1f, want %.1f; HP %d -> %d; loaded effects %+v", got, wantDistance, hp, dst.HP, def.DirectEffects())
				}
				if mode == "miss" || mode == "immune" {
					if landed || dst.HP != hp {
						t.Fatalf("%s applied effects: landed=%v HP=%d -> %d", mode, landed, hp, dst.HP)
					}
				} else if !landed || dst.HP >= hp {
					t.Fatalf("damage failed before knockback: landed=%v HP=%d -> %d", landed, hp, dst.HP)
				}
				if mode == "fatal" && dst.Alive() {
					t.Fatal("fatal target stayed alive")
				}
				if wantDistance > 0 {
					moved := false
					for _, ev := range sink.take() {
						if v, ok := ev.(event.EntityMoved); ok && v.ID == dst.ID && v.Snap && v.To == dst.Pos {
							moved = true
						}
					}
					if !moved {
						t.Fatal("knockback omitted existing snap-move event")
					}
				}
			})
		}
	}
}

// Read-only adapter matching the established PostgreSQL loader tests.
type skillMechanismsSingleUserQuerier struct {
	bin, path, database string
	queries             int
}

func (q *skillMechanismsSingleUserQuerier) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("single-user test adapter does not interpolate SQL parameters")
	}
	q.queries++
	sql := "BEGIN READ ONLY;\n\nSELECT coalesce(json_agg(row_to_json(q)), '[]'::json)::text AS v5_result FROM (" + strings.TrimSuffix(strings.TrimSpace(query), ";") + ") q;\n\nROLLBACK;\n\n"
	cmd := exec.CommandContext(ctx, filepath.Join(q.bin, "postgres"), "--single", "-j", "-D", q.path, q.database)
	cmd.Stdin = strings.NewReader(sql)
	output, err := cmd.CombinedOutput()
	if err != nil || bytes.Contains(output, []byte("ERROR:")) || bytes.Contains(output, []byte("FATAL:")) {
		return nil, fmt.Errorf("read-only query%d: %v: %s", q.queries, err, output)
	}
	start := bytes.Index(output, []byte("v5_result = \""))
	if start < 0 {
		return nil, fmt.Errorf("query%d missing PostgreSQL result", q.queries)
	}
	output = output[start+len("v5_result = \""):]
	end := bytes.Index(output, []byte("\"\t(typeid"))
	if end < 0 {
		return nil, fmt.Errorf("query%d truncated PostgreSQL result", q.queries)
	}
	dec := json.NewDecoder(bytes.NewReader(output[:end]))
	token, err := dec.Token()
	if err != nil || token != json.Delim('[') {
		return nil, fmt.Errorf("query%d JSON array: %v", q.queries, err)
	}
	result := &skillMechanismsSingleUserRows{at: -1}
	for dec.More() {
		if _, err = dec.Token(); err != nil {
			return nil, err
		}
		var row []json.RawMessage
		// Keep column ORDER, including duplicate names from COALESCE expressions.
		// Decoding into a map would lose duplicate columns and falsify Scan results.
		for dec.More() {
			if _, err = dec.Token(); err != nil {
				return nil, err
			}
			var raw json.RawMessage
			if err = dec.Decode(&raw); err != nil {
				return nil, err
			}
			row = append(row, raw)
		}
		if _, err = dec.Token(); err != nil {
			return nil, err
		}
		result.values = append(result.values, row)
	}
	return result, nil
}

type skillMechanismsSingleUserRows struct {
	values [][]json.RawMessage
	at     int
}

func (r *skillMechanismsSingleUserRows) Close()     {}
func (r *skillMechanismsSingleUserRows) Err() error { return nil }
func (r *skillMechanismsSingleUserRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT")
}
func (r *skillMechanismsSingleUserRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *skillMechanismsSingleUserRows) Next() bool                                   { r.at++; return r.at < len(r.values) }
func (r *skillMechanismsSingleUserRows) Scan(dest ...any) error {
	if r.at < 0 || r.at >= len(r.values) {
		return fmt.Errorf("Scan outside row")
	}
	if len(dest) != len(r.values[r.at]) {
		return fmt.Errorf("SQL row width%d != destination%d", len(r.values[r.at]), len(dest))
	}
	for i, v := range r.values[r.at] {
		if dest[i] != nil {
			if err := json.Unmarshal(v, dest[i]); err != nil {
				return fmt.Errorf("column%d: %w", i, err)
			}
		}
	}
	return nil
}
func (r *skillMechanismsSingleUserRows) Values() ([]any, error) {
	if r.at < 0 || r.at >= len(r.values) {
		return nil, fmt.Errorf("Values outside row")
	}
	out := make([]any, len(r.values[r.at]))
	for i, v := range r.values[r.at] {
		if err := json.Unmarshal(v, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (r *skillMechanismsSingleUserRows) RawValues() [][]byte {
	if r.at < 0 || r.at >= len(r.values) {
		return nil
	}
	out := make([][]byte, len(r.values[r.at]))
	for i, v := range r.values[r.at] {
		out[i] = v
	}
	return out
}
func (r *skillMechanismsSingleUserRows) Conn() *pgx.Conn { return nil }

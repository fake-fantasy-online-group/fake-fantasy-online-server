package data

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Normal CI uses the existing testPool/DSN. A socket-free executor may explicitly
// opt into a COPIED, idle, disposable cluster with FANTASY_V5_SKILL_PG_DATA and
// PG_BIN. No daemon is started, no cluster is created, and every SELECT executes
// inside BEGIN READ ONLY; ROLLBACK. Never point this at a production cluster.
func TestV5SkillCurvesActualReadOnlyLoader(t *testing.T) {
	ctx := context.Background()
	var reader Querier
	if path := os.Getenv("FANTASY_V5_SKILL_PG_DATA"); path != "" {
		bin := os.Getenv("PG_BIN")
		if bin == "" {
			t.Fatal("PG_BIN required for explicit single-user validation")
		}
		if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
			t.Fatal("refusing a cluster with an active/stale postmaster.pid; use an idle disposable copy")
		}
		database := os.Getenv("FANTASY_V5_SKILL_PG_DATABASE")
		if database == "" {
			database = "fresh_test"
		}
		reader = &v5SingleUserQuerier{bin: bin, path: path, database: database}
	} else {
		reader = testPool(t)
	}
	table, stats, err := LoadSkills(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 750 || stats.Loaded != 750 {
		t.Fatalf("actual original class rows %+v", stats)
	}
	statuses, statusStats, err := LoadStatuses(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	overlay, _, err := LoadStatusOverlay(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = domain.ValidateSkillStatusRuntimeV5(table, statuses, overlay); err != nil {
		t.Fatal(err)
	}
	ranks := []int32{255, 256, 65535, 65536, 1000000}
	for r := int32(1); r <= 101; r++ {
		ranks = append(ranks, r)
	}
	for _, id := range domain.V5SkillCurveIDs() {
		first, fifth := table[domain.SkillKey{ID: id, Level: 1}], table[domain.SkillKey{ID: id, Level: 5}]
		for _, rank := range ranks {
			a, err := domain.EvaluateV5Skill(first, fifth, rank)
			if err != nil {
				t.Fatal(err)
			}
			b, err := domain.EvaluateV5Skill(first, fifth, rank+1)
			if err != nil {
				t.Fatal(err)
			}
			if v5LoadedOperationalCarrier(b) <= v5LoadedOperationalCarrier(a) {
				t.Fatalf("real skill%d has no consumed carrier growth rank%d->%d", id, rank, rank+1)
			}
			if a.ID != first.ID || a.Kind != first.Kind || a.Prof != first.Prof || a.TargetSelf != first.TargetSelf || a.TargetEnemy != first.TargetEnemy || a.TargetTeam != first.TargetTeam || a.RequiresInvisible != first.RequiresInvisible || a.RequiredWeaponType != first.RequiredWeaponType {
				t.Fatalf("real skill%d lost original identity/condition", id)
			}
			original, effective := first.DirectEffects(), a.DirectEffects()
			if len(effective) < len(original) {
				t.Fatalf("real skill%d removed direct effects", id)
			}
			for i, fx := range original {
				fy := effective[i]
				if fx.Kind != fy.Kind || fx.Target != fy.Target || fx.RequiresPreviousLanded != fy.RequiresPreviousLanded || fx.ClearHarmful != fy.ClearHarmful || fx.GuaranteedHit != fy.GuaranteedHit || !reflect.DeepEqual(fx.RemoveStatusIDs, fy.RemoveStatusIDs) {
					t.Fatalf("real skill%d changed ordered effect%d", id, i)
				}
			}
			for _, app := range a.Statuses {
				if _, ok := statuses.Get(app.ID, app.Level); !ok {
					t.Fatalf("real skill%d rank%d refers to missing state%d/%d", id, rank, app.ID, app.Level)
				}
			}
			for _, trigger := range a.PassiveTriggers {
				if _, ok := statuses.Get(trigger.Status, trigger.StatusLevel); !ok {
					t.Fatalf("real passive%d rank%d refers to missing trigger state%d/%d", id, rank, trigger.Status, trigger.StatusLevel)
				}
			}
		}
	}
	if dir := os.Getenv("FANTASY_V5_EXPORT_ANCHORS_DIR"); dir != "" {
		v5ExportReadOnlyAnchors(t, dir, table, statuses)
	}
	t.Logf("PASS actual read-only LoadSkills: %d class rows, %d battle skills, %d status definitions; adjacent ranks1..102,255/256,65535/65536,1000000/1000001", stats.Loaded, len(domain.V5SkillCurveIDs()), statusStats.Defs)
}

func v5LoadedOperationalCarrier(d domain.SkillDef) int64 {
	n := int64(d.ProgressionShieldFlat)
	for _, fx := range d.DirectEffects() {
		n += int64(fx.DamageFlat) + int64(fx.HealFlat) + int64(fx.RestoreMPFlat) + int64(fx.TauntPower)
	}
	for _, m := range d.PassiveModifiers {
		if m.Kind == domain.PassiveSkillDamageFlat || m.Kind == domain.PassiveEffectRestoreFlat {
			n += int64(m.Value)
		}
	}
	for _, m := range d.PassiveStats {
		if m.Mode == domain.ModeAbsolute {
			n += int64(m.Value)
		}
	}
	for _, m := range d.PassiveStatusAffixes {
		if m.Affix.Mode == domain.ModeAbsolute {
			n += int64(m.Affix.Value)
		}
	}
	for _, s := range d.Statuses {
		n += int64(s.ProgressionDamageShield) + int64(s.ProgressionReflectFlat) + int64(s.ProgressionAttackFlat)
		hp := int64(s.ProgressionHPPerTick)
		if hp < 0 {
			hp = -hp
		}
		n += hp
		for _, a := range append(append([]domain.Affix(nil), s.ExtraAffixes...), s.ProgressionAffixes...) {
			if a.Mode == domain.ModeAbsolute {
				n += int64(a.Value)
			}
		}
	}
	return n
}

type v5SingleUserQuerier struct {
	bin, path, database string
	queries             int
}

func (q *v5SingleUserQuerier) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
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
	result := &v5SingleUserRows{at: -1}
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

type v5SingleUserRows struct {
	values [][]json.RawMessage
	at     int
}

func (r *v5SingleUserRows) Close()                                       {}
func (r *v5SingleUserRows) Err() error                                   { return nil }
func (r *v5SingleUserRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT") }
func (r *v5SingleUserRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *v5SingleUserRows) Next() bool                                   { r.at++; return r.at < len(r.values) }
func (r *v5SingleUserRows) Scan(dest ...any) error {
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
func (r *v5SingleUserRows) Values() ([]any, error) {
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
func (r *v5SingleUserRows) RawValues() [][]byte {
	if r.at < 0 || r.at >= len(r.values) {
		return nil
	}
	out := make([][]byte, len(r.values[r.at]))
	for i, v := range r.values[r.at] {
		out[i] = v
	}
	return out
}
func (r *v5SingleUserRows) Conn() *pgx.Conn { return nil }

func v5ExportReadOnlyAnchors(t *testing.T, dir string, table domain.SkillTable, statuses domain.StatusTable) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	type pair struct {
		ID    domain.SkillID  `json:"id"`
		First domain.SkillDef `json:"first"`
		Fifth domain.SkillDef `json:"fifth"`
	}
	anchors := []pair{}
	for _, id := range domain.V5SkillCurveIDs() {
		a, b := table[domain.SkillKey{ID: id, Level: 1}], table[domain.SkillKey{ID: id, Level: 5}]
		a.FormulaVersion = 0
		b.FormulaVersion = 0
		anchors = append(anchors, pair{id, a, b})
	}
	allStatuses := make([]domain.StatusDef, 0, len(statuses))
	for _, d := range statuses {
		allStatuses = append(allStatuses, d)
	}
	sort.Slice(allStatuses, func(i, j int) bool {
		if allStatuses[i].ID != allStatuses[j].ID {
			return allStatuses[i].ID < allStatuses[j].ID
		}
		return allStatuses[i].Level < allStatuses[j].Level
	})
	hashes := map[string]string{}
	for name, value := range map[string]any{"skill-legacy-anchors.json": anchors, "skill-status-anchors.json": allStatuses} {
		full, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err = json.Unmarshal(full, &decoded); err != nil {
			t.Fatal(err)
		}
		blob, err := json.MarshalIndent(v5OmitZeroFields(decoded), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		blob = append(blob, '\n')
		if err = os.WriteFile(filepath.Join(dir, name), blob, 0644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(blob)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	manifest := map[string]any{"source": "Actual LoadSkills and LoadStatuses via read-only PostgreSQL queries against isolated seeded test cluster", "skill_count": len(anchors), "status_definition_count": len(allStatuses), "sha256": hashes, "reproduce": "FANTASY_V5_SKILL_PG_DATA=<idle disposable copied cluster> PG_BIN=<trusted PostgreSQL bin> FANTASY_V5_EXPORT_ANCHORS_DIR=<output> go test ./internal/data -run '^TestV5SkillCurvesActualReadOnlyLoader$' -count=1 -v", "balance_status": "Structural and numeric regression inputs, not gameplay balance certification"}
	blob, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "skill-anchor-provenance.json"), append(blob, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
func v5OmitZeroFields(value any) any {
	switch x := value.(type) {
	case []any:
		for i, v := range x {
			x[i] = v5OmitZeroFields(v)
		}
		return x
	case map[string]any:
		out := map[string]any{}
		for k, v := range x {
			zero := v == nil
			switch z := v.(type) {
			case bool:
				zero = !z
			case float64:
				zero = z == 0
			case string:
				zero = z == ""
			case []any:
				zero = len(z) == 0
			case map[string]any:
				zero = len(z) == 0
			}
			if !zero {
				out[k] = v5OmitZeroFields(v)
			}
		}
		return out
	default:
		return value
	}
}

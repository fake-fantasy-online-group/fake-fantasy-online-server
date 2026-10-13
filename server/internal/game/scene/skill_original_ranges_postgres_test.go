package scene

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

// The original numeric radius is restored only in this read-only query adapter,
// allowing an exact before/after payload comparison without rewriting fixtures.
type originalRangeBeforeQuery struct{ data.Querier }

func (q originalRangeBeforeQuery) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	query = strings.ReplaceAll(query, "sd.radius", "CASE WHEN sd.skill_id IN (10028,10127,10313) THEN 200 ELSE sd.radius END")
	return q.Querier.Query(ctx, query, args...)
}

func originalRangeFixture(t *testing.T) (domain.SkillTable, domain.SkillTable, domain.StatusTable) {
	t.Helper()
	path := os.Getenv("FANTASY_ORIGINAL_RANGES_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration68 PostgreSQL directory required")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing active/stale-postmaster cluster")
	}
	q := &skillMechanismsSingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	skills, _, err := data.LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := data.LoadSkills(context.Background(), originalRangeBeforeQuery{q})
	if err != nil {
		t.Fatal(err)
	}
	statuses, _, err := data.LoadStatuses(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return skills, before, statuses
}

func TestOriginalSkillRangesPostgres(t *testing.T) {
	skills, before, statuses := originalRangeFixture(t)
	t.Run("only-reviewed-payload-delta", func(t *testing.T) {
		changes := 0
		for _, id := range skills.ProgressionCatalog().IDs() {
			for _, rank := range []int32{1, 2, 3, 4, 5, 20, 21, 60, 255, 65536} {
				got, ok := skills.Get(id, rank)
				if !ok {
					t.Fatal("missing corrected skill", id, rank)
				}
				old, ok := before.Get(id, rank)
				if !ok {
					t.Fatal("missing prior skill", id, rank)
				}
				a, err := json.Marshal(got)
				if err != nil {
					t.Fatal(err)
				}
				b, err := json.Marshal(old)
				if err != nil {
					t.Fatal(err)
				}
				if string(a) == string(b) {
					continue
				}
				if id != 10028 && id != 10127 && id != 10313 {
					t.Fatalf("unrelated payload changed %d/%d", id, rank)
				}
				changes++
				got.Radius, got.Area.Radius = old.Radius, old.Area.Radius
				a, err = json.Marshal(got)
				if err != nil {
					t.Fatal(err)
				}
				if string(a) != string(b) {
					t.Fatalf("non-radius payload changed %d/%d", id, rank)
				}
			}
		}
		if changes != 30 {
			t.Fatalf("changed %d payloads, want exactly 3 skills x 10 golden ranks", changes)
		}
	})
	for _, id := range []domain.SkillID{10028, 10127, 10313} {
		base := int32(100)
		center := domain.SkillAreaCenterCaster
		if id == 10313 {
			base, center = 150, domain.SkillAreaCenterTarget
		}
		for _, rank := range []int32{1, 2, 3, 4, 5, 6, 20, 21, 55, 56, 60, 100, 255, 65536} {
			t.Run(fmt.Sprintf("skill%d/rank%d", id, rank), func(t *testing.T) {
				def, ok := skills.Get(id, rank)
				if !ok {
					t.Fatal("missing skill")
				}
				want := base
				if id != 10313 && rank > 5 {
					growth := rank - 5
					if growth <= 50 {
						want = base * (50 + growth) / 50
					} else {
						want = int32(math.Floor(float64(base) * 2 * math.Sqrt(float64(growth)/50)))
					}
				}
				if def.Radius != want || def.Area.Radius != want || def.Area.Center != center || def.Area.Shape != domain.SkillAreaCircle {
					t.Fatalf("wrong restored geometry: radius%d area%+v want radius%d center%v", def.Radius, def.Area, want, center)
				}
				authored := skills[domain.SkillKey{ID: id, Level: min(rank, 5)}]
				if def.Description != authored.Description {
					t.Fatal("original description mutated")
				}
				s := newCombatScene(t)
				s.skills, s.statuses = skills, statuses
				display := s.skillOriginalEffectLines(def)
				if len(display) == 0 {
					t.Fatal("missing description")
				}
				if rank <= 5 && display[0] != authored.Description {
					t.Fatalf("original prose changed: %q vs %q", display[0], authored.Description)
				}
				if !strings.Contains(display[0], fmt.Sprintf("半径%d", want)) {
					t.Fatalf("display radius differs from runtime: %q want%d", display[0], want)
				}
				// The largest rank checks representation/continuation only, avoiding an
				// unnecessarily enormous synthetic AOI search in the boundary regression.
				if rank == 65536 {
					return
				}
				join(t, s, 1, 1, "caster", 1000, 1000)
				caster := s.players[1]
				cx, cy := caster.Pos.X, caster.Pos.Y
				targetID := domain.EntityID(0)
				var at *domain.Pos
				if center == domain.SkillAreaCenterTarget {
					cx += 300
					targetID = 800
					putMonster(s, targetID, 1000, domain.Stats{}, cx, cy)
					position := domain.Pos{MapID: 7, X: cx, Y: cy}
					at = &position
				}
				r := float64(want)
				// Exact cardinal edge, just outside, and diagonal points prevent a
				// diameter interpretation or a bounding-square-only inclusion shortcut.
				offsets := [][2]float64{{r - 1, 0}, {r, 0}, {r + 1, 0}, {0, r}, {0, r + 1}, {r * 0.7, r * 0.7}, {r * 0.8, r * 0.8}, {-r, 0}, {0, -r}}
				expected := []domain.EntityID{}
				if targetID != 0 {
					expected = append(expected, targetID)
				}
				for i, d := range offsets {
					mid := domain.EntityID(900 + i)
					putMonster(s, mid, 1000, domain.Stats{}, cx+d[0], cy+d[1])
					if d[0]*d[0]+d[1]*d[1] <= r*r {
						expected = append(expected, mid)
					}
				}
				dead := putMonster(s, 999, 0, domain.Stats{}, cx, cy)
				_ = dead
				got := s.resolveTargets(caster, def, targetID, nil)
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("actual AOI targets %v want %v for radius%d", got, expected, want)
				}
				if at != nil {
					if got := s.resolveTargets(caster, def, 0, at); !reflect.DeepEqual(got, expected) {
						t.Fatalf("ground-center AOI targets %v want %v", got, expected)
					}
				} else {
					// Self-centered circles must ignore a distant selected target and point.
					far := putMonster(s, 2000, 1000, domain.Stats{}, cx+r+400, cy)
					if got := s.resolveTargets(caster, def, far.ID, &far.Pos); !reflect.DeepEqual(got, expected) {
						t.Fatalf("self-centered area moved to selected target: %v want%v", got, expected)
					}
				}
			})
		}
	}
}

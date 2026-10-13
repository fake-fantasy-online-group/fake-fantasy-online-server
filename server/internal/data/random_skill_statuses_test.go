package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

type randomSkillStatusQuery struct{ rows [][]json.RawMessage }

func (q randomSkillStatusQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "FROM game_skill_statuses") {
		return nil, fmt.Errorf("unexpected random status query")
	}
	return &v5SingleUserRows{at: -1, values: q.rows}, nil
}

func TestLoadRandomStatusPool(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		valid               bool
	}{
		{"valid", `[10313,1,1,1006,1,4000,6,"",false,"any",true,1,1]`, `[10313,1,2,1008,2,4000,6,"",false,"any",true,1,3]`, true},
		{"zero weight", `[10313,1,1,1006,1,4000,6,"",false,"any",true,1,0]`, "", false},
		{"mixed chances", `[10313,1,1,1006,1,4000,6,"",false,"any",true,1,1]`, `[10313,1,2,1008,2,3000,6,"",false,"any",true,1,1]`, false},
		{"missing state", `[10313,1,1,1006,1,4000,6,"",false,"any",false,1,1]`, "", false},
		{"duplicate identity", `[10313,1,1,1006,1,4000,6,"",false,"any",true,1,1]`, `[10313,1,2,1006,2,4000,6,"",false,"any",true,1,1]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q randomSkillStatusQuery
			for _, raw := range []string{tc.first, tc.second} {
				if raw == "" {
					continue
				}
				var row []json.RawMessage
				if err := json.Unmarshal([]byte(raw), &row); err != nil {
					t.Fatal(err)
				}
				q.rows = append(q.rows, row)
			}
			got, err := loadStructuredSkillStatuses(context.Background(), q)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid {
				apps := got[domain.SkillKey{ID: 10313, Level: 1}]
				if len(apps) != 2 || apps[1].RandomWeight != 3 || apps[0].RandomGroup != 1 || apps[0].ChanceBP != 4000 || !apps[0].ChanceExplicit {
					t.Fatalf("lost pool config: %+v", apps)
				}
			}
		})
	}
}

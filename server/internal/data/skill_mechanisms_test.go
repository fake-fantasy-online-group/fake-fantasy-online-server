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

// This scanner-level regression is always run; the corresponding scene test
// also verifies actual PostgreSQL LoadSkills -> damage -> world displacement.
type skillMechanismQuery struct{ row []json.RawMessage }

func (q skillMechanismQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "FROM game_skill_effect_remove_statuses") {
		return &v5SingleUserRows{at: -1}, nil
	}
	if strings.Contains(sql, "FROM game_skill_effects") {
		return &v5SingleUserRows{at: -1, values: [][]json.RawMessage{q.row}}, nil
	}
	return nil, fmt.Errorf("unexpected skill mechanism query: %s", sql)
}

func TestSkillMechanismsLoadKnockbackChance(t *testing.T) {
	var row []json.RawMessage
	if err := json.Unmarshal([]byte(`[10223,1,2,"knockback",0,0,137,false,10000,"target",true,0,0]`), &row); err != nil {
		t.Fatal(err)
	}
	effects, err := loadStructuredSkillEffects(context.Background(), skillMechanismQuery{row: row})
	if err != nil {
		t.Fatal(err)
	}
	got := effects[domain.SkillKey{ID: 10223, Level: 1}]
	if len(got) != 1 {
		t.Fatalf("effects = %+v", got)
	}
	fx := got[0]
	if fx.Kind != domain.EffectKnockback || fx.KnockbackDistance != 137 || fx.ChanceBP != 10000 || !fx.ChanceExplicit || !fx.RequiresPreviousLanded {
		t.Fatalf("configured knockback payload lost during loading: %+v", fx)
	}
}

type skillMechanismStatusQuery struct{ row []json.RawMessage }

func (q skillMechanismStatusQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "FROM game_skill_statuses") {
		return nil, fmt.Errorf("unexpected scoped status query")
	}
	return &v5SingleUserRows{at: -1, values: [][]json.RawMessage{q.row}}, nil
}

func TestSkillMechanismsLoadScopedStatusTargets(t *testing.T) {
	for _, target := range []string{"any", "player", "non_player", "invalid"} {
		var row []json.RawMessage
		raw := fmt.Sprintf(`[10317,1,1,1008,2,10000,60,"",false,%q,true]`, target)
		if err := json.Unmarshal([]byte(raw), &row); err != nil {
			t.Fatal(err)
		}
		apps, err := loadStructuredSkillStatuses(context.Background(), skillMechanismStatusQuery{row: row})
		if target == "invalid" {
			if err == nil {
				t.Fatal("invalid target accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got := apps[domain.SkillKey{ID: 10317, Level: 1}]
		want := domain.SkillStatusTarget(target)
		if target == "any" {
			want = domain.SkillStatusTargetAny
		}
		if len(got) != 1 || got[0].TargetType != want || !got[0].ChanceExplicit || got[0].ChanceBP != 10000 {
			t.Fatalf("scoped payload mismatch: %+v", got)
		}
	}
}

type skillMechanismStrengthQuery struct{ rows [][]json.RawMessage }

func (q skillMechanismStrengthQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "to_regclass") {
		return &v5SingleUserRows{at: -1, values: [][]json.RawMessage{{json.RawMessage(`true`)}}}, nil
	}
	if strings.Contains(sql, "FROM game_status_overlay_strengths") {
		return &v5SingleUserRows{at: -1, values: q.rows}, nil
	}
	return nil, fmt.Errorf("unexpected strength query")
}
func TestSkillMechanismsLoadOverlayStrengthValidation(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		affixes   []domain.Affix
		valid     bool
	}{
		{"valid", `[1008,19,1,-1]`, []domain.Affix{{Attr: 19, Mode: 1, Value: -18}}, true},
		{"higher", `[1008,19,1,1]`, []domain.Affix{{Attr: 19, Mode: 1, Value: 18}}, true},
		{"zero order", `[1008,19,1,0]`, nil, false},
		{"unknown order", `[1008,19,1,2]`, nil, false},
		{"unknown mode", `[1008,19,2,-1]`, nil, false},
		{"unknown attr", `[1008,999,1,-1]`, nil, false},
		{"missing status", `[1009,19,1,-1]`, nil, false},
		{"missing attr", `[1008,19,1,-1]`, nil, false},
		{"duplicate attr", `[1008,19,1,-1]`, []domain.Affix{{Attr: 19, Mode: 1, Value: -10}, {Attr: 19, Mode: 1, Value: -10}}, false},
		{"mixed modes", `[1008,19,1,-1]`, []domain.Affix{{Attr: 19, Mode: 1, Value: -10}, {Attr: 19, Mode: 0, Value: -10}}, false},
		{"wrong mode", `[1008,19,1,-1]`, []domain.Affix{{Attr: 19, Mode: 0, Value: -10}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var row []json.RawMessage
			if err := json.Unmarshal([]byte(tc.raw), &row); err != nil {
				t.Fatal(err)
			}
			table := domain.StatusTable{{ID: 1008, Level: 25}: {ID: 1008, Level: 25, Affixes: tc.affixes}}
			err := loadStatusOverlayStrengths(context.Background(), skillMechanismStrengthQuery{rows: [][]json.RawMessage{row}}, table)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid && table[domain.StatusKey{ID: 1008, Level: 25}].OverlayStrength == nil {
				t.Fatal("rule lost")
			}
			if tc.valid && loadStatusOverlayStrengths(context.Background(), skillMechanismStrengthQuery{rows: [][]json.RawMessage{row, row}}, table) == nil {
				t.Fatal("duplicate selector accepted")
			}
		})
	}
}

type skillMechanismLogicalLevelQuery struct{ row []json.RawMessage }

func (q skillMechanismLogicalLevelQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "FROM game_custom_status_affixes") {
		return &v5SingleUserRows{at: -1}, nil
	}
	if strings.Contains(sql, "FROM game_custom_statuses") {
		return &v5SingleUserRows{at: -1, values: [][]json.RawMessage{q.row}}, nil
	}
	return nil, fmt.Errorf("unexpected custom status query")
}
func TestSkillMechanismsLoadLogicalLevelBounds(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{{"0", true}, {"36000", true}, {"9223372036854775807", true}, {"-1", false}, {"9223372036854775808", false}} {
		var row []json.RawMessage
		raw := fmt.Sprintf(`[1008,21,"致盲","命中下降10%%",1,8,0,0,0,0,0,0,0,0,null,false,false,false,0,false,7074,true,false,%s]`, tc.value)
		if err := json.Unmarshal([]byte(raw), &row); err != nil {
			t.Fatal(err)
		}
		table := domain.StatusTable{}
		err := loadCustomStatuses(context.Background(), skillMechanismLogicalLevelQuery{row: row}, table)
		if (err == nil) != tc.valid {
			t.Fatalf("logical level %s valid=%v error=%v", tc.value, tc.valid, err)
		}
	}
}

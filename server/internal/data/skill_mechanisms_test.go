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

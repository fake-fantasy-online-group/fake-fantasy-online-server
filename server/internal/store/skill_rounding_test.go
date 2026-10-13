package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

type roundingSaveQuery struct {
	querier
	calls int
	sql   string
	args  []any
	err   error
}

func (q *roundingSaveQuery) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q.calls++
	q.sql, q.args = sql, append([]any(nil), args...)
	return pgconn.NewCommandTag("UPDATE 1"), q.err
}

type roundingCharRow struct{ raw []byte }

func (r roundingCharRow) Scan(dest ...any) error {
	// The existing independently encoded status/cooldown columns precede the
	// appended credit column. All other scalar fields are their zero defaults.
	for _, v := range dest {
		if b, ok := v.(*[]byte); ok {
			*b = []byte("[]")
		}
	}
	*dest[len(dest)-3].(*[]byte) = []byte("{}")
	*dest[len(dest)-1].(*[]byte) = r.raw
	return nil
}

func TestSkillRoundingCharacterSaveLoad(t *testing.T) {
	credits := map[domain.SkillID]domain.SkillRoundingState{
		10018: {Mana: domain.SkillRoundingCredit{Numerator: 37, Denominator: 146},
			Cooldown: domain.SkillRoundingCredit{Numerator: 41, Denominator: 14600}},
		10432: {Mana: domain.SkillRoundingCredit{Numerator: 2, Denominator: 149}},
	}
	c := &domain.Character{ID: 7, SkillRoundingCredits: credits}
	q := &roundingSaveQuery{}
	if err := saveChar(context.Background(), q, c); err != nil {
		t.Fatal(err)
	}
	if q.calls != 1 || len(q.args) != 60 || !strings.Contains(q.sql, "skill_rounding_credits=$60::jsonb") {
		t.Fatalf("credit was not persisted atomically with character: calls=%d args=%d", q.calls, len(q.args))
	}
	if !strings.HasSuffix(strings.TrimSpace(charCols), "skill_rounding_credits") {
		t.Fatal("SELECT order mismatch")
	}
	raw := []byte(q.args[59].(string))
	loaded, err := scanChar(roundingCharRow{raw})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(credits, loaded.SkillRoundingCredits) {
		t.Fatalf("lost saved fractions: %+v", loaded.SkillRoundingCredits)
	}
	clone := loaded.Clone()
	delete(clone.SkillRoundingCredits, 10018)
	if len(loaded.SkillRoundingCredits) != 2 {
		t.Fatal("character clone shares credit map")
	}
	if err := saveChar(context.Background(), q, &domain.Character{ID: 8}); err != nil {
		t.Fatal(err)
	}
	if q.args[59] != "{}" {
		t.Fatalf("new character default %v", q.args[59])
	}
	if _, err := scanChar(roundingCharRow{[]byte("{}")}); err != nil {
		t.Fatal(err)
	}
}

func TestSkillRoundingCorruptionDoesNotResetCredit(t *testing.T) {
	bad := []string{"", "null", "[]", "{} {}", `{"10018":null}`, `{"10018":{"mana":null}}`,
		`{"10018":{"mana":{"n":37,"d":146}},"010018":{}}`, `{"+10018":{}}`,
		`{"10018":{"mana":{"n":37,"d":146,"N":0}}}`, `{"10018":{"Mana":{}}}`,
		`{"10018":{"mana":{"n":null,"d":146}}}`, `{"10018":{"cooldown":{"n":0,"d":null}}}`,
		`{"10018":{"mana":{"n":1,"d":0}}}`,
		`{"10018":{"mana":{"n":-1,"d":10}}}`, `{"10018":{"mana":{"n":10,"d":10}}}`,
		`{"10018":{"mana":{"n":1,"d":9223372036854775807}}}`,
		`{"10018":{"mana":{"n":1,"d":10,"unknown":0}}}`, `{"999":{"mana":{"n":1,"d":10}}}`}
	for _, raw := range bad {
		t.Run(raw, func(t *testing.T) {
			if _, err := scanChar(roundingCharRow{[]byte(raw)}); err == nil {
				t.Fatal("corrupt credit accepted")
			}
		})
	}
	c := &domain.Character{ID: 7, SkillRoundingCredits: map[domain.SkillID]domain.SkillRoundingState{10018: {Mana: domain.SkillRoundingCredit{Numerator: 2, Denominator: 1}}}}
	q := &roundingSaveQuery{}
	if err := saveChar(context.Background(), q, c); err == nil || q.calls != 0 {
		t.Fatal("invalid credit reached database")
	}
	q.err = errors.New("transaction aborted")
	c.SkillRoundingCredits = nil
	if err := saveChar(context.Background(), q, c); !errors.Is(err, q.err) {
		t.Fatalf("save failure hidden: %v", err)
	}
	var old map[domain.SkillID]domain.SkillRoundingState
	if err := json.Unmarshal([]byte(`{"10018":{"mana":{"n":0,"d":0},"cooldown":{"n":0,"d":0}}}`), &old); err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateSkillRoundingCredits(old); err != nil {
		t.Fatal(err)
	}
}

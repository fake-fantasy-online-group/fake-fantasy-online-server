package data
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

// These are loader decision tests. Existing PostgreSQL integration tests remain
// responsible for checking the real schema and query column types.
type affixProvenanceRow struct {
	arm, attr, value, mode, probability int32
	named                               bool
}

type affixProvenanceRows struct {
	pgx.Rows
	rows   []affixProvenanceRow
	next   int
	closed bool
}

func (r *affixProvenanceRows) Close()     { r.closed = true }
func (r *affixProvenanceRows) Err() error { return nil }
func (r *affixProvenanceRows) Next() bool {
	if r.next >= len(r.rows) {
		return false
	}
	r.next++
	return true
}
func (r *affixProvenanceRows) Scan(dest ...any) error {
	if len(dest) != 6 || r.next < 1 || r.next > len(r.rows) {
		return fmt.Errorf("unexpected affix scan")
	}
	v := r.rows[r.next-1]
	for i, n := range []int32{v.arm, v.attr, v.value, v.mode, v.probability} {
		*dest[i].(*int32) = n
	}
	*dest[5].(*bool) = v.named
	return nil
}

type affixProvenanceQuery struct{ rows *affixProvenanceRows }

func (q affixProvenanceQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(strings.ToUpper(sql), "WHERE") || !strings.Contains(sql, "mode, prob,") {
		return nil, fmt.Errorf("query dropped unresolved source rows")
	}
	return q.rows, nil
}

func TestLoadAffixesPreservesUnknownSourceWithoutGrantingStats(t *testing.T) {
	items := ItemDefs{}
	for id := domain.ItemID(1); id <= 5; id++ {
		items[id] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Equip: &domain.EquipDef{Slot: 4}}
	}
	rows := &affixProvenanceRows{rows: []affixProvenanceRow{
		{1, domain.AttrSTR, 9, 0, 100, true},
		{2, domain.AttrCrit, 100, 0, 30, true},
		{3, 1401, 0, 1, 100, false},
		{4, domain.AttrINT, 9, 0, 100, true},
		{4, 772, 0, 0, 100, false},
	}}
	skipped, err := loadAffixes(context.Background(), affixProvenanceQuery{rows}, items)
	if err != nil || skipped != 3 || !rows.closed {
		t.Fatalf("loader skipped=%d closed=%v err=%v", skipped, rows.closed, err)
	}
	for id := domain.ItemID(1); id <= 5; id++ {
		def := items[id]
		wantUnknown := id >= 2 && id <= 4
		wantCount := 0
		if id == 1 || id == 4 {
			wantCount = 1
		}
		if def.Equip.UnknownInnateProperties != wantUnknown || len(def.Equip.Affixes) != wantCount {
			t.Fatalf("item%d got unknown=%v affixes=%+v", id, def.Equip.UnknownInnateProperties, def.Equip.Affixes)
		}
		if domain.NaturalAttributeCount(def, domain.Stack{}) != wantCount || domain.EquipmentSocketCapacity(def, domain.Stack{}) != 5-wantCount {
			t.Fatalf("item%d provenance changed live stats or socket budget", id)
		}
		if got := domain.AstralBaseEligible(def, domain.Stack{}); got != (id == 5) {
			t.Fatalf("item%d eligibility=%v", id, got)
		}
		if id == 2 || id == 3 {
			if reason := domain.AstralBaseIneligibility(def, domain.Stack{}); !strings.Contains(reason, "尚未确认") {
				t.Fatalf("unresolved source was presented as a confirmed affix: %s", reason)
			}
		}
	}
}

package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in read-only queries against an explicitly supplied idle disposable 118
// cluster. The fixed codex_test database is a clone, never a production DSN.
func TestPowerCodexCurrentPostgresLoaders(t *testing.T) {
	path := os.Getenv("FANTASY_CODEX_PG_DATA")
	if path == "" {
		t.Skip("explicit idle disposable PostgreSQL 118 clone required")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); !os.IsNotExist(err) {
		t.Fatal("cluster not demonstrably idle", err)
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "codex_test"}
	items, err := LoadItems(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []domain.ItemID{domain.PowerCodexItemID, domain.PowerScrollItemID} {
		d, ok := items[id]
		if !ok {
			t.Fatal("missing item", id)
		}
		if d.InstanceKind != domain.ItemInstanceEquipment || d.Equip != nil || d.Stackable || d.CanTrade || d.CanMail || d.ConsumeOnUse || !d.InventoryTabKnown {
			t.Fatalf("bad item contract %d: %+v", id, d)
		}
		st := domain.NewStack(d, 1)
		if st.UID <= 0 {
			t.Fatal("item omitted from endgame snapshot without UID", id)
		}
	}
	quests, err := LoadQuests(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ApplyQuestSteps(context.Background(), codexQuestQuerier{q}, quests); err != nil {
		t.Fatal(err)
	}
	quest, ok := quests[domain.PowerCodexQuestID]
	if !ok || quest.RecoveryItem != domain.PowerCodexItemID || quest.Repeatable || len(quest.Steps) != 1 || quest.Steps[0].Kind != domain.QuestStepTalk || quest.Steps[0].To != "巧匠左慈" || len(quest.Reward.Items) != 1 || quest.Reward.Items[0].Item != domain.PowerCodexItemID || quest.Reward.Items[0].Qty != 1 {
		t.Fatalf("bad quest contract: %+v", quest)
	}
	t.Log("real PostgreSQL118 LoadItems/LoadQuests: book and scroll have positive instance UIDs, no equipment behavior, correct binding-policy templates and quest reward")
}

// Only the fixed trusted base-layer argument is permitted by this test adapter.
type codexQuestQuerier struct{ inner *v5SingleUserQuerier }

func (q codexQuestQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if len(args) == 0 {
		return q.inner.Query(ctx, sql)
	}
	if len(args) != 1 || args[0] != "base" {
		return nil, fmt.Errorf("unexpected quest fixture parameters")
	}
	return q.inner.Query(ctx, strings.ReplaceAll(sql, "$1", "'base'"))
}

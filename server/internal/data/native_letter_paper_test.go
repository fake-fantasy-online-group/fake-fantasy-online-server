package data

import (
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestNativeLetterPaperProvenanceAndMigrationScope(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if domain.MailLetterPaperID != 3309 {
		t.Fatal("native paper identity changed")
	}
	item := read("../../migrations/000000_init-3.sql")
	if !strings.Contains(item, "(3309, '信纸', 1000, 1,") {
		t.Fatal("missing original paper item/price evidence")
	}
	help := read("../../migrations/000000_init-29.sql")
	for _, evidence := range []string{"寄送信件只需使用道具信纸", "耗费的信纸数量与群发的人数相对应", "左键双击物品栏里的特定道具"} {
		if !strings.Contains(help, evidence) {
			t.Fatal("missing native mail usage evidence", evidence)
		}
	}
	init := read("../../migrations/000000_init-127.sql")
	upgrade := read("../../migrations/upgrades/000127_20261029_native_letter_paper.sql")
	if strings.TrimSpace(upgrade) != "BEGIN;\n"+strings.TrimSpace(init)+"\n\nCOMMIT;" {
		t.Fatal("init/transactional upgrade drift")
	}
	for _, guard := range []string{"id=3309 AND name='信纸'", "stock_source_shop_id=1002", "VALUES(1002,next_slot,3309)", "slot<>expected", "next_slot>=120", "WHERE shop_id=1002 AND item_id=3309", "strpos(source_note,marker)=0"} {
		if !strings.Contains(init, guard) {
			t.Fatal("missing migration boundary", guard)
		}
	}
	for _, forbidden := range []string{"UPDATE game_items", "DELETE FROM", "UPDATE map_npcs", "schema_migrations", "VALUES(1002,next_slot,3310)"} {
		if strings.Contains(init, forbidden) {
			t.Fatal("unrequested price/stock/NPC change", forbidden)
		}
	}
}

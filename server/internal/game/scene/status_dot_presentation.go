package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Player-facing text is intentionally only the short mechanism paragraph.
// Name and live remaining time are rendered by the existing native status UI.
// Actual damage, snapshots, amplifiers and proc budgets belong in audit receipts,
// never this tooltip. No extra per-tick presentation broadcast is necessary.
func genericDOTDescription(a domain.Active) string {
	kind := map[string]string{domain.DOTBleed: "物理", domain.DOTPoison: "毒性", domain.DOTBurn: "火焰", domain.DOTRadiant: "光系"}[domain.GenericDOTFamily(a.Def)]
	return fmt.Sprintf("持续%s伤害，每%d秒结算一次。", kind, a.Def.IntervalSec)
}

func genericPublishedDOT(effects []event.EntityStatusEffect) bool {
	for _, effect := range effects {
		switch effect.Effect {
		case "流血", "灼光", "中毒", "燃烧", "燃烧1":
			return true
		}
	}
	return false
}

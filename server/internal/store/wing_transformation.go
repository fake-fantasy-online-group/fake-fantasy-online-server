package store

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// A narrow migration-authored edge is the only exception to immutable UID/item
// identity. The generic saveItemInstances UPSERT remains unchanged.
const transformWingInstanceSQL = `UPDATE item_instances i SET item_id=$4,bound=TRUE,updated_at=now()
 WHERE i.uid=$2 AND i.item_id=$3 AND i.instance_kind=1 AND NOT i.locked
 AND EXISTS(SELECT 1 FROM character_items b WHERE b.char_id=$1 AND b.uid=i.uid AND b.item_id=$3 AND b.count=1 AND NOT b.locked)
 AND EXISTS(SELECT 1 FROM game_wing_transform_edges e WHERE e.source_item=$3 AND e.target_item=$4 AND e.operation IN ('wing.upgrade','wing.change'))`

func (t *pgTx) TransformWingInstance(ctx context.Context, charID, uid int64, source, target domain.ItemID) error {
	tag, err := t.tx.Exec(ctx, transformWingInstanceSQL, charID, uid, int32(source), int32(target))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("wing instance ownership or transformation edge changed")
	}
	return nil
}
func (p *Postgres) TransformWingInstance(context.Context, int64, int64, domain.ItemID, domain.ItemID) error {
	return fmt.Errorf("wing transformation requires transaction")
}
func (m *Memory) TransformWingInstance(context.Context, int64, int64, domain.ItemID, domain.ItemID) error {
	return fmt.Errorf("wing transformation requires transaction")
}
func (t *txView) TransformWingInstance(ctx context.Context, charID, uid int64, source, target domain.ItemID) error {
	bag := t.m.bags[charID]
	if bag == nil {
		return fmt.Errorf("wing owner bag missing")
	}
	found := 0
	bag.Each(func(_ int, st domain.Stack) {
		if st.UID == uid && st.Item == source && st.Count == 1 && !st.Locked {
			found++
		}
	})
	if found != 1 || target <= 0 {
		return fmt.Errorf("invalid wing instance")
	}
	return nil
}
func wingTransformationOf(p domain.EndgameCommitPlan) (before, after domain.Stack, err error) {
	count := 0
	p.Before.Bag.Each(func(_ int, old domain.Stack) {
		if old.UID <= 0 {
			return
		}
		p.Snapshot.Bag.Each(func(_ int, next domain.Stack) {
			if next.UID == old.UID && next.Item != old.Item {
				count++
				before, after = old, next
			}
		})
	})
	if count == 0 {
		return
	}
	if count != 1 || before.Count != 1 || before.Locked || !after.Bound || (before.InstanceKind != domain.ItemInstanceNone && before.InstanceKind != domain.ItemInstanceEquipment) || after.InstanceKind != domain.ItemInstanceEquipment {
		return before, after, fmt.Errorf("invalid wing transformation plan")
	}
	preserved := after
	preserved.InstanceKind = before.InstanceKind
	preserved.Item = before.Item
	preserved.Bound = before.Bound
	preserved.Endgame.Revision = before.Endgame.Revision
	preserved.Durability = before.Durability
	preserved.MaxDurability = before.MaxDurability
	if preserved != before || after.Endgame.Revision != before.Endgame.Revision+1 {
		return before, after, fmt.Errorf("wing transformation lost instance state")
	}
	return
}

package store

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func validateEndgameItem(st domain.Stack) error {
	v := st.Endgame
	if v.UniqueID == 0 {
		if v.Revision != 0 || v.UniqueRollBP != 0 {
			return fmt.Errorf("endgame: invalid ordinary metadata")
		}
		return nil
	}
	if st.UID <= 0 || st.Count != 1 || (st.InstanceKind != domain.ItemInstanceNone && st.InstanceKind != domain.ItemInstanceEquipment) || v.UniqueID != int32(st.Item) || v.Revision < 0 || v.UniqueRollBP < 8000 || v.UniqueRollBP > 12000 {
		return fmt.Errorf("endgame: invalid unique metadata uid=%d", st.UID)
	}
	return nil
}
func validateEndgameSnapshot(s domain.Snapshot) error {
	var err error
	visit := func(st domain.Stack) {
		if err == nil {
			err = validateEndgameItem(st)
		}
	}
	s.Bag.Each(func(_ int, st domain.Stack) { visit(st) })
	s.Worn.Each(func(_ domain.EquipSlot, st domain.Stack) { visit(st) })
	s.ChangeSet.Each(func(_ domain.EquipSlot, st domain.Stack) { visit(st) })
	s.Warehouse.Each(func(_, _ int, st domain.Stack) { visit(st) })
	if s.Stall != nil {
		for _, v := range s.Stall.Items {
			visit(v.Stack)
		}
	}
	if s.Char != nil && (s.Char.Trial.KeyReward < 0 || s.Char.Trial.KeyReward > 14 || len(s.Char.Trial.CompletionID) > 64) {
		return fmt.Errorf("endgame: invalid frozen trial reward")
	}
	return err
}
func saveEndgameItem(ctx context.Context, q querier, st domain.Stack) error {
	if err := validateEndgameItem(st); err != nil {
		return err
	}
	if st.Endgame.UniqueID == 0 {
		_, err := q.Exec(ctx, `DELETE FROM equipment_endgame_state WHERE uid=$1`, st.UID)
		return err
	}
	_, err := q.Exec(ctx, `INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES($1,$2,$3,$4) ON CONFLICT(uid) DO UPDATE SET revision=EXCLUDED.revision,unique_id=EXCLUDED.unique_id,unique_roll_bp=EXCLUDED.unique_roll_bp`, st.UID, st.Endgame.Revision, st.Endgame.UniqueID, st.Endgame.UniqueRollBP)
	return err
}
func loadEndgameItems(ctx context.Context, q querier, items map[int64]domain.Stack) error {
	ids := make([]int64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `SELECT uid,revision,unique_id,unique_roll_bp FROM equipment_endgame_state WHERE uid=ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var v domain.EndgameItemState
		if err := rows.Scan(&id, &v.Revision, &v.UniqueID, &v.UniqueRollBP); err != nil {
			return err
		}
		st := items[id]
		st.Endgame = v
		if err := validateEndgameItem(st); err != nil {
			return err
		}
		items[id] = st
	}
	return rows.Err()
}

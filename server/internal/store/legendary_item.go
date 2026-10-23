package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func validateLegendaryItem(st domain.Stack) error {
	if err := st.Legendary.Validate(); err != nil {
		return err
	}
	if st.Legendary.PowerID != "" && (st.UID <= 0 || st.Count != 1 || st.Endgame.UniqueID != 0 || st.RolledAffixCount != 0 || st.SocketCount != 0 || (st.InstanceKind != domain.ItemInstanceNone && st.InstanceKind != domain.ItemInstanceEquipment)) {
		return fmt.Errorf("invalid legendary instance uid=%d", st.UID)
	}
	return nil
}
func saveLegendaryItem(ctx context.Context, q querier, st domain.Stack) error {
	if err := validateLegendaryItem(st); err != nil {
		return err
	}
	if st.Legendary.PowerID == "" {
		_, err := q.Exec(ctx, `DELETE FROM equipment_legendary_state WHERE uid=$1`, st.UID)
		return err
	}
	raw, err := json.Marshal(st.Legendary)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO equipment_legendary_state(uid,state) VALUES($1,$2::jsonb) ON CONFLICT(uid) DO UPDATE SET state=EXCLUDED.state`, st.UID, string(raw))
	return err
}
func loadLegendaryItems(ctx context.Context, q querier, items map[int64]domain.Stack) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	rows, err := q.Query(ctx, `SELECT uid,state FROM equipment_legendary_state WHERE uid=ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		st := items[id]
		if err := json.Unmarshal(raw, &st.Legendary); err != nil {
			return err
		}
		if err := validateLegendaryItem(st); err != nil {
			return err
		}
		items[id] = st
	}
	return rows.Err()
}

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func validateWingPowerItem(st domain.Stack) error {
	if err := st.WingPowers.Validate(); err != nil {
		return err
	}
	if st.WingPowers != (domain.WingPowers{}) && (st.UID <= 0 || st.Count != 1 || st.InstanceKind != domain.ItemInstanceEquipment || st.Legendary.PowerID != "" || st.Endgame.UniqueID != 0) {
		return fmt.Errorf("invalid wing power equipment uid=%d", st.UID)
	}
	return nil
}
func saveWingPowerItem(ctx context.Context, q querier, st domain.Stack) error {
	if err := validateWingPowerItem(st); err != nil {
		return err
	}
	if st.WingPowers == (domain.WingPowers{}) {
		_, err := q.Exec(ctx, `DELETE FROM equipment_wing_power_state WHERE uid=$1`, st.UID)
		return err
	}
	raw, err := json.Marshal(st.WingPowers)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO equipment_wing_power_state(uid,state) VALUES($1,$2::jsonb) ON CONFLICT(uid) DO UPDATE SET state=EXCLUDED.state`, st.UID, string(raw))
	return err
}
func loadWingPowerItems(ctx context.Context, q querier, items map[int64]domain.Stack) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	rows, err := q.Query(ctx, `SELECT uid,state FROM equipment_wing_power_state WHERE uid=ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			return err
		}
		st := items[id]
		// JSON arrays larger than three would otherwise be silently truncated by Go.
		var entries []domain.WingPower
		if err = json.Unmarshal(raw, &entries); err != nil {
			return err
		}
		if len(entries) != 3 {
			return fmt.Errorf("invalid wing power array length")
		}
		copy(st.WingPowers[:], entries)
		if err = validateWingPowerItem(st); err != nil {
			return err
		}
		items[id] = st
	}
	return rows.Err()
}

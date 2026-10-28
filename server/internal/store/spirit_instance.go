package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"io"
)

func saveSpiritInstance(ctx context.Context, q querier, st domain.Stack) error {
	if err := domain.ValidateSpiritStack(st); err != nil {
		return err
	}
	if st.InstanceKind == domain.ItemInstanceSpirit {
		raw, err := json.Marshal(st.Spirit)
		if err != nil {
			return err
		}
		_, err = q.Exec(ctx, `INSERT INTO spirit_instances(uid,state) VALUES($1,$2::jsonb) ON CONFLICT(uid) DO UPDATE SET state=EXCLUDED.state`, st.UID, string(raw))
		return err
	}
	if st.InstanceKind != domain.ItemInstanceEquipment && st.InstanceKind != domain.ItemInstanceNone {
		return nil
	}
	if st.FusedSpirit.Empty() && st.SpiritAppearance == 0 {
		_, err := q.Exec(ctx, `DELETE FROM equipment_spirit_fusions WHERE equipment_uid=$1`, st.UID)
		return err
	}
	var nested any
	if !st.FusedSpirit.Empty() {
		child := st.FusedSpirit.Stack()
		tag, err := q.Exec(ctx, `INSERT INTO item_instances(uid,item_id,instance_kind,bound,locked,created_source) VALUES($1,$2,$3,$4,$5,'spirit-fusion') ON CONFLICT(uid) DO UPDATE SET bound=EXCLUDED.bound,locked=EXCLUDED.locked,updated_at=now() WHERE item_instances.item_id=EXCLUDED.item_id AND item_instances.instance_kind=EXCLUDED.instance_kind`, child.UID, int32(child.Item), int16(child.InstanceKind), child.Bound, child.Locked)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("spirit: fused UID identity collision")
		}
		if err := saveSpiritInstance(ctx, q, child); err != nil {
			return err
		}
		nested = child.UID
	}
	_, err := q.Exec(ctx, `INSERT INTO equipment_spirit_fusions(equipment_uid,spirit_uid,appearance_item_id) VALUES($1,$2,$3) ON CONFLICT(equipment_uid) DO UPDATE SET spirit_uid=EXCLUDED.spirit_uid,appearance_item_id=EXCLUDED.appearance_item_id`, st.UID, nested, int32(st.SpiritAppearance))
	return err
}
func loadSpiritInstances(ctx context.Context, q querier, items map[int64]domain.Stack) error {
	ids := make([]int64, 0, len(items))
	for uid := range items {
		ids = append(ids, uid)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `SELECT uid,state FROM spirit_instances WHERE uid=ANY($1)`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uid int64
		var raw []byte
		if err = rows.Scan(&uid, &raw); err != nil {
			rows.Close()
			return err
		}
		st := items[uid]
		if st.Spirit, err = decodeSpiritState(raw); err != nil {
			rows.Close()
			return err
		}
		if err = domain.ValidateSpiritStack(st); err != nil {
			rows.Close()
			return err
		}
		items[uid] = st
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = q.Query(ctx, `SELECT f.equipment_uid,f.appearance_item_id,COALESCE(i.uid,0),COALESCE(i.item_id,0),COALESCE(i.instance_kind,0),COALESCE(i.bound,false),COALESCE(i.locked,false),COALESCE(s.state,'{}'::jsonb) FROM equipment_spirit_fusions f LEFT JOIN item_instances i ON i.uid=f.spirit_uid LEFT JOIN spirit_instances s ON s.uid=f.spirit_uid WHERE f.equipment_uid=ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var ap domain.ItemID
		var a domain.SpiritAttachment
		var kind int16
		var raw []byte
		if err = rows.Scan(&uid, &ap, &a.UID, &a.Item, &kind, &a.Bound, &a.Locked, &raw); err != nil {
			return err
		}
		if a.UID > 0 {
			if kind != int16(domain.ItemInstanceSpirit) {
				return fmt.Errorf("spirit: invalid nested instance kind %d", kind)
			}
			if a.State, err = decodeSpiritState(raw); err != nil {
				return err
			}
		}
		st := items[uid]
		st.FusedSpirit = a
		st.SpiritAppearance = ap
		if err = domain.ValidateSpiritStack(st); err != nil {
			return err
		}
		items[uid] = st
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, st := range items {
		if st.InstanceKind == domain.ItemInstanceSpirit && st.Spirit.Kind == 0 {
			return fmt.Errorf("spirit: missing persisted state uid=%d", st.UID)
		}
	}
	return nil
}

// Only new spirit ownership is enforced here; legacy duplicate fixtures do not
// gain new unrelated restrictions. Every spirit UID must have exactly one owner.
func validateSpiritOwnership(s domain.Snapshot) error {
	counts := map[int64]int{}
	spirits := map[int64]bool{}
	visit := func(st domain.Stack) {
		if st.Empty() {
			return
		}
		if st.UID > 0 {
			counts[st.UID]++
		}
		if st.InstanceKind == domain.ItemInstanceSpirit {
			spirits[st.UID] = true
		}
		for _, uid := range st.SocketUIDs {
			if uid > 0 {
				counts[uid]++
			}
		}
		if !st.FusedSpirit.Empty() {
			counts[st.FusedSpirit.UID]++
			spirits[st.FusedSpirit.UID] = true
		}
	}
	if s.Bag != nil {
		s.Bag.Each(func(_ int, st domain.Stack) { visit(st) })
	}
	if s.Worn != nil {
		s.Worn.Each(func(_ domain.EquipSlot, st domain.Stack) { visit(st) })
	}
	if s.ChangeSet != nil {
		s.ChangeSet.Each(func(_ domain.EquipSlot, st domain.Stack) { visit(st) })
	}
	if s.Warehouse != nil {
		s.Warehouse.Each(func(_, _ int, st domain.Stack) { visit(st) })
	}
	if s.Stall != nil {
		for _, v := range s.Stall.Items {
			visit(v.Stack)
		}
	}
	for uid := range spirits {
		if uid <= 0 || counts[uid] != 1 {
			return fmt.Errorf("spirit: duplicate ownership uid=%d", uid)
		}
	}
	return nil
}

// Decode variable-length arrays before copying: encoding/json silently truncates
// oversized JSON arrays when decoding straight into fixed Go arrays.
func decodeSpiritState(raw []byte) (domain.SpiritState, error) {
	var wire struct {
		LegacyFixed bool
		Kind        uint8
		AttrCount   uint8
		Attrs       []int32
		Rolls       [][]uint8
		LastSlots   []uint8
		LastValid   bool
	}
	var state domain.SpiritState
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return state, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return state, fmt.Errorf("spirit: trailing state data")
	}
	if len(wire.Attrs) != domain.SpiritMaxAttributes || len(wire.Rolls) != domain.SpiritMaxAttributes || len(wire.LastSlots) != domain.SpiritMaxAttributes {
		return state, fmt.Errorf("spirit: invalid persisted array dimensions")
	}
	state.LegacyFixed = wire.LegacyFixed
	state.Kind = wire.Kind
	state.AttrCount = wire.AttrCount
	state.LastValid = wire.LastValid
	copy(state.Attrs[:], wire.Attrs)
	copy(state.LastSlots[:], wire.LastSlots)
	for i, row := range wire.Rolls {
		if len(row) != domain.SpiritMaxRolls {
			return domain.SpiritState{}, fmt.Errorf("spirit: invalid persisted roll dimensions")
		}
		copy(state.Rolls[i][:], row)
	}
	if err := state.Validate(); err != nil {
		return domain.SpiritState{}, err
	}
	return state, nil
}

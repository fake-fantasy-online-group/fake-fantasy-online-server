package store

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (p *Postgres) LoadFamilyStash(ctx context.Context, charID int64) (*domain.FamilyStash, error) {
	return loadFamilyStash(ctx, p.pool, charID)
}

func (t *pgTx) LoadFamilyStash(ctx context.Context, charID int64) (*domain.FamilyStash, error) {
	return loadFamilyStash(ctx, t.tx, charID)
}

func familyStashHeader(ctx context.Context, q querier, charID int64, lock bool) (domain.FamilyStash, error) {
	if charID <= 0 {
		return domain.FamilyStash{}, fmt.Errorf("store: 家族仓库角色号无效")
	}
	query := `SELECT f.id,s.volume,m.position_id,COALESCE(cfg.take_position,2),
		(m.char_id=f.leader_char_id)
		FROM game_family_members m
		JOIN game_families f ON f.id=m.family_id
		JOIN gamedata.ov_famstash s ON s.stash_level=f.level
		LEFT JOIN game_family_stash_settings cfg ON cfg.family_id=f.id
		WHERE m.char_id=$1`
	if lock {
		query += ` FOR UPDATE OF f`
	}
	var out domain.FamilyStash
	var position, takePosition int16
	err := q.QueryRow(ctx, query, charID).Scan(&out.FamilyID, &out.Capacity, &position,
		&takePosition, &out.IsLeader)
	if err != nil {
		return domain.FamilyStash{}, err
	}
	out.TakePos = domain.FamilyPositionID(takePosition)
	out.CanTake = position > 0 && position <= takePosition
	if out.FamilyID <= 0 || out.Capacity <= 0 || out.Capacity > 1000 ||
		out.TakePos < domain.FamilyLeader || out.TakePos > domain.FamilyOrdinary {
		return domain.FamilyStash{}, fmt.Errorf("store: 家族仓库头无效: %+v", out)
	}
	return out, nil
}

func loadFamilyStash(ctx context.Context, q querier, charID int64) (*domain.FamilyStash, error) {
	out, err := familyStashHeader(ctx, q, charID, false)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: 读取家族仓库头: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT stash_index,uid,item_id,count,durability,max_durability,
		durability_wear_raw,bound,locked,refine_level,socket_count,sockets,
		wash_quality,wash_count,wash_attrs,wash_values,wash_modes,fused_appearance_item_id,deposited_by
		FROM game_family_stash_items WHERE family_id=$1 ORDER BY stash_index`, out.FamilyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var instanceUIDs []int64
	for rows.Next() {
		var entry domain.FamilyStashEntry
		var socketCount, washQuality, washCount int16
		var socketIDs, washAttrs, washValues, washModes []int32
		if err := rows.Scan(&entry.Index, &entry.Stack.UID, &entry.Stack.Item, &entry.Stack.Count,
			&entry.Stack.Durability, &entry.Stack.MaxDurability, &entry.Stack.DurabilityWearRaw,
			&entry.Stack.Bound, &entry.Stack.Locked, &entry.Stack.RefineLevel,
			&socketCount, &socketIDs, &washQuality, &washCount, &washAttrs, &washValues,
			&washModes, &entry.Stack.FusedAppearance, &entry.By); err != nil {
			return nil, err
		}
		entry.Stack.SocketCount = uint8(socketCount)
		for i := range entry.Stack.Sockets {
			if i < len(socketIDs) {
				entry.Stack.Sockets[i] = domain.ItemID(socketIDs[i])
			}
		}
		loadWashArrays(&entry.Stack, washQuality, washCount, washAttrs, washValues, washModes)
		if entry.Stack.UID > 0 {
			instanceUIDs = append(instanceUIDs, entry.Stack.UID)
		}
		if entry.Index < 0 || entry.Index >= out.Capacity || entry.Stack.Empty() || entry.By == "" {
			return nil, fmt.Errorf("store: 家族仓库第%d格无效", entry.Index)
		}
		out.Entries = append(out.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	facts, err := loadItemInstances(ctx, q, instanceUIDs)
	if err != nil {
		return nil, err
	}
	for i := range out.Entries {
		if out.Entries[i].Stack.UID <= 0 {
			continue
		}
		out.Entries[i].Stack, err = hydrateStack(out.Entries[i].Stack, facts)
		if err != nil {
			return nil, err
		}
	}
	return &out, nil
}

func (t *pgTx) DepositFamilyStash(ctx context.Context, charID int64, stack domain.Stack, by string) error {
	header, err := familyStashHeader(ctx, t.tx, charID, true)
	by = strings.TrimSpace(by)
	if err != nil {
		return fmt.Errorf("store: 读取家族仓库存入权限: %w", err)
	}
	if stack.Empty() || stack.Locked || stack.Bound || by == "" || len([]rune(by)) > 30 {
		return fmt.Errorf("store: 家族仓库存入参数无效")
	}
	if err := saveItemInstances(ctx, t.tx, "family_stash", stack); err != nil {
		return err
	}
	rows, err := t.tx.Query(ctx, `SELECT stash_index FROM game_family_stash_items
		WHERE family_id=$1 ORDER BY stash_index FOR UPDATE`, header.FamilyID)
	if err != nil {
		return err
	}
	used := make(map[int32]struct{})
	for rows.Next() {
		var index int32
		if err := rows.Scan(&index); err != nil {
			rows.Close()
			return err
		}
		used[index] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	index := int32(-1)
	for i := int32(0); i < header.Capacity; i++ {
		if _, exists := used[i]; !exists {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("store: 家族仓库已满")
	}
	wa, wv, wm := washArraysOf(stack)
	_, err = t.tx.Exec(ctx, `INSERT INTO game_family_stash_items
		(family_id,stash_index,uid,item_id,count,durability,max_durability,durability_wear_raw,
		 bound,locked,refine_level,socket_count,sockets,wash_quality,wash_count,wash_attrs,
		 wash_values,wash_modes,fused_appearance_item_id,deposited_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		header.FamilyID, index, stack.UID, stack.Item, stack.Count, stack.Durability,
		stack.MaxDurability, stack.DurabilityWearRaw, stack.Bound, stack.Locked,
		stack.RefineLevel, int16(stack.SocketCount), socketIDsOf(stack), int16(stack.WashQuality),
		int16(stack.WashCount), wa, wv, wm, stack.FusedAppearance, by)
	return err
}

func (t *pgTx) WithdrawFamilyStash(ctx context.Context, charID int64, index int32,
	expected domain.Stack) error {
	header, err := familyStashHeader(ctx, t.tx, charID, true)
	if err != nil {
		return fmt.Errorf("store: 读取家族仓库取出权限: %w", err)
	}
	if !header.CanTake || index < 0 || index >= header.Capacity || expected.Empty() {
		return fmt.Errorf("store: 家族仓库取出参数或权限无效")
	}
	loaded, err := loadFamilyStashEntryForUpdate(ctx, t.tx, header.FamilyID, index)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(loaded, expected) {
		return fmt.Errorf("store: 家族仓库格已变化")
	}
	result, err := t.tx.Exec(ctx, `DELETE FROM game_family_stash_items
		WHERE family_id=$1 AND stash_index=$2`, header.FamilyID, index)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("store: 家族仓库格删除失败")
	}
	return nil
}

func loadFamilyStashEntryForUpdate(ctx context.Context, q querier, familyID int64,
	index int32) (domain.Stack, error) {
	var stack domain.Stack
	var socketCount, washQuality, washCount int16
	var socketIDs, washAttrs, washValues, washModes []int32
	err := q.QueryRow(ctx, `SELECT uid,item_id,count,durability,max_durability,durability_wear_raw,
		bound,locked,refine_level,socket_count,sockets,wash_quality,wash_count,wash_attrs,
		wash_values,wash_modes,fused_appearance_item_id FROM game_family_stash_items
		WHERE family_id=$1 AND stash_index=$2 FOR UPDATE`, familyID, index).
		Scan(&stack.UID, &stack.Item, &stack.Count, &stack.Durability, &stack.MaxDurability,
			&stack.DurabilityWearRaw, &stack.Bound, &stack.Locked, &stack.RefineLevel,
			&socketCount, &socketIDs, &washQuality, &washCount, &washAttrs, &washValues,
			&washModes, &stack.FusedAppearance)
	if err != nil {
		return domain.Stack{}, err
	}
	stack.SocketCount = uint8(socketCount)
	for i := range stack.Sockets {
		if i < len(socketIDs) {
			stack.Sockets[i] = domain.ItemID(socketIDs[i])
		}
	}
	loadWashArrays(&stack, washQuality, washCount, washAttrs, washValues, washModes)
	if stack.UID > 0 {
		facts, err := loadItemInstances(ctx, q, []int64{stack.UID})
		if err != nil {
			return domain.Stack{}, err
		}
		stack, err = hydrateStack(stack, facts)
		if err != nil {
			return domain.Stack{}, err
		}
	}
	return stack, nil
}

func (t *pgTx) SetFamilyStashTakePosition(ctx context.Context, charID int64,
	position domain.FamilyPositionID) error {
	header, err := familyStashHeader(ctx, t.tx, charID, true)
	if err != nil {
		return fmt.Errorf("store: 读取家族仓库设置权限: %w", err)
	}
	if !header.IsLeader || position < domain.FamilyLeader || position > domain.FamilyOrdinary {
		return fmt.Errorf("store: 家族仓库权限参数无效")
	}
	var available bool
	if err := t.tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM game_families f
		JOIN gamedata.ov_famlevel l ON l.fam_level=f.level
		JOIN gamedata.ov_famposperm p ON p.fam_pos=$2
		WHERE f.id=$1 AND (p.fam_pos IN (1,2,10) OR p.fam_pos BETWEEN 3 AND 2+l.max_custom_pos))`,
		header.FamilyID, position).Scan(&available); err != nil || !available {
		return fmt.Errorf("store: 该家族等级没有职位 %d", position)
	}
	_, err = t.tx.Exec(ctx, `INSERT INTO game_family_stash_settings(family_id,take_position)
		VALUES($1,$2) ON CONFLICT(family_id) DO UPDATE SET take_position=EXCLUDED.take_position`,
		header.FamilyID, position)
	return err
}

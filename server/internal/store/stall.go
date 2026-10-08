package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) LoadStall(ctx context.Context, charID int64) (*domain.Stall, error) {
	return loadStall(ctx, p.pool, charID)
}

func (t *pgTx) LoadStall(ctx context.Context, charID int64) (*domain.Stall, error) {
	return loadStall(ctx, t.tx, charID)
}

func loadStall(ctx context.Context, q querier, charID int64) (*domain.Stall, error) {
	var typ int16
	var name string
	err := q.QueryRow(ctx, `SELECT stall_type,name FROM character_stalls WHERE char_id=$1`, charID).
		Scan(&typ, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: 查角色摊位: %w", err)
	}
	stall := domain.NewStall(domain.StallType(typ), name)
	if stall == nil {
		return nil, fmt.Errorf("store: 角色 %d 摊位状态非法", charID)
	}
	rows, err := q.Query(ctx, `
		SELECT position,uid,item_id,count,unit_price,durability,max_durability,
		       durability_wear_raw,bound,locked,refine_level,socket_count,sockets,
		       wash_quality,wash_count,wash_attrs,wash_values,wash_modes,fused_appearance_item_id
		  FROM character_stall_items WHERE char_id=$1 ORDER BY position`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查摊位托管物品: %w", err)
	}
	expected := 0
	var instanceUIDs []int64
	for rows.Next() {
		var position int
		var item domain.StallItem
		var socketCount, washQuality, washCount int16
		var sockets, wa, wv, wm []int32
		if err := rows.Scan(&position, &item.Stack.UID, &item.Stack.Item, &item.Stack.Count,
			&item.UnitPrice, &item.Stack.Durability, &item.Stack.MaxDurability,
			&item.Stack.DurabilityWearRaw, &item.Stack.Bound, &item.Stack.Locked,
			&item.Stack.RefineLevel, &socketCount, &sockets, &washQuality, &washCount,
			&wa, &wv, &wm, &item.Stack.FusedAppearance); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: 读摊位托管物品: %w", err)
		}
		item.Stack.SocketCount = uint8(socketCount)
		for i := range item.Stack.Sockets {
			if i < len(sockets) {
				item.Stack.Sockets[i] = domain.ItemID(sockets[i])
			}
		}
		loadWashArrays(&item.Stack, washQuality, washCount, wa, wv, wm)
		if item.Stack.UID > 0 {
			instanceUIDs = append(instanceUIDs, item.Stack.UID)
		}
		if position != expected || !stall.Add(item) {
			rows.Close()
			return nil, fmt.Errorf("store: 角色 %d 摊位第 %d 项非法", charID, position)
		}
		expected++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	facts, err := loadItemInstances(ctx, q, instanceUIDs)
	if err != nil {
		return nil, err
	}
	for i := range stall.Items {
		if stall.Items[i].Stack.UID <= 0 {
			continue
		}
		st, err := hydrateStack(stall.Items[i].Stack, facts)
		if err != nil {
			return nil, err
		}
		stall.Items[i].Stack = st
	}

	sales, err := q.Query(ctx, `
		SELECT occurred_at,item_name,count,money
		  FROM character_stall_sales WHERE char_id=$1 ORDER BY position`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查摊位成交记录: %w", err)
	}
	for sales.Next() {
		var sale domain.StallSale
		if err := sales.Scan(&sale.Time, &sale.Item, &sale.Count, &sale.Money); err != nil {
			sales.Close()
			return nil, err
		}
		stall.AddSale(sale)
	}
	if err := sales.Err(); err != nil {
		sales.Close()
		return nil, err
	}
	sales.Close()
	return stall, nil
}

func saveStall(ctx context.Context, tx pgx.Tx, charID int64, stall *domain.Stall) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_stalls WHERE char_id=$1`, charID); err != nil {
		return fmt.Errorf("store: 清角色摊位: %w", err)
	}
	if stall == nil {
		return nil
	}
	if !stall.Type.Valid() || stall.Name == "" || len(stall.Items) > 1<<16-1 || len(stall.Sales) > 1<<16-1 {
		return fmt.Errorf("store: 角色 %d 摊位状态非法", charID)
	}
	instances := make([]domain.Stack, 0, len(stall.Items))
	for _, item := range stall.Items {
		instances = append(instances, item.Stack)
	}
	if err := saveItemInstances(ctx, tx, "stall", instances...); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO character_stalls(char_id,stall_type,name) VALUES($1,$2,$3)`,
		charID, int16(stall.Type), stall.Name); err != nil {
		return fmt.Errorf("store: 写角色摊位: %w", err)
	}
	for position, item := range stall.Items {
		s := item.Stack
		wa, wv, wm := washArraysOf(s)
		if _, err := tx.Exec(ctx, `
			INSERT INTO character_stall_items(char_id,position,uid,item_id,count,unit_price,
			 durability,max_durability,durability_wear_raw,bound,locked,refine_level,
			 socket_count,sockets,wash_quality,wash_count,wash_attrs,wash_values,wash_modes,
			 fused_appearance_item_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
			charID, position, s.UID, int32(s.Item), s.Count, item.UnitPrice,
			s.Durability, s.MaxDurability, s.DurabilityWearRaw, s.Bound, s.Locked,
			s.RefineLevel, int16(s.SocketCount), socketIDsOf(s), int16(s.WashQuality),
			int16(s.WashCount), wa, wv, wm, int32(s.FusedAppearance)); err != nil {
			return fmt.Errorf("store: 写摊位托管物品: %w", err)
		}
	}
	for position, sale := range stall.Sales {
		if _, err := tx.Exec(ctx, `
			INSERT INTO character_stall_sales(char_id,position,occurred_at,item_name,count,money)
			VALUES($1,$2,$3,$4,$5,$6)`, charID, position, sale.Time, sale.Item,
			sale.Count, sale.Money); err != nil {
			return fmt.Errorf("store: 写摊位成交记录: %w", err)
		}
	}
	return nil
}

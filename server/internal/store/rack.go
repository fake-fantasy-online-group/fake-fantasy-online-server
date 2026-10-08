package store

import (
	"context"
	"fmt"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (p *Postgres) LoadRackRefundRule(ctx context.Context) (domain.RackRefundRule, error) {
	return loadRackRefundRule(ctx, p.pool)
}

func (t *pgTx) LoadRackRefundRule(ctx context.Context) (domain.RackRefundRule, error) {
	return loadRackRefundRule(ctx, t.tx)
}

func loadRackRefundRule(ctx context.Context, q querier) (domain.RackRefundRule, error) {
	var rule domain.RackRefundRule
	err := q.QueryRow(ctx, `SELECT refund_pct,window_days,used_ok
		FROM game_rack_refund_rule WHERE singleton=TRUE`).
		Scan(&rule.Percent, &rule.WindowDay, &rule.UsedOK)
	if err != nil {
		return domain.RackRefundRule{}, fmt.Errorf("store: 读取货架退货规则: %w", err)
	}
	if !rule.Valid() {
		return domain.RackRefundRule{}, fmt.Errorf("store: 货架退货规则无效: %+v", rule)
	}
	return rule, nil
}

func (p *Postgres) LoadRackRefundOffers(ctx context.Context, charID int64,
	rule domain.RackRefundRule) ([]domain.RackRefundOffer, error) {
	return loadRackRefundOffers(ctx, p.pool, charID, rule)
}

func (t *pgTx) LoadRackRefundOffers(ctx context.Context, charID int64,
	rule domain.RackRefundRule) ([]domain.RackRefundOffer, error) {
	return loadRackRefundOffers(ctx, t.tx, charID, rule)
}

func loadRackRefundOffers(ctx context.Context, q querier, charID int64,
	rule domain.RackRefundRule) ([]domain.RackRefundOffer, error) {
	if charID <= 0 || !rule.Valid() {
		return nil, fmt.Errorf("store: 货架退货查询参数无效")
	}
	rows, err := q.Query(ctx, `SELECT item_id,unit_price,bundle,
		LEAST(SUM(shares-refunded_shares),2147483647)::INT,MIN(purchased_at),
		GREATEST(0,$2::INT-FLOOR(EXTRACT(EPOCH FROM (now()-MIN(purchased_at)))/86400)::INT)
		FROM game_rack_purchases
		WHERE char_id=$1 AND refunded_shares<shares
		  AND purchased_at>=now()-make_interval(days => $2::INT)
		GROUP BY item_id,unit_price,bundle
		ORDER BY MIN(purchased_at),item_id,unit_price,bundle`, charID, rule.WindowDay)
	if err != nil {
		return nil, fmt.Errorf("store: 查询货架可退流水: %w", err)
	}
	defer rows.Close()
	var out []domain.RackRefundOffer
	for rows.Next() {
		var offer domain.RackRefundOffer
		if err := rows.Scan(&offer.Item, &offer.UnitPrice, &offer.Bundle,
			&offer.RemainingShares, &offer.OldestPurchase, &offer.LeftDays); err != nil {
			return nil, err
		}
		if offer.Item <= 0 || offer.UnitPrice <= 0 || offer.Bundle <= 0 ||
			offer.RemainingShares <= 0 || offer.LeftDays < 0 {
			return nil, fmt.Errorf("store: 货架可退流水无效: %+v", offer)
		}
		out = append(out, offer)
	}
	return out, rows.Err()
}

func (t *pgTx) RecordRackPurchase(ctx context.Context, charID int64, item domain.ItemID,
	unitPrice, bundle, shares int32, purchasedAt time.Time) error {
	if charID <= 0 || item <= 0 || unitPrice <= 0 || bundle <= 0 || shares <= 0 || purchasedAt.IsZero() {
		return fmt.Errorf("store: 货架购买流水参数无效")
	}
	_, err := t.tx.Exec(ctx, `INSERT INTO game_rack_purchases
		(char_id,item_id,unit_price,bundle,shares,refunded_shares,purchased_at)
		VALUES($1,$2,$3,$4,$5,0,$6)`, charID, item, unitPrice, bundle, shares, purchasedAt)
	return err
}

func (t *pgTx) ConsumeRackRefund(ctx context.Context, charID int64, item domain.ItemID,
	unitPrice, bundle, shares int32, purchasedAfter time.Time) error {
	if charID <= 0 || item <= 0 || unitPrice <= 0 || bundle <= 0 || shares <= 0 || purchasedAfter.IsZero() {
		return fmt.Errorf("store: 货架退货流水参数无效")
	}
	rows, err := t.tx.Query(ctx, `SELECT id,shares-refunded_shares
		FROM game_rack_purchases
		WHERE char_id=$1 AND item_id=$2 AND unit_price=$3 AND bundle=$4
		  AND refunded_shares<shares AND purchased_at>=$5
		ORDER BY purchased_at,id FOR UPDATE`, charID, item, unitPrice, bundle, purchasedAfter)
	if err != nil {
		return err
	}
	type refundable struct {
		id   int64
		left int32
	}
	var candidates []refundable
	var total int64
	for rows.Next() {
		var row refundable
		if err := rows.Scan(&row.id, &row.left); err != nil {
			rows.Close()
			return err
		}
		if row.left <= 0 {
			rows.Close()
			return fmt.Errorf("store: 货架退货流水剩余份数无效")
		}
		candidates = append(candidates, row)
		total += int64(row.left)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if total < int64(shares) {
		return fmt.Errorf("store: 可退购买份数不足")
	}
	remaining := shares
	for _, row := range candidates {
		if remaining == 0 {
			break
		}
		consume := row.left
		if consume > remaining {
			consume = remaining
		}
		if _, err := t.tx.Exec(ctx, `UPDATE game_rack_purchases
			SET refunded_shares=refunded_shares+$1 WHERE id=$2`, consume, row.id); err != nil {
			return err
		}
		remaining -= consume
	}
	return nil
}

package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadPresentation(ctx context.Context, q Querier, items ItemDefs) (domain.AnnouncementPool, error) {
	var p domain.AnnouncementPool
	rows, err := q.Query(ctx, `SELECT speed,interval_sec,enabled,period_min FROM game_announcement_settings WHERE id=1`)
	if err != nil {
		return p, err
	}
	if !rows.Next() {
		rows.Close()
		return p, fmt.Errorf("missing announcement settings")
	}
	err = rows.Scan(&p.Speed, &p.IntervalSec, &p.Enabled, &p.PeriodMin)
	rows.Close()
	if err != nil {
		return p, err
	}
	rows, err = q.Query(ctx, `SELECT id,text FROM game_announcements WHERE enabled ORDER BY id`)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var l domain.AnnouncementLine
		if err = rows.Scan(&l.ID, &l.Text); err != nil {
			rows.Close()
			return p, err
		}
		p.Lines = append(p.Lines, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	rows, err = q.Query(ctx, `SELECT item_id,tier,skin FROM game_horn_items ORDER BY item_id`)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var tier, skin uint8
		if err = rows.Scan(&id, &tier, &skin); err != nil {
			return p, err
		}
		d, ok := items[id]
		if !ok || !d.ConsumeOnUse {
			return p, fmt.Errorf("horn item %d missing or not consumable", id)
		}
		d.HornTier, d.HornSkin = tier, skin
		items[id] = d
	}
	return p, rows.Err()
}

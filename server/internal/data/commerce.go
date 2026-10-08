package data

import (
	"context"
	"fmt"
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadRackCatalog(ctx context.Context, q Querier, items ItemDefs) (domain.RackCatalog, error) {
	rows, err := q.Query(ctx, `SELECT id,name FROM game_rack_categories ORDER BY position,id`)
	if err != nil {
		return domain.RackCatalog{}, fmt.Errorf("data: 查询货架分类: %w", err)
	}
	var categories []domain.RackCategory
	for rows.Next() {
		var id int16
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return domain.RackCatalog{}, err
		}
		categories = append(categories, domain.RackCategory{ID: uint8(id), Name: name})
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT g.item_id,g.category_id,g.price,g.quantity,g.flags,g.remain,g.part,
		COALESCE(p.pet_id,0)
		FROM game_rack_goods g LEFT JOIN game_rack_pet_goods p ON p.item_id=g.item_id
		WHERE g.active=TRUE ORDER BY g.category_id,g.item_id`)
	if err != nil {
		return domain.RackCatalog{}, fmt.Errorf("data: 查询货架商品: %w", err)
	}
	defer rows.Close()
	var goods []domain.RackGood
	for rows.Next() {
		var good domain.RackGood
		var category, flags int16
		if err := rows.Scan(&good.Item, &category, &good.Price, &good.Quantity,
			&flags, &good.Remain, &good.Part, &good.Pet); err != nil {
			return domain.RackCatalog{}, err
		}
		if _, ok := items[good.Item]; (!ok && good.Pet == 0) || (ok && good.Pet != 0) || good.Pet < 0 || good.Price <= 0 || good.Price > math.MaxInt32 ||
			good.Quantity <= 0 || category < 0 || category > 255 || flags < 0 || flags > 255 {
			return domain.RackCatalog{}, fmt.Errorf("data: 货架商品%d配置无效", good.Item)
		}
		good.Category, good.Flags = uint8(category), uint8(flags)
		goods = append(goods, good)
	}
	catalog := domain.NewRackCatalog(categories, goods)
	if !catalog.Valid() {
		return domain.RackCatalog{}, fmt.Errorf("data: 货架目录为空或重复")
	}
	return catalog, rows.Err()
}

func LoadDepositRule(ctx context.Context, q Querier) (domain.DepositRule, error) {
	var rule domain.DepositRule
	rows, err := q.Query(ctx, `SELECT auto_credit,caiyu_per_unit,max_amount
		FROM game_deposit_rule WHERE singleton=TRUE`)
	if err != nil {
		return domain.DepositRule{}, fmt.Errorf("data: 读取充值规则: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return domain.DepositRule{}, fmt.Errorf("data: 缺少充值规则")
	}
	if err := rows.Scan(&rule.AutoCredit, &rule.CaiyuPerUnit, &rule.MaxAmount); err != nil {
		return domain.DepositRule{}, fmt.Errorf("data: 读取充值规则: %w", err)
	}
	if !rule.Valid() {
		return domain.DepositRule{}, fmt.Errorf("data: 充值规则无效: %+v", rule)
	}
	return rule, nil
}

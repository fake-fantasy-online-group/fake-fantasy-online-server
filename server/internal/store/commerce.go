package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (p *Postgres) RecordDeposit(ctx context.Context, charID int64, chain string, amount int32, caiyu int64) (int64, error) {
	return recordDeposit(ctx, p.pool, charID, chain, amount, caiyu)
}

func (t *pgTx) RecordDeposit(ctx context.Context, charID int64, chain string, amount int32, caiyu int64) (int64, error) {
	return recordDeposit(ctx, t.tx, charID, chain, amount, caiyu)
}

func recordDeposit(ctx context.Context, q querier, charID int64, chain string, amount int32, caiyu int64) (int64, error) {
	chain = strings.TrimSpace(chain)
	if charID <= 0 || chain == "" || len([]rune(chain)) > 40 || amount <= 0 || caiyu <= 0 {
		return 0, fmt.Errorf("store: 充值参数无效")
	}
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO game_deposit_requests(char_id,chain,amount,caiyu,status)
		VALUES($1,$2,$3,$4,'credited') RETURNING id`, charID, chain, amount, caiyu).Scan(&id)
	return id, err
}

// saveAccountCaiyu 只随完整快照事务提交，不能让仅更新登录状态的 SaveChar 写钱包。
// 调用方必须持有全服账号租约，直到最终快照提交后才允许其它角色加载余额。
func saveAccountCaiyu(ctx context.Context, q querier, c *domain.Character) error {
	ct, err := q.Exec(ctx, `UPDATE accounts SET caiyu=$3
 WHERE id=$2 AND EXISTS (SELECT 1 FROM characters WHERE id=$1 AND account_id=$2)`,
		c.ID, c.AccountID, c.Caiyu)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return domain.ErrCharNotFound
	}
	return nil
}

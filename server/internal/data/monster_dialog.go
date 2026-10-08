package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadMonsterDialogs 只读 PostgreSQL 的 ov_dialog，保留 row_no 原始顺序。
// 这里仅验证可安全索引的字段，不推断 Scene 对应的触发时机。
func LoadMonsterDialogs(ctx context.Context, q Querier) (*domain.MonsterDialogs, error) {
	rows, err := q.Query(ctx, `
		SELECT d.row_no, d.mon_id, d.scene, d.speak_grp_1_4, d.desc_,
		       EXISTS (SELECT 1 FROM gamedata.ov_cmon m WHERE m.index = d.mon_id)
		  FROM gamedata.ov_dialog d
		 ORDER BY d.row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 ov_dialog: %w", err)
	}
	defer rows.Close()

	var entries []domain.MonsterDialog
	for rows.Next() {
		var rowNo, monster, scene, group int32
		var text string
		var knownMonster bool
		if err := rows.Scan(&rowNo, &monster, &scene, &group, &text, &knownMonster); err != nil {
			return nil, fmt.Errorf("data: 读 ov_dialog 行: %w", err)
		}
		if rowNo < 0 || monster <= 0 || !knownMonster || scene < 0 || scene > 255 ||
			group < 1 || group > 4 || text == "" {
			return nil, fmt.Errorf("data: ov_dialog 行 %d 非法: mon=%d scene=%d group=%d known=%t text_empty=%t",
				rowNo, monster, scene, group, knownMonster, text == "")
		}
		entries = append(entries, domain.MonsterDialog{
			Monster: domain.MonsterID(monster), Scene: uint8(scene), Group: uint8(group), Text: text,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 ov_dialog: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("data: ov_dialog 为空")
	}
	return domain.NewMonsterDialogs(entries), nil
}

package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 单独的位置表现与施法者动作、弹道、受击表现分开处理。
// 当前客户端RunData对纯位置atk及位置hit，通过groundPos调用PlayAt。
// 前者无坐标时回退到caster，后者回退到target。只匹配原表明确pos=4
// 且没有混合实体范围受击的配置，不能把所有atk/范围技能都当成位置效果。
func loadGroundSkillEffects(ctx context.Context, q Querier) (map[domain.SkillID]bool, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT skill_id FROM gamedata.ov_skillshow
		WHERE (atk_show_id>0 AND atk_show_pos=4
		AND fly_show_id=0 AND be_h_show_id=0 AND r_be_h_show_id=0)
		OR (be_h_show_id>0 AND be_h_show_pos=4 AND r_be_h_show_id=0)`)
	if err != nil {
		return nil, fmt.Errorf("data: 查位置技能特效: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillID]bool)
	for rows.Next() {
		var id domain.SkillID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

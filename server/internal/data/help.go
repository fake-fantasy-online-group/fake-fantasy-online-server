package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadHelpTopics 加载当前普通角色可见的帮助主题。ov_help.fly_show=0 是普通
// 帮助组；1/2 是飞升分组，而当前角色模型尚未实现飞升状态，不能混发。
func LoadHelpTopics(ctx context.Context, q Querier) ([]domain.HelpTopic, error) {
	rows, err := q.Query(ctx, `
		SELECT fly_topic, help
		  FROM gamedata.ov_help
		 WHERE fly_show = 0 AND fly_topic <> '' AND help <> ''
		 ORDER BY row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查帮助主题: %w", err)
	}
	defer rows.Close()
	var out []domain.HelpTopic
	for rows.Next() {
		var topic domain.HelpTopic
		if err := rows.Scan(&topic.Title, &topic.Content); err != nil {
			return nil, fmt.Errorf("data: 读帮助主题: %w", err)
		}
		out = append(out, topic)
		if len(out) > 255 {
			return nil, fmt.Errorf("data: 普通帮助主题超过 U8 上限: %d", len(out))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历帮助主题: %w", err)
	}
	return out, nil
}

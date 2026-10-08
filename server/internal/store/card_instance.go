package store

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func saveCardInstance(ctx context.Context, q querier, uid int64, card domain.CardInstanceState) error {
	if !card.Initialized || int(card.Count) > len(card.Effects) {
		return fmt.Errorf("store: 卡实例 %d 属性未初始化或越界", uid)
	}
	ops, attrs, modes := make([]int32, 8), make([]int32, 8), make([]int32, 8)
	probabilities, values := make([]int32, 8), make([]int32, 8)
	for i, e := range card.Effects {
		ops[i], attrs[i], modes[i], probabilities[i], values[i] = e.Op, e.Attr, e.Mode, e.Probability, e.Value
	}
	_, err := q.Exec(ctx, `INSERT INTO card_instances(uid,effect_count,ops,attrs,modes,probabilities,effect_values)
 VALUES($1,$2,$3,$4,$5,$6,$7)
 ON CONFLICT(uid) DO UPDATE SET effect_count=EXCLUDED.effect_count,ops=EXCLUDED.ops,
 attrs=EXCLUDED.attrs,modes=EXCLUDED.modes,probabilities=EXCLUDED.probabilities,effect_values=EXCLUDED.effect_values`,
		uid, int16(card.Count), ops, attrs, modes, probabilities, values)
	if err != nil {
		return fmt.Errorf("store: 写卡实例 %d 属性: %w", uid, err)
	}
	return nil
}

// loadCardInstances 同时加载背包中的卡和装备孔内的卡；缺失状态是存档错误，
// 不允许重登时按模板重新生成属性。
func loadCardInstances(ctx context.Context, q querier, facts map[int64]domain.Stack) error {
	uids := make([]int64, 0)
	for _, st := range facts {
		if st.InstanceKind == domain.ItemInstanceSocketCard {
			uids = append(uids, st.UID)
		}
		for _, uid := range st.SocketUIDs {
			if uid > 0 {
				uids = append(uids, uid)
			}
		}
	}
	if len(uids) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `SELECT i.uid,i.item_id,i.bound,i.locked,c.effect_count,
 c.ops,c.attrs,c.modes,c.probabilities,c.effect_values
 FROM item_instances i JOIN card_instances c ON c.uid=i.uid
 WHERE i.uid=ANY($1) AND i.instance_kind=2`, uids)
	if err != nil {
		return fmt.Errorf("store: 读取卡实例属性: %w", err)
	}
	defer rows.Close()
	cards := make(map[int64]domain.Stack)
	for rows.Next() {
		var st domain.Stack
		var count int16
		var ops, attrs, modes, probabilities, values []int32
		if err := rows.Scan(&st.UID, &st.Item, &st.Bound, &st.Locked, &count, &ops, &attrs, &modes, &probabilities, &values); err != nil {
			return err
		}
		if count < 0 || count > 8 || len(ops) != 8 || len(attrs) != 8 || len(modes) != 8 || len(probabilities) != 8 || len(values) != 8 {
			return fmt.Errorf("store: 卡实例 %d 属性数组非法", st.UID)
		}
		st.Card = domain.CardInstanceState{Initialized: true, Count: uint8(count), Bound: st.Bound, Locked: st.Locked}
		for i := range st.Card.Effects {
			st.Card.Effects[i] = domain.CardEffect{Op: ops[i], Attr: attrs[i], Mode: modes[i], Probability: probabilities[i], Value: values[i]}
		}
		cards[st.UID] = st
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for uid, st := range facts {
		if st.InstanceKind == domain.ItemInstanceSocketCard {
			card, ok := cards[uid]
			if !ok || card.Item != st.Item {
				return fmt.Errorf("store: 卡实例 %d 属性缺失或模板不符", uid)
			}
			st.Card = card.Card
		}
		for i, cardUID := range st.SocketUIDs {
			if cardUID == 0 {
				continue
			}
			card, ok := cards[cardUID]
			if !ok || card.Item != st.Sockets[i] {
				return fmt.Errorf("store: 装备 %d 孔内卡 %d 属性缺失或模板不符", uid, cardUID)
			}
			st.SocketCards[i] = card.Card
		}
		facts[uid] = st
	}
	return nil
}

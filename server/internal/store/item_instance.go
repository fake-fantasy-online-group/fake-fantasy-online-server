package store

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func rolledArraysOf(st domain.Stack) (cards, attrs, values, modes []int32) {
	cards, attrs = make([]int32, 5), make([]int32, 5)
	values, modes = make([]int32, 5), make([]int32, 5)
	for i := range st.RolledAffixes {
		cards[i] = st.RolledAffixes[i].CardID
		attrs[i] = st.RolledAffixes[i].Affix.Attr
		values[i] = st.RolledAffixes[i].Affix.Value
		modes[i] = st.RolledAffixes[i].Affix.Mode
	}
	return
}

func socketUIDsOf(st domain.Stack) []int64 {
	out := make([]int64, len(st.SocketUIDs))
	copy(out, st.SocketUIDs[:])
	return out
}

// saveItemInstances 在写任何位置表之前更新统一实例事实源。位置表保留旧列
// 作为协议快照缓存，但随机属性与孔内卡实例只以这里为准。
func saveItemInstances(ctx context.Context, q querier, source string, stacks ...domain.Stack) error {
	seen := make(map[int64]struct{}, len(stacks))
	for _, st := range stacks {
		if st.Empty() {
			continue
		}
		kind := st.InstanceKind
		if kind == domain.ItemInstanceNone && st.UID > 0 {
			// 兼容旧测试夹具和迁移前内存快照；历史 UID 只曾用于装备。
			kind = domain.ItemInstanceEquipment
		}
		if kind == domain.ItemInstanceNone {
			continue
		}
		if st.UID <= 0 || st.Count != 1 {
			return fmt.Errorf("store: 实例物品 %d 缺少合法UID或数量 uid=%d count=%d", st.Item, st.UID, st.Count)
		}
		if _, ok := seen[st.UID]; ok {
			continue
		}
		seen[st.UID] = struct{}{}
		tag, err := q.Exec(ctx, `
			INSERT INTO item_instances(uid,item_id,instance_kind,bound,locked,created_source)
			VALUES($1,$2,$3,$4,$5,$6)
			ON CONFLICT(uid) DO UPDATE SET bound=EXCLUDED.bound,locked=EXCLUDED.locked,updated_at=now()
			WHERE item_instances.item_id=EXCLUDED.item_id
			  AND item_instances.instance_kind=EXCLUDED.instance_kind`,
			st.UID, int32(st.Item), int16(kind), st.Bound, st.Locked, source)
		if err != nil {
			return fmt.Errorf("store: 写物品实例 %d: %w", st.UID, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("store: 物品实例UID冲突 uid=%d item=%d kind=%d", st.UID, st.Item, kind)
		}
		if kind == domain.ItemInstanceSocketCard {
			if err := saveCardInstance(ctx, q, st.UID, st.Card); err != nil {
				return err
			}
		}
		if kind != domain.ItemInstanceEquipment {
			continue
		}
		// 新获得的卡可能在第一次周期存档前就被镶入。此时它已不在背包位置
		// 中，必须从装备的孔关系补写卡实例根，否则重登后只剩模板号。
		for i := range st.Sockets {
			if st.Sockets[i] == 0 && st.SocketUIDs[i] == 0 {
				continue
			}
			if st.Sockets[i] <= 0 || st.SocketUIDs[i] <= 0 {
				return fmt.Errorf("store: 装备实例 %d 第%d孔模板与卡实例不成对", st.UID, i)
			}
			tag, err := q.Exec(ctx, `
				INSERT INTO item_instances(uid,item_id,instance_kind,created_source,bound,locked)
				VALUES($1,$2,$3,$4,$5,$6)
				ON CONFLICT(uid) DO UPDATE SET updated_at=now(),bound=EXCLUDED.bound,locked=EXCLUDED.locked
				WHERE item_instances.item_id=EXCLUDED.item_id
				  AND item_instances.instance_kind=EXCLUDED.instance_kind`,
				st.SocketUIDs[i], int32(st.Sockets[i]), int16(domain.ItemInstanceSocketCard), "socket", st.SocketCards[i].Bound, st.SocketCards[i].Locked)
			if err != nil {
				return fmt.Errorf("store: 写装备实例 %d 第%d孔卡实例: %w", st.UID, i, err)
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("store: 孔内卡实例UID冲突 uid=%d item=%d", st.SocketUIDs[i], st.Sockets[i])
			}
			if err := saveCardInstance(ctx, q, st.SocketUIDs[i], st.SocketCards[i]); err != nil {
				return err
			}
		}
		cards, attrs, values, modes := rolledArraysOf(st)
		wa, wv, wm := washArraysOf(st)
		if _, err := q.Exec(ctx, `
			INSERT INTO equipment_instances(uid,durability,max_durability,durability_wear_raw,
			 bound,locked,refine_level,socket_count,socket_item_ids,socket_instance_uids,
			 rolled_affix_count,rolled_card_ids,rolled_attrs,rolled_values,rolled_modes,
			 wash_quality,wash_count,wash_attrs,wash_values,wash_modes,fused_appearance_item_id,
			 fused_soul_item_id,fused_dragon_item_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
			ON CONFLICT(uid) DO UPDATE SET durability=EXCLUDED.durability,
			 max_durability=EXCLUDED.max_durability,durability_wear_raw=EXCLUDED.durability_wear_raw,
			 bound=EXCLUDED.bound,locked=EXCLUDED.locked,refine_level=EXCLUDED.refine_level,
			 socket_count=EXCLUDED.socket_count,socket_item_ids=EXCLUDED.socket_item_ids,
			 socket_instance_uids=EXCLUDED.socket_instance_uids,
			 rolled_affix_count=EXCLUDED.rolled_affix_count,rolled_card_ids=EXCLUDED.rolled_card_ids,
			 rolled_attrs=EXCLUDED.rolled_attrs,rolled_values=EXCLUDED.rolled_values,
			 rolled_modes=EXCLUDED.rolled_modes,wash_quality=EXCLUDED.wash_quality,
			 wash_count=EXCLUDED.wash_count,wash_attrs=EXCLUDED.wash_attrs,
			 wash_values=EXCLUDED.wash_values,wash_modes=EXCLUDED.wash_modes,
			 fused_appearance_item_id=EXCLUDED.fused_appearance_item_id,
			 fused_soul_item_id=EXCLUDED.fused_soul_item_id,
			 fused_dragon_item_id=EXCLUDED.fused_dragon_item_id`,
			st.UID, st.Durability, st.MaxDurability, st.DurabilityWearRaw, st.Bound, st.Locked,
			st.RefineLevel, int16(st.SocketCount), socketIDsOf(st), socketUIDsOf(st),
			int16(st.RolledAffixCount), cards, attrs, values, modes, int16(st.WashQuality),
			int16(st.WashCount), wa, wv, wm, int32(st.FusedAppearance), int32(st.FusedSoul), int32(st.FusedDragon)); err != nil {
			return fmt.Errorf("store: 写装备实例 %d: %w", st.UID, err)
		}
	}
	return nil
}

// loadItemInstances 一次批量读取位置内涉及的实例，避免登录时逐格查询。
func loadItemInstances(ctx context.Context, q querier, uids []int64) (map[int64]domain.Stack, error) {
	out := make(map[int64]domain.Stack, len(uids))
	if len(uids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `
		SELECT i.uid,i.item_id,i.instance_kind,COALESCE(e.bound,i.bound),COALESCE(e.locked,i.locked),e.uid IS NOT NULL,
		       COALESCE(e.durability,0),COALESCE(e.max_durability,0),
		       COALESCE(e.durability_wear_raw,0),COALESCE(e.refine_level,0),
		       COALESCE(e.socket_count,0),COALESCE(e.socket_item_ids,ARRAY[0,0,0,0,0]),
		       COALESCE(e.socket_instance_uids,ARRAY[0,0,0,0,0]::BIGINT[]),
		       COALESCE(e.rolled_affix_count,0),COALESCE(e.rolled_card_ids,ARRAY[0,0,0,0,0]),
		       COALESCE(e.rolled_attrs,ARRAY[0,0,0,0,0]),COALESCE(e.rolled_values,ARRAY[0,0,0,0,0]),
		       COALESCE(e.rolled_modes,ARRAY[0,0,0,0,0]),COALESCE(e.wash_quality,0),
		       COALESCE(e.wash_count,0),COALESCE(e.wash_attrs,ARRAY[0,0,0,0]),
		       COALESCE(e.wash_values,ARRAY[0,0,0,0]),COALESCE(e.wash_modes,ARRAY[0,0,0,0]),
		       COALESCE(e.fused_appearance_item_id,0),COALESCE(e.fused_soul_item_id,0),COALESCE(e.fused_dragon_item_id,0)
		  FROM item_instances i LEFT JOIN equipment_instances e ON e.uid=i.uid
		 WHERE i.uid=ANY($1)`, uids)
	if err != nil {
		return nil, fmt.Errorf("store: 批量查物品实例: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var st domain.Stack
		var kind, socketCount, rolledCount, washQuality, washCount int16
		var isEquipment bool
		var sockets, rolledCards, rolledAttrs, rolledValues, rolledModes []int32
		var socketUIDs []int64
		var washAttrs, washValues, washModes []int32
		if err := rows.Scan(&st.UID, &st.Item, &kind, &st.Bound, &st.Locked, &isEquipment,
			&st.Durability, &st.MaxDurability, &st.DurabilityWearRaw, &st.RefineLevel,
			&socketCount, &sockets, &socketUIDs, &rolledCount, &rolledCards, &rolledAttrs,
			&rolledValues, &rolledModes, &washQuality, &washCount, &washAttrs, &washValues,
			&washModes, &st.FusedAppearance, &st.FusedSoul, &st.FusedDragon); err != nil {
			return nil, fmt.Errorf("store: 读物品实例: %w", err)
		}
		st.Count = 1
		st.InstanceKind = domain.ItemInstanceKind(kind)
		if isEquipment {
			st.SocketCount, st.RolledAffixCount = uint8(socketCount), uint8(rolledCount)
			for i := range st.Sockets {
				if i < len(sockets) {
					st.Sockets[i] = domain.ItemID(sockets[i])
				}
				if i < len(socketUIDs) {
					st.SocketUIDs[i] = socketUIDs[i]
				}
				if i < len(rolledCards) {
					st.RolledAffixes[i].CardID = rolledCards[i]
				}
				if i < len(rolledAttrs) {
					st.RolledAffixes[i].Affix.Attr = rolledAttrs[i]
				}
				if i < len(rolledValues) {
					st.RolledAffixes[i].Affix.Value = rolledValues[i]
				}
				if i < len(rolledModes) {
					st.RolledAffixes[i].Affix.Mode = rolledModes[i]
				}
			}
			loadWashArrays(&st, washQuality, washCount, washAttrs, washValues, washModes)
		}
		out[st.UID] = st
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历物品实例: %w", err)
	}
	rows.Close()
	if err := loadCardInstances(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

func hydrateStack(cached domain.Stack, facts map[int64]domain.Stack) (domain.Stack, error) {
	if cached.UID <= 0 {
		return cached, nil
	}
	fact, ok := facts[cached.UID]
	if !ok {
		return cached, fmt.Errorf("store: 位置引用不存在的物品实例 uid=%d item=%d", cached.UID, cached.Item)
	}
	if fact.Item != cached.Item {
		return cached, fmt.Errorf("store: 位置与实例模板冲突 uid=%d location=%d instance=%d",
			cached.UID, cached.Item, fact.Item)
	}
	// 历史位置表没有 instance_kind 列；装备的类型可由模板确定，保留旧栈的
	// 零值以兼容既有快照。卡片则必须带回 kind=2，镶嵌入口据此拒绝普通物品。
	if fact.InstanceKind == domain.ItemInstanceEquipment && cached.InstanceKind == domain.ItemInstanceNone {
		fact.InstanceKind = domain.ItemInstanceNone
	}
	return fact, nil
}

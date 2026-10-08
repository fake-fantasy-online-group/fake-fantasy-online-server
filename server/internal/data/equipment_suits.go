package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 套装效果引用的是 ov_card.index（词条定义号），不是 ov_card.item_id
// （可放进背包的实体卡）。与现有装备词条使用同一份 PostgreSQL 配置。
func loadEquipmentSuits(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT id, suit_name, ARRAY[
		arm1_id,arm2_id,arm3_id,arm4_id,arm5_id,arm6_id,arm7_id,arm8_id,
		arm9_id,arm10_id,arm11_id,arm12_id,arm13_id,arm14_id,arm15_id,arm16_id,
		arm17_id,arm18_id,arm19_id,arm20_id,arm21_id,arm22_id,arm23_id,arm24_id]
		FROM gamedata.ov_excesuit ORDER BY row_no`)
	if err != nil {
		return fmt.Errorf("data: 查套装成员: %w", err)
	}
	suits := make(map[int32]*domain.EquipmentSuit)
	for rows.Next() {
		var suit domain.EquipmentSuit
		var ids []int32
		if err := rows.Scan(&suit.ID, &suit.Name, &ids); err != nil {
			rows.Close()
			return err
		}
		groups := make(map[int32]int)
		for _, id := range ids {
			if id == 0 {
				continue
			}
			def, ok := items[domain.ItemID(id)]
			if !ok || def.Equip == nil {
				rows.Close()
				return fmt.Errorf("data: 套装%d成员%d不存在", suit.ID, id)
			}
			slot := def.Equip.Slot
			if slot == int32(domain.SlotTwoHand) {
				slot = int32(domain.SlotWeapon)
			}
			index, ok := groups[slot]
			if !ok {
				index = len(suit.Groups)
				groups[slot] = index
				suit.Groups = append(suit.Groups, nil)
			}
			suit.Groups[index] = append(suit.Groups[index], def.ID)
			if def.Equip.Suit != nil {
				rows.Close()
				return fmt.Errorf("data: 装备%d属于多个套装", id)
			}
			def.Equip.Suit = &suit
		}
		suits[suit.ID] = &suit
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	rows, err = q.Query(ctx, `SELECT s.id, x.seq, e.attr_id, e.mode, e.prob, e.value, e.op_type,
		COALESCE(to_jsonb(d)->>('lv'||e.value||'_desc'), ''),
		COALESCE(to_jsonb(d)->>('lv'||e.value||'_name'), '')
		FROM gamedata.ov_excesuit s
		JOIN gamedata.ov_excesuit_entry x ON x.row_no=s.row_no
		LEFT JOIN gamedata.ov_card c ON c.index=x.ref_id
		LEFT JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no
		LEFT JOIN gamedata.ov_exceptdesc d ON d.type=e.attr_id AND e.mode=4
		ORDER BY s.row_no,x.idx,e.idx`)
	if err != nil {
		return fmt.Errorf("data: 查套装效果: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, op int32
		var bonus domain.EquipmentSuitBonus
		var name string
		if err := rows.Scan(&id, &bonus.Pieces, &bonus.Affix.Attr, &bonus.Affix.Mode,
			&bonus.Probability, &bonus.Affix.Value, &op, &bonus.Description, &name); err != nil {
			return fmt.Errorf("data: 套装效果引用不完整: %w", err)
		}
		suit := suits[id]
		if suit == nil || bonus.Pieces <= 0 || int(bonus.Pieces) > len(suit.Groups) {
			return fmt.Errorf("data: 套装%d件数门槛%d非法", id, bonus.Pieces)
		}
		if bonus.Description == "" {
			bonus.Description = name
		}
		if op != 1 || bonus.Probability != 100 || !domain.SupportsEquipmentSuitAffix(bonus.Affix) {
			bonus.DisabledReason = "效果尚未实现"
		}
		suit.Bonuses = append(suit.Bonuses, bonus)
	}
	return rows.Err()
}

package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

func socketIDsOf(s domain.Stack) []int32 {
	out := make([]int32, len(s.Sockets))
	for i, id := range s.Sockets {
		out[i] = int32(id)
	}
	return out
}

func washArraysOf(s domain.Stack) (attrs, values, modes []int32) {
	attrs, values, modes = make([]int32, 4), make([]int32, 4), make([]int32, 4)
	for i, affix := range s.WashAffixes {
		attrs[i], values[i], modes[i] = affix.Attr, affix.Value, affix.Mode
	}
	return
}

func loadWashArrays(st *domain.Stack, quality, count int16, attrs, values, modes []int32) {
	st.WashQuality, st.WashCount = uint8(quality), uint8(count)
	for i := range st.WashAffixes {
		if i < len(attrs) {
			st.WashAffixes[i].Attr = attrs[i]
		}
		if i < len(values) {
			st.WashAffixes[i].Value = values[i]
		}
		if i < len(modes) {
			st.WashAffixes[i].Mode = modes[i]
		}
	}
}

// 背包持久化。
//
// 铁律: **角色行与背包在同一个事务里写。**
// 分开写会出现"经验存了、物品没存"这种崩溃后的半截状态,
// 而那正是玩家会拿去申诉的那一类 —— 他记得自己打到了什么。

// LoadSkills 读一个角色学会的技能。没有时返回空表, 不是 nil。
func (p *Postgres) LoadSkills(ctx context.Context, charID int64) (domain.Learned, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT skill_id, level FROM character_skills WHERE char_id = $1`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查技能: %w", err)
	}
	defer rows.Close()

	out := domain.Learned{}
	for rows.Next() {
		var id, lv int32
		if err := rows.Scan(&id, &lv); err != nil {
			return nil, fmt.Errorf("store: 读技能行: %w", err)
		}
		out[domain.SkillID(id)] = lv
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历技能: %w", err)
	}
	return out, nil
}

func (t *pgTx) LoadSkills(ctx context.Context, charID int64) (domain.Learned, error) {
	return nil, fmt.Errorf("store: 事务内不支持读技能")
}

func saveSkills(ctx context.Context, tx pgx.Tx, charID int64, l domain.Learned) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_skills WHERE char_id = $1`, charID); err != nil {
		return fmt.Errorf("store: 清技能: %w", err)
	}
	var rows [][]any
	for id, lv := range l {
		// 战斗技能 0 级仍表示没学，不能存；生活技能的数值是
		// 熟练度，刚学会时的 0 必须保留，否则重登后会变回“未学习”。
		if lv > 0 || (lv == 0 && domain.IsLifeSkill(id)) {
			rows = append(rows, []any{charID, int32(id), lv})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_skills"},
		[]string{"char_id", "skill_id", "level"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: 写技能: %w", err)
	}
	return nil
}

// LoadQuests 读一个角色的任务本。没有时返回空表, 不是 nil。
func (p *Postgres) LoadQuests(ctx context.Context, charID int64) (domain.QuestLog, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT task_id, state, progress FROM character_quests WHERE char_id = $1`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查任务本: %w", err)
	}
	defer rows.Close()

	out := domain.QuestLog{}
	for rows.Next() {
		var id, state, progress int32
		if err := rows.Scan(&id, &state, &progress); err != nil {
			return nil, fmt.Errorf("store: 读任务行: %w", err)
		}
		out[domain.QuestID(id)] = domain.QuestEntry{
			State: domain.QuestState(state), Progress: progress}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历任务本: %w", err)
	}
	rows.Close()

	kills, err := p.pool.Query(ctx, `
		SELECT task_id, monster_id, progress
		  FROM character_quest_kills
		 WHERE char_id = $1`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查任务击杀进度: %w", err)
	}
	defer kills.Close()
	for kills.Next() {
		var taskID, monsterID, progress int32
		if err := kills.Scan(&taskID, &monsterID, &progress); err != nil {
			return nil, fmt.Errorf("store: 读任务击杀进度: %w", err)
		}
		entry, ok := out[domain.QuestID(taskID)]
		if !ok || entry.State == domain.QuestFinished || progress <= 0 {
			continue
		}
		if entry.Kills == nil {
			entry.Kills = map[domain.MonsterID]int32{}
		}
		entry.Kills[domain.MonsterID(monsterID)] = progress
		out[domain.QuestID(taskID)] = entry
	}
	if err := kills.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历任务击杀进度: %w", err)
	}
	return out, nil
}

func (t *pgTx) LoadQuests(ctx context.Context, charID int64) (domain.QuestLog, error) {
	return nil, fmt.Errorf("store: 事务内不支持读任务本")
}

func saveQuests(ctx context.Context, tx pgx.Tx, charID int64, l domain.QuestLog) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_quests WHERE char_id = $1`, charID); err != nil {
		return fmt.Errorf("store: 清任务本: %w", err)
	}
	ids := make([]domain.QuestID, 0, len(l))
	for id := range l {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var rows [][]any
	for _, id := range ids {
		e := l[id]
		rows = append(rows, []any{charID, int32(id), int32(e.State), e.Progress})
	}
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_quests"},
		[]string{"char_id", "task_id", "state", "progress"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: 写任务本: %w", err)
	}
	var killRows [][]any
	for _, id := range ids {
		e := l[id]
		monsters := make([]domain.MonsterID, 0, len(e.Kills))
		for monster, progress := range e.Kills {
			if progress > 0 && e.State != domain.QuestFinished {
				monsters = append(monsters, monster)
			}
		}
		sort.Slice(monsters, func(i, j int) bool { return monsters[i] < monsters[j] })
		for _, monster := range monsters {
			killRows = append(killRows, []any{charID, int32(id), int32(monster), e.Kills[monster]})
		}
	}
	if len(killRows) == 0 {
		return nil
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"character_quest_kills"},
		[]string{"char_id", "task_id", "monster_id", "progress"}, pgx.CopyFromRows(killRows)); err != nil {
		return fmt.Errorf("store: 写任务击杀进度: %w", err)
	}
	return nil
}

// LoadEquips 读一个角色身上穿的。没有任何行时返回空的一套, 不是 nil。
func (p *Postgres) LoadEquips(ctx context.Context, charID int64) (*domain.EquipSet, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT slot, uid, item_id, durability, max_durability, durability_wear_raw, bound, locked, refine_level, socket_count, sockets,
		        wash_quality, wash_count, wash_attrs, wash_values, wash_modes, fused_appearance_item_id
		   FROM character_equips WHERE char_id = $1`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查装备: %w", err)
	}
	defer rows.Close()

	worn := domain.NewEquipSet()
	var instanceUIDs []int64
	for rows.Next() {
		var slot int32
		var st domain.Stack
		var socketCount int16
		var socketIDs []int32
		var washQuality, washCount int16
		var washAttrs, washValues, washModes []int32
		if err := rows.Scan(&slot, &st.UID, &st.Item, &st.Durability,
			&st.MaxDurability, &st.DurabilityWearRaw, &st.Bound, &st.Locked, &st.RefineLevel,
			&socketCount, &socketIDs, &washQuality, &washCount,
			&washAttrs, &washValues, &washModes, &st.FusedAppearance); err != nil {
			return nil, fmt.Errorf("store: 读装备行: %w", err)
		}
		loadWashArrays(&st, washQuality, washCount, washAttrs, washValues, washModes)
		st.SocketCount = uint8(socketCount)
		for i := range st.Sockets {
			if i < len(socketIDs) {
				st.Sockets[i] = domain.ItemID(socketIDs[i])
			}
		}
		st.Count = 1 // 装备永远一件一格
		if st.UID > 0 {
			instanceUIDs = append(instanceUIDs, st.UID)
		}
		worn.Set(domain.EquipSlot(slot), st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历装备: %w", err)
	}
	facts, err := loadItemInstances(ctx, p.pool, instanceUIDs)
	if err != nil {
		return nil, err
	}
	var hydrateErr error
	worn.Each(func(slot domain.EquipSlot, cached domain.Stack) {
		if hydrateErr != nil || cached.UID <= 0 {
			return
		}
		var st domain.Stack
		st, hydrateErr = hydrateStack(cached, facts)
		if hydrateErr == nil {
			worn.Set(slot, st)
		}
	})
	if hydrateErr != nil {
		return nil, hydrateErr
	}
	return worn, nil
}

func (t *pgTx) LoadEquips(ctx context.Context, charID int64) (*domain.EquipSet, error) {
	return nil, fmt.Errorf("store: 事务内不支持读装备")
}

// LoadChangeSet 读取快速换装面板托管的备用装备。表内 cell 是客户端部位号，
// 物品真实穿戴槽仍在 game_equipment 中，切换时由场景重新复核。
func (p *Postgres) LoadChangeSet(ctx context.Context, charID int64) (*domain.ChangeSet, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT cell, uid, item_id, durability, max_durability, durability_wear_raw,
		        bound, locked, refine_level, socket_count, sockets,
		        wash_quality, wash_count, wash_attrs, wash_values, wash_modes,
		        fused_appearance_item_id
		   FROM character_change_set_items WHERE char_id = $1 ORDER BY cell`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查快速换装套装: %w", err)
	}
	defer rows.Close()
	set := domain.NewChangeSet()
	var instanceUIDs []int64
	for rows.Next() {
		var cell int32
		var st domain.Stack
		var socketCount, washQuality, washCount int16
		var socketIDs, washAttrs, washValues, washModes []int32
		if err := rows.Scan(&cell, &st.UID, &st.Item, &st.Durability,
			&st.MaxDurability, &st.DurabilityWearRaw, &st.Bound, &st.Locked,
			&st.RefineLevel, &socketCount, &socketIDs, &washQuality, &washCount,
			&washAttrs, &washValues, &washModes, &st.FusedAppearance); err != nil {
			return nil, fmt.Errorf("store: 读快速换装行: %w", err)
		}
		st.Count = 1
		st.SocketCount = uint8(socketCount)
		for i := range st.Sockets {
			if i < len(socketIDs) {
				st.Sockets[i] = domain.ItemID(socketIDs[i])
			}
		}
		loadWashArrays(&st, washQuality, washCount, washAttrs, washValues, washModes)
		if st.UID > 0 {
			instanceUIDs = append(instanceUIDs, st.UID)
		}
		if !set.Set(domain.EquipSlot(cell), st) {
			return nil, fmt.Errorf("store: 角色 %d 快速换装部位 %d 非法", charID, cell)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历快速换装套装: %w", err)
	}
	facts, err := loadItemInstances(ctx, p.pool, instanceUIDs)
	if err != nil {
		return nil, err
	}
	var hydrateErr error
	set.Each(func(cell domain.EquipSlot, cached domain.Stack) {
		if hydrateErr != nil || cached.UID <= 0 {
			return
		}
		var st domain.Stack
		st, hydrateErr = hydrateStack(cached, facts)
		if hydrateErr == nil {
			set.Set(cell, st)
		}
	})
	if hydrateErr != nil {
		return nil, hydrateErr
	}
	return set, nil
}

func (t *pgTx) LoadChangeSet(ctx context.Context, charID int64) (*domain.ChangeSet, error) {
	return nil, fmt.Errorf("store: 事务内不支持读快速换装套装")
}

func saveEquips(ctx context.Context, tx pgx.Tx, charID int64, worn *domain.EquipSet) error {
	var instances []domain.Stack
	worn.Each(func(_ domain.EquipSlot, st domain.Stack) { instances = append(instances, st) })
	if err := saveItemInstances(ctx, tx, "equipment", instances...); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM character_equips WHERE char_id = $1`, charID); err != nil {
		return fmt.Errorf("store: 清装备: %w", err)
	}
	var rows [][]any
	worn.Each(func(slot domain.EquipSlot, s domain.Stack) {
		wa, wv, wm := washArraysOf(s)
		rows = append(rows, []any{charID, int32(slot), s.UID, int32(s.Item), s.Durability,
			s.MaxDurability, s.DurabilityWearRaw, s.Bound, s.Locked, s.RefineLevel,
			int16(s.SocketCount), socketIDsOf(s), int16(s.WashQuality), int16(s.WashCount), wa, wv, wm,
			int32(s.FusedAppearance)})
	})
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_equips"},
		[]string{"char_id", "slot", "uid", "item_id", "durability", "max_durability", "durability_wear_raw", "bound", "locked", "refine_level", "socket_count", "sockets", "wash_quality", "wash_count", "wash_attrs", "wash_values", "wash_modes", "fused_appearance_item_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: 写装备: %w", err)
	}
	return nil
}

func saveChangeSet(ctx context.Context, tx pgx.Tx, charID int64, set *domain.ChangeSet) error {
	var instances []domain.Stack
	set.Each(func(_ domain.EquipSlot, st domain.Stack) { instances = append(instances, st) })
	if err := saveItemInstances(ctx, tx, "change_set", instances...); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM character_change_set_items WHERE char_id = $1`, charID); err != nil {
		return fmt.Errorf("store: 清快速换装套装: %w", err)
	}
	var rows [][]any
	set.Each(func(cell domain.EquipSlot, st domain.Stack) {
		wa, wv, wm := washArraysOf(st)
		rows = append(rows, []any{charID, int32(cell), st.UID, int32(st.Item), st.Durability,
			st.MaxDurability, st.DurabilityWearRaw, st.Bound, st.Locked, st.RefineLevel,
			int16(st.SocketCount), socketIDsOf(st), int16(st.WashQuality), int16(st.WashCount),
			wa, wv, wm, int32(st.FusedAppearance)})
	})
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_change_set_items"},
		[]string{"char_id", "cell", "uid", "item_id", "durability", "max_durability",
			"durability_wear_raw", "bound", "locked", "refine_level", "socket_count",
			"sockets", "wash_quality", "wash_count", "wash_attrs", "wash_values",
			"wash_modes", "fused_appearance_item_id"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: 写快速换装套装: %w", err)
	}
	return nil
}

// LoadBag 读一个角色的背包。没有任何物品行时返回空背包, 不是 nil。
func (p *Postgres) LoadBag(ctx context.Context, charID int64, slots int) (*domain.Bag, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT slot, uid, item_id, count, durability, max_durability, durability_wear_raw, bound, locked, refine_level, socket_count, sockets,
		        wash_quality, wash_count, wash_attrs, wash_values, wash_modes, fused_appearance_item_id
		   FROM character_items WHERE char_id = $1`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查背包: %w", err)
	}
	defer rows.Close()

	bag := domain.NewBag(slots)
	var instanceUIDs []int64
	for rows.Next() {
		var slot int
		var st domain.Stack
		var socketCount int16
		var socketIDs []int32
		var washQuality, washCount int16
		var washAttrs, washValues, washModes []int32
		if err := rows.Scan(&slot, &st.UID, &st.Item, &st.Count, &st.Durability,
			&st.MaxDurability, &st.DurabilityWearRaw, &st.Bound, &st.Locked, &st.RefineLevel,
			&socketCount, &socketIDs, &washQuality, &washCount,
			&washAttrs, &washValues, &washModes, &st.FusedAppearance); err != nil {
			return nil, fmt.Errorf("store: 读背包行: %w", err)
		}
		loadWashArrays(&st, washQuality, washCount, washAttrs, washValues, washModes)
		st.SocketCount = uint8(socketCount)
		for i := range st.Sockets {
			if i < len(socketIDs) {
				st.Sockets[i] = domain.ItemID(socketIDs[i])
			}
		}
		if st.UID > 0 {
			instanceUIDs = append(instanceUIDs, st.UID)
		}
		// 格号超出当前容量时丢弃并报警 —— 背包缩容(换了小包)会走到这。
		// 静默丢弃是不行的, 那等于吞了玩家的东西。
		if !bag.Set(slot, st) {
			return nil, fmt.Errorf("store: 角色 %d 的第 %d 格超出背包容量 %d, 拒绝加载",
				charID, slot, bag.Cap())
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历背包: %w", err)
	}
	facts, err := loadItemInstances(ctx, p.pool, instanceUIDs)
	if err != nil {
		return nil, err
	}
	var hydrateErr error
	bag.Each(func(slot int, cached domain.Stack) {
		if hydrateErr != nil || cached.UID <= 0 {
			return
		}
		var st domain.Stack
		st, hydrateErr = hydrateStack(cached, facts)
		if hydrateErr == nil {
			bag.Set(slot, st)
		}
	})
	if hydrateErr != nil {
		return nil, hydrateErr
	}
	return bag, nil
}

// LoadBag 在事务内暂不支持 —— 现在没有"事务里读背包"的用例,
// 真需要时(交易)会走一条显式加锁的路径, 不是这个。
func (t *pgTx) LoadBag(ctx context.Context, charID int64, slots int) (*domain.Bag, error) {
	return nil, fmt.Errorf("store: 事务内不支持读背包")
}

func (p *Postgres) LoadWarehouse(ctx context.Context, charID int64, rule domain.WarehouseRule) (*domain.Warehouse, error) {
	return loadWarehouse(ctx, p.pool, charID, rule)
}

func (t *pgTx) LoadWarehouse(ctx context.Context, charID int64, rule domain.WarehouseRule) (*domain.Warehouse, error) {
	return loadWarehouse(ctx, t.tx, charID, rule)
}

func (p *Postgres) LoadWardrobe(ctx context.Context, charID int64, rule domain.WardrobeRule) (*domain.Wardrobe, error) {
	return loadWardrobe(ctx, p.pool, charID, rule)
}

func (t *pgTx) LoadWardrobe(ctx context.Context, charID int64, rule domain.WardrobeRule) (*domain.Wardrobe, error) {
	return loadWardrobe(ctx, t.tx, charID, rule)
}

func loadWardrobe(ctx context.Context, q querier, charID int64, rule domain.WardrobeRule) (*domain.Wardrobe, error) {
	if !rule.Valid() {
		return nil, fmt.Errorf("store: 非法衣柜规则")
	}
	capacity := int32(0)
	err := q.QueryRow(ctx, `SELECT capacity FROM character_wardrobes WHERE char_id=$1`, charID).Scan(&capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: 查角色衣柜状态: %w", err)
	}
	if capacity < 0 || capacity > rule.MaxCapacity || capacity > 0 && capacity < rule.OpenCapacity {
		return nil, fmt.Errorf("store: 角色 %d 衣柜容量非法 %d", charID, capacity)
	}
	w := domain.NewWardrobe(capacity)
	rows, err := q.Query(ctx, `
		SELECT item_id,category,worn,position
		  FROM character_wardrobe_items
		 WHERE char_id=$1
		 ORDER BY position`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查角色衣柜物品: %w", err)
	}
	defer rows.Close()
	expectedPosition := int32(0)
	for rows.Next() {
		var entry domain.WardrobeEntry
		var category int16
		var position int32
		if err := rows.Scan(&entry.Item, &category, &entry.Worn, &position); err != nil {
			return nil, fmt.Errorf("store: 读角色衣柜行: %w", err)
		}
		entry.Category = domain.WardrobeCategory(category)
		if position != expectedPosition || !w.Restore(entry) {
			return nil, fmt.Errorf("store: 角色 %d 衣柜第 %d 项状态非法", charID, position)
		}
		expectedPosition++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历角色衣柜: %w", err)
	}
	return w, nil
}

func loadWarehouse(ctx context.Context, q querier, charID int64, rule domain.WarehouseRule) (*domain.Warehouse, error) {
	if !rule.Valid() {
		return nil, fmt.Errorf("store: 非法个人仓库规则")
	}
	pageCount := int16(rule.InitialPages)
	var money int64
	err := q.QueryRow(ctx,
		`SELECT page_count, money FROM character_warehouses WHERE char_id=$1`, charID).
		Scan(&pageCount, &money)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: 查个人仓库状态: %w", err)
	}
	if pageCount <= 0 || pageCount > 255 || money < 0 {
		return nil, fmt.Errorf("store: 角色 %d 个人仓库状态非法 pages=%d money=%d", charID, pageCount, money)
	}
	w := domain.NewWarehouse(uint8(pageCount), rule.SlotsPerPage, rule.MaxStack)
	if w == nil || !w.SetMoney(money) {
		return nil, fmt.Errorf("store: 构造角色 %d 个人仓库失败", charID)
	}
	rows, err := q.Query(ctx, `
		SELECT page, slot, uid, item_id, count, durability, max_durability,
		       durability_wear_raw, bound, locked, refine_level, socket_count, sockets,
		       wash_quality, wash_count, wash_attrs, wash_values, wash_modes, fused_appearance_item_id
		  FROM character_warehouse_items
		 WHERE char_id=$1
		 ORDER BY page, slot`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查个人仓库物品: %w", err)
	}
	defer rows.Close()
	var instanceUIDs []int64
	for rows.Next() {
		var page, slot int
		var st domain.Stack
		var socketCount, washQuality, washCount int16
		var socketIDs, washAttrs, washValues, washModes []int32
		if err := rows.Scan(&page, &slot, &st.UID, &st.Item, &st.Count, &st.Durability,
			&st.MaxDurability, &st.DurabilityWearRaw, &st.Bound, &st.Locked, &st.RefineLevel,
			&socketCount, &socketIDs, &washQuality, &washCount,
			&washAttrs, &washValues, &washModes, &st.FusedAppearance); err != nil {
			return nil, fmt.Errorf("store: 读个人仓库物品: %w", err)
		}
		if st.Count > rule.MaxStack {
			return nil, fmt.Errorf("store: 角色 %d 仓库 %d/%d 数量 %d 超过上限 %d",
				charID, page, slot, st.Count, rule.MaxStack)
		}
		st.SocketCount = uint8(socketCount)
		for i := range st.Sockets {
			if i < len(socketIDs) {
				st.Sockets[i] = domain.ItemID(socketIDs[i])
			}
		}
		loadWashArrays(&st, washQuality, washCount, washAttrs, washValues, washModes)
		if st.UID > 0 {
			instanceUIDs = append(instanceUIDs, st.UID)
		}
		if !w.Set(page, slot, st) {
			return nil, fmt.Errorf("store: 角色 %d 仓库格 %d/%d 越界", charID, page, slot)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历个人仓库物品: %w", err)
	}
	facts, err := loadItemInstances(ctx, q, instanceUIDs)
	if err != nil {
		return nil, err
	}
	var hydrateErr error
	w.Each(func(page, slot int, cached domain.Stack) {
		if hydrateErr != nil || cached.UID <= 0 {
			return
		}
		var st domain.Stack
		st, hydrateErr = hydrateStack(cached, facts)
		if hydrateErr == nil {
			w.Set(page, slot, st)
		}
	})
	if hydrateErr != nil {
		return nil, hydrateErr
	}
	return w, nil
}

// SaveSnapshot 把角色行与背包一起写。
//
// 背包用"整包重写"而不是增量: 先删后插。
// 增量更新要跟踪每一格的变化, 那套簿记出一次错就是刷物品漏洞;
// 一个角色最多几十格, 整包重写的代价完全可以接受。
func (p *Postgres) SaveSnapshot(ctx context.Context, snap domain.Snapshot) error {
	if err := validatePetItems(snap); err != nil {
		return err
	}
	if snap.Char == nil {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: 开事务: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := saveAccountCaiyu(ctx, tx, snap.Char); err != nil {
		return err
	}
	if err := saveChar(ctx, tx, snap.Char); err != nil {
		return err
	}
	if snap.Bag != nil {
		if err := saveBag(ctx, tx, snap.Char.ID, snap.Bag); err != nil {
			return err
		}
	}
	if snap.Worn != nil {
		if err := saveEquips(ctx, tx, snap.Char.ID, snap.Worn); err != nil {
			return err
		}
	}
	if snap.ChangeSet != nil {
		if err := saveChangeSet(ctx, tx, snap.Char.ID, snap.ChangeSet); err != nil {
			return err
		}
	}
	if snap.Warehouse != nil {
		if err := saveWarehouse(ctx, tx, snap.Char.ID, snap.Warehouse); err != nil {
			return err
		}
	}
	if snap.Wardrobe != nil {
		if err := saveWardrobe(ctx, tx, snap.Char.ID, snap.Wardrobe); err != nil {
			return err
		}
	}
	if err := saveStall(ctx, tx, snap.Char.ID, snap.Stall); err != nil {
		return err
	}
	if snap.Char.Skills != nil {
		if err := saveSkills(ctx, tx, snap.Char.ID, snap.Char.Skills); err != nil {
			return err
		}
	}
	if snap.Char.Quests != nil {
		if err := saveQuests(ctx, tx, snap.Char.ID, snap.Char.Quests); err != nil {
			return err
		}
	}
	// 宠物无条件写(哪怕是空的) —— 只在非空时写的话, 把最后一只宠放生
	// 这件事就永远存不进去, 重登它又回来了
	if err := savePets(ctx, tx, snap.Char.ID, snap.Char.Pets); err != nil {
		return err
	}
	if err := saveTitles(ctx, tx, snap.Char.ID, snap.Char.OwnedTitles); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: 提交存档: %w", err)
	}
	return nil
}

func saveBag(ctx context.Context, tx pgx.Tx, charID int64, bag *domain.Bag) error {
	var instances []domain.Stack
	bag.Each(func(_ int, st domain.Stack) { instances = append(instances, st) })
	if err := saveItemInstances(ctx, tx, "bag", instances...); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM character_items WHERE char_id = $1`, charID); err != nil {
		return fmt.Errorf("store: 清背包: %w", err)
	}
	var rows [][]any
	bag.Each(func(slot int, s domain.Stack) {
		wa, wv, wm := washArraysOf(s)
		rows = append(rows, []any{charID, slot, s.UID, int32(s.Item), s.Count, s.Durability,
			s.MaxDurability, s.DurabilityWearRaw, s.Bound, s.Locked, s.RefineLevel,
			int16(s.SocketCount), socketIDsOf(s), int16(s.WashQuality), int16(s.WashCount), wa, wv, wm,
			int32(s.FusedAppearance)})
	})
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_items"},
		[]string{"char_id", "slot", "uid", "item_id", "count", "durability", "max_durability", "durability_wear_raw", "bound", "locked", "refine_level", "socket_count", "sockets", "wash_quality", "wash_count", "wash_attrs", "wash_values", "wash_modes", "fused_appearance_item_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: 写背包: %w", err)
	}
	return nil
}

func saveWarehouse(ctx context.Context, tx pgx.Tx, charID int64, w *domain.Warehouse) error {
	if w == nil || w.PageCount() <= 0 || w.PageCount() > 255 || w.SlotsPerPage() <= 0 || w.Money() < 0 {
		return fmt.Errorf("store: 角色 %d 个人仓库状态非法", charID)
	}
	var instances []domain.Stack
	w.Each(func(_, _ int, st domain.Stack) { instances = append(instances, st) })
	if err := saveItemInstances(ctx, tx, "warehouse", instances...); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO character_warehouses(char_id,page_count,money)
		VALUES($1,$2,$3)
		ON CONFLICT(char_id) DO UPDATE SET page_count=EXCLUDED.page_count,money=EXCLUDED.money`,
		charID, w.PageCount(), w.Money()); err != nil {
		return fmt.Errorf("store: 写个人仓库状态: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM character_warehouse_items WHERE char_id=$1`, charID); err != nil {
		return fmt.Errorf("store: 清个人仓库物品: %w", err)
	}
	var records [][]any
	w.Each(func(page, slot int, st domain.Stack) {
		wa, wv, wm := washArraysOf(st)
		records = append(records, []any{charID, page, slot, st.UID, int32(st.Item), st.Count,
			st.Durability, st.MaxDurability, st.DurabilityWearRaw, st.Bound, st.Locked,
			st.RefineLevel, int16(st.SocketCount), socketIDsOf(st), int16(st.WashQuality),
			int16(st.WashCount), wa, wv, wm, int32(st.FusedAppearance)})
	})
	if len(records) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_warehouse_items"},
		[]string{"char_id", "page", "slot", "uid", "item_id", "count", "durability",
			"max_durability", "durability_wear_raw", "bound", "locked", "refine_level",
			"socket_count", "sockets", "wash_quality", "wash_count", "wash_attrs",
			"wash_values", "wash_modes", "fused_appearance_item_id"}, pgx.CopyFromRows(records))
	if err != nil {
		return fmt.Errorf("store: 写个人仓库物品: %w", err)
	}
	return nil
}

func saveWardrobe(ctx context.Context, tx pgx.Tx, charID int64, w *domain.Wardrobe) error {
	if w == nil || w.Capacity() < 0 || w.Count() > int(w.Capacity()) || w.Capacity() == 0 && w.Count() != 0 {
		return fmt.Errorf("store: 角色 %d 衣柜状态非法", charID)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM character_wardrobe_items WHERE char_id=$1`, charID); err != nil {
		return fmt.Errorf("store: 清角色衣柜物品: %w", err)
	}
	if w.Capacity() == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM character_wardrobes WHERE char_id=$1`, charID); err != nil {
			return fmt.Errorf("store: 清未开启角色衣柜: %w", err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO character_wardrobes(char_id,capacity)
		VALUES($1,$2)
		ON CONFLICT(char_id) DO UPDATE SET capacity=EXCLUDED.capacity`, charID, w.Capacity()); err != nil {
		return fmt.Errorf("store: 写角色衣柜状态: %w", err)
	}
	records := make([][]any, 0, w.Count())
	w.Each(func(position int, entry domain.WardrobeEntry) {
		records = append(records, []any{charID, int32(entry.Item), int16(entry.Category), entry.Worn, position})
	})
	if len(records) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_wardrobe_items"},
		[]string{"char_id", "item_id", "category", "worn", "position"}, pgx.CopyFromRows(records))
	if err != nil {
		return fmt.Errorf("store: 写角色衣柜物品: %w", err)
	}
	return nil
}

func saveTitles(ctx context.Context, tx pgx.Tx, charID int64, titles []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_titles WHERE char_id = $1`, charID); err != nil {
		return fmt.Errorf("store: 清理角色称号: %w", err)
	}
	if len(titles) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(titles))
	for _, title := range titles {
		rows = append(rows, []any{charID, title})
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"character_titles"},
		[]string{"char_id", "title"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: 写角色称号: %w", err)
	}
	return nil
}

func (t *pgTx) SaveSnapshot(ctx context.Context, snap domain.Snapshot) error {
	if err := validatePetItems(snap); err != nil {
		return err
	}
	if snap.Char == nil {
		return nil
	}
	if err := saveAccountCaiyu(ctx, t.tx, snap.Char); err != nil {
		return err
	}
	if err := saveChar(ctx, t.tx, snap.Char); err != nil {
		return err
	}
	if snap.Bag != nil {
		if err := saveBag(ctx, t.tx, snap.Char.ID, snap.Bag); err != nil {
			return err
		}
	}
	if snap.Worn != nil {
		if err := saveEquips(ctx, t.tx, snap.Char.ID, snap.Worn); err != nil {
			return err
		}
	}
	if snap.ChangeSet != nil {
		if err := saveChangeSet(ctx, t.tx, snap.Char.ID, snap.ChangeSet); err != nil {
			return err
		}
	}
	if snap.Warehouse != nil {
		if err := saveWarehouse(ctx, t.tx, snap.Char.ID, snap.Warehouse); err != nil {
			return err
		}
	}
	if snap.Wardrobe != nil {
		if err := saveWardrobe(ctx, t.tx, snap.Char.ID, snap.Wardrobe); err != nil {
			return err
		}
	}
	if err := saveStall(ctx, t.tx, snap.Char.ID, snap.Stall); err != nil {
		return err
	}
	if snap.Char.Skills != nil {
		if err := saveSkills(ctx, t.tx, snap.Char.ID, snap.Char.Skills); err != nil {
			return err
		}
	}
	if snap.Char.Quests != nil {
		if err := saveQuests(ctx, t.tx, snap.Char.ID, snap.Char.Quests); err != nil {
			return err
		}
	}
	if err := savePets(ctx, t.tx, snap.Char.ID, snap.Char.Pets); err != nil {
		return err
	}
	return saveTitles(ctx, t.tx, snap.Char.ID, snap.Char.OwnedTitles)
}

// LoadPets 读一个角色养的宠物。没有时返回 nil。
func (p *Postgres) LoadPets(ctx context.Context, charID int64) ([]domain.PetInstance, error) {
	return loadPets(ctx, p.pool, charID)
}

func (t *pgTx) LoadPets(ctx context.Context, charID int64) ([]domain.PetInstance, error) {
	return loadPets(ctx, t.tx, charID)
}

func loadPets(ctx context.Context, q querier, charID int64) ([]domain.PetInstance, error) {
	rows, err := q.Query(ctx, `
		SELECT inst_id, slot, pet_id, name, level, exp,
		       base_str, base_vit, base_int, base_spi, base_agi, base_dex,
		       hp, mp, trust, starve, hatched, gender, prefix_id, free_points, allocated_points, active_skill,
		       deployed, riding,
		       learn_item, learn_skill, learn_chance_bp, hatch_prefix_rate_pct,
		       transmog_model,transmog_until,riding_saddle,pp_ai_used,
		 COALESCE((SELECT item_uid FROM char_pet_items x WHERE x.char_id=char_pets.char_id AND x.inst_id=char_pets.inst_id),0)
		  FROM char_pets WHERE char_id = $1 ORDER BY slot, inst_id`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查宠物: %w", err)
	}
	defer rows.Close()

	var out []domain.PetInstance
	for rows.Next() {
		var it domain.PetInstance
		var gender int16
		if err := rows.Scan(&it.ID, &it.Slot, &it.Def, &it.Name, &it.Level, &it.Exp,
			&it.Base.STR, &it.Base.VIT, &it.Base.INT,
			&it.Base.SPI, &it.Base.AGI, &it.Base.DEX,
			&it.HP, &it.MP, &it.Trust, &it.Starve,
			&it.Hatched, &gender, &it.Prefix, &it.FreePoints, &it.AllocatedPoints, &it.ActiveSkill,
			&it.Deployed, &it.Riding,
			&it.PendingLearn.Item, &it.PendingLearn.Skill, &it.PendingLearn.ChanceBP,
			&it.HatchPrefixRatePct, &it.TransmogModel, &it.TransmogUntil, &it.RidingSaddle, &it.PPAiUsed, &it.ItemUID); err != nil {
			return nil, fmt.Errorf("store: 读宠物行: %w", err)
		}
		if (it.Hatched && gender != 0 && gender != 1) || (!it.Hatched && gender != int16(domain.PetGenderUnknown)) {
			return nil, fmt.Errorf("store: 宠物 %d 性别状态无效: hatched=%t gender=%d", it.ID, it.Hatched, gender)
		}
		it.Gender = uint8(gender)
		if it.ItemUID <= 0 {
			return nil, fmt.Errorf("pet %d missing item association", it.ID)
		}
		if !it.TransmogUntil.After(time.Unix(0, 0)) {
			it.TransmogUntil = time.Time{}
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	skillRows, err := q.Query(ctx, `
		SELECT inst_id, skill_id, level
		  FROM char_pet_skills WHERE char_id = $1 ORDER BY inst_id, skill_id`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查宠物技能: %w", err)
	}
	defer skillRows.Close()
	byInst := make(map[domain.PetInstID]*domain.PetInstance, len(out))
	for i := range out {
		byInst[out[i].ID] = &out[i]
	}
	for skillRows.Next() {
		var inst domain.PetInstID
		var skill domain.PetSkill
		if err := skillRows.Scan(&inst, &skill.ID, &skill.Level); err != nil {
			return nil, fmt.Errorf("store: 读宠物技能行: %w", err)
		}
		if pet := byInst[inst]; pet != nil {
			pet.Skills = append(pet.Skills, skill)
		}
	}
	if err := skillRows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历宠物技能: %w", err)
	}
	return out, nil
}

// savePets 写一个角色的宠物。**与角色行同事务** ——
// 分开写会留下"宠物升级存了、经验没存"的半截状态。
func savePets(ctx context.Context, tx pgx.Tx, charID int64, pets []domain.PetInstance) error {
	if _, err := tx.Exec(ctx, `DELETE FROM char_pets WHERE char_id = $1`, charID); err != nil {
		return fmt.Errorf("store: 清宠物: %w", err)
	}
	if len(pets) == 0 {
		return nil
	}
	normalizeStoredPetSlots(pets)
	rows := make([][]any, 0, len(pets))
	for _, it := range pets {
		transmogUntil := it.TransmogUntil
		if transmogUntil.IsZero() {
			transmogUntil = time.Unix(0, 0).UTC()
		}
		rows = append(rows, []any{charID, int64(it.ID), it.Slot, int32(it.Def), it.Name,
			it.Level, it.Exp,
			it.Base.STR, it.Base.VIT, it.Base.INT,
			it.Base.SPI, it.Base.AGI, it.Base.DEX,
			it.HP, it.MP, it.Trust, it.Starve,
			it.Hatched, int16(it.Gender), it.Prefix, it.FreePoints, it.AllocatedPoints, int32(it.ActiveSkill),
			it.Deployed, it.Riding,
			int32(it.PendingLearn.Item), int32(it.PendingLearn.Skill), it.PendingLearn.ChanceBP,
			it.HatchPrefixRatePct, it.TransmogModel, transmogUntil, int32(it.RidingSaddle), it.PPAiUsed})
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"char_pets"},
		[]string{"char_id", "inst_id", "slot", "pet_id", "name", "level", "exp",
			"base_str", "base_vit", "base_int", "base_spi", "base_agi", "base_dex",
			"hp", "mp", "trust", "starve", "hatched", "gender", "prefix_id", "free_points", "allocated_points", "active_skill",
			"deployed", "riding",
			"learn_item", "learn_skill", "learn_chance_bp", "hatch_prefix_rate_pct",
			"transmog_model", "transmog_until", "riding_saddle", "pp_ai_used"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: 写宠物: %w", err)
	}
	for _, it := range pets {
		if it.ItemUID > 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO char_pet_items(char_id,inst_id,item_uid) VALUES($1,$2,$3)`, charID, it.ID, it.ItemUID); err != nil {
				return fmt.Errorf("save pet item association: %w", err)
			}
		}
	}
	var skillRows [][]any
	for _, it := range pets {
		for _, skill := range it.Skills {
			skillRows = append(skillRows, []any{charID, int64(it.ID), int32(skill.ID), skill.Level})
		}
	}
	if len(skillRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"char_pet_skills"},
			[]string{"char_id", "inst_id", "skill_id", "level"}, pgx.CopyFromRows(skillRows)); err != nil {
			return fmt.Errorf("store: 写宠物技能: %w", err)
		}
	}
	return nil
}

func normalizeStoredPetSlots(pets []domain.PetInstance) {
	seen := make(map[int32]bool, len(pets))
	valid := true
	for i := range pets {
		if pets[i].Slot < 0 || pets[i].Slot >= domain.MaxPets || seen[pets[i].Slot] {
			valid = false
			break
		}
		seen[pets[i].Slot] = true
	}
	if valid {
		return
	}
	for i := range pets {
		pets[i].Slot = int32(i)
	}
}

// ── 好友 ──
//
// **双向存两行**，加/删都在一个事务里写两条 —— 半截状态会变成
// "我列表里有他、他列表里没我"，而那种不对称在游戏里表现为
// "他上线我收到通知，我上线他收不到"，极难对得上账。

// LoadFriends 读一个角色的好友列表。没有时返回 nil。
//
// 连着查一次角色表拿名字：好友列表要显示名字，而名字不在关系表里 ——
// 存一份冗余名字的话，对方改名之后你的列表就永远停在旧名字上。
func (p *Postgres) LoadFriends(ctx context.Context, charID int64) ([]domain.Friend, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT f.friend_id, c.name, f.remark
		  FROM char_friends f JOIN characters c ON c.id = f.friend_id
		 WHERE f.char_id = $1
		 ORDER BY f.created_at`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查好友: %w", err)
	}
	defer rows.Close()

	var out []domain.Friend
	for rows.Next() {
		var f domain.Friend
		if err := rows.Scan(&f.Char, &f.Name, &f.Remark); err != nil {
			return nil, fmt.Errorf("store: 读好友行: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// 好友操作**不在别人的事务里做**: 它自己就要写两行, 嵌进外层事务
// 会让"加好友"和无关的存档绑在一起, 一个失败拖垮另一个。
func (t *pgTx) LoadFriends(context.Context, int64) ([]domain.Friend, error) {
	return nil, fmt.Errorf("store: 事务内不支持读好友")
}
func (t *pgTx) AddFriend(context.Context, int64, int64) error {
	return fmt.Errorf("store: 事务内不支持加好友")
}
func (t *pgTx) RemoveFriend(context.Context, int64, int64) error {
	return fmt.Errorf("store: 事务内不支持删好友")
}
func (t *pgTx) SetFriendRemark(context.Context, int64, int64, string) error {
	return fmt.Errorf("store: 事务内不支持改好友备注")
}
func (t *pgTx) CountFriends(context.Context, int64) (int, error) {
	return 0, fmt.Errorf("store: 事务内不支持数好友")
}

// AddFriend 互加好友。**两行同事务写。**
//
// 用 ON CONFLICT DO NOTHING 而不是先查后插：并发下"查完还没插"的窗口里
// 对方也可能在加你，先查后插会撞主键。
func (p *Postgres) AddFriend(ctx context.Context, a, b int64) error {
	if a == b {
		return fmt.Errorf("store: 不能加自己为好友")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: 开事务: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO char_friends (char_id, friend_id) VALUES ($1,$2), ($2,$1)
		ON CONFLICT DO NOTHING`, a, b); err != nil {
		return fmt.Errorf("store: 加好友: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: 提交加好友: %w", err)
	}
	return nil
}

// RemoveFriend 互删好友。同样两行同事务。
//
// **单方面删除也是双向的**：留着"他还当我是好友"这半边，
// 只会让他一直收到我的上下线通知，而我这边看不到他。
func (p *Postgres) RemoveFriend(ctx context.Context, a, b int64) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: 开事务: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM char_friends
		 WHERE (char_id = $1 AND friend_id = $2)
		    OR (char_id = $2 AND friend_id = $1)`, a, b); err != nil {
		return fmt.Errorf("store: 删好友: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: 提交删好友: %w", err)
	}
	return nil
}

// SetFriendRemark 改备注。只改自己这一边 ——
// 备注是**我给他起的名字**，不该出现在他的列表里。
func (p *Postgres) SetFriendRemark(ctx context.Context, charID, friendID int64, remark string) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE char_friends SET remark = $3 WHERE char_id = $1 AND friend_id = $2`,
		charID, friendID, remark)
	if err != nil {
		return fmt.Errorf("store: 改好友备注: %w", err)
	}
	return nil
}

// CountFriends 数一个角色有多少好友。加好友前查上限用。
func (p *Postgres) CountFriends(ctx context.Context, charID int64) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx,
		`SELECT count(*) FROM char_friends WHERE char_id = $1`, charID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: 数好友: %w", err)
	}
	return n, nil
}

// LoadBlocks 读取单向屏蔽名单。名称始终从 characters 表联查，
// 角色改名后不会留下旧名副本。
func (p *Postgres) LoadBlocks(ctx context.Context, charID int64) ([]domain.BlockEntry, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT b.blocked_char_id, c.name, b.scope
		  FROM char_blocks b JOIN characters c ON c.id = b.blocked_char_id
		 WHERE b.char_id = $1
		 ORDER BY b.created_at, b.blocked_char_id`, charID)
	if err != nil {
		return nil, fmt.Errorf("store: 查屏蔽名单: %w", err)
	}
	defer rows.Close()
	var out []domain.BlockEntry
	for rows.Next() {
		var entry domain.BlockEntry
		var scope int16
		if err := rows.Scan(&entry.Char, &entry.Name, &scope); err != nil {
			return nil, fmt.Errorf("store: 读屏蔽行: %w", err)
		}
		entry.Scope = uint8(scope)
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (p *Postgres) UpsertBlock(ctx context.Context, charID, blockedCharID int64, scope uint8) error {
	if charID == blockedCharID || scope > 1 {
		return fmt.Errorf("store: 无效屏蔽关系")
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO char_blocks(char_id, blocked_char_id, scope)
		VALUES($1,$2,$3)
		ON CONFLICT (char_id, blocked_char_id)
		DO UPDATE SET scope=EXCLUDED.scope`, charID, blockedCharID, int16(scope))
	if err != nil {
		return fmt.Errorf("store: 写屏蔽关系: %w", err)
	}
	return nil
}

func (p *Postgres) RemoveBlock(ctx context.Context, charID, blockedCharID int64) error {
	_, err := p.pool.Exec(ctx,
		`DELETE FROM char_blocks WHERE char_id=$1 AND blocked_char_id=$2`,
		charID, blockedCharID)
	if err != nil {
		return fmt.Errorf("store: 删屏蔽关系: %w", err)
	}
	return nil
}

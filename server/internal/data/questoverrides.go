package data

import (
	"context"
	"fmt"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// QuestOverrideStats 记录经过运行时实体校验后启用的服务端任务规则。
type QuestOverrideStats struct {
	Tasks          int
	Disabled       int
	Steps          int
	SpecialDrops   int
	DropGroups     int
	DropGuaranteed int
	DropHigh       int
	DropMedium     int
	DropLow        int
}

type questOverrideFile struct {
	Tasks         []questStepTask     `json:"tasks"`
	DisabledTasks []questDisabledTask `json:"disabledTasks"`
	DropRates     []questDropRate     `json:"dropRates"`
}

type questDisabledTask struct {
	ID     int32  `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type questDropRate struct {
	Task        int32  `json:"task"`
	TaskName    string `json:"taskName"`
	Step        int    `json:"step"`
	Monster     int32  `json:"monster"`
	MonsterName string `json:"monsterName"`
	RatePct     int32  `json:"ratePct"`
	Tier        string `json:"tier"`
	Reason      string `json:"reason"`
}

func loadQuestOverrideFile(ctx context.Context, q Querier) (questOverrideFile, error) {
	var doc questOverrideFile
	tasks, err := loadQuestStepTasks(ctx, q, "verified")
	if err != nil {
		return doc, err
	}
	doc.Tasks = tasks

	rows, err := q.Query(ctx, `
		SELECT task_id, task_name, disabled_reason
		  FROM game_task_disabled
		 ORDER BY task_id`)
	if err != nil {
		return doc, fmt.Errorf("data: 查禁用任务: %w", err)
	}
	for rows.Next() {
		var row questDisabledTask
		if err := rows.Scan(&row.ID, &row.Name, &row.Reason); err != nil {
			rows.Close()
			return doc, fmt.Errorf("data: 读禁用任务: %w", err)
		}
		doc.DisabledTasks = append(doc.DisabledTasks, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return doc, fmt.Errorf("data: 遍历禁用任务: %w", err)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		SELECT task_id, task_name, step_no, monster_id, monster_name,
		       rate_pct, tier, reason
		  FROM game_task_drop_rates
		 ORDER BY task_id, step_no, monster_id`)
	if err != nil {
		return doc, fmt.Errorf("data: 查任务专属掉率: %w", err)
	}
	for rows.Next() {
		var row questDropRate
		if err := rows.Scan(&row.Task, &row.TaskName, &row.Step, &row.Monster,
			&row.MonsterName, &row.RatePct, &row.Tier, &row.Reason); err != nil {
			rows.Close()
			return doc, fmt.Errorf("data: 读任务专属掉率: %w", err)
		}
		doc.DropRates = append(doc.DropRates, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return doc, fmt.Errorf("data: 遍历任务专属掉率: %w", err)
	}
	rows.Close()
	return doc, nil
}

// ApplyQuestOverrides 用数据库中人工核实过的线性流程覆盖客户端缺失或被压平的步骤。
//
// 这层数据不是客户端原始资源，因而单独分层存放。整层数据会先转换并验证 NPC、
// 物品、怪物模板和怪物落位；任意一项无法闭合时一条都不应用，避免开出一半能做、
// 一半卡死的任务。
func ApplyQuestOverrides(ctx context.Context, q Querier, all domain.QuestTable, items ItemDefs,
	monsters MonsterDefs, spawns SpawnTable, npcs NPCTable, loot DropTable) (QuestOverrideStats, error) {
	var stats QuestOverrideStats
	doc, err := loadQuestOverrideFile(ctx, q)
	if err != nil {
		return stats, err
	}
	if len(doc.Tasks) == 0 {
		return stats, fmt.Errorf("data: 数据库 verified 任务流程层没有任务")
	}

	pending := make(map[domain.QuestID]domain.QuestDef, len(doc.Tasks))
	seen := make(map[domain.QuestID]struct{}, len(doc.Tasks))
	disabled := make(map[domain.QuestID]struct{}, len(doc.DisabledTasks))
	for _, raw := range doc.DisabledTasks {
		id := domain.QuestID(raw.ID)
		if id == 0 {
			return stats, fmt.Errorf("data: 禁用任务含 task_id=0")
		}
		if _, dup := disabled[id]; dup {
			return stats, fmt.Errorf("data: 禁用任务 task_id=%d 重复", id)
		}
		def, ok := all[id]
		if !ok {
			return stats, fmt.Errorf("data: 禁用任务 %d 不在运行时任务表", id)
		}
		if raw.Name == "" || raw.Name != def.Name {
			return stats, fmt.Errorf("data: 禁用任务 %d 名称 %q，运行时为 %q", id, raw.Name, def.Name)
		}
		if raw.Reason == "" {
			return stats, fmt.Errorf("data: 禁用任务 %d 缺少原因", id)
		}
		disabled[id] = struct{}{}
	}
	for _, raw := range doc.Tasks {
		id := domain.QuestID(raw.ID)
		if id == 0 {
			return stats, fmt.Errorf("data: 任务核实规则含 task_id=0")
		}
		if _, dup := seen[id]; dup {
			return stats, fmt.Errorf("data: 任务核实规则 task_id=%d 重复", id)
		}
		seen[id] = struct{}{}
		// 禁用是最终运行裁决。一个任务可能已有核实流程，但奖励、
		// NPC 或目标怪仍不可达；这时应跳过覆盖并在末尾统一删除，
		// 不能让一条已明确禁用的任务关停整个任务系统。
		if _, blocked := disabled[id]; blocked {
			continue
		}
		def, ok := all[id]
		if !ok {
			return stats, fmt.Errorf("data: 核实任务 %d 不在运行时任务表", id)
		}
		if raw.Name != "" && raw.Name != def.Name {
			return stats, fmt.Errorf("data: 核实任务 %d 名称 %q，运行时为 %q", id, raw.Name, def.Name)
		}
		if raw.Publisher == "" || raw.Publisher != def.NPC {
			return stats, fmt.Errorf("data: 核实任务 %d 发布者 %q，运行时为 %q", id, raw.Publisher, def.NPC)
		}
		if len(raw.Steps) == 0 {
			return stats, fmt.Errorf("data: 核实任务 %d 没有步骤", id)
		}

		steps := make([]domain.QuestStep, 0, len(raw.Steps))
		for i, row := range raw.Steps {
			step, err := convertQuestStep(id, i, row)
			if err != nil {
				return stats, err
			}
			wantDone := i == len(raw.Steps)-1
			if row.Done != wantDone {
				return stats, fmt.Errorf("data: 核实任务 %d 第 %d 步 done=%v，期望 %v", id, i+1, row.Done, wantDone)
			}
			steps = append(steps, step)
		}
		if err := preserveHardQuestGoals(def, steps); err != nil {
			return stats, err
		}
		def.AcceptGive = convertQuestItems(raw.AcceptGive)
		def.Steps = steps
		pending[id] = def
	}
	dropRates := doc.DropRates[:0]
	for _, row := range doc.DropRates {
		if _, blocked := disabled[domain.QuestID(row.Task)]; !blocked {
			dropRates = append(dropRates, row)
		}
	}
	if err := attachQuestDropRates(dropRates, pending, monsters); err != nil {
		return stats, err
	}

	spawnCounts := make(map[domain.MonsterID]int)
	for _, points := range spawns {
		for _, point := range points {
			spawnCounts[point.Monster]++
		}
	}
	npcExists := make(map[string]bool)
	for _, placements := range npcs {
		for _, npc := range placements {
			npcExists[npc.Name] = true
		}
	}

	ids := make([]int, 0, len(pending))
	for id := range pending {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, rawID := range ids {
		def := pending[domain.QuestID(rawID)]
		drops, err := validateQuestOverride(def, items, monsters, spawnCounts, npcExists, loot)
		if err != nil {
			return stats, err
		}
		stats.Tasks++
		stats.Steps += len(def.Steps)
		stats.SpecialDrops += drops.Relations
		stats.DropGroups += drops.Groups
		stats.DropGuaranteed += drops.Guaranteed
		stats.DropHigh += drops.High
		stats.DropMedium += drops.Medium
		stats.DropLow += drops.Low
	}

	for id, def := range pending {
		all[id] = def
	}
	for id := range disabled {
		delete(all, id)
		stats.Disabled++
	}
	return stats, nil
}

func attachQuestDropRates(rows []questDropRate, pending map[domain.QuestID]domain.QuestDef,
	monsters MonsterDefs) error {
	for _, row := range rows {
		id := domain.QuestID(row.Task)
		def, ok := pending[id]
		if !ok {
			return fmt.Errorf("data: 任务掉率引用未核实任务 %d", id)
		}
		if row.TaskName == "" || row.TaskName != def.Name {
			return fmt.Errorf("data: 任务掉率 %d 名称 %q，运行时为 %q", id, row.TaskName, def.Name)
		}
		if row.Step <= 0 || row.Step > len(def.Steps) {
			return fmt.Errorf("data: 任务掉率 %d 步骤 %d 越界", id, row.Step)
		}
		monsterID := domain.MonsterID(row.Monster)
		monster, ok := monsters[monsterID]
		if monsterID == 0 || !ok {
			return fmt.Errorf("data: 任务掉率 %d 怪物 %d 不存在", id, monsterID)
		}
		if row.MonsterName == "" || row.MonsterName != monster.Name {
			return fmt.Errorf("data: 任务掉率 %d 怪物 %d 名称 %q，运行时为 %q",
				id, monsterID, row.MonsterName, monster.Name)
		}
		step := &def.Steps[row.Step-1]
		step.Drops = append(step.Drops, domain.QuestDropRule{
			Monster: monsterID, RatePct: row.RatePct,
			Tier: domain.QuestDropTier(row.Tier), Reason: row.Reason,
		})
		pending[id] = def
	}
	return nil
}

func preserveHardQuestGoals(def domain.QuestDef, steps []domain.QuestStep) error {
	if def.GoalItem != 0 && def.GoalQty > 0 {
		var found int32
		for _, step := range steps {
			for _, goal := range step.RequiredItems() {
				if goal.Item == def.GoalItem && goal.Qty > found {
					found = goal.Qty
				}
			}
		}
		if found < def.GoalQty {
			return fmt.Errorf("data: 核实任务 %d 放松物品目标 %d：%d < %d",
				def.ID, def.GoalItem, found, def.GoalQty)
		}
	}
	for _, old := range def.Steps {
		for _, hard := range old.Kill {
			var found int32
			for _, step := range steps {
				for _, goal := range step.Kill {
					if goal.Monster == hard.Monster && goal.Qty > found {
						found = goal.Qty
					}
				}
			}
			if found < hard.Qty {
				return fmt.Errorf("data: 核实任务 %d 放松击杀目标 %d：%d < %d",
					def.ID, hard.Monster, found, hard.Qty)
			}
		}
	}
	return nil
}

type questDropValidationStats struct {
	Relations  int
	Groups     int
	Guaranteed int
	High       int
	Medium     int
	Low        int
}

func validateQuestOverride(def domain.QuestDef, items ItemDefs, monsters MonsterDefs,
	spawnCounts map[domain.MonsterID]int, npcExists map[string]bool, loot DropTable) (questDropValidationStats, error) {
	var dropStats questDropValidationStats
	if !npcExists[def.NPC] {
		return dropStats, fmt.Errorf("data: 核实任务 %d 发布 NPC %q 没有落位", def.ID, def.NPC)
	}
	available := make(map[domain.ItemID]int64)
	ordinaryDrop := make(map[domain.ItemID]bool)
	for monster, entries := range loot {
		if spawnCounts[monster] == 0 {
			continue
		}
		for _, entry := range entries {
			if entry.RatePct > 0 && entry.Item != 0 {
				ordinaryDrop[entry.Item] = true
			}
			for _, item := range entry.Choices {
				if entry.RatePct > 0 && item != 0 {
					ordinaryDrop[item] = true
				}
			}
		}
	}
	if err := validateGeneratedItems(def.ID, "接取给予", def.AcceptGive, items, available); err != nil {
		return dropStats, err
	}

	for index, step := range def.Steps {
		where := fmt.Sprintf("第 %d 步", index+1)
		if !npcExists[step.To] {
			return dropStats, fmt.Errorf("data: 核实任务 %d %s目标 NPC %q 没有落位", def.ID, where, step.To)
		}
		for _, goal := range step.Kill {
			monster, ok := monsters[goal.Monster]
			if goal.Monster == 0 || goal.Qty <= 0 || !ok {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s击杀怪物 %d 不存在", def.ID, where, goal.Monster)
			}
			if goal.Name != "" && goal.Name != monster.Name {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s怪物 %d 名称 %q，运行时为 %q",
					def.ID, where, goal.Monster, goal.Name, monster.Name)
			}
			if spawnCounts[goal.Monster] == 0 {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s怪物 %d 没有刷怪点", def.ID, where, goal.Monster)
			}
		}

		sourceTotals := make(map[domain.MonsterID]int32)
		for _, goal := range step.Collect {
			item, ok := items[goal.Item]
			if goal.Item == 0 || goal.Qty <= 0 || !ok {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s收集物品 %d 不存在", def.ID, where, goal.Item)
			}
			if goal.Name != "" && goal.Name != item.Name {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s物品 %d 名称 %q，运行时为 %q",
					def.ID, where, goal.Item, goal.Name, item.Name)
			}
			if len(goal.Sources) == 0 {
				if available[goal.Item] < int64(goal.Qty) && !ordinaryDrop[goal.Item] {
					return dropStats, fmt.Errorf("data: 核实任务 %d %s物品 %q 没有任务掉落或此前 NPC 给予来源",
						def.ID, where, item.Name)
				}
			} else {
				seen := make(map[domain.MonsterID]bool, len(goal.Sources))
				for _, source := range goal.Sources {
					monster, ok := monsters[source]
					if source == 0 || !ok {
						return dropStats, fmt.Errorf("data: 核实任务 %d %s物品 %q 的来源怪物 %d 不存在",
							def.ID, where, item.Name, source)
					}
					if seen[source] {
						return dropStats, fmt.Errorf("data: 核实任务 %d %s物品 %q 重复来源怪物 %d",
							def.ID, where, item.Name, source)
					}
					seen[source] = true
					if spawnCounts[source] == 0 {
						return dropStats, fmt.Errorf("data: 核实任务 %d %s物品 %q 的来源怪物 %d(%s)没有刷怪点",
							def.ID, where, item.Name, source, monster.Name)
					}
					sourceTotals[source] += goal.Qty
				}
				available[goal.Item] += int64(goal.Qty)
				dropStats.Relations++
			}
		}
		if err := validateQuestDropRules(def, where, step, sourceTotals, monsters, spawnCounts, &dropStats); err != nil {
			return dropStats, err
		}
		for _, goal := range step.Take {
			item, ok := items[goal.Item]
			if goal.Item == 0 || goal.Qty <= 0 || !ok {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s交付物品 %d 不存在", def.ID, where, goal.Item)
			}
			if goal.Name != "" && goal.Name != item.Name {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s物品 %d 名称 %q，运行时为 %q",
					def.ID, where, goal.Item, goal.Name, item.Name)
			}
			if len(goal.Sources) != 0 {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s交付物品 %q 不应配置当前怪物来源",
					def.ID, where, item.Name)
			}
		}

		for _, goal := range step.RequiredItems() {
			if available[goal.Item] < int64(goal.Qty) && !ordinaryDrop[goal.Item] {
				return dropStats, fmt.Errorf("data: 核实任务 %d %s所需物品 %d 数量没有可达来源",
					def.ID, where, goal.Item)
			}
			if !ordinaryDrop[goal.Item] {
				available[goal.Item] -= int64(goal.Qty)
			}
		}
		if err := validateGeneratedItems(def.ID, where+" NPC 给予", step.Give, items, available); err != nil {
			return dropStats, err
		}
	}
	return dropStats, nil
}

func validateQuestDropRules(def domain.QuestDef, where string, step domain.QuestStep,
	sourceTotals map[domain.MonsterID]int32, monsters MonsterDefs,
	spawnCounts map[domain.MonsterID]int, stats *questDropValidationStats) error {
	seen := make(map[domain.MonsterID]bool, len(step.Drops))
	for _, rule := range step.Drops {
		if seen[rule.Monster] {
			return fmt.Errorf("data: 核实任务 %d %s怪物 %d 重复任务掉率", def.ID, where, rule.Monster)
		}
		seen[rule.Monster] = true
		total := sourceTotals[rule.Monster]
		if total <= 0 {
			return fmt.Errorf("data: 核实任务 %d %s怪物 %d 的掉率没有匹配收集物品", def.ID, where, rule.Monster)
		}
		monster, ok := monsters[rule.Monster]
		if !ok || spawnCounts[rule.Monster] == 0 {
			return fmt.Errorf("data: 核实任务 %d %s掉率怪物 %d 不可达", def.ID, where, rule.Monster)
		}
		if rule.Reason == "" {
			return fmt.Errorf("data: 核实任务 %d %s怪物 %d 掉率缺少理由", def.ID, where, rule.Monster)
		}
		guaranteed := monster.Kind == domain.MonsterBoss || spawnCounts[rule.Monster] == 1
		switch rule.Tier {
		case domain.QuestDropGuaranteed:
			if !guaranteed || rule.RatePct != 100 {
				return fmt.Errorf("data: 核实任务 %d %s怪物 %d 必掉档不满足 BOSS/唯一落位或概率不是100%%",
					def.ID, where, rule.Monster)
			}
			stats.Guaranteed++
		case domain.QuestDropHigh:
			if guaranteed || total < 8 || rule.RatePct != 85 {
				return fmt.Errorf("data: 核实任务 %d %s怪物 %d 高掉率档要求非唯一、需求>=8且概率85%%",
					def.ID, where, rule.Monster)
			}
			stats.High++
		case domain.QuestDropMedium:
			if guaranteed || total >= 8 || rule.RatePct != 50 {
				return fmt.Errorf("data: 核实任务 %d %s怪物 %d 中掉率档要求非唯一、需求<8且概率50%%",
					def.ID, where, rule.Monster)
			}
			stats.Medium++
		case domain.QuestDropLow:
			if guaranteed || total != 1 || rule.RatePct != 20 {
				return fmt.Errorf("data: 核实任务 %d %s怪物 %d 低掉率档要求非唯一、单件且概率20%%",
					def.ID, where, rule.Monster)
			}
			stats.Low++
		default:
			return fmt.Errorf("data: 核实任务 %d %s怪物 %d 掉率档 %q 不支持",
				def.ID, where, rule.Monster, rule.Tier)
		}
		stats.Groups++
	}
	for source := range sourceTotals {
		if !seen[source] {
			return fmt.Errorf("data: 核实任务 %d %s来源怪物 %d 没有显式任务掉率", def.ID, where, source)
		}
	}
	return nil
}

func validateGeneratedItems(task domain.QuestID, where string, goals []domain.QuestItemGoal,
	items ItemDefs, available map[domain.ItemID]int64) error {
	for _, goal := range goals {
		item, ok := items[goal.Item]
		if goal.Item == 0 || goal.Qty <= 0 || !ok {
			return fmt.Errorf("data: 核实任务 %d %s物品 %d 不存在", task, where, goal.Item)
		}
		if goal.Name != "" && goal.Name != item.Name {
			return fmt.Errorf("data: 核实任务 %d %s物品 %d 名称 %q，运行时为 %q",
				task, where, goal.Item, goal.Name, item.Name)
		}
		if len(goal.Sources) != 0 {
			return fmt.Errorf("data: 核实任务 %d %s物品 %q 不应配置怪物来源", task, where, item.Name)
		}
		available[goal.Item] += int64(goal.Qty)
	}
	return nil
}

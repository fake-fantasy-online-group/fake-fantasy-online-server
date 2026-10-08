package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// QuestStepStats 记录客户端分步数据与服务端任务表的合并结果。
type QuestStepStats struct {
	SourceTasks int
	Applied     int
	Extra       int
	Steps       int
}

type questStepFile struct {
	Tasks []questStepTask `json:"tasks"`
}

type questStepTask struct {
	ID         int32           `json:"id"`
	Name       string          `json:"name"`
	Publisher  string          `json:"publisher"`
	AcceptGive []questStepItem `json:"acceptGive"`
	Steps      []questStepRow  `json:"steps"`
}

type questStepRow struct {
	To      string          `json:"to"`
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Say     string          `json:"say"`
	Done    bool            `json:"done"`
	Collect []questStepItem `json:"collect"`
	Kill    []questStepItem `json:"kill"`
	Take    []questStepItem `json:"take"`
	Give    []questStepItem `json:"give"`
}

type questStepItem struct {
	ID      int32   `json:"id"`
	Qty     int32   `json:"n"`
	Name    string  `json:"name"`
	Sources []int32 `json:"sourceMonsters"`
}

// loadQuestStepTasks 从规范化数据库表读取一个任务流程层。base 是客户端提取的
// 基础流程，verified 是人工闭环核实后的服务端覆盖；两层都只在开服时全量读取。
func loadQuestStepTasks(ctx context.Context, q Querier, layer string) ([]questStepTask, error) {
	if layer != "base" && layer != "verified" {
		return nil, fmt.Errorf("data: 不支持的任务流程层 %q", layer)
	}
	rows, err := q.Query(ctx, `
		SELECT task_id, task_name, publisher
		  FROM game_task_flows
		 WHERE layer = $1
		 ORDER BY task_id`, layer)
	if err != nil {
		return nil, fmt.Errorf("data: 查 %s 任务流程: %w", layer, err)
	}
	tasks := make([]questStepTask, 0)
	index := make(map[int32]int)
	for rows.Next() {
		var task questStepTask
		if err := rows.Scan(&task.ID, &task.Name, &task.Publisher); err != nil {
			rows.Close()
			return nil, fmt.Errorf("data: 读 %s 任务流程: %w", layer, err)
		}
		if _, dup := index[task.ID]; dup {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务流程 task_id=%d 重复", layer, task.ID)
		}
		index[task.ID] = len(tasks)
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("data: 遍历 %s 任务流程: %w", layer, err)
	}
	rows.Close()

	taskAt := func(taskID int32) (*questStepTask, error) {
		i, ok := index[taskID]
		if !ok {
			return nil, fmt.Errorf("data: %s 任务流程子表引用不存在任务 %d", layer, taskID)
		}
		return &tasks[i], nil
	}

	rows, err = q.Query(ctx, `
		SELECT task_id, ordinal, item_id, qty, item_name
		  FROM game_task_flow_accept_items
		 WHERE layer = $1
		 ORDER BY task_id, ordinal`, layer)
	if err != nil {
		return nil, fmt.Errorf("data: 查 %s 接取给予: %w", layer, err)
	}
	for rows.Next() {
		var taskID, ordinal int32
		var item questStepItem
		if err := rows.Scan(&taskID, &ordinal, &item.ID, &item.Qty, &item.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("data: 读 %s 接取给予: %w", layer, err)
		}
		task, err := taskAt(taskID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if ordinal != int32(len(task.AcceptGive)+1) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 接取给予序号不连续: %d", layer, taskID, ordinal)
		}
		task.AcceptGive = append(task.AcceptGive, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("data: 遍历 %s 接取给予: %w", layer, err)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		SELECT task_id, step_no, target_npc, kind, text, say, done
		  FROM game_task_flow_steps
		 WHERE layer = $1
		 ORDER BY task_id, step_no`, layer)
	if err != nil {
		return nil, fmt.Errorf("data: 查 %s 任务步骤: %w", layer, err)
	}
	for rows.Next() {
		var taskID, stepNo int32
		var step questStepRow
		if err := rows.Scan(&taskID, &stepNo, &step.To, &step.Type, &step.Text,
			&step.Say, &step.Done); err != nil {
			rows.Close()
			return nil, fmt.Errorf("data: 读 %s 任务步骤: %w", layer, err)
		}
		task, err := taskAt(taskID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if stepNo != int32(len(task.Steps)+1) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 步骤序号不连续: %d", layer, taskID, stepNo)
		}
		task.Steps = append(task.Steps, step)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("data: 遍历 %s 任务步骤: %w", layer, err)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		SELECT task_id, step_no, phase, ordinal, item_id, qty, item_name
		  FROM game_task_flow_step_items
		 WHERE layer = $1
		 ORDER BY task_id, step_no, phase, ordinal`, layer)
	if err != nil {
		return nil, fmt.Errorf("data: 查 %s 步骤物品: %w", layer, err)
	}
	for rows.Next() {
		var taskID, stepNo, ordinal int32
		var phase string
		var item questStepItem
		if err := rows.Scan(&taskID, &stepNo, &phase, &ordinal,
			&item.ID, &item.Qty, &item.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("data: 读 %s 步骤物品: %w", layer, err)
		}
		task, err := taskAt(taskID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if stepNo <= 0 || int(stepNo) > len(task.Steps) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 步骤物品引用步骤 %d 越界", layer, taskID, stepNo)
		}
		step := &task.Steps[stepNo-1]
		var target *[]questStepItem
		switch phase {
		case "collect":
			target = &step.Collect
		case "take":
			target = &step.Take
		case "give":
			target = &step.Give
		default:
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 步骤 %d 物品阶段 %q 不支持",
				layer, taskID, stepNo, phase)
		}
		if ordinal != int32(len(*target)+1) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 步骤 %d %s 物品序号不连续: %d",
				layer, taskID, stepNo, phase, ordinal)
		}
		*target = append(*target, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("data: 遍历 %s 步骤物品: %w", layer, err)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		SELECT task_id, step_no, phase, item_ordinal, source_ordinal, monster_id
		  FROM game_task_flow_item_sources
		 WHERE layer = $1
		 ORDER BY task_id, step_no, phase, item_ordinal, source_ordinal`, layer)
	if err != nil {
		return nil, fmt.Errorf("data: 查 %s 任务物品来源: %w", layer, err)
	}
	for rows.Next() {
		var taskID, stepNo, itemOrdinal, sourceOrdinal, monsterID int32
		var phase string
		if err := rows.Scan(&taskID, &stepNo, &phase, &itemOrdinal,
			&sourceOrdinal, &monsterID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("data: 读 %s 任务物品来源: %w", layer, err)
		}
		task, err := taskAt(taskID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if stepNo <= 0 || int(stepNo) > len(task.Steps) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 物品来源引用步骤 %d 越界", layer, taskID, stepNo)
		}
		step := &task.Steps[stepNo-1]
		var items []questStepItem
		switch phase {
		case "collect":
			items = step.Collect
		case "take":
			items = step.Take
		case "give":
			items = step.Give
		default:
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 物品来源阶段 %q 不支持", layer, taskID, phase)
		}
		if itemOrdinal <= 0 || int(itemOrdinal) > len(items) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 步骤 %d 物品来源序号 %d 越界",
				layer, taskID, stepNo, itemOrdinal)
		}
		item := &items[itemOrdinal-1]
		if sourceOrdinal != int32(len(item.Sources)+1) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 步骤 %d 来源序号不连续: %d",
				layer, taskID, stepNo, sourceOrdinal)
		}
		item.Sources = append(item.Sources, monsterID)
		// items 是底层切片的副本，元素写入仍会回到 step 对应的切片。
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("data: 遍历 %s 任务物品来源: %w", layer, err)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		SELECT task_id, step_no, ordinal, monster_id, qty, monster_name
		  FROM game_task_flow_kills
		 WHERE layer = $1
		 ORDER BY task_id, step_no, ordinal`, layer)
	if err != nil {
		return nil, fmt.Errorf("data: 查 %s 击杀目标: %w", layer, err)
	}
	for rows.Next() {
		var taskID, stepNo, ordinal int32
		var goal questStepItem
		if err := rows.Scan(&taskID, &stepNo, &ordinal, &goal.ID, &goal.Qty, &goal.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("data: 读 %s 击杀目标: %w", layer, err)
		}
		task, err := taskAt(taskID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if stepNo <= 0 || int(stepNo) > len(task.Steps) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 击杀目标引用步骤 %d 越界", layer, taskID, stepNo)
		}
		step := &task.Steps[stepNo-1]
		if ordinal != int32(len(step.Kill)+1) {
			rows.Close()
			return nil, fmt.Errorf("data: %s 任务 %d 步骤 %d 击杀目标序号不连续: %d",
				layer, taskID, stepNo, ordinal)
		}
		step.Kill = append(step.Kill, goal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("data: 遍历 %s 击杀目标: %w", layer, err)
	}
	rows.Close()
	return tasks, nil
}

// ApplyQuestSteps 用数据库中的 base 流程覆盖 ov_task 的扁平兜底目标。
// 数值奖励仍来自 ov_task；这里只接入流程、目标 NPC 与中间任务物品。
func ApplyQuestSteps(ctx context.Context, q Querier, all domain.QuestTable) (QuestStepStats, error) {
	var stats QuestStepStats
	tasks, err := loadQuestStepTasks(ctx, q, "base")
	if err != nil {
		return stats, err
	}
	doc := questStepFile{Tasks: tasks}
	stats.SourceTasks = len(doc.Tasks)
	seen := make(map[domain.QuestID]struct{}, len(doc.Tasks))
	pending := make(map[domain.QuestID]domain.QuestDef, len(all))
	for _, raw := range doc.Tasks {
		id := domain.QuestID(raw.ID)
		if id == 0 {
			return stats, fmt.Errorf("data: 任务步骤含 task_id=0")
		}
		if _, dup := seen[id]; dup {
			return stats, fmt.Errorf("data: 任务步骤 task_id=%d 重复", id)
		}
		seen[id] = struct{}{}
		def, ok := all[id]
		if !ok {
			stats.Extra++
			continue
		}
		if len(raw.Steps) == 0 {
			return stats, fmt.Errorf("data: 任务 %d %q 没有步骤", id, raw.Name)
		}
		if raw.Publisher == "" {
			return stats, fmt.Errorf("data: 任务 %d %q 没有发布者", id, raw.Name)
		}
		steps := make([]domain.QuestStep, 0, len(raw.Steps))
		for i, row := range raw.Steps {
			step, err := convertQuestStep(id, i, row)
			if err != nil {
				return stats, err
			}
			// 客户端数据的 done 精确落在最后一步；锁住这个结构，避免数据版本
			// 漂移后服务端仍按“最后一行完成”悄悄发错奖励。
			wantDone := i == len(raw.Steps)-1
			if row.Done != wantDone {
				return stats, fmt.Errorf("data: 任务 %d 第 %d 步 done=%v，期望 %v", id, i+1, row.Done, wantDone)
			}
			steps = append(steps, step)
		}
		// ov_task 的 5 组物品/5 组怪物是服务端判定的硬目标；
		// task_steps 主要补流程、步骤类型与目标 NPC。少量版本差异不能
		// 用后者直接覆盖前者，否则会放松真实的击杀/收集条件。
		if len(def.Steps) == 1 {
			mergeQuestObjectives(steps, def.Steps[0])
		}
		if def.Exchange != nil {
			if len(steps) != 1 || steps[0].Kind != domain.QuestStepExchange ||
				len(steps[0].Collect) != len(def.Exchange.Items) {
				return stats, fmt.Errorf("data: 按量兑换任务 %d 的流程与兑换规则不一致", id)
			}
			for i, item := range def.Exchange.Items {
				if steps[0].Collect[i].Item != item.Item || steps[0].Collect[i].Qty != 0 {
					return stats, fmt.Errorf("data: 按量兑换任务 %d 的第 %d 件物品与流程不一致", id, i+1)
				}
			}
		}
		def.AcceptGive = convertQuestItems(raw.AcceptGive)
		def.NPC = raw.Publisher
		def.Steps = steps
		pending[id] = def
		stats.Applied++
		stats.Steps += len(steps)
	}
	if stats.Applied != len(all) {
		return stats, fmt.Errorf("data: 任务步骤只覆盖 %d/%d 个运行时任务", stats.Applied, len(all))
	}
	// 所有运行时任务都通过结构校验后再一次性替换，避免文件后半段
	// 损坏时留下“前半部已开启、后半部 fail-closed”的混合规则。
	for id, def := range pending {
		all[id] = def
	}
	return stats, nil
}

// mergeQuestObjectives 把 ov_task 的扁平终态目标合并进结构化步骤。
// 同 id 目标留在步骤文件指定的位置，数量取更严格值；步骤文件
// 遗漏的硬目标附加到最后一步，在发奖前必须全部达成。
func mergeQuestObjectives(steps []domain.QuestStep, raw domain.QuestStep) {
	if len(steps) == 0 {
		return
	}
	for _, goal := range raw.Collect {
		found := false
		for i := range steps {
			for j := range steps[i].Collect {
				if steps[i].Collect[j].Item == goal.Item {
					if steps[i].Collect[j].Qty < goal.Qty {
						steps[i].Collect[j].Qty = goal.Qty
					}
					found = true
				}
			}
			for j := range steps[i].Take {
				if steps[i].Take[j].Item == goal.Item {
					if steps[i].Take[j].Qty < goal.Qty {
						steps[i].Take[j].Qty = goal.Qty
					}
					found = true
				}
			}
		}
		if !found {
			last := len(steps) - 1
			steps[last].Collect = append(steps[last].Collect, goal)
		}
	}
	for _, goal := range raw.Kill {
		found := false
		for i := range steps {
			for j := range steps[i].Kill {
				if steps[i].Kill[j].Monster == goal.Monster {
					if steps[i].Kill[j].Qty < goal.Qty {
						steps[i].Kill[j].Qty = goal.Qty
					}
					found = true
				}
			}
		}
		if !found {
			last := len(steps) - 1
			steps[last].Kill = append(steps[last].Kill, goal)
		}
	}
}

func convertQuestStep(task domain.QuestID, index int, row questStepRow) (domain.QuestStep, error) {
	step := domain.QuestStep{
		Kind:    domain.QuestStepKind(row.Type),
		To:      row.To,
		Text:    row.Text,
		Say:     row.Say,
		Collect: convertQuestItems(row.Collect),
		Take:    convertQuestItems(row.Take),
		Give:    convertQuestItems(row.Give),
	}
	if step.To == "" {
		return domain.QuestStep{}, fmt.Errorf("data: 任务 %d 第 %d 步没有目标 NPC", task, index+1)
	}
	if !step.Known() || step.Kind == domain.QuestStepObjective {
		return domain.QuestStep{}, fmt.Errorf("data: 任务 %d 第 %d 步类型 %q 不支持", task, index+1, row.Type)
	}
	for _, goal := range row.Kill {
		if goal.ID <= 0 {
			return domain.QuestStep{}, fmt.Errorf("data: 任务 %d 第 %d 步怪物 id=%d 非法", task, index+1, goal.ID)
		}
		step.Kill = append(step.Kill, domain.QuestMonsterGoal{
			Monster: domain.MonsterID(goal.ID), Qty: goal.Qty, Name: goal.Name,
		})
	}
	return step, nil
}

func convertQuestItems(in []questStepItem) []domain.QuestItemGoal {
	out := make([]domain.QuestItemGoal, 0, len(in))
	for _, item := range in {
		if item.ID == 0 {
			continue
		}
		goal := domain.QuestItemGoal{
			Item: domain.ItemID(item.ID), Qty: item.Qty, Name: item.Name,
		}
		for _, source := range item.Sources {
			if source != 0 {
				goal.Sources = append(goal.Sources, domain.MonsterID(source))
			}
		}
		out = append(out, goal)
	}
	return out
}

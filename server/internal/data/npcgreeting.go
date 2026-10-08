package data

import (
	"context"
	"fmt"
)

// NPCGreetingStats 是用原始 Hints 补齐服务端 NPC 问候时的对账数据。
type NPCGreetingStats struct {
	Maps     int
	NPCs     int
	Matched  int
	NoSource int
}

// AttachNPCGreetings 用 PostgreSQL 中保留的原始 Hints 给地图 NPC 落位补上专属问候。
//
// Hints 是五元组序列。这里只采用前四项全为 0 的无条件项；带等级、任务或职业
// 条件的项在条件语义完全闭合前不能冒充默认台词。找不到无条件项的 NPC 留空，
// 由领域层按 NPC 职能生成稳定兜底句。
func AttachNPCGreetings(ctx context.Context, q Querier, catalog *MapCatalog, table NPCTable) (NPCGreetingStats, error) {
	byID := make(map[int32]string)
	textRows, err := q.Query(ctx, `SELECT id,text FROM game_npc_greeting_text ORDER BY id`)
	if err != nil {
		return NPCGreetingStats{}, fmt.Errorf("data: 查 game_npc_greeting_text: %w", err)
	}
	for textRows.Next() {
		var id int32
		var text string
		if err := textRows.Scan(&id, &text); err != nil {
			textRows.Close()
			return NPCGreetingStats{}, fmt.Errorf("data: 读 game_npc_greeting_text 行: %w", err)
		}
		byID[id] = text
	}
	if err := textRows.Err(); err != nil {
		textRows.Close()
		return NPCGreetingStats{}, fmt.Errorf("data: 遍历 game_npc_greeting_text: %w", err)
	}
	textRows.Close()
	if len(byID) == 0 {
		return NPCGreetingStats{}, fmt.Errorf("data: game_npc_greeting_text 为空")
	}

	hintsByResource := make(map[string]map[int32][]int32)
	hintRows, err := q.Query(ctx, `SELECT resource_name,npc_id,hints FROM game_map_npc_hints ORDER BY resource_name,npc_id`)
	if err != nil {
		return NPCGreetingStats{}, fmt.Errorf("data: 查 game_map_npc_hints: %w", err)
	}
	for hintRows.Next() {
		var resource string
		var id int32
		var hints []int32
		if err := hintRows.Scan(&resource, &id, &hints); err != nil {
			hintRows.Close()
			return NPCGreetingStats{}, fmt.Errorf("data: 读 game_map_npc_hints 行: %w", err)
		}
		if hintsByResource[resource] == nil {
			hintsByResource[resource] = make(map[int32][]int32)
		}
		hintsByResource[resource][id] = hints
	}
	if err := hintRows.Err(); err != nil {
		hintRows.Close()
		return NPCGreetingStats{}, fmt.Errorf("data: 遍历 game_map_npc_hints: %w", err)
	}
	hintRows.Close()
	if len(hintsByResource) == 0 {
		return NPCGreetingStats{}, fmt.Errorf("data: game_map_npc_hints 为空")
	}

	stats := NPCGreetingStats{}
	for mapID, placements := range table {
		def, ok := catalog.ByID(mapID)
		if !ok {
			stats.NoSource += len(placements)
			continue
		}
		stats.Maps++
		for i := range placements {
			stats.NPCs++
			hints, found := hintsByResource[def.File][placements[i].NPCID]
			if !found {
				stats.NoSource++
				continue
			}
			text := unconditionalNPCGreeting(hints, byID)
			if text == "" {
				continue
			}
			placements[i].Greeting = text
			stats.Matched++
		}
		table[mapID] = placements
	}
	return stats, nil
}

func unconditionalNPCGreeting(hints []int32, byID map[int32]string) string {
	for i := 0; i+4 < len(hints); i += 5 {
		if hints[i] != 0 || hints[i+1] != 0 || hints[i+2] != 0 || hints[i+3] != 0 {
			continue
		}
		if text := byID[hints[i+4]]; text != "" {
			return text
		}
	}
	return ""
}

package data

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Only monster templates already admitted by LoadMonsters can receive family
// metadata. Noncombat rows cannot gain skill eligibility through this table.
func loadMonsterFamilies(ctx context.Context, q Querier, defs MonsterDefs) error {
	for id, def := range defs {
		def.Family = domain.MonsterFamilyUnknown
		defs[id] = def
	}
	rows, err := q.Query(ctx, `SELECT monster_id, family FROM game_monster_families ORDER BY monster_id`)
	if err != nil {
		return fmt.Errorf("data: 查怪物族群: %w", err)
	}
	defer rows.Close()
	seen := map[domain.MonsterID]bool{}
	for rows.Next() {
		var id domain.MonsterID
		var f domain.MonsterFamily
		if err := rows.Scan(&id, &f); err != nil {
			return fmt.Errorf("data: 读怪物族群: %w", err)
		}
		if !f.Valid() || seen[id] {
			return fmt.Errorf("data: invalid or duplicate monster family %d/%q", id, f)
		}
		seen[id] = true
		def, ok := defs[id]
		if !ok || !def.Kind.Hostile() {
			continue
		}
		def.Family = f
		defs[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历怪物族群: %w", err)
	}
	return nil
}

func loadSkillTargetFamilies(ctx context.Context, q Querier, table domain.SkillTable) error {
	rows, err := q.Query(ctx, `SELECT skill_id, family FROM game_skill_target_families ORDER BY skill_id, family`)
	if err != nil {
		return fmt.Errorf("data: 查技能目标族群: %w", err)
	}
	defer rows.Close()
	rules := map[domain.SkillID][]domain.MonsterFamily{}
	for rows.Next() {
		var id domain.SkillID
		var f domain.MonsterFamily
		if err := rows.Scan(&id, &f); err != nil {
			return fmt.Errorf("data: 读技能目标族群: %w", err)
		}
		if !f.Valid() || f == domain.MonsterFamilyUnknown {
			return fmt.Errorf("data: invalid target family %d/%q", id, f)
		}
		for _, existing := range rules[id] {
			if existing == f {
				return fmt.Errorf("data: duplicate target family %d/%q", id, f)
			}
		}
		rules[id] = append(rules[id], f)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历技能目标族群: %w", err)
	}
	for key, def := range table {
		def.TargetFamilies = append([]domain.MonsterFamily(nil), rules[def.ID]...)
		table[key] = def
	}
	return nil
}

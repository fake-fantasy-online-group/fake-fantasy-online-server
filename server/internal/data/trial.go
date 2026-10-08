package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func LoadTrial(ctx context.Context, q Querier, defs MonsterDefs, dungeons domain.DungeonTable) (*domain.TrialRules, error) {
	r := &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{}, Monsters: map[int32]map[domain.MonsterID]domain.MonsterDef{}}
	// 试炼怪会按关卡改写 Kind（小怪一律普通、boss 一律 BOSS），染色档位必须跟着
	// 改写后的类别走，不能沿用原型模板的档位。
	colorProfiles, err := loadMonsterColorProfiles(ctx, q)
	if err != nil {
		return nil, err
	}
	head, err := q.Query(ctx, `SELECT normal_count,boss_count,quest_id,min_level,max_level,anchor_level,anchor_units,boss_offset,penalty_gap,penalty_per_level,first_pct,reward_chance,reward_min,reward_max,pill_id,fragment_id FROM game_trial_rules WHERE id=1`)
	if err != nil {
		return nil, err
	}
	if !head.Next() {
		head.Close()
		return nil, fmt.Errorf("missing trial rules")
	}
	err = head.Scan(&r.NormalCount, &r.BossCount, &r.Quest, &r.MinLevel, &r.MaxLevel, &r.AnchorLevel, &r.AnchorUnits, &r.BossOffset, &r.PenaltyGap, &r.PenaltyPerLevel, &r.FirstPct, &r.RewardChance, &r.RewardMin, &r.RewardMax, &r.Pill, &r.Fragment)
	head.Close()
	if err != nil {
		return nil, err
	}
	if r.NormalCount < 1 || r.BossCount < 1 || r.MinLevel < 1 || r.MaxLevel < r.AnchorLevel || r.AnchorLevel <= r.MinLevel || r.AnchorUnits <= 0 || r.BossOffset < 0 || r.PenaltyGap < 0 || r.PenaltyPerLevel < 0 || r.FirstPct <= 0 || r.RewardChance < 0 || r.RewardChance > 100 || r.RewardMin < 1 || r.RewardMax < r.RewardMin {
		return nil, fmt.Errorf("invalid trial rules")
	}
	rows, err := q.Query(ctx, `SELECT cycle,experience_pct FROM game_trial_rounds ORDER BY cycle`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cycle, pct int32
		if err = rows.Scan(&cycle, &pct); err != nil {
			rows.Close()
			return nil, err
		}
		if int(cycle) != len(r.Multipliers) || pct <= 0 {
			rows.Close()
			return nil, fmt.Errorf("invalid trial cycle")
		}
		r.Multipliers = append(r.Multipliers, pct)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(r.Multipliers) != 6 {
		return nil, fmt.Errorf("trial requires six cycles")
	}
	rows, err = q.Query(ctx, `SELECT e.claim_map,e.entry_map,e.claim_npc,e.entry_npc,n.x,n.y,e.price FROM game_trial_entrances e JOIN map_defs m ON m.id=e.entry_map JOIN map_npcs n ON n.map_file=m.file AND n.name=e.entry_npc ORDER BY e.claim_map`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e domain.TrialEntrance
		if err = rows.Scan(&e.ClaimMap, &e.EntryMap, &e.ClaimNPC, &e.EntryNPC, &e.EntryPos.X, &e.EntryPos.Y, &e.Price); err != nil {
			rows.Close()
			return nil, err
		}
		if e.Price < 0 || e.EntryPos.X <= 0 || e.EntryPos.Y <= 0 {
			rows.Close()
			return nil, fmt.Errorf("invalid trial entrance")
		}
		e.EntryPos.MapID = e.EntryMap
		r.Entrances = append(r.Entrances, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(r.Entrances) != 4 {
		return nil, fmt.Errorf("trial requires four entrances")
	}
	rows, err = q.Query(ctx, `SELECT t.monster_id,t.level,t.hp,t.atk_min,t.atk_max,t.matk,t.def,t.mdef,t.hit,t.boss,a.id,a.view_dist,a.trace_dist,a.basic_attack_dist,a.keep_dist,a.help_radius,a.skill_check_ms,a.global_cooldown_ms FROM game_trial_monster_stats t JOIN game_monster_ai_templates a ON a.id=t.ai_template ORDER BY t.level,t.monster_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id domain.MonsterID
		var level, hp, amin, amax, matk, def, mdef, hit int32
		var boss bool
		var ai domain.MonsterAIProfile
		var skillMS, cooldownMS int32
		if err = rows.Scan(&id, &level, &hp, &amin, &amax, &matk, &def, &mdef, &hit, &boss, &ai.ID, &ai.ViewDist, &ai.TraceDist, &ai.BasicAttackDist, &ai.KeepDist, &ai.HelpRadius, &skillMS, &cooldownMS); err != nil {
			rows.Close()
			return nil, err
		}
		d, ok := defs[id]
		if !ok || hp <= 0 || level < r.MinLevel || level > r.MaxLevel+r.BossOffset {
			rows.Close()
			return nil, fmt.Errorf("invalid trial monster %d level %d", id, level)
		}
		ai.SkillCheckEvery = domain.Ticks(int(skillMS))
		ai.GlobalCooldown = domain.Ticks(int(cooldownMS))
		d.AI = ai
		d.ViewDist = ai.ViewDist
		d.TraceDist = ai.TraceDist
		d.Level = level
		d.HP = hp
		d.Exp = 0
		d.EliteID = 0
		d.Kind = domain.MonsterNormal
		if boss {
			d.Kind = domain.MonsterBoss
		}
		d.ColorProfile = colorProfileForKind(colorProfiles, d.Kind.String())
		d.Aggressive = true
		d.NoAttack = false
		crit, mcrit := d.Stats.CritRate, d.Stats.MCritRate
		d.Stats = domain.NewMonsterStatsRange(amin, amax, def, hit, matk, mdef, d.Stats.AtkSpeedMS, d.Stats.MoveSpeed)
		d.Stats.CritRate, d.Stats.MCritRate = crit, mcrit
		if r.Monsters[level] == nil {
			r.Monsters[level] = map[domain.MonsterID]domain.MonsterDef{}
		}
		r.Monsters[level][id] = d
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT map_id,spawn_id,x,y,dir,candidates FROM game_trial_spawns ORDER BY map_id,spawn_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p domain.TrialSpawn
		var ids []int32
		if err = rows.Scan(&p.Point.Pos.MapID, &p.Point.ID, &p.Point.Pos.X, &p.Point.Pos.Y, &p.Point.Dir, &ids); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := dungeons[p.Point.Pos.MapID]; !ok || len(ids) == 0 {
			rows.Close()
			return nil, fmt.Errorf("invalid trial spawn")
		}
		for _, id := range ids {
			for l := r.MinLevel; l <= r.MaxLevel+r.BossOffset; l++ {
				if _, ok := r.Monsters[l][domain.MonsterID(id)]; !ok {
					rows.Close()
					return nil, fmt.Errorf("missing trial monster stats %d/%d", id, l)
				}
			}
			p.Candidates = append(p.Candidates, domain.MonsterID(id))
		}
		r.Maps[p.Point.Pos.MapID] = append(r.Maps[p.Point.Pos.MapID], p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(r.Maps) != 5 {
		return nil, fmt.Errorf("trial requires five maps")
	}
	for mapID, points := range r.Maps {
		normal, boss := 0, 0
		for _, p := range points {
			d := r.Monsters[r.MinLevel][p.Candidates[0]]
			if d.Kind == domain.MonsterBoss {
				boss++
			} else {
				normal++
			}
		}
		if normal < int(r.NormalCount) || boss < int(r.BossCount) {
			return nil, fmt.Errorf("trial map %d has insufficient spawn positions", mapID)
		}
	}
	return r, nil
}

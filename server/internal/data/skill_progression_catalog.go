package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadSkillProgressionCatalog reuses the native first-rank identity and learning
// requirements. Only membership/branch, one explicit prerequisite exception,
// synergy coefficients and bounded growth policy live in the new typed tables.
func LoadSkillProgressionCatalog(ctx context.Context, q Querier, skills domain.SkillTable) (*domain.SkillProgressionCatalog, error) {
	in := domain.SkillProgressionData{Policies: map[domain.SkillID]domain.SkillGrowthPolicy{}}
	manifest, err := q.Query(ctx, `SELECT expected_skills,expected_synergies,expected_prerequisite_overrides,
 expected_restored_statuses,expected_restored_applications,
 (SELECT count(*) FROM game_custom_statuses WHERE configuration_version=62),
 (SELECT count(*) FROM game_skill_statuses WHERE configuration_version=62)
 FROM game_skill_configuration_manifest WHERE id=1`)
	if err != nil {
		return nil, fmt.Errorf("data: skill configuration manifest (migration62): %w", err)
	}
	var expectedPrerequisites, expectedStatuses, expectedApps, statuses, apps int
	if !manifest.Next() {
		manifest.Close()
		return nil, fmt.Errorf("data: skill configuration manifest missing")
	}
	err = manifest.Scan(&in.ExpectedSkills, &in.ExpectedSynergies, &expectedPrerequisites, &expectedStatuses, &expectedApps, &statuses, &apps)
	if err != nil {
		manifest.Close()
		return nil, err
	}
	if err = manifest.Err(); err != nil {
		manifest.Close()
		return nil, err
	}
	manifest.Close()
	if statuses != expectedStatuses || apps != expectedApps {
		return nil, fmt.Errorf("data: native skill restoration coverage incomplete statuses=%d/%d applications=%d/%d", statuses, expectedStatuses, apps, expectedApps)
	}
	rows, err := q.Query(ctx, `SELECT m.skill_id,m.branch,m.use_native_prerequisites,
 p.skill_id IS NOT NULL,COALESCE(p.attribute,0),COALESCE(p.attribute_cap_rank,0),
 COALESCE(p.efficiency_cap_rank,0),COALESCE(p.range_cap_rank,0),
 COALESCE(p.protection_cap_rank,0),COALESCE(p.attack_cap_rank,0),COALESCE(p.attack_targets,'{}'::integer[])
 FROM game_skill_progression m LEFT JOIN game_skill_growth_policy p USING(skill_id) ORDER BY m.skill_id`)
	if err != nil {
		return nil, fmt.Errorf("data: skill progression configuration (migration 62): %w", err)
	}
	native := map[domain.SkillID]bool{}
	byID := map[domain.SkillID]int{}
	for rows.Next() {
		var d domain.SkillIdentity
		var p domain.SkillGrowthPolicy
		var useNative, hasPolicy bool
		if err = rows.Scan(&d.ID, &d.Branch, &useNative, &hasPolicy, &p.Attribute, &p.AttributeCapRank, &p.EfficiencyCapRank, &p.RangeCapRank, &p.ProtectionCapRank, &p.AttackCapRank, &p.AttackTargets); err != nil {
			rows.Close()
			return nil, err
		}
		first, ok := skills[domain.SkillKey{ID: d.ID, Level: 1}]
		if !ok || !hasPolicy {
			rows.Close()
			return nil, fmt.Errorf("data: skill %d missing original identity or growth policy", d.ID)
		}
		d.Name, d.Profession, d.UnlockLevel = first.Name, first.Prof, first.LevelNeed
		if useNative {
			d.Prerequisites = append([]domain.SkillID(nil), first.Prerequisites...)
		}
		native[d.ID] = useNative
		byID[d.ID] = len(in.Identities)
		in.Identities = append(in.Identities, d)
		in.Policies[d.ID] = p
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT skill_id,ordinal,prerequisite_id FROM game_skill_prerequisite_overrides ORDER BY skill_id,ordinal`)
	if err != nil {
		return nil, err
	}
	prerequisites := 0
	for rows.Next() {
		var id, dep domain.SkillID
		var ord int
		if err = rows.Scan(&id, &ord, &dep); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := byID[id]
		if !ok || native[id] || ord != len(in.Identities[i].Prerequisites)+1 {
			rows.Close()
			return nil, fmt.Errorf("invalid explicit skill prerequisite %d/%d", id, ord)
		}
		in.Identities[i].Prerequisites = append(in.Identities[i].Prerequisites, dep)
		prerequisites++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if prerequisites != expectedPrerequisites {
		return nil, fmt.Errorf("data: missing prerequisite overrides %d/%d", prerequisites, expectedPrerequisites)
	}
	rows, err = q.Query(ctx, `SELECT target_skill_id,source_skill_id,rank_mode,effect,per_point_bp FROM game_skill_synergies ORDER BY edge_order`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var edge domain.SkillSynergy
		if err = rows.Scan(&edge.Target, &edge.Source, &edge.RankMode, &edge.Effect, &edge.PerPointBP); err != nil {
			rows.Close()
			return nil, err
		}
		in.Synergies = append(in.Synergies, edge)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return domain.NewSkillProgressionCatalog(in)
}

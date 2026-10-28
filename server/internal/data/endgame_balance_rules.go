package data

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loadEndgameRuleActive(ctx context.Context, q Querier, table string) (int32, error) {
	rows, err := q.Query(ctx, "SELECT version FROM "+table+" WHERE id=1")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, fmt.Errorf("missing active %s", table)
	}
	var v int32
	if err = rows.Scan(&v); err != nil {
		return 0, err
	}
	if rows.Next() {
		return 0, fmt.Errorf("duplicate active %s", table)
	}
	return v, rows.Err()
}
func LoadLootRules(ctx context.Context, q Querier, items map[domain.ItemID]domain.ItemDef) (*domain.LootRuleTable, error) {
	active, err := loadEndgameRuleActive(ctx, q, "game_loot_rule_active")
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT jsonb_build_object('Version',r.version,'LegendaryPowerPolicy',COALESCE(to_jsonb(r)->'legendary_power_policy','0'::jsonb),'LegendaryPowerRolls',COALESCE(to_jsonb(r)->'legendary_power_rolls','false'::jsonb),'RetiredCubeMaterials',COALESCE(to_jsonb(r)->'retired_cube_materials','false'::jsonb),'MinRuneLevel',r.min_rune_level,'UniqueBasePPB',r.unique_base_ppb,'UniqueMFBoost',r.unique_mf_boost,'UniqueMFDivisor',r.unique_mf_divisor,'OutdoorEquipment',to_jsonb(r)->'outdoor_equipment','OutdoorNormalEquipment',to_jsonb(r)->'outdoor_normal_equipment','RunePool',(SELECT jsonb_agg(item_id ORDER BY seq) FROM game_loot_rune_pool p WHERE p.version=r.version),'RuneWeights',(SELECT CASE WHEN bool_and(weight=1) THEN NULL ELSE jsonb_agg(weight ORDER BY seq) END FROM game_loot_rune_pool p WHERE p.version=r.version),'SocketLevels',(SELECT jsonb_agg(jsonb_build_object('MinLevel',min_level,'Cap',cap) ORDER BY min_level) FROM game_loot_socket_levels s WHERE s.version=r.version),'Grades',(SELECT jsonb_agg(jsonb_build_object('Grade',grade,'RuneRateBP',rune_rate_bp,'SocketWeights',socket_weights) ORDER BY grade) FROM game_loot_grades g WHERE g.version=r.version))::text FROM game_loot_rule_versions r WHERE published ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []domain.LootRuleSet
	for rows.Next() {
		var text string
		var r domain.LootRuleSet
		if err = rows.Scan(&text); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(text), &r); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return domain.NewLootRuleTable(active, all, items)
}
func LoadRiftRules(ctx context.Context, q Querier, loot *domain.LootRuleTable, items map[domain.ItemID]domain.ItemDef) (*domain.RiftRuleTable, error) {
	active, err := loadEndgameRuleActive(ctx, q, "game_rift_rule_active")
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT (jsonb_build_object('WingRewards',to_jsonb(r)->'wing_rewards','GuardianHPDivisor',COALESCE(NULLIF(to_jsonb(r)->'guardian_hp_divisor','null'::jsonb),'0'::jsonb),'CumulativeLegendaryPerTier',COALESCE(NULLIF(to_jsonb(r)->'cumulative_legendary_per_tier','null'::jsonb),'0'::jsonb),'FloorLayoutVersion',COALESCE(to_jsonb(r)->'floor_layout_version','0'::jsonb),'GuardianRecentWindow',COALESCE(to_jsonb(r)->'guardian_recent_window','0'::jsonb),'EquipmentPools',r.equipment_pools,'EquipmentSlotFallbacks',to_jsonb(r)->'equipment_slot_fallbacks','GuardianArrivalMS',r.guardian_arrival_ms,'CompletionLevelExperience',r.completion_level_experience,'StatAnchorTier',r.stat_anchor_tier,'NormalBossEquipment',r.normal_boss_equipment,'FloorPopulationMultiplier',r.floor_population_multiplier,'Version',r.version,'LootVersion',r.loot_version,'MaxTier',r.max_tier,'ProgressMax',r.progress_max,'NormalProgress',r.normal_progress,'GlobeProgress',r.globe_progress,'BlueOrbs',r.blue_orbs,'GoldOrbs',r.gold_orbs,'TimedMS',r.timed_ms,'HardMS',r.hard_ms,'ReconnectMS',r.reconnect_ms,'DeathPenaltyMS',r.death_penalty_ms,'PresenceBP',r.presence_bp,'TierJumpSeconds',r.tier_jump_seconds,'HPGrowthBP',r.hp_growth_bp,'DamageGrowthBP',r.damage_growth_bp,'PartyHPBP',r.party_hpbp,'Equipment',r.equipment,'Runes',r.runes,'DustBase',r.dust_base,'DustTierDivisor',r.dust_tier_divisor,'UniqueBaseChance',r.unique_base_chance,'UniquePerTier',r.unique_per_tier,'UniqueCap',r.unique_cap,'GuaranteedUniqueTier',r.guaranteed_unique_tier,'TimelyEquipment',r.timely_equipment,'TimelyRunes',r.timely_runes,'TimelyDust',r.timely_dust,'ExperienceDivisor',r.experience_divisor,'DustItem',r.dust_item) || jsonb_build_object('Name',r.name,'BossName',r.boss_name,'ExitName',r.exit_name,'ExitGreeting',r.exit_greeting,'ExitSprite',r.exit_sprite,'ExitPortrait',r.exit_portrait,'NPCModel',r.npc_model,'AbilityMaxConcurrent',r.ability_max_concurrent,'AbilityRange',r.ability_range,'AbilityRecoveryMS',r.ability_recovery_ms,'AbilityStartMS',r.ability_start_ms,'AbilityJitterMS',r.ability_jitter_ms,'AbilityResetMS',r.ability_reset_ms,'AbilityUnavailableMS',r.ability_unavailable_ms,'AbilityCooldownMS',r.ability_cooldown_ms,'AbilityClassReductionMS',r.ability_class_reduction_ms,'AbilityBossDamagePct',r.ability_boss_damage_pct,'VisualSkill',r.visual_skill,'VisualSkillLevel',r.visual_skill_level) || jsonb_build_object('Classes',(SELECT jsonb_agg(jsonb_build_object('Class',class,'HPBP',hp_bp,'DamageBP',damage_bp) ORDER BY class) FROM game_rift_class_rules c WHERE c.version=r.version),'Abilities',(SELECT jsonb_agg(jsonb_build_object('ID',id,'Name',name,'Affix',affix,'Warning',warning,'Shape',shape,'Radius',radius,'InnerRadius',inner_radius,'Length',length,'DelayMS',delay_ms,'DamagePct',damage_pct) ORDER BY id) FROM game_rift_ability_rules a WHERE a.version=r.version)) || COALESCE(r.elite_rules,'{}'::jsonb))::text FROM game_rift_rule_versions r WHERE published ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []domain.RiftRules
	for rows.Next() {
		var text string
		var r domain.RiftRules
		if err = rows.Scan(&text); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(text), &r); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	entrances, err := loadRiftEntrances(ctx, q)
	if err != nil {
		return nil, err
	}
	return domain.NewRiftRuleTable(active, all, loot, items, entrances...)
}

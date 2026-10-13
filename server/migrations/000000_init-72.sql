-- User-authorized PROJECT DESIGN, 2026-10-13: old area Heaven's Punishment.
-- This is not a claim about Tencent/QQ Free Fantasy original random pools.
-- Native damage, resource/cast/cooldown curves, original complete prose and
-- restored radius 150 remain unchanged. Every landed, living target selects
-- ONE equally weighted state, then makes ONE common 40% base effect contest.
ALTER TABLE game_skill_statuses ADD COLUMN IF NOT EXISTS random_group INTEGER NOT NULL DEFAULT 0
 CHECK(random_group>=0);
ALTER TABLE game_skill_statuses ADD COLUMN IF NOT EXISTS random_weight INTEGER NOT NULL DEFAULT 0
 CHECK(random_weight>=0);
DO $random_status_constraint$
BEGIN
 IF NOT EXISTS(SELECT FROM pg_constraint WHERE conrelid='game_skill_statuses'::regclass AND conname='game_skill_statuses_random_pair') THEN
  ALTER TABLE game_skill_statuses ADD CONSTRAINT game_skill_statuses_random_pair
   CHECK((random_group=0 AND random_weight=0) OR (random_group>0 AND random_weight>0));
 END IF;
END
$random_status_constraint$;

DO $random_punishment$
DECLARE fresh BOOLEAN;
BEGIN
 INSERT INTO game_skill_mechanism_sources(skill_id,mechanic,source_url,source_revision,imported)
 VALUES(10313,'random_status_pool','project-design://heavens-punishment-2026-10-13',
  '用户授权本项目设计；非原版认证；每命中存活目标等权4选1、40%基础判定',false)
 ON CONFLICT DO NOTHING;
 fresh := FOUND;
 IF NOT fresh THEN RETURN; END IF;
 -- Never override any administrator-defined application or restore a pool
 -- row deliberately removed after import.
 IF EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=10313) THEN RETURN; END IF;
 IF (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id=10313 AND skill_level BETWEEN 1 AND 5)<>5 THEN
  RAISE EXCEPTION 'Heavens Punishment authored ranks missing';
 END IF;
 -- These pure roots are the already reviewed, icon-backed shared definitions
 -- from migration66. Missing/deleted or modified definitions require review;
 -- never silently recreate an administrator's removed status.
 IF (SELECT count(*) FROM game_custom_statuses WHERE status_id=1034 AND status_level BETWEEN 31 AND 35
      AND block_move AND immobile=1 AND interval_sec=0 AND status_type=1 AND icon_known AND icon>0
      AND hp_change_value=0 AND physical_damage_reduction_pct=0 AND magic_damage_reduction_pct=0
      AND physical_damage_taken_pct=0 AND magic_damage_taken_pct=0 AND damage_shield=0
      AND attack_cap IS NULL AND NOT prevent_invisible AND NOT attack_speed_is_frequency
      AND NOT hidden AND NOT replace_native)<>5 OR
    EXISTS(SELECT FROM game_custom_status_affixes WHERE status_id=1034 AND status_level BETWEEN 31 AND 35) THEN
  RAISE EXCEPTION 'reviewed pure-root status definitions missing or modified';
 END IF;
 INSERT INTO game_skill_statuses(skill_id,skill_level,status_order,status_id,status_level,
   chance_bp,duration_sec,configuration_version,target_type,random_group,random_weight)
 SELECT 10313,lv,p.ord,p.id,lv+p.rank_offset,4000,lv+p.duration_base,72,'any',1,1
 FROM generate_series(1,5) lv
 CROSS JOIN (VALUES (1,1006,0,5),(2,1007,0,5),(3,1008,1,5),(4,1034,30,2)) p(ord,id,rank_offset,duration_base);
 UPDATE game_skill_mechanism_sources SET imported=true WHERE skill_id=10313 AND mechanic='random_status_pool';
END
$random_punishment$;

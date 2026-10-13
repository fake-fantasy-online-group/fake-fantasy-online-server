-- User-approved project rule, 2026-10-13: Advanced Backstab (10201) adds
-- a 100% BASE stun attempt lasting 3 seconds after Backstab (10209) lands.
-- The existing 20% stealth-preservation roll is independent and unchanged.
-- Damage hit, living target, effect resistance, immunity and overlay checks
-- remain in the generic passive-trigger/status path; no ID-specific combat.
-- All five original ranks are 100%/3s. Equal first/fifth control duration uses
-- the existing rank6+ effect-hit continuation, never invented duration growth.
DO $advanced_backstab_stun$
BEGIN
 INSERT INTO game_skill_mechanism_sources(skill_id,mechanic,source_url,source_revision,imported)
 VALUES(10201,'backstab_stun','project-design://advanced-backstab-2026-10-13',
  'User approved base probability100% and duration3s; independent stealth/stun is project default, not historical authentication',false)
 ON CONFLICT DO NOTHING;
 IF NOT FOUND THEN RETURN; END IF;
 -- An administrator's existing trigger configuration owns the whole skill.
 IF EXISTS(SELECT FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201)
 THEN RETURN; END IF;
 IF (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id=10201 AND skill_level BETWEEN 1 AND 5 AND prof=3 AND skill_type=18)<>5 OR
    (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id=10209 AND skill_level BETWEEN 1 AND 5 AND prof=3 AND skill_type=1)<>5
 THEN RAISE EXCEPTION 'missing native Advanced Backstab anchors'; END IF;
 -- Rewrite only the approved numbers in the exact original sentence. Changed
 -- administrator prose is not treated as an untouched automatic-import target.
 IF (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id=10201 AND skill_level BETWEEN 1 AND 5
      AND desc_=format('背刺时20%%的几率不会中止隐身状态，%s%%几率击晕对手。',skill_level*5))<>5
 THEN RETURN; END IF;
 -- Original 1002/27 is a pure three-second stun with the original icon7055.
 -- Its reuse is a project choice after user approval, not a recovered old
 -- Backstab-to-status mapping. Never borrow another skill's stat penalties.
 IF (SELECT count(*) FROM gamedata.ov_exceptdetail d
     JOIN gamedata.ov_exceptdetail_entry e USING(row_no)
     JOIN gamedata.ov_exceptdesc n ON n.type=d.type
     WHERE d.type=1002 AND e.except_level=27 AND e.idx=26 AND e.last_time=3
       AND e.interval=0 AND e.except_type=1 AND e.no_inform_client=0
       AND n.level_small_maps[27]=7055)<>1 OR
    (SELECT count(*) FROM gamedata.ov_exceptdetail d
     JOIN gamedata.ov_exceptdetail_effect_entry e USING(row_no)
     WHERE d.type=1002 AND e.idx=26 AND e.op_type=1 AND e.prob=100
       AND e.attr_id=83 AND e.mode=0 AND e.value=0)<>1 OR
    EXISTS(SELECT FROM gamedata.ov_exceptdetail d
     JOIN gamedata.ov_exceptdetail_effect_entry e USING(row_no)
     WHERE d.type=1002 AND e.idx=26 AND e.op_type=1 AND e.prob=100
       AND NOT(e.attr_id=83 AND e.mode=0 AND e.value=0)) OR
    EXISTS(SELECT FROM game_custom_statuses WHERE status_id=1002 AND status_level=27)
 THEN RAISE EXCEPTION 'native three-second pure stun missing or modified'; END IF;

 INSERT INTO game_skill_passive_trigger_statuses(passive_skill_id,passive_skill_level,trigger_order,
   target_skill_id,status_id,status_level,chance_bp,duration_sec)
 SELECT 10201,lv,1,10209,1002,27,10000,3 FROM generate_series(1,5) lv;
 UPDATE gamedata.ov_skilldesc
 SET desc_='背刺时20%的几率不会中止隐身状态，100%几率击晕对手3秒。'
 WHERE skill_id=10201 AND skill_level BETWEEN 1 AND 5;
 UPDATE game_skill_mechanism_sources SET imported=true
 WHERE skill_id=10201 AND mechanic='backstab_stun';
END
$advanced_backstab_stun$;

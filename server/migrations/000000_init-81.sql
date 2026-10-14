-- Paired client/server release gate: activate the staged, immutable rift v6.
-- Run only after the v6-capable client and server are ready for the same release.
-- One atomic DO statement; no rule, monster, skill, character, or run is rewritten.
DO $activate_elite_skills$
DECLARE
 active_version INTEGER;
 original JSONB;
 staged JSONB;
 expected_config JSONB := $config${"Elite":{"HazardMaxConcurrent":6,"Guardians":[{"MonsterID":1011,"Abilities":[0,3,6]},{"MonsterID":1092,"Abilities":[0,3,7]},{"MonsterID":1060,"Abilities":[5,3,7]},{"MonsterID":1073,"Abilities":[1,6,7]},{"MonsterID":1085,"Abilities":[1,3,7]},{"MonsterID":1136,"Abilities":[5,4,6]},{"MonsterID":3067,"Abilities":[0,1,6]},{"MonsterID":3044,"Abilities":[0,3,4]}]},"Abilities":[{"ID":0,"Mechanism":"golden_body","Trigger":"active","Name":"金身","Affix":"金身","Warning":"金身期间免疫负面状态并减伤95%","Shape":"","Radius":0,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":0,"CooldownMS":30000,"DurationMS":5000,"DamageReductionPct":95},{"ID":1,"Mechanism":"clones","Trigger":"active","Name":"分身","Affix":"分身","Warning":"召唤三个保留原普通技能的分身","Shape":"","Radius":0,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":0,"CooldownMS":30000,"DurationMS":20000,"Count":3,"HPPct":10},{"ID":2,"Mechanism":"death_lava","Trigger":"death","Name":"熔岩球","Affix":"熔岩球","Warning":"死亡熔岩球将在三秒后爆炸","Shape":"circle","Radius":140,"InnerRadius":0,"Length":0,"DelayMS":3000,"DamagePct":160,"DurationMS":3000},{"ID":3,"Mechanism":"lightning_orb","Trigger":"active","Name":"闪电球","Affix":"闪电球","Warning":"闪电球产生向外扩散的伤害环","Shape":"ring","Radius":220,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":30,"CooldownMS":20000,"DurationMS":10000,"PulseMS":2000,"FirstPulseMS":1000,"PropagationMS":1000,"Width":40},{"ID":4,"Mechanism":"ice_orb","Trigger":"active","Name":"冰球","Affix":"冰球","Warning":"冰球将在三秒后爆炸并尝试冻结","Shape":"circle","Radius":120,"InnerRadius":0,"Length":0,"DelayMS":3000,"DamagePct":140,"CooldownMS":20000,"DurationMS":3000,"FreezeMS":2000},{"ID":5,"Mechanism":"agility","Trigger":"passive","Name":"敏捷","Affix":"敏捷","Warning":"移动速度提高100%","Shape":"","Radius":0,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":0,"MoveSpeedPct":100},{"ID":6,"Mechanism":"poison_pool","Trigger":"active","Name":"毒池","Affix":"毒池","Warning":"连续停留使每秒毒伤递增，离开后重置","Shape":"circle","Radius":120,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":5,"CooldownMS":30000,"DurationMS":20000,"PulseMS":1000,"StepDamagePct":1,"MaxDamagePct":15},{"ID":7,"Mechanism":"walls","Trigger":"active","Name":"筑墙","Affix":"筑墙","Warning":"三面短墙阻挡移动，十五秒后消失","Shape":"line","Radius":0,"InnerRadius":0,"Length":80,"DelayMS":0,"DamagePct":0,"CooldownMS":30000,"DurationMS":15000,"Count":3,"Width":20,"Gap":80}]}$config$::jsonb;
BEGIN
 -- Serialize against concurrent selector changes, including a later release.
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL OR active_version < 5 THEN
  RAISE EXCEPTION 'rift6 activation requires active rift5 (or an already activated later version)';
 END IF;
 IF active_version > 6 THEN
  RETURN; -- A replay must never downgrade, or validate obsolete dependencies of, a later release.
 END IF;

 -- Published data is immutable; these locks also keep reference data stable
 -- between the dependency checks and the compare-and-set below.
 LOCK TABLE game_rift_rule_versions, game_rift_class_rules, game_rift_ability_rules,
  game_loot_rule_versions, game_loot_rune_pool, game_loot_socket_levels, game_loot_grades,
  game_items, game_endgame_rune_rules, gamedata.ov_item,
  game_monsters, game_monster_ai_templates, game_monster_skills,
  game_monster_skill_effects, gamedata.ov_skilldesc IN SHARE MODE;

 IF (SELECT count(*) FROM pg_trigger
     WHERE tgname='immutable_published_rules' AND NOT tgisinternal AND tgenabled IN ('O','A')
       AND tgfoid='guard_published_endgame_rules()'::regprocedure
       AND tgrelid IN ('game_rift_rule_versions'::regclass,'game_rift_class_rules'::regclass,
        'game_rift_ability_rules'::regclass,'game_loot_rule_versions'::regclass,
        'game_loot_rune_pool'::regclass,'game_loot_socket_levels'::regclass,'game_loot_grades'::regclass)) <> 7 THEN
  RAISE EXCEPTION 'rift6 activation requires enabled immutable published-rule guards';
 END IF;
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r
  WHERE version=5 AND published AND loot_version=2 AND guardian_arrival_ms=5000 AND elite_rules IS NULL;
 SELECT to_jsonb(r) INTO staged FROM game_rift_rule_versions r WHERE version=6 AND published;
 IF original IS NULL OR staged IS DISTINCT FROM
    (original || jsonb_build_object('version',6,'elite_rules',expected_config)) THEN
  RAISE EXCEPTION 'rift6 activation requires the exact published migration80 configuration';
 END IF;
 IF (SELECT count(*) FROM game_rift_rule_versions WHERE version BETWEEN 1 AND 6 AND published) <> 6 THEN
  RAISE EXCEPTION 'rift6 activation requires all published historical rift versions';
 END IF;
 IF (SELECT count(*) FROM game_rift_class_rules WHERE version=6) <> 4 OR
    (SELECT count(*) FROM game_rift_ability_rules WHERE version=6) <> 3 OR
 EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=5
          EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=6)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=6
          EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=5)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=5
          EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=6)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=6
          EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=5)) THEN
  RAISE EXCEPTION 'rift6 activation requires complete unchanged class and legacy ability dependencies';
 END IF;

 -- Rewards still depend on immutable weighted loot2. Do not change its selector.
 IF NOT EXISTS(SELECT FROM game_loot_rule_versions WHERE version=1 AND published) OR
    NOT EXISTS(SELECT FROM game_loot_rule_versions WHERE version=2 AND published) OR
    (SELECT to_jsonb(r) FROM game_loot_rule_versions r WHERE version=2) IS DISTINCT FROM
    ((SELECT to_jsonb(r) FROM game_loot_rule_versions r WHERE version=1) || '{"version":2}'::jsonb) OR
    NOT EXISTS(SELECT FROM game_items WHERE id=(original->>'dust_item')::integer) OR
    (SELECT count(*) FROM game_loot_rune_pool WHERE version=2) <> 28 OR
    (SELECT count(*) FROM game_loot_grades WHERE version=2) <> 4 OR
    (SELECT count(*) FROM game_loot_socket_levels WHERE version=2) <> 4 OR
 EXISTS(SELECT FROM game_loot_rune_pool p
  LEFT JOIN game_items i ON i.id=p.item_id
  LEFT JOIN game_endgame_rune_rules rr ON rr.item_id=p.item_id
  LEFT JOIN gamedata.ov_item card ON card.index=p.item_id
  WHERE p.version=2 AND (i.id IS NULL OR rr.item_id IS NULL OR card.index IS NULL)) OR
 EXISTS(
  WITH expected AS (
   SELECT row_number() OVER(ORDER BY i.level_need,p.item_id)::integer AS seq,p.item_id,
          133-4*row_number() OVER(ORDER BY i.level_need,p.item_id) AS weight
   FROM game_loot_rune_pool p JOIN gamedata.ov_item i ON i.index=p.item_id WHERE p.version=1
  )
  (SELECT * FROM expected EXCEPT ALL SELECT seq,item_id,weight FROM game_loot_rune_pool WHERE version=2)
  UNION ALL (SELECT seq,item_id,weight FROM game_loot_rune_pool WHERE version=2 EXCEPT ALL SELECT * FROM expected)
 ) OR
 EXISTS((SELECT min_level,cap FROM game_loot_socket_levels WHERE version=1 EXCEPT ALL SELECT min_level,cap FROM game_loot_socket_levels WHERE version=2)
  UNION ALL (SELECT min_level,cap FROM game_loot_socket_levels WHERE version=2 EXCEPT ALL SELECT min_level,cap FROM game_loot_socket_levels WHERE version=1)) OR
 EXISTS((SELECT grade,rune_rate_bp,socket_weights FROM game_loot_grades WHERE version=1 EXCEPT ALL SELECT grade,rune_rate_bp,socket_weights FROM game_loot_grades WHERE version=2)
  UNION ALL (SELECT grade,rune_rate_bp,socket_weights FROM game_loot_grades WHERE version=2 EXCEPT ALL SELECT grade,rune_rate_bp,socket_weights FROM game_loot_grades WHERE version=1)) THEN
  RAISE EXCEPTION 'rift6 activation requires complete published weighted loot2 dependencies';
 END IF;

 -- These eight originals are the only guardian candidates. Missing native
 -- content must block activation, never silently select an ordinary monster.
 IF EXISTS(SELECT FROM (VALUES
   (1011,211,'boss_caster'),(1092,292,'boss_melee'),(1060,260,'boss_melee'),
   (1073,273,'boss_caster'),(1085,285,'boss_caster'),(1136,336,'boss_caster'),
   (3067,210,'boss_melee'),(3044,341,'boss_caster')
  ) expected(id,sprite,ai_template)
  LEFT JOIN game_monsters m ON m.id=expected.id
  LEFT JOIN game_monster_ai_templates ai ON ai.id=m.ai_template
  WHERE m.id IS NULL OR m.sprite IS DISTINCT FROM expected.sprite OR m.kind IS DISTINCT FROM 'BOSS'
     OR m.ai_template IS DISTINCT FROM expected.ai_template OR ai.id IS NULL OR m.name='') THEN
  RAISE EXCEPTION 'rift6 activation requires all eight original guardian identities and AI templates';
 END IF;
 IF EXISTS(SELECT FROM (VALUES
   (1011,12013,1),(1011,12003,4),(1011,12009,4),
   (1092,12013,1),(1092,12001,5),(1092,12002,5),
   (1060,12011,3),(1060,12010,1),(1073,12011,3),(1073,10010,3),
   (1085,12013,1),(1085,12023,1),(1085,12014,4),
   (1136,12022,1),(3067,12013,1),(3067,12034,1),(3044,12021,1)
  ) expected(monster_id,skill_id,skill_lv)
  WHERE NOT EXISTS(SELECT FROM game_monster_skills ms
   WHERE ms.monster_id=expected.monster_id AND ms.skill_id=expected.skill_id
     AND ms.skill_lv=expected.skill_lv AND ms.kind IN ('enemy','self') AND ms.enabled
     AND ms.chance_bp>0
     AND EXISTS(SELECT FROM gamedata.ov_skilldesc sd WHERE sd.skill_id=ms.skill_id)
     AND (ms.fallback_damage_pct>0 OR EXISTS(SELECT FROM game_monster_skill_effects fx
          WHERE fx.skill_id=ms.skill_id AND fx.skill_lv IN (0,ms.skill_lv))))) THEN
  RAISE EXCEPTION 'rift6 activation requires enabled original guardian skills and resolved skill content';
 END IF;

 -- Idempotent on 6, monotonic on later versions, and the only persistent write.
 IF active_version=5 THEN
  UPDATE game_rift_rule_active SET version=6 WHERE id=1 AND version=5;
  IF NOT FOUND THEN RAISE EXCEPTION 'rift active selector changed during v6 activation'; END IF;
 END IF;
END $activate_elite_skills$;

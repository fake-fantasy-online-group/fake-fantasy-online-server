-- Stage the approved eight-mechanism runtime as immutable rules v6.
-- Deliberately do not activate: client hazard presentation/collision is pending.
-- v1-v5 remain byte-compatible; existing runs continue their frozen version.
ALTER TABLE game_rift_rule_versions ADD COLUMN IF NOT EXISTS elite_rules JSONB;
DO $elite_skills$
DECLARE original JSONB; revised JSONB;
BEGIN
 SELECT to_jsonb(r) INTO original FROM game_rift_rule_versions r WHERE version=5 AND published AND guardian_arrival_ms=5000 AND elite_rules IS NULL;
 IF original IS NULL THEN RAISE EXCEPTION 'elite skills require published rift5'; END IF;
 revised := original || jsonb_build_object('version',6,'published',false,'elite_rules',$config${"Elite":{"HazardMaxConcurrent":6,"Guardians":[{"MonsterID":1011,"Abilities":[0,3,6]},{"MonsterID":1092,"Abilities":[0,3,7]},{"MonsterID":1060,"Abilities":[5,3,7]},{"MonsterID":1073,"Abilities":[1,6,7]},{"MonsterID":1085,"Abilities":[1,3,7]},{"MonsterID":1136,"Abilities":[5,4,6]},{"MonsterID":3067,"Abilities":[0,1,6]},{"MonsterID":3044,"Abilities":[0,3,4]}]},"Abilities":[{"ID":0,"Mechanism":"golden_body","Trigger":"active","Name":"金身","Affix":"金身","Warning":"金身期间免疫负面状态并减伤95%","Shape":"","Radius":0,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":0,"CooldownMS":30000,"DurationMS":5000,"DamageReductionPct":95},{"ID":1,"Mechanism":"clones","Trigger":"active","Name":"分身","Affix":"分身","Warning":"召唤三个保留原普通技能的分身","Shape":"","Radius":0,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":0,"CooldownMS":30000,"DurationMS":20000,"Count":3,"HPPct":10},{"ID":2,"Mechanism":"death_lava","Trigger":"death","Name":"熔岩球","Affix":"熔岩球","Warning":"死亡熔岩球将在三秒后爆炸","Shape":"circle","Radius":140,"InnerRadius":0,"Length":0,"DelayMS":3000,"DamagePct":160,"DurationMS":3000},{"ID":3,"Mechanism":"lightning_orb","Trigger":"active","Name":"闪电球","Affix":"闪电球","Warning":"闪电球产生向外扩散的伤害环","Shape":"ring","Radius":220,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":30,"CooldownMS":20000,"DurationMS":10000,"PulseMS":2000,"FirstPulseMS":1000,"PropagationMS":1000,"Width":40},{"ID":4,"Mechanism":"ice_orb","Trigger":"active","Name":"冰球","Affix":"冰球","Warning":"冰球将在三秒后爆炸并尝试冻结","Shape":"circle","Radius":120,"InnerRadius":0,"Length":0,"DelayMS":3000,"DamagePct":140,"CooldownMS":20000,"DurationMS":3000,"FreezeMS":2000},{"ID":5,"Mechanism":"agility","Trigger":"passive","Name":"敏捷","Affix":"敏捷","Warning":"移动速度提高100%","Shape":"","Radius":0,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":0,"MoveSpeedPct":100},{"ID":6,"Mechanism":"poison_pool","Trigger":"active","Name":"毒池","Affix":"毒池","Warning":"连续停留使每秒毒伤递增，离开后重置","Shape":"circle","Radius":120,"InnerRadius":0,"Length":0,"DelayMS":0,"DamagePct":5,"CooldownMS":30000,"DurationMS":20000,"PulseMS":1000,"StepDamagePct":1,"MaxDamagePct":15},{"ID":7,"Mechanism":"walls","Trigger":"active","Name":"筑墙","Affix":"筑墙","Warning":"三面短墙阻挡移动，十五秒后消失","Shape":"line","Radius":0,"InnerRadius":0,"Length":80,"DelayMS":0,"DamagePct":0,"CooldownMS":30000,"DurationMS":15000,"Count":3,"Width":20,"Gap":80}]}$config$::jsonb);
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=6) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,revised)).*;
  INSERT INTO game_rift_class_rules SELECT 6,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=5;
  -- Legacy rows remain for schema compatibility; elite_rules replaces the complete catalog on read.
  INSERT INTO game_rift_ability_rules SELECT 6,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=5;
  UPDATE game_rift_rule_versions SET published=true WHERE version=6;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=6) IS DISTINCT FROM (revised || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift rules version 6 already has different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=5 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=6)
 UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=6 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=5)) THEN
  RAISE EXCEPTION 'rift6 must preserve class scaling';
 END IF;
 -- No UPDATE game_rift_rule_active here. Activation is a separate approved rollout.
END $elite_skills$;

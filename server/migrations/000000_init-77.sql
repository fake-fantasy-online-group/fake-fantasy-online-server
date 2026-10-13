-- Only the choice of rune changes. Monster rune occurrence rates and all other
-- loot/reward parameters remain unchanged. The 28 runes are ranked by their
-- authoritative physical-card use level, then weighted 129,125,...,21 (sum 2100).
-- Thus the highest-level rune has exactly 1% of rune selections.
-- Historical loot1 and rift1/2 remain immutable and retain equal rune choices.
ALTER TABLE game_loot_rune_pool ADD COLUMN IF NOT EXISTS weight BIGINT NOT NULL DEFAULT 1 CHECK (weight BETWEEN 1 AND 1000000);

DO $rune_weights$
DECLARE original_loot JSONB; weighted_loot JSONB; original_rift JSONB; weighted_rift JSONB;
BEGIN
 SELECT to_jsonb(r) INTO original_loot FROM game_loot_rule_versions r WHERE version=1 AND published;
 SELECT to_jsonb(r) INTO original_rift FROM game_rift_rule_versions r WHERE version=2 AND published AND loot_version=1 AND floor_population_multiplier=2;
 IF original_loot IS NULL OR original_rift IS NULL THEN
  RAISE EXCEPTION 'rune weight upgrade requires published loot1 and doubled-density rift2';
 END IF;
 IF (SELECT count(*) FROM game_loot_rune_pool WHERE version=1)<>28 OR
    EXISTS(SELECT FROM game_loot_rune_pool WHERE version=1 AND weight<>1) OR
    NOT (SELECT count(*)=28 AND count(DISTINCT i.level_need)=28 AND min(i.level_need)=12 AND max(i.level_need)=60
         FROM game_loot_rune_pool p
         JOIN game_endgame_rune_rules r ON r.item_id=p.item_id
         JOIN gamedata.ov_item i ON i.index=p.item_id
         WHERE p.version=1) THEN
  RAISE EXCEPTION 'rune weight upgrade requires 28 equal-weight runes with distinct physical-card levels 12 through 60';
 END IF;

 weighted_loot := original_loot || '{"version":2,"published":false}'::jsonb;
 IF NOT EXISTS(SELECT FROM game_loot_rule_versions WHERE version=2) THEN
  INSERT INTO game_loot_rule_versions SELECT (jsonb_populate_record(NULL::game_loot_rule_versions,weighted_loot)).*;
  INSERT INTO game_loot_rune_pool(version,seq,item_id,weight)
   SELECT 2,rank::integer,item_id,133-4*rank FROM (
    SELECT p.item_id,row_number() OVER(ORDER BY i.level_need,p.item_id) AS rank
    FROM game_loot_rune_pool p JOIN gamedata.ov_item i ON i.index=p.item_id WHERE p.version=1
   ) ranked;
  INSERT INTO game_loot_socket_levels SELECT 2,min_level,cap FROM game_loot_socket_levels WHERE version=1;
  INSERT INTO game_loot_grades SELECT 2,grade,rune_rate_bp,socket_weights FROM game_loot_grades WHERE version=1;
  UPDATE game_loot_rule_versions SET published=true WHERE version=2;
 ELSIF (SELECT to_jsonb(r) FROM game_loot_rule_versions r WHERE version=2) IS DISTINCT FROM (weighted_loot || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'loot rules version 2 is already occupied by different configuration';
 END IF;
 IF EXISTS(
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
  RAISE EXCEPTION 'loot rules version 2 must contain exactly the weighted rune pool and preserve other loot rules';
 END IF;

 weighted_rift := original_rift || '{"version":3,"published":false,"loot_version":2}'::jsonb;
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=3) THEN
  INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,weighted_rift)).*;
  INSERT INTO game_rift_class_rules SELECT 3,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=2;
  INSERT INTO game_rift_ability_rules SELECT 3,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=2;
  UPDATE game_rift_rule_versions SET published=true WHERE version=3;
 ELSIF (SELECT to_jsonb(r) FROM game_rift_rule_versions r WHERE version=3) IS DISTINCT FROM (weighted_rift || '{"published":true}'::jsonb) THEN
  RAISE EXCEPTION 'rift rules version 3 is already occupied by different configuration';
 END IF;
 IF EXISTS((SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=2 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=3)
  UNION ALL (SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=3 EXCEPT ALL SELECT class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=2)) OR
 EXISTS((SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=2 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=3)
  UNION ALL (SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=3 EXCEPT ALL SELECT id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=2)) THEN
  RAISE EXCEPTION 'rift rules version 3 must preserve class and ability rules';
 END IF;
 -- Replays never replace an operator's newer active version.
 UPDATE game_loot_rule_active SET version=2 WHERE id=1 AND version=1;
 UPDATE game_rift_rule_active SET version=3 WHERE id=1 AND version=2;
END $rune_weights$;

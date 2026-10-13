-- Equipment-inclusive authored synergies. Original skill/status payloads,
-- native passive relationships, player allocations and equipment are unchanged.
-- Apply after73, in the migration runner's transaction.
ALTER TABLE game_skill_synergies DROP CONSTRAINT IF EXISTS game_skill_synergies_rank_mode_check;
ALTER TABLE game_skill_synergies ADD CONSTRAINT game_skill_synergies_rank_mode_check
 CHECK(rank_mode IN ('hard','effective_raw','effective_training'));
CREATE TABLE IF NOT EXISTS game_skill_synergy_update_history (
 configuration_version integer PRIMARY KEY CHECK(configuration_version>0),
 previous_rows jsonb NOT NULL,
 updated_edges integer NOT NULL CHECK(updated_edges>=0)
);
DO $$
DECLARE changed integer;
BEGIN
 IF EXISTS(SELECT FROM game_skill_synergy_update_history WHERE configuration_version=74) THEN
  RETURN;
 END IF;
 IF (SELECT count(*) FROM game_skill_synergies)<>128 OR
    (SELECT expected_synergies FROM game_skill_configuration_manifest WHERE id=1) IS DISTINCT FROM 128 OR
    (SELECT count(*) FROM game_skill_progression)<>149 THEN
  RAISE EXCEPTION 'migration74 requires complete skill configuration';
 END IF;
 IF EXISTS(SELECT FROM game_skill_synergies e
 LEFT JOIN gamedata.ov_skilldesc d ON d.skill_id=e.target_skill_id AND d.skill_level=1
 WHERE d.skill_id IS NULL OR d.level_need IS NULL OR d.level_need<1 OR d.level_need>100) THEN
  RAISE EXCEPTION 'migration74 requires original beneficiary learning levels';
 END IF;
 INSERT INTO game_skill_synergy_update_history(configuration_version,previous_rows,updated_edges)
 SELECT 74, jsonb_agg(to_jsonb(e) ORDER BY edge_order),0 FROM game_skill_synergies e;
 -- Only migrate the exact reviewed old edge. Existing administrator coefficient,
 -- mode or effect changes remain intact; the history marker protects later edits.
 WITH original(target,source,effect,bp) AS (VALUES
  (10002,10009,'damage',100),
  (10002,10013,'mana_economy',50),
  (10021,10006,'damage',100),
  (10021,10005,'width',50),
  (10022,10021,'damage',100),
  (10022,10028,'radius',50),
  (10102,10111,'damage',100),
  (10104,10113,'damage',100),
  (10121,10119,'damage',100),
  (10121,10117,'radius',50),
  (10209,10208,'damage',100),
  (10209,10219,'elite_damage',50),
  (10213,10214,'damage',100),
  (10213,10215,'radius',50),
  (10216,10215,'damage',100),
  (10216,10217,'freeze_duration',50),
  (10216,10228,'radius',50),
  (10301,10302,'heal_flat',200),
  (10301,10304,'heal_power',100),
  (10301,10306,'mana_economy',50),
  (10304,10301,'heal_power',100),
  (10304,10303,'heal_flat',200),
  (10304,10307,'radius',50),
  (10319,10317,'damage',100),
  (10319,10326,'mana_economy',50),
  (10401,10402,'damage',100),
  (10402,10401,'damage',100),
  (10409,10410,'damage',100),
  (10003,10013,'damage',100),
  (10004,10003,'damage',100),
  (10005,10017,'damage',100),
  (10006,10005,'damage',100),
  (10007,10023,'damage',100),
  (10009,10002,'damage',100),
  (10013,10003,'damage',100),
  (10015,10003,'damage',100),
  (10017,10005,'damage',100),
  (10023,10022,'damage',100),
  (10027,10002,'damage',100),
  (10103,10117,'damage',100),
  (10105,10116,'damage',100),
  (10111,10102,'damage',100),
  (10113,10104,'damage',100),
  (10116,10104,'damage',100),
  (10117,10103,'damage',100),
  (10119,10121,'damage',100),
  (10122,10121,'damage',100),
  (10123,10122,'damage',100),
  (10125,10123,'damage',100),
  (10126,10102,'damage',100),
  (10205,10221,'damage',100),
  (10206,10205,'damage',100),
  (10210,10209,'damage',100),
  (10212,10209,'damage',100),
  (10214,10213,'damage',100),
  (10215,10214,'damage',100),
  (10217,10216,'damage',100),
  (10219,10209,'damage',100),
  (10221,10205,'damage',100),
  (10223,10206,'damage',100),
  (10225,10223,'damage',100),
  (10317,10319,'damage',100),
  (10312,10317,'damage',100),
  (10326,10312,'damage',100),
  (10313,10326,'damage',100),
  (10319,10313,'damage',100),
  (10405,10402,'damage',100),
  (10408,10405,'damage',100),
  (10410,10409,'damage',100),
  (10411,10410,'damage',100)
 )
 UPDATE game_skill_synergies e SET
 rank_mode='effective_training',
 effect=CASE WHEN e.effect='heal_flat' THEN 'heal_power' ELSE e.effect END,
 per_point_bp=CASE WHEN d.level_need<30 THEN 300 WHEN d.level_need<50 THEN 400
                   WHEN d.level_need<70 THEN 500 ELSE 600 END
 FROM original o,gamedata.ov_skilldesc d
 WHERE e.target_skill_id=o.target AND e.source_skill_id=o.source
 AND e.effect=o.effect AND e.per_point_bp=o.bp AND e.rank_mode='hard'
 AND d.skill_id=e.target_skill_id AND d.skill_level=1;
 GET DIAGNOSTICS changed=ROW_COUNT;
 UPDATE game_skill_synergy_update_history SET updated_edges=changed WHERE configuration_version=74;
END $$;

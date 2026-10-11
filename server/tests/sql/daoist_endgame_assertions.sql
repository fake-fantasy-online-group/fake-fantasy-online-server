-- Run only against a disposable test database initialized from all init fragments.
-- No credentials, production data, or network server are needed.
BEGIN;
CREATE FUNCTION pg_temp.assert_true(ok boolean, label text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF ok IS DISTINCT FROM true THEN RAISE EXCEPTION 'assertion failed: %', label; END IF;
END $$;
CREATE FUNCTION pg_temp.expect_error(statement text, expected text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE actual text;
BEGIN
  BEGIN
    EXECUTE statement;
  EXCEPTION WHEN OTHERS THEN
    GET STACKED DIAGNOSTICS actual = RETURNED_SQLSTATE;
  END;
  IF actual IS DISTINCT FROM expected THEN
    RAISE EXCEPTION 'expected SQLSTATE %, got % for %', expected, actual, statement;
  END IF;
END $$;
SELECT pg_temp.assert_true((SELECT count(*)=4 FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('equipment_endgame_state','character_endgame_state','endgame_receipts','endgame_runs')), 'four additive tables');
INSERT INTO accounts(id, username) VALUES (920000001, 'endgame_sql_validation');
INSERT INTO characters(id,account_id,slot,name,race,map_id,pos_x,pos_y) VALUES(920000001,920000001,0,'endgame_sql_validation',1,7,10,20);
SELECT pg_temp.assert_true((SELECT trial_key_reward=0 AND trial_completion_id='' FROM characters WHERE id=920000001), 'old/new characters receive frozen-trial defaults');
UPDATE characters SET trial_key_reward=14,trial_completion_id=repeat('x',64) WHERE id=920000001;
SELECT pg_temp.expect_error('UPDATE characters SET trial_key_reward=15 WHERE id=920000001', '23514');
SELECT pg_temp.expect_error('UPDATE characters SET trial_key_reward=-1 WHERE id=920000001', '23514');
SELECT pg_temp.expect_error('UPDATE characters SET trial_completion_id=repeat(''x'',65) WHERE id=920000001', '23514');
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(920000001,32001,1);
INSERT INTO equipment_instances(uid,refine_level,socket_count,socket_item_ids) VALUES(920000001,3,2,ARRAY[101,102,0,0,0]);
INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES(920000001,0,32001,8000);
UPDATE equipment_endgame_state SET revision=1,unique_roll_bp=12000 WHERE uid=920000001;
SELECT pg_temp.assert_true((SELECT i.item_id=32001 AND e.refine_level=3 AND e.socket_item_ids=ARRAY[101,102,0,0,0] AND x.unique_roll_bp=12000 FROM item_instances i JOIN equipment_instances e USING(uid) JOIN equipment_endgame_state x USING(uid) WHERE uid=920000001), 'overlay shares UID without replacing native equipment');
SELECT pg_temp.expect_error('INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES(920000002,0,32001,10000)', '23503');
SELECT pg_temp.expect_error('INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES(920000001,0,32001,10000)', '23505');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET revision=-1 WHERE uid=920000001', '23514');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET unique_id=0 WHERE uid=920000001', '23514');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET unique_roll_bp=7999 WHERE uid=920000001', '23514');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET unique_roll_bp=12001 WHERE uid=920000001', '23514');
SELECT pg_temp.assert_true((SELECT formation_roll_bp=0 FROM equipment_endgame_state WHERE uid=920000001), 'legacy formation roll remains zero');
UPDATE equipment_endgame_state SET formation_roll_bp=10417 WHERE uid=920000001;
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET formation_roll_bp=7999 WHERE uid=920000001', '23514');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET formation_roll_bp=12001 WHERE uid=920000001', '23514');
SELECT pg_temp.assert_true((SELECT formation_roll_bp=10417 FROM equipment_endgame_state WHERE uid=920000001), 'formation roll persisted exactly');

-- Ordinary equipment carries the same durable mutation revision, with no unique roll.
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(920000003,1001,1);
INSERT INTO equipment_instances(uid,socket_count) VALUES(920000003,1);
INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES(920000003,1,0,0);
UPDATE equipment_endgame_state SET revision=2 WHERE uid=920000003;
SELECT pg_temp.assert_true((SELECT revision=2 AND unique_id=0 AND unique_roll_bp=0 FROM equipment_endgame_state WHERE uid=920000003), 'ordinary revision retained');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET revision=-1 WHERE uid=920000003', '23514');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET unique_roll_bp=8000 WHERE uid=920000003', '23514');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET unique_id=-1 WHERE uid=920000003', '23514');
SELECT pg_temp.expect_error('UPDATE equipment_endgame_state SET unique_id=1001 WHERE uid=920000003', '23514');
INSERT INTO character_endgame_state VALUES(920000001,1,'{"keys":2,"season":"test"}');
INSERT INTO endgame_runs VALUES('sql-validation-run',1,'active','{"elapsed":17}');
INSERT INTO endgame_receipts VALUES(920000001,'sql-validation-01','cube',repeat('a',64),1,'{"ok":true}');
SELECT pg_temp.assert_true((SELECT payload->>'keys'='2' FROM character_endgame_state WHERE char_id=920000001), 'JSONB state reads back');
SELECT pg_temp.assert_true((SELECT response->>'ok'='true' FROM endgame_receipts WHERE char_id=920000001), 'JSONB receipt reads back');
DO $$ DECLARE n integer; BEGIN
  UPDATE character_endgame_state SET revision=2,payload='{"keys":1}' WHERE char_id=920000001 AND revision=1;
  GET DIAGNOSTICS n = ROW_COUNT; PERFORM pg_temp.assert_true(n=1,'state current CAS succeeds');
  UPDATE character_endgame_state SET revision=2,payload='{"keys":99}' WHERE char_id=920000001 AND revision=1;
  GET DIAGNOSTICS n = ROW_COUNT; PERFORM pg_temp.assert_true(n=0,'state stale CAS changes nothing');
  INSERT INTO character_endgame_state VALUES(920000001,1,'{}') ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS n = ROW_COUNT; PERFORM pg_temp.assert_true(n=0,'state initial CAS cannot overwrite');
  UPDATE endgame_runs SET revision=2,state='completed' WHERE run_id='sql-validation-run' AND revision=1;
  GET DIAGNOSTICS n = ROW_COUNT; PERFORM pg_temp.assert_true(n=1,'run current CAS succeeds');
  UPDATE endgame_runs SET revision=2,state='corrupt' WHERE run_id='sql-validation-run' AND revision=1;
  GET DIAGNOSTICS n = ROW_COUNT; PERFORM pg_temp.assert_true(n=0,'run stale CAS changes nothing');
  INSERT INTO endgame_runs VALUES('sql-validation-run',1,'corrupt','{}') ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS n = ROW_COUNT; PERFORM pg_temp.assert_true(n=0,'run initial CAS cannot overwrite');
  INSERT INTO endgame_receipts VALUES(920000001,'sql-validation-01','cube',repeat('b',64),2,'{}') ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS n = ROW_COUNT; PERFORM pg_temp.assert_true(n=0,'duplicate receipt returns zero affected rows');
END $$;
SELECT pg_temp.expect_error('INSERT INTO endgame_receipts VALUES(920000001,''sql-validation-01'',''cube'',repeat(''a'',64),1,''{}'')', '23505');
SELECT pg_temp.expect_error('INSERT INTO endgame_receipts VALUES(920000001,''short'',''cube'',repeat(''a'',64),1,''{}'')', '23514');
SELECT pg_temp.expect_error('INSERT INTO endgame_receipts VALUES(920000001,repeat(''x'',65),''cube'',repeat(''a'',64),1,''{}'')', '23514');
SELECT pg_temp.expect_error('INSERT INTO endgame_receipts VALUES(920000001,''sql-validation-02'',''cube'',''bad'',1,''{}'')', '23514');
SELECT pg_temp.expect_error('UPDATE character_endgame_state SET revision=0 WHERE char_id=920000001', '23514');
SELECT pg_temp.expect_error('UPDATE endgame_runs SET revision=0 WHERE run_id=''sql-validation-run''', '23514');
SELECT pg_temp.expect_error('UPDATE endgame_runs SET run_id='''' WHERE run_id=''sql-validation-run''', '23514');
SELECT pg_temp.expect_error('UPDATE endgame_runs SET payload=''invalid''::jsonb WHERE run_id=''sql-validation-run''', '22P02');
-- A failed final receipt insertion must roll back preceding changes in the same unit.
DO $$ BEGIN
  BEGIN
    UPDATE character_endgame_state SET revision=3 WHERE char_id=920000001;
    UPDATE equipment_instances SET refine_level=10 WHERE uid=920000001;
    UPDATE equipment_endgame_state SET revision=2 WHERE uid=920000001;
    UPDATE equipment_endgame_state SET revision=3 WHERE uid=920000003;
    UPDATE characters SET trial_key_reward=0 WHERE id=920000001;
    UPDATE endgame_runs SET revision=3 WHERE run_id='sql-validation-run';
    INSERT INTO endgame_receipts VALUES(920000001,'sql-validation-01','cube',repeat('a',64),3,'{}');
    RAISE EXCEPTION 'expected duplicate receipt';
  EXCEPTION WHEN unique_violation THEN NULL;
  END;
  PERFORM pg_temp.assert_true((SELECT revision=2 FROM character_endgame_state WHERE char_id=920000001),'failed receipt rolls back state');
  PERFORM pg_temp.assert_true((SELECT refine_level=3 FROM equipment_instances WHERE uid=920000001),'failed receipt rolls back native equipment');
  PERFORM pg_temp.assert_true((SELECT revision=1 FROM equipment_endgame_state WHERE uid=920000001),'failed receipt rolls back overlay');
  PERFORM pg_temp.assert_true((SELECT revision=2 FROM equipment_endgame_state WHERE uid=920000003),'failed receipt rolls back ordinary overlay');
  PERFORM pg_temp.assert_true((SELECT trial_key_reward=14 FROM characters WHERE id=920000001),'failed receipt rolls back trial reward');
  PERFORM pg_temp.assert_true((SELECT revision=2 FROM endgame_runs WHERE run_id='sql-validation-run'),'failed receipt rolls back run');
END $$;
SAVEPOINT cascade_check;
DELETE FROM item_instances WHERE uid=920000001;
SELECT pg_temp.assert_true(NOT EXISTS(SELECT FROM equipment_endgame_state WHERE uid=920000001),'UID deletion cascades overlay');
DELETE FROM characters WHERE id=920000001;
SELECT pg_temp.assert_true(NOT EXISTS(SELECT FROM character_endgame_state WHERE char_id=920000001) AND NOT EXISTS(SELECT FROM endgame_receipts WHERE char_id=920000001),'character deletion cascades state and receipts');
ROLLBACK TO SAVEPOINT cascade_check;
SELECT pg_temp.assert_true((SELECT count(*)=1 FROM equipment_endgame_state WHERE uid=920000001),'savepoint rollback restores UID overlay');
ROLLBACK;
DO $$ BEGIN
  IF EXISTS(SELECT FROM accounts WHERE id=920000001) OR EXISTS(SELECT FROM endgame_runs WHERE run_id='sql-validation-run') OR EXISTS(SELECT FROM equipment_endgame_state WHERE uid=920000001) THEN
    RAISE EXCEPTION 'outer transaction rollback leaked rows';
  END IF;
END $$;
SELECT 'DAOIST_ENDGAME_SQL_PASS' AS result;

-- OFFLINE EXISTING DATABASE ONLY. Stop/drain all game services and prevent restart.
-- Apply with psql -v ON_ERROR_STOP=1 -f this-file. No player identifiers required.
-- Only the 47 explicitly approved class-set templates, including their seven
-- set weapons. No special weapons, new drops, unclaimed Rift grants or history.
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
SELECT pg_advisory_xact_lock(1936224119,102);
CREATE TABLE IF NOT EXISTS class_set_true_conversion (
 migration_key TEXT PRIMARY KEY CHECK (migration_key = 'class-set-true-20261022'),
 converted_instances BIGINT NOT NULL CHECK (converted_instances >= 0),
 completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $conversion$
DECLARE expected BIGINT; changed BIGINT; tbl TEXT; extra TEXT;
BEGIN
 IF EXISTS (SELECT 1 FROM class_set_true_conversion WHERE migration_key='class-set-true-20261022') THEN
  RAISE NOTICE 'Class-set conversion already applied; no items changed';
  RETURN;
 END IF;
 PERFORM pg_stat_clear_snapshot();
 IF EXISTS (SELECT 1 FROM pg_stat_activity WHERE datid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND pid<>pg_backend_pid())
 OR EXISTS (SELECT 1 FROM pg_prepared_xacts WHERE database=current_database()) THEN
  RAISE EXCEPTION 'Class-set conversion requires an offline database with no other sessions or prepared transactions';
 END IF;
 LOCK TABLE item_instances,equipment_instances,character_items,character_equips,
 character_change_set_items,character_warehouse_items,character_mail_attachments,
 game_family_stash_items,character_stall_items,character_stalls
 IN ACCESS EXCLUSIVE MODE NOWAIT;
 CREATE TEMP TABLE class_set_true_map(old_id INT PRIMARY KEY,new_id INT UNIQUE NOT NULL) ON COMMIT DROP;
 INSERT INTO class_set_true_map(old_id,new_id) VALUES
 (8401,1800008401),
 (8451,1800008451),
 (9026,1800009026),
 (2126,1800002126),
 (2426,1800002426),
 (2526,1800002526),
 (2624,1800002624),
 (2576,1800002576),
 (9511,1800009511),
 (9512,1800009512),
 (8101,1800008101),
 (9029,1800009029),
 (2127,1800002127),
 (2427,1800002427),
 (2527,1800002527),
 (2625,1800002625),
 (2577,1800002577),
 (9811,1800009811),
 (9812,1800009812),
 (8201,1800008201),
 (9027,1800009027),
 (2128,1800002128),
 (2428,1800002428),
 (2528,1800002528),
 (2626,1800002626),
 (2578,1800002578),
 (9611,1800009611),
 (9612,1800009612),
 (8005,1800008005),
 (8051,1800008051),
 (9028,1800009028),
 (2129,1800002129),
 (2429,1800002429),
 (2529,1800002529),
 (2627,1800002627),
 (2579,1800002579),
 (9711,1800009711),
 (9712,1800009712),
 (8302,1800008302),
 (9030,1800009030),
 (2130,1800002130),
 (2430,1800002430),
 (2530,1800002530),
 (2628,1800002628),
 (2580,1800002580),
 (9911,1800009911),
 (9912,1800009912);
 CREATE TEMP VIEW class_set_owned_locations AS
 SELECT 'character_items'::text AS location, p.uid, p.item_id, to_jsonb(p) - 'item_id' AS untouched FROM character_items p
 UNION ALL
 SELECT 'character_equips'::text AS location, p.uid, p.item_id, to_jsonb(p) - 'item_id' AS untouched FROM character_equips p
 UNION ALL
 SELECT 'character_change_set_items'::text AS location, p.uid, p.item_id, to_jsonb(p) - 'item_id' AS untouched FROM character_change_set_items p
 UNION ALL
 SELECT 'character_warehouse_items'::text AS location, p.uid, p.item_id, to_jsonb(p) - 'item_id' AS untouched FROM character_warehouse_items p
 UNION ALL
 SELECT 'character_mail_attachments'::text AS location, p.uid, p.item_id, to_jsonb(p) - 'item_id' AS untouched FROM character_mail_attachments p
 UNION ALL
 SELECT 'game_family_stash_items'::text AS location, p.uid, p.item_id, to_jsonb(p) - 'item_id' AS untouched FROM game_family_stash_items p
 UNION ALL
 SELECT 'character_stall_items'::text AS location, p.uid, p.item_id, to_jsonb(p) - 'item_id' AS untouched FROM character_stall_items p JOIN character_stalls h ON h.char_id=p.char_id WHERE h.stall_type=0;
 CREATE TEMP TABLE class_set_conversion_targets ON COMMIT DROP AS
 SELECT p.*,m.new_id FROM class_set_owned_locations p JOIN class_set_true_map m ON m.old_id=p.item_id;
 SELECT count(*) INTO expected FROM class_set_conversion_targets;
 IF EXISTS (SELECT 1 FROM class_set_conversion_targets WHERE uid<=0)
 OR EXISTS (SELECT 1 FROM class_set_conversion_targets GROUP BY uid HAVING count(*)<>1) THEN
  RAISE EXCEPTION 'Invalid legacy UID or duplicate ownership; conversion aborted without repairing player data';
 END IF;
 IF EXISTS (SELECT 1 FROM class_set_conversion_targets t LEFT JOIN item_instances i ON i.uid=t.uid
 LEFT JOIN equipment_instances e ON e.uid=t.uid
 WHERE i.uid IS NULL OR e.uid IS NULL OR i.item_id<>t.item_id OR i.instance_kind<>1) THEN
  RAISE EXCEPTION 'Position/item/equipment fact mismatch; conversion aborted';
 END IF;
 IF EXISTS (SELECT 1 FROM class_set_conversion_targets t JOIN class_set_owned_locations p ON p.uid=t.uid
 GROUP BY t.uid HAVING count(*)<>1) THEN
  RAISE EXCEPTION 'Target UID appears at more than one owned location';
 END IF;
 -- A sell stall has UNIQUE(char_id,item_id). Never merge or delete two instances.
 IF EXISTS (SELECT p.char_id,COALESCE(m.new_id,p.item_id)
 FROM character_stall_items p JOIN character_stalls h ON h.char_id=p.char_id AND h.stall_type=0
 LEFT JOIN class_set_true_map m ON m.old_id=p.item_id
 GROUP BY p.char_id,COALESCE(m.new_id,p.item_id) HAVING count(*)>1) THEN
  RAISE EXCEPTION 'Old/true template collision in sell stall; resolve manually before conversion';
 END IF;
 CREATE TEMP TABLE class_set_conversion_facts ON COMMIT DROP AS
 SELECT t.uid,to_jsonb(i)-'item_id' AS item_before,to_jsonb(e) AS equipment_before
 FROM class_set_conversion_targets t JOIN item_instances i USING(uid) JOIN equipment_instances e USING(uid);
 UPDATE item_instances i SET item_id=t.new_id FROM class_set_conversion_targets t WHERE i.uid=t.uid AND i.item_id=t.item_id;
 GET DIAGNOSTICS changed=ROW_COUNT;
 IF changed<>expected THEN RAISE EXCEPTION 'Item fact conversion count mismatch'; END IF;
 FOREACH tbl IN ARRAY ARRAY['character_items','character_equips','character_change_set_items',
 'character_warehouse_items','character_mail_attachments','game_family_stash_items','character_stall_items'] LOOP
  extra:='';
  IF tbl='character_stall_items' THEN extra:=' AND EXISTS (SELECT 1 FROM character_stalls h WHERE h.char_id=p.char_id AND h.stall_type=0)'; END IF;
  EXECUTE format('UPDATE %I p SET item_id=t.new_id FROM class_set_conversion_targets t WHERE t.location=%L AND p.uid=t.uid AND p.item_id=t.item_id%s',tbl,tbl,extra);
 END LOOP;
 IF EXISTS (SELECT 1 FROM class_set_conversion_targets t LEFT JOIN class_set_owned_locations p ON p.location=t.location AND p.uid=t.uid
 WHERE p.uid IS NULL OR p.item_id<>t.new_id OR p.untouched<>t.untouched)
 OR EXISTS (SELECT 1 FROM class_set_conversion_facts f JOIN item_instances i USING(uid) JOIN equipment_instances e USING(uid)
 WHERE (to_jsonb(i)-'item_id')<>f.item_before OR to_jsonb(e)<>f.equipment_before) THEN
  RAISE EXCEPTION 'Conversion changed non-template instance fields or missed a location';
 END IF;
 PERFORM pg_stat_clear_snapshot();
 IF EXISTS (SELECT 1 FROM pg_stat_activity WHERE datid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND pid<>pg_backend_pid()) THEN
  RAISE EXCEPTION 'Another database session appeared during maintenance';
 END IF;
 INSERT INTO class_set_true_conversion(migration_key,converted_instances) VALUES('class-set-true-20261022',expected);
 DROP VIEW class_set_owned_locations;
 RAISE NOTICE 'Converted % existing class-set instances; UID and all non-template fields preserved',expected;
END
$conversion$;
COMMIT;

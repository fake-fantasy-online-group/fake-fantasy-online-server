-- Fresh-database initialization only. This is not an upgrades/ file and must
-- not be applied to an already-initialized production database.
--
-- 000099 publishes immutable Rift 10 but leaves game_rift_rule_active at 9;
-- it is contractually forbidden from activating the selector. Rift 10
-- (selector 10, floor_layout_version 6, published) was activated by a
-- separate maintenance step after 99, and official later-migration tests
-- model the same out-of-band UPDATE.
--
-- There is no numbered upgrade slot between 99 and 105, and already-applied
-- production upgrades through 127 must keep their identity. The empty-init
-- tool therefore splices this hook after 000000_init-99.sql so a fresh
-- database can walk 105-122 (10->19) to the same active selector production
-- reached after that historical activation.
DO $fresh_activate_rift10$
DECLARE active_version INTEGER;
BEGIN
 SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE;
 IF active_version IS NULL OR active_version < 9 THEN
  RAISE EXCEPTION 'fresh rift10 activation requires active rift9 or later';
 END IF;
 IF NOT EXISTS (
  SELECT FROM game_rift_rule_versions
   WHERE version=10 AND published AND floor_layout_version=6
 ) THEN
  RAISE EXCEPTION 'fresh rift10 activation requires published rift10 with generator 6';
 END IF;
 IF active_version=9 THEN
  UPDATE game_rift_rule_active SET version=10 WHERE id=1 AND version=9;
  IF NOT FOUND THEN RAISE EXCEPTION 'rift active selector changed during fresh v10 activation'; END IF;
 END IF;
END $fresh_activate_rift10$;

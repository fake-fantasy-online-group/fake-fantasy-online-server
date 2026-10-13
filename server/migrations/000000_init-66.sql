-- Missing native skill mechanics: explicitly authorized QQ Free Fantasy PC
-- parameter supplement. Source is a community gameplay transcription, NOT an
-- official original-package assertion. Only previously missing parameters are
-- imported; original damage, range, descriptions and authored ranks stay intact.
-- Source revision 2025-08-27 / oldid=4368, accessed 2026-10-13:
-- https://wiki.biligame.com/qqffo/index.php?title=药师&oldid=4368
-- Holy Light: non-player 60s, accuracy -10/-15/-20/-25/-30%; player 8s,
-- -10/-12/-14/-16/-18%. Divine Might: non-player 30%, player 10%/3s.
-- Existing native Divine Might non-player durations 6/7/8/9/10s are retained.
-- These target predicates do NOT enable PvP or change ordinary target admission.
ALTER TABLE game_skill_statuses ADD COLUMN IF NOT EXISTS target_type TEXT NOT NULL DEFAULT 'any'
 CHECK (target_type IN ('any','player','non_player'));
-- A storage-level alias only for the old cross-ID level comparison.
ALTER TABLE game_custom_statuses ADD COLUMN IF NOT EXISTS legacy_overlay_level_bp BIGINT NOT NULL DEFAULT 0
 CHECK (legacy_overlay_level_bp>=0);
-- Only same-ID replacement uses this optional final-affix strength axis.
-- Other IDs and all cross-ID overlay rules retain their original level ordering.
CREATE TABLE IF NOT EXISTS game_status_overlay_strengths (
 status_id INTEGER PRIMARY KEY CHECK(status_id>0),
 attr_id INTEGER NOT NULL CHECK(attr_id>0),
 mode INTEGER NOT NULL CHECK(mode IN (0,1)),
 value_order INTEGER NOT NULL CHECK(value_order IN (-1,1))
);

-- A first-import ledger prevents a replay from re-adding an administrator's
-- intentionally removed application or overwriting pre-existing configurations.
CREATE TABLE IF NOT EXISTS game_skill_mechanism_sources (
 skill_id INTEGER NOT NULL CHECK(skill_id>0),
 mechanic TEXT NOT NULL CHECK(length(mechanic)>0),
 source_url TEXT NOT NULL CHECK(length(source_url)>0),
 source_revision TEXT NOT NULL CHECK(length(source_revision)>0),
 imported BOOLEAN NOT NULL,
 PRIMARY KEY(skill_id,mechanic)
);

DO $skill_mechanisms$
DECLARE
 id INTEGER;
 icon_id INTEGER;
 fresh BOOLEAN;
BEGIN
 FOR id IN SELECT unnest(ARRAY[10317,10326]) LOOP
  INSERT INTO game_skill_mechanism_sources(skill_id,mechanic,source_url,source_revision,imported)
   VALUES(id,'target_scoped_status','https://wiki.biligame.com/qqffo/index.php?title=药师&oldid=4368',
    'QQ自由幻想端游社区整理，2025-08-27；仅补缺参',false)
   ON CONFLICT DO NOTHING;
  fresh := FOUND;
  IF NOT fresh THEN CONTINUE; END IF;
  IF EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=id) THEN CONTINUE; END IF;
  IF (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id=id AND skill_level BETWEEN 1 AND 5) <> 5 THEN
   RAISE EXCEPTION 'missing authored skill ranks for %',id;
  END IF;
  IF id=10317 THEN
   INSERT INTO game_status_overlay_strengths(status_id,attr_id,mode,value_order) VALUES(1008,19,1,-1) ON CONFLICT DO NOTHING;
   SELECT level_small_maps[1] INTO icon_id FROM gamedata.ov_exceptdesc WHERE type=1008;
   IF icon_id IS NULL OR icon_id<=0 THEN RAISE EXCEPTION 'blind status icon missing'; END IF;
   IF EXISTS(SELECT FROM gamedata.ov_exceptdetail d JOIN gamedata.ov_exceptdetail_entry e USING(row_no)
             WHERE d.type=1008 AND e.except_level BETWEEN 21 AND 25) THEN
    RAISE EXCEPTION 'reserved scoped blind status levels already native';
   END IF;
   INSERT INTO game_custom_statuses(status_id,status_level,name,description,status_type,duration_sec,
     icon,icon_known,configuration_version,legacy_overlay_level_bp)
    SELECT 1008,20+lv,'致盲',format('命中下降%s%%；',8+lv*2),1,8,icon_id,true,66,(8+lv*2)*2000
    FROM generate_series(1,5) lv;
   INSERT INTO game_custom_status_affixes(status_id,status_level,affix_order,attr_id,value,mode)
    SELECT 1008,20+lv,1,19,-(8+lv*2),1 FROM generate_series(1,5) lv;
   INSERT INTO game_skill_statuses(skill_id,skill_level,status_order,status_id,status_level,
     chance_bp,duration_sec,configuration_version,target_type)
    SELECT id,lv,1,1008,lv+1,10000,60,66,'non_player' FROM generate_series(1,5) lv
    UNION ALL
    SELECT id,lv,2,1008,lv+20,10000,8,66,'player' FROM generate_series(1,5) lv;
  ELSE
   SELECT level_small_maps[1] INTO icon_id FROM gamedata.ov_exceptdesc WHERE type=1034;
   IF icon_id IS NULL OR icon_id<=0 THEN RAISE EXCEPTION 'root status icon missing'; END IF;
   IF EXISTS(SELECT FROM gamedata.ov_exceptdetail d JOIN gamedata.ov_exceptdetail_entry e USING(row_no)
             WHERE d.type=1034 AND (e.except_level BETWEEN 31 AND 35 OR e.except_level=41)) THEN
    RAISE EXCEPTION 'reserved scoped root status levels already native';
   END IF;
   -- Pure root: the old 1034/11..15 rows also stun and reduce defense by 100%.
   -- Those extra mechanics are not promised by this skill and are not imported.
   INSERT INTO game_custom_statuses(status_id,status_level,name,description,status_type,duration_sec,
     block_move,immobile,icon,icon_known,configuration_version)
    SELECT 1034,30+lv,'定身','无法移动；',1,5+lv,true,1,icon_id,true,66
    FROM generate_series(1,5) lv
    UNION ALL SELECT 1034,41,'定身','无法移动；',1,3,true,1,icon_id,true,66;
   INSERT INTO game_skill_statuses(skill_id,skill_level,status_order,status_id,status_level,
     chance_bp,duration_sec,configuration_version,target_type)
    SELECT id,lv,1,1034,lv+30,3000,5+lv,66,'non_player' FROM generate_series(1,5) lv
    UNION ALL
    SELECT id,lv,2,1034,41,1000,3,66,'player' FROM generate_series(1,5) lv;
  END IF;
  UPDATE game_skill_mechanism_sources SET imported=true WHERE skill_id=id AND mechanic='target_scoped_status';
 END LOOP;
END
$skill_mechanisms$;

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
-- Authored non-equipment binding policy, never inferred from route or trade flags.
-- Upgrade MUST run offline after final character save and a verified backup.
-- No merge, UID reassignment, lock/effect/quantity/currency change or ACL change.
ALTER TABLE game_items ADD COLUMN IF NOT EXISTS fixed_binding boolean;
ALTER TABLE game_items ADD COLUMN IF NOT EXISTS binding_rule_source text;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS fixed_binding boolean;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS binding_rule_source text;
CREATE TEMP TABLE fixed_item_binding_stage(id integer PRIMARY KEY,name text NOT NULL,bound boolean NOT NULL,"table" text NOT NULL,source text NOT NULL) ON COMMIT DROP;
INSERT INTO fixed_item_binding_stage SELECT * FROM json_to_recordset($fixed_item_bindings$
[
  {
    "id": 4001,
    "name": "铁矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8691;000000_init-28.sql:5763"
  },
  {
    "id": 4004,
    "name": "铜矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8724;000000_init-28.sql:5766"
  },
  {
    "id": 4007,
    "name": "银矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8760;000000_init-28.sql:5769"
  },
  {
    "id": 4010,
    "name": "金矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8793;000000_init-28.sql:5773"
  },
  {
    "id": 4013,
    "name": "水晶矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8826;000000_init-28.sql:5776"
  },
  {
    "id": 4016,
    "name": "乌金矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8861;000000_init-28.sql:5779"
  },
  {
    "id": 4019,
    "name": "星尘矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8895;000000_init-28.sql:5783"
  },
  {
    "id": 4022,
    "name": "太阴矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8928;000000_init-28.sql:5786"
  },
  {
    "id": 4025,
    "name": "太阳矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:8963;000000_init-28.sql:5789"
  },
  {
    "id": 4028,
    "name": "神天矿石",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-29.sql:12492;000000_init-28.sql:5792"
  },
  {
    "id": 8038,
    "name": "武器灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-24.sql:10260;000000_init-28.sql:8564"
  },
  {
    "id": 11682,
    "name": "暗之羽",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:2729;000000_init-28.sql:10119"
  },
  {
    "id": 24608,
    "name": "项灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-25.sql:2335;000000_init-27.sql:13974"
  },
  {
    "id": 24640,
    "name": "鞋灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-25.sql:2359;000000_init-27.sql:13998"
  },
  {
    "id": 25027,
    "name": "杂乱的羽毛",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:6065;000000_init-27.sql:14120"
  },
  {
    "id": 25028,
    "name": "精致的羽毛",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:6066;000000_init-27.sql:14121"
  },
  {
    "id": 25029,
    "name": "神奇的羽毛",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-30.sql:6067;000000_init-27.sql:14122"
  },
  {
    "id": 26206,
    "name": "上阵加持丹",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-30.sql:6953;000000_init-27.sql:15007"
  },
  {
    "id": 26387,
    "name": "逆袭令牌（绑定）",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-30.sql:6979;000000_init-27.sql:15036"
  },
  {
    "id": 27418,
    "name": "山河印（绑定）",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-30.sql:7118;000000_init-27.sql:15172"
  },
  {
    "id": 27420,
    "name": "山河战场入场凭证（绑定）",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-30.sql:7120;000000_init-27.sql:15174"
  },
  {
    "id": 27424,
    "name": "山河图腾（绑定）",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-30.sql:7124;000000_init-27.sql:15181"
  },
  {
    "id": 29874,
    "name": "衣灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-25.sql:4979;000000_init-27.sql:17720"
  },
  {
    "id": 35716,
    "name": "玫瑰（绑定版）",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-30.sql:8956;000000_init-28.sql:1041"
  },
  {
    "id": 36200,
    "name": "须弥副本钥匙绑定版",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-29.sql:12641;000000_init-28.sql:1233"
  },
  {
    "id": 42564,
    "name": "超强生命药水绑定",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-29.sql:12903;000000_init-28.sql:2281"
  },
  {
    "id": 47004,
    "name": "帽灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-25.sql:6374;000000_init-28.sql:2683"
  },
  {
    "id": 47028,
    "name": "拳灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-25.sql:6401;000000_init-28.sql:2713"
  },
  {
    "id": 47490,
    "name": "面灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-26.sql:214;000000_init-28.sql:3050"
  },
  {
    "id": 47501,
    "name": "羽灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-26.sql:226;000000_init-28.sql:3062"
  },
  {
    "id": 47640,
    "name": "法宝灵",
    "bound": false,
    "table": "game_equipment",
    "source": "000000_init-26.sql:263;000000_init-28.sql:3097"
  },
  {
    "id": 1900028001,
    "name": "星篆·角木蛟",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:12"
  },
  {
    "id": 1900028002,
    "name": "星篆·亢金龙",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:13"
  },
  {
    "id": 1900028003,
    "name": "星篆·氐土貉",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:14"
  },
  {
    "id": 1900028004,
    "name": "星篆·房日兔",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:15"
  },
  {
    "id": 1900028005,
    "name": "星篆·心月狐",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:16"
  },
  {
    "id": 1900028006,
    "name": "星篆·尾火虎",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:17"
  },
  {
    "id": 1900028007,
    "name": "星篆·箕水豹",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:18"
  },
  {
    "id": 1900028008,
    "name": "星篆·斗木獬",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:19"
  },
  {
    "id": 1900028009,
    "name": "星篆·牛金牛",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:20"
  },
  {
    "id": 1900028010,
    "name": "星篆·女土蝠",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:21"
  },
  {
    "id": 1900028011,
    "name": "星篆·虚日鼠",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:22"
  },
  {
    "id": 1900028012,
    "name": "星篆·危月燕",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:23"
  },
  {
    "id": 1900028013,
    "name": "星篆·室火猪",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:24"
  },
  {
    "id": 1900028014,
    "name": "星篆·壁水貐",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:25"
  },
  {
    "id": 1900028015,
    "name": "星篆·奎木狼",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:26"
  },
  {
    "id": 1900028016,
    "name": "星篆·娄金狗",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:27"
  },
  {
    "id": 1900028017,
    "name": "星篆·胃土雉",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:28"
  },
  {
    "id": 1900028018,
    "name": "星篆·昴日鸡",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:29"
  },
  {
    "id": 1900028019,
    "name": "星篆·毕月乌",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:30"
  },
  {
    "id": 1900028020,
    "name": "星篆·觜火猴",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:31"
  },
  {
    "id": 1900028021,
    "name": "星篆·参水猿",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:32"
  },
  {
    "id": 1900028022,
    "name": "星篆·井木犴",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:33"
  },
  {
    "id": 1900028023,
    "name": "星篆·鬼金羊",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:34"
  },
  {
    "id": 1900028024,
    "name": "星篆·柳土獐",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:35"
  },
  {
    "id": 1900028025,
    "name": "星篆·星日马",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:36"
  },
  {
    "id": 1900028026,
    "name": "星篆·张月鹿",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:37"
  },
  {
    "id": 1900028027,
    "name": "星篆·翼火蛇",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:38"
  },
  {
    "id": 1900028028,
    "name": "星篆·轸水蚓",
    "bound": false,
    "table": "game_items",
    "source": "000000_init-59.sql:39"
  },
  {
    "id": 1900028101,
    "name": "洞天符钥",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-59.sql:40"
  },
  {
    "id": 1900028201,
    "name": "星髓",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-59.sql:41"
  },
  {
    "id": 1900028202,
    "name": "净煞尘",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-59.sql:42"
  },
  {
    "id": 1900028203,
    "name": "八仙余韵",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-59.sql:43"
  },
  {
    "id": 1900028301,
    "name": "周天鼎",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-84.sql:37"
  },
  {
    "id": 1900028302,
    "name": "万象秘典",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-118.sql:16"
  },
  {
    "id": 1900028303,
    "name": "威能卷轴",
    "bound": true,
    "table": "game_items",
    "source": "000000_init-118.sql:17"
  }
]
$fixed_item_bindings$::json) AS x(id integer,name text,bound boolean,"table" text,source text);
LOCK TABLE game_items,game_equipment,item_instances,card_instances,equipment_instances,equipment_spirit_fusions,
 character_items,character_warehouse_items,character_stall_items,character_mail_attachments,
 game_family_stash_items,character_equips,character_change_set_items IN SHARE ROW EXCLUSIVE MODE;
DO $guard$
BEGIN
 IF (SELECT count(*) FROM fixed_item_binding_stage)<>66 OR
 (SELECT count(*) FROM fixed_item_binding_stage s JOIN game_items i ON i.id=s.id AND i.name=s.name WHERE s."table"='game_items')<>57 OR
 (SELECT count(*) FROM fixed_item_binding_stage s JOIN game_equipment i ON i.id=s.id AND i.name=s.name WHERE s."table"='game_equipment')<>9 OR
 EXISTS(SELECT FROM fixed_item_binding_stage s JOIN game_items i USING(id) WHERE s."table"<>'game_items') OR
 EXISTS(SELECT FROM fixed_item_binding_stage s JOIN game_equipment i USING(id) WHERE s."table"<>'game_equipment') THEN
  RAISE EXCEPTION 'fixed binding authored template identity/coverage mismatch';
 END IF;
 IF EXISTS(SELECT FROM character_equips e JOIN fixed_item_binding_stage s ON s.id=e.item_id) OR
 EXISTS(SELECT FROM character_change_set_items e JOIN fixed_item_binding_stage s ON s.id=e.item_id) THEN
  RAISE EXCEPTION 'fixed binding target unexpectedly occupies actual equipment location';
 END IF;
 IF EXISTS(SELECT FROM game_items i JOIN fixed_item_binding_stage s USING(id) WHERE i.fixed_binding IS NOT NULL AND i.fixed_binding<>s.bound) OR
 EXISTS(SELECT FROM game_equipment i JOIN fixed_item_binding_stage s USING(id) WHERE i.fixed_binding IS NOT NULL AND i.fixed_binding<>s.bound) THEN
  RAISE EXCEPTION 'conflicting previously authored binding policy';
 END IF;
END $guard$;
UPDATE game_items i SET fixed_binding=s.bound,binding_rule_source=s.source FROM fixed_item_binding_stage s
 WHERE s."table"='game_items' AND i.id=s.id AND (i.fixed_binding IS DISTINCT FROM s.bound OR i.binding_rule_source IS DISTINCT FROM s.source);
UPDATE game_equipment i SET fixed_binding=s.bound,binding_rule_source=s.source FROM fixed_item_binding_stage s
 WHERE s."table"='game_equipment' AND i.id=s.id AND (i.fixed_binding IS DISTINCT FROM s.bound OR i.binding_rule_source IS DISTINCT FROM s.source);
-- Capture complete ownership first. No unowned historical root is repaired/deleted.
CREATE TEMP TABLE fixed_binding_locations ON COMMIT DROP AS
 SELECT 'bag'::text location,uid,item_id FROM character_items
 UNION ALL SELECT 'warehouse',uid,item_id FROM character_warehouse_items
 UNION ALL SELECT 'stall',uid,item_id FROM character_stall_items
 UNION ALL SELECT 'mail',uid,item_id FROM character_mail_attachments
 UNION ALL SELECT 'family',uid,item_id FROM game_family_stash_items
 UNION ALL SELECT 'equip',uid,item_id FROM character_equips
 UNION ALL SELECT 'change',uid,item_id FROM character_change_set_items;
CREATE TEMP TABLE fixed_binding_refs ON COMMIT DROP AS
 SELECT uid,item_id FROM fixed_binding_locations WHERE uid>0
 UNION ALL SELECT c.uid,c.item_id FROM equipment_instances e JOIN fixed_binding_locations p ON p.uid=e.uid
 JOIN item_instances c ON c.uid=ANY(e.socket_instance_uids)
 UNION ALL SELECT c.uid,c.item_id FROM equipment_spirit_fusions f JOIN fixed_binding_locations p ON p.uid=f.equipment_uid
 JOIN item_instances c ON c.uid=f.spirit_uid;
CREATE TEMP TABLE fixed_binding_active ON COMMIT DROP AS
 SELECT DISTINCT r.uid,r.item_id,s.bound FROM fixed_binding_refs r JOIN fixed_item_binding_stage s ON s.id=r.item_id;
DO $identity$
BEGIN
 IF EXISTS(SELECT FROM fixed_binding_active a LEFT JOIN item_instances i USING(uid) WHERE i.uid IS NULL OR i.item_id<>a.item_id) OR
 EXISTS(SELECT FROM fixed_binding_refs r JOIN fixed_binding_active a USING(uid) GROUP BY r.uid HAVING count(*)<>1) OR
 EXISTS(SELECT FROM fixed_binding_active a JOIN item_instances i USING(uid) LEFT JOIN card_instances c USING(uid)
 WHERE i.instance_kind=2 AND c.uid IS NULL) THEN
  RAISE EXCEPTION 'fixed binding active UID missing, identity mismatch, duplicate ownership or missing card';
 END IF;
END $identity$;
-- Byte-for-byte JSON field invariants except precisely the authored binding bit.
CREATE TEMP TABLE fixed_binding_before(table_name text PRIMARY KEY,row_count bigint,digest text) ON COMMIT DROP;
DO $before$
DECLARE tbl text; removed text;
BEGIN
 FOREACH tbl IN ARRAY ARRAY['item_instances','card_instances','equipment_instances','character_items','character_warehouse_items','character_stall_items','character_mail_attachments','game_family_stash_items'] LOOP
  removed := CASE WHEN tbl='card_instances' THEN 'snapshot_bound' ELSE 'bound' END;
  EXECUTE format('INSERT INTO fixed_binding_before SELECT %L,count(*),md5(coalesce(string_agg((to_jsonb(t)-%L)::text,''|'' ORDER BY (to_jsonb(t)-%L)::text),'''')) FROM %I t',tbl,removed,removed,tbl);
 END LOOP;
END $before$;
CREATE TEMP TABLE fixed_binding_unowned_before ON COMMIT DROP AS
 SELECT i.uid,to_jsonb(i) AS root,to_jsonb(c) AS card,to_jsonb(e) AS equipment FROM item_instances i
 JOIN fixed_item_binding_stage s ON s.id=i.item_id LEFT JOIN card_instances c USING(uid) LEFT JOIN equipment_instances e USING(uid)
 WHERE NOT EXISTS(SELECT FROM fixed_binding_active a WHERE a.uid=i.uid);
UPDATE item_instances i SET bound=a.bound FROM fixed_binding_active a WHERE i.uid=a.uid AND i.bound IS DISTINCT FROM a.bound;
UPDATE card_instances c SET snapshot_bound=a.bound FROM fixed_binding_active a JOIN item_instances i USING(uid)
 WHERE c.uid=a.uid AND i.instance_kind=2 AND c.snapshot_bound IS DISTINCT FROM a.bound;
-- Kind1 books are non-equipment definitions but may own an equipment storage row.
UPDATE equipment_instances e SET bound=a.bound FROM fixed_binding_active a WHERE e.uid=a.uid AND e.bound IS DISTINCT FROM a.bound;
DO $locations$
DECLARE tbl text;
BEGIN
 FOREACH tbl IN ARRAY ARRAY['character_items','character_warehouse_items','character_stall_items','character_mail_attachments','game_family_stash_items'] LOOP
  EXECUTE format('UPDATE %I t SET bound=s.bound FROM fixed_item_binding_stage s WHERE t.item_id=s.id AND t.bound IS DISTINCT FROM s.bound',tbl);
 END LOOP;
END $locations$;
DO $verify$
DECLARE before_row record; removed text; n bigint; h text;
BEGIN
 FOR before_row IN SELECT * FROM fixed_binding_before LOOP
  removed := CASE WHEN before_row.table_name='card_instances' THEN 'snapshot_bound' ELSE 'bound' END;
  EXECUTE format('SELECT count(*),md5(coalesce(string_agg((to_jsonb(t)-%L)::text,''|'' ORDER BY (to_jsonb(t)-%L)::text),'''')) FROM %I t',removed,removed,before_row.table_name) INTO n,h;
  IF n<>before_row.row_count OR h IS DISTINCT FROM before_row.digest THEN RAISE EXCEPTION 'fixed binding changed non-binding fields/table %',before_row.table_name; END IF;
 END LOOP;
 IF EXISTS(SELECT FROM fixed_binding_unowned_before b LEFT JOIN item_instances i USING(uid) LEFT JOIN card_instances c USING(uid) LEFT JOIN equipment_instances e USING(uid)
 WHERE b.root IS DISTINCT FROM to_jsonb(i) OR b.card IS DISTINCT FROM to_jsonb(c) OR b.equipment IS DISTINCT FROM to_jsonb(e)) THEN
  RAISE EXCEPTION 'unowned historical instance changed';
 END IF;
 IF EXISTS(SELECT FROM fixed_binding_active a JOIN item_instances i USING(uid) LEFT JOIN card_instances c USING(uid) LEFT JOIN equipment_instances e USING(uid)
 WHERE i.bound IS DISTINCT FROM a.bound OR (i.instance_kind=2 AND c.snapshot_bound IS DISTINCT FROM a.bound) OR (e.uid IS NOT NULL AND e.bound IS DISTINCT FROM a.bound)) THEN
  RAISE EXCEPTION 'active instance binding verification failed';
 END IF;
END $verify$;
COMMIT;

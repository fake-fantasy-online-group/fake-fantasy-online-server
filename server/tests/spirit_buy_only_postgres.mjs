import {createRequire} from 'node:module';
import fs from 'node:fs';
import assert from 'node:assert/strict';
const {PGlite}=createRequire(import.meta.url)(process.env.PGLITE_MODULE||'@electric-sql/pglite');
const root=new URL('../migrations/',import.meta.url);
const sql=fs.readFileSync(new URL('upgrades/000125_20261028_spirit_buy_only.sql',root),'utf8');
const init=fs.readFileSync(new URL('000000_init-125.sql',root),'utf8');
const previous=fs.readFileSync(new URL('upgrades/000124_20261028_ui_review_metadata.sql',root),'utf8');
assert.equal(sql,'BEGIN;\n'+init+'COMMIT;\n');
const db=new PGlite();
await db.exec(`
CREATE SCHEMA gamedata;
CREATE TABLE game_items(id int PRIMARY KEY,name text,use_action text,price int);
CREATE TABLE game_equipment(id int PRIMARY KEY,price int);
CREATE TABLE gamedata.ov_item(row_no int PRIMARY KEY,index int,name text,can_deal int,can_mail int,use_waste int);
CREATE TABLE gamedata.ov_desc(row_no int PRIMARY KEY,index int,name_2 text,small_map int,big_map int,desc_ text,can_deal int,can_mail int);
CREATE TABLE map_npcs(map_file text,npc_id int,name text,script text,sell_list int);
CREATE TABLE game_shops(shop_id int PRIMARY KEY,source_kind text,source_note text,allow_sell boolean,allow_repair boolean,stock_source_shop_id int);
CREATE TABLE game_shop_items(shop_id int,slot int,item_id int);
CREATE TABLE preserved_state(kind text PRIMARY KEY,state jsonb);
INSERT INTO preserved_state VALUES('quest','{"completed":true,"reclaim":true}'),('daily','{"day":"2026-10-28"}'),('instance','{"uid":42,"bound":true,"roll":8123}');
INSERT INTO game_items VALUES(1900028302,'万象秘典','power_codex',0),(1900028303,'威能卷轴','power_scroll',0),(47501,'羽灵','',10000000);
INSERT INTO game_equipment VALUES(47501,10000000);
INSERT INTO gamedata.ov_item VALUES(1900028302,1900028302,'万象秘典',0,0,0),(1900028303,1900028303,'威能卷轴',0,0,0);
INSERT INTO gamedata.ov_desc VALUES(5486,10757,'封印的涅磐之书',12143,12143,'native',1,1),(1900028302,1900028302,'',999,999,'old book',0,0),(1900028303,1900028303,'',25030,25030,'old scroll',0,0),(99,99,'other',9,9,'untouched',1,1);
INSERT INTO map_npcs VALUES('qw0007',257,'羽灵仙子','Nyuling1',6203),('qw0007',163,'鞋灵仙子','Nxieling',6201),('qw0007',999,'戒灵仙子','ring',777);
INSERT INTO game_shops SELECT i,'inferred','spirit sealed purchase 10 gold',true,false,NULL FROM generate_series(6201,6209) i;
INSERT INTO game_shops VALUES(777,'original','ring',true,true,NULL);
INSERT INTO game_shop_items VALUES(6203,0,47501),(6201,0,24640);
`);

await db.exec(`DELETE FROM map_npcs;DELETE FROM game_shop_items;
INSERT INTO map_npcs(map_file,npc_id,name,script,sell_list) VALUES
('qw0007',163,'鞋灵仙子','Nxieling',6201),('qw0007',241,'拳灵仙子','Nquanling',6202),('qw0007',257,'羽灵仙子','Nyuling1',6203),
('qw0007',273,'衣灵仙子','Nyiling',6204),('qw0007',79,'项灵仙子','Nxiangling',6205),('qw0007',122,'面灵仙子','Nmianling',6206),
('qw0007',279,'帽灵仙子','Nmaoling',6207),('qw0007',283,'法宝灵仙子','Nfabaoling',6208),('qw0007',287,'武灵仙子','Nwuling',6209),
('qw0007',130,'戒灵仙子','Njieling',0),('qw0007',999,'ordinary','merchant',777);
INSERT INTO game_shop_items VALUES(6201,0,24640),(6202,0,47028),(6203,0,47501),(6204,0,29874),(6205,0,24608),(6206,0,47490),(6207,0,47004),(6208,0,47640),(6209,0,8038),(777,0,1);`);
await db.exec(previous);
const tables=['game_items','game_equipment','gamedata.ov_item','gamedata.ov_desc','map_npcs','game_shops','game_shop_items','preserved_state'];
async function snapshot(){const out={};for(const table of tables)out[table]=(await db.query(`SELECT to_jsonb(t) AS v FROM ${table} t ORDER BY to_jsonb(t)::text`)).rows;return out;}
const baseline=await snapshot();
for(const mutate of [
 "DELETE FROM map_npcs WHERE npc_id=163", "UPDATE map_npcs SET script='wrong' WHERE npc_id=241",
 "UPDATE game_shops SET source_note='wrong' WHERE shop_id=6204", "DELETE FROM game_shops WHERE shop_id=6205",
 "UPDATE game_shop_items SET item_id=1 WHERE shop_id=6206", "INSERT INTO game_shop_items VALUES(6207,1,2)",
 "INSERT INTO map_npcs VALUES('qw0008',1,'other','other',6208)","UPDATE map_npcs SET name='wrong' WHERE npc_id=287"
]){
 await db.exec('BEGIN;'+mutate);await assert.rejects(db.exec(init),/requires nine exclusive/);await db.exec('ROLLBACK');assert.deepEqual(await snapshot(),baseline);
}
await db.exec(sql);const after=await snapshot();
for(const table of tables.filter(t=>t!=='game_shops'))assert.deepEqual(after[table],baseline[table],table);
for(const {v:old} of baseline.game_shops){const current=(await db.query('SELECT * FROM game_shops WHERE shop_id=$1',[old.shop_id])).rows[0];assert.deepEqual(current,{...old,allow_sell:old.shop_id>=6201&&old.shop_id<=6209?false:old.allow_sell});}
assert.equal((await db.query('SELECT count(*)::int n FROM game_shops WHERE shop_id BETWEEN 6201 AND 6209 AND NOT allow_sell')).rows[0].n,9);
assert.deepEqual((await db.query("SELECT sell_list FROM map_npcs WHERE script='Njieling'")).rows,[{sell_list:0}]);
await db.exec(sql);assert.deepEqual(await snapshot(),after);
await db.exec('BEGIN');await db.exec(init);await db.exec('ROLLBACK');assert.deepEqual(await snapshot(),after);
await db.close();
console.log('PASS:125 after124; all9 spirit shops buy-only;8 identity/collision failures roll back; idempotence/init parity; ordinary merchant/ring/stock/prices/book metadata/quest/daily/state unchanged (reduced PostgreSQL fixture).');

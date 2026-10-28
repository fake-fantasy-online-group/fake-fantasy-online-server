// Reduced PostgreSQL prerequisites, not a production/full-seed deployment.
import {createRequire} from 'node:module';
import fs from 'node:fs';
import assert from 'node:assert/strict';
const {PGlite}=createRequire(import.meta.url)(process.env.PGLITE_MODULE||'@electric-sql/pglite');
const root=new URL('../migrations/',import.meta.url);
const sql=fs.readFileSync(new URL('upgrades/000124_20261028_ui_review_metadata.sql',root),'utf8');
const init=fs.readFileSync(new URL('000000_init-124.sql',root),'utf8');
assert.equal(sql,'BEGIN;\n'+init+'COMMIT;\n');
// Bind icon evidence to the original seed, not an invented label-to-icon mapping.
const original=fs.readFileSync(new URL('000000_init-28.sql',root),'utf8');
assert.match(original,/\(5486, 10757, '', 0, 12143, 12143,[^\n]+封印的涅磐之书/);
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
const tables=['game_items','game_equipment','gamedata.ov_item','gamedata.ov_desc','map_npcs','game_shops','game_shop_items','preserved_state'];
async function snapshot(){const result={};for(const table of tables)result[table]=(await db.query(`SELECT to_jsonb(t) AS v FROM ${table} t ORDER BY to_jsonb(t)::text`)).rows;return result;}
const baseline=await snapshot();
const failures=[
 ["DELETE FROM game_items WHERE id=1900028303",/requires power codex/],
 ["UPDATE game_items SET use_action='other' WHERE id=1900028302",/requires power codex/],
 ["INSERT INTO game_equipment VALUES(1900028302,0)",/identity collision/],
 ["UPDATE gamedata.ov_item SET name='wrong' WHERE index=1900028302",/identity collision/],
 ["UPDATE gamedata.ov_item SET index=123 WHERE row_no=1900028302",/identity collision/],
 ["UPDATE gamedata.ov_desc SET index=123 WHERE row_no=1900028303",/identity collision/],
 ["DELETE FROM gamedata.ov_desc WHERE index=1900028303",/identity collision/],
 ["UPDATE gamedata.ov_desc SET small_map=1 WHERE index=10757",/native nirvana/],
 ["UPDATE gamedata.ov_desc SET name_2='wrong' WHERE index=10757",/native nirvana/],
 ["UPDATE map_npcs SET script='wrong' WHERE npc_id=257",/exclusive native/],
 ["UPDATE game_shops SET source_note='wrong' WHERE shop_id=6203",/exclusive native/],
 ["UPDATE game_shop_items SET item_id=1 WHERE shop_id=6203",/exclusive native/],
 ["INSERT INTO game_shop_items VALUES(6203,1,123)",/exclusive native/],
 ["INSERT INTO map_npcs VALUES('qw0008',1,'other','other',6203)",/exclusive native/]
];
for(const [mutation,error] of failures){
 await db.exec('BEGIN;'+mutation);
 await assert.rejects(db.exec(init),error);
 await db.exec('ROLLBACK');
 assert.deepEqual(await snapshot(),baseline,mutation);
}
await db.exec(sql);
const after=await snapshot();
for(const table of tables.filter(t=>!['gamedata.ov_desc','game_shops'].includes(t)))assert.deepEqual(after[table],baseline[table],table);
const rows=(await db.query('SELECT * FROM gamedata.ov_desc ORDER BY index')).rows;
assert.equal(rows.find(r=>r.index===1900028302).desc_,'巧匠左慈赠予的秘典，铭记分解暗金时留下的威能。右击翻阅。');
assert.equal(rows.find(r=>r.index===1900028303).desc_,'记录一项威能的卷轴。右击后选择暗金装备或羽翼，将此卷所载威能附着其上。');
for(const field of ['small_map','big_map'])assert.equal(rows.find(r=>r.index===1900028302)[field],12143);
for(let i=0;i<baseline['gamedata.ov_desc'].length;i++){
 const old=baseline['gamedata.ov_desc'][i].v;const current=rows.find(r=>r.row_no===old.row_no);
 const expected={...old};if(old.index===1900028302)Object.assign(expected,{small_map:12143,big_map:12143,desc_:current.desc_});
 if(old.index===1900028303)expected.desc_=current.desc_;
 assert.deepEqual(current,expected);
}
for(const {v:old} of baseline.game_shops){const current=(await db.query('SELECT * FROM game_shops WHERE shop_id=$1',[old.shop_id])).rows[0];assert.deepEqual(current,{...old,allow_sell:old.shop_id===6203?false:old.allow_sell});}
await db.exec(sql);assert.deepEqual(await snapshot(),after);
// Explicit surrounding rollback of the fresh-init path.
await db.exec("BEGIN;UPDATE gamedata.ov_desc SET desc_='rollback sentinel' WHERE index=1900028302;");
await db.exec(init);await db.exec('ROLLBACK');assert.deepEqual(await snapshot(),after);
await db.close();
console.log('PASS:124 exact short metadata/native12143; 14 identity/prerequisite/collision failures roll back; idempotent; init parity/rollback; only wing shop sell disabled; other8/ring/buy prices/binding/consumption/quest/instance/daily untouched (reduced PostgreSQL fixture).');

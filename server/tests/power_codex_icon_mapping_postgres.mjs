import {createRequire} from 'node:module';
import fs from 'node:fs';
import assert from 'node:assert/strict';
const {PGlite}=createRequire(import.meta.url)(process.env.PGLITE_MODULE||'@electric-sql/pglite');
const root=new URL('../migrations/',import.meta.url);
const sql=fs.readFileSync(new URL('upgrades/000126_20261028_power_codex_icon_mapping.sql',root),'utf8');
const init=fs.readFileSync(new URL('000000_init-126.sql',root),'utf8');
assert.equal(sql,'BEGIN;\n'+init+'COMMIT;\n');
const db=new PGlite();
await db.exec(`CREATE SCHEMA gamedata;
CREATE TABLE game_items(id int PRIMARY KEY,use_action text);
CREATE TABLE gamedata.ov_desc(index int,small_map int,big_map int);
CREATE TABLE gamedata.ov_item_icon_special(row_no integer PRIMARY KEY,big_special_id text,arm_id bigint,icon_small text);
INSERT INTO game_items VALUES(1900028302,'power_codex'),(1900028303,'power_scroll');
INSERT INTO gamedata.ov_desc VALUES(1900028302,12143,12143),(1900028303,39440,39440);
INSERT INTO gamedata.ov_item_icon_special VALUES(0,'8230',57093,'8231');`);
const snap=async()=> (await db.query('SELECT * FROM gamedata.ov_item_icon_special ORDER BY row_no')).rows;
const before=await snap();
for(const mutation of [
 "INSERT INTO gamedata.ov_item_icon_special VALUES(1900028302,'1',999,'1')",
 "INSERT INTO gamedata.ov_item_icon_special VALUES(999,'12143',1900028302,'12143')",
 "INSERT INTO gamedata.ov_item_icon_special VALUES(1900028302,NULL,1900028302,NULL)",
 "UPDATE gamedata.ov_desc SET small_map=1 WHERE index=1900028302",
 "UPDATE game_items SET use_action='wrong' WHERE id=1900028303"
]) {await db.exec('BEGIN;'+mutation);await assert.rejects(db.exec(init),/Power icon/);await db.exec('ROLLBACK');assert.deepEqual(await snap(),before);}
await db.exec(sql);const after=await snap();assert.equal(after.length,3);assert.deepEqual(after[0],before[0]);assert.equal(after[1].icon_small,'12143');assert.equal(after[2].icon_small,'39440');
await db.exec(sql);assert.deepEqual(await snap(),after);
console.log('PASS PostgreSQL icon mapping apply, idempotence, collision/null/prerequisite rollback and unrelated row preservation');await db.close();

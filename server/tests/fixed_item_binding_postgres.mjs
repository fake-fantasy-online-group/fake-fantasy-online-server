// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/fixed_item_binding_postgres.mjs
import fs from 'node:fs';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const sql=fs.readFileSync(new URL('../migrations/000000_init-127.sql',import.meta.url),'utf8');
const upgrade=fs.readFileSync(new URL('../migrations/upgrades/000127_20261029_fixed_item_binding.sql',import.meta.url),'utf8');
assert.equal(upgrade,'BEGIN;\n'+sql+'COMMIT;\n');
const rules=JSON.parse(sql.split('$fixed_item_bindings$')[1]);
assert.equal(rules.length,66);assert.equal(new Set(rules.map(x=>x.id)).size,66);
assert(!rules.some(x=>x.id>=3334&&x.id<=3338));
async function fixture(){
 const db=new PGlite();
 await db.exec(`CREATE TABLE game_items(id int primary key,name text); CREATE TABLE game_equipment(id int primary key,name text);
 CREATE TABLE item_instances(uid bigint primary key,item_id int,instance_kind int,bound bool,locked bool,created_source text);
 CREATE TABLE card_instances(uid bigint primary key,effect_count int,ops int[],snapshot_bound bool,snapshot_locked bool);
 CREATE TABLE equipment_instances(uid bigint primary key,bound bool,locked bool,socket_instance_uids bigint[],durability int);
 CREATE TABLE equipment_spirit_fusions(equipment_uid bigint primary key,spirit_uid bigint,appearance_item_id int);
 CREATE TABLE character_items(char_id bigint,slot int,uid bigint,item_id int,count int,bound bool,locked bool);
 CREATE TABLE character_warehouse_items(LIKE character_items);CREATE TABLE character_stall_items(LIKE character_items);
 CREATE TABLE character_mail_attachments(LIKE character_items);CREATE TABLE game_family_stash_items(LIKE character_items);
 CREATE TABLE character_equips(LIKE character_items);CREATE TABLE character_change_set_items(LIKE character_items);`);
 for(const x of rules)await db.query(`INSERT INTO ${x.table}(id,name) VALUES($1,$2)`,[x.id,x.name]);
 await db.exec(`INSERT INTO item_instances VALUES(1,1900028001,2,true,true,'bag'),(2,1900028302,1,false,true,'bag'),(3,24640,4,true,true,'spirit-fusion'),(4,1900028002,2,true,true,'socket'),(5,900,1,true,true,'equipment'),(99,1900028001,2,true,true,'retired');
 INSERT INTO card_instances VALUES(1,1,ARRAY[99],NULL,true),(4,1,ARRAY[88],true,true),(99,1,ARRAY[77],false,true);
 INSERT INTO equipment_instances VALUES(2,false,true,ARRAY[0::bigint],0),(5,true,true,ARRAY[4::bigint],79);
 INSERT INTO equipment_spirit_fusions VALUES(5,3,24640);
 INSERT INTO character_items VALUES(7,101,1,1900028001,3,true,true),(7,102,2,1900028302,1,false,true),(7,103,0,4001,889,true,true),(7,104,0,1900028201,20,false,true),(7,105,0,3334,5,true,false);
 INSERT INTO character_equips VALUES(7,1,5,900,1,true,true);
 INSERT INTO character_warehouse_items VALUES(8,1,0,4004,2,true,true);
 INSERT INTO character_stall_items VALUES(8,1,0,4007,2,true,true);
 INSERT INTO character_mail_attachments VALUES(8,1,0,25027,2,true,true);
 INSERT INTO game_family_stash_items VALUES(8,1,0,25028,2,true,true);`);
 return db;
}
let db=await fixture();await db.exec(upgrade);await db.exec(upgrade);
assert.deepEqual((await db.query('SELECT uid,bound,locked FROM item_instances ORDER BY uid')).rows.map(x=>[Number(x.uid),x.bound,x.locked]),[[1,false,true],[2,true,true],[3,false,true],[4,false,true],[5,true,true],[99,true,true]]);
assert.deepEqual((await db.query('SELECT uid,snapshot_bound,snapshot_locked FROM card_instances ORDER BY uid')).rows.map(x=>[Number(x.uid),x.snapshot_bound,x.snapshot_locked]),[[1,false,true],[4,false,true],[99,false,true]]);
assert.deepEqual((await db.query('SELECT count,bound,locked FROM character_items ORDER BY slot')).rows.map(x=>[x.count,x.bound,x.locked]),[[3,false,true],[1,true,true],[889,false,true],[20,true,true],[5,true,false]]);
assert.equal((await db.query('SELECT durability FROM equipment_instances WHERE uid=5')).rows[0].durability,79);
for(const table of ['character_warehouse_items','character_stall_items','character_mail_attachments','game_family_stash_items'])assert.equal((await db.query(`SELECT bound FROM ${table}`)).rows[0].bound,false);
await db.close();
for(const [label,bad] of [
 ['wrong name',"UPDATE game_items SET name='collision' WHERE id=4001"],
 ['wrong table',"INSERT INTO game_items VALUES(24640,'鞋灵')"],
 ['missing template','DELETE FROM game_items WHERE id=4001'],
 ['actual equipped target','INSERT INTO character_equips VALUES(8,1,1,1900028001,1,true,false)'],
 ['uid identity','UPDATE item_instances SET item_id=1900028002 WHERE uid=1'],
 ['duplicate ownership','INSERT INTO character_warehouse_items SELECT * FROM character_items WHERE uid=1'],
 ['missing card','DELETE FROM card_instances WHERE uid=1'],
 ]){
 db=await fixture();await db.exec(bad);
 await assert.rejects(db.exec(upgrade),undefined,label);await db.exec('ROLLBACK');
 assert.equal((await db.query("SELECT count(*)::int n FROM information_schema.columns WHERE table_name='game_items' AND column_name='fixed_binding'")).rows[0].n,0,label+' rolled back schema');
 assert.equal((await db.query('SELECT bound FROM item_instances WHERE uid=1')).rows[0].bound,true,label+' rolled back item');
 await db.close();
}
console.log('fixed item binding migration: 66 authored identities, idempotence, all containers/nested instances, immutable nonbinding fields, orphan preservation and fail-closed rollback PASS');

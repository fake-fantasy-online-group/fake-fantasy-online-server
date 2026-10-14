// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/portable_cube_postgres.mjs
import {createRequire} from 'node:module';
import fs from 'node:fs';
import assert from 'node:assert/strict';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const db=new PGlite();
const read=n=>fs.readFileSync(new URL(`../migrations/000000_init-${n}.sql`,import.meta.url),'utf8');
const init=read(0), flows=read(32);
await db.exec('CREATE SCHEMA gamedata');
for(const table of ['gamedata.ov_item','gamedata.ov_desc']){
 const start=init.indexOf(`CREATE TABLE IF NOT EXISTS ${table} (`), end=init.indexOf('\n);',start)+3;
 assert(start>=0 && end>start); await db.exec(init.slice(start,end));
}
await db.exec(`CREATE TABLE game_items(id integer primary key,name text,level int,price bigint,raw bytea);
CREATE TABLE game_equipment(id integer primary key);
CREATE TABLE game_tasks(id integer primary key,name text,npc text,map text,level_min int,level_max int,descr text,reward text);
CREATE TABLE game_task_rewards(task_id integer primary key);
CREATE TABLE game_task_reward_items(task_id int,seq int,item_id int,qty int,primary key(task_id,seq));
CREATE TABLE map_npcs(map_file text,npc_id int,name text,script text);
CREATE TABLE map_teleports(map_file text,proc_id int,map_to int,poly int[]);
INSERT INTO map_npcs VALUES('qw0007',73,'巧匠左慈','N611FY'),('qw0506',1,'周天鼎','Nzhoutianding'),('qw0007',2,'原有NPC','N2');
INSERT INTO map_teleports VALUES('qw0007',5,506,ARRAY[9250,5980,9385,6060,9250,6140,9115,6060]),('qw0506',1,7,ARRAY[583,580,630,620,675,591,642,550]);
INSERT INTO gamedata.ov_desc(row_no,index,small_map) VALUES(4866,9515,9509);`);
for(const table of ['game_task_flows','game_task_flow_steps']){
 const start=flows.indexOf(`CREATE TABLE ${table} (`),end=flows.indexOf('\n);',start)+3;
 assert(start>=0 && end>start); await db.exec(flows.slice(start,end));
}
const snapshot=async()=> (await db.query(`SELECT jsonb_build_object('item',(SELECT to_jsonb(i) FROM game_items i WHERE id=1900028301),'native',(SELECT to_jsonb(i) FROM gamedata.ov_item i WHERE index=1900028301),'desc',(SELECT to_jsonb(i) FROM gamedata.ov_desc i WHERE index=1900028301),'task',(SELECT to_jsonb(q) FROM game_tasks q WHERE id=1900030001),'steps',(SELECT jsonb_agg(to_jsonb(s)) FROM game_task_flow_steps s),'npcs',(SELECT jsonb_agg(to_jsonb(n)) FROM map_npcs n),'doors',(SELECT jsonb_agg(to_jsonb(t)) FROM map_teleports t)) AS value`)).rows[0].value;
const seed=JSON.parse(read(61).split('$cube_rules$')[1]);
for(const id of new Set(seed.flatMap(r=>[...r.Materials,...r.Outputs].map(x=>x.ItemID)))) await db.query('INSERT INTO game_items(id) VALUES($1)',[id]);
await db.exec(read(61));
await db.exec(`CREATE TABLE characters(id bigint primary key,trial_daily_completed integer not null default 0,trial_cycle integer not null default 0);
CREATE TABLE character_quests(char_id bigint,task_id integer,state integer,progress integer,primary key(char_id,task_id));
CREATE TABLE game_trial_rules(quest_id integer);
CREATE TABLE endgame_runs(run_id text primary key,state text,payload jsonb);
INSERT INTO game_trial_rules VALUES(99); INSERT INTO characters(id,trial_daily_completed) VALUES(1,1),(2,0);
INSERT INTO character_quests VALUES(1,99,2,0);`);
await db.exec(read(83));
const doors=(await snapshot()).doors;
await db.exec(read(84)); await db.exec(read(85)); await db.exec(read(86)); const one=await snapshot();
assert.equal(one.item.use_action,'cube'); assert.equal(one.native.self_use,1); assert.equal(one.native.use_waste,0);
assert.equal(one.native.can_pile,0); assert.equal(one.native.can_mail,0); assert.equal(one.native.can_deal,0);
assert.equal(one.desc.small_map,9509); assert.equal(one.task.requires_trial_completion,true); assert.equal(one.task.recovery_item,1900028301);
assert.equal(one.task.recovery_lore.split('\n').length,4); assert.equal(one.steps.length,1); assert.equal(one.steps[0].kind,'rift_complete'); assert.equal(one.steps[0].target_npc,'巧匠左慈');
assert.equal(one.npcs.some(n=>n.script==='Nzhoutianding'),false); assert.equal(one.npcs.length,2); assert.deepEqual(one.doors,doors);
for(const n of [83,84,85,86]) await db.exec(read(n));
assert.deepEqual(await snapshot(),one);
assert.deepEqual((await db.query('SELECT id FROM game_cube_recipes WHERE NOT enabled ORDER BY id')).rows.map(r=>r.id),[1007,1008,1009,1012]);
assert.equal((await db.query('SELECT count(*) AS n FROM game_cube_recipes WHERE enabled')).rows[0].n,62);
assert.deepEqual((await db.query('SELECT id,money FROM game_cube_recipes WHERE id IN (1006,1010,1011) ORDER BY id')).rows,[{id:1006,money:0},{id:1010,money:10000000},{id:1011,money:1000000}]);
assert.equal((await db.query('SELECT count(*) AS n FROM game_cube_materials WHERE recipe_id BETWEEN 12001 AND 12028')).rows[0].n,0);
assert.equal((await db.query('SELECT count FROM game_cube_outputs WHERE recipe_id=1002 AND item_id=1900028203')).rows[0].count,1);
assert.deepEqual((await db.query('SELECT id,trial_completed_once FROM characters ORDER BY id')).rows,[{id:1,trial_completed_once:true},{id:2,trial_completed_once:false}]);
await db.exec("UPDATE game_items SET name='collision' WHERE id=1900028301");
await assert.rejects(db.exec(read(84)),/identity collision/);
console.log('PASS combined migrations83–86: 62 active revised recipes/four disabled, permanent qualification, nonconsuming DB-configured tool, trial-gated single-step quest, four lore segments, legacy NPC removed, original doors untouched, replay idempotent, collision rejected.');
await db.close();

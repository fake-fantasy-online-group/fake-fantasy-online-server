// Usage: RIFT_NATIVE_DATA=/path/to/extracted/native/sql PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/rift_equipment_postgres.mjs
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import crypto from 'node:crypto';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
import fs from 'node:fs';
import assert from 'node:assert/strict';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const out=process.env.RIFT_NATIVE_DATA;
assert(out,'RIFT_NATIVE_DATA must identify the extracted original native SQL directory');
const read=n=>fs.readFileSync(`${root}/migrations/000000_init-${n}.sql`,'utf8');
const db=new PGlite();
await db.exec('CREATE SCHEMA gamedata;');
for(const table of ['gamedata.ov_arm','game_equipment','game_monsters','game_monster_drops','map_monster_spawns']) {
 const re=new RegExp('CREATE TABLE IF NOT EXISTS '+table.replaceAll('.','\\.')+' \\([\\s\\S]*?\\n\\);');
 await db.exec(read(0).match(re)[0]);
}
for(const sql of read(0).matchAll(/ALTER TABLE (?:game_equipment|game_monsters) ADD COLUMN[^;]*;/g))await db.exec(sql[0]);
const sourceHashes={};
for(const table of ['gamedata_ov_arm','game_equipment','game_monsters','game_monster_drops','map_monster_spawns']) {
 const text=fs.readFileSync(out+'/'+table+'-inserts.sql','utf8');
 sourceHashes[table]=crypto.createHash('sha256').update(text).digest('hex');
 await db.exec(text);
}
console.log('Native source SHA256',JSON.stringify(sourceHashes));
console.log('Migration82 SHA256',crypto.createHash('sha256').update(read(82)).digest('hex'));
const statSQL=[...read(32).matchAll(/UPDATE game_monsters AS target SET level=source\.level[\s\S]*?WHERE target\.id=source\.id;/g)].map(x=>x[0]).join('\n');
fs.writeFileSync(out+'/game_monsters-stats.sql',statSQL);await db.exec(statSQL);
console.log('stats', (await db.query('SELECT count(*) FROM game_monsters WHERE level IS NOT NULL')).rows);
await db.exec('CREATE INDEX idx_spawns_monster ON map_monster_spawns(monster);CREATE INDEX idx_monsters_level ON game_monsters(level);CREATE INDEX idx_monsters_kind ON game_monsters(kind);CREATE INDEX idx_mondrop_item ON game_monster_drops(item_id);ANALYZE;');
const migration=read(82);const query= migration.slice(migration.indexOf(' WITH sources AS'),migration.indexOf(' IF pools IS NULL')).replace(' INTO pools','');
const result=(await db.query(query)).rows[0].jsonb_agg;
fs.writeFileSync(out+'/verified-pools.json',JSON.stringify(result,null,2));
console.log('POOL SUMMARY',JSON.stringify(result.map(x=>({level:x.MonsterLevel,sources:x.SourceMonsters.length,items:x.Items.length}))));
const ids=[...new Set(result.flatMap(x=>x.Items))];console.log('DistinctPoolItems',ids.length);
const itemAudit=(await db.query(`SELECT a.index as id,a.name,e.level_req,a.level_need,a.level,a.position_,a.query_type,a.no_type_drop,a.newbie,a.warrior,a.swordman,a.stabber,a.druggist,a.magician,a.sr_warrior,a.sr_swordman,a.sr_stabber,a.sr_druggist,a.sr_magician FROM gamedata.ov_arm a JOIN game_equipment e ON e.id=a.index WHERE a.index=ANY($1) ORDER BY a.index`,[ids])).rows;
fs.writeFileSync(out+'/verified-items.json',JSON.stringify(itemAudit,null,2));
const sourceAudit=(await db.query('SELECT id,name,level,kind,elite_id FROM game_monsters WHERE id=ANY($1) ORDER BY level,id',[[...new Set(result.flatMap(x=>x.SourceMonsters))]])).rows;
fs.writeFileSync(out+'/verified-sources.json',JSON.stringify(sourceAudit,null,2));
// Real historical rules migrations, with their documented item prerequisites.
const items=JSON.parse(read(59).split('$endgame_items$')[1]);
await db.exec('CREATE TABLE game_items(id integer primary key);CREATE TABLE gamedata.ov_item(index integer primary key, level_need integer);CREATE TABLE game_endgame_rune_rules(item_id integer primary key);');
for(const i of items)await db.query('INSERT INTO game_items VALUES($1)',[i.id]);
for(const i of items.filter(i=>i.id>=1900028001&&i.id<=1900028028)){await db.query('INSERT INTO gamedata.ov_item VALUES($1,$2)',[i.id,i.level]);await db.query('INSERT INTO game_endgame_rune_rules VALUES($1)',[i.id]);}
for(const n of [63,76,77,78,79,80])await db.exec(read(n));
await db.exec('UPDATE game_rift_rule_active SET version=6 WHERE id=1');
const snapshot=async()=> (await db.query("SELECT jsonb_build_object('versions',(SELECT jsonb_agg(to_jsonb(r) ORDER BY version) FROM game_rift_rule_versions r),'classes',(SELECT jsonb_agg(to_jsonb(c) ORDER BY version,class) FROM game_rift_class_rules c),'abilities',(SELECT jsonb_agg(to_jsonb(a) ORDER BY version,id) FROM game_rift_ability_rules a),'active',(SELECT version FROM game_rift_rule_active WHERE id=1)) AS data")).rows[0].data;
const before=await snapshot();await db.exec(migration);const after=await snapshot();
assert.equal(after.active,7);assert.equal(after.versions.length,7);assert.deepEqual(after.versions[6].equipment_pools,result);
assert.deepEqual(after.versions.slice(0,6).map(({equipment_pools,...r})=>r),before.versions);
assert.deepEqual(after.classes.filter(x=>x.version<=6),before.classes);assert.deepEqual(after.abilities.filter(x=>x.version<=6),before.abilities);
await db.exec(migration);assert.deepEqual(await snapshot(),after);
const queries=[...fs.readFileSync(root+'/internal/data/endgame_balance_rules.go','utf8').matchAll(/q.Query\(ctx, `([^`]+)`\)/g)].map(x=>x[1]);const loader={};for(const q of queries)loader[q]=(await db.query(q)).rows.map(r=>Object.values(r));fs.writeFileSync(out+'/loader-results.json',JSON.stringify(loader,null,2));console.log('PASS '+queries.length+' verbatim current production loader SELECTs');
console.log('PASS migration82: native-source query, active7, immutable prior1-6, exact child copy, replay stable.');
fs.writeFileSync(out+'/rules-snapshot.json',JSON.stringify(after,null,2));
for(const sql of [
 "UPDATE game_rift_rule_versions SET equipment_pools='[]'::jsonb WHERE version=7",
 "DELETE FROM game_rift_class_rules WHERE version=7",
 "DELETE FROM game_rift_ability_rules WHERE version=7",
 "UPDATE game_rift_rule_versions SET name='changed' WHERE version=6"
])await assert.rejects(()=>db.exec(sql),/immutable/);
assert.deepEqual(await snapshot(),after);
await db.exec('BEGIN; UPDATE game_rift_rule_active SET version=5 WHERE id=1;');
await assert.rejects(()=>db.exec(migration),/requires active rift6/);await db.exec('ROLLBACK;');
await db.exec('BEGIN; UPDATE game_rift_rule_active SET version=6 WHERE id=1; DELETE FROM game_monster_drops;');
await assert.rejects(()=>db.exec(migration),/no native preascension/);await db.exec('ROLLBACK;');
await db.exec("BEGIN; INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,to_jsonb(r)||'{\"version\":8,\"published\":false}'::jsonb)).* FROM game_rift_rule_versions r WHERE version=7; UPDATE game_rift_rule_versions SET published=true WHERE version=8; UPDATE game_rift_rule_active SET version=8 WHERE id=1;");
await db.exec(migration);assert.equal((await snapshot()).active,8);await db.exec('ROLLBACK;');
assert.deepEqual(await snapshot(),after);
console.log('PASS published mutation guards, inactive5 rejection, empty native pool atomic failure, later selector never downgraded');
fs.writeFileSync(out+'/verification-report.json',JSON.stringify({engine:(await db.query('SELECT version()')).rows[0].version,sourceHashes,migrationSHA256:crypto.createHash('sha256').update(migration).digest('hex'),bands:result.length,items:ids.length,checks:['activation7','history1–6 unchanged','pool source equality','idempotent replay','production loader queries','immutable guards','inactive5 rejection','empty pool failure','monotonic later selector']},null,2));
export {db,root,out,result,ids,itemAudit};
if (!process.env.POOL_KEEP_DB) await db.close();

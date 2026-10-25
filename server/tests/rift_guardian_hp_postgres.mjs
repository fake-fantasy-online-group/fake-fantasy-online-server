// Run: PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/rift_guardian_hp_postgres.mjs
// Actual PostgreSQL engine and verbatim migration/loader SQL. Reduced setup:
// production migrations63/76/77/78/79/80 build frozen rules and guards, a fixture
// reconstructs Rift11 native pools, then real106/107 produce Loot3/Rift12.
// This is not a full initializer, live database, or concurrent migration test.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
const require = createRequire(import.meta.url);
const {PGlite} = require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = n => fs.readFileSync(`${root}/migrations/000000_init-${n}.sql`, 'utf8');
const body = read(109);
const upgrade = fs.readFileSync(`${root}/migrations/upgrades/000109_20261024_rift_guardian_hp.sql`, 'utf8');
assert.equal(upgrade, '-- Existing databases: apply atomically; do not rerun fresh init.\nBEGIN;\n' + body + 'COMMIT;\n');
const fixture = JSON.parse(fs.readFileSync(`${root}/internal/game/scene/testdata/rift_native_equipment_pools.json`, 'utf8'));
const excluded = new Set(JSON.parse(read(105).split('$excluded$')[1]).map(x => x.id));
const loader = fs.readFileSync(`${root}/internal/data/endgame_balance_rules.go`, 'utf8').split('func LoadRiftRules(')[1].match(/q\.Query\(ctx, `([^`]+)`\)/)[1];
const children = ['game_rift_class_rules', 'game_rift_ability_rules'];
const tables = ['game_rift_rule_versions','game_rift_rule_active',...children,'game_loot_rule_versions','game_loot_rule_active','game_loot_rune_pool','game_loot_socket_levels','game_loot_grades','focused_saved_state'];
const json = v => "'" + JSON.stringify(v).replaceAll("'", "''") + "'::jsonb";
const clone = (from,to,extra={}) => `INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,to_jsonb(r)||${json({version:to,published:false,...extra})})).* FROM game_rift_rule_versions r WHERE version=${from};` + children.map(t => `INSERT INTO ${t} SELECT (jsonb_populate_record(NULL::${t},to_jsonb(r)||'{"version":${to}}'::jsonb)).* FROM ${t} r WHERE version=${from};`).join('') + `UPDATE game_rift_rule_versions SET published=true WHERE version=${to};`;
const corrupt = (table,sql) => `ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules;${sql};ALTER TABLE ${table} ENABLE TRIGGER immutable_published_rules;`;
const db = new PGlite();
const pass = message => console.log('PASS ' + message);
const canonical = rows => rows.map(({version,...r})=>r).sort((a,b)=>JSON.stringify(a).localeCompare(JSON.stringify(b)));
async function snapshot() {
 const result={};
 for (const t of tables) result[t]=(await db.query(`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb) data FROM ${t} r`)).rows[0].data;
 result.column=(await db.query("SELECT data_type,is_nullable,column_default FROM information_schema.columns WHERE table_name='game_rift_rule_versions' AND column_name='guardian_hp_divisor'")).rows;
 return result;
}
const projection = async () => (await db.query(loader)).rows.map(r=>JSON.parse(Object.values(r)[0]));
try {
 console.log((await db.query('SELECT version()')).rows[0].version);
 await db.exec('CREATE TABLE game_items(id INTEGER PRIMARY KEY); CREATE SCHEMA gamedata; CREATE TABLE gamedata.ov_item(index INTEGER PRIMARY KEY,level_need INTEGER); CREATE TABLE game_endgame_rune_rules(item_id INTEGER PRIMARY KEY);');
 const items=JSON.parse(read(59).split('$endgame_items$')[1]);
 for(const item of items) await db.query('INSERT INTO game_items VALUES($1)',[item.id]);
 for(const item of items.filter(i=>i.id>=1900028001 && i.id<=1900028028)) {
  await db.query('INSERT INTO gamedata.ov_item VALUES($1,$2)',[item.id,item.level]);
  await db.query('INSERT INTO game_endgame_rune_rules VALUES($1)',[item.id]);
 }
 for(const n of [63,76,77,78,79,80]) await db.exec(read(n));
 await db.exec('ALTER TABLE game_rift_rule_versions ADD COLUMN equipment_pools JSONB, ADD COLUMN equipment_slot_fallbacks JSONB, ADD COLUMN floor_layout_version INTEGER, ADD COLUMN guardian_recent_window INTEGER;');
 await db.exec(clone(6,11,{equipment_pools:fixture.Pools.map(p=>({...p,Items:p.Items.filter(id=>!excluded.has(id))})),equipment_slot_fallbacks:fixture.SlotFallbacks,floor_layout_version:6,guardian_recent_window:3}));
 await db.exec('UPDATE game_rift_rule_active SET version=11 WHERE id=1;' + read(106) + read(107));
 await db.exec(`CREATE TABLE focused_saved_state(id INTEGER PRIMARY KEY, payload JSONB); INSERT INTO focused_saved_state VALUES(1,'{"paidRun":{"RulesVersion":12},"readyGrant":[123],"playerHP":456}');`);
 const before=await snapshot(), prior=await projection();
 assert(before.game_rift_rule_versions.some(r=>r.version===12));
 assert(prior.every(r=>r.GuardianHPDivisor===0));
 pass('production loader SQL accepts missing optional column and preserves historical zero policy');
 const verify = (actual,input=before,active=13) => {
  const source=input.game_rift_rule_versions.find(r=>r.version===12);
  assert.deepEqual(actual.game_rift_rule_versions.find(r=>r.version===13),{...source,version:13,guardian_hp_divisor:10});
  assert.deepEqual(actual.game_rift_rule_versions.filter(r=>r.version!==13),input.game_rift_rule_versions.map(r=>({guardian_hp_divisor:null,...r})));
  assert.equal(actual.game_rift_rule_active[0].version,active);
  for(const table of children) {
   assert.deepEqual(actual[table].filter(r=>r.version!==13),input[table]);
   assert.deepEqual(canonical(actual[table].filter(r=>r.version===13)),canonical(actual[table].filter(r=>r.version===12)));
  }
  for(const table of tables.filter(t=>!t.startsWith('game_rift_'))) assert.deepEqual(actual[table],input[table],table);
  assert.deepEqual(actual.column,[{data_type:'integer',is_nullable:'YES',column_default:null}]);
 };
 async function reject(name,mutation,pattern) {
  await db.exec('BEGIN;'+mutation+';SAVEPOINT input;');
  const input=await snapshot();
  await assert.rejects(()=>db.exec(body),pattern);
  await db.exec('ROLLBACK TO SAVEPOINT input');
  assert.deepEqual(await snapshot(),input,name);
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(),before,name);
  pass(name+' rejected atomically');
 }
 await reject('missing selector','DELETE FROM game_rift_rule_active',/active selector/);
 for(const [field,value] of Object.entries({published:'false',loot_version:'2',cumulative_legendary_per_tier:'20',normal_boss_equipment:'9',equipment:'1',timely_equipment:'1',unique_base_chance:'1',unique_per_tier:'1',unique_cap:'1'})) await reject('source '+field,corrupt('game_rift_rule_versions',`UPDATE game_rift_rule_versions SET ${field}=${value} WHERE version=12`),/unchanged published rift12/);
 for(const table of children) await reject('incomplete '+table,corrupt(table,`DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=12 LIMIT 1)`),/complete rift12 dependencies/);
 for(const table of ['game_rift_rule_versions',...children]) await reject('disabled guard '+table,`ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules`,/enabled immutable rule guards/);
 for(const definition of ['TEXT','INTEGER DEFAULT 10','INTEGER NOT NULL DEFAULT 10']) await reject('conflicting column '+definition,'ALTER TABLE game_rift_rule_versions ADD COLUMN guardian_hp_divisor '+definition,/nullable INTEGER column/);
 await reject('conflicting Rift13',clone(12,13,{equipment:1}),/occupied by different configuration/);
 await reject('conflicting Rift13 child',body+corrupt('game_rift_class_rules','UPDATE game_rift_class_rules SET hp_bp=hp_bp+1 WHERE version=13 AND class=3'),/preserve class and ability dependencies/);
 await db.exec('BEGIN;'+body);verify(await snapshot());await db.exec('ROLLBACK');assert.deepEqual(await snapshot(),before);pass('fresh-body caller rollback restores rows, children, selector and schema');
 for(const active of [1,11,14]) {
  await db.exec('BEGIN;'+(active===14?clone(12,14):'')+`UPDATE game_rift_rule_active SET version=${active};`);
  const input=await snapshot();await db.exec(body);verify(await snapshot(),input,active);await db.exec(body);verify(await snapshot(),input,active);await db.exec('ROLLBACK');
 }
 pass('older selections and a future rule are never overridden, including replay');
 await db.exec(upgrade);const after=await snapshot();verify(after);
 assert.deepEqual(await projection(),[...prior,{...prior.at(-1),Version:13,GuardianHPDivisor:10}]);
 pass('upgrade publishes exact Rift12 clone with only divisor10 and version13; loader projects exact old/new rules');
 await db.exec(body);await db.exec(upgrade);assert.deepEqual(await snapshot(),after);pass('fresh body and upgrade are idempotent');
 for(const sql of ['UPDATE game_rift_rule_versions SET guardian_hp_divisor=10 WHERE version=12','UPDATE game_rift_rule_versions SET guardian_hp_divisor=1 WHERE version=13','DELETE FROM game_rift_rule_versions WHERE version=13','UPDATE game_rift_class_rules SET hp_bp=1 WHERE version=13','DELETE FROM game_rift_ability_rules WHERE version=13']) await assert.rejects(()=>db.exec(sql),/immutable/);
 assert.deepEqual(await snapshot(),after);pass('historical/new published rules and children remain immutable');
 console.log('All focused migration109 PostgreSQL checks passed. Reduced prerequisite fixture; no full initializer, live database, or concurrency test.');
} finally {await db.close();}

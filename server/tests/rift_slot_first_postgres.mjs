// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/rift_slot_first_postgres.mjs
// Real PostgreSQL engine, production migrations/loader with reduced prerequisite fixtures.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
const require = createRequire(import.meta.url);
const {PGlite} = require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = n => fs.readFileSync(`${root}/migrations/000000_init-${n}.sql`, 'utf8');
const body = read(111);
const upgrade = fs.readFileSync(`${root}/migrations/upgrades/000111_20261025_rift_slot_first.sql`, 'utf8');
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
 await db.exec(`CREATE TABLE focused_saved_state(id INTEGER PRIMARY KEY, payload JSONB); INSERT INTO focused_saved_state VALUES(1,'{"paidRun":{"RulesVersion":14},"readyGrant":[123],"legacyRun":{"RulesVersion":12},"playerHP":456}');`);
 await db.exec(read(109));
 await db.exec('ALTER TABLE game_rift_rule_versions ADD COLUMN wing_rewards JSONB');
 const rewards=JSON.parse(fs.readFileSync(`${root}/config/rift-wing-rewards-v14.json`, 'utf8'));
 await db.exec(clone(13,14,{wing_rewards:rewards})+'UPDATE game_rift_rule_active SET version=14');
 const before=await snapshot(), prior=await projection();
 const verify = (actual,input=before,active=15) => {
  const source=input.game_rift_rule_versions.find(r=>r.version===14);
  assert.deepEqual(actual.game_rift_rule_versions.find(r=>r.version===15),{...source,version:15});
  assert.deepEqual(actual.game_rift_rule_versions.filter(r=>r.version!==15),input.game_rift_rule_versions);
  assert.equal(actual.game_rift_rule_active[0].version,active);
  for(const table of children) {
   assert.deepEqual(actual[table].filter(r=>r.version!==15),input[table]);
   assert.deepEqual(canonical(actual[table].filter(r=>r.version===15)),canonical(actual[table].filter(r=>r.version===14)));
  }
  for(const table of tables.filter(t=>!t.startsWith('game_rift_'))) assert.deepEqual(actual[table],input[table],table);
  assert.deepEqual(actual.column,input.column);
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
 for(const [field,value] of Object.entries({published:'false',loot_version:'2',cumulative_legendary_per_tier:'20',normal_boss_equipment:'9',equipment:'1',timely_equipment:'1',unique_base_chance:'1',unique_per_tier:'1',unique_cap:'1',guardian_hp_divisor:'1',wing_rewards:'NULL'})) await reject('source '+field,corrupt('game_rift_rule_versions',`UPDATE game_rift_rule_versions SET ${field}=${value} WHERE version=14`),/unchanged published rift14/);
 await reject('rare feather drift',corrupt('game_rift_rule_versions',`UPDATE game_rift_rule_versions SET wing_rewards=jsonb_set(wing_rewards,'{RareFeather,ChanceBP}','100') WHERE version=14`),/unchanged published rift14/);
 for(const table of children) await reject('incomplete '+table,corrupt(table,`DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=14 LIMIT 1)`),/complete rift14 dependencies/);
 for(const table of ['game_rift_rule_versions',...children]) await reject('disabled guard '+table,`ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules`,/enabled immutable rule guards/);
 await reject('conflicting Rift15',clone(14,15,{equipment:1}),/occupied by different configuration/);
 for(const table of children) await reject('conflicting Rift15 '+table,body+corrupt(table,`DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=15 LIMIT 1)`),/preserve class and ability dependencies/);
 await db.exec('BEGIN;'+body);verify(await snapshot());await db.exec('ROLLBACK');assert.deepEqual(await snapshot(),before);pass('fresh-body rollback restores rules, dependencies, selector and saved grants');
 for(const active of [1,13,16]) {
  await db.exec('BEGIN;'+(active===16?clone(14,16):'')+`UPDATE game_rift_rule_active SET version=${active};`);
  const input=await snapshot();await db.exec(body);verify(await snapshot(),input,active);await db.exec(body);verify(await snapshot(),input,active);await db.exec('ROLLBACK');
 }
 pass('older rollback selections and future rules are never overridden, including replay');
 await db.exec(upgrade);const after=await snapshot();verify(after);
 assert.deepEqual(await projection(),[...prior,{...prior.at(-1),Version:15}]);
 pass('upgrade and production loader retain every Rift14 field including GuardianHPDivisor10 and complete WingRewards');
 await db.exec(body);await db.exec(upgrade);assert.deepEqual(await snapshot(),after);pass('fresh body and upgrade are idempotent');
 for(const sql of ['UPDATE game_rift_rule_versions SET guardian_hp_divisor=1 WHERE version=14','UPDATE game_rift_rule_versions SET guardian_hp_divisor=1 WHERE version=15','DELETE FROM game_rift_rule_versions WHERE version=15','UPDATE game_rift_class_rules SET hp_bp=1 WHERE version=15','DELETE FROM game_rift_ability_rules WHERE version=15']) await assert.rejects(()=>db.exec(sql),/immutable/);
 assert.deepEqual(await snapshot(),after);pass('historical/new published rules and children remain immutable');
 console.log('All focused migration111 PostgreSQL checks passed. Reduced prerequisite fixture; no full initializer, live database, or concurrency test.');
} finally {await db.close();}

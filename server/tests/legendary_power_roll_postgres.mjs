// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/legendary_power_roll_postgres.mjs
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
const body = read(115);
const upgrade = fs.readFileSync(`${root}/migrations/upgrades/000115_20261027_legendary_power_rolls.sql`, 'utf8');
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
 result.column=(await db.query("SELECT data_type,is_nullable,column_default FROM information_schema.columns WHERE table_name='game_loot_rule_versions' AND column_name='legendary_power_rolls'")).rows;
 return result;
}
const lootLoader = fs.readFileSync(`${root}/internal/data/endgame_balance_rules.go`, 'utf8').split('func LoadLootRules(')[1].match(/q\.Query\(ctx, `([^`]+)`\)/)[1];
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
 await db.exec(read(111));
 await db.exec(read(112));
 const before=await snapshot();
 const verify = (actual,input=before,lootActive=5,riftActive=17) => {
  const oldLoot=input.game_loot_rule_versions.map(r=>({...r,legendary_power_rolls:false}));
  assert.deepEqual(actual.game_loot_rule_versions.filter(r=>r.version!==5),oldLoot);
  assert.deepEqual(actual.game_loot_rule_versions.find(r=>r.version===5),{...oldLoot.find(r=>r.version===4),version:5,legendary_power_rolls:true});
  assert.deepEqual(actual.game_rift_rule_versions.filter(r=>r.version!==17),input.game_rift_rule_versions);
  assert.deepEqual(actual.game_rift_rule_versions.find(r=>r.version===17),{...input.game_rift_rule_versions.find(r=>r.version===16),version:17,loot_version:5});
  assert.equal(actual.game_loot_rule_active[0].version,lootActive);
  assert.equal(actual.game_rift_rule_active[0].version,riftActive);
  for(const table of ['game_loot_rune_pool','game_loot_socket_levels','game_loot_grades',...children]) {
   const from=table.startsWith('game_loot_')?4:16,to=from===4?5:17;
   assert.deepEqual(actual[table].filter(r=>r.version!==to),input[table]);
   assert.deepEqual(canonical(actual[table].filter(r=>r.version===to)),canonical(actual[table].filter(r=>r.version===from)));
  }
  assert.deepEqual(actual.focused_saved_state,input.focused_saved_state);
  assert.deepEqual(actual.column,[{data_type:'boolean',is_nullable:'NO',column_default:'false'}]);
 };
 async function reject(name,mutation,pattern) {
  await db.exec('BEGIN;'+mutation+';SAVEPOINT input;');
  const input=await snapshot();
  await assert.rejects(()=>db.exec(body),pattern);
  await db.exec('ROLLBACK TO SAVEPOINT input');
  assert.deepEqual(await snapshot(),input,name);
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(),before,name);pass(name+' rejected atomically');
 }
 await reject('missing loot selector','DELETE FROM game_loot_rule_active',/active loot and rift selectors/);
 await reject('missing rift selector','DELETE FROM game_rift_rule_active',/active loot and rift selectors/);
 await reject('conflicting column','ALTER TABLE game_loot_rule_versions ADD COLUMN legendary_power_rolls TEXT',/boolean power roll column/);
 for(const table of ['game_loot_rule_versions','game_loot_rune_pool','game_loot_socket_levels','game_loot_grades','game_rift_rule_versions',...children]) await reject('disabled guard '+table,`ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules`,/enabled immutable rule guards/);
 await reject('loot source drift',corrupt('game_loot_rule_versions','UPDATE game_loot_rule_versions SET unique_base_ppb=1 WHERE version=4'),/unchanged published loot4/);
 await reject('rift source drift',corrupt('game_rift_rule_versions','UPDATE game_rift_rule_versions SET normal_boss_equipment=9 WHERE version=16'),/unchanged published rift16/);
 for(const table of ['game_loot_rune_pool','game_loot_socket_levels','game_loot_grades',...children]) {
  const version=table.startsWith('game_loot_')?4:16;
  await reject('missing source '+table,corrupt(table,`DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=${version} LIMIT 1)`),/complete source dependencies/);
 }
 await db.exec('BEGIN;'+body);verify(await snapshot());await db.exec('ROLLBACK');assert.deepEqual(await snapshot(),before);pass('rollback restores schema, rules, selectors and saved grants');
 await db.exec('BEGIN;UPDATE game_loot_rule_active SET version=1;UPDATE game_rift_rule_active SET version=15;');
 const older=await snapshot();await db.exec(body);verify(await snapshot(),older,1,15);await db.exec(body);verify(await snapshot(),older,1,15);await db.exec('ROLLBACK');pass('older selections preserved on replay');
 await db.exec(upgrade);const after=await snapshot();verify(after);
 const projected=(await db.query(lootLoader)).rows.map(r=>JSON.parse(Object.values(r)[0]));
 assert.deepEqual(projected.map(r=>[r.Version,r.LegendaryPowerPolicy,r.LegendaryPowerRolls]),[[1,0,false],[2,0,false],[3,0,false],[4,1,false],[5,1,true]]);
 assert.deepEqual(projected[4],{...projected[3],Version:5,LegendaryPowerRolls:true});
 pass('production loader preserves old flags and enables numerical rolls only on loot5');
 await db.exec(body);await db.exec(upgrade);assert.deepEqual(await snapshot(),after);pass('fresh and upgrade idempotent');
 for(const table of ['game_loot_rune_pool','game_loot_socket_levels','game_loot_grades',...children]) {
  const version=table.startsWith('game_loot_')?5:17;
  await db.exec('BEGIN;'+corrupt(table,`DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=${version} LIMIT 1)`));
  await assert.rejects(()=>db.exec(body),/preserve source dependencies/);await db.exec('ROLLBACK');
 }
 for(const [table,version] of [['game_loot_rule_versions',5],['game_rift_rule_versions',17]]) {
  await db.exec('BEGIN;'+corrupt(table,`UPDATE ${table} SET published=false WHERE version=${version}`));
  await assert.rejects(()=>db.exec(body),/occupied by different configuration/);await db.exec('ROLLBACK');
 }
 await db.exec('BEGIN;');
 await db.exec("INSERT INTO game_loot_rule_versions SELECT (jsonb_populate_record(NULL::game_loot_rule_versions,to_jsonb(r)||'{\"version\":6,\"published\":false}'::jsonb)).* FROM game_loot_rule_versions r WHERE version=5;UPDATE game_loot_rule_versions SET published=true WHERE version=6;");
 await db.exec(clone(17,18)+'UPDATE game_loot_rule_active SET version=6;UPDATE game_rift_rule_active SET version=18;');
 const future=await snapshot();await db.exec(body);assert.deepEqual(await snapshot(),future);await db.exec('ROLLBACK');pass('newer selectors never downgraded');
 for(const sql of ['UPDATE game_loot_rule_versions SET legendary_power_rolls=true WHERE version=4','UPDATE game_loot_rule_versions SET legendary_power_rolls=false WHERE version=5','DELETE FROM game_rift_rule_versions WHERE version=17','UPDATE game_rift_class_rules SET hp_bp=1 WHERE version=17','DELETE FROM game_loot_grades WHERE version=5']) await assert.rejects(()=>db.exec(sql),/immutable/);
 assert.deepEqual(await snapshot(),after);pass('old/new published guards intact and saved state untouched');
 console.log('Focused migration115 PostgreSQL checks passed. Reduced prerequisites; no live database or concurrency test.');
} finally {await db.close();}

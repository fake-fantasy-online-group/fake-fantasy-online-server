// Run: PGLITE_MODULE=/absolute/path/to/@electric-sql/pglite node server/tests/outdoor_equipment_postgres.mjs
// Actual PostgreSQL engine and verbatim SQL. Reduced dependencies, not a full
// initializer, deployed database, native equipment fixture, or concurrency test.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';

const require = createRequire(import.meta.url);
const {PGlite} = require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = n => fs.readFileSync(`${root}/migrations/000000_init-${n}.sql`, 'utf8');
const body = read(106);
const upgrade = fs.readFileSync(`${root}/migrations/upgrades/000106_20261024_outdoor_equipment_colours.sql`, 'utf8');
assert.equal(upgrade, '-- Existing databases: apply atomically; do not rerun fresh init.\nBEGIN;\n' + body + 'COMMIT;\n');
const expected = {AffixWeights: [20, 25, 20, 15, 10, 5], LegendaryBasePPB: 50000000};
const expectedNormal = {AffixWeights: [46,20,15,10,5,2], LegendaryBasePPB: 20000000};
const loaderSource = fs.readFileSync(`${root}/internal/data/endgame_balance_rules.go`, 'utf8');
const lootLoader = loaderSource.slice(loaderSource.indexOf('func LoadLootRules('), loaderSource.indexOf('func LoadRiftRules('));
const loaderQueries = [...lootLoader.matchAll(/q\.Query\(ctx, `([^`]+)`\)/g)];
assert.equal(loaderQueries.length, 1, 'expected exactly one verbatim published-loot projection');
const loaderSQL = loaderQueries[0][1];
const children = ['game_loot_rune_pool', 'game_loot_socket_levels', 'game_loot_grades'];
const tables = ['game_loot_rule_versions', 'game_loot_rule_active', ...children,
  'game_rift_rule_versions', 'game_rift_rule_active', 'game_rift_class_rules', 'game_rift_ability_rules',
  'game_items', 'game_endgame_rune_rules', 'gamedata.ov_item', 'focused_player_state'];
const db = new PGlite();
const pass = message => console.log('PASS ' + message);
console.log((await db.query('SELECT version()')).rows[0].version);
try {
  // Independently reconstruct the historical published rules from migrations,
  // including all weighted rune IDs/rates/sockets, rather than mocking loot2.
  await db.exec(`CREATE TABLE game_items(id INTEGER PRIMARY KEY);
    CREATE SCHEMA gamedata;
    CREATE TABLE gamedata.ov_item(index INTEGER PRIMARY KEY,level_need INTEGER);
    CREATE TABLE game_endgame_rune_rules(item_id INTEGER PRIMARY KEY);`);
  const items = JSON.parse(read(59).split('$endgame_items$')[1]);
  for (const item of items) await db.query('INSERT INTO game_items VALUES($1)', [item.id]);
  for (const item of items.filter(i => i.id >= 1900028001 && i.id <= 1900028028)) {
    await db.query('INSERT INTO gamedata.ov_item VALUES($1,$2)', [item.id, item.level]);
    await db.query('INSERT INTO game_endgame_rune_rules VALUES($1)', [item.id]);
  }
  for (const n of [63, 76, 77, 78, 79, 80]) await db.exec(read(n));
  // Unrelated persisted player/paid-run/grant sentinels. Their reduced schema is
  // deliberately not presented as production player-data coverage.
  await db.exec(`CREATE TABLE focused_player_state(id INTEGER PRIMARY KEY,items JSONB,paid_run JSONB,ready_grant JSONB);
    INSERT INTO focused_player_state VALUES(1,'[{"uid":123,"quality":6}]',
      '{"LootRules":{"Version":2,"UniqueBasePPB":10000000},"RiftRules":{"Version":5}}',
      '{"items":[{"uid":456,"quality":2}],"state":"ready"}');`);

  async function snapshot() {
    const state = {};
    for (const table of tables) state[table] = (await db.query(`SELECT
      COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb) data FROM ${table} r`)).rows[0].data;
    state.column = (await db.query(`SELECT column_name,data_type,is_nullable,column_default FROM information_schema.columns
      WHERE table_schema='public' AND table_name='game_loot_rule_versions' AND column_name IN ('outdoor_equipment','outdoor_normal_equipment') ORDER BY column_name`)).rows;
    return state;
  }
  const active = async () => (await db.query('SELECT version FROM game_loot_rule_active WHERE id=1')).rows[0]?.version;
  const clone = (source, target, extra = {}) => {
    let sql = `INSERT INTO game_loot_rule_versions SELECT (jsonb_populate_record(NULL::game_loot_rule_versions,
      to_jsonb(r)||'${JSON.stringify({version: target, published: false, ...extra})}'::jsonb)).*
      FROM game_loot_rule_versions r WHERE version=${source};`;
    for (const table of children) sql += `INSERT INTO ${table} SELECT
      (jsonb_populate_record(NULL::${table},to_jsonb(r)||'{"version":${target}}'::jsonb)).*
      FROM ${table} r WHERE version=${source};`;
    return sql;
  };
  const corrupt = (table, sql) => `ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules;
    ${sql}; ALTER TABLE ${table} ENABLE TRIGGER immutable_published_rules;`;
  const before = await snapshot();
  assert.equal(await active(), 2);
  const loadProjection = async () => (await db.query(loaderSQL)).rows.map(row => JSON.parse(Object.values(row)[0]));
  const beforeProjection = await loadProjection();
  assert.deepEqual(beforeProjection.map(r => [r.Version, r.OutdoorEquipment, r.OutdoorNormalEquipment]), [[1, null, null], [2, null, null]]);
  pass('verbatim current LoadLootRules SELECT accepts absent column and projects historical OutdoorEquipment null');

  async function rejectsWithoutChanges(name, mutation, error) {
    await db.exec('BEGIN;' + mutation + ';SAVEPOINT input_state;');
    const input = await snapshot();
    await assert.rejects(() => db.exec(body), error, name);
    await db.exec('ROLLBACK TO SAVEPOINT input_state');
    assert.deepEqual(await snapshot(), input, name + ' leaked migration effects');
    await db.exec('ROLLBACK');
    assert.deepEqual(await snapshot(), before, name + ' leaked test setup');
    pass(name + ' fails closed without changes');
  }

  await rejectsWithoutChanges('missing selector', 'DELETE FROM game_loot_rule_active', /active loot selector/);
  await rejectsWithoutChanges('unpublished loot2', corrupt('game_loot_rule_versions',
    'UPDATE game_loot_rule_versions SET published=false WHERE version=2'), /unchanged published loot2/);
  for (const table of children) {
    await rejectsWithoutChanges('incomplete ' + table, corrupt(table,
      `DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=2 LIMIT 1)`), /complete loot2 dependencies/);
  }
  for (const table of ['game_loot_rule_versions', ...children]) {
    await rejectsWithoutChanges('disabled guard on ' + table,
      `ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules`, /enabled immutable rule guards/);
  }
  await rejectsWithoutChanges('wrong guard operations', `DROP TRIGGER immutable_published_rules ON game_loot_grades;
    CREATE TRIGGER immutable_published_rules BEFORE UPDATE ON game_loot_grades
    FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules()`, /enabled immutable rule guards/);
  await rejectsWithoutChanges('wrong guard function', `CREATE FUNCTION focused_fake_guard() RETURNS trigger LANGUAGE plpgsql
    AS $$ BEGIN RETURN NEW; END $$; DROP TRIGGER immutable_published_rules ON game_loot_grades;
    CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_loot_grades
    FOR EACH ROW EXECUTE FUNCTION focused_fake_guard()`, /enabled immutable rule guards/);
  for (const definition of ['TEXT', "JSONB DEFAULT '{}'::jsonb", "JSONB NOT NULL DEFAULT '{}'::jsonb"]) {
    await rejectsWithoutChanges('conflicting column ' + definition,
      `ALTER TABLE game_loot_rule_versions ADD COLUMN outdoor_equipment ${definition}`, /nullable JSONB column without a default/);
  }
  await rejectsWithoutChanges('conflicting normal column', 'ALTER TABLE game_loot_rule_versions ADD COLUMN outdoor_normal_equipment TEXT', /nullable JSONB column without a default/);
  await rejectsWithoutChanges('modified historical normal rules', 'ALTER TABLE game_loot_rule_versions ADD COLUMN outdoor_normal_equipment JSONB;' + corrupt('game_loot_rule_versions', "UPDATE game_loot_rule_versions SET outdoor_normal_equipment='{}' WHERE version=2"), /unchanged published loot2/);
  await rejectsWithoutChanges('modified historical loot2', `ALTER TABLE game_loot_rule_versions ADD COLUMN outdoor_equipment JSONB;` +
    corrupt('game_loot_rule_versions', `UPDATE game_loot_rule_versions SET outdoor_equipment='{}' WHERE version=2`), /unchanged published loot2/);
  await rejectsWithoutChanges('occupied loot3', clone(2, 3, {unique_base_ppb: 999}) +
    'UPDATE game_loot_rule_versions SET published=true WHERE version=3', /loot3 is occupied/);
  await rejectsWithoutChanges('unpublished matching loot3', body + corrupt('game_loot_rule_versions',
    'UPDATE game_loot_rule_versions SET published=false WHERE version=3'), /loot3 is occupied/);
  for (const table of children) {
    await rejectsWithoutChanges('occupied loot3 missing child in ' + table, body + corrupt(table,
      `DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=3 LIMIT 1)`), /must preserve every loot2/);
  }
  await rejectsWithoutChanges('occupied loot3 changed rune weight', body + corrupt('game_loot_rune_pool',
    'UPDATE game_loot_rune_pool SET weight=weight+1 WHERE version=3 AND seq=1'), /must preserve every loot2/);
  await rejectsWithoutChanges('occupied loot3 changed socket cap', body + corrupt('game_loot_socket_levels',
    'UPDATE game_loot_socket_levels SET cap=0 WHERE version=3'), /must preserve every loot2/);
  await rejectsWithoutChanges('occupied loot3 changed grade rate', body + corrupt('game_loot_grades',
    'UPDATE game_loot_grades SET rune_rate_bp=0 WHERE version=3'), /must preserve every loot2/);

  function verifyMigrated(state, selector = 3) {
    assert.equal(state.game_loot_rule_active[0].version, selector);
    assert.deepEqual(state.column, ['outdoor_equipment','outdoor_normal_equipment'].map(column_name=>({column_name,data_type:'jsonb',is_nullable:'YES',column_default:null})));
    const source = state.game_loot_rule_versions.find(r => r.version === 2);
    const created = state.game_loot_rule_versions.find(r => r.version === 3);
    assert.deepEqual(created, {...source, version: 3, published: true, outdoor_equipment: expected, outdoor_normal_equipment: expectedNormal});
    assert.deepEqual(state.game_loot_rule_versions.filter(r => r.version < 3),
      before.game_loot_rule_versions.map(r => ({...r, outdoor_equipment: null, outdoor_normal_equipment:null})));
    const canonical = rows => rows.map(({version, ...row}) => row)
      .sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
    for (const table of children) {
      assert.deepEqual(state[table].filter(r => r.version < 3), before[table]);
      assert.deepEqual(canonical(state[table].filter(r => r.version === 3)),
        canonical(state[table].filter(r => r.version === 2)));
    }
    for (const table of tables.filter(t => !t.startsWith('game_loot_'))) assert.deepEqual(state[table], before[table], table);
  }

  // Fresh initialization owns the transaction; upgrade owns an explicit one.
  await db.exec('BEGIN;' + body);
  verifyMigrated(await snapshot());
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(), before);
  pass('fresh transaction rollback restores old rules, selectors and schema');
  for (const selector of [1, 99]) {
    await db.exec('BEGIN;' + (selector === 99 ? clone(2, 99) +
      'UPDATE game_loot_rule_versions SET published=true WHERE version=99;' : '') +
      `UPDATE game_loot_rule_active SET version=${selector};` + body);
    verifyMigrated(await snapshot(), selector);
    await db.exec(body);
    assert.equal(await active(), selector);
    await db.exec('ROLLBACK');
    assert.deepEqual(await snapshot(), before);
    pass('active' + selector + ' preserved on initial application and replay');
  }
  await db.exec(upgrade);
  const after = await snapshot();
  verifyMigrated(after);
  pass('upgrade publishes/activates exact loot3 while preserving old rules, all dependencies, Rift and sentinels');
  const afterProjection = await loadProjection();
  assert.deepEqual(afterProjection.slice(0, 2), beforeProjection);
  assert.deepEqual(afterProjection[2], {...beforeProjection[1], Version: 3, OutdoorEquipment: expected, OutdoorNormalEquipment:expectedNormal});
  assert.equal(afterProjection.length, 3);
  pass('verbatim current LoadLootRules SELECT projects exact loot3 override and unchanged historical rules/dependencies');
  await db.exec(body);
  await db.exec(upgrade);
  assert.deepEqual(await snapshot(), after);
  pass('fresh and upgrade replays are exactly idempotent');

  for (const sql of [
    "UPDATE game_loot_rule_versions SET outdoor_equipment='{}' WHERE version=3",
    "UPDATE game_loot_rule_versions SET outdoor_equipment='{}' WHERE version=2",
    'DELETE FROM game_loot_rule_versions WHERE version=1',
    'DELETE FROM game_loot_rune_pool WHERE version=3',
    'UPDATE game_loot_socket_levels SET cap=0 WHERE version=3',
    'UPDATE game_loot_grades SET rune_rate_bp=0 WHERE version=3',
    'INSERT INTO game_loot_socket_levels VALUES(3,99,5)',
  ]) await assert.rejects(() => db.exec(sql), /immutable/);
  assert.deepEqual(await snapshot(), after);
  pass('published versions and children remain immutable under original production guards');
  console.log('All focused migration106 PostgreSQL checks passed. No deployed database or full initializer was used.');
} finally {
  await db.close();
}

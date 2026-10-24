// Run: PGLITE_MODULE=/absolute/path/to/@electric-sql/pglite node server/tests/rift_cumulative_postgres.mjs
// Actual PostgreSQL engine, production rule schemas/guards and verbatim SQL.
// Reduced setup: migrations63/76/77/78/79/80 build Rift1–6 and Loot1–2; Rift7
// uses the checked-in native-pool fixture instead of rerunning the native-drop
// catalog extraction. Real migrations90/92/99/105 then build Rift8–11, with
// reduced native hat/face/back prerequisites, and106 builds Loot3. This is not
// a full initializer, live-database, migration82 catalog, or concurrency test.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';

const require = createRequire(import.meta.url);
const {PGlite} = require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = n => fs.readFileSync(`${root}/migrations/000000_init-${n}.sql`, 'utf8');
const body = read(107);
const upgrade = fs.readFileSync(`${root}/migrations/upgrades/000107_20261024_rift_cumulative_legendaries.sql`, 'utf8');
assert.equal(upgrade, '-- Existing databases: apply atomically; do not rerun fresh init.\nBEGIN;\n' + body + 'COMMIT;\n');
const fixture = JSON.parse(fs.readFileSync(`${root}/internal/game/scene/testdata/rift_native_equipment_pools.json`, 'utf8'));
const excluded = JSON.parse(read(105).split('$excluded$')[1]);
const excludedIDs = new Set(excluded.map(x => x.id));
assert.equal(excludedIDs.size, 12);
assert.equal(fixture.Pools.length, 60);
const expectedPools = fixture.Pools.map(p => ({...p, Items: p.Items.filter(id => !excludedIDs.has(id))}));
assert(expectedPools.every(p => p.Items.length > 0));
for (const id of excludedIDs) assert(fixture.Pools.some(p => p.Items.includes(id)), `native fixture is missing hat ${id}`);
const fallbackItems = fixture.Items.filter(i => [1, 10].includes(i.Equip?.Slot));
assert.deepEqual(fallbackItems.map(i => i.ID).sort((a, b) => a - b), fixture.SlotFallbacks);
assert.deepEqual(fixture.SlotFallbacks, [...Array.from({length: 10}, (_, i) => 2601 + i), ...Array.from({length: 7}, (_, i) => 2701 + i)]);

const loaderSource = fs.readFileSync(`${root}/internal/data/endgame_balance_rules.go`, 'utf8');
function loaderSQL(name, next) {
  const source = loaderSource.slice(loaderSource.indexOf(`func ${name}(`), next ? loaderSource.indexOf(`func ${next}(`) : undefined);
  const queries = [...source.matchAll(/q\.Query\(ctx, `([^`]+)`\)/g)];
  assert.equal(queries.length, 1, `expected one verbatim ${name} projection`);
  return queries[0][1];
}
const lootSQL = loaderSQL('LoadLootRules', 'LoadRiftRules');
const riftSQL = loaderSQL('LoadRiftRules');
const riftChildren = ['game_rift_class_rules', 'game_rift_ability_rules'];
const lootChildren = ['game_loot_rune_pool', 'game_loot_socket_levels', 'game_loot_grades'];
const guarded = ['game_rift_rule_versions', ...riftChildren, 'game_loot_rule_versions', ...lootChildren];
const tables = ['game_rift_rule_versions', 'game_rift_rule_active', ...riftChildren,
  'game_loot_rule_versions', 'game_loot_rule_active', ...lootChildren,
  'game_items', 'game_endgame_rune_rules', 'gamedata.ov_item',
  'game_equipment', 'gamedata.ov_arm', 'game_equip_extra', 'focused_player_state'];
const expectedChanges = {version: 12, published: true, loot_version: 3,
  cumulative_legendary_per_tier: 10, equipment: 0, timely_equipment: 0,
  unique_base_chance: 0, unique_per_tier: 0, unique_cap: 0};
const db = new PGlite();
const pass = message => console.log('PASS ' + message);
const json = value => "'" + JSON.stringify(value).replaceAll("'", "''") + "'::jsonb";
const clone = (source, target, extra = {}) => {
  let sql = `INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,
    to_jsonb(r)||${json({version: target, published: false, ...extra})})).*
    FROM game_rift_rule_versions r WHERE version=${source};`;
  for (const table of riftChildren) sql += `INSERT INTO ${table} SELECT
    (jsonb_populate_record(NULL::${table},to_jsonb(r)||'{"version":${target}}'::jsonb)).*
    FROM ${table} r WHERE version=${source};`;
  return sql;
};
const publish = version => `UPDATE game_rift_rule_versions SET published=true WHERE version=${version};`;
const corrupt = (table, sql) => `ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules;
  ${sql}; ALTER TABLE ${table} ENABLE TRIGGER immutable_published_rules;`;
const active = async () => (await db.query('SELECT version FROM game_rift_rule_active WHERE id=1')).rows[0]?.version;
const projection = async sql => (await db.query(sql)).rows.map(row => JSON.parse(Object.values(row)[0]));
const canonical = rows => rows.map(({version, ...row}) => row)
  .sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));

async function snapshot() {
  const state = {};
  for (const table of tables) state[table] = (await db.query(`SELECT
    COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb) data FROM ${table} r`)).rows[0].data;
  state.column = (await db.query(`SELECT data_type,is_nullable,column_default FROM information_schema.columns
    WHERE table_schema='public' AND table_name='game_rift_rule_versions' AND column_name='cumulative_legendary_per_tier'`)).rows;
  return state;
}

console.log((await db.query('SELECT version()')).rows[0].version);
try {
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

  // Only migration82's expensive native-drop extraction is replaced by its
  // independently checked-in fixture. Do not claim a full initializer here.
  await db.exec('ALTER TABLE game_rift_rule_versions ADD COLUMN equipment_pools JSONB;');
  await db.exec(clone(6, 7, {equipment_pools: fixture.Pools}) + publish(7) +
    'UPDATE game_rift_rule_active SET version=7 WHERE id=1;');
  for (const n of [90, 92, 99]) await db.exec(read(n));
  await db.exec(`CREATE TABLE game_equipment(id INTEGER PRIMARY KEY,name TEXT,slot INTEGER,level_req INTEGER);
    CREATE TABLE gamedata.ov_arm(index INTEGER PRIMARY KEY,level_need INTEGER,level INTEGER,sex INTEGER DEFAULT 0,
      query_type INTEGER,no_type_drop INTEGER DEFAULT 0,position_ INTEGER,
      newbie INTEGER DEFAULT 0,warrior INTEGER DEFAULT 0,swordman INTEGER DEFAULT 0,
      stabber INTEGER DEFAULT 0,druggist INTEGER DEFAULT 0,magician INTEGER DEFAULT 0,
      sr_warrior INTEGER DEFAULT 0,sr_swordman INTEGER DEFAULT 0,sr_stabber INTEGER DEFAULT 0,
      sr_druggist INTEGER DEFAULT 0,sr_magician INTEGER DEFAULT 0);
    CREATE TABLE game_equip_extra(arm_id INTEGER,seq INTEGER,attr_id INTEGER,value INTEGER,
      mode INTEGER DEFAULT 0,PRIMARY KEY(arm_id,seq));`);
  async function nativeItem(id, name, slot, levelReq, native) {
    await db.query('INSERT INTO game_equipment VALUES($1,$2,$3,$4)', [id, name, slot, levelReq]);
    const columns = Object.keys(native);
    await db.query(`INSERT INTO gamedata.ov_arm(index,${columns.join(',')})
      VALUES(${[id, ...Object.values(native)].map((_, i) => '$' + (i + 1)).join(',')})`, [id, ...Object.values(native)]);
  }
  for (const hat of excluded) await nativeItem(hat.id, hat.name, 2, 1,
    {level_need: 1, level: 5, sex: hat.sex, query_type: 9, no_type_drop: 0, position_: 102});
  for (const item of fallbackItems) {
    const e = item.Equip, need = e.Need;
    assert(!e.AscensionOnly && !e.NoTypeDrop && !e.Affixes?.length);
    const gates = Object.fromEntries(['warrior', 'swordman', 'stabber', 'druggist', 'magician']
      .map((gate, i) => [gate, Number(Boolean(need.Professions & (1 << i)))]));
    await nativeItem(item.ID, item.Name, e.Slot, e.LevelReq, {level_need: need.Level,
      level: e.ResourceLevel, sex: need.Sex, query_type: e.Slot === 1 ? 15 : 13,
      no_type_drop: Number(e.NoTypeDrop), position_: e.Position, newbie: Number(need.BeginnerAllowed), ...gates});
  }
  await db.exec('UPDATE game_rift_rule_active SET version=10 WHERE id=1;' + read(105) + read(106));
  // Unrelated persisted player/paid-run/grant sentinels. Their reduced schema is
  // deliberately not presented as production player-data or grant coverage.
  await db.exec(`CREATE TABLE focused_player_state(id INTEGER PRIMARY KEY,items JSONB,paid_run JSONB,ready_grant JSONB);
    INSERT INTO focused_player_state VALUES(1,'[{"uid":123,"quality":6}]',
      '{"LootRules":{"Version":2,"UniqueBasePPB":10000000},"RiftRules":{"Version":11,"Equipment":1,"TimelyEquipment":1}}',
      '{"items":[{"uid":456,"quality":2}],"state":"ready"}');`);
  const before = await snapshot();
  assert.equal(await active(), 11);
  assert.equal(before.game_loot_rule_active[0].version, 3);
  assert.deepEqual(before.column, []);
  const source = before.game_rift_rule_versions.find(r => r.version === 11);
  assert.deepEqual(source.equipment_pools, expectedPools);
  assert.deepEqual(source.equipment_slot_fallbacks, fixture.SlotFallbacks);
  assert.equal(source.loot_version, 2);
  assert.equal(source.equipment, 1);
  assert.equal(source.timely_equipment, 1);
  assert.equal(source.normal_boss_equipment, 10);
  assert.equal(source.floor_layout_version, 6);
  assert.equal(source.guardian_recent_window, 3);
  pass('reconstructed published Rift11 with native pools minus twelve hats, all seventeen fallbacks, original elite rules and children');

  const beforeLootProjection = await projection(lootSQL);
  const beforeRiftProjection = await projection(riftSQL);
  assert.deepEqual(beforeLootProjection.map(r => r.Version), [1, 2, 3]);
  assert.deepEqual(beforeRiftProjection.map(r => r.Version), Array.from({length: 11}, (_, i) => i + 1));
  assert(beforeRiftProjection.every(r => r.CumulativeLegendaryPerTier === 0));
  pass('verbatim current LoadLootRules and LoadRiftRules SELECTs execute before107, including missing-column zero compatibility');

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

  await rejectsWithoutChanges('missing selector', 'DELETE FROM game_rift_rule_active', /active selector/);
  await rejectsWithoutChanges('missing source Rift11', 'UPDATE game_rift_rule_active SET version=10;' +
    riftChildren.map(table => corrupt(table, `DELETE FROM ${table} WHERE version=11`)).join('') +
    corrupt('game_rift_rule_versions', 'DELETE FROM game_rift_rule_versions WHERE version=11'), /unchanged published rift11/);
  for (const [field, value] of Object.entries({published: 'false', loot_version: '1', equipment_pools: 'NULL',
    equipment_slot_fallbacks: 'NULL', elite_rules: 'NULL', floor_layout_version: '5', guardian_recent_window: '0',
    normal_boss_equipment: '9', equipment: '0', timely_equipment: '0'})) {
    await rejectsWithoutChanges('invalid source Rift11 ' + field, corrupt('game_rift_rule_versions',
      `UPDATE game_rift_rule_versions SET ${field}=${value} WHERE version=11`), /unchanged published rift11/);
  }
  await rejectsWithoutChanges('source Rift11 already opted into cumulative rewards',
    'ALTER TABLE game_rift_rule_versions ADD COLUMN cumulative_legendary_per_tier INTEGER;' +
    corrupt('game_rift_rule_versions', 'UPDATE game_rift_rule_versions SET cumulative_legendary_per_tier=10 WHERE version=11'),
    /unchanged published rift11/);
  await rejectsWithoutChanges('missing Loot3', 'UPDATE game_loot_rule_active SET version=2;' +
    lootChildren.map(table => corrupt(table, `DELETE FROM ${table} WHERE version=3`)).join('') +
    corrupt('game_loot_rule_versions', 'DELETE FROM game_loot_rule_versions WHERE version=3'), /complete published outdoor loot3/);
  await rejectsWithoutChanges('unpublished Loot3', corrupt('game_loot_rule_versions',
    'UPDATE game_loot_rule_versions SET published=false WHERE version=3'), /complete published outdoor loot3/);
  await rejectsWithoutChanges('conflicting Loot3 outdoor rates', corrupt('game_loot_rule_versions',
    `UPDATE game_loot_rule_versions SET outdoor_equipment='{}'::jsonb WHERE version=3`), /complete published outdoor loot3/);
  for (const table of lootChildren) await rejectsWithoutChanges('incomplete Loot3 ' + table,
    corrupt(table, `DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=3 LIMIT 1)`), /complete published outdoor loot3/);
  for (const table of riftChildren) await rejectsWithoutChanges('incomplete Rift11 ' + table,
    corrupt(table, `DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=11 LIMIT 1)`), /complete rift11 dependencies/);
  for (const table of guarded) await rejectsWithoutChanges('disabled guard on ' + table,
    `ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules`, /enabled immutable rule guards/);
  await rejectsWithoutChanges('replica-only guard', 'ALTER TABLE game_rift_ability_rules ENABLE REPLICA TRIGGER immutable_published_rules',
    /enabled immutable rule guards/);
  await rejectsWithoutChanges('wrong guard operations', `DROP TRIGGER immutable_published_rules ON game_rift_class_rules;
    CREATE TRIGGER immutable_published_rules BEFORE UPDATE ON game_rift_class_rules
    FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules()`, /enabled immutable rule guards/);
  await rejectsWithoutChanges('wrong guard function', `CREATE FUNCTION focused_fake_guard() RETURNS trigger LANGUAGE plpgsql
    AS $$ BEGIN RETURN NEW; END $$; DROP TRIGGER immutable_published_rules ON game_rift_ability_rules;
    CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_rift_ability_rules
    FOR EACH ROW EXECUTE FUNCTION focused_fake_guard()`, /enabled immutable rule guards/);
  for (const definition of ['TEXT', 'BIGINT', 'INTEGER DEFAULT 0', 'INTEGER NOT NULL DEFAULT 0']) {
    await rejectsWithoutChanges('conflicting column ' + definition,
      `ALTER TABLE game_rift_rule_versions ADD COLUMN cumulative_legendary_per_tier ${definition}`, /nullable INTEGER column without a default/);
  }
  await rejectsWithoutChanges('occupied Rift12', clone(11, 12, {unique_base_chance: 999}) + publish(12), /rift12 is occupied/);
  await rejectsWithoutChanges('unpublished matching Rift12', body + corrupt('game_rift_rule_versions',
    'UPDATE game_rift_rule_versions SET published=false WHERE version=12'), /rift12 is occupied/);
  for (const [table, field] of [['game_rift_class_rules', 'hp_bp'], ['game_rift_ability_rules', 'damage_pct']]) {
    await rejectsWithoutChanges('Rift12 missing child in ' + table, body + corrupt(table,
      `DELETE FROM ${table} WHERE ctid=(SELECT ctid FROM ${table} WHERE version=12 LIMIT 1)`), /must preserve class and ability dependencies/);
    await rejectsWithoutChanges('Rift12 changed child in ' + table, body + corrupt(table,
      `UPDATE ${table} SET ${field}=${field}+1 WHERE version=12`), /must preserve class and ability dependencies/);
  }

  function verifyMigrated(state, input = before, selector = 12) {
    assert.equal(state.game_rift_rule_active[0].version, selector);
    assert.deepEqual(state.column, [{data_type: 'integer', is_nullable: 'YES', column_default: null}]);
    const original = input.game_rift_rule_versions.find(r => r.version === 11);
    const created = state.game_rift_rule_versions.find(r => r.version === 12);
    assert.deepEqual(created, {...original, cumulative_legendary_per_tier: null, ...expectedChanges});
    assert.deepEqual(state.game_rift_rule_versions.filter(r => r.version !== 12),
      input.game_rift_rule_versions.map(r => ({cumulative_legendary_per_tier: null, ...r})));
    for (const table of riftChildren) {
      assert.deepEqual(state[table].filter(r => r.version !== 12), input[table]);
      assert.deepEqual(canonical(state[table].filter(r => r.version === 12)),
        canonical(state[table].filter(r => r.version === 11)));
    }
    for (const table of tables.filter(t => !t.startsWith('game_rift_'))) assert.deepEqual(state[table], input[table], table);
  }

  // Fresh initialization owns the transaction. Roll back after publication and
  // activation, proving both the added schema and inserted rows disappear.
  await db.exec('BEGIN;' + body);
  verifyMigrated(await snapshot());
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(), before);
  pass('fresh transaction rollback restores source rules, all dependencies, selectors, sentinels and schema');

  // Exercise the actual upgrade's explicit BEGIN/COMMIT failure path, not just
  // a manually wrapped body. The separate fixture change is deliberately
  // committed before the upgrade and must remain the only durable change.
  await db.exec(corrupt('game_rift_rule_versions', 'UPDATE game_rift_rule_versions SET published=false WHERE version=11'));
  const invalidUpgradeInput = await snapshot();
  await assert.rejects(() => db.exec(upgrade), /unchanged published rift11/);
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(), invalidUpgradeInput);
  await db.exec(corrupt('game_rift_rule_versions', 'UPDATE game_rift_rule_versions SET published=true WHERE version=11'));
  assert.deepEqual(await snapshot(), before);
  pass('actual upgrade failure rolls back all attempted migration effects, including its schema addition');

  // Includes both older paging gates and a future release. Only exactly11 moves.
  for (const selector of [1, 9, 10, 13, 99]) {
    await db.exec('BEGIN;' + (selector > 12 ? clone(11, selector) + publish(selector) : '') +
      `UPDATE game_rift_rule_active SET version=${selector};`);
    const input = await snapshot();
    await db.exec(body);
    verifyMigrated(await snapshot(), input, selector);
    const after = await snapshot();
    await db.exec(body);
    assert.deepEqual(await snapshot(), after);
    assert.equal(await active(), selector);
    await db.exec('ROLLBACK');
    assert.deepEqual(await snapshot(), before);
    pass('active Rift' + selector + ' is not overridden on initial application or replay');
  }
  await db.exec('BEGIN;ALTER TABLE game_rift_rule_versions ADD COLUMN cumulative_legendary_per_tier INTEGER;' +
    clone(11, 99, {cumulative_legendary_per_tier: 20}) + publish(99) +
    'UPDATE game_rift_rule_active SET version=99;');
  const existingColumnInput = await snapshot();
  await db.exec(body);
  verifyMigrated(await snapshot(), existingColumnInput, 99);
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(), before);
  pass('existing nullable integer column and a future rule value are preserved without forcing activation');

  await db.exec('BEGIN;' + body + 'UPDATE game_rift_rule_active SET version=11;' + body);
  verifyMigrated(await snapshot());
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(), before);
  pass('an already published exact Rift12 can activate from11 without duplicating rules or children');

  // Rift admission policy pins Loot3 independently of the outdoor active
  // selector; publishing Rift12 must not activate a separate loot version.
  await db.exec('BEGIN;UPDATE game_loot_rule_active SET version=2;');
  const inactiveLootInput = await snapshot();
  await db.exec(body);
  verifyMigrated(await snapshot(), inactiveLootInput);
  assert.equal((await snapshot()).game_loot_rule_active[0].version, 2);
  await db.exec('ROLLBACK');
  assert.deepEqual(await snapshot(), before);
  pass('Rift12 pins published Loot3 without altering an independently selected outdoor Loot2');

  await db.exec(upgrade);
  const after = await snapshot();
  verifyMigrated(after);
  pass('atomic upgrade publishes/activates exact Rift12 clone with Loot3, cumulative10, zero old special slots/chances, unchanged source and children');
  const afterLootProjection = await projection(lootSQL);
  const afterRiftProjection = await projection(riftSQL);
  assert.deepEqual(afterLootProjection, beforeLootProjection);
  assert.equal(afterRiftProjection.length, 12);
  assert.deepEqual(afterRiftProjection.slice(0, 11), beforeRiftProjection);
  assert.deepEqual(afterRiftProjection[11], {...beforeRiftProjection[10], Version: 12, LootVersion: 3,
    CumulativeLegendaryPerTier: 10, Equipment: 0, TimelyEquipment: 0,
    UniqueBaseChance: 0, UniquePerTier: 0, UniqueCap: 0});
  pass('verbatim current LoadLootRules/LoadRiftRules SELECTs execute after107 and project exact new rewards, unchanged historical values and dependencies');
  await db.exec(body);
  await db.exec(upgrade);
  assert.deepEqual(await snapshot(), after);
  pass('fresh-body and upgrade replays are exactly idempotent with active Rift12');

  for (const sql of [
    'UPDATE game_rift_rule_versions SET cumulative_legendary_per_tier=20 WHERE version=12',
    'UPDATE game_rift_rule_versions SET cumulative_legendary_per_tier=10 WHERE version=11',
    'UPDATE game_rift_rule_versions SET equipment=2 WHERE version=1',
    "UPDATE game_rift_rule_versions SET equipment_pools='[]' WHERE version=12",
    "UPDATE game_rift_rule_versions SET equipment_slot_fallbacks='[]' WHERE version=12",
    'DELETE FROM game_rift_rule_versions WHERE version=12',
    'DELETE FROM game_rift_class_rules WHERE version=12',
    'UPDATE game_rift_class_rules SET hp_bp=hp_bp+1 WHERE version=11',
    'UPDATE game_rift_ability_rules SET damage_pct=damage_pct+1 WHERE version=12',
    'DELETE FROM game_rift_ability_rules WHERE version=11',
    'INSERT INTO game_rift_class_rules SELECT 12,class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=11',
    'INSERT INTO game_rift_ability_rules SELECT 12,id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=11',
    "UPDATE game_loot_rule_versions SET outdoor_equipment='{}' WHERE version=3",
    'DELETE FROM game_loot_rune_pool WHERE version=3',
    'UPDATE game_loot_socket_levels SET cap=0 WHERE version=3',
    'UPDATE game_loot_grades SET rune_rate_bp=0 WHERE version=3',
  ]) await assert.rejects(() => db.exec(sql), /immutable/);
  assert.deepEqual(await snapshot(), after);
  pass('original production guards keep published old/new Rift rules, children and Loot3 immutable');
  console.log('All focused migration107 PostgreSQL checks passed. Reduced native prerequisites; no full initializer, deployed database, or concurrency test was used.');
} finally {
  await db.close();
}

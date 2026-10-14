// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/cube_retirement_postgres.mjs
import { createRequire } from 'node:module';
import fs from 'node:fs';
import assert from 'node:assert/strict';
const require = createRequire(import.meta.url);
const { PGlite } = require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const read = n => fs.readFileSync(new URL(`../migrations/000000_init-${n}.sql`, import.meta.url), 'utf8');
const db = new PGlite();
const seed = JSON.parse(read(61).split('$cube_rules$')[1]);
await db.exec('CREATE TABLE game_items(id integer primary key);');
for (const id of new Set(seed.flatMap(r=>[...r.Materials,...r.Outputs].map(x=>x.ItemID)))) await db.query('INSERT INTO game_items VALUES($1)', [id]);
await db.exec(read(61));
const snapshot = async () => (await db.query(`SELECT jsonb_build_object('recipes',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM game_cube_recipes r),'materials',(SELECT jsonb_agg(to_jsonb(m) ORDER BY recipe_id,seq) FROM game_cube_materials m),'outputs',(SELECT jsonb_agg(to_jsonb(o) ORDER BY recipe_id,seq) FROM game_cube_outputs o)) AS data`)).rows[0].data;
const before = await snapshot();
await db.exec(read(83));
const after = await snapshot();
const disabled = [1007,1008,1009,1012];
assert.deepEqual(after.recipes.filter(r=>!r.enabled).map(r=>r.id),disabled);
assert.equal(after.recipes.filter(r=>r.enabled).length,62);
assert.equal(new Set(after.recipes.filter(r=>r.enabled).map(r=>r.operation)).size,8);
assert.deepEqual(after.recipes.map(({enabled,...r})=>r),before.recipes);
assert.deepEqual(after.materials,before.materials);
assert.deepEqual(after.outputs,before.outputs);
await db.exec(read(83));
assert.deepEqual(await snapshot(),after);
// Execute the exact production SQL, including the enabled boolean read by Go.
const loader = fs.readFileSync(new URL('../internal/data/endgame_cube.go',import.meta.url),'utf8');
for (const [,sql] of loader.matchAll(/q.Query\(ctx, `([^`]+)`\)/g)) {
 const result = await db.query(sql);
 if (sql.includes('FROM game_cube_recipes')) {
  assert.equal(result.rows.length,66);
  assert.deepEqual(result.rows.filter(r=>!r.enabled).map(r=>r.id),disabled);
 }
}
console.log('PASS migration83: 62 enabled recipes, 8 operations, 4 disabled identities, unchanged historical definitions/children, idempotent replay, production loader queries.');
await db.close();

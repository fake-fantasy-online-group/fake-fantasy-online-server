// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/cube_costs_postgres.mjs
// Runs the real PostgreSQL migration, then exercises Go matching/execution with
// the resulting database rows (no hand-maintained duplicate recipe fixture).
import { createRequire } from 'node:module';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
const require = createRequire(import.meta.url);
const { PGlite } = require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const read = n => fs.readFileSync(new URL(`../migrations/000000_init-${n}.sql`, import.meta.url), 'utf8');
const db = new PGlite();
const seed = JSON.parse(read(61).split('$cube_rules$')[1]);
await db.exec('CREATE TABLE game_items(id integer primary key);');
for (const id of new Set(seed.flatMap(r=>[...r.Materials,...r.Outputs].map(x=>x.ItemID)))) await db.query('INSERT INTO game_items VALUES($1)', [id]);
await db.exec(read(61));
await db.exec(read(83));
const snapshot = async () => (await db.query(`SELECT jsonb_build_object('recipes',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM game_cube_recipes r),'materials',(SELECT jsonb_agg(to_jsonb(m) ORDER BY recipe_id,seq) FROM game_cube_materials m),'outputs',(SELECT jsonb_agg(to_jsonb(o) ORDER BY recipe_id,seq) FROM game_cube_outputs o)) AS data`)).rows[0].data;
const before = await snapshot();
await db.exec(read(86));
const after = await snapshot();
const disabled = [1007,1008,1009,1012];
assert.deepEqual(after.recipes.filter(r=>!r.enabled).map(r=>r.id),disabled);
assert.equal(after.recipes.filter(r=>r.enabled).length,62);
for (const r of after.recipes) {
 const mats=after.materials.filter(m=>m.recipe_id===r.id);
 assert.equal(r.material_count,mats.length);
 assert.deepEqual(mats.map(m=>m.seq),mats.map((_,i)=>i+1));
 assert.equal(r.output_count,after.outputs.filter(o=>o.recipe_id===r.id).length);
 if (disabled.includes(r.id)) {
  assert.deepEqual(r,before.recipes.find(x=>x.id===r.id));
  assert.deepEqual(mats,before.materials.filter(x=>x.recipe_id===r.id));
 }
}
const byID=id=>after.recipes.find(r=>r.id===id);
const mats=id=>after.materials.filter(r=>r.recipe_id===id).map(r=>[r.item_id,r.count]);
const marrow=1900028201,dust=1900028202,echo=1900028203;
assert.deepEqual(mats(1006),[[marrow,4]]);
assert.deepEqual(mats(1010),[[marrow,120],[dust,30],[echo,4]]);
assert.deepEqual(mats(1011),[[marrow,40],[dust,25],[echo,4]]);
assert.equal(byID(1006).money,0);
assert.equal(byID(1010).money,10000000);
assert.equal(byID(1011).money,1000000);
for(let i=0;i<28;i++) {
 assert.equal(byID(11001+i).money,0);
 assert.deepEqual(mats(11001+i),[[marrow,30],[dust,10]]);
 assert.equal(byID(12001+i).money,0);
 assert.equal(byID(12001+i).source_count,3);
 assert.deepEqual(mats(12001+i),[]);
}
assert.deepEqual(after.outputs,before.outputs.map(o=>o.recipe_id===1002&&o.item_id===echo?{...o,count:1}:o));
// Replaying both retirement and rebalance must preserve the complete snapshot.
await db.exec(read(83));
await db.exec(read(86));
assert.deepEqual(await snapshot(),after);
const rows=after.recipes.filter(r=>r.enabled).map(r=>({ID:r.id,Name:r.name,Operation:r.operation,Description:r.description,TargetRequired:r.target_required,Money:r.money,SourceCount:r.source_count,QualityStepBP:r.quality_step_bp,Materials:after.materials.filter(m=>m.recipe_id===r.id).map(m=>({ItemID:m.item_id,Count:m.count})),Outputs:after.outputs.filter(o=>o.recipe_id===r.id).map(o=>({ItemID:o.item_id,Count:o.count,LevelDivisor:o.level_divisor,LevelCap:o.level_cap,Multiplier:o.multiplier}))}));
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'cube-costs-'));
try {
 const fixture=path.join(temp,'rules.json');
 fs.writeFileSync(fixture,JSON.stringify(rows));
 execFileSync('go',['test','./internal/domain','-run','^TestCubeRevisedCostsFromDatabase$','-count=1','-v'],{cwd:fileURLToPath(new URL('..',import.meta.url)),env:{...process.env,CUBE_REVISED_RULES:fixture},stdio:'inherit'});
} finally { fs.rmSync(temp,{recursive:true,force:true}); await db.close(); }
console.log('PASS migration86: exact costs/yields, contiguous child counts, unchanged retired definitions, idempotent replay, database-backed Go matching/execution.');

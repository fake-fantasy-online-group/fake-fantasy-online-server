// Real PostgreSQL engine with focused prerequisite schema, not a live initializer.
// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/spirit_migration_postgres.mjs
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE||'@electric-sql/pglite');
import fs from 'node:fs';import assert from 'node:assert/strict';
const root=fileURLToPath(new URL('../migrations/',import.meta.url));
const schema120=fs.readFileSync(root+'upgrades/000120_20261028_spirit_instances.sql','utf8');
const schema121=fs.readFileSync(root+'upgrades/000121_20261028_spirit_appearance_recipes.sql','utf8');
const source37=fs.readFileSync(root+'000000_init-37.sql','utf8');
const craftSchema=source37.match(/CREATE TABLE IF NOT EXISTS game_craft_extra_recipes \([\s\S]*?\n\);/)[0];
const db=new PGlite();
await db.exec(`CREATE TABLE item_instances(uid BIGINT PRIMARY KEY,instance_kind SMALLINT NOT NULL CHECK(instance_kind IN(1,2,3)));INSERT INTO item_instances VALUES(1,1),(2,2),(3,3); CREATE SCHEMA gamedata;CREATE TABLE gamedata.ov_combine(row_no integer PRIMARY KEY);INSERT INTO gamedata.ov_combine VALUES(1),(4781);`+craftSchema);
await db.exec(schema120);await db.exec(schema121);await db.exec(schema120);await db.exec(schema121);
assert.deepEqual((await db.query('SELECT * FROM item_instances ORDER BY uid')).rows,[{uid:1,instance_kind:1},{uid:2,instance_kind:2},{uid:3,instance_kind:3}]);
assert.equal((await db.query('SELECT count(*)::int n FROM game_craft_extra_recipes')).rows[0].n,22);
assert.equal((await db.query('SELECT count(DISTINCT product_item_id)::int n FROM game_craft_extra_recipes')).rows[0].n,20);
await db.exec(`INSERT INTO item_instances VALUES(4,4);INSERT INTO spirit_instances VALUES(4,'{"Kind":1}');INSERT INTO equipment_spirit_fusions VALUES(1,4,0);`);
assert.equal((await db.query('SELECT count(*)::int n FROM equipment_spirit_fusions')).rows[0].n,1);
// A conflicting original recipe aborts the transaction without any partial writes.
await db.exec('DELETE FROM game_craft_extra_recipes;INSERT INTO gamedata.ov_combine VALUES(5600);');
await assert.rejects(db.exec(schema121),/collides with original/);await db.exec('ROLLBACK;');
assert.equal((await db.query('SELECT count(*)::int n FROM game_craft_extra_recipes')).rows[0].n,0);
await db.exec('DELETE FROM gamedata.ov_combine WHERE row_no=5600;INSERT INTO game_craft_extra_recipes(row_no,product_item_id) VALUES(5600,999);');
await assert.rejects(db.exec(schema121),/already belongs to different data/);await db.exec('ROLLBACK;');
assert.deepEqual((await db.query('SELECT row_no,product_item_id FROM game_craft_extra_recipes')).rows,[{row_no:5600,product_item_id:999}]);
// Missing prerequisite schema fails atomically and does not leave spirit tables.
const empty=new PGlite();await assert.rejects(empty.exec(schema120),/item_instances/);await empty.exec('ROLLBACK;');
assert.equal((await empty.query("SELECT to_regclass('spirit_instances') AS t")).rows[0].t,null);
await db.exec('DROP TABLE game_craft_extra_recipes;');await assert.rejects(db.exec(schema121),/game_craft_extra_recipes/);await db.exec('ROLLBACK;');
assert.equal((await db.query('SELECT count(*)::int n FROM spirit_instances')).rows[0].n,1);
console.log('PASS: 120/121 apply twice; 22 routes/20 products; legacy instances unchanged; new spirit/fusion writes; both collision guards atomic; missing dependencies atomic.');await empty.close();await db.close();

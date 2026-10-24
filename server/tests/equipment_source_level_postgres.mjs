// Actual PostgreSQL via PGlite, focused schema/save/load SQL rather than full server integration.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const body=fs.readFileSync(`${root}/migrations/000000_init-108.sql`,'utf8');
const upgrade=fs.readFileSync(`${root}/migrations/upgrades/000108_20261024_equipment_source_level.sql`,'utf8');
assert.equal(upgrade,'-- Existing databases: apply atomically; do not rerun fresh init.\nBEGIN;\n'+body+'COMMIT;\n');
const source=fs.readFileSync(`${root}/internal/store/endgame_item.go`,'utf8');
const save=source.match(/`(INSERT INTO equipment_endgame_state[^`]+)`/)[1];
const load=source.match(/`(SELECT uid,revision[^`]+)`/)[1];
let checks=0;const pass=msg=>{checks++;console.log('PASS '+msg)};
const db=new PGlite();
try {
 await db.exec(`CREATE TABLE equipment_endgame_state(uid BIGINT PRIMARY KEY,revision BIGINT NOT NULL DEFAULT0,unique_id INTEGER NOT NULL DEFAULT0,unique_roll_bp INTEGER NOT NULL DEFAULT0,formation_roll_bp INTEGER NOT NULL DEFAULT0,unique_content_version INTEGER NOT NULL DEFAULT0,formation_content_version INTEGER NOT NULL DEFAULT0);`.replaceAll('DEFAULT0','DEFAULT 0'));
 await db.exec(`INSERT INTO equipment_endgame_state(uid,revision) VALUES(1,7)`);
 await db.exec(upgrade);
 assert.deepEqual((await db.query('SELECT uid,revision,source_level FROM equipment_endgame_state')).rows,[{uid:1,revision:7,source_level:0}]);pass('old metadata remains unchanged apart from explicit unknown0 provenance');
 await db.exec(upgrade);pass('upgrade replay idempotent');
 for(const level of [0,43,2147483647]) {
  await db.query(save,[2,4,0,0,10417,0,0,level]);
  const row=(await db.query(load,[[2]])).rows[0];assert.equal(row.source_level,level);assert.equal(row.formation_roll_bp,10417);assert.equal(row.revision,4);
 }pass('verbatim production save/load SQL retains generation level and unrelated metadata');
 await assert.rejects(db.query(save,[3,0,0,0,0,0,0,-1]));pass('negative provenance rejected by database');
 await db.exec(`ALTER TABLE equipment_endgame_state DROP CONSTRAINT equipment_endgame_source_level_nonnegative; ALTER TABLE equipment_endgame_state ADD CONSTRAINT equipment_endgame_source_level_nonnegative CHECK(source_level > -2)`);
 await assert.rejects(db.exec(upgrade));await db.exec('ROLLBACK');pass('conflicting same-name weaker constraint rejected');
 await db.exec(`ALTER TABLE equipment_endgame_state DROP CONSTRAINT equipment_endgame_source_level_nonnegative; ALTER TABLE equipment_endgame_state ALTER COLUMN source_level SET DEFAULT 1`);
 await assert.rejects(db.exec(upgrade));await db.exec('ROLLBACK');pass('conflicting schema default rejected');
 console.log(`PASS ${checks} source-level PostgreSQL checks`);
} finally {await db.close()}

// Run: PGLITE_MODULE=/absolute/path/to/node_modules/@electric-sql/pglite node server/tests/elite_activation_postgres.mjs
// Reduced dependency database, real PostgreSQL engine and verbatim migration SQL.
// Native monster/skill fixture statements come from migrations 0, 2, 31, 32,
// never from the activation migration's expected rows. This is not a full DB import.
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const read=n=>fs.readFileSync(`${root}/migrations/000000_init-${n}.sql`,'utf8');
const db=new PGlite();
const report={engine:(await db.query('select version()')).rows[0].version,scope:'Reduced dependency database; native source SQL, not full migration-chain import; no concurrent-session test',sha256:{},checks:[]};
for(const n of [0,2,31,32,59,63,76,77,78,79,80,81])report.sha256[n]=crypto.createHash('sha256').update(read(n)).digest('hex');
const pass=name=>{report.checks.push(name);console.log('PASS '+name)};
// Split SQL while preserving quoted strings, dollar blocks and comments.
function statements(s){let out=[],start=0,q=null,dollar=null,line=false,block=false;for(let i=0;i<s.length;i++){const c=s[i],n=s[i+1];if(line){if(c==='\n')line=false;continue}if(block){if(c==='*'&&n==='/'){block=false;i++}continue}if(q){if(c===q){if(n===q)i++;else q=null}continue}if(dollar){if(s.startsWith(dollar,i)){i+=dollar.length-1;dollar=null}continue}if(c==='-'&&n==='-'){line=true;i++;continue}if(c==='/'&&n==='*'){block=true;i++;continue}if(c==="'"||c==='"'){q=c;continue}if(c==='$'){const m=s.slice(i).match(/^\$[a-zA-Z_0-9]*\$/);if(m){dollar=m[0];i+=dollar.length-1;continue}}if(c===';'){out.push(s.slice(start,i+1));start=i+1}}return out}
const clean=s=>s.replace(/--[^\n]*/g,'').trim();
async function select(n,predicate){let count=0;for(const s of statements(read(n)))if(predicate(clean(s))){await db.exec(s);count++}assert(count>0,`no selected fixture statements from ${n}`)}
await db.exec('CREATE TABLE game_items(id integer primary key); CREATE SCHEMA gamedata; CREATE TABLE gamedata.ov_item(index integer primary key, level_need integer); CREATE TABLE game_endgame_rune_rules(item_id integer primary key);');
const items=JSON.parse(read(59).split('$endgame_items$')[1]);
for(const i of items)await db.query('INSERT INTO game_items VALUES($1)',[i.id]);
for(const i of items.filter(i=>i.id>=1900028001&&i.id<=1900028028)){await db.query('INSERT INTO gamedata.ov_item VALUES($1,$2)',[i.id,i.level]);await db.query('INSERT INTO game_endgame_rune_rules VALUES($1)',[i.id]);}
await select(0,s=>/^CREATE TABLE IF NOT EXISTS (game_monsters|game_monster_ai_templates|game_monster_skills|game_monster_skill_effects|gamedata\.ov_skilldesc)\s*\(/.test(s)||/^ALTER TABLE game_monsters ADD COLUMN/.test(s)||/^INSERT INTO game_monster_ai_templates\s/.test(s));
await select(2,s=>/^INSERT INTO "game_monsters"/.test(s));
await select(31,s=>/^INSERT INTO "gamedata"\."ov_skilldesc"/.test(s));
await select(32,s=>/^UPDATE game_monsters AS target SET/.test(s)||/^INSERT INTO game_monster_skills\s/.test(s)||/^INSERT INTO game_monster_skill_effects\s/.test(s)||/^UPDATE game_monster_skill_effects\s/.test(s)||/^UPDATE game_monster_skills\s/.test(s)||/^WITH (candidates AS|boss_rules\s)/.test(s)||/^UPDATE game_monsters\s+SET ai_template/.test(s));
pass('Native identities, classifications, AI and enabled skill/effect content built from independent source SQL');
for(const n of [63,76,77,78,79,80])await db.exec(read(n));
const tables=['game_rift_rule_versions','game_rift_class_rules','game_rift_ability_rules','game_rift_rule_active','game_loot_rule_versions','game_loot_rune_pool','game_loot_socket_levels','game_loot_grades','game_loot_rule_active','game_monsters','game_monster_ai_templates','game_monster_skills','game_monster_skill_effects','gamedata.ov_skilldesc','game_items','game_endgame_rune_rules','gamedata.ov_item'];
const snapshot=async()=>{let o={};for(const t of tables)o[t]=(await db.query(`SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb) data FROM ${t} r`)).rows[0].data;return o};
const active=async()=>(await db.query('SELECT version FROM game_rift_rule_active WHERE id=1')).rows[0]?.version;
assert.equal(await active(),5);const before=await snapshot();
await db.exec(read(81));assert.equal(await active(),6);const after=await snapshot();assert.deepEqual({...after,game_rift_rule_active:before.game_rift_rule_active},before);
pass('Fresh migration81 activates 5 to 6; every other fixture table and loot selector unchanged');
await db.exec(read(81));assert.deepEqual(await snapshot(),after);pass('Replay on active6 is exactly idempotent');
for(const sql of ["UPDATE game_rift_rule_versions SET name='tampered' WHERE version=6","DELETE FROM game_rift_rule_versions WHERE version=5","DELETE FROM game_rift_class_rules WHERE version=6","DELETE FROM game_rift_ability_rules WHERE version=6","UPDATE game_loot_rule_versions SET min_rune_level=1 WHERE version=2"]){await assert.rejects(()=>db.exec(sql),/immutable/)}
pass('Original migration63 immutable published guards still reject rule tampering');
// Corruption cases use a temporary transaction and explicitly disable/re-enable a
// guard only to manufacture invalid persisted input; each case is rolled back.
const corrupt=(table,sql)=>`ALTER TABLE ${table} DISABLE TRIGGER immutable_published_rules; ${sql}; ALTER TABLE ${table} ENABLE TRIGGER immutable_published_rules;`;
const cases=[
['absent active selector','DELETE FROM game_rift_rule_active',/requires active rift5/],
['active4','UPDATE game_rift_rule_active SET version=4',/requires active rift5/],
['disabled immutable guard','ALTER TABLE game_rift_class_rules DISABLE TRIGGER immutable_published_rules',/immutable published-rule guards/],
['wrong immutable guard function',`CREATE FUNCTION fake_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$; DROP TRIGGER immutable_published_rules ON game_rift_class_rules; CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_rift_class_rules FOR EACH ROW EXECUTE FUNCTION fake_guard()`,/immutable published-rule guards/],
['changed v6 config',corrupt('game_rift_rule_versions',`UPDATE game_rift_rule_versions SET elite_rules=jsonb_set(elite_rules,'{Elite,HazardMaxConcurrent}','7') WHERE version=6`),/exact published/],
['unpublished v6',corrupt('game_rift_rule_versions','UPDATE game_rift_rule_versions SET published=false WHERE version=6'),/exact published/],
['missing historical publication',corrupt('game_rift_rule_versions','UPDATE game_rift_rule_versions SET published=false WHERE version=1'),/historical rift versions/],
['missing class',corrupt('game_rift_class_rules','DELETE FROM game_rift_class_rules WHERE version=6 AND class=(SELECT min(class) FROM game_rift_class_rules WHERE version=6)'),/class and legacy ability/],
['changed legacy ability',corrupt('game_rift_ability_rules',"UPDATE game_rift_ability_rules SET name='bad' WHERE version=6"),/class and legacy ability/],
['changed loot weight',corrupt('game_loot_rune_pool','UPDATE game_loot_rune_pool SET weight=weight+1 WHERE version=2 AND seq=1'),/weighted loot2/],
['missing rune source','DELETE FROM gamedata.ov_item WHERE index=1900028001',/weighted loot2/],
['wrong native sprite','UPDATE game_monsters SET sprite=1 WHERE id=1011',/guardian identities/],
['wrong native classification',"UPDATE game_monsters SET kind='普通' WHERE id=1092",/guardian identities/],
['wrong native AI',"UPDATE game_monsters SET ai_template='melee' WHERE id=1060",/guardian identities/],
['missing native skill','DELETE FROM game_monster_skills WHERE monster_id=1073 AND skill_id=10010',/original guardian skills/],
['disabled native skill','UPDATE game_monster_skills SET enabled=false WHERE monster_id=1085 AND skill_id=12023',/original guardian skills/],
['zero native cast chance','UPDATE game_monster_skills SET chance_bp=0 WHERE monster_id=1136',/original guardian skills/],
['missing skill description','DELETE FROM gamedata.ov_skilldesc WHERE skill_id=12034',/original guardian skills/],
['missing native effects','DELETE FROM game_monster_skill_effects WHERE skill_id=12021; UPDATE game_monster_skills SET fallback_damage_pct=0 WHERE monster_id=3044',/original guardian skills/]
];
for(const [name,mutation,error] of cases){await db.exec('BEGIN; UPDATE game_rift_rule_active SET version=5;'+mutation+'; SAVEPOINT damaged;');await assert.rejects(()=>db.exec(read(81)),error);await db.exec('ROLLBACK TO SAVEPOINT damaged');assert.notEqual(await active(),6);await db.exec('ROLLBACK');assert.deepEqual(await snapshot(),after);pass('Rejects '+name+' without selector activation; rollback restores baseline')}
await db.exec(`BEGIN; INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,to_jsonb(r)||'{"version":7,"published":false}'::jsonb)).* FROM game_rift_rule_versions r WHERE version=6; UPDATE game_rift_rule_active SET version=7; DELETE FROM game_monsters WHERE id=1011; ALTER TABLE game_rift_class_rules DISABLE TRIGGER immutable_published_rules;`);
const future=await snapshot();await db.exec(read(81));assert.equal(await active(),7);assert.deepEqual(await snapshot(),future);await db.exec('ROLLBACK');pass('Active7 replay never downgrades or validates obsolete v6 dependencies');
report.passed=true;fs.writeFileSync(process.env.ELITE_ACTIVATION_REPORT||`${root}/docs/endgame/elite-activation-postgres-report.json`,JSON.stringify(report,null,2)+'\n');await db.close();

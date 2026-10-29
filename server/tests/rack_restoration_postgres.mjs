// PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/rack_restoration_postgres.mjs
// Real PostgreSQL engine (PGlite), focused native-derived catalog; not live DB or a full server launch.
import fs from 'node:fs';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const root=new URL('../',import.meta.url);
const read=p=>fs.readFileSync(new URL(p,root),'utf8');
const fixture=JSON.parse(execFileSync('python3',[new URL('rack_catalog_source.py',import.meta.url).pathname],{encoding:'utf8',maxBuffer:16*1024*1024}));
const report=JSON.parse(read('docs/commerce/rack-restoration-129.json'));
const body=read('migrations/000000_init-129.sql');
const upgrade=read('migrations/upgrades/000129_20261029_rack_restoration.sql');
assert.equal(upgrade,"-- Existing databases: explicit authorized maintenance only, never rerun fresh init.\nBEGIN;\nSET LOCAL lock_timeout = '5s';\nSET LOCAL statement_timeout = '60s';\n"+body+'COMMIT;\n');
const db=new PGlite();let checks=0;const pass=m=>{checks++;console.log('PASS '+m)};
const names=new Map(fixture.items.map(r=>[r.id,r.name]));
const pets=new Map(fixture.pets.map(r=>[r.pet_id,r]));
const profiles=new Map(fixture.profiles.map(r=>[r.pet_id,r]));
const mappings=new Map([...fixture.pet_goods,...fixture.eggs].map(r=>[r.item_id,r.pet_id]));
const saddleItems=new Set(fixture.saddles.map(r=>r.item_id));
const saddlePets=new Set(fixture.saddle_pets.map(r=>r.pet_id));
const natural=id=>{const p=profiles.get(id),native=pets.get(id);return native.horse_base_spd>0&&(p?(p.initial_skill_names.includes('坐骑')||!p.forbidden_skill_names.includes('坐骑')):[1,2,3,4,5,6].some(n=>native[`init_skills${n}_id`]===13002))};
const required=new Set([...saddlePets].filter(id=>!natural(id)));
const expectedRemoved=fixture.goods.filter(g=>saddleItems.has(g.item_id)||required.has(mappings.get(g.item_id))||(g.item_id>=4001&&g.item_id<=4030)||/蟠桃|戒灵/.test(names.get(g.item_id)||''));
assert.deepEqual(report.removed.map(r=>r.item_id).sort((a,b)=>a-b),expectedRemoved.map(r=>r.item_id).sort((a,b)=>a-b));
assert.equal(expectedRemoved.length,442);assert.equal(expectedRemoved.filter(r=>r.active).length,441);
assert.equal(report.site_overlap_count,373);assert.equal(report.site_missing.length,59);assert.equal(report.unpriced_tier_zero.length,11);
assert.equal(report.repriced.length,180);assert.equal(report.costumes.length,329);
const siteTiers=new Map(report.site_pet_tiers.map(r=>[r.pet_id,r.tier]));
for(const r of fixture.pet_goods)if(siteTiers.has(r.pet_id))assert.equal(pets.get(r.pet_id).pet_circle,siteTiers.get(r.pet_id));
for(const r of report.repriced){assert.equal(r.new_price,pets.get(r.pet_id).pet_circle*10);assert.equal(fixture.goods.find(g=>g.item_id===r.item_id).flags&4,0);}
pass('full native catalog independently yields 442 retirement rows / 441 active, 180 prices, 329 category decisions');
const insert=async (table,rows,columns)=>{
 if(!rows.length)return;
 const cols=columns.map(c=>c.split(' ')[0]);
 await db.query(`INSERT INTO ${table}(${cols.join(',')}) SELECT ${cols.join(',')} FROM jsonb_to_recordset($1::jsonb) AS x(${columns.join(',')})`,[JSON.stringify(rows)]).catch(e=>{throw new Error(table+": "+e.message+" "+e.detail)});
};
const allRows=async t=>(await db.query(`SELECT to_jsonb(x) AS row FROM ${t} x ORDER BY to_jsonb(x)::text`)).rows.map(r=>r.row);
try {
 await db.exec(`CREATE SCHEMA gamedata;
 CREATE TABLE game_items(id int PRIMARY KEY,name text NOT NULL);
 CREATE TABLE game_equipment(id int PRIMARY KEY,name text NOT NULL);
 CREATE TABLE game_rack_categories(id smallint PRIMARY KEY,name text,position smallint);
 CREATE TABLE game_rack_goods(item_id int PRIMARY KEY,category_id smallint,price int CHECK(price>0),quantity int,flags smallint,remain int,part text,active bool);
 CREATE TABLE game_rack_pet_goods(item_id int PRIMARY KEY,pet_id int);
 CREATE TABLE game_item_pet_eggs(item_id int PRIMARY KEY,pet_id int,name text,description text);
 CREATE TABLE gamedata.ov_petgrow(row_no int,pet_id int,name text,pet_circle int,horse_base_spd int,init_skills1_id int,init_skills2_id int,init_skills3_id int,init_skills4_id int,init_skills5_id int,init_skills6_id int);
 CREATE TABLE game_pet_profiles(pet_id int,initial_skill_names text[],forbidden_skill_names text[]);
 CREATE TABLE game_item_saddle_pets(item_id int,pet_id int);
 CREATE TABLE gamedata.ov_avatararm(row_no int,index int,category int,avatar_index int,cannot_attach int,avatar_pos int);
 CREATE TABLE game_shop_items(shop_id int,slot int,item_id int,PRIMARY KEY(shop_id,slot));
 CREATE TABLE game_shops(shop_id int PRIMARY KEY,stock_source_shop_id int,source_note text,allow_sell bool DEFAULT true,allow_repair bool DEFAULT true,repair_price_bp int DEFAULT 10000,special_repair_price_bp int DEFAULT 30000,special_repair_nianli int DEFAULT 0,source_kind text DEFAULT 'captured');
 CREATE TABLE account_preservation_probe(id int,balance bigint);
 CREATE TABLE owned_item_preservation_probe(uid int,item_id int);
 CREATE TABLE owned_pet_preservation_probe(uid int,pet_id int);
 CREATE TABLE purchase_preservation_probe(id int,item_id int,paid_price int);
 INSERT INTO account_preservation_probe VALUES(1,9999);
 INSERT INTO game_rack_categories VALUES(0,'变装',0),(1,'法宝',1),(2,'部位灵',2),(3,'宠物材料',3),(4,'功能道具',4),(5,'坐骑',5),(6,'龙系列',6),(7,'宠物蛋',7),(8,'强化材料',8);`);
 // Keep equipment identities separate to exercise both branches of the ring-stock query.
 await insert('game_items',fixture.items.filter(r=>!fixture.equipment_ids.includes(r.id)),['id int','name text']);
 await insert('game_equipment',fixture.items.filter(r=>fixture.equipment_ids.includes(r.id)),['id int','name text']);
 await insert('game_rack_goods',fixture.goods,['item_id int','category_id smallint','price int','quantity int','flags smallint','remain int','part text','active bool']);
 await insert('game_rack_pet_goods',fixture.pet_goods,['item_id int','pet_id int']);
 await insert('game_item_pet_eggs',fixture.eggs,['item_id int','pet_id int','name text','description text']);
 await insert('gamedata.ov_petgrow',fixture.pets,['row_no int','pet_id int','name text','pet_circle int','horse_base_spd int',...Array.from({length:6},(_,n)=>`init_skills${n+1}_id int`)]);
 await insert('game_pet_profiles',fixture.profiles,['pet_id int','initial_skill_names text[]','forbidden_skill_names text[]']);
 await insert('game_item_saddle_pets',fixture.saddle_pets,['item_id int','pet_id int']);
 await insert('gamedata.ov_avatararm',fixture.avatars,['row_no int','index int','category int','avatar_index int','cannot_attach int','avatar_pos int']);
 await db.exec(`INSERT INTO owned_item_preservation_probe SELECT item_id,item_id FROM game_rack_goods;
 INSERT INTO owned_pet_preservation_probe SELECT pet_id,pet_id FROM game_rack_pet_goods;
 INSERT INTO purchase_preservation_probe SELECT item_id,item_id,price FROM game_rack_goods;
 INSERT INTO game_shops(shop_id,stock_source_shop_id,source_note) VALUES(100,NULL,'native stock'),(101,100,'shared stock'),(102,NULL,'ring-only');
 UPDATE game_shops SET allow_sell=false WHERE shop_id IN(100,102);
 INSERT INTO game_shop_items VALUES(100,0,23624),(100,1,4001),(100,2,12090),(102,0,23625);`);
 const preservedTables=['game_pet_profiles','game_item_saddle_pets','game_items','game_equipment','game_rack_pet_goods','game_item_pet_eggs','gamedata.ov_petgrow','gamedata.ov_avatararm','game_shops','account_preservation_probe','owned_item_preservation_probe','owned_pet_preservation_probe','purchase_preservation_probe'];
 const snapshots=Object.fromEntries(await Promise.all(preservedTables.map(async t=>[t,await allRows(t)])));
 const before=await allRows('game_rack_goods');
 await db.exec(upgrade);const after=await allRows('game_rack_goods');const newGoods=new Map(after.map(r=>[r.item_id,r]));
 const removedSet=new Set(expectedRemoved.map(r=>r.item_id));const prices=new Map(report.repriced.map(r=>[r.item_id,r.new_price]));const parts=new Map(report.costumes.map(r=>[r.item_id,r]));
 for(const row of before){const want={...row};if(removedSet.has(row.item_id))want.active=false;if(prices.has(row.item_id)&&want.active)want.price=prices.get(row.item_id);if(parts.has(row.item_id)){want.category_id=parts.get(row.item_id).category_id;want.part=parts.get(row.item_id).part;}assert.deepEqual(newGoods.get(row.item_id),want);}
 assert.equal(after.filter(r=>r.active).length,612);pass('every one of 1,054 catalog rows exactly matches intended delta; 612 remain active');
 assert.deepEqual(await allRows('game_shop_items'),[{shop_id:100,slot:0,item_id:4001},{shop_id:100,slot:1,item_id:12090}].sort((a,b)=>JSON.stringify(a).localeCompare(JSON.stringify(b))));
 for(const t of preservedTables)assert.deepEqual(await allRows(t),snapshots[t],t);
 pass('only ring stock removed from NPC/shared shop; definitions, owned probes, balances, history and native metadata unchanged');
 const shopSQL=read('internal/data/shop.go').match(/q.Query\(ctx, `([^`]+)`/)[1];
 const shops=(await db.query(shopSQL)).rows;
 for(const id of [100,101])assert.deepEqual(shops.filter(r=>r.shop_id===id).map(r=>[r.slot,r.item_id]),[[0,4001],[1,12090]]);
 assert.equal(shops.find(r=>r.shop_id===102).slot,null);
 pass('verbatim NPC loader sees contiguous direct/shared stock and an empty ring-only shop');
 for(const r of report.unpriced_tier_zero)assert.equal(newGoods.get(r.item_id).price,r.old_price);
 for(const id of [12356,12357])assert(newGoods.get(90000000+id).active);
 assert.equal(newGoods.get(3633).active,false);assert.equal(newGoods.get(3287).active,false);
 pass('11 tier-zero prices preserved; naturally rideable camels retained; both saddle-dependent egg routes retired');
 const slotParts={1:'面饰',2:'头盔',8:'服装',10:'背包',4:'武器',13:'武器'};
 for(const g of after.filter(r=>r.category_id===0)){const av=fixture.avatars.filter(r=>r.index===g.item_id);const slot=g.item_id===50125?1:av[0].category;assert.equal(g.part,slotParts[slot]);}
 assert.equal(newGoods.get(10626).category_id,4);
 pass('all 328 wearables classified by actual equipment slot; non-wearable 灵魂之证 moved out');
 const loader=read('internal/data/commerce.go');const sql=[...loader.matchAll(/q.Query\(ctx, `([^`]+)`/g)].map(r=>r[1]).find(s=>s.includes('FROM game_rack_goods'));
 const loaded=(await db.query(sql)).rows;assert.equal(loaded.length,612);assert(loaded.every(r=>!removedSet.has(r.item_id)));
 pass('verbatim production rack loader returns no retired listing and all 612 retained goods');
 await db.exec(upgrade);assert.deepEqual(await allRows('game_rack_goods'),after);pass('incremental replay is idempotent');
 // Separate init body executes with the initializer's transaction, same semantics.
 await db.exec('BEGIN;'+body+'COMMIT;');assert.deepEqual(await allRows('game_rack_goods'),after);pass('fresh-init body equals upgrade and replays without temp-table leakage');
 for(const [mutate,restore] of [
  ["UPDATE game_items SET name='changed' WHERE id=10436","UPDATE game_items SET name='降灵鞍-神牛' WHERE id=10436"],
  ["UPDATE gamedata.ov_petgrow SET pet_circle=99 WHERE pet_id=12356","UPDATE gamedata.ov_petgrow SET pet_circle=3 WHERE pet_id=12356"],
  ["UPDATE game_item_pet_eggs SET pet_id=12356 WHERE item_id=3287","UPDATE game_item_pet_eggs SET pet_id=11858 WHERE item_id=3287"],
  ["UPDATE game_rack_goods SET flags=4 WHERE item_id=90012356","UPDATE game_rack_goods SET flags=0 WHERE item_id=90012356"],
  ["UPDATE game_pet_profiles SET forbidden_skill_names=ARRAY[]::text[] WHERE pet_id=11857",`UPDATE game_pet_profiles SET forbidden_skill_names=ARRAY[${profiles.get(11857).forbidden_skill_names.map(x=>"'"+x+"'").join(",")}] WHERE pet_id=11857`],
  ["INSERT INTO gamedata.ov_avatararm(row_no,index,category) VALUES(1999999,3050,2)","DELETE FROM gamedata.ov_avatararm WHERE row_no=1999999"],
  ["INSERT INTO game_rack_goods VALUES(1999999999,0,99,1,0,-1,'',true)","DELETE FROM game_rack_goods WHERE item_id=1999999999"]
 ]){
  await db.exec(mutate);const unchanged=await allRows('game_rack_goods');await assert.rejects(db.exec(upgrade),/rack restore/);await db.exec('ROLLBACK');assert.deepEqual(await allRows('game_rack_goods'),unchanged);await db.exec(restore);
 }
 pass('renamed item, changed egg binding/tier/currency/saddle requirement and conflicting/unaudited costume fail atomically');
 // Hidden/nonstandard promotions and a newly listed native ring variant are included.
 const extra=fixture.items.find(r=>r.name.includes('戒灵')&&!newGoods.has(r.id));assert(extra);
 await db.query("INSERT INTO game_rack_goods VALUES($1,255,1,1,3,-1,'',true)",[extra.id]);
 await db.query('INSERT INTO game_shop_items VALUES(100,3,$1)',[extra.id]);await db.exec(upgrade);
 assert.equal((await db.query('SELECT active FROM game_rack_goods WHERE item_id=$1',[extra.id])).rows[0].active,false);
 assert.equal((await db.query('SELECT count(*)::int AS n FROM game_shop_items WHERE item_id=$1',[extra.id])).rows[0].n,0);
 pass('all ring-spirit shop listings, including hidden promotional and non-rack variants, are excluded');
 console.log(`PASS ${checks} focused PostgreSQL catalog contracts`);
}catch(e){console.error("FAIL",e.message,e.detail||"");process.exitCode=1;}finally{await db.close()}

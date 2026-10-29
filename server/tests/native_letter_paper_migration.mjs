// Executes the SQL in isolated in-memory PostgreSQL WASM; never connects to a live database.
import fs from 'node:fs';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {PGlite}=require(process.env.PGLITE_MODULE || '@electric-sql/pglite');
const sql=fs.readFileSync(new URL('../migrations/upgrades/000128_20261029_native_letter_paper.sql',import.meta.url),'utf8');
const init=fs.readFileSync(new URL('../migrations/000000_init-128.sql',import.meta.url),'utf8');
assert.equal(sql.trim(),'BEGIN;\n'+init.trim()+'\n\nCOMMIT;');
const fixture=`CREATE TABLE game_items(id INT PRIMARY KEY,name TEXT,price BIGINT);
CREATE TABLE map_npcs(map_file TEXT,npc_id INT,name TEXT,sell_list INT);
CREATE TABLE game_shops(shop_id INT PRIMARY KEY,stock_source_shop_id INT,source_note TEXT NOT NULL DEFAULT 'existing',source_kind TEXT DEFAULT 'captured');
CREATE TABLE game_shop_items(shop_id INT REFERENCES game_shops,slot INT CHECK(slot>=0 AND slot<120),item_id INT,PRIMARY KEY(shop_id,slot),UNIQUE(shop_id,item_id));
INSERT INTO game_items VALUES(3309,'信纸',1000),(3310,'包袱',2000),(3380,'遗忘水晶',5000);
INSERT INTO map_npcs VALUES('qw0007',52,'鬼谷先生',1002),('qw0019',24,'无妄老人',1302),('qw0032',39,'白羽书仙萧洞玄',1402),('qw0058',47,'先知风涯',1502),('qw0154',24,'技能大师智羽',1902);
INSERT INTO game_shops(shop_id,stock_source_shop_id) VALUES(1001,NULL),(1002,NULL),(1302,1002),(1402,1002),(1502,1002),(1902,1002),(9999,NULL);
INSERT INTO game_shop_items VALUES(1002,0,3380),(1002,1,3310),(9999,0,3380);`;
const snap=async(db)=>JSON.stringify((await db.query(`SELECT jsonb_build_array((SELECT jsonb_agg(t ORDER BY shop_id) FROM game_shops t),(SELECT jsonb_agg(t ORDER BY shop_id,slot) FROM game_shop_items t),(SELECT jsonb_agg(t ORDER BY id) FROM game_items t)) AS data`)).rows);
const mutations={normal:'',existing:"INSERT INTO game_shop_items VALUES(1002,2,3309);",custom_price:'UPDATE game_items SET price=1234 WHERE id=3309;',missing_item:'DELETE FROM game_items WHERE id=3309;',bad_price:'UPDATE game_items SET price=0 WHERE id=3309;',bad_alias:'UPDATE game_shops SET stock_source_shop_id=1001 WHERE shop_id=1902;',renamed_npc:"UPDATE map_npcs SET name='other' WHERE sell_list=1002;",wrong_map:"UPDATE map_npcs SET map_file='pw0007' WHERE sell_list=1002;",local_alias_stock:'INSERT INTO game_shop_items VALUES(1302,0,3309);',gapped_stock:'UPDATE game_shop_items SET slot=3 WHERE shop_id=1002 AND slot=1;',full_stock:'INSERT INTO game_shop_items SELECT 1002,n,100000+n FROM generate_series(2,119) n;'};
let checks=0;
for(const [name,mutation] of Object.entries(mutations)){
 const db=new PGlite();
 try{
  await db.exec(fixture);if(mutation)await db.exec(mutation);const before=await snap(db);
  if(['normal','existing','custom_price'].includes(name)){
   await db.exec(sql);const once=await snap(db);await db.exec(sql);assert.equal(await snap(db),once);checks++;
   assert.deepEqual((await db.query('SELECT slot,item_id FROM game_shop_items WHERE shop_id=1002 ORDER BY slot')).rows,[{slot:0,item_id:3380},{slot:1,item_id:3310},{slot:2,item_id:3309}]);checks++;
   assert.equal((await db.query('SELECT price FROM game_items WHERE id=3309')).rows[0].price,name==='custom_price'?1234:1000);checks++;
   assert.equal((await db.query('SELECT count(*)::INT n FROM game_shops s JOIN game_shop_items i ON i.shop_id=COALESCE(s.stock_source_shop_id,s.shop_id) WHERE s.shop_id IN(1002,1302,1402,1502,1902) AND i.item_id=3309')).rows[0].n,5);checks++;
   assert.equal((await db.query('SELECT source_note FROM game_shops WHERE shop_id=9999')).rows[0].source_note,'existing');checks++;
  }else{
   await assert.rejects(db.exec(sql),/letter paper/);await db.exec('ROLLBACK;');assert.equal(await snap(db),before);checks++;
  }
  console.log('PASS',name);
 }finally{await db.close();}
}
console.log(`PASS ${checks} PostgreSQL-WASM migration assertions; no live database or external PostgreSQL server used`);

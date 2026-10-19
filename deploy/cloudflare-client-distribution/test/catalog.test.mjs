import test from 'node:test';
import assert from 'node:assert/strict';
import {queryCatalog} from '../site/fantasy-db/assets/public-catalog.mjs';
import {item} from './fixtures.mjs';
const snapshot={items:[item(),item({kind:'equipment',id:2,name:'长剑',typeCode:2,level:20,price:5000,sex:1,professions:['战士'],stackable:false,maxAttack:99,stats:{attack:99,str:4}}),item({id:3,name:'药水',description:'恢复生命',price:200})]};
const query=s=>queryCatalog(snapshot,new URLSearchParams(s));
test('name, description, ID, category, profession, sex, stats and flags use snapshot',()=>{
  for(const [q,ids] of [['q=长剑',[2]],['q=恢复',[3]],['q=2',[2]],['kind=item',[1,3]],['category=equipment:2',[2]],['profession=战士',[2]],['sex=2',[]],['stat=str&minStat=3',[2]],['stackable=0',[2]],['minPrice=101&maxPrice=1000',[3]],['minLevel=1',[2]]]) assert.deepEqual(query(q).items.map(i=>i.id),ids,q);
});
test('pagination, repeated sorts and invalid filters do not mutate snapshot',()=>{
  assert.deepEqual(query('sort=price-desc&pageSize=1&page=2').items.map(i=>i.id),[3]);
  assert.deepEqual(query('sort=attack-desc').items.map(i=>i.id),[2,1,3]);
  assert.deepEqual(query('').items.map(i=>i.id),[1,2,3]);
  for(const q of ['page=0','pageSize=999','minLevel=10&maxLevel=5','kind=npc','sex=9','stat=unknown','tradable=true','sort=bad','minStat=-1']) assert.throws(()=>query(q));
});

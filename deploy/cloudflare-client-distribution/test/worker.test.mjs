import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile,rm} from 'node:fs/promises';
import worker from '../src/worker.mjs';
import {parseJSON,validateDispatch,validateCurrent,clientFilename} from '../site/shared/contracts.mjs';
import {fixture} from './fixtures.mjs';
import {prepareRelease} from '../tools/release.mjs';
const req = (path,method='GET')=>new Request('https://download.example.invalid'+path,{method});
test('same current powers exact dispatch and release, with no-store GET/HEAD',async t=>{
  const f = await fixture(); t.after(()=>rm(f.root,{recursive:true,force:true})); const {current}=await prepareRelease(f.configFile,f.stage);
  let gets=0; const env = {RELEASES:{async get(key){assert.equal(key,'current.json');gets++;const text=JSON.stringify(current);return {size:text.length,text:async()=>text};}},ASSETS:{fetch(){throw new Error('not allowed');}}};
  const dispatch = await worker.fetch(req('/dispatch.json'),env); assert.deepEqual(await dispatch.json(),current.dispatch); assert.equal(dispatch.headers.get('cache-control'),'no-store');
  const release = await worker.fetch(req('/release.json'),env); assert.deepEqual(await release.json(),current);
  const head=await worker.fetch(req('/dispatch.json','HEAD'),env); assert.equal(await head.text(),''); assert.equal(head.headers.get('cache-control'),'no-store');assert.equal(gets,3);
});
test('missing/invalid/oversized release returns 503 without exposing configuration',async()=>{
  for(const env of [{},{RELEASES:{get:async()=>null}},{RELEASES:{get:async()=>({size:999999,text:async()=>'{secret}'})}},{RELEASES:{get:async()=>({size:4,text:async()=>'null'})}}]) {
    const r=await worker.fetch(req('/dispatch.json'),env); assert.equal(r.status,503); assert.equal(await r.text(),'{"error":"No verified release is available"}\n');
  }
});
test('no public GM, proxy, arbitrary bucket or writes; static UI receives security headers',async()=>{
  let staticCalls=0;
  const env={ASSETS:{fetch:async()=>{staticCalls++;return new Response('ui');}}};
  for(const path of ['/api/gm/accounts','/api/catalog/items','/current.json','/resources/a.pkg','/download/client.apk/../../api/gm/accounts']) assert.equal((await worker.fetch(req(path),env)).status,404);
  for(const path of ['/dispatch.json','/release.json','/fantasy-db/']) assert.equal((await worker.fetch(req(path,'POST'),env)).status,405);
  assert.equal(staticCalls,0); const r=await worker.fetch(req('/fantasy-db/'),env);assert.equal(r.headers.get('x-frame-options'),'DENY');assert.equal(staticCalls,1);
});
test('strict JSON and exact version-map contract retain empty-map authoritative semantics',()=>{
  for(const text of ['{"update":1,"update":2}','{"a":{"x":1,"x":2}}','{"a":1} {}']) assert.throws(()=>parseJSON(text));
  assert.deepEqual(parseJSON('{"a":[1,{"b":"\\\""}]}'),{a:[1,{b:'"'}]});
  assert.doesNotThrow(()=>validateDispatch({servers:[],update:{schemaVersion:1,resourceManifests:{android:{}}}}));
  for(const map of [null,{android:null},{android:{'01.0.0':{}}}]) assert.throws(()=>validateDispatch({servers:[],update:{schemaVersion:1,resourceManifests:map}}));
});
test('public copies contain no account management API or live relation fetch',async()=>{
  const app=await readFile(new URL('../site/fantasy-db/assets/app.js',import.meta.url),'utf8'), gm=await readFile(new URL('../site/fantasy-db/assets/gm-tools.js',import.meta.url),'utf8'), html=await readFile(new URL('../site/fantasy-db/index.html',import.meta.url),'utf8');
  for(const text of [app,gm,html]) {assert.ok(!text.includes('/api/gm'));assert.ok(!text.includes('account-panel'));assert.ok(!text.includes('/api/catalog/relations'));}
  assert.ok(gm.includes('function buildCommand')); assert.ok(app.includes('itemTooltip'));
});

test('client filename and 8 GiB download contract match platform consumers',()=>{
  for(const [p,name] of [['android','app.apk'],['windows','app.exe'],['windows','app.zip'],['macos','app.dmg'],['macos','app.zip'],['linux','app.tar.gz']])assert.doesNotThrow(()=>clientFilename(p,'https://assets.example.invalid/'+name));
  for(const [p,name] of [['android','app.exe'],['android','app%20.apk'],['windows','app.apk'],['macos','.zip'],['macos','app/']])assert.throws(()=>clientFilename(p,'https://assets.example.invalid/'+name));
  const client={version:'1.0.0',artifactUrl:'https://assets.example.invalid/app.apk',sha256:'a'.repeat(64),sizeBytes:8*1024*1024*1024+1};
  assert.throws(()=>validateDispatch({servers:[],update:{schemaVersion:1,resourceManifests:{},clients:{android:client}}}),/sizeBytes/);
});

test('catalog projection rejects unknown private fields even with valid JSON',async()=>{
  const {validateCatalog}=await import('../site/shared/contracts.mjs');
  const {item,hash}=await import('./fixtures.mjs');
  const snapshot={schemaVersion:1,contentRevision:'sha256:'+hash('fixture'),generatedAt:'2026-10-19T00:00:00Z',items:[item()],meta:{itemCount:1,equipmentCount:0,total:1,categories:[],maxLevel:0,npcCount:0,taskCount:0,skillCount:0,maps:[]}};
  const index={schemaVersion:1,contentRevision:snapshot.contentRevision,generatedAt:snapshot.generatedAt,sha256:hash('snapshot'),snapshot:'catalog-'+hash('snapshot')+'.json',bytes:123,counts:{item:1,equipment:0,total:1}};
  assert.doesNotThrow(()=>validateCatalog(index,snapshot));snapshot.items[0].accountId=123;assert.throws(()=>validateCatalog(index,snapshot),/unknown field accountId/);
});

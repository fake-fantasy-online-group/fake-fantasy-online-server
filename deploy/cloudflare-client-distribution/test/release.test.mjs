import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile,writeFile,mkdir,rm} from 'node:fs/promises';
import path from 'node:path';
import {prepareRelease,verifyLocal,verifyRemote,publishRelease,verifyResponse} from '../tools/release.mjs';
import {parseResourceManifest} from '../tools/resource-manifest.mjs';
import {fixture,FakeStore,publicFetcher,jsonBytes,hash} from './fixtures.mjs';
async function setup(t) { const f = await fixture(); t.after(()=>rm(f.root,{recursive:true,force:true})); return f; }
test('prepare stages exact bytes, pins all platforms, preserves servers, verifies catalog and resource closure',async t=>{
  const f = await setup(t), result = await prepareRelease(f.configFile,f.stage);
  assert.deepEqual(result.current.dispatch.servers,f.dispatch.servers);
  assert.equal(result.current.dispatch.update.schemaVersion,1);
  assert.equal(result.plan.files.length,9);
  for (const platform of ['android','windows','macos']) assert.equal(result.current.dispatch.update.resourceManifests[platform]['1.0.0'].sha256.length,64);
  await verifyLocal(f.stage); await verifyRemote(result.current,result.plan,publicFetcher(f.blobs));
  assert.equal(await readFile(f.configFile,'utf8'),jsonBytes(f.config).toString());
});
test('publish verifies every immutable object before one conditional current switch',async t=>{
  const f = await setup(t); await prepareRelease(f.configFile,f.stage);
  const artifacts = new FakeStore(), metadata = new FakeStore(); let reads=0;
  const fetcher = async (...args)=>{reads++; return publicFetcher(f.blobs)(...args);};
  metadata.beforePut = ()=>assert.equal(reads,9);
  const result = await publishRelease(f.stage,{artifacts,metadata,expectedETag:'absent',confirmPublish:true,fetcher});
  assert.equal(result.files,9); assert.equal(metadata.puts.length,1); assert.equal(metadata.puts[0].options.ifNoneMatch,'*');
  assert.ok(artifacts.puts.every(p=>p.options.ifNoneMatch === '*' && p.options.cacheControl.includes('immutable')));
  const current = JSON.parse(metadata.objects.get('current.json').bytes); assert.equal(current.releaseId,'synthetic-test');
  await publishRelease(f.stage,{artifacts,metadata,expectedETag:result.etag,confirmPublish:true,fetcher:publicFetcher(f.blobs)});
  assert.equal(artifacts.puts.length,9); assert.equal(metadata.puts.length,2);
});
test('confirmation and stale ETag refuse uploads',async t=>{
  const f = await setup(t); await prepareRelease(f.configFile,f.stage); const artifacts = new FakeStore(),metadata = new FakeStore();
  await assert.rejects(publishRelease(f.stage,{artifacts,metadata,expectedETag:'absent',fetcher:publicFetcher(f.blobs)}),/confirm-publish/);
  await assert.rejects(publishRelease(f.stage,{artifacts,metadata,expectedETag:'"stale"',confirmPublish:true,fetcher:publicFetcher(f.blobs)}),/ETag changed/);
  assert.equal(artifacts.puts.length,0); assert.equal(metadata.puts.length,0);
});
test('remote corruption, CORS, compression or interrupted GET never switches current',async t=>{
  for (const mode of ['corruption','cors','gzip','interrupted']) {
    const f = await fixture(); t.after(()=>rm(f.root,{recursive:true,force:true})); await prepareRelease(f.configFile,f.stage);
    const artifacts = new FakeStore(),metadata = new FakeStore();
    const fetcher = mode === 'corruption' ? async ()=>new Response('wrong') : mode === 'interrupted' ? async ()=>{throw new Error('network interrupted');} : publicFetcher(f.blobs,mode === 'cors' ? {'access-control-allow-origin':'https://elsewhere.example.invalid'} : {'content-encoding':'gzip'});
    await assert.rejects(publishRelease(f.stage,{artifacts,metadata,expectedETag:'absent',confirmPublish:true,fetcher})); assert.equal(metadata.puts.length,0);
  }
});
test('competing publisher wins without being overwritten',async t=>{
  const f = await setup(t); await prepareRelease(f.configFile,f.stage); const artifacts = new FakeStore(),metadata = new FakeStore();
  metadata.beforePut = ()=>metadata.objects.set('current.json',{bytes:Buffer.from('other release'),etag:'"other"'});
  await assert.rejects(publishRelease(f.stage,{artifacts,metadata,expectedETag:'absent',confirmPublish:true,fetcher:publicFetcher(f.blobs)}),/412/);
  assert.equal(metadata.objects.get('current.json').bytes.toString(),'other release');
});
test('immutable key collision fails before pointer mutation',async t=>{
  const f = await setup(t); const {plan}=await prepareRelease(f.configFile,f.stage), artifacts = new FakeStore(),metadata = new FakeStore();
  artifacts.objects.set(plan.files[0].key,{bytes:Buffer.from('wrong'),etag:'"x"'});
  await assert.rejects(publishRelease(f.stage,{artifacts,metadata,expectedETag:'absent',confirmPublish:true,fetcher:publicFetcher(f.blobs)})); assert.equal(metadata.puts.length,0);
});
test('local changed bytes, missing resource and extra files fail closed',async t=>{
  const f = await setup(t); const {plan} = await prepareRelease(f.configFile,f.stage);
  await writeFile(path.join(f.stage,plan.files[0].localPath),'changed'); await assert.rejects(verifyLocal(f.stage),/mismatch/);
  const missing = await fixture(); t.after(()=>rm(missing.root,{recursive:true,force:true})); missing.config.files.shift(); await writeFile(missing.configFile,jsonBytes(missing.config));
  await assert.rejects(prepareRelease(missing.configFile,missing.stage),/missing local artifact/);
  const extra = await fixture(); t.after(()=>rm(extra.root,{recursive:true,force:true})); const secret = path.join(extra.root,'never-upload.txt'); await writeFile(secret,'not a real secret'); extra.config.files.push({path:secret,key:'releases/synthetic-test/extra.txt'}); await writeFile(extra.configFile,jsonBytes(extra.config));
  await assert.rejects(prepareRelease(extra.configFile,extra.stage),/unreferenced/);
});
test('wrong @client, path traversal, executable resources and duplicate JSON are rejected',async t=>{
  const f = await setup(t), entry=f.config.files.find(x=>x.key.endsWith('android.tsv'));
  await writeFile(entry.path,(await readFile(entry.path,'utf8')).replace('@client\tandroid','@client\twindows'));
  await assert.rejects(prepareRelease(f.configFile,f.stage),/@client/);
  for (const source of ['../test.pkg','data/CON.pkg','data/a%20.pkg','data/a.part','data/a\\b.pkg']) assert.throws(()=>parseResourceManifest(Buffer.from(`FFO-MOBILE-1\n@client\tandroid\t1.0.0\n${source}\tdata/test.pkg\t1\t${hash('x')}\n`),'https://assets.example.invalid/set/m.tsv'));
  assert.throws(()=>parseResourceManifest(Buffer.from(`FFO-MOBILE-1\n@client\tandroid\t1.0.0\ndata/file.bin\tgameassets/file.dll\t1\t${hash('x')}\n`),'https://assets.example.invalid/set/m.tsv'),/unsupported/);
});
test('response verifier checks content hash rather than trusting ETag',async()=>{
  await assert.rejects(verifyResponse(new Response('abc',{headers:{etag:hash('abc')}}),{sha256:hash('abd'),sizeBytes:3}),/mismatch/);
});

test('publisher rejects source executables and incomplete dispatch even with valid output hashes',async t=>{
  assert.throws(()=>parseResourceManifest(Buffer.from(`FFO-MOBILE-1\n@client\tandroid\t1.0.0\npayload.exe\tdata/main.pkg\t1\t${hash('x')}\n`),'https://assets.example.invalid/set/m.tsv'),/passive source/);
  for (const change of [d=>{d.servers=[];},d=>{delete d.update;}]) {
    const f=await fixture();t.after(()=>rm(f.root,{recursive:true,force:true}));change(f.dispatch);await writeFile(f.config.dispatchFile,jsonBytes(f.dispatch));
    await assert.rejects(prepareRelease(f.configFile,f.stage),/requires servers and update/);
  }
});

test('prepare rejects a wrongly named platform package even when bytes are locally valid',async t=>{
  const f=await setup(t), old=f.dispatch.update.clients.android.artifactUrl, next=old.replace('.apk','.exe');
  f.dispatch.update.clients.android.artifactUrl=next;f.config.files.find(x=>x.key.endsWith('android.apk')).key=f.config.files.find(x=>x.key.endsWith('android.apk')).key.replace('.apk','.exe');
  await writeFile(f.config.dispatchFile,jsonBytes(f.dispatch));await writeFile(f.configFile,jsonBytes(f.config));
  await assert.rejects(prepareRelease(f.configFile,f.stage),/unsupported client package extension/);
});


test('scoped passive overlays match client source and destination boundaries',()=>{
  const accepted = ['gameassets/respatch/effect/龙霸.eff','gameassets/respatch/magic/magic/magic9990.gso'];
  const parse = (source,destination,platform='android') => parseResourceManifest(Buffer.from(`FFO-MOBILE-1\n@client\t${platform}\t1.0.0\n${source}\t${destination}\t1\t${hash('x')}\n`),'https://assets.example.invalid/set/m.tsv');
  for (const platform of ['android','windows','macos']) for (const value of accepted) {
    const result = parse(value,value,platform);
    assert.equal(result.entries[0].destination,value);
    assert.equal(result.entries[0].url,'https://assets.example.invalid/set/'+value.split('/').map(encodeURIComponent).join('/'));
  }
  for (const value of ['gameassets/respatch/item/item/item5002.gso','gameassets/rawmap/world.gso','ReferenceClient/rawmap/world.gso',
    'gameassets/respatch/other.gso','gameassets/respatch/effect/a.gso','gameassets/respatch/magic/magic/a.eff',
    'gameassets/respatch/effect/sub/a.eff','gameassets/respatch/magic/magic/sub/a.gso',
    'gameassets/respatch/effect/a.dll','gameassets/respatch/effect/a.lua','AddOns/a.eff',
    'gameassets/Respatch/effect/a.eff','ReferenceClient/respatch/effect/a.eff']) {
    assert.throws(()=>parse(value,accepted[0]),/passive source/);
    assert.throws(()=>parse(accepted[0],value),/unsupported resource destination/);
  }
  assert.throws(()=>parse('a.json','Music/mapmusic.json'),/unsupported resource destination/);
});

test('overlay release prepares, verifies and publishes complete Unicode resource closure',async t=>{
  const f = await setup(t), paths = ['gameassets/respatch/effect/龙霸.eff','gameassets/respatch/magic/magic/magic9990.gso'];
  const payload = Buffer.from('synthetic passive overlay\n');
  for (const relative of paths) {
    const local = path.join(f.input,'resources',relative), key = `releases/synthetic-test/resources/${relative}`;
    await mkdir(path.dirname(local),{recursive:true}); await writeFile(local,payload);
    f.config.files.push({path:local,key});
    f.blobs.set('https://assets.example.invalid/'+key.split('/').map(encodeURIComponent).join('/'),payload);
  }
  for (const platform of ['android','windows','macos']) {
    const entry = f.config.files.find(x=>x.key.endsWith(`/${platform}.tsv`));
    const bytes = Buffer.from(await readFile(entry.path,'utf8')+paths.map(p=>`${p}\t${p}\t${payload.length}\t${hash(payload)}\n`).join(''));
    await writeFile(entry.path,bytes); f.blobs.set('https://assets.example.invalid/'+entry.key,bytes);
  }
  await writeFile(f.configFile,jsonBytes(f.config));
  const {current,plan} = await prepareRelease(f.configFile,f.stage);
  assert.equal(plan.files.length,11);
  await verifyLocal(f.stage); await verifyRemote(current,plan,publicFetcher(f.blobs));
  const artifacts = new FakeStore(), metadata = new FakeStore();
  const result = await publishRelease(f.stage,{artifacts,metadata,expectedETag:'absent',confirmPublish:true,fetcher:publicFetcher(f.blobs)});
  assert.equal(result.files,11); assert.equal(metadata.puts.length,1);
});

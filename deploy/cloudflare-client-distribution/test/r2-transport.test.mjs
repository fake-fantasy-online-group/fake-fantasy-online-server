import test from 'node:test';
import assert from 'node:assert/strict';
import {R2Store,sign} from '../tools/r2-s3.mjs';
const options={accountId:'a'.repeat(32),bucket:'synthetic-bucket',accessKey:'SYNTHETIC_ACCESS',secretKey:'synthetic-test-secret'};
test('R2-only transport signs the condition and checksum, rejects redirects, and has no bucket management',async()=>{
  let recorded;
  const store=new R2Store({...options,fetcher:async(url,args)=>{recorded={url,args};return new Response(null,{headers:{etag:'"synthetic"'}});}});
  await store.put('releases/test/file.json',{body:'{}',sizeBytes:2,sha256:'a'.repeat(64),ifNoneMatch:'*',cacheControl:'public, max-age=31536000, immutable',contentType:'application/json'});
  assert.equal(recorded.url.origin,'https://'+'a'.repeat(32)+'.r2.cloudflarestorage.com');assert.equal(recorded.url.pathname,'/synthetic-bucket/releases/test/file.json');
  assert.equal(recorded.args.redirect,'error');assert.equal(recorded.args.headers['if-none-match'],'*');assert.equal(recorded.args.headers['x-amz-content-sha256'],'a'.repeat(64));
  assert.ok(recorded.args.headers.authorization.includes('if-none-match'));assert.ok(!recorded.args.headers.authorization.includes(options.secretKey));
  assert.equal(typeof store.delete,'undefined');assert.equal(typeof store.createBucket,'undefined');
});
test('SigV4 signature is deterministic at fixed time; changing ETag changes signature',()=>{
  const input={method:'PUT',url:new URL('https://'+'a'.repeat(32)+'.r2.cloudflarestorage.com/synthetic-bucket/current.json'),headers:{'x-amz-content-sha256':'a'.repeat(64),'if-match':'"old"'},accessKey:options.accessKey,secretKey:options.secretKey,date:new Date('2026-10-19T00:00:00Z')};
  const signed=sign(input);assert.equal(signed['x-amz-date'],'20261019T000000Z');assert.equal(signed.authorization,sign(input).authorization);assert.notEqual(signed.authorization,sign({...input,headers:{...input.headers,'if-match':'"new"'}}).authorization);
});
test('R2 412 and network failure are reported once without retry or overwrite',async()=>{
  let calls=0;
  const store=new R2Store({...options,fetcher:async()=>{calls++;return new Response('conflict',{status:412});}});
  await assert.rejects(store.put('current.json',{ifMatch:'"old"'}),/conditional write conflict/);assert.equal(calls,1);
  store.fetcher=async()=>{calls++;throw new Error('connection lost, outcome unknown');};
  await assert.rejects(store.put('current.json',{ifMatch:'"old"'}),/outcome unknown/);assert.equal(calls,2);
});
test('only missing reads return null; other provider errors are failures',async()=>{
  const store=new R2Store({...options,fetcher:async()=>new Response(null,{status:404})});assert.equal(await store.get('x'),null);
  await assert.rejects(store.put('x',{}),/R2 PUT failed/);
  for(const opts of [{...options,accountId:'https://elsewhere.invalid'},{...options,bucket:'../bucket'},{...options,secretKey:''}])assert.throws(()=>new R2Store(opts));
});

// Cross-language acceptance: supply an actual catalog-export manifest generated
// from an approved synthetic database fixture, never a production data dump.
import assert from 'node:assert/strict';
import path from 'node:path';
import {readFile,writeFile,rm} from 'node:fs/promises';
import {fixture,jsonBytes,publicFetcher,base} from './fixtures.mjs';
import {prepareRelease,verifyLocal,verifyRemote} from '../tools/release.mjs';
import {loadSnapshot,queryCatalog} from '../site/fantasy-db/assets/public-catalog.mjs';
const file=process.argv[2];assert.ok(file,'Pass the absolute synthetic Go-export manifest.json path');
const index=JSON.parse(await readFile(file,'utf8'));
assert.equal(index.sourceRevision,'synthetic-fixture-only','this acceptance command only uses the labelled synthetic fixture');
const f=await fixture({catalog:false});
try {
  for(const name of ['manifest.json',index.snapshot]) {
    const source=path.join(path.dirname(file),name),key=`releases/${f.config.releaseId}/catalog/${name}`;
    f.config.files.push({path:source,key});f.blobs.set(base+key,await readFile(source));
  }
  f.config.catalogManifestUrl=`${base}releases/${f.config.releaseId}/catalog/manifest.json`;
  await writeFile(f.configFile,jsonBytes(f.config));
  const {current,plan}=await prepareRelease(f.configFile,f.stage);await verifyLocal(f.stage);await verifyRemote(current,plan,publicFetcher(f.blobs));
  const fetcher=async(url,options)=>url==='/release.json'?new Response(jsonBytes(current)):publicFetcher(f.blobs)(url,options);
  const snapshot=await loadSnapshot(fetcher);assert.equal(snapshot.contentRevision,index.contentRevision);assert.equal(snapshot.items.length,index.counts.total);
  const result=queryCatalog(snapshot,new URLSearchParams('pageSize=60'));assert.equal(result.total,2);
  console.log(`Go → Node → browser-data contract PASS: ${result.total} synthetic public rows, raw-byte SHA-256 ${index.sha256}, revision ${snapshot.contentRevision}. No external network or upload.`);
}finally{await rm(f.root,{recursive:true,force:true});}

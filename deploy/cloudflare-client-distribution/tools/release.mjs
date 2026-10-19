#!/usr/bin/env node
import {createHash} from 'node:crypto';
import {createReadStream,constants} from 'node:fs';
import {readFile,writeFile,mkdir,copyFile,lstat,readdir,realpath} from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {parseArgs} from 'node:util';
import {assert,fields,parseJSON,validateCurrent,validateCatalog,httpsURL,MAX_CURRENT_BYTES,MAX_MANIFEST_BYTES} from '../site/shared/contracts.mjs';
import {safePath,parseResourceManifest} from './resource-manifest.mjs';
import {R2Store} from './r2-s3.mjs';
const here = path.dirname(fileURLToPath(import.meta.url));
const repository = path.resolve(here,'../../..');
const IMMUTABLE = 'public, max-age=31536000, immutable';
const bytesOf = value => Buffer.from(JSON.stringify(value,null,2)+'\n');
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
async function readJSON(file,max = 16*1024*1024) {
  const stat = await lstat(file); assert(stat.isFile() && !stat.isSymbolicLink() && stat.size <= max, 'JSON input must be a bounded regular file');
  return parseJSON(await readFile(file,'utf8'));
}
async function outsideRepository(value) {
  const actual = await realpath(value);
  assert(actual !== repository && !actual.startsWith(repository + path.sep), 'private input/staging must be outside the Git repository');
  return actual;
}
export async function digestFile(file) {
  const stat = await lstat(file); assert(stat.isFile() && !stat.isSymbolicLink(), 'only materialized regular files are allowed');
  const hash = createHash('sha256'); let sizeBytes = 0;
  for await (const chunk of createReadStream(file)) { sizeBytes += chunk.length; hash.update(chunk); }
  assert(sizeBytes === stat.size, 'file changed during hashing');
  return {sha256:hash.digest('hex'),sizeBytes};
}
function same(actual, expected, label) {
  assert(actual.sha256 === expected.sha256 && actual.sizeBytes === expected.sizeBytes, `${label}: SHA-256 or size mismatch`);
}
function artifactURL(base,key) { return new URL(key.split('/').map(encodeURIComponent).join('/'),base).href; }
function mime(key) {
  if (key.endsWith('.json')) return 'application/json';
  if (key.endsWith('.tsv')) return 'text/tab-separated-values; charset=utf-8';
  if (key.endsWith('.apk')) return 'application/vnd.android.package-archive';
  return 'application/octet-stream';
}
async function walk(directory,prefix,out) {
  assert((await lstat(directory)).isDirectory() && !(await lstat(directory)).isSymbolicLink(), 'directories must be materialized');
  for (const item of await readdir(directory,{withFileTypes:true})) {
    assert(!item.isSymbolicLink(), 'symlinks are not allowed');
    const key = prefix + '/' + item.name, source = path.join(directory,item.name);
    if (item.isDirectory()) await walk(source,key,out);
    else { assert(item.isFile(), 'special files are not allowed'); out.push({path:source,key}); }
  }
}
function graph(current, files) {
  const map = new Map(files.map(f=>[f.url,f])), used = new Set();
  const requireFile = url => { const file = map.get(url); assert(file, `missing local artifact for ${url}`); used.add(url); return file; };
  return {map,used,requireFile};
}
export async function validateGraph(current,files,root) {
  validateCurrent(current);
  const {used,requireFile} = graph(current,files), parsed = new Map();
  const read = file => readFile(path.join(root,file.localPath));
  async function manifest(url,pin,platform,appVersion) {
    const file = requireFile(url); if (pin) same(file,pin,'manifest binding');
    assert(file.sizeBytes <= MAX_MANIFEST_BYTES,'manifest too large');
    let value = parsed.get(url);
    if (!value) {
      value = parseResourceManifest(await read(file),url); parsed.set(url,value);
      for (const entry of value.entries) same(requireFile(entry.url),entry,'resource payload');
    }
    if (platform) assert(value.platform === platform && value.appVersion === appVersion,'manifest @client does not match dispatch map');
    return value;
  }
  const u = current.dispatch.update;
  if (u) {
    assert(!u.hotupdateManifestUrl, 'hot-update code publication is outside this passive-resource workflow');
    if (u.resourceManifestUrl) {
      assert(u.resourceManifestSha256 && u.resourceManifestSizeBytes, 'published default manifest must be pinned');
      await manifest(u.resourceManifestUrl,{sha256:u.resourceManifestSha256,sizeBytes:u.resourceManifestSizeBytes});
    }
    for (const [platform,versions] of Object.entries(u.resourceManifests || {})) for (const [appVersion,binding] of Object.entries(versions)) await manifest(binding.manifestUrl,binding,platform,appVersion);
    for (const [platform,client] of Object.entries(u.clients || {})) {
      same(requireFile(client.artifactUrl),client,'client package');
      if (u.resourceManifests !== undefined) assert(u.resourceManifests[platform]?.[client.version], 'published client needs an exact matching resource map');
      else {
        const legacy = await manifest(u.resourceManifestUrl);
        assert(legacy.platform === platform && legacy.appVersion === client.version,'legacy manifest does not match downloadable client');
      }
    }
  }
  if (current.catalog) {
    const file = requireFile(current.catalog.manifestUrl); same(file,current.catalog,'catalog index');
    const index = parseJSON((await read(file)).toString('utf8'));
    assert(index.schemaVersion === 1 && /^catalog-[a-f0-9]{64}\.json$/.test(index.snapshot) && index.snapshot === `catalog-${index.sha256}.json`, 'invalid catalog snapshot filename');
    assert(Number.isSafeInteger(index.bytes) && index.bytes > 0 && index.bytes <= 64*1024*1024,'invalid catalog size');
    const snapshotFile = requireFile(new URL(index.snapshot,file.url).href);
    same(snapshotFile,{sha256:index.sha256,sizeBytes:index.bytes},'catalog snapshot');
    const snapshot = parseJSON((await read(snapshotFile)).toString('utf8'));
    validateCatalog(index,snapshot);
  }
  assert(used.size === files.length, 'unreferenced files found: refuse to upload extra/private files');
}
function pin(file,existing,label) {
  if (existing.sha256 !== undefined || existing.sizeBytes !== undefined) same(file,existing,label);
  return {sha256:file.sha256,sizeBytes:file.sizeBytes};
}
export async function prepareRelease(configFile,outDirectory,{defaultDispatchUrl} = {}) {
  await outsideRepository(configFile);
  const config = await readJSON(configFile);
  fields(config,['releaseId','defaultDispatchUrl','artifactBaseUrl','dispatchFile','catalogManifestUrl','files','directories'],'release config');
  if (defaultDispatchUrl !== undefined) assert(defaultDispatchUrl === config.defaultDispatchUrl, 'CLI default dispatch must match release configuration');
  httpsURL(config.defaultDispatchUrl); httpsURL(config.artifactBaseUrl);
  assert(config.artifactBaseUrl.endsWith('/'),'artifactBaseUrl must end with /');
  assert(/^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$/.test(config.releaseId),'invalid releaseId');
  assert(Array.isArray(config.files || []) && Array.isArray(config.directories || []),'invalid file list');
  const inputBase = path.dirname(configFile), dispatchFile = path.resolve(inputBase,config.dispatchFile);
  await outsideRepository(dispatchFile);
  const dispatch = await readJSON(dispatchFile,65536), inputs = [...(config.files || [])];
  for (const directory of config.directories || []) {
    fields(directory,['path','keyPrefix'],'directory');
    const directoryPath = path.resolve(inputBase,directory.path); await outsideRepository(directoryPath);
    await walk(directoryPath,directory.keyPrefix,inputs);
  }
  // Never overwrite a prior staging directory. Partial staging is safe to inspect
  // and discard manually; nothing here touches Cloudflare or source files.
  await outsideRepository(path.dirname(path.resolve(outDirectory)));
  await mkdir(outDirectory,{recursive:false,mode:0o700});
  const seen = new Set(), files = [];
  for (const input of inputs) {
    fields(input,['path','key','contentType'],'file'); safePath(input.key);
    assert(input.key.startsWith(`releases/${config.releaseId}/`),'every artifact key must be inside this immutable release');
    assert(!seen.has(input.key.toLowerCase()),'duplicate/case-colliding artifact key'); seen.add(input.key.toLowerCase());
    const source = path.resolve(inputBase,input.path); await outsideRepository(source);
    const digest = await digestFile(source); assert(digest.sizeBytes > 0,'empty publication file');
    const localPath = 'objects/' + input.key, destination = path.join(outDirectory,localPath);
    await mkdir(path.dirname(destination),{recursive:true}); await copyFile(source,destination,constants.COPYFILE_EXCL);
    same(await digestFile(destination),digest,'staged input');
    const contentType = input.contentType || mime(input.key);
    assert(typeof contentType === 'string' && /^[a-zA-Z0-9/;.= _+-]+$/.test(contentType),'invalid content type');
    files.push({key:input.key,localPath,url:artifactURL(config.artifactBaseUrl,input.key),...digest,contentType});
  }
  const lookup = new Map(files.map(f=>[f.url,f]));
  const fileAt = url => { const f = lookup.get(url); assert(f,`missing file for ${url}`); return f; };
  const u = dispatch.update;
  if (u) {
    if (u.resourceManifestUrl) {
      const p = pin(fileAt(u.resourceManifestUrl),{sha256:u.resourceManifestSha256,sizeBytes:u.resourceManifestSizeBytes},'default manifest');
      u.resourceManifestSha256 = p.sha256; u.resourceManifestSizeBytes = p.sizeBytes;
    }
    if (u.resourceManifests) for (const versions of Object.values(u.resourceManifests)) for (const item of Object.values(versions)) Object.assign(item,pin(fileAt(item.manifestUrl),item,'mapped manifest'));
    if (u.clients) for (const item of Object.values(u.clients)) Object.assign(item,pin(fileAt(item.artifactUrl),item,'client'));
  }
  const current = {schemaVersion:1,releaseId:config.releaseId,defaultDispatchUrl:config.defaultDispatchUrl,artifactBaseUrl:config.artifactBaseUrl,dispatch};
  if (config.catalogManifestUrl) current.catalog = {manifestUrl:config.catalogManifestUrl,...pin(fileAt(config.catalogManifestUrl),{},'catalog')};
  await validateGraph(current,files,outDirectory);
  const currentBytes = bytesOf(current); assert(currentBytes.length <= MAX_CURRENT_BYTES,'current descriptor too large');
  await writeFile(path.join(outDirectory,'current.json'),currentBytes,{flag:'wx',mode:0o600});
  const plan = {schemaVersion:1,currentSha256:sha(currentBytes),files};
  await writeFile(path.join(outDirectory,'plan.json'),bytesOf(plan),{flag:'wx',mode:0o600});
  return {current,plan};
}
export async function verifyLocal(directory) {
  await outsideRepository(directory);
  const plan = await readJSON(path.join(directory,'plan.json'),256*1024*1024);
  fields(plan,['schemaVersion','currentSha256','files'],'plan'); assert(plan.schemaVersion === 1 && Array.isArray(plan.files),'invalid plan');
  const currentPath = path.join(directory,'current.json');
  const currentStat = await lstat(currentPath); assert(currentStat.isFile() && !currentStat.isSymbolicLink() && currentStat.size <= MAX_CURRENT_BYTES,'current descriptor must be a bounded regular file');
  const currentBytes = await readFile(currentPath); assert(currentBytes.length <= MAX_CURRENT_BYTES,'current descriptor too large');
  assert(sha(currentBytes) === plan.currentSha256,'staged current.json changed');
  const current = validateCurrent(parseJSON(currentBytes.toString('utf8'))), seen = new Set();
  for (const f of plan.files) {
    fields(f,['key','localPath','url','sha256','sizeBytes','contentType'],'plan file'); safePath(f.key);
    assert(f.key.startsWith(`releases/${current.releaseId}/`) && f.localPath === 'objects/' + f.key,'unsafe staged path');
    assert(f.url === artifactURL(current.artifactBaseUrl,f.key),'artifact URL changed');
    assert(!seen.has(f.key.toLowerCase()),'duplicate/case-colliding plan key'); seen.add(f.key.toLowerCase());
    const staged = path.join(directory,f.localPath), actual = await realpath(staged);
    assert(actual.startsWith((await realpath(directory)) + path.sep),'symlink escapes staging');
    same(await digestFile(staged),f,'local artifact');
  }
  await validateGraph(current,plan.files,directory);
  return {current,plan,currentBytes};
}
export async function verifyResponse(response,expected,{corsOrigin,immutable=false} = {}) {
  assert(response?.ok && response.status === 200,'artifact GET must return 200 without redirect');
  assert(!response.headers.get('content-encoding') || response.headers.get('content-encoding') === 'identity','encoded artifact rejected');
  if (immutable) assert((response.headers.get('cache-control') || '').includes('immutable'),'artifact must be immutable');
  if (corsOrigin) assert(['*',corsOrigin].includes(response.headers.get('access-control-allow-origin')),'R2 CORS must allow the website origin');
  const declared = response.headers.get('content-length');
  if (declared !== null) assert(Number(declared) === expected.sizeBytes,'artifact Content-Length mismatch');
  const hash = createHash('sha256'), reader = response.body?.getReader(); assert(reader,'artifact response has no body');
  let sizeBytes = 0;
  try { while (true) { const {value,done} = await reader.read(); if (done) break; sizeBytes += value.length; assert(sizeBytes <= expected.sizeBytes,'artifact larger than expected'); hash.update(value); } }
  catch(error) { await reader.cancel(); throw error; }
  same({sha256:hash.digest('hex'),sizeBytes},expected,'downloaded artifact');
}
export async function verifyRemote(current,plan,fetcher = fetch) {
  // Verify bytes from the exact client-facing origin, not HEAD, ETag, or S3 metadata.
  for (const file of plan.files) {
    const response = await fetcher(file.url,{headers:{'Accept-Encoding':'identity','Origin':new URL(current.defaultDispatchUrl).origin},cache:'no-store',redirect:'error',signal:AbortSignal.timeout(30*60*1000)});
    await verifyResponse(response,file,{corsOrigin:new URL(current.defaultDispatchUrl).origin,immutable:true});
  }
}
export async function publishRelease(directory,{artifacts,metadata,expectedETag,confirmPublish,fetcher = fetch}) {
  assert(confirmPublish === true,'publishing requires --confirm-publish');
  assert(expectedETag === 'absent' || /^"[^"\r\n]+"$/.test(expectedETag || ''),'pass --expected-etag absent or the exact quoted current ETag');
  const {current,plan,currentBytes} = await verifyLocal(directory);
  // Check the expected old state before any upload; the final conditional PUT
  // repeats this check atomically, so a competing publisher cannot be clobbered.
  const old = await metadata.head('current.json');
  assert(expectedETag === 'absent' ? !old : old?.headers.get('etag') === expectedETag,'current ETag changed; no upload performed');
  for (const file of plan.files) {
    const existing = await artifacts.get(file.key);
    if (existing) await verifyResponse(existing,file);
    else {
      assert(file.sizeBytes <= 5 * 1024 * 1024 * 1024, 'file exceeds single-PUT limit; use a separately approved immutable multipart upload first');
      await artifacts.put(file.key,{path:path.join(directory,file.localPath),...file,ifNoneMatch:'*',cacheControl:IMMUTABLE});
      await verifyResponse(await artifacts.get(file.key),file);
    }
  }
  await verifyRemote(current,plan,fetcher);
  // Recheck staging after network waits; do not switch the pointer after a local edit.
  const finalCheck = await verifyLocal(directory);
  assert(finalCheck.currentBytes.equals(currentBytes) && JSON.stringify(finalCheck.plan) === JSON.stringify(plan), 'staging changed during publication');
  const result = await metadata.put('current.json',{body:currentBytes,sha256:sha(currentBytes),sizeBytes:currentBytes.length,contentType:'application/json',cacheControl:'no-store',...(expectedETag === 'absent' ? {ifNoneMatch:'*'} : {ifMatch:expectedETag})});
  const newETag = result.headers.get('etag'); await result.body?.cancel();
  const downloaded = await metadata.get('current.json');
  assert(newETag && downloaded?.headers.get('etag') === newETag,'current changed after publish; inspect state before retrying');
  await verifyResponse(downloaded,{sha256:sha(currentBytes),sizeBytes:currentBytes.length});
  return {releaseId:current.releaseId,etag:newETag,files:plan.files.length};
}
function usage() {
  console.log(`Local-first Cloudflare distribution CLI (no implicit uploads):
  node tools/release.mjs prepare --config /private/release.json --out /private/new-stage --default-dispatch-url https://download.example.invalid/dispatch.json
  node tools/release.mjs verify --stage /private/new-stage [--remote]
  node tools/release.mjs publish --stage /private/new-stage --account-id ID --artifact-bucket NAME --metadata-bucket NAME --expected-etag '"ETAG"' --confirm-publish
First publish uses --expected-etag absent. Existing R2 credentials are read only from R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY. Never paste credentials into arguments or repository files.`);
}
async function main() {
  const [command,...args] = process.argv.slice(2);
  if (!command || command === '--help') { usage(); return; }
  const {values:v} = parseArgs({args,options:{'default-dispatch-url':{type:'string'},config:{type:'string'},out:{type:'string'},stage:{type:'string'},remote:{type:'boolean'},'account-id':{type:'string'},'artifact-bucket':{type:'string'},'metadata-bucket':{type:'string'},'expected-etag':{type:'string'},'confirm-publish':{type:'boolean'}},strict:true});
  if (command === 'prepare') {
    assert(v.config && v.out && v['default-dispatch-url'],'prepare requires --config, --out and explicit --default-dispatch-url');
    const result = await prepareRelease(path.resolve(v.config),path.resolve(v.out),{defaultDispatchUrl:v['default-dispatch-url']});
    console.log(`Prepared ${result.current.releaseId}: ${result.plan.files.length} verified local files. No upload performed.`);
  } else if (command === 'verify') {
    assert(v.stage,'verify requires --stage');
    const result = await verifyLocal(path.resolve(v.stage));
    if (v.remote) await verifyRemote(result.current,result.plan);
    console.log(`Verified ${result.plan.files.length} files ${v.remote ? 'locally and from public HTTPS origin' : 'locally only'}. No upload performed.`);
  } else if (command === 'publish') {
    assert(v['confirm-publish'] && v.stage,'publish requires --stage and explicit --confirm-publish');
    assert(v['artifact-bucket'] !== v['metadata-bucket'],'public artifacts and private metadata require separate buckets');
    const options = {accountId:v['account-id'],accessKey:process.env.R2_ACCESS_KEY_ID,secretKey:process.env.R2_SECRET_ACCESS_KEY};
    const result = await publishRelease(path.resolve(v.stage),{artifacts:new R2Store({...options,bucket:v['artifact-bucket']}),metadata:new R2Store({...options,bucket:v['metadata-bucket']}),expectedETag:v['expected-etag'],confirmPublish:true});
    console.log(`Published ${result.releaseId}; current ETag ${result.etag}. Verify /dispatch.json and /release.json before announcing availability.`);
  } else throw new Error('unknown command');
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) main().catch(error=>{console.error(`Stopped: ${error.message}`);process.exitCode=1;});
